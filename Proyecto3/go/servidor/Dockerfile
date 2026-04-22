FROM golang:1.24-alpine AS builder

RUN apk add --no-cache protoc
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

ENV PATH="$PATH:/go/bin"
WORKDIR /app

COPY servidor/go.mod ./go.mod
COPY servidor/main.go ./main.go
COPY proto/ ./proto/

RUN protoc --go_out=. --go-grpc_out=. proto/reporte.proto
RUN go mod tidy
RUN go build -o servidor_bin main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/servidor_bin .
EXPOSE 50051
CMD ["./servidor_bin"]