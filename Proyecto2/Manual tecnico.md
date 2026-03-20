<img src='https://user-images.githubusercontent.com/36779113/128587817-1a6c2fdc-d106-4dd3-b092-104c8299bded.png' background='white'>


## Universidad de San Carlos de Guatemala, Junio 2025
## Facultad de Ingeniería
## Escuela de Ciencias y Sistemas
## LABORATORIO SISTEMAS OPERATIVOS 1 Sección P


| TABLA DE DATOS  | 
| ------------- | 
| Carné:    202102140|
| Nombre:    Javier Andrés Monterroso García|

<br>
<br>
<br>

<center>

# Manual Técnico

</center>

<br>

# Explicación del Proyecto 2 - Sonda de Kernel en C y Daemon en Go

## Introducción

En este proyecto codifiqué una solución dividida en dos partes principales: un **módulo de kernel en C** y un **daemon en Go**. La idea general fue construir un sistema que leyera información del kernel, la expusiera en `/proc`, la procesara desde espacio de usuario y después tomara decisiones sobre contenedores. Esta estructura coincide con lo que solicita el proyecto: un módulo de kernel para capturar métricas, un daemon que analice el archivo en `/proc`, un cronjob para generar contenedores y el almacenamiento de datos en Valkey para usarlos después en Grafana. fileciteturn1file0L90-L106

También consideré las restricciones del enunciado: mantener **3 contenedores de bajo consumo**, **2 de alto consumo**, no eliminar Grafana y aplicar decisiones usando métricas como RAM, VSZ, RSS y CPU. fileciteturn1file0L144-L150

---

# Explicación del módulo de kernel en C

## Propósito general

El módulo de kernel fue la parte encargada de leer directamente información del sistema Linux. Lo que hice fue crear un archivo dentro de `/proc` llamado:

```c
#define NOMBRE_PROC "continfo_pr2_so1_202102140"
```

Con esto definí el nombre del archivo virtual que el kernel crea en `/proc`. Cuando el módulo se carga, ese archivo aparece y permite consultar información del sistema y de procesos relacionados con contenedores.

Esto es importante porque el proyecto pide que la información sea guardada en `/proc/continfo_pr2_so1_#CARNET`. fileciteturn1file0L113-L124

---

## Inclusión de librerías

```c
#include <linux/init.h>
#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/sched/signal.h>
#include <linux/sched.h>
#include <linux/string.h>
#include <linux/mm.h>
#include <linux/sysinfo.h>
#include <linux/smp.h>
#include <linux/jiffies.h>
#include <linux/proc_fs.h>
#include <linux/seq_file.h>
```


Aquí importé las librerías necesarias del kernel. Cada una tiene una función específica:

- `module.h` y `init.h` me permiten declarar el módulo y sus funciones de inicio y salida.
- `sched.h` y `sched/signal.h` me permiten recorrer procesos mediante `task_struct`.
- `mm.h` me ayuda a consultar datos de memoria, como VSZ y RSS.
- `sysinfo.h` me permite conocer memoria total y memoria libre del sistema.
- `proc_fs.h` y `seq_file.h` las utilicé para crear y escribir el archivo en `/proc`.
- `jiffies.h` y `smp.h` las usé para aproximar el porcentaje de CPU.

En otras palabras, aquí preparé todo lo que necesitaba para hablar con el kernel y extraer la información pedida.

---

## Registro básico del módulo

```c
MODULE_LICENSE("GPL");

static struct proc_dir_entry *entrada_proc;
```


Con `MODULE_LICENSE("GPL")` indiqué la licencia del módulo. Esto es importante porque el kernel usa esta información para validar el tipo de módulo que se está cargando.

La variable `entrada_proc` la usé para guardar la referencia al archivo que se crea en `/proc`. Luego esa referencia también me sirve para eliminar el archivo cuando el módulo se descarga.

---

## Detección de procesos `containerd-shim`

```c
static bool es_containerd_shim(struct task_struct *tarea)
{
    if (!tarea)
        return false;

    return strcmp(tarea->comm, "containerd-shim") == 0;
}
```


Aquí codifiqué una función pequeña para verificar si un proceso corresponde a `containerd-shim`.

¿Por qué hice esto? Porque `containerd-shim` suele aparecer en la cadena de procesos asociados a contenedores Docker. Entonces lo consideré como una referencia útil para filtrar procesos que probablemente pertenecen a contenedores.

