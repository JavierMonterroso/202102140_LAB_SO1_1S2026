package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rutaProc           = "/proc/continfo_pr2_so1_202102140"
	intervaloLectura   = 20 * time.Second
	nombreScriptCron   = "crear_contenedores.sh"
	nombreScriptModulo = "cargar_modulo.sh"
)

type Proceso struct {
	PID               int
	Nombre            string
	Comando           string
	VSZKB             uint64
	RSSKB             uint64
	MemoriaPorcentaje uint64
	CPUPorcentaje     uint64
}

type DatosProc struct {
	RAMTotalKB    uint64
	RAMLibreKB    uint64
	RAMUsadaKB    uint64
	TotalProcesos int
	Procesos      []Proceso
}

var (
	ctx context.Context
	rdb *redis.Client
)

func main() {
	ctx = context.Background()

	log.Println("========================================")
	log.Println("Iniciando daemon del Proyecto 2")
	log.Println("========================================")

	conectarValkey()

	err := ejecutarScriptCargaModulo()
	if err != nil {
		log.Printf("ERROR cargando modulo: %v", err)
	} else {
		log.Println("Modulo cargado correctamente")
	}

	err = crearCronjob()
	if err != nil {
		log.Printf("ERROR creando cronjob: %v", err)
	} else {
		log.Println("Cronjob creado correctamente")
	}

	log.Println("Entrando al loop principal...")
	ticker := time.NewTicker(intervaloLectura)
	defer ticker.Stop()

	for {
		log.Println("--------------------------------------------------")
		log.Println("Nueva iteracion:", time.Now().Format("2006-01-02 15:04:05"))

		datos, err := leerArchivoProc(rutaProc)
		if err != nil {
			log.Printf("ERROR leyendo %s: %v", rutaProc, err)
		} else {
			imprimirResumen(datos)
			err = guardarEnValkey(datos)
			if err != nil {
				log.Printf("ERROR guardando en Valkey: %v", err)
			} else {
				log.Println("Datos guardados en Valkey correctamente")
			}
		}

		<-ticker.C
	}
}

func conectarValkey() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("No se pudo conectar a Valkey: %v", err)
	}

	log.Println("Conexion a Valkey exitosa")
}

func obtenerRutaScript(nombre string) string {
	rutaEjecutable, err := os.Executable()
	if err != nil {
		log.Printf("No se pudo obtener la ruta del ejecutable: %v", err)
		return ""
	}

	directorioEjecutable := filepath.Dir(rutaEjecutable)
	ruta := filepath.Join(directorioEjecutable, "scripts", nombre)
	return ruta
}

func ejecutarScriptCargaModulo() error {
	rutaScript := obtenerRutaScript(nombreScriptModulo)
	log.Println("Ejecutando script de carga de modulo:", rutaScript)

	cmd := exec.Command("bash", rutaScript)
	salida, err := cmd.CombinedOutput()
	log.Println("Salida script modulo:")
	log.Println(string(salida))

	return err
}

func crearCronjob() error {
	rutaScript := obtenerRutaScript(nombreScriptCron)

	// cada 2 minutos, como pide el proyecto
	lineaCron := fmt.Sprintf("*/2 * * * * bash %s", rutaScript)

	comando := fmt.Sprintf(`(crontab -l 2>/dev/null | grep -v "%s"; echo "%s") | crontab -`, nombreScriptCron, lineaCron)

	log.Println("Creando cronjob con linea:", lineaCron)

	cmd := exec.Command("bash", "-c", comando)
	salida, err := cmd.CombinedOutput()
	log.Println("Salida creacion cronjob:")
	log.Println(string(salida))

	return err
}

