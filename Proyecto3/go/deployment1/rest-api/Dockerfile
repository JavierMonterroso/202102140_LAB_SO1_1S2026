FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY go/deployment1/rest-api/go.mod ./
COPY go/deployment1/rest-api/main.go ./

RUN go mod tidy
RUN go build -o rest_api_bin main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/rest_api_bin .
EXPOSE 8081
CMD ["./rest_api_bin"]
