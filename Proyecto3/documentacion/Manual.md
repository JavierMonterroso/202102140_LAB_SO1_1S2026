
<img src='https://user-images.githubusercontent.com/36779113/128587817-1a6c2fdc-d106-4dd3-b092-104c8299bded.png' background='white'>


## Universidad de San Carlos de Guatemala, Junio 2025
## Facultad de Ingeniería
## Escuela de Ciencias y Sistemas
## LABORATORIO SISTEMAS OPERATIVOS 1 S12026


| TABLA DE DATOS  | 
| ------------- | 
| Carné:    202102140|
| Nombre:    Javier Andrés Monterroso García|

<br> 
<br> 
<br> 
<br> 

<center>

# Manual Técnico del Proyecto 3

</center>






En este documento explico como desplegue y probe el proyecto en Kubernetes. El enfoque principal esta en los manifiestos de Kubernetes, Deployments, Services, Gateway API, KubeVirt, Valkey, Grafana, RabbitMQ y Locust.

La aplicacion sigue este flujo:

```text
Locust
  -> Gateway API
  -> API Rust
  -> Go Deployment 1 REST
  -> Go Deployment 1 gRPC Client
  -> Go Deployment 2 gRPC Server
  -> Go Deployment 2 Rabbit Writer
  -> RabbitMQ
  -> Go Rabbit Consumer
  -> Valkey/Redis en KubeVirt
  -> Grafana en KubeVirt
```

## Estructura General

La estructura principal del proyecto quedo asi:

```text
Proyecto3/
├── api-rust/
│   ├── Dockerfile
│   ├── src/main.rs
│   └── k8s/
│       ├── deployment.yaml
│       ├── hpa.yaml
│       └── service.yaml
├── gateway/
│   └── k8s/
│       ├── gateway.yaml
│       └── httproute.yaml
├── go/
│   ├── proto/report.proto
│   ├── deployment1/
│   │   ├── rest-api/
│   │   ├── grpc-client/
│   │   └── k8s/
│   │       ├── deployment.yaml
│   │       └── service.yaml
│   ├── deployment2/
│   │   ├── grpc-server/
│   │   ├── rabbit-writer/
│   │   └── k8s/
│   │       ├── deployment.yaml
│   │       └── service.yaml
│   └── rabbit-consumer/
│       ├── Dockerfile
│       ├── main.go
│       └── k8s/deployment.yaml
├── rabbitmq/
│   └── k8s/
│       ├── deployment.yaml
│       └── service.yaml
├── kubevirt/
│   ├── emul-kubevirt.yaml
│   ├── k8s/
│   └── valkey/
├── k8s/01-namespace.yaml
└── test/locustfile.py
```

## Namespace

Primero cree el namespace del proyecto:

```text
k8s/01-namespace.yaml
```

Contenido principal:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: py3-202102140
```

Comando:

```sh
kubectl apply -f k8s/01-namespace.yaml
```

Este namespace lo use para separar casi todos los recursos del proyecto:

```text
py3-202102140
```

## Gateway API

En lugar de usar Ingress, expuse el endpoint publico con Kubernetes Gateway API. Esta parte es importante porque es la entrada oficial del sistema: Locust no le pega directamente a un Pod ni a un Service interno, sino a una IP publica creada por GKE.

El Gateway funciona como una puerta de entrada HTTP administrada por Kubernetes/GKE. Por separado, el `HTTPRoute` define que rutas acepta esa puerta y hacia que Service interno debe enviar cada peticion.

Lo importante es que aqui no expuse directamente el Deployment de Rust con un `NodePort`. Use Gateway API porque GKE crea un balanceador HTTP externo administrado, le asigna una IP publica y luego Kubernetes mantiene la relacion entre esa IP, la ruta y el Service interno. De esta forma el trafico externo entra por una sola puerta controlada y mis Pods siguen siendo internos.

La division queda asi:

```text
Gateway      = crea/representa la entrada publica
HTTPRoute    = define que path se acepta y hacia donde se manda
Service      = selecciona los Pods internos
Deployment   = mantiene las replicas de la aplicacion
Pod          = ejecuta el contenedor real
```

Archivos:

```text
gateway/
└── k8s/
    ├── gateway.yaml
    └── httproute.yaml
```

### `gateway/k8s/gateway.yaml`

Aqui cree el Gateway publico:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: api-gateway
  namespace: py3-202102140
spec:
  gatewayClassName: gke-l7-global-external-managed
  listeners:
  - name: http
    protocol: HTTP
    port: 80
```

El campo `gatewayClassName` indica que GKE debe crear un balanceador externo global administrado:

```text
gke-l7-global-external-managed
```

El listener `http` indica que el Gateway escucha trafico HTTP en el puerto `80`. Cuando este recurso queda programado, GKE asigna una IP publica. Esa IP fue la que use como host en Locust.

En otras palabras, este YAML no contiene una IP escrita a mano. Yo solo le digo a Kubernetes que quiero un Gateway HTTP publico; GKE se encarga de aprovisionar el balanceador, reservar/asignar la direccion y conectar ese balanceador con los recursos internos del cluster.

El campo `namespace` tambien es importante:

```yaml
namespace: py3-202102140
```

Esto hace que el Gateway viva dentro del namespace del proyecto. Como el `HTTPRoute` y el `api-rust-service` tambien estan en ese namespace, la referencia entre recursos queda directa y no necesito permisos ni referencias cruzadas entre namespaces.

Para revisar si el Gateway ya estaba listo use:

```sh
kubectl get gateway api-gateway -n py3-202102140
```

El campo importante es:

```text
PROGRAMMED=True
```

Cuando aparece `True`, significa que el balanceador ya fue creado y el Gateway ya puede recibir trafico.

