package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

type ProcesoInfo struct {
	PID, Nombre, ID, Cmd, VSZ, RSS, PMem, PCPU string
	VSZValue, RSSValue, PMemVal, PCPUVal       float64
}

var (
	ctx       = context.Background()
	rdb       *redis.Client
	pathProc  = "/proc/continfo_pr2_so1_202102140"
	rng       = rand.New(rand.NewSource(time.Now().UnixNano()))
	intervalo = 20 * time.Second
)

func main() {
	// Modo especial que usa el cronjob para crear una tanda aleatoria
	if len(os.Args) > 1 && os.Args[1] == "spawn" {
		conectarValkey()
		crearLoteContenedores()
		return
	}

	fmt.Println("--- DAEMON GO: INICIANDO SERVICIO COMPLETO ---")

	fmt.Println("Levantando Grafana y Valkey (Docker Compose)...")
	if err := exec.Command("docker", "compose", "-f", "grafana-valkey/docker-compose.yml", "up", "-d").Run(); err != nil {
		log.Fatalf("No se pudo levantar Docker Compose: %v", err)
	}

	fmt.Println("Conectando a Valkey...")
	conectarValkey()

	fmt.Println("Ejecutando script de carga del módulo...")
	if err := exec.Command("bash", "kernel_module/load_module.sh").Run(); err != nil {
		log.Fatalf("No se pudo cargar el módulo del kernel: %v", err)
	}

	// Crear estado base si el sistema aún no tiene contenedores del proyecto
	inicializarEstadoBase()

	configurarCronjob()
	fmt.Println("Cronjob de creación configurado (ejecución cada 2 minutos).")

	configurarLimpiezaAlSalir()

	fmt.Printf("Entrando en Loop Principal (Intervalo de lectura: %s)...\n", intervalo)
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()

	for range ticker.C {
		revisarLogsDeCronjob()
		ejecutarCicloMonitoreo()
	}
}


func clasificarProceso(proceso ProcesoInfo) string {
	nombre := strings.ToLower(proceso.Nombre)
	cmd := strings.ToLower(proceso.Cmd)

	// Bajo consumo: alpine sleep 240
	if strings.Contains(cmd, "sleep 240") || nombre == "sleep" {
		return "bajo"
	}

	// Alto consumo RAM: go-client
	if strings.Contains(nombre, "client") || strings.Contains(cmd, "go-client") {
		return "alto"
	}

	// Alto consumo CPU: alpine con bucle infinito y bc
	if strings.Contains(cmd, "while true") || strings.Contains(cmd, "bc") || nombre == "sh" {
		return "alto"
	}

	return "alto"
}

// Lee /proc y devuelve dos listas:
// - procesos de bajo consumo
// - procesos de alto consumo
func leerProcesosDesdeProc() ([]ProcesoInfo, []ProcesoInfo) {
	archivo, err := os.Open(pathProc)
	if err != nil {
		log.Printf("Error crítico: No se pudo leer %s.", pathProc)
		return nil, nil
	}
	defer archivo.Close()

	var bajos []ProcesoInfo
	var altos []ProcesoInfo

	escaner := bufio.NewScanner(archivo)
	ahora := time.Now().Unix()

	for escaner.Scan() {
		partes := strings.Split(escaner.Text(), "|")
		if len(partes) == 0 {
			continue
		}

		if partes[0] == "SISTEMA" {
			// Guardar datos del sistema para Grafana
			if len(partes) >= 4 {
				rdb.HSet(ctx, "system:now", map[string]interface{}{
					"total": partes[1],
					"libre": partes[2],
					"uso":   partes[3],
				})

				datoGrafana := fmt.Sprintf("t=%d,v=%s", ahora, partes[3])
				rdb.LPush(ctx, "history:ram", datoGrafana)
				rdb.LTrim(ctx, "history:ram", 0, 100)
			}
			continue
		}

		if partes[0] != "PROCESO" || len(partes) < 9 {
			continue
		}

		nombreLower := strings.ToLower(partes[2])

		// Nunca tocar contenedores de infraestructura
		if strings.Contains(nombreLower, "grafana") ||
			strings.Contains(nombreLower, "valkey") ||
			strings.Contains(nombreLower, "redis") {
			continue
		}

		idLimpio := strings.TrimPrefix(partes[3], "docker-")
		idLimpio = strings.TrimSuffix(idLimpio, ".scope")

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

		proceso.VSZValue, _ = strconv.ParseFloat(proceso.VSZ, 64)
		proceso.RSSValue, _ = strconv.ParseFloat(proceso.RSS, 64)
		proceso.PMemVal, _ = strconv.ParseFloat(proceso.PMem, 64)
		proceso.PCPUVal, _ = strconv.ParseFloat(proceso.PCPU, 64)

		idCorto := proceso.ID
		if len(idCorto) > 8 {
			idCorto = idCorto[:8]
		}

		member := fmt.Sprintf("ID: %s", idCorto)
		rdb.ZAdd(ctx, "ranking:ram", redis.Z{Score: proceso.PMemVal, Member: member})
		rdb.ZAdd(ctx, "ranking:cpu", redis.Z{Score: proceso.PCPUVal, Member: member})

		tipo := clasificarProceso(proceso)
		if tipo == "bajo" {
			bajos = append(bajos, proceso)
		} else {
			altos = append(altos, proceso)
		}
	}

	return bajos, altos
}


