#https://grpc.io/docs/languages/go/quickstart/
#protoc --proto_path=./proto --go_out=plugins=grpc:./proto ./proto/*.proto
#protoc --proto_path=./proto --go-grpc_out=. ./proto/*.proto

protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    ./proto/*.proto