### `gateway/k8s/httproute.yaml`

Aqui conecte la ruta publica con el Service de Rust. El Gateway solo abre la entrada; el `HTTPRoute` decide como se enruta el trafico.

```yaml
matches:
  - path:
      type: PathPrefix
      value: /grpc-202102140
backendRefs:
  - name: api-rust-service
    port: 80
```

La regla usa `PathPrefix`, por lo que cualquier peticion cuyo path inicie con `/grpc-202102140` se manda al backend indicado.

El campo `parentRefs` del archivo completo enlaza esta ruta con el Gateway:

```yaml
parentRefs:
- name: api-gateway
```

Con esto le indico a Kubernetes que esta regla pertenece al Gateway `api-gateway`. Luego, dentro de `rules`, defino el path aceptado y el backend. El backend no es un Pod, sino un Service:

```yaml
backendRefs:
  - name: api-rust-service
    port: 80
```

Eso es correcto porque en Kubernetes normalmente no se enruta trafico directamente a Pods desde recursos externos. El Service es el punto estable: aunque el Pod de Rust muera y Kubernetes cree otro, el Gateway sigue enviando al mismo Service.

Esto significa que cuando Locust envia datos a:

```text
http://<IP-GATEWAY>/grpc-202102140
```

el trafico llega al Service:

```text
api-rust-service
```

Ese Service despues se encarga de enviar la peticion a uno de los Pods del Deployment de Rust. Asi el flujo publico queda separado del flujo interno:

```text
Internet / Locust
  -> Gateway
  -> HTTPRoute
  -> api-rust-service
  -> Pod api-rust
```

Comandos:

```sh
kubectl apply -f gateway/k8s/gateway.yaml
kubectl apply -f gateway/k8s/httproute.yaml
kubectl get gateway api-gateway -n py3-202102140 -w
```

Para obtener la IP publica:

```sh
kubectl get gateway api-gateway -n py3-202102140
```

En mi caso use:

```text
http://35.201.66.155/grpc-202102140
```

Tambien use `curl` para validar manualmente el Gateway:

```sh
curl -X POST http://35.201.66.155/grpc-202102140 \
  -H "Content-Type: application/json" \
  -d '{"country":"usa","warplanes_in_air":40,"warships_in_water":15,"timestamp":"2026-05-01T19:55:00Z"}'
```

Si todo esta bien, la API Rust responde que el reporte fue recibido y enviado a Go.

Un detalle que revise durante las pruebas fue el estado `PROGRAMMED`. Si el Gateway aparece con `PROGRAMMED=True`, significa que GKE ya termino de preparar el balanceador. Si aparece `Unknown` o no tiene `ADDRESS`, normalmente solo hay que esperar un poco o revisar eventos del Gateway.

## API Rust

Archivos:

```text
api-rust/
├── Dockerfile
├── src/main.rs
└── k8s/
    ├── deployment.yaml
    ├── hpa.yaml
    └── service.yaml
```

### Codigo Rust

El archivo principal es:

```text
api-rust/src/main.rs
```

Aqui realice una API con Axum. Su responsabilidad es recibir los reportes enviados por Locust, validar el JSON de entrada y reenviar el contenido hacia el primer Deployment de Go.

El endpoint principal es:

```rust
.route("/grpc-202102140", post(receive_report))
```

La estructura recibida es:

```rust
struct WarReport {
    country: String,
    warplanes_in_air: i32,
    warships_in_water: i32,
    timestamp: String,
}
```

Este struct representa el JSON que llega desde Locust. Por ejemplo:

```json
{
  "country": "usa",
  "warplanes_in_air": 40,
  "warships_in_water": 15,
  "timestamp": "2026-05-01T19:55:00Z"
}
```

Rust es la primera aplicacion del flujo, por eso aqui tambien deje logs claros. Los logs me ayudan a demostrar que la peticion si entro por el Gateway y que Rust si la reenvio al siguiente salto.

El handler `receive_report` recibe el JSON ya parseado por Axum:

```rust
async fn receive_report(Json(payload): Json<WarReport>) -> impl IntoResponse
```

Despues llama a `forward_to_go`, que abre una conexion TCP hacia el Service interno de Go. En vez de usar una libreria HTTP completa, arme manualmente el request HTTP:

```rust
let addr = format!("{host}:{port}");
let mut stream = TcpStream::connect(&addr).await?;
```

Las variables de entorno controlan el destino:

```text
GO_INGEST_HOST=go-ingest-service
GO_INGEST_PORT=80
GO_INGEST_PATH=/report
```

Esto me permite cambiar el destino desde Kubernetes sin recompilar la imagen.

La funcion `forward_to_go` hace tres cosas principales:

- arma el JSON que va a mandar a Go
- abre una conexion a `go-ingest-service:80`
- lee la respuesta HTTP que devuelve Go

Si Go responde con codigo `2xx`, Rust responde `200 OK` hacia Locust. Si Go falla, Rust responde `502 Bad Gateway`, porque el error ocurrio en el siguiente servicio del flujo.

Tambien agregue una carga CPU-bound por request:

```rust
for i in 0..5_000_000 {
    _dummy = _dummy.wrapping_add(i);
}
```

Esto no simula el HPA. Lo que hace es generar consumo real de CPU para que Kubernetes pueda medir carga y el HPA escale el Deployment de Rust.

Es decir, Locust genera peticiones reales, Rust procesa peticiones reales, y el HPA escala con metricas reales de CPU. La carga extra solo sirve para que el consumo de CPU sea visible durante la prueba.

### Dockerfile de Rust

Archivo:

```text
api-rust/Dockerfile
```

Use una etapa `builder` con Rust:

```dockerfile
FROM rust:latest AS builder
RUN cargo build --release
```

Luego copie el binario a una imagen liviana:

```dockerfile
FROM debian:bookworm-slim
COPY --from=builder /app/target/release/api-rust /usr/local/bin/api-rust
EXPOSE 8080
CMD ["api-rust"]
```

Comandos de build y push:

```sh
docker build -t javiermonterroso14/api-rust:202102140 -f api-rust/Dockerfile .
docker push javiermonterroso14/api-rust:202102140
```

### Deployment de Rust

Archivo:

```text
api-rust/k8s/deployment.yaml
```

Aqui cree el Deployment:

```yaml
name: api-rust-deploy
namespace: py3-202102140
replicas: 1
```

La imagen usada:

```yaml
image: javiermonterroso14/api-rust:202102140
```

La API escucha en el puerto `8080`:

```yaml
ports:
  - containerPort: 8080
```

Las variables de entorno indican hacia donde enviar el reporte:

```yaml
GO_INGEST_HOST: go-ingest-service
GO_INGEST_PORT: "80"
GO_INGEST_PATH: /report
```

Esto conecta Rust con el primer Deployment de Go. Dentro de Kubernetes, Rust no usa una IP fija para llegar a Go; usa DNS interno:

```text
go-ingest-service
```

Kubernetes resuelve ese nombre hacia el Service, y el Service balancea hacia los Pods disponibles de `go-ingest-deploy`.

### Service de Rust

Archivo:

```text
api-rust/k8s/service.yaml
```

Aqui cree un Service interno:

```yaml
name: api-rust-service
port: 80
targetPort: 8080
```

El Gateway usa este Service como backend. Este Service traduce el puerto externo interno `80` hacia el puerto real del contenedor `8080`.

```text
Gateway -> api-rust-service:80 -> api-rust Pod:8080
```

### HPA de Rust

Archivo:

```text
api-rust/k8s/hpa.yaml
```

Aqui configure el escalado automatico:

```yaml
minReplicas: 1
maxReplicas: 3
averageUtilization: 30
```

Esto significa que Kubernetes puede mover `api-rust-deploy` de 1 a 3 replicas cuando el uso promedio de CPU supera el 30%.

La metrica que usa el HPA depende de los `resources.requests.cpu` del Deployment. En este caso defini un request de CPU pequeño para que el porcentaje de utilizacion pueda observarse durante la carga:

```yaml
resources:
  requests:
    cpu: "10m"
  limits:
    cpu: "50m"
```

Con esto pude demostrar que el escalado es automatico: yo no escalo manualmente Rust, sino que Kubernetes decide segun la CPU.

Comandos:

```sh
kubectl apply -f api-rust/k8s/deployment.yaml
kubectl apply -f api-rust/k8s/service.yaml
kubectl apply -f api-rust/k8s/hpa.yaml
```

Verificacion:

```sh
kubectl get deployment api-rust-deploy -n py3-202102140
kubectl get hpa api-rust-hpa -n py3-202102140
kubectl get pods -n py3-202102140 | grep api-rust
```

Para ver el escalado en vivo:

```sh
kubectl get hpa -n py3-202102140 -w
kubectl get deployment api-rust-deploy -n py3-202102140 -w
```

## Go Deployment 1: REST API y gRPC Client

Archivos:

```text
go/deployment1/
├── rest-api/
│   ├── Dockerfile
│   └── main.go
├── grpc-client/
│   ├── Dockerfile
│   └── main.go
└── k8s/
    ├── deployment.yaml
    └── service.yaml
```

### Funcion del Deployment 1

Aqui use un solo Deployment con dos contenedores:

- `go-rest-api`
- `go-grpc-client`

El contenedor `go-rest-api` recibe el HTTP desde Rust en `/report`. Su trabajo es recibir el mismo JSON militar, imprimir logs de seguimiento y reenviarlo al segundo contenedor del mismo Pod.

El contenedor `go-grpc-client` recibe ese JSON por HTTP interno, lo transforma al mensaje definido en `report.proto`, y luego llama al servidor gRPC del segundo Deployment.

Aqui separo dos responsabilidades. El REST API habla HTTP porque Rust le envia HTTP. El gRPC Client existe para convertir ese reporte al contrato gRPC del proyecto. Asi no mezclo toda la logica en un solo proceso y puedo mostrar claramente el paso HTTP -> gRPC.

La comunicacion entre los dos contenedores del Deployment 1 es por `localhost`, porque ambos viven en el mismo Pod y comparten la red:

```text
go-rest-api -> localhost:50052/send -> go-grpc-client
```

Luego el `go-grpc-client` convierte el reporte a gRPC y lo envia al Service:

```text
go-processing-service:50051
```

El flujo de este Deployment queda asi:

```text
Rust
  -> go-ingest-service:80
  -> go-rest-api:8081
  -> localhost:50052
  -> go-grpc-client
  -> go-processing-service:50051
```

### YAML del Deployment 1

Archivo:

```text
go/deployment1/k8s/deployment.yaml
```

Nombre:

```yaml
name: go-ingest-deploy
```

Tiene 1 replica:

```yaml
replicas: 1
```

Y dos containers dentro del mismo pod:

```yaml
- name: go-rest-api
  containerPort: 8081
- name: go-grpc-client
  containerPort: 50052
```

El REST API llama al gRPC Client por localhost:

```yaml
GRPC_CLIENT_URL: http://localhost:50052/send
```

Esto funciona porque ambos contenedores viven dentro del mismo pod. No necesito crear un Service para comunicar `go-rest-api` con `go-grpc-client`, ya que el trafico nunca sale del Pod.

