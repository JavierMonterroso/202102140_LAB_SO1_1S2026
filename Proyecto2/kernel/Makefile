obj-m += continfo_pr2_so1.o

KDIR := /lib/modules/$(shell uname -r)/build
PWD  := $(shell pwd)

all:
	@echo "==> Compilando modulo..."
	$(MAKE) -C $(KDIR) M=$(PWD) modules

clean:
	@echo "==> Limpiando archivos generados..."
	$(MAKE) -C $(KDIR) M=$(PWD) clean