func crearContenedorBajo() {
	fmt.Println("Creando contenedor de BAJO consumo (alpine sleep 240)...")
	if err := exec.Command("docker", "run", "-d", "alpine", "sleep", "240").Run(); err != nil {
		log.Printf("Error al crear contenedor bajo: %v", err)
	}
}


func crearContenedorAlto() {
	tipo := rng.Intn(2)

	if tipo == 0 {
		fmt.Println("Creando contenedor de ALTO consumo (RAM - go-client)...")
		if err := exec.Command("docker", "run", "-d", "roldyoran/go-client").Run(); err != nil {
			log.Printf("Error al crear contenedor alto RAM: %v", err)
		}
	} else {
		fmt.Println("Creando contenedor de ALTO consumo (CPU - alpine con bc)...")
		if err := exec.Command("docker", "run", "-d", "alpine", "sh", "-c", "while true; do echo '2^20' | bc > /dev/null; sleep 2; done").Run(); err != nil {
			log.Printf("Error al crear contenedor alto CPU: %v", err)
		}
	}
}

// Crea el estado base del sistema:
// 3 bajos + 2 altos
func crearEstadoBase() {
	fmt.Println("No hay contenedores del proyecto. Creando estado base inicial (3 bajos + 2 altos)...")

	for i := 0; i < 3; i++ {
		crearContenedorBajo()
	}

	for i := 0; i < 2; i++ {
		crearContenedorAlto()
	}
}


func inicializarEstadoBase() {
	fmt.Println("Verificando estado inicial de contenedores del proyecto...")

	bajos, altos := leerProcesosDesdeProc()
	totalProyecto := len(bajos) + len(altos)

	if totalProyecto == 0 {
		crearEstadoBase()
	} else {
		fmt.Printf("Ya existen contenedores del proyecto: %d bajos y %d altos. No se crea estado base.\n", len(bajos), len(altos))
	}
}


func reponerBajosFaltantes(cantidadBajos int) {
	for cantidadBajos < 3 {
		fmt.Printf("Faltan contenedores de bajo consumo. Reponiendo... (%d/3 actuales)\n", cantidadBajos)
		crearContenedorBajo()
		cantidadBajos++
	}
}


func gestionarExcedentes(lista []ProcesoInfo, limite int, etiqueta string) {
	if len(lista) > limite {
		fmt.Printf("ALERTA: Excedente en %s (%d/%d).\n", etiqueta, len(lista), limite)

		sort.Slice(lista, func(i, j int) bool {
			if lista[i].RSSValue != lista[j].RSSValue {
				return lista[i].RSSValue > lista[j].RSSValue
			}
			if lista[i].VSZValue != lista[j].VSZValue {
				return lista[i].VSZValue > lista[j].VSZValue
			}
			return lista[i].PMemVal > lista[j].PMemVal
		})

		for i := 0; i < len(lista)-limite; i++ {
			idCorto := lista[i].ID
			if len(idCorto) > 12 {
				idCorto = idCorto[:12]
			}

			fmt.Printf(
				"Borrando: PID %s | ID %s (%s) | RSS=%s KB | VSZ=%s KB | %%MEM=%s\n",
				lista[i].PID,
				idCorto,
				lista[i].Nombre,
				lista[i].RSS,
				lista[i].VSZ,
				lista[i].PMem,
			)

			if err := exec.Command("docker", "rm", "-f", lista[i].ID).Run(); err != nil {
				log.Printf("Error al borrar contenedor %s: %v", idCorto, err)
				continue
			}

			ahora := time.Now().Unix()
			rdb.LPush(ctx, "history:deletions", ahora)

			fmt.Printf("Contenedor %s eliminado.\n", idCorto)
		}
	}
}


