package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

type ReporteGuerra struct {
	Country          string `json:"country"`
	WarplanesInAir  int32  `json:"warplanes_in_air"`
	WarshipsInWater int32  `json:"warships_in_water"`
	Timestamp        string `json:"timestamp"`
}

func main() {
	ctx := context.Background()

	urlRabbit := os.Getenv("RABBITMQ_URL")
	if urlRabbit == "" {
		urlRabbit = "amqp://guest:guest@localhost:5672/"
	}

	urlValkey := os.Getenv("VALKEY_URL")
	if urlValkey == "" {
		urlValkey = "localhost:6379"
	}

	nombreCola := "reportes"

	conexionRabbit, err := amqp091.Dial(urlRabbit)
	if err != nil {
		log.Fatalf("No se pudo conectar a RabbitMQ: %v", err)
	}
	defer conexionRabbit.Close()

	canalRabbit, err := conexionRabbit.Channel()
	if err != nil {
		log.Fatalf("No se pudo abrir canal de RabbitMQ: %v", err)
	}
	defer canalRabbit.Close()

	_, err = canalRabbit.QueueDeclare(
		nombreCola,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("No se pudo declarar la cola: %v", err)
	}

	clienteValkey := redis.NewClient(&redis.Options{
		Addr: urlValkey,
	})

	if err := clienteValkey.Ping(ctx).Err(); err != nil {
		log.Fatalf("No se pudo conectar a Valkey: %v", err)
	}

	mensajes, err := canalRabbit.Consume(
		nombreCola,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("No se pudo consumir la cola: %v", err)
	}

	log.Println("Consumer conectado")
	log.Printf("Consumiendo mensajes desde RabbitMQ cola '%s'", nombreCola)
	log.Printf("Guardando datos en Valkey en %s", urlValkey)

	for mensaje := range mensajes {
		var reporte ReporteGuerra

		if err := json.Unmarshal(mensaje.Body, &reporte); err != nil {
			log.Printf("Mensaje inválido: %v", err)
			mensaje.Nack(false, false)
			continue
		}

		log.Printf(
			"Consumido: país=%s aviones=%d barcos=%d timestamp=%s",
			reporte.Country,
			reporte.WarplanesInAir,
			reporte.WarshipsInWater,
			reporte.Timestamp,
		)

		clienteValkey.Incr(ctx, "total_reportes")
		clienteValkey.Incr(ctx, "reportes:pais:"+reporte.Country)
		clienteValkey.Set(ctx, "ultimo_reporte", string(mensaje.Body), 0)

		if err := mensaje.Ack(false); err != nil {
			log.Printf("No se pudo confirmar mensaje: %v", err)
		}
	}
}