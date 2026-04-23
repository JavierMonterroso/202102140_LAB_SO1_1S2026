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

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Estudiante SO1 - 202102140");
#define NOMBRE_PROC "continfo_pr2_so1_202102140"

static struct proc_dir_entry *entrada_proc;

/* --- Funciones de memoria --- */
static unsigned long obtener_ram_total_mb(void) {
    struct sysinfo i; si_meminfo(&i);
    return (i.totalram * i.mem_unit) >> 20;
}

static unsigned long obtener_ram_libre_mb(void) {
    struct sysinfo i; si_meminfo(&i);
    return (i.freeram * i.mem_unit) >> 20;
}

/* --- Funciones de identificación --- */
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

/* --- Función de lectura principal --- */
static int mostrar_informacion(struct seq_file *archivo, void *v) {
    struct task_struct *tarea;
    char id_cont[65], cmdline[64];
    unsigned long ram_total_mb = obtener_ram_total_mb();
    unsigned long ram_libre_mb = obtener_ram_libre_mb();
    unsigned long total_ram_kb = ram_total_mb << 10;

    // FORMATO SISTEMA: SISTEMA|Total|Libre|Uso
    seq_printf(archivo, "SISTEMA|%lu|%lu|%lu\n",
               ram_total_mb,
               ram_libre_mb,
               ram_total_mb - ram_libre_mb);

    rcu_read_lock();
    for_each_process(tarea) {
        struct mm_struct *mm;

        if (!tarea->mm || !tarea->parent) continue;

        /* --- FILTRO DE INFRAESTRUCTURA --- */
        /* Si el proceso es Grafana o Valkey, lo ignoramos para proteger la arquitectura */
        if (strncmp(tarea->comm, "grafana", 7) == 0 ||
            strncmp(tarea->comm, "valkey-server", 13) == 0 ||
            strncmp(tarea->comm, "redis-server", 12) == 0) {
            continue;
        }

        // Filtro de contenedores (Solo hijos de containerd-shim)
        if (strncmp(tarea->parent->comm, "containerd-shim", 15) != 0) continue;

        obtener_id_contenedor(tarea, id_cont, sizeof(id_cont));
        obtener_cmdline(tarea, cmdline, sizeof(cmdline));

        mm = get_task_mm(tarea);
        if (mm) {
            unsigned long vsz_kb = (mm->total_vm << PAGE_SHIFT) >> 10;
            unsigned long rss_kb = (get_mm_rss(mm) << PAGE_SHIFT) >> 10;

            // Escalado a 10000 para simular decimales (XX.XX)
            unsigned long p_mem = (total_ram_kb > 0) ? (rss_kb * 10000) / total_ram_kb : 0;

            u64 total_time = tarea->utime + tarea->stime;
            u64 process_age_jiffies = nsecs_to_jiffies(ktime_get_ns() - tarea->start_time);
            unsigned long p_cpu = (process_age_jiffies > 0) ? (total_time * 10000ULL) / process_age_jiffies : 0;

            // FORMATO PROCESO: PROCESO|PID|Nombre|ID|Cmd|VSZ|RSS|%MEM|%CPU
            seq_printf(archivo, "PROCESO|%d|%s|%s|%s|%lu|%lu|%lu.%02lu|%lu.%02lu\n",
                       tarea->pid,
                       tarea->comm,
                       id_cont,
                       cmdline,
                       vsz_kb,
                       rss_kb,
                       p_mem / 100, p_mem % 100,
                       p_cpu / 100, p_cpu % 100);

            mmput(mm);
        }
    }

    rcu_read_unlock();
    return 0;
}

static int abrir_archivo_proc(struct inode *n, struct file *f) {
    return single_open(f, mostrar_informacion, NULL);
}

static const struct proc_ops ops = {
    .proc_open = abrir_archivo_proc,
    .proc_read = seq_read,
    .proc_lseek = seq_lseek,
    .proc_release = single_release,
};

static int __init inicio(void) {
    entrada_proc = proc_create(NOMBRE_PROC, 0444, NULL, &ops);
    return entrada_proc ? 0 : -ENOMEM;
}

static void __exit fin(void) {
    proc_remove(entrada_proc);
}

module_init(inicio);
module_exit(fin);