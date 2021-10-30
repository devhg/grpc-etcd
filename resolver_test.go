package discovery

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"

	pb "github.com/devhg/etcd-grpc/proto"
)

var etcdAddrs = []string{"127.0.0.1:123", "127.0.0.1:22379", "127.0.0.1:32379"}

func TestResolver(t *testing.T) {
	// etcd中注册5个grpc服务
	go newServer(t, ":1001", "1.0.0", 1)
	go newServer(t, ":1002", "1.0.0", 1)
	go newServer(t, ":1003", "1.0.0", 1)
	go newServer(t, ":1004", "1.0.0", 1)
	go newServer(t, ":1006", "1.0.0", 10)

	// server 启动有一顶延时
	time.Sleep(time.Second)

	r := NewResolver(etcdAddrs, "g.srv.mail", zap.NewNop())
	resolver.Register(r)

	/*g.srv.mail经测试，这个可以随便写，底层只是取scheme对应的Build对象*/
	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
	addr := fmt.Sprintf("%s:///%s", r.Scheme(), "g.srv.mail")
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithInsecure(),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
		grpc.WithBlock())

	if err != nil {
		t.Fatalf("failed to dial %v", err)
	}
	defer conn.Close()

	c := pb.NewMailServiceClient(conn)

	// 进行十次数据请求
	for i := 0; i < 10; i++ {
		resp, err := c.SendMail(context.Background(), &pb.MailRequest{Mail: "a@b.c", Text: "你好！"})
		if err != nil || !resp.Ok {
			t.Fatalf("say hello failed %v %v", err, resp)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type MailService struct {
	pb.UnimplementedMailServiceServer
	Addr string
}

func (m *MailService) SendMail(ctx context.Context, req *pb.MailRequest) (*pb.MailResponse, error) {
	fmt.Printf("[%s] 邮箱:%s;发送内容:%s\n", m.Addr, req.Mail, req.Text)
	return &pb.MailResponse{Ok: true}, nil
}

func newServer(t *testing.T, port string, version string, weight int64) {
	srvInfo := ServiceInfo{
		Name:    "g.srv.mail",
		Addr:    "127.0.0.1" + port,
		Version: version,
		Weight:  weight,
	}

	register := NewRegister(etcdAddrs, zap.NewNop())
	defer register.Stop()

	err := register.Start(srvInfo, 1)
	if err != nil {
		t.Fatal(err)
	}

	lis, err := net.Listen("tcp", port)
	if err != nil {
		t.Fatalf("failed to listen %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterMailServiceServer(s, &MailService{Addr: srvInfo.Addr})

	t.Log("run server at", srvInfo.Addr)
	if err := s.Serve(lis); err != nil {
		t.Fatalf("failed to server %v", err)
	}
}
