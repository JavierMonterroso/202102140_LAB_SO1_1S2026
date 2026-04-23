
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

# Manual Técnico del Proyecto de Monitoreo de Contenedores

</center>


## 1. Introducción

En este proyecto desarrollé un sistema de monitoreo de contenedores que integra varias capas de trabajo. Primero realicé un módulo de kernel en C para exponer información del sistema y de procesos de contenedores mediante `/proc`. Después desarrollé un daemon en Go que lee esa información, la clasifica, toma decisiones sobre los contenedores y almacena métricas en Valkey. Finalmente configuré Grafana para visualizar la información en un dashboard.

La idea general del proyecto es mantener una carga controlada de contenedores, monitorear su comportamiento y mostrar métricas históricas y actuales de memoria y CPU.

La arquitectura general del proyecto quedó así:

```text
Módulo de kernel en C
        ↓
Archivo /proc/continfo_pr2_so1_202102140
        ↓
Daemon en Go
        ↓
Valkey
        ↓
Grafana
```

---

<br> 
<br> 
<br> 

## 2. Estructura general del proyecto

La estructura principal del proyecto quedó organizada en varios componentes:

- `kernel_module/` contiene el módulo de kernel en C y su `Makefile`.
- `go_daemon/` contiene el daemon principal en Go.
- `grafana-valkey/` contiene `docker-compose.yml`, `Dockerfile` y los volúmenes de persistencia.
- `start.sh` inicia el sistema.
- `stop.sh` detiene el sistema sin borrar la infraestructura persistente.
- `load_module.sh` compila y carga el módulo del kernel.

Cada parte tiene una responsabilidad distinta. El módulo obtiene información del sistema. El daemon procesa, decide y administra contenedores. Valkey guarda la información. Grafana la presenta.

---

<br> 
<br> 
<br> 

## 3. Módulo de kernel en C

## 3.1 Propósito del módulo

Codifiqué un módulo de kernel cuyo objetivo es crear un archivo dentro de `/proc` para exponer información del sistema y de procesos asociados a contenedores. El archivo creado es:

```text
/proc/continfo_pr2_so1_202102140
```

Este archivo es la base de todo el proyecto, porque desde ahí el daemon obtiene la información que luego procesa.

---

## 3.2 Inclusión de cabeceras

Realicé las siguientes inclusiones:

```c
#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/init.h>
#include <linux/proc_fs.h>
#include <linux/seq_file.h>
#include <linux/sched.h>
#include <linux/sched/signal.h>
#include <linux/mm.h>
#include <linux/sched/mm.h>
#include <linux/cgroup.h>
#include <linux/jiffies.h>
```

Como podemos ver, utilicé cabeceras del kernel para varios propósitos:

- `module.h`, `kernel.h`, `init.h` para la estructura general del módulo.
- `proc_fs.h` y `seq_file.h` para crear y administrar el archivo en `/proc`.
- `sched.h` y `sched/signal.h` para recorrer procesos.
- `mm.h` y `sched/mm.h` para acceder a información de memoria.
- `cgroup.h` para identificar contenedores.
- `jiffies.h` para el cálculo relacionado con CPU.

---

## 3.3 Definición del módulo

Definí los metadatos básicos del módulo así:

```c
MODULE_LICENSE("GPL");
MODULE_AUTHOR("Estudiante SO1 - 202102140");
#define NOMBRE_PROC "continfo_pr2_so1_202102140"
```

Aquí definí:

- la licencia del módulo,
- el autor,
- y el nombre del archivo que se creará en `/proc`.

También declaré el puntero para la entrada en `/proc`:

```c
static struct proc_dir_entry *entrada_proc;
```

---

## 3.4 Obtención de memoria total y memoria libre

Para obtener la memoria total y libre del sistema, realicé estas funciones:

```c
static unsigned long obtener_ram_total_mb(void) {
    struct sysinfo i; si_meminfo(&i);
    return (i.totalram * i.mem_unit) >> 20;
}

static unsigned long obtener_ram_libre_mb(void) {
    struct sysinfo i; si_meminfo(&i);
    return (i.freeram * i.mem_unit) >> 20;
}
```

En ambas funciones utilicé `si_meminfo`, que devuelve información global de memoria del sistema. Luego multipliqué por `mem_unit` para tener el tamaño real en bytes y después apliqué un desplazamiento de bits `>> 20`, que equivale a dividir entre `2^20`, es decir, convertir a MB.

Entonces:

- `obtener_ram_total_mb()` devuelve la RAM total del sistema en MB.
- `obtener_ram_libre_mb()` devuelve la RAM libre del sistema en MB.

Estas dos funciones son importantes porque después las uso para calcular la RAM usada:

```c
ram_total_mb - ram_libre_mb
```

---

