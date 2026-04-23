# Manual de Usuario – Sistema de Monitoreo de Contenedores

<br>


El sistema permite:

- Ver consumo de RAM y CPU de contenedores
- Ver el uso de memoria en el tiempo
- Ver el estado actual del sistema
- Ver cuántos contenedores han sido eliminados automáticamente

---



## Cómo iniciar el sistema

Para iniciar el sistema ejecuté:

```bash
./start.sh
```

Este comando realiza automáticamente:

- Compila el daemon
- Inicia el daemon
- Levanta Grafana y Valkey
- Carga el módulo del kernel

Luego de esto, el sistema ya queda funcionando.

---

## Cómo acceder al dashboard

Abrí el navegador y accedí a:

```text
http://localhost:3001
```

Credenciales:

```text
Usuario: admin
Contraseña: admin
```

Luego ingresé al dashboard del proyecto.

---

## Panel principal del dashboard

![\[Colocar imagen: Vista general del dashboard completo\]](img/dashboard_general.png)

En esta pantalla se puede ver todo el sistema funcionando en tiempo real.  
Cada panel muestra una métrica distinta.

---

## Top 5 contenedores por consumo de RAM

![\[Colocar imagen: Gráfica de pastel de RAM\]](img/TOP5RAM.png)

Este gráfico muestra los 5 contenedores que más memoria RAM están consumiendo.

Cada sección del gráfico representa un contenedor.

Ejemplo de lo que se ve:

- ID del contenedor
- Porcentaje de memoria que está usando

Interpretación:

- Si un contenedor ocupa un segmento más grande, significa que está usando más RAM.
- Esto ayuda a identificar rápidamente qué contenedor está consumiendo más recursos.

---

## Top 5 contenedores por consumo de CPU

![\[Colocar imagen: Gráfica de pastel de CPU\]](img/TOP5CPU.png)

Este gráfico muestra los 5 contenedores con mayor consumo de CPU.

Cada sección representa un contenedor.

Interpretación:

- Un valor más grande significa mayor uso de CPU.
- Este valor es comparativo entre contenedores.

Este panel sirve para detectar procesos que están consumiendo mucho procesamiento.

---

## Uso de RAM en el tiempo

![\[Colocar imagen: Gráfica de línea de RAM\]](img/RAMENELTIEMPO.png)

Este gráfico muestra cómo ha cambiado el uso de RAM del sistema con el tiempo.

En el eje horizontal se muestra el tiempo.  
En el eje vertical se muestra la RAM usada.

Interpretación:

- Si la línea sube, el sistema está usando más memoria.
- Si baja, se está liberando memoria.

Este panel es útil para ver el comportamiento del sistema a lo largo del tiempo.

---

## Total de RAM

![\[Colocar imagen: Panel con número grande TOTAL RAM\]](img/TOTALRAM.png)

Este panel muestra la memoria total del sistema.

Ejemplo:

```text
15.7 GB
```

Este valor normalmente no cambia drasticamente, ya que depende del hardware de la máquina.

---

## RAM libre

![\[Colocar imagen: Panel con número grande FREE RAM\]](img/FREERAM.png)

Este panel muestra cuánta memoria RAM está libre en el sistema.

Interpretación:

- Si el valor es alto, el sistema tiene suficiente memoria disponible.
- Si el valor es bajo, el sistema está bajo presión de memoria.

---

## RAM usada

![\[Colocar imagen: Panel con número grande RAM USADA\]](img/RAMUSADA.png)

Este panel muestra cuánta memoria RAM está siendo utilizada.

Este valor se calcula como:

```text
RAM total - RAM libre
```

Interpretación:

- Mientras más alto sea, más cargado está el sistema.

---

## Total de contenedores eliminados

![\[Colocar imagen: Panel con número TOTAL ELIMINADOS\]](img/TOTALCONTENEDORES.png)

Este panel muestra cuántos contenedores han sido eliminados automáticamente.

El sistema elimina contenedores cuando hay más de los permitidos.

Interpretación:

- Este número aumenta cuando el sistema regula la carga.
- Es una evidencia de que el daemon está funcionando correctamente.

---

## 12. Funcionamiento automático del sistema

El sistema trabaja de forma automática.

Cada cierto tiempo:

1. Lee información del sistema
2. Clasifica los contenedores
3. Verifica si hay demasiados contenedores
4. Elimina los que sobran
5. Crea nuevos si hacen falta

El usuario no necesita intervenir manualmente.

---

## Cómo detener el sistema

Para detener el sistema ejecuté:

```bash
./stop.sh
```

Este comando:

- Detiene el daemon
- Detiene Grafana y Valkey
- Elimina contenedores de prueba
- Mantiene los datos guardados

---


