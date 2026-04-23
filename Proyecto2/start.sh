#!/bin/bash
echo "--- INICIANDO SISTEMA (MODO PERSISTENTE) ---"

# 1. Compilar el Daemon (Siempre para tener la última lógica)
echo "🔨 Compilando Daemon de Go..."
cd go_daemon || exit 1
go build -o ../sopes-daemon daemon.go
cd .. || exit 1

# 2. Reiniciar el proceso del daemon si ya existe
if [ -f daemon.pid ]; then
    echo "Reiniciando proceso del Daemon..."
    kill "$(cat daemon.pid)" 2>/dev/null
    rm -f daemon.pid
fi

# 3. Iniciar daemon
./sopes-daemon &
echo $! > daemon.pid

echo "Daemon iniciado correctamente."