## 3.5 Obtención del ID del contenedor

Para identificar el contenedor al que pertenece un proceso, realicé esta función:

```c
static void obtener_id_contenedor(struct task_struct *t, char *buf, int tam) {
    struct cgroup *cgrp;
    rcu_read_lock();
    cgrp = task_cgroup(t, 0);

    if (cgrp && cgrp->kn) {
        strncpy(buf, cgrp->kn->name, tam - 1);
        buf[tam - 1] = '\0';
    } else {
        strcpy(buf, "N/A");
    }

    rcu_read_unlock();
}
```

Aquí utilicé `task_cgroup` para obtener el cgroup del proceso. Si el proceso tiene cgroup válido y existe `kn`, copio el nombre al buffer de salida. Si no existe, devuelvo `"N/A"`.

Este dato es importante porque me permite identificar a qué contenedor pertenece cada proceso y luego usar ese valor en el daemon para administración y ranking.

---

## 3.6 Obtención de la línea de comando

Para obtener el comando asociado a un proceso, realicé la siguiente función:

```c
static void obtener_cmdline(struct task_struct *t, char *buf, int tam) {
    struct mm_struct *mm = get_task_mm(t);

    if (mm) {
        int len = mm->arg_end - mm->arg_start;
        if (len > tam - 1) len = tam - 1;

        if (len > 0) {
            access_process_vm(t, mm->arg_start, buf, len, 0);
            buf[len] = '\0';
        } else {
            strcpy(buf, "N/A");
        }

        mmput(mm);
    } else {
        strcpy(buf, "N/A");
    }
}
```

Aquí primero obtengo la estructura de memoria del proceso con `get_task_mm`. Luego calculo el tamaño de la línea de argumentos usando `arg_end - arg_start`. Después leo la memoria del proceso con `access_process_vm`.

Esto me permite obtener el comando real con el que fue creado el proceso. Este dato es clave porque en el daemon lo uso para clasificar si un contenedor es de bajo consumo o alto consumo.

Por ejemplo:

- si el comando contiene `sleep 240`, lo clasifico como bajo consumo;
- si el comando contiene `go-client` o `while true`, lo clasifico como alto consumo.

---

## 3.7 Función principal de lectura del archivo `/proc`

La parte central del módulo es esta función:

```c
static int mostrar_informacion(struct seq_file *archivo, void *v)
```

Aquí es donde genero todo el contenido que se escribe en el archivo de `/proc`.

Primero defino variables:

```c
struct task_struct *tarea;
char id_cont[65], cmdline[64];
unsigned long ram_total_mb = obtener_ram_total_mb();
unsigned long ram_libre_mb = obtener_ram_libre_mb();
unsigned long total_ram_kb = ram_total_mb << 10;
```

Aquí calculo:

- RAM total en MB,
- RAM libre en MB,
- RAM total en KB, para poder usarla luego en el cálculo de porcentaje de memoria.

---

## 3.8 Escritura de la línea del sistema

La primera línea que escribo en `/proc` es la línea general del sistema:

```c
seq_printf(archivo, "SISTEMA|%lu|%lu|%lu\n",
           ram_total_mb,
           ram_libre_mb,
           ram_total_mb - ram_libre_mb);
```

Esta línea tiene el formato:

```text
SISTEMA|Total|Libre|Uso
```

Por ejemplo, si el sistema tiene 16000 MB de RAM total y 2000 MB libres, la salida se vería parecida a esto:

```text
SISTEMA|16000|2000|14000
```

Esta línea es importante porque el daemon la usa para actualizar:

- RAM total,
- RAM libre,
- RAM usada,
- historial de RAM.

---

## 3.9 Recorrido de procesos

Después recorro todos los procesos con:

```c
for_each_process(tarea) {
```

Este macro del kernel me permite iterar sobre todos los procesos que existen en el sistema.

Sin embargo, no me interesa cualquier proceso. Por eso agregué filtros.

---

## 3.10 Filtro de procesos no válidos

Primero descarté procesos que no tengan estructura de memoria o padre:

```c
if (!tarea->mm || !tarea->parent) continue;
```

Esto evita problemas con procesos del kernel o procesos que no tienen una estructura de memoria de usuario válida.

---

## 3.11 Filtro de infraestructura

Luego agregué un filtro para no incluir procesos de la infraestructura que soporta el dashboard:

```c
if (strncmp(tarea->comm, "grafana", 7) == 0 ||
    strncmp(tarea->comm, "valkey-server", 13) == 0 ||
    strncmp(tarea->comm, "redis-server", 12) == 0) {
    continue;
}
```

Este filtro lo hice porque Grafana y Valkey forman parte de la infraestructura del proyecto y no deben mezclarse con los contenedores de carga que el daemon administra.

