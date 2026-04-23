
# Nombre del objeto del módulo (debe coincidir con el archivo .c)
obj-m += continfo_pr2_so1_202102140.o

# Directorio de cabeceras del kernel en ejecución
DIR_KERNEL := /lib/modules/$(shell uname -r)/build

# Directorio actual
DIR_ACTUAL := $(shell pwd)

# ── Regla principal
all:
	$(MAKE) -C $(DIR_KERNEL) M=$(DIR_ACTUAL) modules

# ── Limpieza
clean:
	$(MAKE) -C $(DIR_KERNEL) M=$(DIR_ACTUAL) clean

# ── Carga del módulo
cargar:
	sudo insmod continfo_pr2_so1_202102140.ko
	@echo "Módulo cargado. Verifica con: dmesg | tail -5"

# ── Descarga del módulo 
descargar:
	sudo rmmod continfo_pr2_so1_202102140
	@echo "Módulo descargado."

# ── Prueba rápida 
probar:
	@cat /proc/continfo_pr2_so1_202102140 