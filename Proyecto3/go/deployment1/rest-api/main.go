package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type WarReport struct {
	Country         string `json:"country"`
	WarplanesInAir  int32  `json:"warplanes_in_air"`
	WarshipsInWater int32  `json:"warships_in_water"`
	Timestamp       string `json:"timestamp"`
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func main() {
	grpcClientURL := envOrDefault("GRPC_CLIENT_URL", "http://localhost:50052/send")
	client := &http.Client{Timeout: 5 * time.Second}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	http.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "metodo no permitido", http.StatusMethodNotAllowed)
			return
		}

		var report WarReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			http.Error(w, fmt.Sprintf("json invalido: %v", err), http.StatusBadRequest)
			return
		}

		log.Printf("[GO-REST][1] Recibido desde Rust: pais=%s aviones=%d barcos=%d timestamp=%s",
			report.Country, report.WarplanesInAir, report.WarshipsInWater, report.Timestamp)

		body, err := json.Marshal(report)
		if err != nil {
			http.Error(w, fmt.Sprintf("no se pudo serializar reporte: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("[GO-REST][2] Enviando al container interno go-grpc-client: %s", grpcClientURL)

		req, err := http.NewRequest(http.MethodPost, grpcClientURL, bytes.NewReader(body))
		if err != nil {
			http.Error(w, fmt.Sprintf("no se pudo crear request interno: %v", err), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		res, err := client.Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("fallo llamada al grpc-client: %v", err), http.StatusBadGateway)
			return
		}
		defer res.Body.Close()

		resBody, _ := io.ReadAll(res.Body)
		if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
			http.Error(w, fmt.Sprintf("grpc-client respondio %s: %s", res.Status, string(resBody)), http.StatusBadGateway)
			return
		}

		log.Printf("[GO-REST][3] go-grpc-client respondio %s: %s", res.Status, string(resBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(resBody)
	})

	log.Println("[GO-REST][BOOT] REST API Go escuchando en :8081")
	log.Printf("[GO-REST][BOOT] Siguiente salto interno: %s", grpcClientURL)
	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatalf("fallo REST API Go: %v", err)
	}
}