Es decir, quise proteger la arquitectura para que el daemon no eliminara ni clasificara por error procesos de monitoreo o almacenamiento.

---

## 3.12 Filtro de contenedores

Después agregué este filtro:

```c
if (strncmp(tarea->parent->comm, "containerd-shim", 15) != 0) continue;
```

Con esto me aseguro de procesar únicamente procesos que cuelgan de `containerd-shim`, que es el proceso normalmente asociado a la ejecución de contenedores.

Este filtro es heurístico, pero en mi entorno de laboratorio fue válido y suficiente. Además, como también filtré Grafana y Valkey, la lista final quedó enfocada a los procesos realmente relevantes del proyecto.

---

## 3.13 Obtención de métricas por proceso

Una vez que el proceso pasa los filtros, obtengo:

```c
obtener_id_contenedor(tarea, id_cont, sizeof(id_cont));
obtener_cmdline(tarea, cmdline, sizeof(cmdline));
```

y luego obtengo la memoria del proceso:

```c
mm = get_task_mm(tarea);
if (mm) {
    unsigned long vsz_kb = (mm->total_vm << PAGE_SHIFT) >> 10;
    unsigned long rss_kb = (get_mm_rss(mm) << PAGE_SHIFT) >> 10;
```

Aquí calculé:

- `vsz_kb`: memoria virtual total del proceso en KB.
- `rss_kb`: memoria residente real del proceso en KB.

Para `vsz_kb` uso `mm->total_vm`, que representa páginas virtuales, y lo convierto a KB.  
Para `rss_kb` uso `get_mm_rss(mm)`, que devuelve las páginas residentes reales, y lo convierto a KB.

---

## 3.14 Cálculo de porcentaje de memoria

El porcentaje de memoria lo calculé así:

```c
unsigned long p_mem = (total_ram_kb > 0) ? (rss_kb * 10000) / total_ram_kb : 0;
```

Aquí multipliqué por `10000` para conservar dos decimales sin usar flotantes dentro del kernel.

Luego, al imprimirlo, lo hago así:

```c
p_mem / 100, p_mem % 100
```

De esta forma, si por ejemplo el valor interno fuera `734`, lo imprimo como:

```text
7.34
```

En otras palabras, `p_mem` representa `%MEM`.

---

## 3.15 Cálculo de CPU

El valor de CPU lo calculé así:

```c
u64 total_time = tarea->utime + tarea->stime;
u64 process_age_jiffies = nsecs_to_jiffies(ktime_get_ns() - tarea->start_time);
unsigned long p_cpu = (process_age_jiffies > 0) ? (total_time * 10000ULL) / process_age_jiffies : 0;
```

Aquí tomé:

- `utime + stime`, que representa el tiempo acumulado de CPU del proceso,
- la edad del proceso en jiffies,
- y calculé una relación entre ambos.

Este valor no es idéntico al porcentaje que muestra `top`, pero sí me sirve como métrica comparativa entre procesos. Por esa razón después en el dashboard decidí manejar esta métrica como un valor numérico de ranking.

---

## 3.16 Escritura de la línea de proceso

Finalmente escribo cada proceso con este formato:

```c
seq_printf(archivo, "PROCESO|%d|%s|%s|%s|%lu|%lu|%lu.%02lu|%lu.%02lu\n",
           tarea->pid,
           tarea->comm,
           id_cont,
           cmdline,
           vsz_kb,
           rss_kb,
           p_mem / 100, p_mem % 100,
           p_cpu / 100, p_cpu % 100);
```

El formato queda así:

```text
PROCESO|PID|Nombre|ID|Cmd|VSZ|RSS|%MEM|%CPU
```

Este formato fue elegido porque luego en Go puedo separar fácilmente cada campo con `strings.Split(..., "|")`.

---

## 3.17 Creación del archivo `/proc`

La apertura del archivo se hace así:

```c
static int abrir_archivo_proc(struct inode *n, struct file *f) {
    return single_open(f, mostrar_informacion, NULL);
}
```

y las operaciones del archivo se definen así:

```c
static const struct proc_ops ops = {
    .proc_open = abrir_archivo_proc,
    .proc_read = seq_read,
    .proc_lseek = seq_lseek,
    .proc_release = single_release,
};
```

Luego creo la entrada en `/proc` en el `init` del módulo:

```c
static int __init inicio(void) {
    entrada_proc = proc_create(NOMBRE_PROC, 0444, NULL, &ops);
    return entrada_proc ? 0 : -ENOMEM;
}
```

Aquí le indico al kernel que cree el archivo `continfo_pr2_so1_202102140` con permisos de solo lectura.

Finalmente, en la salida del módulo elimino la entrada:

```c
static void __exit fin(void) {
    proc_remove(entrada_proc);
}
```

