package discovery

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

const schema = "etcd"

// Resolver 实现grpc的grpc.resolve.Builder接口的Build与Scheme方法
type Resolver struct {
	service     string
	schema      string
	DialTimeout int

	// etcd Addrs
	endpoints []string

	cli         *clientv3.Client
	cc          resolver.ClientConn
	keyPrefix   string
	srvAddrList []resolver.Address

	closeCh chan struct{}
	watchCh clientv3.WatchChan

	logger *zap.Logger
}

// NewResolver return resolver builder
// endpoints example: http://127.0.0.1:2379 http://127.0.0.1:12379 http://127.0.0.1:22379"
// service is service name
func NewResolver(endpoints []string, service string, logger *zap.Logger) resolver.Builder {
	return &Resolver{schema: schema, endpoints: endpoints, service: service, logger: logger}
}

// Scheme return etcd schema
func (r *Resolver) Scheme() string {
	// 最好用这种，因为grpc resolver.Register(r)在注册时，会取scheme，如果一个系统有多个grpc发现，就会覆盖之前注册的
	return schema + "_" + r.service
}

// ResolveNow .
func (r *Resolver) ResolveNow(rn resolver.ResolveNowOptions) {}

// Close .
func (r *Resolver) Close() {
	r.closeCh <- struct{}{}
}

// Build to resolver.Resolver
// 实现grpc.resolve.Builder接口的方法
func (r *Resolver) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r.cc = cc
	r.keyPrefix = r.service

	if _, err := r.start(); err != nil {
		r.logger.Error("Build", zap.Error(err))
		return nil, err
	}

	return r, nil
}

func (r *Resolver) start() (chan<- struct{}, error) {
	var err error
	r.cli, err = clientv3.New(clientv3.Config{
		Endpoints:   r.endpoints,
		DialTimeout: time.Duration(r.DialTimeout) * time.Second,
	})
	if err != nil {
		r.logger.Error("start", zap.Error(err))
		return nil, err
	}

	r.closeCh = make(chan struct{})

	// 同步获取所有地址信息
	if err := r.sync(); err != nil {
		r.logger.Error("sync", zap.Error(err))
		return nil, err
	}

	go r.watch()

	return r.closeCh, nil
}

// sync 同步获取所有地址信息
func (r *Resolver) sync() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := r.cli.Get(ctx, r.keyPrefix, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	r.srvAddrList = make([]resolver.Address, 0)

	for _, kv := range resp.Kvs {
		info, err := parseValue(kv.Value)
		if err != nil {
			continue
		}
		addr := resolver.Address{
			Addr:       info.Addr,
			Attributes: attributes.New("weight", info.Weight),
		}
		r.srvAddrList = append(r.srvAddrList, addr)
	}

	// 核心：通过这个告诉grpc 有哪些addr
	return r.cc.UpdateState(resolver.State{Addresses: r.srvAddrList})
}

func (r *Resolver) watch() {
	// 返回一个channel，我们可以监听这个channel
	watchCh := r.cli.Watch(context.Background(), r.keyPrefix, clientv3.WithPrefix() /*clientv3.WithPrevKV()*/)
	ticker := time.NewTicker(time.Minute)
	for {
		select {
		case <-r.closeCh:
			return
		case evResp, ok := <-watchCh:
			if ok {
				r.update(evResp.Events)
			}
		case <-ticker.C:
			if err := r.sync(); err != nil {
				r.logger.Error("sync failed", zap.Error(err))
			}
		}
	}
}

func (r *Resolver) update(events []*clientv3.Event) {
	addrDict := make(map[string]resolver.Address)
	for _, addr := range r.srvAddrList {
		addrDict[addr.Addr] = addr
	}

	// 核心：通过这个告诉grpc 有哪些addr
	update := func() {
		addrList := make([]resolver.Address, 0, len(addrDict))
		for _, v := range addrDict {
			addrList = append(addrList, v)
		}
		r.srvAddrList = addrList

		err := r.cc.UpdateState(resolver.State{Addresses: r.srvAddrList})
		if err != nil {
			r.logger.Error("update state failed", zap.Error(err))
		}
		log.Println(r.srvAddrList)
	}

	for _, ev := range events {
		switch ev.Type {
		case mvccpb.PUT:
			info, err := parseValue(ev.Kv.Value)
			if err != nil {
				r.logger.Error("put failed", zap.Error(err))
			} else {
				addrDict[string(ev.Kv.Key)] = resolver.Address{
					Addr:       info.Addr,
					Attributes: attributes.New("weight", info.Weight),
				}
			}
		case mvccpb.DELETE:
			delete(addrDict, string(ev.Kv.Key))
		}
	}
	update()
}

func parseValue(value []byte) (ServiceInfo, error) {
	info := ServiceInfo{}
	if err := json.Unmarshal(value, &info); err != nil {
		return ServiceInfo{}, err
	}
	return info, nil
}
