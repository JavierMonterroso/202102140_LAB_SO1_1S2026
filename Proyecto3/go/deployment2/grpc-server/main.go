package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	pb "go-grpc-server/proto"

	"google.golang.org/grpc"
)

type WarReport struct {
	Country         string `json:"country"`
	WarplanesInAir  int32  `json:"warplanes_in_air"`
	WarshipsInWater int32  `json:"warships_in_water"`
	Timestamp       string `json:"timestamp"`
}

type server struct {
	pb.UnimplementedWarReportServiceServer
	writerURL string
	client    *http.Client
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func (s *server) SendReport(ctx context.Context, req *pb.WarReportRequest) (*pb.WarReportResponse, error) {
	report := WarReport{
		Country:         req.Country.String(),
		WarplanesInAir:  req.WarplanesInAir,
		WarshipsInWater: req.WarshipsInWater,
		Timestamp:       req.Timestamp,
	}

	log.Printf("[GO-GRPC-SERVER][1] Recibido por gRPC: pais=%s aviones=%d barcos=%d timestamp=%s",
		report.Country, report.WarplanesInAir, report.WarshipsInWater, report.Timestamp)

	body, err := json.Marshal(report)
	if err != nil {
		return &pb.WarReportResponse{Status: "json-error"}, nil
	}

	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, s.writerURL, bytes.NewReader(body))
	if err != nil {
		return &pb.WarReportResponse{Status: "writer-request-error"}, nil
	}
	reqHTTP.Header.Set("Content-Type", "application/json")

	res, err := s.client.Do(reqHTTP)
	if err != nil {
		log.Printf("[GO-GRPC-SERVER][ERROR] No se pudo llamar al writer: %v", err)
		return &pb.WarReportResponse{Status: "writer-unavailable"}, nil
	}
	defer res.Body.Close()

	resBody, _ := io.ReadAll(res.Body)
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		log.Printf("[GO-GRPC-SERVER][ERROR] Writer respondio %s: %s", res.Status, string(resBody))
		return &pb.WarReportResponse{Status: fmt.Sprintf("writer-%d", res.StatusCode)}, nil
	}

	log.Printf("[GO-GRPC-SERVER][2] Writer RabbitMQ respondio %s: %s", res.Status, string(resBody))
	return &pb.WarReportResponse{Status: "published"}, nil
}

func main() {
	writerURL := envOrDefault("WRITER_URL", "http://localhost:50053/publish")

	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("no se pudo escuchar en :50051: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterWarReportServiceServer(grpcServer, &server{
		writerURL: writerURL,
		client:    &http.Client{Timeout: 5 * time.Second},
	})

	log.Println("[GO-GRPC-SERVER][BOOT] Escuchando gRPC en :50051")
	log.Printf("[GO-GRPC-SERVER][BOOT] Writer interno: %s", writerURL)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("fallo grpc-server: %v", err)
	}
}