func leerArchivoProc(ruta string) (*DatosProc, error) {
	archivo, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer archivo.Close()

	resultado := &DatosProc{
		Procesos: make([]Proceso, 0),
	}

	scanner := bufio.NewScanner(archivo)

	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea == "" {
			continue
		}

		switch {
		case strings.HasPrefix(linea, "RAM_TOTAL_KB="):
			resultado.RAMTotalKB = parsearUint(strings.TrimPrefix(linea, "RAM_TOTAL_KB="))

		case strings.HasPrefix(linea, "RAM_LIBRE_KB="):
			resultado.RAMLibreKB = parsearUint(strings.TrimPrefix(linea, "RAM_LIBRE_KB="))

		case strings.HasPrefix(linea, "RAM_USADA_KB="):
			resultado.RAMUsadaKB = parsearUint(strings.TrimPrefix(linea, "RAM_USADA_KB="))

		case strings.HasPrefix(linea, "TOTAL_PROCESOS="):
			resultado.TotalProcesos = int(parsearUint(strings.TrimPrefix(linea, "TOTAL_PROCESOS=")))

		case strings.HasPrefix(linea, "PID="):
			proceso, err := parsearLineaProceso(linea)
			if err != nil {
				log.Printf("No se pudo parsear linea de proceso: %v", err)
				continue
			}
			resultado.Procesos = append(resultado.Procesos, proceso)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return resultado, nil
}

func parsearLineaProceso(linea string) (Proceso, error) {
	var p Proceso

	partes := strings.Split(linea, "|")
	for _, parte := range partes {
		campo := strings.SplitN(parte, "=", 2)
		if len(campo) != 2 {
			continue
		}

		clave := campo[0]
		valor := campo[1]

		switch clave {
		case "PID":
			p.PID = int(parsearUint(valor))
		case "NOMBRE":
			p.Nombre = valor
		case "COMANDO":
			p.Comando = valor
		case "VSZ_KB":
			p.VSZKB = parsearUint(valor)
		case "RSS_KB":
			p.RSSKB = parsearUint(valor)
		case "MEMORIA_PORCENTAJE":
			p.MemoriaPorcentaje = parsearUint(valor)
		case "CPU_PORCENTAJE":
			p.CPUPorcentaje = parsearUint(valor)
		}
	}

	return p, nil
}

func parsearUint(valor string) uint64 {
	numero, err := strconv.ParseUint(strings.TrimSpace(valor), 10, 64)
	if err != nil {
		return 0
	}
	return numero
}

func imprimirResumen(datos *DatosProc) {
	log.Println("Resumen de memoria:")
	log.Printf("RAM total: %d KB", datos.RAMTotalKB)
	log.Printf("RAM libre: %d KB", datos.RAMLibreKB)
	log.Printf("RAM usada: %d KB", datos.RAMUsadaKB)
	log.Printf("Total procesos: %d", datos.TotalProcesos)
	log.Printf("Procesos parseados: %d", len(datos.Procesos))

	limite := 5
	if len(datos.Procesos) < limite {
		limite = len(datos.Procesos)
	}

	log.Println("Primeros procesos parseados:")
	for i := 0; i < limite; i++ {
		p := datos.Procesos[i]
		log.Printf("PID=%d NOMBRE=%s CMD=%s RSS=%d CPU=%d",
			p.PID, p.Nombre, p.Comando, p.RSSKB, p.CPUPorcentaje)
	}
}

func guardarEnValkey(datos *DatosProc) error {
	timestamp := time.Now().UnixMilli()

	claveMemoria := fmt.Sprintf("memoria:%d", timestamp)
	err := rdb.HSet(ctx, claveMemoria, map[string]interface{}{
		"ram_total_kb":   datos.RAMTotalKB,
		"ram_libre_kb":   datos.RAMLibreKB,
		"ram_usada_kb":   datos.RAMUsadaKB,
		"total_procesos": datos.TotalProcesos,
		"timestamp":      timestamp,
	}).Err()
	if err != nil {
		return err
	}

	rdb.Expire(ctx, claveMemoria, 30*time.Minute)

	for _, p := range datos.Procesos {
		claveProceso := fmt.Sprintf("proceso:%d:%d", p.PID, timestamp)

		err = rdb.HSet(ctx, claveProceso, map[string]interface{}{
			"pid":                 p.PID,
			"nombre":              p.Nombre,
			"comando":             p.Comando,
			"vsz_kb":              p.VSZKB,
			"rss_kb":              p.RSSKB,
			"memoria_porcentaje":  p.MemoriaPorcentaje,
			"cpu_porcentaje":      p.CPUPorcentaje,
			"timestamp":           timestamp,
		}).Err()
		if err != nil {
			return err
		}

		rdb.Expire(ctx, claveProceso, 30*time.Minute)
	}

	return nil
}