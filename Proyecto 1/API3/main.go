package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	VM        string `json:"VM"`
	Carnet    string `json:"carnet"`
}

type CallResponse struct {
	APIName    string `json:"apiname"`
	Message    string `json:"message"`
	Connection bool   `json:"connection"`
	Carnet     string `json:"carnet"`
}

func main() {

	miCarnet := os.Getenv("CARNET")
	if miCarnet == "" {
		miCarnet = "202102140"
	}

	// API1 (VM1)
	api1URL := os.Getenv("API1_HEALTH_URL")
	if api1URL == "" {
		api1URL = "http://vm-api-1:8081/health"
	}

	// API2 (VM1)
	api2URL := os.Getenv("API2_HEALTH_URL")
	if api2URL == "" {
		api2URL = "http://vm-api-1:8082/health"
	}

	// 1. Endpoint /health
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		res := HealthResponse{
			Status:    "UP",
			Message:   "API3 is Ready",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			VM:        "VM2",
			Carnet:    miCarnet,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	// 2. Llamada a API1 (VM1)
	http.HandleFunc("/api3/"+miCarnet+"/call-api1", func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		callAPI(w, "API1", "VM1", api1URL, miCarnet)
	})

	// 3. Llamada a API2 (VM1)
	http.HandleFunc("/api3/"+miCarnet+"/call-api2", func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		callAPI(w, "API2", "VM1", api2URL, miCarnet)
	})

	fmt.Println("API3 iniciada en el puerto 8083...")
	if err := http.ListenAndServe(":8083", nil); err != nil {
		fmt.Println("Error iniciando servidor:", err)
	}
}

func callAPI(w http.ResponseWriter, targetName string, targetVM string, url string, carnet string) {

	w.Header().Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)

	if err != nil || resp.StatusCode != http.StatusOK {

		_ = json.NewEncoder(w).Encode(CallResponse{
			APIName:    targetName,
			Message:    "ERROR: The " + targetName + " located on the " + targetVM + " is not working",
			Connection: false,
			Carnet:     carnet,
		})
		return
	}
	defer resp.Body.Close()

	_ = json.NewEncoder(w).Encode(CallResponse{
		APIName:    targetName,
		Message:    "The " + targetName + " located on the " + targetVM + " is working",
		Connection: true,
		Carnet:     carnet,
	})
}
