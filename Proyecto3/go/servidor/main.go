package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"

	pb "servidor/proto"
)

type ServidorReporte struct {
	pb.UnimplementedServicioReporteGuerraServer
}

func (s *ServidorReporte) EnviarReporte(
	ctx context.Context,
	reporte *pb.ReporteGuerraRequest,
) (*pb.ReporteGuerraResponse, error) {
	log.Printf(
		"Servidor gRPC recibió reporte: país=%s aviones=%d barcos=%d timestamp=%s",
		reporte.Country,
		reporte.WarplanesInAir,
		reporte.WarshipsInWater,
		reporte.Timestamp,
	)

	return &pb.ReporteGuerraResponse{
		Estado:  "ok",
		Mensaje: "Reporte recibido por servidor gRPC",
	}, nil
}

func main() {
	escucha, errorEscucha := net.Listen("tcp", ":50051")
	if errorEscucha != nil {
		log.Fatalf("No se pudo escuchar en puerto 50051: %v", errorEscucha)
	}

	servidorGrpc := grpc.NewServer()
	pb.RegisterServicioReporteGuerraServer(servidorGrpc, &ServidorReporte{})

	log.Println("Servidor gRPC escuchando en puerto 50051")

	errorServidor := servidorGrpc.Serve(escucha)
	if errorServidor != nil {
		log.Fatalf("No se pudo iniciar el servidor gRPC: %v", errorServidor)
	}
}