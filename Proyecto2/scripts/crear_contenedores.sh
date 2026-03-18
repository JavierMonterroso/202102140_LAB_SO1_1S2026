#!/bin/bash

set -e

BAJOS_DESEADOS=3
ALTOS_DESEADOS=2

PREFIJO_BAJO="contenedor_bajo"
PREFIJO_ALTO="contenedor_alto"

echo "========================================"
echo "Ejecutando cronjob de creacion de contenedores..."
echo "========================================"

BAJOS_ACTUALES=$(docker ps -a --format '{{.Names}}' | grep "^${PREFIJO_BAJO}_" | wc -l)
ALTOS_ACTUALES=$(docker ps -a --format '{{.Names}}' | grep "^${PREFIJO_ALTO}_" | wc -l)

echo "Contenedores bajos actuales: $BAJOS_ACTUALES"
echo "Contenedores altos actuales: $ALTOS_ACTUALES"

while [ "$BAJOS_ACTUALES" -lt "$BAJOS_DESEADOS" ]; do
    NOMBRE="${PREFIJO_BAJO}_$(date +%s)_$RANDOM"
    echo "Creando contenedor de bajo consumo: $NOMBRE"
    docker run -d --name "$NOMBRE" alpine sleep 240 >/dev/null
    BAJOS_ACTUALES=$((BAJOS_ACTUALES + 1))
done

while [ "$ALTOS_ACTUALES" -lt "$ALTOS_DESEADOS" ]; do
    NOMBRE="${PREFIJO_ALTO}_$(date +%s)_$RANDOM"
    echo "Creando contenedor de alto consumo: $NOMBRE"
    docker run -d --name "$NOMBRE" alpine sh -c "while true; do :; done" >/dev/null
    ALTOS_ACTUALES=$((ALTOS_ACTUALES + 1))
done

echo "========================================"
echo "Cronjob finalizado correctamente"
echo "========================================"