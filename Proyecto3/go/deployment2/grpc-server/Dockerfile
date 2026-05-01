FROM golang:1.24-alpine AS builder
RUN apk add --no-cache protoc
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
ENV PATH="$PATH:/go/bin"
WORKDIR /app

COPY go/deployment2/grpc-server/go.mod ./
COPY go/deployment2/grpc-server/main.go ./
COPY go/proto/ ./proto/

RUN protoc --go_out=. --go-grpc_out=. proto/report.proto
RUN go mod tidy
RUN go build -o grpc_server_bin main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/grpc_server_bin .
EXPOSE 50051
CMD ["./grpc_server_bin"]