Esta decision tambien reduce recursos de Kubernetes: no cree un Service extra para el `go-grpc-client` porque no lo necesita otro Pod directamente. Solo lo usa el contenedor vecino dentro del mismo Pod.

El contenedor `go-grpc-client` si necesita salir del Pod para llegar al Deployment 2. Por eso usa:

```yaml
GRPC_SERVER_ADDR: go-processing-service:50051
```

Ese nombre lo resuelve el DNS interno del cluster y apunta al Service del segundo Deployment.

### Service del Deployment 1

Archivo:

```text
go/deployment1/k8s/service.yaml
```

Expone el puerto HTTP:

```yaml
name: go-ingest-service
port: 80
targetPort: 8081
```

Rust usa este Service para enviar los reportes. El Service selecciona Pods con:

```yaml
selector:
  app: go-ingest
```

Eso permite que si en algun momento este Deployment tuviera mas replicas, el Service podria balancear entre sus Pods.

### Dockerfiles de Deployment 1

El Dockerfile de `rest-api` compila el binario Go y lo copia a Alpine:

```dockerfile
FROM golang:1.24-alpine AS builder
RUN go build -o rest_api_bin main.go
FROM alpine:latest
CMD ["./rest_api_bin"]
```

El Dockerfile de `grpc-client` instala `protoc`, genera codigo desde `report.proto` y compila:

```dockerfile
RUN apk add --no-cache protoc
RUN protoc --go_out=. --go-grpc_out=. proto/report.proto
```

Este paso es necesario porque el codigo gRPC de Go no se escribe manualmente. A partir del `.proto`, `protoc` genera las estructuras, el cliente y las interfaces que usa el servidor.

Comandos:

```sh
docker build -t javiermonterroso14/go-rest-api:202102140 -f go/deployment1/rest-api/Dockerfile .
docker push javiermonterroso14/go-rest-api:202102140

docker build -t javiermonterroso14/go-grpc-client:202102140 -f go/deployment1/grpc-client/Dockerfile .
docker push javiermonterroso14/go-grpc-client:202102140
```

Despliegue:

```sh
kubectl apply -f go/deployment1/k8s/deployment.yaml
kubectl apply -f go/deployment1/k8s/service.yaml
```

## Go Deployment 2: gRPC Server y Rabbit Writer

Archivos:

```text
go/deployment2/
├── grpc-server/
│   ├── Dockerfile
│   └── main.go
├── rabbit-writer/
│   ├── Dockerfile
│   └── main.go
└── k8s/
    ├── deployment.yaml
    └── service.yaml
```

### Funcion del Deployment 2

Aqui realice otro Deployment con dos contenedores:

- `go-grpc-server`
- `go-rabbit-writer`

El `go-grpc-server` recibe la llamada gRPC en el puerto `50051`. Este contenedor implementa el servicio definido en `report.proto`.

Luego llama por HTTP al writer interno:

```text
http://localhost:50053/publish
```

El `go-rabbit-writer` publica el mensaje en RabbitMQ, en la cola:

```text
reportes
```

Igual que en el Deployment 1, aqui use dos contenedores dentro del mismo Pod para separar responsabilidades:

- `go-grpc-server`: recibe gRPC y traduce el reporte a JSON.
- `go-rabbit-writer`: se encarga de conectarse a RabbitMQ y publicar en la cola.

El motivo de esta separacion es que el servidor gRPC queda enfocado en recibir la llamada remota y responder al cliente, mientras que el writer se enfoca en la comunicacion con RabbitMQ. El writer usa AMQP, declara la cola si hace falta y publica el cuerpo del reporte.

La comunicacion entre `go-grpc-server` y `go-rabbit-writer` vuelve a ser por `localhost`, porque ambos contenedores estan en el mismo Pod:

```text
go-grpc-server -> localhost:50053/publish -> go-rabbit-writer
```

Esto hace que el segundo Deployment tenga una entrada externa al Pod por gRPC, pero internamente separe el paso de publicacion hacia RabbitMQ.

El flujo interno queda:

```text
go-grpc-client
  -> go-processing-service:50051
  -> go-grpc-server
  -> localhost:50053/publish
  -> go-rabbit-writer
  -> rabbitmq-service:5672
```

### YAML del Deployment 2

Archivo:

```text
go/deployment2/k8s/deployment.yaml
```

Nombre:

```yaml
name: go-processing-deploy
```

Contenedores:

```yaml
- name: go-grpc-server
  containerPort: 50051
- name: go-rabbit-writer
  containerPort: 50053
```

El gRPC Server apunta al writer local:

```yaml
WRITER_URL: http://localhost:50053/publish
```

El writer apunta a RabbitMQ:

```yaml
RABBITMQ_URL: amqp://guest:guest@rabbitmq-service:5672/
RABBITMQ_QUEUE: reportes
```

Estas variables son importantes porque evitan quemar valores dentro del binario. Si cambio el nombre del Service de RabbitMQ o la cola, puedo modificar el YAML y volver a aplicar el Deployment.

### Service del Deployment 2

Archivo:

```text
go/deployment2/k8s/service.yaml
```

Expone el puerto gRPC:

```yaml
name: go-processing-service
port: 50051
targetPort: 50051
```

El gRPC Client del Deployment 1 usa este Service. Este Service apunta a los Pods con label:

```yaml
app: go-processing
```

Por eso el cliente no necesita conocer la IP del Pod; solo usa el DNS interno:

```text
go-processing-service:50051
```

Si este Deployment tiene mas de una replica, el Service puede balancear las llamadas gRPC entre los Pods disponibles. Por eso este punto es el que conviene usar para las pruebas comparativas con 1 y 2 replicas.



## Protobuf

Archivo:

```text
go/proto/report.proto
```

