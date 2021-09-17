cd example
protoc --proto_path=./proto --go_out=plugins=grpc:./proto ./proto/*.proto

# protoc  --go_out=. ./proto/*.proto