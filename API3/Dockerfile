FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o api2 .

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /app/api2 /api2

EXPOSE 8083
USER nonroot:nonroot
ENTRYPOINT ["/api3"]