---

<br> 
<br> 
<br> 

## 4. Daemon en Go

## 4.1 Propósito del daemon

El daemon en Go es el componente que se encarga de:

- leer el archivo `/proc`,
- interpretar la información,
- clasificar procesos,
- crear y eliminar contenedores,
- mantener la política de 3 contenedores de bajo consumo y 2 de alto consumo,
- guardar métricas en Valkey,
- y servir de puente entre el kernel y Grafana.

---

## 4.2 Estructura de datos

Definí esta estructura:

```go
type ProcesoInfo struct {
	PID, Nombre, ID, Cmd, VSZ, RSS, PMem, PCPU string
	VSZValue, RSSValue, PMemVal, PCPUVal       float64
}
```

Aquí guardo tanto los valores originales en texto como sus conversiones a `float64`.

Esto me sirve porque:

- los strings se usan para imprimir o mostrar,
- los float64 se usan para ordenar, comparar y tomar decisiones.

---

## 4.3 Variables globales

Definí estas variables:

```go
var (
	ctx       = context.Background()
	rdb       *redis.Client
	pathProc  = "/proc/continfo_pr2_so1_202102140"
	rng       = rand.New(rand.NewSource(time.Now().UnixNano()))
	intervalo = 20 * time.Second
)
```

Aquí configuré:

- el contexto para Valkey,
- el cliente de redis,
- la ruta del archivo `/proc`,
- el generador aleatorio,
- y el intervalo de monitoreo.

El intervalo quedó en 20 segundos porque el daemon debe revisar constantemente el estado del sistema.

---

## 4.4 Flujo principal del daemon

El `main` quedó así, en estructura general:

```go
func main() {
	if len(os.Args) > 1 && os.Args[1] == "spawn" {
		conectarValkey()
		crearLoteContenedores()
		return
	}

	exec.Command("docker", "compose", "-f", "grafana-valkey/docker-compose.yml", "up", "-d").Run()
	conectarValkey()
	exec.Command("bash", "kernel_module/load_module.sh").Run()

	inicializarEstadoBase()
	configurarCronjob()
	configurarLimpiezaAlSalir()

	ticker := time.NewTicker(intervalo)
	for range ticker.C {
		revisarLogsDeCronjob()
		ejecutarCicloMonitoreo()
	}
}
```

Este flujo principal hace varias cosas importantes.

---

## 4.5 Modo `spawn`

El daemon puede ejecutarse en modo normal o en modo especial:

```go
if len(os.Args) > 1 && os.Args[1] == "spawn" {
	conectarValkey()
	crearLoteContenedores()
	return
}
```

Cuando lo invoco con `spawn`, no entra al ciclo principal. Solo se conecta a Valkey y crea una tanda aleatoria de contenedores. Este modo es el que usa el cronjob.

---

## 4.6 Levantamiento de infraestructura

En el flujo normal, primero levanto Grafana y Valkey:

```go
exec.Command("docker", "compose", "-f", "grafana-valkey/docker-compose.yml", "up", "-d").Run()
```

Luego me conecto a Valkey y cargo el módulo del kernel:

```go
conectarValkey()
exec.Command("bash", "kernel_module/load_module.sh").Run()
```

Esto asegura que antes de comenzar el monitoreo ya exista:

- almacenamiento,
- visualización,
- y la fuente de datos `/proc`.

---

## 4.7 Inicialización del estado base

Después llamo a:

```go
inicializarEstadoBase()
```

Esta función revisa si ya existen contenedores del proyecto. Si no existen, crea el estado base inicial: 3 bajos y 2 altos.

La lógica es esta:

```go
bajos, altos := leerProcesosDesdeProc()
totalProyecto := len(bajos) + len(altos)

if totalProyecto == 0 {
	crearEstadoBase()
}
```

Esto evita duplicar contenedores cuando el sistema ya estaba levantado.

---

## 4.8 Clasificación de procesos

La clasificación se hace con esta función:

```go
func clasificarProceso(proceso ProcesoInfo) string
```

Dentro de ella tomé decisiones basadas en nombre y línea de comando.

### Bajo consumo

```go
if strings.Contains(cmd, "sleep 240") || nombre == "sleep" {
	return "bajo"
}
```

Esto identifica a los contenedores `alpine sleep 240`.

### Alto consumo RAM

```go
if strings.Contains(nombre, "client") || strings.Contains(cmd, "go-client") {
	return "alto"
}
```

Esto identifica al contenedor `roldyoran/go-client`.

### Alto consumo CPU

```go
if strings.Contains(cmd, "while true") || strings.Contains(cmd, "bc") || nombre == "sh" {
	return "alto"
}
```

Esto identifica al contenedor Alpine que ejecuta un bucle infinito con `bc`.

