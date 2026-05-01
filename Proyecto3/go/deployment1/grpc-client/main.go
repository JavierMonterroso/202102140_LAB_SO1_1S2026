package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	pb "go-grpc-client/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type WarReport struct {
	Country         string `json:"country"`
	WarplanesInAir  int32  `json:"warplanes_in_air"`
	WarshipsInWater int32  `json:"warships_in_water"`
	Timestamp       string `json:"timestamp"`
}

func normalizeCountry(country string) string {
	switch strings.ToUpper(country) {
	case "USA":
		return "usa"
	case "RUS":
		return "rus"
	case "CHN":
		return "chn"
	case "ESP":
		return "esp"
	case "GTM", "GMT":
		return "gmt"
	default:
		return "countries_unknown"
	}
}

func countryToProto(country string) pb.Countries {
	switch normalizeCountry(country) {
	case "usa":
		return pb.Countries_usa
	case "rus":
		return pb.Countries_rus
	case "chn":
		return pb.Countries_chn
	case "esp":
		return pb.Countries_esp
	case "gmt":
		return pb.Countries_gmt
	default:
		return pb.Countries_countries_unknown
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func main() {
	grpcServerAddr := envOrDefault("GRPC_SERVER_ADDR", "go-processing-service:50051")

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "metodo no permitido", http.StatusMethodNotAllowed)
			return
		}

		var report WarReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			http.Error(w, fmt.Sprintf("json invalido: %v", err), http.StatusBadRequest)
			return
		}

		protoCountry := normalizeCountry(report.Country)

		log.Printf("[GO-GRPC-CLIENT][1] Recibido desde go-rest-api: pais=%s aviones=%d barcos=%d timestamp=%s",
			report.Country, report.WarplanesInAir, report.WarshipsInWater, report.Timestamp)
		log.Printf("[GO-GRPC-CLIENT][2] Convirtiendo a WarReportRequest: country_enum=%s warplanes_in_air=%d warships_in_water=%d",
			protoCountry, report.WarplanesInAir, report.WarshipsInWater)

		conn, err := grpc.Dial(grpcServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			http.Error(w, fmt.Sprintf("no se pudo conectar al grpc-server: %v", err), http.StatusBadGateway)
			return
		}
		defer conn.Close()

		client := pb.NewWarReportServiceClient(conn)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		res, err := client.SendReport(ctx, &pb.WarReportRequest{
			Country:         countryToProto(report.Country),
			WarplanesInAir:  report.WarplanesInAir,
			WarshipsInWater: report.WarshipsInWater,
			Timestamp:       report.Timestamp,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("fallo llamada gRPC a %s: %v", grpcServerAddr, err), http.StatusBadGateway)
			return
		}

		log.Printf("[GO-GRPC-CLIENT][3] grpc-server respondio status=%s", res.Status)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"%s","country":"%s"}`, res.Status, protoCountry)
	})

	log.Println("[GO-GRPC-CLIENT][BOOT] API interna escuchando en :50052")
	log.Printf("[GO-GRPC-CLIENT][BOOT] Siguiente salto gRPC: %s", grpcServerAddr)
	if err := http.ListenAndServe(":50052", nil); err != nil {
		log.Fatalf("fallo grpc-client interno: %v", err)
	}
}