func ejecutarCicloMonitoreo() {
	fmt.Printf("\n[%s] Iniciando escaneo de métricas en /proc...\n", time.Now().Format("15:04:05"))

	// Primera lectura del estado actual
	bajos, altos := leerProcesosDesdeProc()

	fmt.Printf("esumen: %d Bajo Consumo | %d Alto Consumo.\n", len(bajos), len(altos))

	// Eliminar excedentes usando la política priorizada
	gestionarExcedentes(bajos, 3, "BAJO CONSUMO")
	gestionarExcedentes(altos, 2, "ALTO CONSUMO")

	time.Sleep(2 * time.Second)

	bajosActualizados, _ := leerProcesosDesdeProc()

	reponerBajosFaltantes(len(bajosActualizados))

	time.Sleep(2 * time.Second)

	bajosFinales, altosFinales := leerProcesosDesdeProc()
	fmt.Printf("Estado final del ciclo: %d Bajo Consumo | %d Alto Consumo.\n", len(bajosFinales), len(altosFinales))

	mostrarEstadoDocker()
}

func mostrarEstadoDocker() {
	fmt.Println("\n--- DOCKER PS ---")
	comandoPS := exec.Command("docker", "ps", "--format", "table {{.ID}}\t{{.Image}}\t{{.Status}}\t{{.Names}}")
	salidaPS, err := comandoPS.Output()
	if err != nil {
		log.Printf("Error al ejecutar docker ps: %v", err)
		return
	}
	fmt.Print(string(salidaPS))
	fmt.Println("-------------------------------------------------------------------------------------")
}


func crearLoteContenedores() {
	for i := 0; i < 5; i++ {
		tipo := rng.Intn(3)
		var mensaje string
		var err error

		if tipo == 0 {
			mensaje = "Alto Consumo (RAM) - roldyoran/go-client"
			err = exec.Command("docker", "run", "-d", "roldyoran/go-client").Run()
		} else if tipo == 1 {
			mensaje = "Alto Consumo (CPU) - alpine con bucle 'bc'"
			err = exec.Command("docker", "run", "-d", "alpine", "sh", "-c", "while true; do echo '2^20' | bc > /dev/null; sleep 2; done").Run()
		} else {
			mensaje = "Bajo Consumo - alpine sleep 240"
			err = exec.Command("docker", "run", "-d", "alpine", "sleep", "240").Run()
		}

		if err != nil {
			log.Printf("Error al crear contenedor del cronjob: %v", err)
			continue
		}

		rdb.RPush(ctx, "logs:spawn", fmt.Sprintf("Contenedor %d/5 creado: %s", i+1, mensaje))
	}
}

func revisarLogsDeCronjob() {
	for {
		mensaje, err := rdb.LPop(ctx, "logs:spawn").Result()
		if err != nil {
			break
		}
		fmt.Printf("[NOTIFICACIÓN CRONJOB] %s\n", mensaje)
	}
}



func conectarValkey() {
	rdb = redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	for i := 0; i < 10; i++ {
		if err := rdb.Ping(ctx).Err(); err == nil {
			fmt.Println("Conexión con Valkey establecida.")
			return
		}
		time.Sleep(2 * time.Second)
	}
	log.Fatal("No se pudo conectar a Valkey.")
}

func configurarCronjob() {
	ejecutable, err := os.Executable()
	if err != nil {
		log.Printf("No se pudo obtener ruta del ejecutable: %v", err)
		return
	}

	lineaCron := fmt.Sprintf("*/2 * * * * %s spawn", ejecutable)

	comando := fmt.Sprintf(`(crontab -l 2>/dev/null | grep -v "%s"; echo "%s") | crontab -`, ejecutable, lineaCron)
	if err := exec.Command("bash", "-c", comando).Run(); err != nil {
		log.Printf("Error al configurar cronjob: %v", err)
	}
}

func configurarLimpiezaAlSalir() {
	canalSenales := make(chan os.Signal, 1)
	signal.Notify(canalSenales, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-canalSenales
		fmt.Println("\nCleanup detectado. Limpiando...")

		ejecutable, _ := os.Executable()
		comando := fmt.Sprintf(`crontab -l 2>/dev/null | grep -v "%s" | crontab -`, ejecutable)
		exec.Command("bash", "-c", comando).Run()

		exec.Command("sudo", "rmmod", "continfo_pr2_so1_202102140").Run()
		os.Exit(0)
	}()
}