Si no coincide con ningún patrón, por seguridad lo clasifico como alto:

```go
return "alto"
```

---

## 4.9 Lectura del archivo `/proc`

La función principal de lectura es:

```go
func leerProcesosDesdeProc() ([]ProcesoInfo, []ProcesoInfo)
```

Primero abro el archivo:

```go
archivo, err := os.Open(pathProc)
```

Luego lo leo línea por línea:

```go
escaner := bufio.NewScanner(archivo)
for escaner.Scan() {
	partes := strings.Split(escaner.Text(), "|")
```

Como el formato del módulo está separado por `|`, me resulta sencillo obtener cada campo.

---

## 4.10 Procesamiento de la línea del sistema

Cuando la línea comienza con `SISTEMA`, actualizo información en Valkey:

```go
rdb.HSet(ctx, "system:now", map[string]interface{}{
	"total": partes[1],
	"libre": partes[2],
	"uso":   partes[3],
})
```

Aquí guardo el estado actual de memoria en una estructura hash.

Además, genero un dato histórico:

```go
datoGrafana := fmt.Sprintf("t=%d,v=%s", ahora, partes[3])
rdb.LPush(ctx, "history:ram", datoGrafana)
rdb.LTrim(ctx, "history:ram", 0, 100)
```

Con esto guardo el historial de RAM usada. El valor se guarda como par `t=timestamp, v=valor`.

Luego limito la lista a 101 registros efectivos, porque el `LTrim(0,100)` conserva índices 0 a 100.

---

## 4.11 Procesamiento de líneas de procesos

Cuando la línea corresponde a un proceso, verifico:

```go
if partes[0] != "PROCESO" || len(partes) < 9 {
	continue
}
```

Después descarto infraestructura:

```go
if strings.Contains(nombreLower, "grafana") ||
	strings.Contains(nombreLower, "valkey") ||
	strings.Contains(nombreLower, "redis") {
	continue
}
```

Esto refuerza el filtro del kernel. Aunque el kernel ya excluye estos procesos, aquí agregué otra capa de seguridad.

---

## 4.12 Limpieza del ID de contenedor

El ID del contenedor puede venir con prefijos o sufijos, así que lo limpio:

```go
idLimpio := strings.TrimPrefix(partes[3], "docker-")
idLimpio = strings.TrimSuffix(idLimpio, ".scope")
```

Luego creo la estructura `ProcesoInfo`:

```go
proceso := ProcesoInfo{
	PID:    partes[1],
	Nombre: partes[2],
	ID:     idLimpio,
	Cmd:    partes[4],
	VSZ:    partes[5],
	RSS:    partes[6],
	PMem:   partes[7],
	PCPU:   partes[8],
}
```

Y convierto los campos numéricos:

```go
proceso.VSZValue, _ = strconv.ParseFloat(proceso.VSZ, 64)
proceso.RSSValue, _ = strconv.ParseFloat(proceso.RSS, 64)
proceso.PMemVal, _ = strconv.ParseFloat(proceso.PMem, 64)
proceso.PCPUVal, _ = strconv.ParseFloat(proceso.PCPU, 64)
```

---

## 4.13 Almacenamiento de rankings

Una vez parseado el proceso, guardo rankings en Valkey:

```go
member := fmt.Sprintf("ID: %s", idCorto)
rdb.ZAdd(ctx, "ranking:ram", redis.Z{Score: proceso.PMemVal, Member: member})
rdb.ZAdd(ctx, "ranking:cpu", redis.Z{Score: proceso.PCPUVal, Member: member})
```

Aquí uso `ZAdd` porque un `sorted set` de Valkey me permite guardar valores ordenados por score.

- En `ranking:ram`, el score es `%MEM`.
- En `ranking:cpu`, el score es el valor de CPU calculado por el módulo.

Esto me facilita luego obtener el Top 5 en Grafana con consultas como:

```text
ZRANGE ranking:ram -5 -1
ZRANGE ranking:cpu -5 -1
```

---

## 4.14 Creación de contenedores

Para cumplir con la política del proyecto, implementé funciones de creación.

### Contenedor bajo

```go
func crearContenedorBajo() {
	exec.Command("docker", "run", "-d", "alpine", "sleep", "240").Run()
}
```

Este contenedor simula bajo consumo porque se mantiene en reposo.

### Contenedor alto

```go
func crearContenedorAlto() {
	tipo := rng.Intn(2)

	if tipo == 0 {
		exec.Command("docker", "run", "-d", "roldyoran/go-client").Run()
	} else {
		exec.Command("docker", "run", "-d", "alpine", "sh", "-c", "while true; do echo '2^20' | bc > /dev/null; sleep 2; done").Run()
	}
}
```

Aquí seleccioné aleatoriamente entre:

