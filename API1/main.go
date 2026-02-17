package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Estructura para el endpoint /health
type HealthResponse struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	VM        string `json:"VM"`
	Carnet    string `json:"carnet"`
}

// Estructura para llamadas entre APIs
type CallResponse struct {
	APIName    string `json:"apiname"`
	Message    string `json:"message"`
	Connection bool   `json:"connection"`
	Carnet     string `json:"carnet"`
}

func main() {
	// Carnet por ENV (con fallback)
	miCarnet := os.Getenv("CARNET")
	if miCarnet == "" {
		miCarnet = "202102140" // default
	}

	// URLs por ENV (con fallback)
	api2URL := os.Getenv("API2_HEALTH_URL")
	if api2URL == "" {
		api2URL = "http://localhost:8082/health"
	}

	api3URL := os.Getenv("API3_HEALTH_URL")
	if api3URL == "" {
		api3URL = "http://192.168.122.X:8083/health" // cambia X o exporta API3_HEALTH_URL
	}

	// 1. Endpoint de Salud
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		res := HealthResponse{
			Status:    "UP",
			Message:   "API1 is Ready",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			VM:        "VM1",
			Carnet:    miCarnet,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	// 2. Llamada a API2
	http.HandleFunc("/api1/"+miCarnet+"/call-api2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		callAPI(w, "API2", api2URL, miCarnet)
	})

	// 3. Llamada a API3
	http.HandleFunc("/api1/"+miCarnet+"/call-api3", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		callAPI(w, "API3", api3URL, miCarnet)
	})

	fmt.Println("API1 iniciada en el puerto 8081...")
	logErr := http.ListenAndServe(":8081", nil)
	if logErr != nil {
		fmt.Println("Error iniciando servidor:", logErr)
	}
}

// Función auxiliar para realizar las peticiones HTTP internas
func callAPI(w http.ResponseWriter, targetName string, url string, carnet string) {
	w.Header().Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)

	if err != nil || resp.StatusCode != http.StatusOK {
		// Manejo de error si la otra API no responde
		w.WriteHeader(http.StatusServiceUnavailable)

		_ = json.NewEncoder(w).Encode(CallResponse{
			APIName:    targetName,
			Message:    "ERROR: The " + targetName + " located on its VM is not working",
			Connection: false,
			Carnet:     carnet,
		})
		return
	}
	defer resp.Body.Close()

	// Si responde bien, devolvemos el éxito
	_ = json.NewEncoder(w).Encode(CallResponse{
		APIName:    targetName,
		Message:    "The " + targetName + " located on its VM is working",
		Connection: true,
		Carnet:     carnet,
	})
}
