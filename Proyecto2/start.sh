#!/bin/bash

set -e

echo "========================================"
echo "Iniciando sistema Proyecto 2"
echo "========================================"

echo "[1] Compilando daemon..."
if ! (cd daemon && go build -o ../daemon-bin .); then
    echo "[ERROR] No se pudo compilar el daemon"
    exit 1
fi

echo "[2] Levantando Grafana y Valkey..."
cd grafana-valkey
docker compose up -d
cd ..

echo "[3] Iniciando daemon..."
./daemon-bin &

DAEMON_PID=$!
echo $DAEMON_PID > daemon.pid

echo "========================================"
echo "Sistema iniciado correctamente"
echo "PID del daemon: $DAEMON_PID"
echo "Grafana disponible en: http://localhost:3001"
echo "========================================"