- un contenedor de alto consumo RAM,
- o un contenedor de alto consumo CPU.

---

## 4.15 Creación del estado base

La función:

```go
func crearEstadoBase()
```

crea exactamente:

- 3 contenedores bajos
- 2 contenedores altos

```go
for i := 0; i < 3; i++ {
	crearContenedorBajo()
}

for i := 0; i < 2; i++ {
	crearContenedorAlto()
}
```

Esta fue la base del sistema para garantizar el estado mínimo exigido.

---

## 4.16 Reposición de contenedores bajos

Como los contenedores `sleep 240` eventualmente terminan, implementé una reposición:

```go
func reponerBajosFaltantes(cantidadBajos int) {
	for cantidadBajos < 3 {
		crearContenedorBajo()
		cantidadBajos++
	}
}
```

De esta forma mantengo siempre los tres contenedores de bajo consumo.

---

## 4.17 Política de eliminación de excedentes

La función:

```go
func gestionarExcedentes(lista []ProcesoInfo, limite int, etiqueta string)
```

se encarga de eliminar contenedores cuando hay más de los permitidos.

Primero ordeno la lista:

```go
sort.Slice(lista, func(i, j int) bool {
	if lista[i].RSSValue != lista[j].RSSValue {
		return lista[i].RSSValue > lista[j].RSSValue
	}
	if lista[i].VSZValue != lista[j].VSZValue {
		return lista[i].VSZValue > lista[j].VSZValue
	}
	return lista[i].PMemVal > lista[j].PMemVal
})
```

La política que definí fue:

1. RSS como criterio principal,
2. VSZ como desempate,
3. `%MEM` como criterio adicional.

Luego elimino los excedentes:

```go
exec.Command("docker", "rm", "-f", lista[i].ID).Run()
```

y registro la eliminación:

```go
rdb.LPush(ctx, "history:deletions", ahora)
```

Así construyo el historial de contenedores eliminados.

---

## 4.18 Ciclo de monitoreo

La función principal de control es:

```go
func ejecutarCicloMonitoreo()
```

Su flujo es:

1. leer el estado actual,
2. eliminar excedentes,
3. esperar un poco,
4. volver a leer el estado,
5. reponer bajos faltantes,
6. mostrar el estado final.

Ese flujo quedó así:

```go
bajos, altos := leerProcesosDesdeProc()
gestionarExcedentes(bajos, 3, "BAJO CONSUMO")
gestionarExcedentes(altos, 2, "ALTO CONSUMO")
time.Sleep(2 * time.Second)
bajosActualizados, _ := leerProcesosDesdeProc()
reponerBajosFaltantes(len(bajosActualizados))
time.Sleep(2 * time.Second)
```

Esto permite que el sistema converja nuevamente al estado esperado.

---

## 4.19 Cronjob

Configuré un cronjob para ejecutar el modo `spawn` cada dos minutos.

```go
func configurarCronjob() {
	ejecutable, _ := os.Executable()
	lineaCron := fmt.Sprintf("*/2 * * * * %s spawn", ejecutable)
	comando := fmt.Sprintf(`(crontab -l 2>/dev/null | grep -v "%s"; echo "%s") | crontab -`, ejecutable, lineaCron)
	exec.Command("bash", "-c", comando).Run()
}
```

Aquí hago varias cosas:

- obtengo la ruta del ejecutable del daemon,
- construyo la línea del cron,
- elimino una posible línea anterior del mismo daemon,
- y agrego una sola línea limpia.

Con esto evité duplicar cronjobs.

---

## 4.20 Creación aleatoria por cronjob

La función que ejecuta el cronjob es:

```go
func crearLoteContenedores()
```

Aquí creo 5 contenedores aleatorios:

```go
for i := 0; i < 5; i++ {
	tipo := rng.Intn(3)
```

Cada uno puede ser:

- alto consumo RAM,
- alto consumo CPU,
- bajo consumo.

Luego guardo un mensaje en Valkey:

```go
rdb.RPush(ctx, "logs:spawn", fmt.Sprintf("Contenedor %d/5 creado: %s", i+1, mensaje))
```

Así el daemon luego puede leer esos mensajes y reportarlos en consola.

---

## 4.21 Manejo de limpieza al salir

Implementé una limpieza controlada:

```go
func configurarLimpiezaAlSalir()
```

Aquí escucho señales del sistema:

```go
signal.Notify(canalSenales, os.Interrupt, syscall.SIGTERM)
```

y cuando el proceso termina, elimino el cronjob asociado y descargo el módulo del kernel:

```go
exec.Command("bash", "-c", comando).Run()
exec.Command("sudo", "rmmod", "continfo_pr2_so1_202102140").Run()
```

Con esto evité dejar elementos colgados cuando el daemon se cierra.