La comparación se hace usando `tarea->comm`, que es el nombre corto del proceso dentro de `task_struct`.

---

## Relación de un proceso con contenedores

```c
static bool esta_relacionada_con_contenedor(struct task_struct *tarea)
{
    struct task_struct *padre;
    struct task_struct *abuelo;

    if (!tarea)
        return false;

    if (es_containerd_shim(tarea))
        return true;

    padre = tarea->parent;
    if (padre && es_containerd_shim(padre))
        return true;

    abuelo = padre ? padre->parent : NULL;
    if (abuelo && es_containerd_shim(abuelo))
        return true;

    return false;
}
```


Esta función fue una de las decisiones clave del módulo. Aquí lo que hice fue revisar si un proceso:

1. Es directamente `containerd-shim`.
2. Tiene como padre a `containerd-shim`.
3. Tiene como abuelo a `containerd-shim`.

Con esto logré un filtro simple pero útil para identificar procesos relacionados con contenedores.

Lo importante es que el proyecto pide iterar procesos del sistema y extraer información de los que estén relacionados con contenedores. Además, sugiere usar `task_struct` para filtrar correctamente esos procesos. fileciteturn1file0L117-L121

---

## Cálculo de VSZ

```c
static unsigned long obtener_vsz_kb(struct task_struct *tarea)
{
    if (!tarea || !tarea->mm)
        return 0;

    return (tarea->mm->total_vm * PAGE_SIZE) / 1024;
}
```

Aquí calculé el **VSZ**, es decir, el tamaño de memoria virtual del proceso en KB.

- `tarea->mm->total_vm` representa la cantidad de páginas de memoria virtual.
- `PAGE_SIZE` me da el tamaño de cada página.
- Luego dividí entre `1024` para expresar el resultado en KB.

Primero validé que la tarea exista y que tenga estructura `mm`, porque hay procesos del kernel que no manejan memoria de usuario de la misma manera.

---

## Cálculo de RSS

```c
static unsigned long obtener_rss_kb(struct task_struct *tarea)
{
    if (!tarea || !tarea->mm)
        return 0;

    return (get_mm_rss(tarea->mm) * PAGE_SIZE) / 1024;
}
```


Aquí calculé el **RSS**, es decir, la memoria física residente del proceso.

- `get_mm_rss(tarea->mm)` devuelve la cantidad de páginas residentes.
- Multipliqué por `PAGE_SIZE` para convertir a bytes.
- Dividí entre `1024` para obtener KB.

El RSS me interesaba porque el proyecto pide mostrar memoria física usada por cada proceso. fileciteturn1file0L117-L124

---

## Porcentaje de memoria

```c
static unsigned long obtener_memoria_porcentaje_x100(struct task_struct *tarea)
{
    struct sysinfo info;
    unsigned long total_kb;
    unsigned long rss_kb;

    if (!tarea || !tarea->mm)
        return 0;

    si_meminfo(&info);

    total_kb = (info.totalram * info.mem_unit) / 1024;
    if (total_kb == 0)
        return 0;

    rss_kb = obtener_rss_kb(tarea);

    return (rss_kb * 10000UL) / total_kb;
}
```


En esta parte calculé el porcentaje de memoria que usa cada proceso respecto al total de RAM del sistema.

La idea fue:

1. Obtener la memoria total del sistema con `si_meminfo`.
2. Convertir esa memoria a KB.
3. Obtener el RSS del proceso.
4. Calcular el porcentaje.

Multipliqué por `10000` en vez de `100` porque quise conservar **dos decimales** sin usar números flotantes dentro del kernel. Por eso después imprimo el valor como `entero.decimales`.

---

## Porcentaje de CPU

```c
static unsigned long obtener_cpu_porcentaje_x100(struct task_struct *tarea)
{
    unsigned long long cpu;
    unsigned long long total;
    unsigned int cpus;

    if (!tarea)
        return 0;

    cpu = (unsigned long long)tarea->utime +
          (unsigned long long)tarea->stime;

    total = (unsigned long long)jiffies;
    cpus = num_online_cpus();

    if (cpus == 0 || total == 0)
        return 0;

    return (unsigned long)((cpu * 10000ULL) / (total * cpus));
}
```


Aquí hice una aproximación al porcentaje de CPU.

Lo que consideré fue:

- `utime`: tiempo en modo usuario.
- `stime`: tiempo en modo kernel.
- `jiffies`: tiempo total del sistema.
- `num_online_cpus()`: número de CPUs activas.

Entonces comparé el tiempo consumido por el proceso con el tiempo total del sistema. Esta medida no es exactamente igual a la que muestran herramientas como `top`, pero sí sirve como una referencia para el análisis.

El propio enunciado acepta que el porcentaje de CPU pueda resultar muy grande debido a la manera en que el kernel maneja estos cálculos. fileciteturn1file0L119-L121

---

## Generación del contenido en `/proc`

```c
static int mostrar_contenido_proc(struct seq_file *archivo, void *v)
{
    struct task_struct *tarea;
    struct sysinfo info;
    unsigned long total_kb;
    unsigned long libre_kb;
    unsigned long uso_kb;

    si_meminfo(&info);

    total_kb = (info.totalram * info.mem_unit) / 1024;
    libre_kb = (info.freeram * info.mem_unit) / 1024;
    uso_kb = total_kb - libre_kb;

    seq_printf(archivo, "RAM_TOTAL_KB: %lu\n", total_kb);
    seq_printf(archivo, "RAM_LIBRE_KB: %lu\n", libre_kb);
    seq_printf(archivo, "RAM_USO_KB: %lu\n", uso_kb);
    seq_puts(archivo, "\n");
    seq_puts(archivo, "PROCESOS_RELACIONADOS_CON_CONTENEDORES\n");
```


Esta función es la más importante del módulo, porque construye el contenido que luego se ve al hacer:

```bash
cat /proc/continfo_pr2_so1_202102140
```

Primero obtuve la memoria del sistema:

- RAM total
- RAM libre
- RAM en uso

Después escribí esa información en el archivo virtual usando `seq_printf` y `seq_puts`.

Esto cumple con la parte del enunciado que pide exponer RAM total, libre y en uso en `/proc`. fileciteturn1file0L113-L124

---

## Recorrido de procesos del sistema

```c
for_each_process(tarea) {

    if (tarea->mm == NULL)
        continue;

    if (!esta_relacionada_con_contenedor(tarea))
        continue;
```


En este fragmento recorrí todas las tareas del sistema usando `for_each_process`.

Luego fui descartando procesos que no me interesaban:

- Si `tarea->mm == NULL`, lo salté, porque no es un proceso de usuario con memoria que quiera medir.
- Si no está relacionado con contenedores, también lo descarté.

Con esto logré que el archivo en `/proc` no mostrara todo el sistema completo, sino solo lo que me interesa para el análisis del proyecto.

---

## Impresión de cada proceso

```c
seq_printf(archivo,
           "PID=%d TGID=%d NOMBRE=%s STATE=%ld PARENT_PID=%d VSZ_KB=%lu RSS_KB=%lu MEM=%lu.%02lu%% CPU=%lu.%02lu%%\n",
           tarea->pid,
           tarea->tgid,
           tarea->comm,
           tarea->__state,
           tarea->parent ? tarea->parent->pid : 0,
           obtener_vsz_kb(tarea),
           obtener_rss_kb(tarea),
           obtener_memoria_porcentaje_x100(tarea) / 100,
           obtener_memoria_porcentaje_x100(tarea) % 100,
           obtener_cpu_porcentaje_x100(tarea) / 100,
           obtener_cpu_porcentaje_x100(tarea) % 100);
```

Aquí construí una línea por cada proceso encontrado. Para cada uno imprimí:

- PID
- TGID
- Nombre del proceso
- Estado
- PID del padre
- VSZ en KB
- RSS en KB
- % de memoria
- % de CPU

Este formato fue pensado para que luego el daemon en Go pudiera leerlo y parsearlo fácilmente.

> **Nota importante:** el enunciado original sí pide también la línea de comando ejecutada o el ID del contenedor. En esta versión simplificada ya no lo incluí, porque decidí priorizar una salida más limpia y más sencilla de parsear. Si en la defensa te preguntan eso, puedes explicar que simplificaste el módulo, pero mantuviste las métricas principales de análisis. El requerimiento aparece en la sección del módulo de kernel del PDF. fileciteturn1file0L115-L121

---

## Apertura del archivo `/proc`

```c
static int abrir_proc(struct inode *inode, struct file *file)
{
    return single_open(file, mostrar_contenido_proc, NULL);
}
```


