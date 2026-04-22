package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "cliente/proto"
)

type ReporteGuerra struct {
	Country         string `json:"country"`
	WarplanesInAir  int32  `json:"warplanes_in_air"`
	WarshipsInWater int32  `json:"warships_in_water"`
	Timestamp       string `json:"timestamp"`
}

type Respuesta struct {
	Estado  string `json:"estado"`
	Mensaje string `json:"mensaje"`
}

func manejarSalud(respuesta http.ResponseWriter, solicitud *http.Request) {
	respuesta.WriteHeader(http.StatusOK)
	_, _ = respuesta.Write([]byte("OK"))
}

func manejarProcesar(respuesta http.ResponseWriter, solicitud *http.Request) {
	if solicitud.Method != http.MethodPost {
		respuesta.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var reporte ReporteGuerra
	errorDecodificacion := json.NewDecoder(solicitud.Body).Decode(&reporte)
	if errorDecodificacion != nil {
		respuesta.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(respuesta).Encode(Respuesta{
			Estado:  "error",
			Mensaje: "JSON inválido",
		})
		return
	}

	conexion, errorConexion := grpc.Dial(
		"127.0.0.1:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if errorConexion != nil {
		respuesta.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(respuesta).Encode(Respuesta{
			Estado:  "error",
			Mensaje: "No se pudo conectar al servidor gRPC",
		})
		return
	}
	defer conexion.Close()

	clienteGrpc := pb.NewServicioReporteGuerraClient(conexion)

	ctx, cancelacion := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelacion()

	respuestaGrpc, errorGrpc := clienteGrpc.EnviarReporte(ctx, &pb.ReporteGuerraRequest{
		Country:         reporte.Country,
		WarplanesInAir:  reporte.WarplanesInAir,
		WarshipsInWater: reporte.WarshipsInWater,
		Timestamp:       reporte.Timestamp,
	})

	if errorGrpc != nil {
		log.Printf("Error enviando por gRPC: %v", errorGrpc)
		respuesta.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(respuesta).Encode(Respuesta{
			Estado:  "error",
			Mensaje: "Falló el envío al servidor gRPC",
		})
		return
	}

	log.Printf(
		"Cliente Go reenvió reporte: país=%s aviones=%d barcos=%d",
		reporte.Country,
		reporte.WarplanesInAir,
		reporte.WarshipsInWater,
	)

	respuesta.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(respuesta).Encode(Respuesta{
		Estado:  respuestaGrpc.Estado,
		Mensaje: respuestaGrpc.Mensaje,
	})
}

func main() {
	http.HandleFunc("/health", manejarSalud)
	http.HandleFunc("/procesar", manejarProcesar)

	log.Println("Cliente Go escuchando en puerto 8081")

	errorServidor := http.ListenAndServe(":8081", nil)
	if errorServidor != nil {
		log.Fatalf("No se pudo iniciar cliente Go: %v", errorServidor)
	}
}