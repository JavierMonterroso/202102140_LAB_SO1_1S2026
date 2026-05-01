FROM golang:1.24-alpine AS builder
RUN apk add --no-cache protoc
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
ENV PATH="$PATH:/go/bin"
WORKDIR /app

COPY go/deployment1/grpc-client/go.mod ./
COPY go/deployment1/grpc-client/main.go ./
COPY go/proto/ ./proto/

RUN protoc --go_out=. --go-grpc_out=. proto/report.proto
RUN go mod tidy
RUN go build -o grpc_client_bin main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/grpc_client_bin .
EXPOSE 50052
CMD ["./grpc_client_bin"]