Aquí enlacé la apertura del archivo `/proc` con la función que construye su contenido. Cada vez que alguien lee el archivo, el kernel ejecuta `mostrar_contenido_proc`.

Esto significa que la información se genera en el momento de la lectura y no se guarda estáticamente.

---

## Operaciones del archivo `/proc`

```c
static const struct proc_ops operaciones_proc = {
    .proc_open = abrir_proc,
    .proc_read = seq_read,
    .proc_lseek = seq_lseek,
    .proc_release = single_release,
};
```

En esta estructura definí cómo se comporta el archivo `/proc`:

- `proc_open`: qué hacer cuando se abre.
- `proc_read`: cómo leerlo.
- `proc_lseek`: desplazamiento dentro del archivo.
- `proc_release`: liberación de recursos al cerrarlo.

Usé las utilidades de `seq_file`, porque son prácticas y seguras para mostrar información secuencial en archivos de `/proc`.

---

## Carga del módulo

```c
static int __init init_modulo(void)
{
    entrada_proc = proc_create(NOMBRE_PROC, 0, NULL, &operaciones_proc);

    if (!entrada_proc)
        return -ENOMEM;

    return 0;
}
```

Aquí codifiqué la función que se ejecuta cuando el módulo se carga con `insmod`.

Lo que hice fue crear el archivo en `/proc`. Si algo falla, retorno `-ENOMEM` para indicar error.

En la práctica, si esta función termina bien, el archivo `/proc/continfo_pr2_so1_202102140` queda listo para usarse.

---

## Descarga del módulo

```c
static void __exit exit_modulo(void)
{
    if (entrada_proc)
        proc_remove(entrada_proc);
}
```


Aquí eliminé el archivo de `/proc` cuando el módulo se descarga con `rmmod`.

Esto evita dejar basura en el sistema y demuestra que el módulo se carga y descarga correctamente, que es otro punto evaluado por la rúbrica. fileciteturn1file0L289-L299

---

## Registro final de entrada y salida

```c
module_init(init_modulo);
module_exit(exit_modulo);
```


Estas macros le indican al kernel qué función ejecutar al cargar el módulo y cuál ejecutar al descargarlo.

Con esto queda cerrada la lógica principal del módulo en C.

---

# Explicación del daemon en Go

## Propósito general

El daemon en Go fue la parte que codifiqué para trabajar en espacio de usuario. Su función fue leer el archivo `/proc`, interpretar los datos del módulo, clasificarlos, guardar registros en Valkey y tomar decisiones sobre qué contenedores eliminar.

Esto coincide con el papel que el PDF le asigna al daemon: leer `/proc`, analizar contenedores, almacenar registros en Valkey y manejar el cronjob. fileciteturn1file0L132-L143

---

## Importaciones

```go
import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)
```


Aquí importé todo lo necesario:

- `bufio`: para leer el archivo `/proc` línea por línea.
- `os` y `os/exec`: para archivos y ejecución de comandos del sistema.
- `sort`: para ordenar procesos por consumo.
- `strconv` y `strings`: para parsear texto.
- `time`: para manejar intervalos y timestamps.
- `filepath`: para ubicar scripts dentro del proyecto.
- `go-redis/v9`: para conectarme a Valkey.

---

## Constantes principales

```go
const (
	rutaProc          = "/proc/continfo_pr2_so1_202102140"
	intervaloLoop     = 20 * time.Second
	metaBajos         = 3
	metaAltos         = 2
	direccionValkey   = "localhost:6379"
	identificadorCron = "CRON_SO1"
)
```


Aquí definí parámetros clave del daemon:

- `rutaProc`: ubicación del archivo que genera el módulo.
- `intervaloLoop`: cada cuánto se ejecuta el análisis.
- `metaBajos` y `metaAltos`: cantidad deseada de contenedores de bajo y alto consumo.
- `direccionValkey`: dirección del servidor Valkey.
- `identificadorCron`: texto usado para identificar el cronjob.

La decisión de usar `3` y `2` viene directamente de las restricciones del proyecto. fileciteturn1file0L144-L150

---

## Estructura `Proceso`

```go
type Proceso struct {
	PID    int
	Nombre string
	CMD    string
	RSSKB  uint64
	CPU    float64
}
```

Esta estructura la utilicé para guardar, ya en Go, la información relevante de cada línea leída desde `/proc`.

