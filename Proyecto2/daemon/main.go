package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rutaProc          = "/proc/continfo_pr2_so1_202102140"
	direccionValkey   = "localhost:6379"
	intervaloLoop     = 20 * time.Second
	identificadorCron = "CRON_PROYECTO2_SO1_202102140"
	metaBajos         = 3
	metaAltos         = 2
	ttlRegistros      = 24 * time.Hour
)

type MetricasSistema struct {
	RAMTotalKB uint64
	RAMLibreKB uint64
	RAMUsoKB   uint64
}

type Proceso struct {
	PID      int
	TGID     int
	Nombre   string
	Estado   string
	PIDPadre int
	CMDLine  string
	VSZKB    uint64
	RSSKB    uint64
	MemPct   float64
	CPUPct   float64
}

type LecturaProc struct {
	Sistema  MetricasSistema
	Procesos []Proceso
}

type TipoContenedor string

const (
	TipoBajo    TipoContenedor = "bajo"
	TipoAlto    TipoContenedor = "alto"
	TipoIgnorar TipoContenedor = "ignorar"
)

var (
	ctx    = context.Background()
	valkey *redis.Client
)

func main() {
	rutaLoadModule := filepath.Join(rutaProyecto(), "scripts", "load_module.sh")
	rutaCreateContainer := filepath.Join(rutaProyecto(), "scripts", "create_container.sh")

	if len(os.Args) > 1 && os.Args[1] == "--cronjob" {
		log.Println("CRONJOB: ejecución iniciada")
		if err := ejecutarScript(rutaCreateContainer); err != nil {
			log.Printf("CRONJOB: error al ejecutar create_container.sh: %v", err)
		} else {
			log.Println("CRONJOB: ejecución finalizada")
		}
		return
	}

	log.Println("DAEMON: iniciando Proyecto 2...")

	conectarValkey()

	if err := ejecutarScript(rutaLoadModule); err != nil {
		log.Printf("DAEMON: no se pudo cargar el módulo: %v", err)
	} else {
		log.Println("DAEMON: módulo cargado correctamente")
	}

	if err := crearCronjob(); err != nil {
		log.Printf("DAEMON: no se pudo crear el cronjob: %v", err)
	} else {
		log.Println("DAEMON: cronjob instalado correctamente")
	}

	seniales := make(chan os.Signal, 1)
	signal.Notify(seniales, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(intervaloLoop)
	defer ticker.Stop()

	log.Printf("DAEMON: loop principal iniciado, intervalo=%s", intervaloLoop)

	for {
		select {
		case <-ticker.C:
			log.Println("DAEMON: ---------------- INICIO CICLO ----------------")
			if err := ejecutarCiclo(); err != nil {
				log.Printf("DAEMON: error en ciclo principal: %v", err)
			}
			log.Println("DAEMON: ---------------- FIN CICLO -------------------")
		case s := <-seniales:
			log.Printf("DAEMON: señal recibida: %v", s)
			limpiezaFinal()
			return
		}
	}
}

func rutaProyecto() string {
	ejecutable, err := os.Executable()
	if err != nil {
		return "/home/javier/pruebas_de_estructuras/proyecto2"
	}

	dirDaemon := filepath.Dir(ejecutable)
	return filepath.Dir(dirDaemon)
}

func ejecutarCiclo() error {
	lectura, err := leerArchivoProc(rutaProc)
	if err != nil {
		return err
	}

	log.Printf("DAEMON: métricas sistema -> RAM_TOTAL=%d KB RAM_LIBRE=%d KB RAM_USO=%d KB",
		lectura.Sistema.RAMTotalKB,
		lectura.Sistema.RAMLibreKB,
		lectura.Sistema.RAMUsoKB,
	)

	log.Printf("DAEMON: total de procesos parseados desde /proc = %d", len(lectura.Procesos))

	guardarEnValkey(lectura)

	bajos, altos := clasificarProcesos(lectura.Procesos)

	log.Printf("DAEMON: clasificación actual -> bajos=%d altos=%d", len(bajos), len(altos))

	for _, p := range bajos {
		log.Printf("DAEMON: BAJO -> PID=%d NOMBRE=%s RSS=%d CPU=%.2f CMDLINE=%s",
			p.PID, p.Nombre, p.RSSKB, p.CPUPct, p.CMDLine)
	}

	for _, p := range altos {
		log.Printf("DAEMON: ALTO -> PID=%d NOMBRE=%s RSS=%d CPU=%.2f CMDLINE=%s",
			p.PID, p.Nombre, p.RSSKB, p.CPUPct, p.CMDLine)
	}

	aplicarPoliticaEliminacion(bajos, altos)

	if len(bajos) < metaBajos {
		log.Printf("DAEMON: aún faltan contenedores bajos -> actual=%d requerido=%d", len(bajos), metaBajos)
	}
	if len(altos) < metaAltos {
		log.Printf("DAEMON: aún faltan contenedores altos -> actual=%d requerido=%d", len(altos), metaAltos)
	}

	return nil
}

func conectarValkey() {
	valkey = redis.NewClient(&redis.Options{
		Addr: direccionValkey,
	})

	if _, err := valkey.Ping(ctx).Result(); err != nil {
		log.Fatalf("DAEMON: no se pudo conectar a Valkey: %v", err)
	}

	log.Println("DAEMON: conexión a Valkey exitosa")
}

func ejecutarScript(ruta string) error {
	log.Printf("DAEMON: ejecutando script %s", ruta)

	cmd := exec.Command("bash", ruta)
	salida, err := cmd.CombinedOutput()

	if len(salida) > 0 {
		log.Printf("DAEMON: salida de %s:\n%s", ruta, string(salida))
	}

	return err
}

func crearCronjob() error {
	ejecutable, err := os.Executable()
	if err != nil {
		return err
	}

	lineaCron := fmt.Sprintf("*/2 * * * * %s --cronjob #%s", ejecutable, identificadorCron)

	_ = exec.Command("bash", "-c",
		fmt.Sprintf(`crontab -l 2>/dev/null | grep -v "%s" | crontab -`, identificadorCron),
	).Run()

	cmd := exec.Command("bash", "-c",
		fmt.Sprintf(`(crontab -l 2>/dev/null; echo '%s') | crontab -`, lineaCron),
	)

	salida, err := cmd.CombinedOutput()
	if len(salida) > 0 {
		log.Printf("DAEMON: salida al crear cronjob:\n%s", string(salida))
	}
	if err != nil {
		return err
	}

	log.Printf("DAEMON: cronjob registrado -> %s", lineaCron)
	return nil
}

func eliminarCronjob() {
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf(`crontab -l 2>/dev/null | grep -v "%s" | crontab -`, identificadorCron),
	)
	salida, err := cmd.CombinedOutput()
	if len(salida) > 0 {
		log.Printf("DAEMON: salida al eliminar cronjob:\n%s", string(salida))
	}
	if err == nil {
		log.Println("DAEMON: cronjob eliminado correctamente")
	}
}

