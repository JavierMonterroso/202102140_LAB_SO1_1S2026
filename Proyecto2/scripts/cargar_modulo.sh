#!/bin/bash

set -e

echo "========================================"
echo "Cargando modulo de kernel..."
echo "========================================"

cd "$(dirname "$0")/../kernel"

echo "[1] Compilando modulo..."
make

echo "[2] Descargando modulo anterior si existe..."
sudo rmmod continfo_pr2_so1 2>/dev/null || true

echo "[3] Cargando modulo nuevo..."
sudo insmod continfo_pr2_so1.ko

echo "[4] Verificando archivo /proc..."
ls /proc/continfo_pr2_so1_202102140

echo "[5] Lectura de prueba..."
cat /proc/continfo_pr2_so1_202102140 | head -n 20

echo "========================================"
echo "Modulo cargado correctamente"
echo "========================================"