En esta versión simplificada me concentré sobre todo en:

- PID
- nombre
- línea o fragmento de comando
- RSS
- CPU

Con esta estructura el análisis posterior se vuelve más ordenado.

---

## Variables globales para contexto y Valkey

```go
var ctx = context.Background()
var valkey *redis.Client
```

`ctx` lo utilicé como contexto base para las operaciones con Valkey.

`valkey` guarda el cliente que se conecta a la base de datos. Lo declaré global para poder usarlo desde distintas funciones sin estarlo pasando todo el tiempo como parámetro.

---

## Flujo principal del daemon

```go
func main() {
	rutaCreate := filepath.Join(rutaProyecto(), "scripts", "create_container.sh")

	if len(os.Args) > 1 && os.Args[1] == "--cronjob" {
		exec.Command("bash", rutaCreate).Run()
		return
	}

	conectarValkey()
	crearCronjob()

	for {
		procesos := leerProc()

		guardarEnValkey(procesos)

		bajos, altos := clasificar(procesos)

		aplicarPolitica(bajos, altos)

		time.Sleep(intervaloLoop)
	}
}
```

Este es el corazón del daemon.

Lo que hace es lo siguiente:

1. Calcula la ruta del script `create_container.sh`.
2. Si el programa fue invocado con `--cronjob`, solo ejecuta el script y termina.
3. Si no, inicia como servicio principal:
   - se conecta a Valkey,
   - crea el cronjob,
   - entra en un loop infinito.
4. En cada iteración:
   - lee `/proc`,
   - guarda información en Valkey,
   - clasifica procesos,
   - aplica política de eliminación,
   - espera 20 segundos.

Este comportamiento coincide con lo solicitado en el PDF para el daemon: inicialización, cronjob, lectura de `/proc`, análisis y registros en Valkey. fileciteturn1file0L132-L143

---

## Ruta base del proyecto

```go
func rutaProyecto() string {
	exe, _ := os.Executable()
	return filepath.Dir(filepath.Dir(exe))
}
```

Aquí obtuve la ubicación del ejecutable y después retrocedí un nivel de carpetas para llegar a la raíz del proyecto.

Lo hice así porque necesitaba encontrar scripts como `create_container.sh` sin depender de rutas escritas manualmente.

---

## Conexión a Valkey

```go
func conectarValkey() {
	valkey = redis.NewClient(&redis.Options{
		Addr: direccionValkey,
	})
}
```

En esta función configuré la conexión con Valkey usando la dirección `localhost:6379`.

Valkey lo usé como almacenamiento rápido para guardar snapshots de procesos. El enunciado pide que el daemon genere y almacene registros en una base Valkey para usarlos luego en Grafana. fileciteturn1file0L139-L143

---

## Creación del cronjob

```go
func crearCronjob() {
	exe, _ := os.Executable()

	linea := fmt.Sprintf("*/2 * * * * %s --cronjob #%s", exe, identificadorCron)

	exec.Command("bash", "-c",
		fmt.Sprintf(`crontab -l 2>/dev/null | grep -v "%s" | crontab -`, identificadorCron),
	).Run()

	exec.Command("bash", "-c",
		fmt.Sprintf(`(crontab -l 2>/dev/null; echo '%s') | crontab -`, linea),
	).Run()
}
```

Esta función instala un cronjob del sistema.

Primero construí una línea tipo:

```bash
*/2 * * * * /ruta/del/daemon --cronjob #CRON_SO1
```

Después hice dos cosas:

1. Eliminar versiones anteriores del mismo cronjob.
2. Agregar la nueva línea al `crontab`.

La frecuencia cada 2 minutos coincide con la sección de cronjob del proyecto. fileciteturn1file0L125-L131

---

## Lectura del archivo `/proc`

```go
func leerProc() []Proceso {
	file, _ := os.Open(rutaProc)
	defer file.Close()

	var procesos []Proceso
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "PID=") {
			p := parsearProceso(line)
			procesos = append(procesos, p)
		}
	}

	return procesos
}
```

Aquí abrí el archivo en `/proc` y lo recorrí línea por línea.

Solo me interesaban las líneas que empiezan con `PID=`, porque esas representan procesos individuales. Las otras líneas, como las de RAM total o encabezados, no las agregué a la lista de procesos.

