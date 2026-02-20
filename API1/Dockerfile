FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./

RUN go mod download

COPY main.go ./

RUN go build -o api1 main.go

FROM alpine:latest
WORKDIR /root/

COPY --from=builder /app/api1 .

EXPOSE 8081

CMD ["./api1"]

