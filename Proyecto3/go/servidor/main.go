package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"github.com/rabbitmq/amqp091-go"

	pb "servidor/proto"
)

type MensajeCola struct {
	Country         string `json:"country"`
	WarplanesInAir  int32  `json:"warplanes_in_air"`
	WarshipsInWater int32  `json:"warships_in_water"`
	Timestamp       string `json:"timestamp"`
}

type ServidorReporte struct {
	pb.UnimplementedServicioReporteGuerraServer
	canalRabbit *amqp091.Channel
	nombreCola  string
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

	mensaje := MensajeCola{
		Country:         reporte.Country,
		WarplanesInAir:  reporte.WarplanesInAir,
		WarshipsInWater: reporte.WarshipsInWater,
		Timestamp:       reporte.Timestamp,
	}

	cuerpo, errorJson := json.Marshal(mensaje)
	if errorJson != nil {
		log.Printf("No se pudo convertir el reporte a JSON: %v", errorJson)
		return &pb.ReporteGuerraResponse{
			Estado:  "error",
			Mensaje: "No se pudo serializar el mensaje",
		}, nil
	}

	errorPublicacion := s.canalRabbit.PublishWithContext(
		ctx,
		"",            // exchange
		s.nombreCola,  // routing key = nombre cola
		false,
		false,
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        cuerpo,
		},
	)

	if errorPublicacion != nil {
		log.Printf("No se pudo publicar en RabbitMQ: %v", errorPublicacion)
		return &pb.ReporteGuerraResponse{
			Estado:  "error",
			Mensaje: "No se pudo publicar en RabbitMQ",
		}, nil
	}

	log.Printf("Reporte publicado en RabbitMQ en la cola '%s'", s.nombreCola)

	return &pb.ReporteGuerraResponse{
		Estado:  "ok",
		Mensaje: "Reporte recibido y publicado en RabbitMQ",
	}, nil
}

func main() {
	urlRabbit := os.Getenv("RABBITMQ_URL")
	if urlRabbit == "" {
		urlRabbit = "amqp://guest:guest@localhost:5672/"
	}

	nombreCola := "reportes"

	conexionRabbit, errorConexionRabbit := amqp091.Dial(urlRabbit)
	if errorConexionRabbit != nil {
		log.Fatalf("No se pudo conectar a RabbitMQ: %v", errorConexionRabbit)
	}
	defer conexionRabbit.Close()

	canalRabbit, errorCanalRabbit := conexionRabbit.Channel()
	if errorCanalRabbit != nil {
		log.Fatalf("No se pudo abrir canal de RabbitMQ: %v", errorCanalRabbit)
	}
	defer canalRabbit.Close()

	_, errorCola := canalRabbit.QueueDeclare(
		nombreCola,
		true,  // durable
		false, // auto delete
		false, // exclusive
		false, // no wait
		nil,
	)
	if errorCola != nil {
		log.Fatalf("No se pudo declarar la cola '%s': %v", nombreCola, errorCola)
	}

	escucha, errorEscucha := net.Listen("tcp", ":50051")
	if errorEscucha != nil {
		log.Fatalf("No se pudo escuchar en puerto 50051: %v", errorEscucha)
	}

	servidor := &ServidorReporte{
		canalRabbit: canalRabbit,
		nombreCola:  nombreCola,
	}

	servidorGrpc := grpc.NewServer()
	pb.RegisterServicioReporteGuerraServer(servidorGrpc, servidor)

	log.Println("Servidor gRPC escuchando en puerto 50051")
	log.Printf("Conectado a RabbitMQ en %s", urlRabbit)
	log.Printf("Publicando mensajes en la cola '%s'", nombreCola)

	errorServidor := servidorGrpc.Serve(escucha)
	if errorServidor != nil {
		log.Fatalf("No se pudo iniciar el servidor gRPC: %v", errorServidor)
	}
}