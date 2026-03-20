#include <linux/init.h>
#include <linux/kernel.h>
#include <linux/module.h>
#include <linux/proc_fs.h>
#include <linux/seq_file.h>
#include <linux/mm.h>
#include <linux/mm_types.h>
#include <linux/sched/signal.h>
#include <linux/sched.h>
#include <linux/sysinfo.h>
#include <linux/version.h>
#include <linux/string.h>

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Javier Monterroso");
MODULE_DESCRIPTION("Modulo de kernel para telemetria de procesos y memoria");
MODULE_VERSION("1.0");

#define NOMBRE_ARCHIVO_PROC "continfo_pr2_so1_202102140"
#define TAMANIO_MAXIMO_LINEA_COMANDO 256

static struct proc_dir_entry *entrada_proc;

static unsigned long obtener_porcentaje_cpu_aproximado(struct task_struct *tarea_proceso)
{
    unsigned long jiffies_totales_sistema;
    unsigned long jiffies_proceso;
    unsigned long porcentaje_cpu = 0;

    jiffies_totales_sistema = jiffies;
    jiffies_proceso = tarea_proceso->utime + tarea_proceso->stime;

    if (jiffies_totales_sistema > 0) {
        porcentaje_cpu = (jiffies_proceso * 100UL) / jiffies_totales_sistema;
    }

    return porcentaje_cpu;
}

/*
 * Intenta obtener la linea de comando real del proceso.
 * Si no se puede, usa el nombre corto del proceso como respaldo.
 */
static void obtener_linea_comando_o_nombre(struct task_struct *tarea_proceso,
                                           char *buffer_salida,
                                           size_t tamanio_buffer)
{
    struct mm_struct *estructura_memoria;
    unsigned long direccion_inicio_argumentos;
    unsigned long direccion_fin_argumentos;
    unsigned long longitud_argumentos;
    int bytes_leidos;
    size_t i;
    char nombre_respaldo[TASK_COMM_LEN];

    if (buffer_salida == NULL || tamanio_buffer == 0) {
        return;
    }

    memset(buffer_salida, 0, tamanio_buffer);

    /*
     * Respaldo: nombre corto del proceso
     */
    get_task_comm(nombre_respaldo, tarea_proceso);
    strscpy(buffer_salida, nombre_respaldo, tamanio_buffer);

    estructura_memoria = get_task_mm(tarea_proceso);
    if (estructura_memoria == NULL) {
        return;
    }

    direccion_inicio_argumentos = estructura_memoria->arg_start;
    direccion_fin_argumentos = estructura_memoria->arg_end;

    if (direccion_fin_argumentos <= direccion_inicio_argumentos) {
        mmput(estructura_memoria);
        return;
    }

    longitud_argumentos = direccion_fin_argumentos - direccion_inicio_argumentos;

    if (longitud_argumentos >= tamanio_buffer) {
        longitud_argumentos = tamanio_buffer - 1;
    }

    /*
     * Lee la zona de memoria donde vive cmdline
     */
    bytes_leidos = access_process_vm(
        tarea_proceso,
        direccion_inicio_argumentos,
        buffer_salida,
        longitud_argumentos,
        0
    );

    mmput(estructura_memoria);

    if (bytes_leidos <= 0) {
        /*
         * Si falla, se conserva el nombre corto ya puesto
         */
        return;
    }

    if (bytes_leidos >= tamanio_buffer) {
        bytes_leidos = tamanio_buffer - 1;
    }

    buffer_salida[bytes_leidos] = '\0';

    /*
     * cmdline suele venir separado por '\0'
     * Lo convertimos a espacios para verlo legible
     */
    for (i = 0; i < bytes_leidos; i++) {
        if (buffer_salida[i] == '\0') {
            buffer_salida[i] = ' ';
        }
    }
}

