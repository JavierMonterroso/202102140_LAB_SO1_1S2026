# KubeVirt + Grafana en Alpine

Este ejemplo replica la estructura del auxiliar para crear una VM Alpine con KubeVirt y exponer Grafana por NodePort.

## Archivos

- `emul-kubevirt.yaml`: habilita KubeVirt con emulacion para GKE.
- `k8s/alpine-vm.yaml`: crea la VM `alpine-test` con Alpine y un disco extra de 5Gi.
- `k8s/service-grafana.yaml`: expone Grafana en el NodePort `32000`.

## Orden de ejecucion

```sh
kubectl apply -f kubevirt/emul-kubevirt.yaml
kubectl get pods -n kubevirt -w

kubectl apply -f kubevirt/k8s/alpine-vm.yaml
kubectl get vmi alpine-test -w

virtctl console alpine-test
```

Dentro de la VM:

```sh
ip link set eth0 up && udhcpc -i eth0

mkdosfs -F 32 /dev/vdb
mkdir -p /mnt/data
mount /dev/vdb /mnt/data

wget https://dl.grafana.com/oss/release/grafana-11.5.0.linux-amd64.tar.gz -O /mnt/data/grafana.tar.gz
tar -zxvf /mnt/data/grafana.tar.gz -C /mnt/data/

mkdir -p /mnt/data/grafana-data /mnt/data/grafana-logs /mnt/data/grafana-plugins

/mnt/data/grafana-v11.5.0/bin/grafana-server \
  --homepath /mnt/data/grafana-v11.5.0 \
  cfg:default.paths.data=/mnt/data/grafana-data \
  cfg:default.paths.logs=/mnt/data/grafana-logs \
  cfg:default.paths.plugins=/mnt/data/grafana-plugins &

netstat -tlnp | grep 3000
```

Salir de la consola con `Ctrl+]`.

Luego aplicar el servicio:

```sh
kubectl apply -f kubevirt/k8s/service-grafana.yaml
kubectl describe svc grafana-nodeport | grep Endpoints
```

Abrir firewall en GCP:

```sh
gcloud compute firewall-rules create grafana-nodeport \
  --allow tcp:32000 \
  --source-ranges 0.0.0.0/0 \
  --description "Grafana NodePort"
```

Obtener la IP externa de un nodo:

```sh
kubectl get nodes -o wide
```

Abrir Grafana:

```text
http://<EXTERNAL-IP>:32000
```

Credenciales por defecto: `admin / admin`.
