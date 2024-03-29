package main

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"

	discovery "github.com/devhg/etcd-grpc"
	pb "github.com/devhg/etcd-grpc/example/server/proto"
)

var etcdAddrs = []string{"127.0.0.1:123", "127.0.0.1:22379", "127.0.0.1:32379"}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	sugar := logger.Sugar()

	r := discovery.NewResolver(etcdAddrs, "g.srv.mail", logger)
	resolver.Register(r)

	// https://github.com/grpc/grpc/blob/master/doc/naming.md
	// The gRPC client library will use the specified scheme to pick the right resolver plugin and pass it the fully qualified name string.

	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
	addr := fmt.Sprintf("%s:///%s", r.Scheme(), "g.srv.mail" /*g.srv.mail经测试，这个可以随便写，底层只是取scheme对应的Build对象*/)

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithInsecure(),
		// grpc.WithBalancerName(roundrobin.Name),
		// 指定初始化round_robin => balancer (后续可以自行定制balancer和 register、resolver 同样的方式)
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
		grpc.WithBlock())

	// 这种方式也行
	// conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithBalancerName("round_robin"))

	if err != nil {
		sugar.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	/*conn, err := grpc.Dial(
	      fmt.Sprintf("%s://%s/%s", "consul", GetConsulHost(), s.Name),
	      //不能block => blockkingPicker打开，在调用轮询时picker_wrapper => picker时若block则不进行robin操作直接返回失败
	      //grpc.WithBlock(),
	      grpc.WithInsecure(),
	      //指定初始化round_robin => balancer (后续可以自行定制balancer和 register、resolver 同样的方式)
	      grpc.WithBalancerName(roundrobin.Name),
	      //grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
	  )
	  //原文链接：https://blog.csdn.net/qq_35916684/article/details/104055246
	*/

	c := pb.NewMailServiceClient(conn)
	// 进行十次数据请求
	for i := 0; i < 10; i++ {
		resp, err := c.SendMail(context.Background(), &pb.MailRequest{Mail: "a@b.c", Text: "你好！"})
		if err != nil {
			sugar.Fatalf("say hello failed %v %v", err, resp)
		}
		sugar.Info(resp)
		time.Sleep(100 * time.Millisecond)
	}
}
