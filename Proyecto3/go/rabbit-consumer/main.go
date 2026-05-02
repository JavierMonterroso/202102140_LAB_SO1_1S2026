package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
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
	ctx := context.Background()
	rabbitURL := envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq-service:5672/")
	queueName := envOrDefault("RABBITMQ_QUEUE", "reportes")
	valkeyAddr := envOrDefault("VALKEY_ADDR", "valkey-service:6379")
	valkeyPassword := envOrDefault("VALKEY_PASSWORD", "")

	conn, err := amqp091.Dial(rabbitURL)
	if err != nil {
		log.Fatalf("no se pudo conectar a RabbitMQ en %s: %v", rabbitURL, err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("no se pudo abrir canal de RabbitMQ: %v", err)
	}
	defer ch.Close()

	valkey := redis.NewClient(&redis.Options{
		Addr:     valkeyAddr,
		Password: valkeyPassword,
		DB:       0,
		Protocol: 2,
	})
	defer valkey.Close()

	if err := valkey.Ping(ctx).Err(); err != nil {
		log.Fatalf("no se pudo conectar a Valkey/Redis en %s: %v", valkeyAddr, err)
	}

	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		log.Fatalf("no se pudo declarar la cola %s: %v", queueName, err)
	}

	messages, err := ch.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("no se pudo consumir la cola %s: %v", queueName, err)
	}

	log.Printf("[GO-RABBIT-CONSUMER][BOOT] Conectado a RabbitMQ=%s cola=%s", rabbitURL, queueName)

	for msg := range messages {
		var report WarReport
		if err := json.Unmarshal(msg.Body, &report); err != nil {
			log.Printf("[GO-RABBIT-CONSUMER][ERROR] Mensaje invalido: %v", err)
			_ = msg.Nack(false, false)
			continue
		}

		log.Printf("[GO-RABBIT-CONSUMER] Consumido pais=%s aviones=%d barcos=%d timestamp=%s",
			report.Country, report.WarplanesInAir, report.WarshipsInWater, report.Timestamp)

		if err := saveReport(ctx, valkey, report); err != nil {
			log.Printf("[GO-RABBIT-CONSUMER][ERROR] No se pudo guardar en Valkey: %v", err)
			_ = msg.Nack(false, true)
			continue
		}

		if err := msg.Ack(false); err != nil {
			log.Printf("[GO-RABBIT-CONSUMER][ERROR] No se pudo confirmar mensaje: %v", err)
		}
	}
}

func saveReport(ctx context.Context, client *redis.Client, report WarReport) error {
	country := strings.ToLower(report.Country)
	planes := int64(report.WarplanesInAir)
	ships := int64(report.WarshipsInWater)

	pipe := client.Pipeline()
	pipe.Set(ctx, "pais_asignado", "USA", 0)
	pipe.Incr(ctx, "total_reportes")
	pipe.ZAdd(ctx, "top_paises_aviones", redis.Z{Score: float64(planes), Member: country})
	pipe.ZAdd(ctx, "top_paises_barcos", redis.Z{Score: float64(ships), Member: country})
	planesFrequency := pipe.HIncrBy(ctx, "frecuencia_aviones", strconv.FormatInt(planes, 10), 1)
	shipsFrequency := pipe.HIncrBy(ctx, "frecuencia_barcos", strconv.FormatInt(ships, 10), 1)
	if country == "usa" {
		pipe.Incr(ctx, "total_reportes_usa")
	}

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return redisPipelineError(err, cmds)
	}
	if err := updateLegacyMinMax(ctx, client, planes, ships); err != nil {
		return err
	}
	if err := updateLegacyMode(ctx, client, "moda_aviones", "max_frecuencia_aviones_conteo", planes, planesFrequency.Val()); err != nil {
		return err
	}
	if err := updateLegacyMode(ctx, client, "moda_barcos", "max_frecuencia_barcos_conteo", ships, shipsFrequency.Val()); err != nil {
		return err
	}
	return saveAssignedCountrySeries(ctx, client, country, planes, ships)
}

func redisPipelineError(execErr error, cmds []redis.Cmder) error {
	for _, cmd := range cmds {
		if err := cmd.Err(); err != nil && err != redis.Nil {
			return fmt.Errorf("%w; comando fallido: %s: %v", execErr, cmd.FullName(), err)
		}
	}
	return execErr
}

func updateLegacyMinMax(ctx context.Context, client *redis.Client, planes, ships int64) error {
	if err := updateMetric(ctx, client, "max_aviones", planes, true); err != nil {
		return err
	}
	if err := updateMetric(ctx, client, "min_aviones", planes, false); err != nil {
		return err
	}
	if err := updateMetric(ctx, client, "max_barcos", ships, true); err != nil {
		return err
	}
	return updateMetric(ctx, client, "min_barcos", ships, false)
}

func updateLegacyMode(ctx context.Context, client *redis.Client, modeKey, maxFrequencyKey string, value, newFrequency int64) error {
	maxFrequency, err := client.Get(ctx, maxFrequencyKey).Int64()
	if err != nil && err != redis.Nil {
		return err
	}
	if err == redis.Nil || newFrequency > maxFrequency {
		if err := client.Set(ctx, modeKey, value, 0).Err(); err != nil {
			return err
		}
		return client.Set(ctx, maxFrequencyKey, newFrequency, 0).Err()
	}
	return nil
}

func saveAssignedCountrySeries(ctx context.Context, client *redis.Client, country string, planes, ships int64) error {
	if country != "usa" {
		return nil
	}

	return client.XAdd(ctx, &redis.XAddArgs{
		Stream: "serie_usa_trafico",
		MaxLen: 100,
		Approx: true,
		Values: map[string]interface{}{
			"aviones": planes,
			"barcos":  ships,
		},
	}).Err()
}

func updateMetric(ctx context.Context, client *redis.Client, key string, value int64, keepMax bool) error {
	current, err := client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return client.Set(ctx, key, value, 0).Err()
	}
	if err != nil {
		return err
	}
	if keepMax && value > current {
		return client.Set(ctx, key, value, 0).Err()
	}
	if !keepMax && value < current {
		return client.Set(ctx, key, value, 0).Err()
	}
	return nil
}
