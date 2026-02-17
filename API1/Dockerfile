FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o api1 .

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /app/api1 /api1

EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/api1"]


