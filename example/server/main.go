package main

import (
	"context"
	"fmt"
	"net"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	discovery "github.com/devhg/etcd-grpc"
	pb "github.com/devhg/etcd-grpc/example/server/proto"
)

type MailService struct {
	pb.UnimplementedMailServiceServer
	Addr string
}

func (s *MailService) SendMail(ctx context.Context, req *pb.MailRequest) (res *pb.MailResponse, err error) {
	fmt.Printf("[%s] 邮箱:%s;发送内容:%s\n", s.Addr, req.Mail, req.Text)
	return &pb.MailResponse{Ok: true}, nil
}

var etcdAddrs = []string{"127.0.0.1:123", "127.0.0.1:22379", "127.0.0.1:32379"}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// etcd中注册5个grpc服务
	var wg sync.WaitGroup
	wg.Add(5)
	go newServer(":1001", "1.0.0", 1, logger)
	go newServer(":1002", "1.0.0", 1, logger)
	go newServer(":1003", "1.0.0", 1, logger)
	go newServer(":1004", "1.0.0", 1, logger)
	go newServer(":1006", "1.0.0", 10, logger)
	wg.Wait()
}

func newServer(port string, version string, weight int64, logger *zap.Logger) {
	addr := "127.0.0.1" + port
	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatal("failed to listen %v", zap.Error(err))
	}
	s := grpc.NewServer()                                     // 创建gRPC服务器
	pb.RegisterMailServiceServer(s, &MailService{Addr: addr}) // 在gRPC服务端注册服务
	reflection.Register(s)                                    // 在给定的gRPC服务器上注册服务器反射服务

	// 服务信息
	srvInfo := discovery.ServiceInfo{
		Name:    "g.srv.mail",
		Addr:    addr,
		Version: version,
		Weight:  weight,
	}

	// grpc 服务注册
	register := discovery.NewRegister(etcdAddrs, logger)
	defer register.Stop()

	err = register.Start(srvInfo, 1)
	if err != nil {
		logger.Fatal("", zap.Error(err))
	}

	logger.Info("run server at", zap.String("addr", srvInfo.Addr))
	if err := s.Serve(lis); err != nil {
		logger.Fatal("failed to server %v", zap.Error(err))
	}
}
