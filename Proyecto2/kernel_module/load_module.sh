#!/bin/bash
echo "--- CARGA DEL MÓDULO DE KERNEL ---"

cd "$(dirname "$0")" || exit 1

echo "Compilando módulo..."
make

if lsmod | grep -q "^continfo_pr2_so1_202102140"; then
    echo "El módulo ya está cargado."
else
    echo "Cargando módulo..."
    sudo insmod continfo_pr2_so1_202102140.ko
    echo "Módulo cargado correctamente."
fi