Aqui defini el contrato gRPC que usan el `go-grpc-client` y el `go-grpc-server`. Este archivo es importante porque ambos lados de la comunicacion deben hablar exactamente el mismo formato.

El archivo `.proto` es como el contrato entre cliente y servidor. El cliente no manda un JSON libre al servidor gRPC; manda un mensaje binario generado a partir de esta definicion. Por eso el `grpc-client` y el `grpc-server` deben usar el mismo `report.proto`.

```proto
service WarReportService {
  rpc SendReport(WarReportRequest) returns (WarReportResponse);
}
```

El servicio tiene un metodo:

```text
SendReport
```

Ese metodo recibe un `WarReportRequest` y devuelve un `WarReportResponse`.

El request contiene:

```proto
message WarReportRequest {
  Countries country = 1;
  int32 warplanes_in_air = 2;
  int32 warships_in_water = 3;
  string timestamp = 4;
}
```

Use `int32` para aviones y barcos porque son cantidades enteras. Use `string` para `timestamp` porque el valor viaja en formato ISO, por ejemplo:

```text
2026-05-01T19:55:00Z
```

Tambien declare un enum para los paises:

```proto
enum Countries {
  countries_unknown = 0;
  usa = 1;
  rus = 2;
  chn = 3;
  esp = 4;
  gmt = 5;
}
```

El enum evita que el servidor gRPC reciba cualquier texto arbitrario como pais. Primero, el cliente convierte el string que venia del JSON a un valor del enum. Si el pais no coincide con los esperados, puede caer en `countries_unknown`.

En Go, el cliente convierte el texto recibido desde el JSON a este enum. Por ejemplo:

```text
"USA" -> Countries_usa
"RUS" -> Countries_rus
```

La respuesta del servicio es sencilla:

```proto
message WarReportResponse {
  string status = 1;
}
```

La use para devolver un estado textual y confirmar que el reporte fue procesado por el servidor gRPC.

El Dockerfile de los componentes gRPC usa `protoc` para generar los archivos `.pb.go` y `_grpc.pb.go`:

```dockerfile
RUN protoc --go_out=. --go-grpc_out=. proto/report.proto
```

Esto genera el codigo Go necesario para crear el cliente y el servidor gRPC.

Nota: en el enunciado aparece `GTM`, pero en el proto quedo escrito como `gmt`. Eso es algo que debo cuidar al documentar, porque `GTM` representa Guatemala y no `GMT` como zona horaria. Para mi dashboard principal use `USA`, porque mi carnet termina en `0`; entonces la serie obligatoria que configure fue `serie_usa_trafico`.

## RabbitMQ

Archivos:

```text
rabbitmq/
└── k8s/
    ├── deployment.yaml
    └── service.yaml
```

### Deployment de RabbitMQ

Archivo:

```text
rabbitmq/k8s/deployment.yaml
```

Aqui cree RabbitMQ con la imagen:

```yaml
image: rabbitmq:3-management
```

Use esta imagen porque incluye RabbitMQ y tambien la interfaz de administracion. Aunque el flujo principal usa AMQP, la imagen con management ayuda a revisar el estado del broker si se necesita.

Expuse dos puertos dentro del pod:

```yaml
5672   # AMQP
15672  # Management
```

El flujo principal usa el puerto `5672`, que es el puerto AMQP. El puerto `15672` corresponde a la interfaz de administracion.

RabbitMQ actua como punto de desacoplamiento. El writer publica mensajes en la cola `reportes`, y el consumer los procesa despues. Esto evita que la API tenga que escribir directamente en Valkey durante la misma peticion.

En el flujo del proyecto RabbitMQ queda entre el procesamiento de Go y Valkey:

```text
go-rabbit-writer -> RabbitMQ -> go-rabbit-consumer -> Valkey
```

La ventaja de esto es que si Valkey tarda o el consumer se reinicia, los mensajes pueden quedarse en la cola temporalmente. Luego, cuando el consumer vuelve a estar disponible, consume los mensajes pendientes. Por eso durante las pruebas era posible que Locust mostrara una cantidad y Valkey terminara con un numero ligeramente mayor si todavia habia mensajes pendientes en RabbitMQ de una prueba anterior.

### Service de RabbitMQ

Archivo:

```text
rabbitmq/k8s/service.yaml
```

Nombre:

```yaml
name: rabbitmq-service
```

Puertos:

```yaml
amqp: 5672
management: 15672
```

Los componentes Go usan:

```text
amqp://guest:guest@rabbitmq-service:5672/
```

El nombre `rabbitmq-service` se resuelve por DNS interno de Kubernetes. Por eso no use IP fija para RabbitMQ.

Este Service no necesita ser publico. Solo lo usan los Pods dentro del cluster:

```text
go-rabbit-writer  -> rabbitmq-service:5672
go-rabbit-consumer -> rabbitmq-service:5672
```

El puerto `15672` queda definido porque la imagen trae management, pero para el recorrido principal del proyecto el puerto importante es `5672`.

Comandos:

```sh
kubectl apply -f rabbitmq/k8s/deployment.yaml
kubectl apply -f rabbitmq/k8s/service.yaml
```

Ver cola:

```sh
kubectl exec -n py3-202102140 deployment/rabbitmq-deploy -- \
  rabbitmqctl list_queues name messages messages_ready messages_unacknowledged
```

Este comando muestra:

```text
messages                 total de mensajes en la cola
messages_ready           mensajes listos para consumir
messages_unacknowledged  mensajes entregados pero aun no confirmados
```

Durante las pruebas observe que si Valkey se limpiaba con `FLUSHALL`, eso no limpiaba RabbitMQ. Por eso tambien revise y limpie la cola antes de una prueba limpia.

