package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

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
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return err
	}

	pipe := client.Pipeline()
	pipe.RPush(ctx, "war_reports:raw", string(reportJSON))
	pipe.Incr(ctx, "war_reports:total")
	pipe.Incr(ctx, "war_reports:country:"+report.Country+":total")
	pipe.ZIncrBy(ctx, "war_reports:planes_by_country", float64(report.WarplanesInAir), report.Country)
	pipe.ZIncrBy(ctx, "war_reports:ships_by_country", float64(report.WarshipsInWater), report.Country)
	pipe.ZIncrBy(ctx, "war_reports:planes_mode", 1, strconv.Itoa(int(report.WarplanesInAir)))
	pipe.ZIncrBy(ctx, "war_reports:ships_mode", 1, strconv.Itoa(int(report.WarshipsInWater)))
	pipe.LPush(ctx, "war_reports:country:"+report.Country+":series", string(reportJSON))
	pipe.LTrim(ctx, "war_reports:country:"+report.Country+":series", 0, 499)

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return redisPipelineError(err, cmds)
	}
	return updateMinMax(ctx, client, report)
}

func redisPipelineError(execErr error, cmds []redis.Cmder) error {
	for _, cmd := range cmds {
		if err := cmd.Err(); err != nil && err != redis.Nil {
			return fmt.Errorf("%w; comando fallido: %s: %v", execErr, cmd.FullName(), err)
		}
	}
	return execErr
}

func updateMinMax(ctx context.Context, client *redis.Client, report WarReport) error {
	if err := updateMetric(ctx, client, "war_reports:max_planes", int64(report.WarplanesInAir), true); err != nil {
		return err
	}
	if err := updateMetric(ctx, client, "war_reports:min_planes", int64(report.WarplanesInAir), false); err != nil {
		return err
	}
	if err := updateMetric(ctx, client, "war_reports:max_ships", int64(report.WarshipsInWater), true); err != nil {
		return err
	}
	return updateMetric(ctx, client, "war_reports:min_ships", int64(report.WarshipsInWater), false)
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
