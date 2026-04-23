#!/bin/bash
echo "--- DETENIENDO SERVICIO (MANTENIENDO DATOS) ---"

# 1. Matar el Daemon
if [ -f daemon.pid ]; then
    kill "$(cat daemon.pid)" 2>/dev/null
    rm -f daemon.pid
    echo "   - Daemon detenido."
fi

# 2. Quitar solo el cronjob del proyecto
if [ -f sopes-daemon ]; then
    EJECUTABLE="$(realpath ./sopes-daemon)"
    (crontab -l 2>/dev/null | grep -F -v "$EJECUTABLE spawn") | crontab -
    echo "   - Cronjob del proyecto eliminado."
else
    echo "   - No se encontró sopes-daemon; no se pudo limpiar el cronjob por ruta."
fi

# 3. Solo DETENER la infraestructura (SIN BORRAR)
cd grafana-valkey || exit 1
docker compose stop
cd .. || exit 1
echo "   - Grafana y Valkey se quedaron en pausa (datos guardados)."

# 4. Limpiar los contenedores de carga (Alpine / Go-client)
echo "🧹 Limpiando carga de trabajo (Alpine/Go-client)..."
docker rm -f $(docker ps -a -q --filter "ancestor=alpine") 2>/dev/null
docker rm -f $(docker ps -a -q --filter "ancestor=roldyoran/go-client") 2>/dev/null

echo "Todo en orden. La infraestructura está lista para la siguiente ejecución."