Limpiar cola antes de una prueba:

```sh
kubectl exec -n py3-202102140 deployment/rabbitmq-deploy -- \
  rabbitmqctl purge_queue reportes
```

## Rabbit Consumer en Go

Archivos:

```text
go/rabbit-consumer/
├── Dockerfile
├── main.go
└── k8s/
    └── deployment.yaml
```

### Codigo del consumer

El consumer se conecta a RabbitMQ:

```text
amqp://guest:guest@rabbitmq-service:5672/
```

Consume la cola:

```text
reportes
```

Luego guarda metricas en Valkey/Redis usando:

```text
valkey-service:6379
```

Este componente es el ultimo proceso de backend antes de Grafana. Su responsabilidad es tomar cada mensaje de RabbitMQ, convertirlo desde JSON a una estructura Go y actualizar las llaves que usa el dashboard.

El consumer usa confirmacion manual de mensajes. Eso significa que no confirma el mensaje apenas lo recibe; primero intenta guardarlo en Valkey. Si todo sale bien, hace `Ack`. Si falla el guardado, hace `Nack` para que el mensaje pueda volver a la cola.

El flujo interno del consumer es:

```text
leer mensaje de RabbitMQ
  -> json.Unmarshal
  -> actualizar metricas en Valkey
  -> confirmar mensaje con Ack
```

Calcule estas llaves principales:

```text
pais_asignado
total_reportes
total_reportes_usa
max_aviones
min_aviones
max_barcos
min_barcos
top_paises_aviones
top_paises_barcos
frecuencia_aviones
frecuencia_barcos
moda_aviones
moda_barcos
max_frecuencia_aviones_conteo
max_frecuencia_barcos_conteo
serie_usa_trafico
```

Estas llaves corresponden directamente a lo que pide el enunciado:

```text
max_aviones          -> valor maximo general de aviones
min_aviones          -> valor minimo general de aviones
max_barcos           -> valor maximo general de barcos
min_barcos           -> valor minimo general de barcos
top_paises_aviones   -> top de paises por aviones
top_paises_barcos    -> top de paises por barcos
moda_aviones         -> moda de aviones
moda_barcos          -> moda de barcos
pais_asignado        -> pais asignado por carnet
total_reportes_usa   -> total de reportes del pais asignado
total_reportes       -> total general recibido
serie_usa_trafico    -> serie temporal del pais asignado
```

Para los maximos y minimos use valores simples (`GET` y `SET`). Para los top use Sorted Sets, porque Redis permite guardar un score y ordenar por ese valor:

```text
ZADD top_paises_aviones <cantidad_aviones> <pais>
ZADD top_paises_barcos <cantidad_barcos> <pais>
```

Para la moda use hashes de frecuencia:

```text
HINCRBY frecuencia_aviones <valor> 1
HINCRBY frecuencia_barcos <valor> 1
```

Cada vez que una frecuencia supera la frecuencia maxima anterior, actualizo `moda_aviones` o `moda_barcos`.

La serie temporal usa Redis Streams:

```go
client.XAdd(ctx, &redis.XAddArgs{
    Stream: "serie_usa_trafico",
    Values: map[string]interface{}{
        "aviones": planes,
        "barcos": ships,
    },
})
```

Esta parte la deje con Streams porque Grafana puede consultar `XRANGE` y graficarlo como Time series. Solo guardo la serie cuando el pais es `usa`, ya que mi carnet termina en `0`.

### Deployment del consumer

Archivo:

```text
go/rabbit-consumer/k8s/deployment.yaml
```

Nombre:

```yaml
name: go-rabbit-consumer-deploy
```

Variables importantes:

```yaml
RABBITMQ_URL: amqp://guest:guest@rabbitmq-service:5672/
RABBITMQ_QUEUE: reportes
VALKEY_ADDR: valkey-service:6379
```

Este Deployment no expone Service, porque solo consume RabbitMQ y escribe a Valkey.

Esto es importante: el consumer no recibe peticiones HTTP externas, no necesita Gateway y no necesita NodePort. Kubernetes solo debe mantenerlo corriendo como Pod. Si se cae, el Deployment crea otro Pod y ese nuevo consumer vuelve a conectarse a RabbitMQ.

Tambien le defini recursos:

```yaml
requests:
  cpu: "10m"
  memory: "32Mi"
limits:
  cpu: "100m"
  memory: "128Mi"
```

Con esto Kubernetes sabe cuantos recursos reservarle y hasta donde limitarlo.

Comandos:

```sh
docker build -t javiermonterroso14/go-rabbit-consumer:202102140-dashboard -f go/rabbit-consumer/Dockerfile .
docker push javiermonterroso14/go-rabbit-consumer:202102140-dashboard

kubectl set image deployment/go-rabbit-consumer-deploy \
  -n py3-202102140 \
  go-rabbit-consumer=javiermonterroso14/go-rabbit-consumer:202102140-dashboard
```

Ver logs:

```sh
kubectl logs -n py3-202102140 deployment/go-rabbit-consumer-deploy --tail=50
```

## KubeVirt, Valkey y Grafana

La carpeta de KubeVirt quedo asi:

```text
kubevirt/
├── emul-kubevirt.yaml
├── guia-kubevirt-grafana.md
├── k8s/
│   ├── alpine-vm.yaml
│   └── service-grafana.yaml
└── valkey/
    ├── README.md
    ├── pvc.yaml
    ├── service.yaml
    └── vm.yaml
```

### KubeVirt con emulacion

Archivo:

```text
kubevirt/emul-kubevirt.yaml
```

Aqui habilite KubeVirt con:

```yaml
useEmulation: true
```

Use emulacion porque el cluster no tenia `/dev/kvm`.