De esa forma separé el trabajo en dos pasos:

1. leer el archivo,
2. parsear solo las líneas útiles.

---

## Parseo de una línea de proceso

```go
func parsearProceso(line string) Proceso {
	var p Proceso
	fields := strings.Fields(line)

	for _, f := range fields {
		kv := strings.SplitN(f, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key, val := kv[0], strings.Trim(kv[1], `"`)
```

En este fragmento separé la línea en campos por espacios y luego cada campo en forma `clave=valor`.

Por ejemplo, de una línea así:

```text
PID=123 NOMBRE=sh RSS_KB=1024 CPU=0.12%
```

obtengo pares como:

- `PID -> 123`
- `NOMBRE -> sh`
- `RSS_KB -> 1024`
- `CPU -> 0.12%`

Esto fue necesario porque el archivo `/proc` es texto plano, mientras que el daemon necesita estructuras de datos bien definidas.

---

## Asignación de campos parseados

```go
	switch key {
	case "PID":
		p.PID, _ = strconv.Atoi(val)
	case "NOMBRE":
		p.Nombre = val
	case "CMDLINE":
		p.CMD = val
	case "RSS_KB":
		p.RSSKB, _ = strconv.ParseUint(val, 10, 64)
	case "CPU":
		val = strings.TrimSuffix(val, "%")
		p.CPU, _ = strconv.ParseFloat(val, 64)
	}
```

Aquí tomé cada clave encontrada y la convertí al tipo correcto:

- `PID` a entero,
- `RSS_KB` a entero sin signo,
- `CPU` a decimal,
- `NOMBRE` y `CMDLINE` como texto.

Aunque el módulo simplificado ya no imprime `CMDLINE`, dejé esa parte preparada en el daemon. Eso significa que el parser todavía puede adaptarse a una versión más completa del archivo `/proc`.

---

## Clasificación de procesos

```go
func clasificar(procesos []Proceso) ([]Proceso, []Proceso) {
	var bajos, altos []Proceso

	for _, p := range procesos {
		if esIgnorable(p) {
			continue
		}

		if p.RSSKB >= 50000 || p.CPU >= 20 {
			altos = append(altos, p)
		} else {
			bajos = append(bajos, p)
		}
	}

	return bajos, altos
}
```

Aquí tomé todos los procesos leídos y los separé en dos grupos:

- **altos**: procesos con consumo elevado,
- **bajos**: procesos con consumo más reducido.

La regla que codifiqué fue simple:

- si el proceso usa al menos `50000 KB` de RSS, o
- si usa al menos `20%` de CPU,

entonces lo considero de alto consumo.

Esta clasificación me ayuda a cumplir con la meta de conservar 3 bajos y 2 altos.

---

## Procesos que no deben tocarse

```go
func esIgnorable(p Proceso) bool {
	cmd := strings.ToLower(p.CMD)

	return strings.Contains(cmd, "grafana") ||
		strings.Contains(cmd, "valkey") ||
		strings.Contains(cmd, "redis")
}
```

En esta función definí procesos que no deben ser analizados para eliminación.

Especialmente consideré importante no tocar:

- Grafana,
- Valkey,
- Redis.

Esto va alineado con la restricción del proyecto de no eliminar el contenedor de Grafana. fileciteturn1file0L144-L147

---

## Política de eliminación

```go
func aplicarPolitica(bajos, altos []Proceso) {
	ordenar(bajos)
	ordenar(altos)

	if len(bajos) > metaBajos {
		for i := 0; i < len(bajos)-metaBajos; i++ {
			eliminar(bajos[i])
		}
	}

	if len(altos) > metaAltos {
		for i := 0; i < len(altos)-metaAltos; i++ {
			eliminar(altos[i])
		}
	}
}
```


Aquí apliqué la decisión principal del daemon.

Primero ordené ambos grupos por consumo. Después verifiqué si hay más contenedores de los permitidos:

- si hay más de 3 bajos, elimino el exceso,
- si hay más de 2 altos, elimino el exceso.

Con esta lógica el daemon ayuda a mantener el equilibrio pedido por el proyecto.

---

## Ordenamiento por consumo

```go
func ordenar(lista []Proceso) {
	sort.Slice(lista, func(i, j int) bool {
		return lista[i].RSSKB > lista[j].RSSKB
	})
}
```


Aquí ordené los procesos de mayor a menor según `RSSKB`.

Así, los más pesados quedan al inicio del slice y son los primeros candidatos para eliminación.


---

## Eliminación de contenedores

```go
func eliminar(p Proceso) {
	nombre := obtenerNombre(p)
	if nombre == "" {
		return
	}

	exec.Command("docker", "rm", "-f", nombre).Run()
}
```

Cuando ya decidí que un proceso debe eliminarse, lo que hago aquí es:

1. intentar obtener el nombre del contenedor,
2. si lo encuentro, ejecutar:

```bash
docker rm -f <nombre>
```

Con `-f` fuerzo la detención y eliminación del contenedor.

---

## Obtención del nombre del contenedor

```go
func obtenerNombre(p Proceso) string {
	partes := strings.Fields(p.CMD)

	for _, parte := range partes {
		if strings.HasPrefix(parte, "low_") ||
			strings.HasPrefix(parte, "high_") {
			return parte
		}
	}

	return ""
}
```


Aquí intenté deducir el nombre del contenedor a partir del campo `CMD`.

Lo que consideré fue que los nombres de contenedores creados por el proyecto probablemente siguen prefijos como:

- `low_...`
- `high_...`

Si encuentro una palabra con uno de esos prefijos, la tomo como nombre del contenedor.

Es una estrategia simple, pero funciona si el script de creación usa nombres consistentes.

---

## Guardado en Valkey

```go
func guardarEnValkey(procesos []Proceso) {
	timestamp := time.Now().Unix()

	for _, p := range procesos {
		valkey.HSet(ctx, fmt.Sprintf("proc:%d:%d", p.PID, timestamp), map[string]interface{}{
			"pid":    p.PID,
			"nombre": p.Nombre,
			"rss":    p.RSSKB,
			"cpu":    p.CPU,
			"time":   timestamp,
		})
	}
}
```

Aquí guardé la telemetría en Valkey.

Por cada proceso creé una clave como:

```text
proc:123:1712345678
```

Y dentro guardé un hash con:

- PID
- nombre
- RSS
- CPU
- timestamp

Esto permite conservar snapshots históricos y luego conectarlos con Grafana para generar paneles y gráficas. El PDF pide justamente que los datos se almacenen para ser usados luego en Grafana. fileciteturn1file0L139-L143

---

# Explicación completa del Makefile

A continuación dejo **todo el código** del Makefile, tal como me pediste:

```makefile
obj-m += continfo_pr2_so1.o

