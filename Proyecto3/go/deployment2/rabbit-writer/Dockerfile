FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY go/deployment2/rabbit-writer/go.mod ./
COPY go/deployment2/rabbit-writer/main.go ./

RUN go mod tidy
RUN go build -o rabbit_writer_bin main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/rabbit_writer_bin .
EXPOSE 50053
CMD ["./rabbit_writer_bin"]

