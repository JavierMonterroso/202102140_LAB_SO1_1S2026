#!/bin/bash

set -e

echo "========================================"
echo "Deteniendo sistema Proyecto 2"
echo "========================================"

echo "[1] Deteniendo daemon..."
if [ -f daemon.pid ]; then
    PID=$(cat daemon.pid)
    kill "$PID" 2>/dev/null || true
    rm -f daemon.pid
    echo "Daemon detenido"
else
    echo "No se encontro daemon.pid"
fi

echo "[2] Eliminando cronjob..."
(crontab -l 2>/dev/null | grep -v "crear_contenedores.sh") | crontab -
echo "Cronjob eliminado"

echo "[3] Descargando modulo..."
sudo rmmod continfo_pr2_so1 2>/dev/null || true
echo "Modulo descargado"

echo "[4] Bajando Grafana y Valkey..."
cd grafana-valkey
docker compose down
cd ..

echo "========================================"
echo "Sistema detenido correctamente"
echo "========================================"