func limpiezaFinal() {
	log.Println("DAEMON: iniciando limpieza final")
	eliminarCronjob()
	log.Println("DAEMON: limpieza final completada")
}

func leerArchivoProc(ruta string) (*LecturaProc, error) {
	log.Printf("DAEMON: leyendo archivo %s", ruta)

	archivo, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer archivo.Close()

	lectura := &LecturaProc{}
	scanner := bufio.NewScanner(archivo)

	enBloqueProcesos := false

	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea == "" {
			continue
		}

		switch {
		case strings.HasPrefix(linea, "RAM_TOTAL_KB:"):
			lectura.Sistema.RAMTotalKB = parsearNumeroDespuesDeDosPuntos(linea)
		case strings.HasPrefix(linea, "RAM_LIBRE_KB:"):
			lectura.Sistema.RAMLibreKB = parsearNumeroDespuesDeDosPuntos(linea)
		case strings.HasPrefix(linea, "RAM_USO_KB:"):
			lectura.Sistema.RAMUsoKB = parsearNumeroDespuesDeDosPuntos(linea)
		case linea == "PROCESOS_RELACIONADOS_CON_CONTENEDORES":
			enBloqueProcesos = true
		case enBloqueProcesos && strings.HasPrefix(linea, "PID="):
			proceso, err := parsearLineaProceso(linea)
			if err == nil {
				lectura.Procesos = append(lectura.Procesos, proceso)
			}
		}
	}

	return lectura, scanner.Err()
}