---

<br> 
<br> 
<br> 

## 5. Dockerfile

El `Dockerfile` quedó así:

```dockerfile
FROM grafana/grafana:11.0.0
RUN grafana-cli plugins install redis-datasource
USER root
RUN mkdir -p /var/lib/grafana && chown -R grafana:grafana /var/lib/grafana
USER grafana
```

### Instrucción 1

```dockerfile
FROM grafana/grafana:11.0.0
```

Aquí uso la imagen oficial de Grafana versión 11.0.0 como base.

### Instrucción 2

```dockerfile
RUN grafana-cli plugins install redis-datasource
```

Instalo el plugin necesario para que Grafana pueda leer datos desde Redis o Valkey.

### Instrucción 3

```dockerfile
USER root
```

Cambio temporalmente al usuario root para poder crear carpetas y asignar permisos.

### Instrucción 4

```dockerfile
RUN mkdir -p /var/lib/grafana && chown -R grafana:grafana /var/lib/grafana
```

Creo el directorio de datos de Grafana y asigno los permisos correctos.

### Instrucción 5

```dockerfile
USER grafana
```

Regreso al usuario de Grafana para ejecutar la aplicación con permisos adecuados.

---

<br> 
<br> 
<br> 


## 6. docker-compose.yml

El archivo quedó así:

```yaml
services:
  valkey:
    image: valkey/valkey:latest
    container_name: valkey-db
    ports:
      - "6379:6379"
    volumes:
      - ./valkey-data:/data
  grafana:
    build: .
    container_name: grafana-valkey
    ports:
      - "3001:3000"
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
    depends_on:
      - valkey
    volumes:
      - ./grafana-data:/var/lib/grafana
```

Lo explico por bloques.

### Servicio Valkey

```yaml
valkey:
  image: valkey/valkey:latest
  container_name: valkey-db
```

Aquí defino el servicio de Valkey, le asigno un nombre fijo y uso la imagen oficial.

```yaml
ports:
  - "6379:6379"
```

Expongo el puerto 6379 para que el daemon pueda conectarse desde el host.

```yaml
volumes:
  - ./valkey-data:/data
```

Monto un volumen local para persistir los datos de Valkey.

### Servicio Grafana

```yaml
grafana:
  build: .
  container_name: grafana-valkey
```

Aquí le indico a Docker Compose que construya Grafana usando mi `Dockerfile`.

```yaml
ports:
  - "3001:3000"
```

Mapeo el puerto 3001 del host al 3000 del contenedor.

```yaml
environment:
  - GF_SECURITY_ADMIN_USER=admin
  - GF_SECURITY_ADMIN_PASSWORD=admin
```

Defino el usuario y contraseña iniciales de Grafana.

```yaml
depends_on:
  - valkey
```

Aseguro que Valkey esté disponible antes de Grafana.

```yaml
volumes:
  - ./grafana-data:/var/lib/grafana
```

Monto un volumen para persistir dashboards, configuraciones y plugins.

---

<br> 
<br> 
<br> 

## 7. Scripts del proyecto

## 7.1 start.sh

El script `start.sh` quedó así:

```bash
#!/bin/bash
echo "--- INICIANDO SISTEMA (MODO PERSISTENTE) ---"

echo "Compilando Daemon de Go..."
cd go_daemon || exit 1
go build -o ../sopes-daemon daemon.go
cd .. || exit 1

if [ -f daemon.pid ]; then
    echo "Reiniciando proceso del Daemon..."
    kill "$(cat daemon.pid)" 2>/dev/null
    rm -f daemon.pid
fi

./sopes-daemon &
echo $! > daemon.pid

echo "Daemon iniciado correctamente."
```

Yo lo usé para compilar e iniciar el daemon.

### Compilación

```bash
go build -o ../sopes-daemon daemon.go
```

Compilo el daemon y genero el ejecutable `sopes-daemon`.

### Reinicio limpio

```bash
if [ -f daemon.pid ]; then
    kill "$(cat daemon.pid)" 2>/dev/null
    rm -f daemon.pid
fi
```

Si ya existía una instancia previa del daemon, la detengo primero.

### Inicio en segundo plano

```bash
./sopes-daemon &
echo $! > daemon.pid
```

Lanzo el daemon en segundo plano y guardo su PID en un archivo.

---

## 7.2 stop.sh

El script `stop.sh` quedó así:

```bash
#!/bin/bash
echo "--- DETENIENDO SERVICIO (MANTENIENDO DATOS) ---"
...
```

Aunque el archivo original tenga texto decorativo, aquí me enfoco en la lógica funcional.

### Detención del daemon

```bash
if [ -f daemon.pid ]; then
    kill "$(cat daemon.pid)" 2>/dev/null
    rm -f daemon.pid
fi
```

