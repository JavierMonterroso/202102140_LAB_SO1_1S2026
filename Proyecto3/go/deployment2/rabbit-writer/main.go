package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type WarReport struct {
	Country         string `json:"country"`
	WarplanesInAir  int32  `json:"warplanes_in_air"`
	WarshipsInWater int32  `json:"warships_in_water"`
	Timestamp       string `json:"timestamp"`
}

type rabbitPublisher struct {
	channel *amqp091.Channel
	queue   string
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func connectRabbit(url, queue string) (*rabbitPublisher, func(), error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil, err
	}

	cleanup := func() {
		_ = ch.Close()
		_ = conn.Close()
	}

	return &rabbitPublisher{channel: ch, queue: queue}, cleanup, nil
}

func main() {
	rabbitURL := envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq-service:5672/")
	queueName := envOrDefault("RABBITMQ_QUEUE", "reportes")

	publisher, cleanup, err := connectRabbit(rabbitURL, queueName)
	if err != nil {
		log.Fatalf("no se pudo conectar a RabbitMQ en %s: %v", rabbitURL, err)
	}
	defer cleanup()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	http.HandleFunc("/publish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "metodo no permitido", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("no se pudo leer body: %v", err), http.StatusBadRequest)
			return
		}

		var report WarReport
		if err := json.Unmarshal(body, &report); err != nil {
			http.Error(w, fmt.Sprintf("json invalido: %v", err), http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		err = publisher.channel.PublishWithContext(ctx, "", publisher.queue, false, false, amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("no se pudo publicar en RabbitMQ: %v", err), http.StatusBadGateway)
			return
		}

		log.Printf("[GO-RABBIT-WRITER] Publicado en cola=%s pais=%s aviones=%d barcos=%d timestamp=%s",
			publisher.queue, report.Country, report.WarplanesInAir, report.WarshipsInWater, report.Timestamp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"published"}`))
	})

	log.Println("[GO-RABBIT-WRITER][BOOT] Writer HTTP escuchando en :50053")
	log.Printf("[GO-RABBIT-WRITER][BOOT] RabbitMQ=%s cola=%s", rabbitURL, queueName)
	if err := http.ListenAndServe(":50053", nil); err != nil {
		log.Fatalf("fallo rabbit-writer: %v", err)
	}
}
