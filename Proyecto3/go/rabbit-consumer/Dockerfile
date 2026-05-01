FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY go/rabbit-consumer/go.mod ./
COPY go/rabbit-consumer/main.go ./

RUN go mod tidy
RUN go build -o rabbit_consumer_bin main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/rabbit_consumer_bin .
CMD ["./rabbit_consumer_bin"]