static int escribir_informacion_en_proc(struct seq_file *archivo_salida, void *datos)
{
    struct sysinfo informacion_memoria;
    struct task_struct *tarea_actual;

    unsigned long ram_total_kb;
    unsigned long ram_libre_kb;
    unsigned long ram_usada_kb;

    int cantidad_procesos = 0;

    printk(KERN_INFO "[continfo] =====================================\n");
    printk(KERN_INFO "[continfo] Entrando a escribir_informacion_en_proc()\n");

    /*
     * Obtener informacion global de memoria RAM
     */
    si_meminfo(&informacion_memoria);

    ram_total_kb = (informacion_memoria.totalram * informacion_memoria.mem_unit) / 1024;
    ram_libre_kb = (informacion_memoria.freeram * informacion_memoria.mem_unit) / 1024;
    ram_usada_kb = ram_total_kb - ram_libre_kb;

    printk(KERN_INFO "[continfo] RAM total: %lu KB\n", ram_total_kb);
    printk(KERN_INFO "[continfo] RAM libre: %lu KB\n", ram_libre_kb);
    printk(KERN_INFO "[continfo] RAM usada: %lu KB\n", ram_usada_kb);

    seq_printf(archivo_salida, "MEMORIA\n");
    seq_printf(archivo_salida, "RAM_TOTAL_KB=%lu\n", ram_total_kb);
    seq_printf(archivo_salida, "RAM_LIBRE_KB=%lu\n", ram_libre_kb);
    seq_printf(archivo_salida, "RAM_USADA_KB=%lu\n", ram_usada_kb);
    seq_printf(archivo_salida, "LISTA_PROCESOS_INICIO\n");

    for_each_process(tarea_actual) {
        struct mm_struct *estructura_memoria = NULL;

        unsigned long memoria_virtual_kb = 0;
        unsigned long memoria_residente_kb = 0;
        unsigned long porcentaje_memoria = 0;
        unsigned long porcentaje_cpu = 0;

        char nombre_proceso[TASK_COMM_LEN];
        char linea_comando[TAMANIO_MAXIMO_LINEA_COMANDO];

        get_task_comm(nombre_proceso, tarea_actual);

        obtener_linea_comando_o_nombre(
            tarea_actual,
            linea_comando,
            sizeof(linea_comando)
        );

        estructura_memoria = get_task_mm(tarea_actual);

        if (estructura_memoria != NULL) {

            memoria_virtual_kb = (estructura_memoria->total_vm * PAGE_SIZE) / 1024;

            memoria_residente_kb = (get_mm_rss(estructura_memoria) * PAGE_SIZE) / 1024;

            if (ram_total_kb > 0) {
                porcentaje_memoria = (memoria_residente_kb * 100UL) / ram_total_kb;
            }

            mmput(estructura_memoria);
        }

        porcentaje_cpu = obtener_porcentaje_cpu_aproximado(tarea_actual);

        seq_printf(
            archivo_salida,
            "PID=%d|NOMBRE=%s|COMANDO=%s|VSZ_KB=%lu|RSS_KB=%lu|MEMORIA_PORCENTAJE=%lu|CPU_PORCENTAJE=%lu\n",
            tarea_actual->pid,
            nombre_proceso,
            linea_comando,
            memoria_virtual_kb,
            memoria_residente_kb,
            porcentaje_memoria,
            porcentaje_cpu
        );

        printk(
            KERN_INFO "[continfo] PID=%d | NOMBRE=%s | CMD=%s | VSZ=%lu KB | RSS=%lu KB | MEM=%lu%% | CPU=%lu%%\n",
            tarea_actual->pid,
            nombre_proceso,
            linea_comando,
            memoria_virtual_kb,
            memoria_residente_kb,
            porcentaje_memoria,
            porcentaje_cpu
        );

        cantidad_procesos++;
    }

    seq_printf(archivo_salida, "LISTA_PROCESOS_FIN\n");
    seq_printf(archivo_salida, "TOTAL_PROCESOS=%d\n", cantidad_procesos);

    printk(KERN_INFO "[continfo] Total de procesos recorridos: %d\n", cantidad_procesos);
    printk(KERN_INFO "[continfo] Saliendo de escribir_informacion_en_proc()\n");
    printk(KERN_INFO "[continfo] =====================================\n");

    return 0;
}

static int abrir_archivo_proc(struct inode *inodo, struct file *archivo)
{
    printk(KERN_INFO "[continfo] Se abrio /proc/%s\n", NOMBRE_ARCHIVO_PROC);
    return single_open(archivo, escribir_informacion_en_proc, NULL);
}

#if LINUX_VERSION_CODE >= KERNEL_VERSION(5,6,0)
static const struct proc_ops operaciones_proc = {
    .proc_open    = abrir_archivo_proc,
    .proc_read    = seq_read,
    .proc_lseek   = seq_lseek,
    .proc_release = single_release,
};
#else
static const struct file_operations operaciones_proc = {
    .open    = abrir_archivo_proc,
    .read    = seq_read,
    .llseek  = seq_lseek,
    .release = single_release,
};
#endif

static int __init iniciar_modulo(void)
{
    printk(KERN_INFO "[continfo] =====================================\n");
    printk(KERN_INFO "[continfo] Iniciando modulo de telemetria\n");
    printk(KERN_INFO "[continfo] Creando archivo /proc/%s\n", NOMBRE_ARCHIVO_PROC);

    entrada_proc = proc_create(NOMBRE_ARCHIVO_PROC, 0, NULL, &operaciones_proc);

    if (entrada_proc == NULL) {
        printk(KERN_ERR "[continfo] ERROR: no se pudo crear /proc/%s\n", NOMBRE_ARCHIVO_PROC);
        return -ENOMEM;
    }

    printk(KERN_INFO "[continfo] Modulo cargado correctamente\n");
    printk(KERN_INFO "[continfo] =====================================\n");
    return 0;
}

static void __exit salir_modulo(void)
{
    printk(KERN_INFO "[continfo] =====================================\n");
    printk(KERN_INFO "[continfo] Descargando modulo\n");

    if (entrada_proc != NULL) {
        remove_proc_entry(NOMBRE_ARCHIVO_PROC, NULL);
        printk(KERN_INFO "[continfo] Se elimino /proc/%s\n", NOMBRE_ARCHIVO_PROC);
    }

    printk(KERN_INFO "[continfo] Modulo descargado correctamente\n");
    printk(KERN_INFO "[continfo] =====================================\n");
}


/*
Prueba
*/
module_init(iniciar_modulo);
module_exit(salir_modulo);