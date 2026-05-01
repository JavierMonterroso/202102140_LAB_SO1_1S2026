# Valkey en KubeVirt

Estos manifiestos crean una VM independiente para Valkey dentro del cluster de Kubernetes.

## Recursos

- `pvc.yaml`: disco persistente para datos de Valkey.
- `vm.yaml`: VM `valkey-vm` administrada por KubeVirt.
- `service.yaml`: servicio interno `valkey-service:6379` para que el consumer escriba datos.

## Instalacion dentro de la VM

Cuando la VM este en estado `Running`, se debe entrar con `virtctl console valkey-vm` e instalar Redis compatible con el protocolo Valkey/Redis dentro de Alpine.

Comandos base dentro de la VM:

```sh
ip link set eth0 up && udhcpc -i eth0

cat > /etc/apk/repositories <<'EOF'
http://dl-cdn.alpinelinux.org/alpine/v3.7/main
http://dl-cdn.alpinelinux.org/alpine/v3.7/community
EOF

apk update
apk add e2fsprogs redis

# Formatear solo la primera vez. No repetir si ya hay datos.
mkdir -p /mnt/valkey
mkfs.ext4 /dev/vdb
mount /dev/vdb /mnt/valkey

cat > /mnt/valkey/valkey.conf <<'EOF'
bind 0.0.0.0
port 6379
dir /mnt/valkey
appendonly yes
protected-mode no
EOF

redis-server /mnt/valkey/valkey.conf
```

El consumer usa el host `valkey-service:6379`.