Comandos:

```sh
kubectl apply -f kubevirt/emul-kubevirt.yaml
kubectl get pods -n kubevirt -w
```

### VM de Valkey

Archivos:

```text
kubevirt/valkey/pvc.yaml
kubevirt/valkey/vm.yaml
kubevirt/valkey/service.yaml
```

El PVC crea un disco de `5Gi`:

```yaml
name: valkey-data-pvc
storage: 5Gi
```

La VM se llama:

```text
valkey-vm
```

Usa Alpine:

```text
quay.io/kubevirt/alpine-container-disk-demo:devel
```

El Service interno se llama:

```text
valkey-service:6379
```

Comandos:

```sh
kubectl apply -f kubevirt/valkey/pvc.yaml
kubectl apply -f kubevirt/valkey/vm.yaml
kubectl apply -f kubevirt/valkey/service.yaml
kubectl get vm,vmi -n py3-202102140
```

Para entrar:

```sh
virtctl console valkey-vm -n py3-202102140
```

Dentro de la VM active red y monte disco:

```sh
ip link set eth0 up
udhcpc -i eth0 || true
mkdir -p /mnt/valkey
mount /dev/vdb /mnt/valkey || true
```

Al inicio Alpine instalaba Redis 4.0.14. Como necesitaba `XADD` y `XRANGE`, compile Redis 7.2.4 dentro de la VM:

```sh
apk update
apk add build-base linux-headers wget tar
cd /tmp
wget https://download.redis.io/releases/redis-7.2.4.tar.gz
tar xzf redis-7.2.4.tar.gz
cd redis-7.2.4
make MALLOC=libc BUILD_TLS=no
cp src/redis-server /usr/local/bin/redis-server
cp src/redis-cli /usr/local/bin/redis-cli
```

Configure Redis:

```sh
cat > /mnt/valkey/valkey.conf <<'EOF'
bind 0.0.0.0
port 6379
dir /mnt/valkey
appendonly yes
protected-mode no
EOF
```

Arranque Redis:

```sh
/usr/local/bin/redis-server /mnt/valkey/valkey.conf
```

Pruebas:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 ping
```

Ver version:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 info server
```

### VM de Grafana

Archivos:

```text
kubevirt/k8s/alpine-vm.yaml
kubevirt/k8s/service-grafana.yaml
```

La VM se llama:

```text
alpine-test
```

El Service de Grafana es:

```text
grafana-nodeport
```

Expone:

```text
3000:32000/TCP
```

Comandos:

```sh
kubectl apply -f kubevirt/k8s/alpine-vm.yaml
kubectl get vm,vmi
virtctl console alpine-test
```

Dentro de la VM instale Grafana:

```sh
ip link set eth0 up
udhcpc -i eth0 || true
mkdosfs -F 32 /dev/vdb
mkdir -p /mnt/data
mount /dev/vdb /mnt/data
wget https://dl.grafana.com/oss/release/grafana-11.5.0.linux-amd64.tar.gz -O /mnt/data/grafana.tar.gz
tar -zxvf /mnt/data/grafana.tar.gz -C /mnt/data/
mkdir -p /mnt/data/grafana-data /mnt/data/grafana-logs /mnt/data/grafana-plugins
```

Arranque:

```sh
/mnt/data/grafana-v11.5.0/bin/grafana-server \
  --homepath /mnt/data/grafana-v11.5.0 \
  cfg:default.paths.data=/mnt/data/grafana-data \
  cfg:default.paths.logs=/mnt/data/grafana-logs \
  cfg:default.paths.plugins=/mnt/data/grafana-plugins
```

Luego aplique el Service:

```sh
kubectl apply -f kubevirt/k8s/service-grafana.yaml
kubectl get svc grafana-nodeport
kubectl get endpoints grafana-nodeport
```

Abri firewall:

```sh
gcloud compute firewall-rules create grafana-nodeport \
  --allow tcp:32000 \
  --source-ranges 0.0.0.0/0 \
  --description "Grafana NodePort"
```

Entre por:

```text
http://<IP-EXTERNA-NODO>:32000
```

## Grafana y Datasource Redis

Como Grafana corre dentro de una VM de KubeVirt, use la IP interna actual de la VMI de Valkey:

```sh
kubectl get vmi valkey-vm -n py3-202102140 -o wide
```

Ejemplo:

```text
redis://10.116.0.20:6379
```

En Grafana configure:

```text
Connections -> Data sources -> Redis
Address: redis://<IP-VALKEY>:6379
```

Para la serie temporal use:

```text
Command: XRANGE
Key: serie_usa_trafico
Start: time range
End: time range
```

Como mi carnet termina en `0`, el pais asignado es:

```text
USA
```

### Paneles del dashboard

El dashboard de Grafana se alimenta de las llaves que escribe el `go-rabbit-consumer`. La idea fue dejar una llave por metrica importante para que cada panel sea facil de configurar.

Para valores maximos y minimos use paneles tipo Stat:

```text
Valor maximo general de aviones:
Command: GET
Key: max_aviones

Valor minimo general de aviones:
Command: GET
Key: min_aviones

Valor maximo general de barcos:
Command: GET
Key: max_barcos

Valor minimo general de barcos:
Command: GET
Key: min_barcos
```

Para las modas tambien use Stat:

```text
Moda de aviones:
Command: GET
Key: moda_aviones

Moda de barcos:
Command: GET
Key: moda_barcos
```

Para el pais asignado y el total de reportes del pais asignado:

```text
Pais asignado:
Command: GET
Key: pais_asignado

Total de reportes USA:
Command: GET
Key: total_reportes_usa
```

Para los top de paises use Sorted Sets:

