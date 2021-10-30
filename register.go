package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ServiceInfo 服务信息
type ServiceInfo struct {
	Name    string `json:"name"`
	Addr    string `json:"addr"`    // 服务地址
	Version string `json:"version"` // 服务版本
	Weight  int64  `json:"weight"`  // 服务权重
}

// Register registers ServiceInfo to etcd，
// Each server will have a register entity
// it can keepAlive by etcd
type Register struct {
	SrvInfo     ServiceInfo
	EtcdAddrs   []string
	DialTimeout int

	stop        chan error
	leaseID     clientv3.LeaseID
	keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse

	client *clientv3.Client
	srvTTL int64
	logger *zap.Logger
}

// NewRegister create a register
func NewRegister(endpoints []string, logger *zap.Logger) *Register {
	return &Register{
		EtcdAddrs:   endpoints,
		DialTimeout: 3,
		logger:      logger,
		stop:        make(chan error),
	}
}

// Start 注册服务启动，注册进etcd并创建租约
func (r *Register) Start(srvInfo ServiceInfo, ttl int64) error {
	var err error

	r.client, err = clientv3.New(clientv3.Config{
		Endpoints:   r.EtcdAddrs,
		DialTimeout: time.Second * 200,
	})
	if err != nil {
		return err
	}

	r.SrvInfo = srvInfo
	r.srvTTL = ttl

	// register SrvInfo
	if err = r.register(); err != nil {
		return err
	}

	go r.keepAlive()

	return nil
}

// Stop 停止服务，从etcd取消注册和撤销租约
func (r *Register) Stop() {
	// 取消注册
	if err := r.unregister(); err != nil {
		r.logger.Error("unregister failed", zap.Error(err))
	}

	// 取消任期
	if err := r.revoke(); err != nil {
		r.logger.Error("revoke failed", zap.Error(err))
	}
	r.stop <- nil
}

// keepAlive 保持自动续约
func (r *Register) keepAlive() {
	ticker := time.NewTicker(time.Duration(r.srvTTL) * time.Second)
	for {
		select {
		case <-r.stop:
			return
		case res := <-r.keepAliveCh:
			r.logger.Info("keepAlive",
				zap.String(r.SrvInfo.Addr, strconv.FormatInt(res.TTL, 10)))
			if res == nil {
				if err := r.register(); err != nil {
					r.logger.Error("register failed", zap.Error(err))
				}
			}
		case <-ticker.C:
			if r.keepAliveCh == nil {
				if err := r.register(); err != nil {
					r.logger.Error("register failed", zap.Error(err))
				}
			}
		}
	}
}

// register 注册进etcd
func (r *Register) register() error {
	key := r.getKey() // g.srv.mail/127.0.0.1:8972
	data, _ := json.Marshal(r.SrvInfo)
	r.logger.Info("register", zap.String(key, string(data)))

	leaseCtx, cancel := context.WithTimeout(context.Background(), time.Duration(r.DialTimeout)*time.Second)
	defer cancel()

	// 创建一个租约
	leaseResp, err := r.client.Grant(leaseCtx, r.srvTTL)
	if err != nil {
		return err
	}

	_, err = r.client.Put(context.Background(), key, string(data), clientv3.WithLease(leaseResp.ID))
	if err != nil {
		return err
	}

	r.leaseID = leaseResp.ID
	r.keepAliveCh, err = r.client.KeepAlive(context.Background(), leaseResp.ID)
	return nil
}

// unregister 从etcd删除当前
func (r *Register) unregister() error {
	_, err := r.client.Delete(context.Background(), r.getKey())
	return err
}

// revoke revokes the given lease.
func (r *Register) revoke() error {
	if _, err := r.client.Revoke(context.Background(), r.leaseID); err != nil {
		return err
	}
	r.logger.Info("service stop", zap.String("key", r.getKey()))
	return nil
}

// getKey 获取etcd的key
func (r *Register) getKey() string {
	return r.SrvInfo.Name + "/" + r.SrvInfo.Addr
}

// UpdateHandler return http handler 可以设置权重的handler
func (r *Register) UpdateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		wi := req.URL.Query().Get("weight")
		weight, err := strconv.Atoi(wi)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		var update = func() error {
			r.SrvInfo.Weight = int64(weight)
			data, err := json.Marshal(r.SrvInfo)
			if err != nil {
				return err
			}
			_, err = r.client.Put(context.Background(), r.getKey(), string(data), clientv3.WithLease(r.leaseID))
			return err
		}

		if err := update(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.Write([]byte("update server weight success"))
	}
}

// GetServerInfo 获取已注册的server信息
func (r *Register) GetServerInfo() (ServiceInfo, error) {
	resp, err := r.client.Get(context.Background(), r.getKey())
	if err != nil {
		return r.SrvInfo, err
	}

	info := ServiceInfo{}
	if resp.Count >= 1 {
		if err := json.Unmarshal(resp.Kvs[0].Value, &info); err != nil {
			return info, err
		}
	}
	return info, nil
}
