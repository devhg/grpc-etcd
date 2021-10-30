# grpc-etcd

grpc 使用 etcd 做注册中心 demo

[example](./example)

* register.go 用于server注册自己到etcd，注册时key采用特殊Prefix
* resolver.go 用于client从etcd获取特定Prefix的kv(server列表)，过程中可以自定义负载均衡策略