Detengo el proceso principal del daemon.

### Eliminación del cronjob

```bash
EJECUTABLE="$(realpath ./sopes-daemon)"
(crontab -l 2>/dev/null | grep -F -v "$EJECUTABLE spawn") | crontab -
```

Aquí elimino solamente la línea del cronjob asociada al proyecto.

### Detención de infraestructura

```bash
cd grafana-valkey || exit 1
docker compose stop
cd .. || exit 1
```

Detengo Grafana y Valkey, pero sin borrar volúmenes ni contenedores persistentes.

### Eliminación de contenedores de carga

```bash
docker rm -f $(docker ps -a -q --filter "ancestor=alpine") 2>/dev/null
docker rm -f $(docker ps -a -q --filter "ancestor=roldyoran/go-client") 2>/dev/null
```

Aquí limpio los contenedores de prueba, ya que esos sí son desechables.

---

## 7.3 load_module.sh

El script de carga del módulo quedó así:

```bash
#!/bin/bash
cd "$(dirname "$0")" || exit 1
make
if lsmod | grep -q "^continfo_pr2_so1_202102140"; then
    ...
else
    sudo insmod continfo_pr2_so1_202102140.ko
fi
```

### Compilación del módulo

```bash
make
```

Compilo el módulo usando el `Makefile`.

### Verificación de carga

```bash
if lsmod | grep -q "^continfo_pr2_so1_202102140"; then
```

Reviso si el módulo ya estaba cargado.

### Carga del módulo

```bash
sudo insmod continfo_pr2_so1_202102140.ko
```

Si no estaba cargado, lo inserto en el kernel.

---

<br> 
<br> 
<br> 

## 8. Makefile del módulo

El `Makefile` quedó así:

```makefile
obj-m += continfo_pr2_so1_202102140.o
DIR_KERNEL := /lib/modules/$(shell uname -r)/build
DIR_ACTUAL := $(shell pwd)
```

### Objeto del módulo

```makefile
obj-m += continfo_pr2_so1_202102140.o
```

Aquí indico cuál es el objeto que se compilará como módulo de kernel.

### Directorio del kernel

```makefile
DIR_KERNEL := /lib/modules/$(shell uname -r)/build
```

Aquí apunto al árbol de compilación del kernel actual.

### Directorio actual

```makefile
DIR_ACTUAL := $(shell pwd)
```

Guardo la ruta actual del módulo.

### Regla principal

```makefile
all:
	$(MAKE) -C $(DIR_KERNEL) M=$(DIR_ACTUAL) modules
```

Compilo el módulo llamando al sistema de compilación del kernel.

### Limpieza

```makefile
clean:
	$(MAKE) -C $(DIR_KERNEL) M=$(DIR_ACTUAL) clean
```

Limpio artefactos generados.

---

<br> 
<br> 
<br> 

## 9. Dashboard en Grafana

En Grafana configuré varias visualizaciones.

### Top 5 RAM

Usé `ranking:ram` con `ZRANGE -5 -1`.  
Esta métrica representa `%MEM`, es decir, porcentaje de memoria total consumido por el proceso.

### Top 5 CPU

Usé `ranking:cpu` con `ZRANGE -5 -1`.  
En este caso la métrica representa el valor de CPU calculado por el módulo. No la interpreté como porcentaje tradicional fijo, sino como valor comparativo para ranking.

### RAM en el tiempo

Usé la lista `history:ram`, que guarda pares `t=timestamp` y `v=uso`.  
Luego en Grafana apliqué transformaciones para extraer `t` y `v`, convertir el tiempo y mostrarlo como serie temporal.

### RAM total, usada y libre

Estas métricas provienen de `system:now`, que es un hash en Valkey actualizado por el daemon.

### Contenedores eliminados

Esta métrica proviene de la lista `history:deletions`, donde registro un timestamp cada vez que elimino un contenedor excedente.

---

<br> 
<br> 
<br> 

## 10. Decisiones técnicas importantes

### Uso de RSS como criterio principal de eliminación

Elegí RSS como criterio principal porque representa memoria residente real. Después utilicé VSZ como desempate y `%MEM` como criterio adicional.

### Uso del filtro `containerd-shim`

Elegí filtrar procesos cuyo padre es `containerd-shim` porque en mi entorno de laboratorio este patrón identificó correctamente los procesos asociados a contenedores.

### Manejo de CPU como valor comparativo

El cálculo de CPU no replica exactamente el comportamiento de herramientas como `top`. Por eso decidí usarlo como métrica comparativa e histórica, útil para ranking, no como porcentaje absoluto estricto.

### Uso de cronjob

El cronjob me permitió generar variabilidad automática en la carga del sistema sin depender de intervención manual constante.

---