func parsearNumeroDespuesDeDosPuntos(linea string) uint64 {
	partes := strings.SplitN(linea, ":", 2)
	if len(partes) != 2 {
		return 0
	}
	numero, _ := strconv.ParseUint(strings.TrimSpace(partes[1]), 10, 64)
	return numero
}

func parsearLineaProceso(linea string) (Proceso, error) {
	var p Proceso
	campos := dividirRespetandoComillas(linea)

	for _, campo := range campos {
		if !strings.Contains(campo, "=") {
			continue
		}

		partes := strings.SplitN(campo, "=", 2)
		clave := partes[0]
		valor := strings.Trim(partes[1], `"`)

		switch clave {
		case "PID":
			p.PID, _ = strconv.Atoi(valor)
		case "TGID":
			p.TGID, _ = strconv.Atoi(valor)
		case "NOMBRE":
			p.Nombre = valor
		case "STATE":
			p.Estado = valor
		case "PARENT_PID":
			p.PIDPadre, _ = strconv.Atoi(valor)
		case "CMDLINE":
			p.CMDLine = valor
		case "VSZ_KB":
			p.VSZKB, _ = strconv.ParseUint(valor, 10, 64)
		case "RSS_KB":
			p.RSSKB, _ = strconv.ParseUint(valor, 10, 64)
		case "MEM":
			p.MemPct = parsearPorcentaje(valor)
		case "CPU":
			p.CPUPct = parsearPorcentaje(valor)
		}
	}

	return p, nil
}

func dividirRespetandoComillas(linea string) []string {
	var resultado []string
	var actual strings.Builder
	enComillas := false

	for _, r := range linea {
		switch r {
		case '"':
			enComillas = !enComillas
			actual.WriteRune(r)
		case ' ':
			if enComillas {
				actual.WriteRune(r)
			} else if actual.Len() > 0 {
				resultado = append(resultado, actual.String())
				actual.Reset()
			}
		default:
			actual.WriteRune(r)
		}
	}

	if actual.Len() > 0 {
		resultado = append(resultado, actual.String())
	}

	return resultado
}

func parsearPorcentaje(valor string) float64 {
	valor = strings.TrimSuffix(valor, "%")
	numero, _ := strconv.ParseFloat(valor, 64)
	return numero
}

func guardarEnValkey(lectura *LecturaProc) {
	timestamp := time.Now().UnixMilli()

	log.Printf("DAEMON: guardando snapshot en Valkey con timestamp=%d", timestamp)

	claveSistema := fmt.Sprintf("so1:sistema:%d", timestamp)
	datosSistema := map[string]interface{}{
		"ram_total_kb": lectura.Sistema.RAMTotalKB,
		"ram_libre_kb": lectura.Sistema.RAMLibreKB,
		"ram_uso_kb":   lectura.Sistema.RAMUsoKB,
		"timestamp":    timestamp,
	}

	_ = valkey.HSet(ctx, claveSistema, datosSistema).Err()
	_ = valkey.Expire(ctx, claveSistema, ttlRegistros).Err()

	for _, p := range lectura.Procesos {
		tipo := tipoContenedor(p)
		if tipo == TipoIgnorar {
			continue
		}

		claveProceso := fmt.Sprintf("so1:proceso:%d:%d", p.PID, timestamp)
		datos := map[string]interface{}{
			"pid":       p.PID,
			"tgid":      p.TGID,
			"nombre":    p.Nombre,
			"estado":    p.Estado,
			"pid_padre": p.PIDPadre,
			"cmdline":   p.CMDLine,
			"vsz_kb":    p.VSZKB,
			"rss_kb":    p.RSSKB,
			"mem_pct":   p.MemPct,
			"cpu_pct":   p.CPUPct,
			"tipo":      tipo,
			"timestamp": timestamp,
		}
		_ = valkey.HSet(ctx, claveProceso, datos).Err()
		_ = valkey.Expire(ctx, claveProceso, ttlRegistros).Err()
	}
}