```text
Top paises por aviones:
Command: ZRANGE
Key: top_paises_aviones
Range Query: Index range
Minimum: 0
Maximum: -1
```

Si necesito confirmar el orden desde terminal, uso `ZREVRANGE` para ver primero los valores mas altos:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 zrevrange top_paises_aviones 0 4 withscores
```

```text
Top paises por barcos:
Command: ZRANGE
Key: top_paises_barcos
Range Query: Index range
Minimum: 0
Maximum: -1
```

Y para barcos:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 zrevrange top_paises_barcos 0 4 withscores
```

Para la serie temporal obligatoria use Time series:

```text
Command: XRANGE
Key: serie_usa_trafico
Start: time range
End: time range
```

Este panel muestra dos series porque cada entrada del stream guarda los campos:

```text
aviones
barcos
```

Por eso Grafana pudo mostrar dos lineas en el mismo panel: una para aviones en aire y otra para barcos en mar.

## Locust

Archivo:

```text
test/locustfile.py
```

Aqui genere trafico hacia:

```text
/grpc-202102140
```

El payload enviado tiene:

```python
country
warplanes_in_air
warships_in_water
timestamp
```

El arreglo de paises:

```python
["USA", "RUS", "CHN", "ESP", "GTM"]
```

Comandos:

```sh
cd ~/Documentos/SO1_PY1/Proyecto3/test
source venv/bin/activate
locust -f locustfile.py --host http://35.201.66.155
```

Luego abri:

```text
http://localhost:8089
```

## Orden de Despliegue

Este fue el orden general que use:

```sh
kubectl apply -f k8s/01-namespace.yaml

kubectl apply -f rabbitmq/k8s/deployment.yaml
kubectl apply -f rabbitmq/k8s/service.yaml

kubectl apply -f kubevirt/emul-kubevirt.yaml
kubectl apply -f kubevirt/valkey/pvc.yaml
kubectl apply -f kubevirt/valkey/vm.yaml
kubectl apply -f kubevirt/valkey/service.yaml

kubectl apply -f go/deployment2/k8s/deployment.yaml
kubectl apply -f go/deployment2/k8s/service.yaml

kubectl apply -f go/deployment1/k8s/deployment.yaml
kubectl apply -f go/deployment1/k8s/service.yaml

kubectl apply -f api-rust/k8s/deployment.yaml
kubectl apply -f api-rust/k8s/service.yaml
kubectl apply -f api-rust/k8s/hpa.yaml

kubectl apply -f go/rabbit-consumer/k8s/deployment.yaml

kubectl apply -f gateway/k8s/gateway.yaml
kubectl apply -f gateway/k8s/httproute.yaml
```

## Comandos de Verificacion

Ver todos los pods:

```sh
kubectl get pods -n py3-202102140 -o wide
```

Ver Services:

```sh
kubectl get svc -n py3-202102140
```

Ver Gateway:

```sh
kubectl get gateway api-gateway -n py3-202102140
```

Ver HPA:

```sh
kubectl get hpa -n py3-202102140
kubectl describe hpa api-rust-hpa -n py3-202102140
```

Ver RabbitMQ:

```sh
kubectl exec -n py3-202102140 deployment/rabbitmq-deploy -- \
  rabbitmqctl list_queues name messages messages_ready messages_unacknowledged
```

Ver Valkey:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 keys '*'
```

Ver metricas principales:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 mget total_reportes total_reportes_usa max_aviones min_aviones max_barcos min_barcos moda_aviones moda_barcos pais_asignado
```

Ver serie temporal:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 xrange serie_usa_trafico - + count 5
```

## Limpieza Antes de Pruebas

Para iniciar una prueba limpia, primero detuve o revise el consumer si era necesario.

Luego limpie Valkey:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 flushall
```

Limpie RabbitMQ:

```sh
kubectl exec -n py3-202102140 deployment/rabbitmq-deploy -- \
  rabbitmqctl purge_queue reportes
```

Verifique que la cola quedara vacia:

```sh
kubectl exec -n py3-202102140 deployment/rabbitmq-deploy -- \
  rabbitmqctl list_queues name messages messages_ready messages_unacknowledged
```

## Prueba del Flujo Completo

Para probar manualmente envie:

```sh
curl -X POST http://35.201.66.155/grpc-202102140 \
  -H "Content-Type: application/json" \
  -d '{"country":"usa","warplanes_in_air":40,"warships_in_water":15,"timestamp":"2026-05-01T19:55:00Z"}'
```

Luego revise logs del consumer:

```sh
kubectl logs -n py3-202102140 deployment/go-rabbit-consumer-deploy --tail=50
```

Y confirme datos en Redis:

```sh
kubectl run redis-test \
  -n py3-202102140 \
  --rm -it \
  --image=redis:7-alpine \
  --restart=Never \
  -- redis-cli -h valkey-service -p 6379 get total_reportes
```

## Pausar y Reanudar el Cluster

Para reducir consumo, escale los nodos del cluster a 0:

```sh
gcloud container clusters resize mumnk8s-cluster \
  --node-pool default-pool \
  --num-nodes 0 \
  --location us-central1-a
```

Antes elimine Gateway/HTTPRoute para quitar el balanceador:

```sh
kubectl delete -f gateway/k8s/httproute.yaml --ignore-not-found=true
kubectl delete -f gateway/k8s/gateway.yaml --ignore-not-found=true
```

Para reanudar:

```sh
gcloud container clusters resize mumnk8s-cluster \
  --node-pool default-pool \
  --num-nodes 3 \
  --location us-central1-a

gcloud container clusters get-credentials mumnk8s-cluster \
  --location us-central1-a
```

Luego revise nodos:

```sh
kubectl get nodes
```