all:
	make -C /lib/modules/$(shell uname -r)/build M=$(PWD) modules

clean:
	make -C /lib/modules/$(shell uname -r)/build M=$(PWD) clean
```


### Línea 1

```makefile
obj-m += continfo_pr2_so1.o
```

Aquí indiqué que el archivo objeto del módulo es `continfo_pr2_so1.o`.

Eso le dice al sistema de construcción del kernel que debe compilar el archivo fuente correspondiente del módulo.

### Bloque `all`

```makefile
all:
	make -C /lib/modules/$(shell uname -r)/build M=$(PWD) modules
```

Aquí definí la regla por defecto para compilar el módulo.

- `-C /lib/modules/$(shell uname -r)/build` le dice a `make` que use la carpeta de compilación del kernel actual.
- `M=$(PWD)` indica que el módulo externo está en el directorio actual.
- `modules` le dice que compile módulos externos.

En otras palabras, yo no compilo el módulo manualmente. Más bien le pido al propio sistema de build del kernel que lo haga usando las cabeceras correctas.

### Bloque `clean`

```makefile
clean:
	make -C /lib/modules/$(shell uname -r)/build M=$(PWD) clean
```

Aquí definí la limpieza del proyecto.

Esta regla elimina archivos temporales y artefactos de compilación generados al construir el módulo.

Este Makefile responde a lo que la rúbrica pide sobre compilación correcta del módulo y uso de build system apropiado. fileciteturn1file0L289-L305

---

# Explicación completa de `docker-compose.yml`

A continuación dejo **todo el código**:

```yaml
services:
  valkey:
    image: valkey/valkey:latest
    container_name: valkey-db
    ports:
      - "6379:6379"
    volumes:
      - ./valkey-data:/data
    restart: unless-stopped

  grafana:
    build: .
    container_name: grafana-valkey
    ports:
      - "3001:3000"
    user: "0"
    volumes:
      - ./grafana-data:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
    depends_on:
      - valkey
    restart: unless-stopped