func clasificarProcesos(procesos []Proceso) ([]Proceso, []Proceso) {
	var bajos []Proceso
	var altos []Proceso

	for _, p := range procesos {
		tipo := tipoContenedor(p)
		switch tipo {
		case TipoBajo:
			bajos = append(bajos, p)
		case TipoAlto:
			altos = append(altos, p)
		case TipoIgnorar:
			log.Printf("DAEMON: ignorando proceso -> PID=%d NOMBRE=%s CMDLINE=%s", p.PID, p.Nombre, p.CMDLine)
		}
	}

	return bajos, altos
}

func esProcesoIgnorable(p Proceso) bool {
	nombre := strings.ToLower(p.Nombre)
	cmd := strings.ToLower(p.CMDLine)

	if strings.Contains(nombre, "grafana") || strings.Contains(cmd, "grafana") {
		return true
	}
	if strings.Contains(nombre, "valkey") || strings.Contains(cmd, "valkey") {
		return true
	}
	if strings.Contains(nombre, "valkey-server") || strings.Contains(cmd, "valkey-server") {
		return true
	}
	if strings.Contains(nombre, "redis") || strings.Contains(cmd, "redis") {
		return true
	}
	return false
}

func tipoContenedor(p Proceso) TipoContenedor {
	if esProcesoIgnorable(p) {
		return TipoIgnorar
	}

	cmd := strings.ToLower(p.CMDLine)

	if strings.Contains(cmd, "go-client") {
		return TipoAlto
	}
	if strings.Contains(cmd, "bc") || strings.Contains(cmd, "2^20") {
		return TipoAlto
	}
	if p.RSSKB >= 50000 || p.CPUPct >= 20.0 {
		return TipoAlto
	}

	return TipoBajo
}

func aplicarPoliticaEliminacion(bajos, altos []Proceso) {
	log.Println("DAEMON: aplicando política de eliminación")

	ordenarProcesosPorConsumo(bajos)
	ordenarProcesosPorConsumo(altos)

	if len(bajos) > metaBajos {
		exceso := len(bajos) - metaBajos
		log.Printf("DAEMON: sobran %d contenedores bajos, se eliminarán los más pesados", exceso)
		for i := 0; i < exceso; i++ {
			log.Printf("DAEMON: candidato eliminación BAJO -> PID=%d RSS=%d CPU=%.2f VSZ=%d CMDLINE=%s",
				bajos[i].PID, bajos[i].RSSKB, bajos[i].CPUPct, bajos[i].VSZKB, bajos[i].CMDLine)
			eliminarContenedor(bajos[i], "exceso_bajo")
		}
	} else {
		log.Println("DAEMON: no sobran contenedores bajos")
	}

	if len(altos) > metaAltos {
		exceso := len(altos) - metaAltos
		log.Printf("DAEMON: sobran %d contenedores altos, se eliminarán los más pesados", exceso)
		for i := 0; i < exceso; i++ {
			log.Printf("DAEMON: candidato eliminación ALTO -> PID=%d RSS=%d CPU=%.2f VSZ=%d CMDLINE=%s",
				altos[i].PID, altos[i].RSSKB, altos[i].CPUPct, altos[i].VSZKB, altos[i].CMDLine)
			eliminarContenedor(altos[i], "exceso_alto")
		}
	} else {
		log.Println("DAEMON: no sobran contenedores altos")
	}
}

func ordenarProcesosPorConsumo(lista []Proceso) {
	sort.Slice(lista, func(i, j int) bool {
		if lista[i].RSSKB != lista[j].RSSKB {
			return lista[i].RSSKB > lista[j].RSSKB
		}
		if lista[i].CPUPct != lista[j].CPUPct {
			return lista[i].CPUPct > lista[j].CPUPct
		}
		return lista[i].VSZKB > lista[j].VSZKB
	})
}

func obtenerNombreContenedor(p Proceso) string {
	log.Printf("DAEMON: intentando identificar contenedor para PID=%d CMDLINE=%s", p.PID, p.CMDLine)

	if strings.HasPrefix(p.CMDLine, "ID_CONTENEDOR=") {
		return strings.TrimPrefix(p.CMDLine, "ID_CONTENEDOR=")
	}

	if strings.HasPrefix(p.CMDLine, "ID_CONTENEDOR_APROX") {
		return ""
	}

	partes := strings.Fields(p.CMDLine)
	for _, parte := range partes {
		if strings.HasPrefix(parte, "low_") ||
			strings.HasPrefix(parte, "high_cpu_") ||
			strings.HasPrefix(parte, "high_ram_") {
			return parte
		}
	}

	cmd := strings.ToLower(p.CMDLine)

	if strings.Contains(cmd, "go-client") {
		return "high_ram"
	}
	if strings.Contains(cmd, "sleep") {
		return "low"
	}
	if strings.Contains(cmd, "bc") || strings.Contains(cmd, "2^20") {
		return "high_cpu"
	}

	return ""
}

func obtenerPrefijosPorTipo(p Proceso) []string {
	switch tipoContenedor(p) {
	case TipoBajo:
		return []string{"low_"}
	case TipoAlto:
		return []string{"high_cpu_", "high_ram_"}
	default:
		return nil
	}
}

func buscarContenedorPorPrefijos(prefijos []string) string {
	if len(prefijos) == 0 {
		return ""
	}

	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	salida, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("DAEMON: no se pudo listar contenedores docker: %v", err)
		return ""
	}

	lineas := strings.Split(strings.TrimSpace(string(salida)), "\n")
	for _, nombre := range lineas {
		nombre = strings.TrimSpace(nombre)
		if nombre == "" {
			continue
		}
		nombreLower := strings.ToLower(nombre)
		if strings.Contains(nombreLower, "grafana") || strings.Contains(nombreLower, "valkey") {
			continue
		}
		for _, prefijo := range prefijos {
			if strings.HasPrefix(nombre, prefijo) {
				log.Printf("DAEMON: fallback encontró contenedor %s para prefijo %s", nombre, prefijo)
				return nombre
			}
		}
	}

	return ""
}

func eliminarContenedor(p Proceso, motivo string) {
	if esProcesoIgnorable(p) {
		log.Printf("DAEMON: proceso ignorado, no se elimina -> PID=%d NOMBRE=%s", p.PID, p.Nombre)
		return
	}

	nombreContenedor := obtenerNombreContenedor(p)

	if nombreContenedor == "" {
		prefijos := obtenerPrefijosPorTipo(p)
		nombreContenedor = buscarContenedorPorPrefijos(prefijos)
	}

	if nombreContenedor == "" {
		log.Printf("DAEMON: no se pudo identificar nombre/ID de contenedor para PID=%d", p.PID)
		return
	}

	log.Printf("DAEMON: intentando eliminar contenedor=%s motivo=%s", nombreContenedor, motivo)

	cmd := exec.Command("docker", "rm", "-f", nombreContenedor)
	salida, err := cmd.CombinedOutput()
	if len(salida) > 0 {
		log.Printf("DAEMON: salida eliminación contenedor %s:\n%s", nombreContenedor, string(salida))
	}

	estado := "ok"
	if err != nil {
		estado = "error"
		log.Printf("DAEMON: no se pudo eliminar contenedor %s: %v", nombreContenedor, err)
	} else {
		log.Printf("DAEMON: contenedor eliminado correctamente -> %s motivo=%s", nombreContenedor, motivo)
	}

	claveEvento := fmt.Sprintf("so1:eliminado:%d:%d", p.PID, time.Now().UnixMilli())
	datosEvento := map[string]interface{}{
		"pid":        p.PID,
		"nombre":     p.Nombre,
		"contenedor": nombreContenedor,
		"cmdline":    p.CMDLine,
		"rss_kb":     p.RSSKB,
		"cpu_pct":    p.CPUPct,
		"vsz_kb":     p.VSZKB,
		"motivo":     motivo,
		"estado":     estado,
		"timestamp":  time.Now().UnixMilli(),
	}

	_ = valkey.HSet(ctx, claveEvento, datosEvento).Err()
	_ = valkey.Expire(ctx, claveEvento, ttlRegistros).Err()
}