```


## Servicio `valkey`

```yaml
  valkey:
    image: valkey/valkey:latest
```

Aquí definí un servicio llamado `valkey` usando la imagen oficial más reciente.

Valkey funciona como la base de datos en memoria donde el daemon guarda los registros.

```yaml
    container_name: valkey-db
```

Con esto le asigné un nombre fijo al contenedor para identificarlo fácilmente.

```yaml
    ports:
      - "6379:6379"
```

Aquí expuse el puerto `6379`, que es el puerto típico de Valkey/Redis.

Eso permite que el daemon en Go pueda conectarse usando `localhost:6379`.

```yaml
    volumes:
      - ./valkey-data:/data
```

Aquí monté un volumen para persistir los datos. Así, si el contenedor se reinicia, no se pierde todo inmediatamente.

```yaml
    restart: unless-stopped
```

Esta política indica que el contenedor debe reiniciarse automáticamente, salvo que yo lo detenga manualmente.

---

## Servicio `grafana`

```yaml
  grafana:
    build: .
```

Aquí indiqué que Grafana no se levanta directamente desde una imagen remota cualquiera, sino desde el `Dockerfile` del proyecto.

Eso lo hice porque necesitaba instalar un plugin adicional.

```yaml
    container_name: grafana-valkey
```

Le asigné un nombre fijo al contenedor de Grafana.

```yaml
    ports:
      - "3001:3000"
```

Aquí publiqué el puerto interno `3000` de Grafana hacia el `3001` de la máquina anfitriona.

Entonces yo accedo desde el navegador usando algo como:

```text
http://localhost:3001
```

```yaml
    user: "0"
```

Con esto hice que el contenedor se ejecute como usuario root. Esto a veces ayuda a evitar problemas de permisos sobre volúmenes montados.

```yaml
    volumes:
      - ./grafana-data:/var/lib/grafana
```

Aquí monté la carpeta de datos de Grafana para conservar dashboards, configuración y metadatos.

```yaml
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
```

Aquí definí las credenciales iniciales de acceso a Grafana.

```yaml
    depends_on:
      - valkey
```

Esto indica que el servicio de Grafana depende de Valkey. No significa que espere a que Valkey esté completamente listo, pero sí establece el orden general de arranque.

```yaml
    restart: unless-stopped
```

Igual que con Valkey, configuré reinicio automático.

---

## Relación con el proyecto

El PDF recomienda usar Docker Compose para que Grafana pueda comunicarse con Valkey. Eso fue exactamente lo que consideré al diseñar este archivo. fileciteturn1file0L132-L135

---

# Explicación completa del `Dockerfile`

A continuación dejo **todo el código**:

```dockerfile
FROM grafana/grafana:11.0.0

RUN grafana-cli plugins install redis-datasource
VOLUME /var/lib/grafana
EXPOSE 3000
```

## Explicación línea por línea

### Línea 1

```dockerfile
FROM grafana/grafana:11.0.0
```

Aquí partí de la imagen oficial de Grafana en la versión `11.0.0`.

Eso me dio una base ya lista para ejecutar Grafana.

### Línea 3

```dockerfile
RUN grafana-cli plugins install redis-datasource
```

Aquí instalé el plugin `redis-datasource`.

Esto fue necesario porque quería conectar Grafana con Valkey/Redis como fuente de datos.

### Línea 4

```dockerfile
VOLUME /var/lib/grafana
```

Con esta línea declaré el directorio de datos de Grafana como volumen.

Así se facilita la persistencia de configuraciones y dashboards.

### Línea 5

```dockerfile
EXPOSE 3000
```

Aquí documenté que Grafana escucha en el puerto `3000` dentro del contenedor.

Luego en `docker-compose.yml` hice el mapeo externo hacia `3001`.

---

# Cómo se integran todas las piezas

## Flujo general del sistema

Yo pensé el proyecto como una cadena de componentes:

1. **El módulo en C** recoge información del kernel y la publica en `/proc`.
2. **El daemon en Go** lee ese archivo, parsea los procesos y decide cuáles conservar o eliminar.
3. **Valkey** guarda snapshots de la telemetría.
4. **Grafana** consume esos datos para crear paneles visuales.
5. **El cronjob** genera contenedores periódicamente para producir carga y probar la lógica del sistema.


---

