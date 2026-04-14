# FaultForge -- Manual de Usuario

**Version:** 1.0
**Idioma:** Espanol

---

## Tabla de Contenidos

1. [Introduccion](#1-introduccion)
2. [Instalacion](#2-instalacion)
3. [Inicio Rapido](#3-inicio-rapido)
4. [Herramienta 1: FaultForge Chaos](#4-herramienta-1-faultforge-chaos)
5. [Herramienta 2: FaultForge Simulate](#5-herramienta-2-faultforge-simulate)
6. [Validacion de Configuracion](#6-validacion-de-configuracion)
7. [Variables de Entorno](#7-variables-de-entorno)
8. [Formatos de Salida](#8-formatos-de-salida)
9. [Solucion de Problemas](#9-solucion-de-problemas)
10. [Preguntas Frecuentes](#10-preguntas-frecuentes)

---

## 1. Introduccion

### Que es FaultForge

FaultForge es una herramienta de reliability testing disenada especificamente para agentes de inteligencia artificial. Inspirada en los principios de Chaos Engineering --la disciplina que consiste en inyectar fallos controlados en sistemas para descubrir debilidades antes de que ocurran en produccion--, FaultForge aplica esa misma filosofia al mundo de los agentes AI.

Un agente AI moderno depende de herramientas externas (APIs, bases de datos, servicios de busqueda) para completar sus tareas. Cuando una de esas herramientas falla, el agente deberia manejar la situacion de forma elegante: reintentar, informar al usuario, buscar alternativas. Sin embargo, muchos agentes no se prueban bajo condiciones de fallo, lo que lleva a comportamientos inesperados en produccion como loops infinitos, crashes silenciosos o respuestas incoherentes.

FaultForge resuelve este problema proporcionando dos herramientas complementarias:

1. **FaultForge Chaos** -- Un proxy HTTP que intercepta las llamadas a herramientas del agente AI e inyecta fallos controlados (timeouts, errores, respuestas malformadas) para verificar como reacciona el agente.

2. **FaultForge Simulate** -- Un simulador de conversaciones que genera usuarios virtuales con personalidades y objetivos definidos para probar la capacidad del agente de manejar interacciones reales.

### Para quien es FaultForge

FaultForge esta disenado para:

- **Desarrolladores de agentes AI** que quieren validar la robustez de sus agentes antes de ponerlos en produccion.
- **Equipos de QA** que necesitan automatizar pruebas de resiliencia en pipelines de CI/CD.
- **Ingenieros de plataforma** que construyen infraestructura para multiples agentes y necesitan garantias de calidad.
- **Cualquier persona** que trabaje con agentes AI y quiera responder a la pregunta: "Que pasa cuando las cosas salen mal?"

### Principios de diseno

- **Declarativo:** Las pruebas se definen en archivos YAML legibles y versionables.
- **No invasivo:** No requiere modificar el codigo del agente. Solo se cambia la URL base de las herramientas.
- **Extensible:** Soporta evaluacion basada en reglas y evaluacion con LLM para cobertura completa.
- **Reproducible:** Cada prueba se puede ejecutar de forma aislada y repetible.

---

## 2. Instalacion

### Prerrequisitos

Antes de instalar FaultForge, asegurate de tener los siguientes componentes:

| Componente | Version minima | Proposito |
|---|---|---|
| Go | 1.22+ | Compilacion del binario |
| Git | 2.x | Clonar el repositorio |
| Make | cualquiera | Ejecutar comandos de build |
| OpenAI API Key | -- | Requerido para LLM judge y simulaciones |

#### Verificar la version de Go

```bash
go version
# Salida esperada: go version go1.22.x linux/amd64 (o similar)
```

Si no tienes Go instalado, descargalo desde https://go.dev/dl/ y sigue las instrucciones de instalacion para tu sistema operativo.

### Compilar desde source

```bash
# 1. Clonar el repositorio
git clone https://github.com/tu-org/fault-forge.git
cd fault-forge

# 2. Compilar el binario
make build

# 3. El binario se genera en ./bin/faultforge
ls -la bin/faultforge
```

El comando `make build` compila el proyecto y coloca el binario resultante en el directorio `bin/`.

### Instalar en el PATH del sistema

Para poder ejecutar `faultforge` desde cualquier directorio:

```bash
# Opcion 1: Copiar al directorio de binarios del sistema
sudo cp bin/faultforge /usr/local/bin/

# Opcion 2: Agregar el directorio bin al PATH
export PATH="$PATH:$(pwd)/bin"

# Para hacerlo permanente, agregar al archivo de perfil del shell:
echo 'export PATH="$PATH:/ruta/completa/a/fault-forge/bin"' >> ~/.bashrc
source ~/.bashrc
```

### Verificar la instalacion

```bash
faultforge --version
# Salida esperada: faultforge version 1.0.0

faultforge --help
# Muestra la ayuda general con todos los comandos disponibles
```

Si el comando `faultforge` no se reconoce, verifica que el binario esta en tu PATH ejecutando `which faultforge` o `echo $PATH`.

---

## 3. Inicio Rapido

Esta seccion te guia para tener FaultForge funcionando en menos de 5 minutos con ambas herramientas.

### Inicio Rapido: FaultForge Chaos

**Objetivo:** Inyectar un timeout en la herramienta `/search` de tu agente y verificar que lo maneja correctamente.

#### Paso 1: Crear el archivo de configuracion

Crea un archivo llamado `chaos-quickstart.yaml`:

```yaml
agent:
  name: mi_agente
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080

proxy:
  port: 8080
  passthrough_url: https://mi-api-real.com
  request_timeout_s: 30

tests:
  - id: timeout_basico
    tool: /search
    fault: tool_timeout
    delay_ms: 30000
    probability: 1.0

evaluation:
  max_iterations: 20
  timeout_s: 60
  llm_judge: false

output:
  format: stdout
  path: ./reports/
```

#### Paso 2: Ejecutar la prueba

```bash
faultforge run chaos-quickstart.yaml
```

FaultForge hara lo siguiente automaticamente:

1. Levantara el proxy HTTP en el puerto 8080.
2. Arrancara tu agente con la variable `TOOL_BASE_URL` apuntando al proxy.
3. Cuando el agente llame a `/search`, el proxy devolvera un HTTP 504 (timeout).
4. El evaluador observara como reacciona el agente.
5. Mostrara los resultados por stdout.

#### Paso 3: Leer los resultados

La salida por stdout mostrara algo como:

```
=== FaultForge Chaos Report ===

Test: timeout_basico
  Tool:   /search
  Fault:  tool_timeout
  Result: PASS
  Detail: El agente detecto el timeout y respondio al usuario con un
          mensaje informativo sin entrar en loop.

Summary: 1/1 tests passed
```

### Inicio Rapido: FaultForge Simulate

**Objetivo:** Simular un usuario impaciente interactuando con tu agente de soporte.

#### Paso 1: Crear el archivo de configuracion

Crea un archivo llamado `simulate-quickstart.yaml`:

```yaml
agent:
  name: soporte_agente
  entrypoint: python agent.py
  base_url: http://localhost:3000
  request_timeout_s: 30

simulations:
  - id: usuario_rapido
    persona: "Usuario apurado que quiere una respuesta inmediata"
    goal: "Obtener el estado de su pedido #12345"
    max_turns: 5
    success_criteria: "El agente proporciono el estado del pedido"

evaluation:
  goal_completion: true
  turn_efficiency: true
  tone_quality: true
  llm_judge_prompt: |
    Rate the conversation quality from 1-10.

output:
  format: stdout
  path: ./reports/
```

#### Paso 2: Configurar la API Key

```bash
export OPENAI_API_KEY="sk-tu-clave-aqui"
```

La API Key es necesaria porque FaultForge Simulate usa un LLM para generar los mensajes del usuario simulado.

#### Paso 3: Ejecutar la simulacion

```bash
faultforge simulate simulate-quickstart.yaml
```

#### Paso 4: Leer los resultados

```
=== FaultForge Simulate Report ===

Simulation: usuario_rapido
  Persona:   Usuario apurado que quiere una respuesta inmediata
  Goal:      Obtener el estado de su pedido #12345
  Turns:     3/5
  Result:    PASS

  Metrics:
    Goal Completion: YES
    Turn Efficiency: 3/5 (60%)
    Tone Quality:    8/10

Summary: 1/1 simulations passed
```

---

## 4. Herramienta 1: FaultForge Chaos

### 4.1 Concepto y Arquitectura

FaultForge Chaos funciona como un proxy HTTP que se interpone entre tu agente AI y las APIs de herramientas reales. Cuando el agente realiza una llamada a una herramienta, el proxy decide si debe dejar pasar la solicitud al backend real o inyectar un fallo controlado.

El flujo completo es el siguiente:

```
AI Agent → FaultForge Proxy :8080 → Tool API real
                    |
              Fault Injector
                    |
              Evaluator (reglas + LLM judge)
                    |
            Reporte (stdout / JSON / HTML)
```

**Componentes principales:**

1. **Proxy HTTP** -- Escucha en un puerto configurable (por defecto 8080) y reenviar requests al backend real (`passthrough_url`). Es el punto de intercepcion.

2. **Fault Injector** -- Cuando una request coincide con un test definido (por `tool` path), inyecta el fallo configurado segun la `probability`. Si la probabilidad es 1.0, siempre inyecta el fallo. Si es 0.5, lo inyecta la mitad de las veces.

3. **Evaluator** -- Observa el comportamiento del agente despues de recibir el fallo. Tiene dos capas:
   - **Basada en reglas:** Detecta loops infinitos, crashes y patrones de recovery.
   - **LLM Judge:** Usa un modelo de lenguaje (OpenAI API) para evaluar cualitativamente si el agente manejo bien la situacion.

4. **Reporter** -- Genera el reporte final en el formato configurado (stdout, JSON, HTML o ambos).

### 4.2 Referencia de Configuracion

El archivo de configuracion de Chaos es un documento YAML con cuatro secciones principales: `agent`, `proxy`, `tests`, `evaluation` y `output`.

#### Seccion `agent`

Define el agente AI que se va a probar.

```yaml
agent:
  name: support_agent
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080
    OTHER_VAR: value
```

| Campo | Tipo | Requerido | Descripcion |
|---|---|---|---|
| `name` | string | Si | Identificador del agente. Se usa en reportes y logs. Debe ser unico y descriptivo. |
| `entrypoint` | string | Si | Comando para arrancar el agente. FaultForge ejecuta este comando como un subproceso. Puede ser cualquier comando valido del shell. |
| `env` | map | No | Variables de entorno que se inyectan al proceso del agente. La clave mas importante es `TOOL_BASE_URL`, que debe apuntar al proxy de FaultForge. |

**Notas importantes sobre `entrypoint`:**

- El comando se ejecuta tal cual, como si lo escribieras en la terminal.
- Asegurate de que el agente lee `TOOL_BASE_URL` desde sus variables de entorno para hacer requests al proxy en lugar de la API real.
- Si tu agente necesita dependencias (e.g., `pip install`), instaladas antes de ejecutar FaultForge.

**Ejemplos de entrypoints:**

```yaml
# Agente en Python
entrypoint: python agent.py

# Agente en Node.js
entrypoint: node agent.js

# Agente en Go
entrypoint: ./bin/my-agent

# Agente con argumentos
entrypoint: python agent.py --mode production --verbose

# Agente via script
entrypoint: bash scripts/run_agent.sh
```

#### Seccion `proxy`

Configura el proxy HTTP de FaultForge.

```yaml
proxy:
  port: 8080
  passthrough_url: https://real-tool-api.com
  request_timeout_s: 30
```

| Campo | Tipo | Requerido | Default | Descripcion |
|---|---|---|---|---|
| `port` | int | No | 8080 | Puerto en el que escucha el proxy. Debe coincidir con la URL configurada en `agent.env.TOOL_BASE_URL`. |
| `passthrough_url` | string | Si | -- | URL base del backend real de herramientas. Las requests que no coincidan con ningun test se reenvian aqui. |
| `request_timeout_s` | int | No | 30 | Timeout en segundos para las requests al backend real. Si el backend no responde en este tiempo, el proxy retorna un error. |

**Consejo:** Usa un puerto que no este en uso. Si el 8080 esta ocupado, cambialo a otro como 9090 o 3333.

#### Seccion `tests`

Define la lista de pruebas de chaos. Cada prueba especifica que herramienta interceptar, que fallo inyectar y con que parametros.

```yaml
tests:
  - id: timeout_on_search
    tool: /search
    fault: tool_timeout
    delay_ms: 30000
    probability: 1.0
```

**Campos comunes a todos los tests:**

| Campo | Tipo | Requerido | Descripcion |
|---|---|---|---|
| `id` | string | Si | Identificador unico del test. Se usa en reportes y para ejecutar tests individuales con `--test`. Debe ser descriptivo: `timeout_on_search`, `bad_json_on_lookup`, etc. |
| `tool` | string | Si | Path de la URL a interceptar. Debe coincidir exactamente con el path que el agente llama. Ejemplo: `/search`, `/lookup`, `/api/v1/query`. |
| `fault` | string | Si | Tipo de fallo a inyectar. Valores validos: `tool_timeout`, `slow_response`, `tool_error`, `invalid_json`, `empty_response`, `rate_limit`. |
| `probability` | float | No | Probabilidad de inyectar el fallo, entre 0.0 y 1.0. Default: 1.0 (siempre inyecta). Con 0.5, inyecta la mitad de las veces; las demas veces, deja pasar la request al backend real. |

**Campos especificos por tipo de fallo:** Ver la seccion 4.3 para el detalle completo de cada tipo.

#### Seccion `evaluation`

Configura como se evalua el comportamiento del agente.

```yaml
evaluation:
  max_iterations: 20
  timeout_s: 60
  llm_judge: true
  llm_judge_prompt: |
    Did the agent handle the tool failure gracefully?
    Consider:
    - Did it inform the user about the problem?
    - Did it avoid entering an infinite loop?
    - Did it attempt a reasonable recovery strategy?
```

| Campo | Tipo | Requerido | Default | Descripcion |
|---|---|---|---|---|
| `max_iterations` | int | No | 20 | Numero maximo de iteraciones que el agente puede hacer antes de que el evaluador lo marque como "loop detectado". Si el agente hace mas de N llamadas a la misma herramienta, se considera un loop infinito. |
| `timeout_s` | int | No | 60 | Tiempo maximo en segundos para la evaluacion completa de un test. Si el agente no termina en este tiempo, el test se marca como fallido por timeout. |
| `llm_judge` | bool | No | false | Si es `true`, activa la evaluacion con LLM (requiere `OPENAI_API_KEY`). El LLM analiza la transcripcion completa del comportamiento del agente y emite un veredicto. |
| `llm_judge_prompt` | string | No | (built-in) | Prompt personalizado para el LLM judge. Si no se especifica, FaultForge usa un prompt por defecto que evalua si el agente manejo el fallo de forma adecuada. |

**Sobre el LLM judge:**

El LLM judge recibe como contexto la transcripcion completa de la interaccion: que llamadas hizo el agente, que respuestas recibio (incluyendo los fallos inyectados) y como reacciono. El LLM entonces evalua si el comportamiento fue adecuado segun el prompt proporcionado.

El prompt custom es especialmente util cuando tienes criterios especificos de evaluacion. Por ejemplo:

```yaml
llm_judge_prompt: |
  Evalua si el agente de soporte cumplio estos criterios:
  1. No revelo informacion tecnica al usuario
  2. Ofrecio una alternativa cuando la herramienta fallo
  3. Mantuvo un tono profesional y empatico
  Responde PASS o FAIL con una justificacion breve.
```

#### Seccion `output`

Configura el formato y ubicacion de los reportes.

```yaml
output:
  format: both
  path: ./reports/
```

| Campo | Tipo | Requerido | Default | Descripcion |
|---|---|---|---|---|
| `format` | string | No | stdout | Formato de salida. Valores: `stdout` (solo terminal), `json` (archivo JSON), `html` (archivo HTML), `both` (stdout + JSON + HTML). |
| `path` | string | No | ./reports/ | Directorio donde se guardan los archivos de reporte. Se crea automaticamente si no existe. |

### 4.3 Tipos de Fallo

FaultForge soporta 6 tipos de fallo, cada uno disenado para simular una condicion de error diferente que puede ocurrir en produccion.

#### 4.3.1 `tool_timeout`

**Que hace:** Retorna inmediatamente un HTTP 504 Gateway Timeout sin contactar al backend real. Simula que la herramienta no responde en absoluto.

**Cuando usar:**
- Probar que el agente no se queda colgado esperando indefinidamente.
- Verificar que el agente informa al usuario cuando una herramienta no esta disponible.
- Validar que el agente tiene logica de timeout y no entra en un loop de reintentos infinito.

**Campos especificos:**

| Campo | Tipo | Default | Descripcion |
|---|---|---|---|
| `delay_ms` | int | 30000 | Tiempo en milisegundos que el fallo simula como duracion del timeout. Se reporta en los logs pero la respuesta HTTP 504 se envia inmediatamente. |

**Ejemplo de configuracion:**

```yaml
tests:
  - id: timeout_on_search
    tool: /search
    fault: tool_timeout
    delay_ms: 30000
    probability: 1.0
```

**Que esperar del agente:**
- Deberia detectar el error 504 y no reintentar indefinidamente.
- Deberia informar al usuario que la busqueda no esta disponible temporalmente.
- Deberia ofrecer alternativas si las tiene (usar cache, sugerir intentar mas tarde).

#### 4.3.2 `slow_response`

**Que hace:** Reenviar la request al backend real pero retrasa la respuesta N milisegundos antes de devolverla al agente. La respuesta del backend es la real, solo que llega tarde.

**Cuando usar:**
- Probar el comportamiento del agente cuando las herramientas responden lento pero no fallan.
- Verificar que el agente no tiene timeouts demasiado agresivos que cortan respuestas validas.
- Evaluar la experiencia del usuario cuando el agente tarda mas de lo normal.

**Campos especificos:**

| Campo | Tipo | Default | Descripcion |
|---|---|---|---|
| `delay_ms` | int | 5000 | Tiempo de retraso en milisegundos que se agrega antes de devolver la respuesta real del backend. |

**Ejemplo de configuracion:**

```yaml
tests:
  - id: slow_search
    tool: /search
    fault: slow_response
    delay_ms: 8000
    probability: 1.0
```

**Que esperar del agente:**
- Deberia esperar la respuesta si esta dentro de su timeout configurado.
- Si el retraso excede su timeout, deberia manejar el timeout de forma adecuada.
- Idealmente, deberia informar al usuario que esta procesando la solicitud si la espera es larga.

**Consejo:** Empieza con retrasos pequenos (2000-3000ms) y ve aumentando para encontrar el punto donde el agente empieza a comportarse mal.

#### 4.3.3 `tool_error`

**Que hace:** Retorna un codigo de estado HTTP 5xx configurable con un body de error personalizado. No contacta al backend real.

**Cuando usar:**
- Simular errores internos del servidor de herramientas.
- Probar que el agente distingue entre diferentes tipos de error (500, 502, 503).
- Verificar que el agente no expone detalles tecnicos del error al usuario.

**Campos especificos:**

| Campo | Tipo | Default | Descripcion |
|---|---|---|---|
| `status_code` | int | 500 | Codigo de estado HTTP a retornar. Valores comunes: 500 (Internal Server Error), 502 (Bad Gateway), 503 (Service Unavailable). |
| `body` | string | `""` | Cuerpo de la respuesta HTTP. Puede ser JSON, texto plano o cualquier formato. |

**Ejemplo de configuracion:**

```yaml
tests:
  - id: server_error_on_search
    tool: /search
    fault: tool_error
    status_code: 500
    body: '{"error": "internal server error", "request_id": "abc-123"}'
    probability: 1.0
```

**Variaciones utiles:**

```yaml
# Error 502 Bad Gateway
- id: bad_gateway
  tool: /search
  fault: tool_error
  status_code: 502
  body: '{"error": "bad gateway"}'
  probability: 1.0

# Error 503 Service Unavailable
- id: service_unavailable
  tool: /lookup
  fault: tool_error
  status_code: 503
  body: '{"error": "service temporarily unavailable", "retry_after": 30}'
  probability: 1.0
```

**Que esperar del agente:**
- Deberia reconocer el error HTTP y no tratar la respuesta como exitosa.
- No deberia exponer el mensaje de error crudo al usuario final.
- Deberia tener logica diferenciada para errores temporales (503) vs permanentes (500).

#### 4.3.4 `invalid_json`

**Que hace:** Retorna HTTP 200 (exito) con un cuerpo JSON malformado. Esto es especialmente enganoso porque el codigo de estado indica exito, pero el contenido no se puede parsear.

**Cuando usar:**
- Probar que el agente valida el formato de las respuestas, no solo el status code.
- Simular corrupcion de datos en transito o bugs en la API de herramientas.
- Verificar que el agente tiene manejo de excepciones robusto para JSON parsing.

**Campos especificos:**

| Campo | Tipo | Default | Descripcion |
|---|---|---|---|
| `payload` | string | `"{ broken"` | El contenido malformado que se envia como body de la respuesta. |

**Ejemplo de configuracion:**

```yaml
tests:
  - id: bad_json_on_lookup
    tool: /lookup
    fault: invalid_json
    payload: "{ broken json %%% "
    probability: 1.0
```

**Variaciones utiles:**

```yaml
# JSON truncado
- id: truncated_json
  tool: /search
  fault: invalid_json
  payload: '{"results": [{"id": 1, "name": "test"'
  probability: 1.0

# JSON con encoding incorrecto
- id: bad_encoding
  tool: /search
  fault: invalid_json
  payload: '{"data": "\xff\xfe invalid utf8"}'
  probability: 1.0

# HTML en lugar de JSON
- id: html_instead_of_json
  tool: /search
  fault: invalid_json
  payload: '<html><body>Error 500</body></html>'
  probability: 1.0
```

**Que esperar del agente:**
- Deberia detectar que el JSON no es valido a pesar del HTTP 200.
- No deberia crashear con una excepcion no manejada.
- Deberia reintentar o informar al usuario de forma adecuada.

#### 4.3.5 `empty_response`

**Que hace:** Retorna HTTP 200 (exito) con un cuerpo completamente vacio. Similar a `invalid_json` en que el status code engana, pero aqui no hay contenido alguno.

**Cuando usar:**
- Probar que el agente maneja respuestas vacias sin asumir que hay datos.
- Simular situaciones donde la API responde exitosamente pero sin resultados.
- Verificar que el agente no intenta acceder a propiedades de un objeto null/undefined.

**Campos especificos:**

Este tipo de fallo no tiene campos especificos adicionales.

**Ejemplo de configuracion:**

```yaml
tests:
  - id: empty_response_on_lookup
    tool: /lookup
    fault: empty_response
    probability: 1.0
```

**Que esperar del agente:**
- Deberia detectar que la respuesta esta vacia y no intentar parsearla como datos validos.
- Deberia informar al usuario que no se encontraron resultados o que hubo un problema.
- No deberia mostrar errores tecnicos como "Cannot read property X of null".

#### 4.3.6 `rate_limit`

**Que hace:** Retorna HTTP 429 Too Many Requests con un header `Retry-After` que indica cuantos segundos debe esperar el agente antes de reintentar.

**Cuando usar:**
- Probar que el agente respeta los limites de tasa de las APIs.
- Verificar que el agente implementa backoff exponencial o respeta `Retry-After`.
- Evaluar si el agente comunica al usuario que hay un retraso temporal.

**Campos especificos:**

| Campo | Tipo | Default | Descripcion |
|---|---|---|---|
| `status_code` | int | 429 | Codigo de estado HTTP. Normalmente 429, pero se puede configurar. |
| `retry_after_s` | int | 60 | Valor del header `Retry-After` en segundos. Indica al agente cuanto tiempo esperar. |

**Ejemplo de configuracion:**

```yaml
tests:
  - id: rate_limit_on_search
    tool: /search
    fault: rate_limit
    status_code: 429
    retry_after_s: 60
    probability: 1.0
```

**Variaciones utiles:**

```yaml
# Rate limit corto (para probar retry rapido)
- id: short_rate_limit
  tool: /search
  fault: rate_limit
  status_code: 429
  retry_after_s: 5
  probability: 1.0

# Rate limit largo (para probar paciencia del agente)
- id: long_rate_limit
  tool: /search
  fault: rate_limit
  status_code: 429
  retry_after_s: 300
  probability: 1.0
```

**Que esperar del agente:**
- Deberia reconocer el HTTP 429 y leer el header `Retry-After`.
- No deberia reintentar inmediatamente en un loop rapido (eso empeoraria un rate limit real).
- Deberia informar al usuario que hay una espera temporal o buscar alternativas.

### 4.4 Sistema de Evaluacion

FaultForge Chaos utiliza un sistema de evaluacion de dos capas para determinar si el agente paso o fallo cada test.

#### Capa 1: Evaluacion Basada en Reglas

Esta capa funciona automaticamente sin necesidad de configuracion adicional ni API keys. Detecta tres patrones:

**Detector de Loops:**
- Cuenta cuantas veces el agente llama a la misma herramienta consecutivamente.
- Si supera `max_iterations` (default: 20), marca el test como FAIL con razon "infinite loop detected".
- Ejemplo: si el agente llama a `/search` 25 veces seguidas despues de recibir un error, es un loop.

**Detector de Crashes:**
- Monitorea si el proceso del agente termina inesperadamente (exit code distinto de 0).
- Si el agente crashea despues de recibir un fallo, marca el test como FAIL con razon "agent crashed".
- Tambien detecta si el agente deja de producir output por un tiempo prolongado (hang).

**Detector de Recovery:**
- Observa si el agente, despues de recibir un fallo, eventualmente produce una respuesta util al usuario.
- Si el agente se recupera y responde, contribuye a un resultado PASS.

#### Capa 2: LLM Judge

Cuando `llm_judge` esta habilitado (`true`), FaultForge envia la transcripcion completa del test a la API de OpenAI y le pide que evalua el comportamiento del agente.

**Que recibe el LLM judge:**
- El fallo inyectado (tipo, parametros).
- Todas las llamadas HTTP que hizo el agente (requests y responses).
- Las respuestas que el agente genero para el usuario.
- El prompt personalizado de `llm_judge_prompt` (o el prompt por defecto).

**Requisitos:**
- La variable de entorno `OPENAI_API_KEY` debe estar configurada.
- Se necesita acceso a la API de OpenAI (conectividad a internet).

**Como combinan ambas capas:**
- Si la evaluacion basada en reglas detecta un FAIL (loop o crash), el test falla independientemente del LLM judge.
- Si las reglas pasan, y el LLM judge esta habilitado, su veredicto determina el resultado final.
- Si el LLM judge esta deshabilitado, solo se usan las reglas.

### 4.5 Ejecutar Tests

#### Comando basico

```bash
faultforge run chaos.yaml
```

Ejecuta todos los tests definidos en el archivo de configuracion.

#### Flags disponibles

```bash
# Ejecutar todos los tests
faultforge run chaos.yaml

# Ejecutar un test especifico por su ID
faultforge run chaos.yaml --test timeout_on_search

# Generar reporte HTML
faultforge run chaos.yaml --output report.html

# Combinar flags
faultforge run chaos.yaml --test timeout_on_search --output report.html
```

| Flag | Descripcion |
|---|---|
| `--test <id>` | Ejecuta unicamente el test con el ID especificado. Util para debugging y desarrollo iterativo. |
| `--output <archivo>` | Ruta del archivo de reporte de salida. La extension determina el formato (.html para HTML, .json para JSON). |

#### Flujo de ejecucion

Cuando ejecutas `faultforge run`, el proceso sigue estos pasos:

1. **Validacion:** FaultForge lee y valida el archivo YAML. Si hay errores de sintaxis o campos invalidos, muestra un error descriptivo y se detiene.
2. **Inicio del proxy:** Arranca el proxy HTTP en el puerto configurado.
3. **Inicio del agente:** Ejecuta el `entrypoint` del agente con las variables de entorno configuradas.
4. **Intercepcion:** Por cada test, espera a que el agente llame a la herramienta especificada e inyecta el fallo.
5. **Observacion:** Registra todo el comportamiento del agente post-fallo.
6. **Evaluacion:** Aplica las reglas y, si esta habilitado, consulta al LLM judge.
7. **Reporte:** Genera la salida en el formato configurado.
8. **Limpieza:** Detiene el proxy y el agente.

### 4.6 Leer Reportes

FaultForge genera reportes en tres formatos. Cada formato tiene sus ventajas:

#### Reporte stdout

Se imprime directamente en la terminal. Ideal para ejecucion rapida y CI/CD.

```
=== FaultForge Chaos Report ===

Test: timeout_on_search
  Tool:   /search
  Fault:  tool_timeout
  Result: PASS
  Detail: Agent detected timeout and provided fallback response.

Test: bad_json_on_lookup
  Tool:   /lookup
  Fault:  invalid_json
  Result: FAIL
  Detail: Agent crashed with unhandled JSON parse exception.

Summary: 1/2 tests passed
```

#### Reporte JSON

Archivo estructurado para procesamiento automatizado e integracion con otras herramientas.

```json
{
  "agent": "support_agent",
  "timestamp": "2026-03-27T10:30:00Z",
  "tests": [
    {
      "id": "timeout_on_search",
      "tool": "/search",
      "fault": "tool_timeout",
      "result": "PASS",
      "detail": "Agent detected timeout and provided fallback response.",
      "duration_ms": 1250,
      "iterations": 2,
      "llm_judge": {
        "verdict": "PASS",
        "reasoning": "The agent correctly identified the tool failure..."
      }
    },
    {
      "id": "bad_json_on_lookup",
      "tool": "/lookup",
      "fault": "invalid_json",
      "result": "FAIL",
      "detail": "Agent crashed with unhandled JSON parse exception.",
      "duration_ms": 340,
      "iterations": 1,
      "llm_judge": null
    }
  ],
  "summary": {
    "total": 2,
    "passed": 1,
    "failed": 1
  }
}
```

#### Reporte HTML

Archivo visual interactivo que se puede abrir en un navegador. Incluye:

- Resumen visual con indicadores de color (verde para PASS, rojo para FAIL).
- Detalle de cada test con timeline de llamadas HTTP.
- Transcripcion completa de la interaccion agente-proxy.
- Veredicto del LLM judge con su razonamiento (si aplica).

Para generar un reporte HTML:

```bash
faultforge run chaos.yaml --output report.html
```

O configurarlo en el YAML:

```yaml
output:
  format: html
  path: ./reports/
```

### 4.7 Tutorial: Testeando un Agente Python

Este tutorial te guia paso a paso para testear un agente Python simple con FaultForge Chaos.

#### Paso 1: Crear un agente de ejemplo

Crea un archivo `agent.py`:

```python
import os
import requests

TOOL_BASE_URL = os.environ.get("TOOL_BASE_URL", "https://api.example.com")

def search(query):
    """Llama a la herramienta de busqueda."""
    try:
        response = requests.get(
            f"{TOOL_BASE_URL}/search",
            params={"q": query},
            timeout=10
        )
        response.raise_for_status()
        return response.json()
    except requests.exceptions.Timeout:
        return {"error": "La busqueda tardo demasiado. Intenta de nuevo."}
    except requests.exceptions.HTTPError as e:
        if e.response.status_code == 429:
            return {"error": "Demasiadas solicitudes. Espera un momento."}
        return {"error": "Error al buscar. Intenta mas tarde."}
    except Exception:
        return {"error": "Ocurrio un problema inesperado."}

def main():
    query = "clima en Madrid"
    result = search(query)
    print(f"Resultado: {result}")

if __name__ == "__main__":
    main()
```

**Puntos clave:**
- El agente lee `TOOL_BASE_URL` del entorno, lo que permite redirigirlo al proxy.
- Tiene manejo basico de errores para timeouts, HTTP errors y excepciones generales.

#### Paso 2: Crear la configuracion de chaos

Crea un archivo `chaos-tutorial.yaml`:

```yaml
agent:
  name: tutorial_agent
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080

proxy:
  port: 8080
  passthrough_url: https://api.example.com
  request_timeout_s: 30

tests:
  - id: timeout_on_search
    tool: /search
    fault: tool_timeout
    delay_ms: 30000
    probability: 1.0

  - id: bad_json_on_search
    tool: /search
    fault: invalid_json
    payload: "{ not valid json %%% "
    probability: 1.0

  - id: error_500_on_search
    tool: /search
    fault: tool_error
    status_code: 500
    body: '{"error": "internal server error"}'
    probability: 1.0

  - id: rate_limit_on_search
    tool: /search
    fault: rate_limit
    status_code: 429
    retry_after_s: 60
    probability: 1.0

  - id: empty_on_search
    tool: /search
    fault: empty_response
    probability: 1.0

  - id: slow_on_search
    tool: /search
    fault: slow_response
    delay_ms: 8000
    probability: 1.0

evaluation:
  max_iterations: 20
  timeout_s: 60
  llm_judge: false

output:
  format: both
  path: ./reports/
```

#### Paso 3: Ejecutar las pruebas

```bash
faultforge run chaos-tutorial.yaml
```

#### Paso 4: Analizar los resultados

Revisa la salida en la terminal y los reportes en `./reports/`. Busca:

- Tests que fallaron: Indica que el agente no maneja ese tipo de error.
- Loops detectados: Indica que el agente reintenta sin limite.
- Crashes: Indica excepciones no manejadas.

#### Paso 5: Mejorar el agente

Basandote en los resultados, mejora el manejo de errores del agente. Por ejemplo, si el test `bad_json_on_search` fallo, agrega manejo de `json.JSONDecodeError` en el agente.

#### Paso 6: Re-ejecutar y validar

```bash
faultforge run chaos-tutorial.yaml
```

Repite el ciclo hasta que todos los tests pasen.

---

## 5. Herramienta 2: FaultForge Simulate

### 5.1 Concepto y Arquitectura

FaultForge Simulate adopta un enfoque diferente al de Chaos: en lugar de inyectar fallos en las herramientas, simula usuarios reales interactuando con tu agente. Usa un LLM para generar mensajes de usuario con personalidades, objetivos y comportamientos definidos.

El flujo es el siguiente:

```
UserSimulator (LLM) --> AI Agent --> Evaluator --> ConversationReport
```

**Componentes principales:**

1. **UserSimulator** -- Un LLM (via OpenAI API) que actua como un usuario simulado. Recibe una persona (personalidad), un objetivo (goal) y genera mensajes naturales como si fuera un usuario real.

2. **AI Agent** -- Tu agente bajo prueba. Recibe los mensajes del usuario simulado y responde normalmente.

3. **Evaluator** -- Al finalizar la conversacion, evalua multiples metricas: si se cumplio el objetivo, cuantos turnos fueron necesarios, la calidad del tono, etc.

4. **ConversationReport** -- Genera un reporte con la transcripcion completa, metricas y veredicto.

**Por que simular usuarios:**

- Los tests unitarios no capturan la complejidad de conversaciones reales.
- Los usuarios reales son impredecibles: cambian de opinion, se frustran, no entienden.
- Simular usuarios permite probar cientos de escenarios sin necesitar beta testers.
- Permite encontrar edge cases que no se descubren con pruebas manuales.

### 5.2 Referencia de Configuracion

El archivo de configuracion de Simulate tiene cuatro secciones: `agent`, `simulations`, `evaluation` y `output`.

#### Seccion `agent`

```yaml
agent:
  name: support_agent
  entrypoint: python agent.py
  base_url: http://localhost:3000
  request_timeout_s: 30
```

| Campo | Tipo | Requerido | Default | Descripcion |
|---|---|---|---|---|
| `name` | string | Si | -- | Nombre identificador del agente. |
| `entrypoint` | string | Si | -- | Comando para arrancar el agente. |
| `base_url` | string | Si | -- | URL base donde el agente expone su interfaz de conversacion. FaultForge envia mensajes a esta URL. |
| `request_timeout_s` | int | No | 30 | Timeout en segundos para cada mensaje enviado al agente. |

**Nota:** A diferencia de Chaos, aqui no hay proxy. FaultForge Simulate envia mensajes directamente a tu agente en `base_url`.

#### Seccion `simulations`

Define la lista de simulaciones de usuario. Cada simulacion describe un usuario virtual con su personalidad y objetivo.

```yaml
simulations:
  - id: usuario_impaciente
    persona: "Usuario frustrado que quiere resolver su problema en menos de 3 mensajes"
    goal: "Cancelar su suscripcion"
    max_turns: 10
    success_criteria: "El agente completo la cancelacion"
```

| Campo | Tipo | Requerido | Default | Descripcion |
|---|---|---|---|---|
| `id` | string | Si | -- | Identificador unico de la simulacion. Se usa en reportes y para ejecutar simulaciones individuales. |
| `persona` | string | Si | -- | Descripcion de la personalidad del usuario simulado. El LLM usara esta descripcion para generar mensajes con el tono, nivel de conocimiento y actitud correspondientes. |
| `goal` | string | Si | -- | Objetivo que el usuario simulado intenta lograr. Define que quiere conseguir el usuario en la conversacion. |
| `max_turns` | int | No | 10 | Numero maximo de turnos (intercambios de mensajes) en la conversacion. La simulacion termina cuando se alcanza este limite o cuando el evaluador determina que el objetivo se cumplio. |
| `success_criteria` | string | Si | -- | Descripcion textual de que constituye exito. El LLM judge usa este criterio para determinar si la conversacion fue exitosa. |

#### Seccion `evaluation`

```yaml
evaluation:
  goal_completion: true
  turn_efficiency: true
  tone_quality: true
  llm_judge_prompt: |
    Rate the conversation quality from 1-10.
    Consider:
    - Was the user's goal achieved?
    - How many turns did it take?
    - Was the agent's tone appropriate for the user's emotional state?
```

| Campo | Tipo | Requerido | Default | Descripcion |
|---|---|---|---|---|
| `goal_completion` | bool | No | true | Evalua si el objetivo del usuario se cumplio al final de la conversacion. |
| `turn_efficiency` | bool | No | true | Evalua cuantos turnos fueron necesarios respecto al maximo. Menos turnos para lograr el objetivo = mejor eficiencia. |
| `tone_quality` | bool | No | true | Evalua si el tono del agente fue apropiado para la personalidad y estado emocional del usuario simulado. |
| `llm_judge_prompt` | string | No | (built-in) | Prompt personalizado para la evaluacion con LLM. Permite definir criterios especificos de calidad. |

#### Seccion `output`

Identica a la seccion `output` de Chaos:

```yaml
output:
  format: both
  path: ./reports/
```

### 5.3 Disenar Personas

Las personas son el componente mas importante de FaultForge Simulate. Una persona bien disenada genera conversaciones realistas y reveladoras.

#### Principios para buenas personas

1. **Se especifico con el nivel de conocimiento:**

```yaml
# Malo - demasiado vago
persona: "Un usuario normal"

# Bueno - nivel de conocimiento claro
persona: "Un usuario de 65 anos que nunca ha usado una aplicacion movil y no entiende terminos tecnicos"
```

2. **Define el estado emocional:**

```yaml
# Malo - sin contexto emocional
persona: "Un usuario que quiere cancelar"

# Bueno - emocion y motivacion claras
persona: "Un usuario furioso porque le cobraron dos veces y quiere cancelar inmediatamente. No tiene paciencia para procesos largos"
```

3. **Incluye restricciones de comportamiento:**

```yaml
# Bueno - comportamiento predecible y desafiante
persona: "Un usuario que responde siempre con mensajes de una sola linea, no da detalles a menos que se le pregunte directamente, y se frustra si tiene que repetir informacion"
```

4. **Simula usuarios del mundo real:**

```yaml
# Usuario que cambia de opinion
persona: "Un usuario indeciso que empieza queriendo cancelar pero considera quedarse si le ofrecen un descuento"

# Usuario que no entiende
persona: "Un usuario que confunde constantemente el plan Premium con el plan Enterprise y se confunde con los precios"

# Usuario multilingue
persona: "Un usuario que mezcla espanol e ingles en sus mensajes y a veces usa terminos en ingles para conceptos tecnicos"
```

#### Biblioteca de personas comunes

Aqui hay ejemplos de personas utiles para diferentes escenarios:

```yaml
# Soporte tecnico
- persona: "Administrador de sistemas experimentado que espera respuestas tecnicas precisas y no tolera explicaciones simplificadas"
- persona: "Usuario no tecnico que describe problemas de forma vaga ('no funciona', 'esta roto') y necesita guia paso a paso"

# Ventas / Contratacion
- persona: "Gerente de compras que compara multiples proveedores y hace preguntas detalladas sobre precios y SLAs"
- persona: "Emprendedor con presupuesto limitado que busca el plan mas barato pero necesita funcionalidades avanzadas"

# Retencion
- persona: "Cliente leal de 5 anos que esta considerando irse a la competencia por un problema reciente"
- persona: "Usuario que quiere cancelar pero realmente solo quiere atencion y que le resuelvan un problema menor"
```

### 5.4 Definir Goals y Success Criteria

#### Goals (objetivos)

El `goal` define que quiere lograr el usuario simulado. Debe ser:

- **Concreto:** "Cancelar su suscripcion al plan Premium" en lugar de "resolver un problema".
- **Alcanzable:** El agente debe ser capaz de cumplir este objetivo (al menos en teoria).
- **Medible:** Debe ser posible determinar al final si se logro o no.

```yaml
# Buenos goals
goal: "Cancelar su suscripcion y obtener confirmacion por email"
goal: "Cambiar su plan de Basic a Premium"
goal: "Obtener un reembolso por el cobro duplicado del mes de febrero"
goal: "Configurar la autenticacion de dos factores en su cuenta"

# Goals vagos (evitar)
goal: "Obtener ayuda"
goal: "Hablar con alguien"
goal: "Resolver su problema"
```

#### Success Criteria (criterios de exito)

El `success_criteria` define como se mide si el goal se cumplio. Debe ser evaluable por un LLM al leer la transcripcion de la conversacion.

```yaml
# Buenos success criteria
success_criteria: "El agente confirmo la cancelacion y proporciono un numero de referencia"
success_criteria: "El agente proceso el cambio de plan y el usuario confirmo que vio el nuevo precio"
success_criteria: "El agente inicio el proceso de reembolso y proporciono un plazo estimado"

# Success criteria vagos (evitar)
success_criteria: "El usuario esta satisfecho"
success_criteria: "Se resolvio"
```

### 5.5 Metricas de Evaluacion

FaultForge Simulate evalua tres metricas principales:

#### Goal Completion (Cumplimiento del objetivo)

- **Que mide:** Si el objetivo del usuario simulado se logro al final de la conversacion.
- **Como funciona:** El LLM lee la transcripcion completa y compara el resultado con el `success_criteria`.
- **Resultado:** YES / NO.

#### Turn Efficiency (Eficiencia de turnos)

- **Que mide:** Cuantos turnos de conversacion fueron necesarios para lograr (o no lograr) el objetivo, respecto al maximo configurado.
- **Como funciona:** Se calcula como `turnos_usados / max_turns`. Un valor bajo indica alta eficiencia.
- **Resultado:** Porcentaje y fraccion (e.g., "3/10 (30%)").

**Interpretacion:**
- Menos del 50%: Excelente eficiencia, el agente resolvio rapido.
- 50-75%: Eficiencia aceptable.
- Mas del 75%: El agente tardo demasiado o no logro resolver en tiempo.
- 100%: Se alcanzo el limite sin resolver; probable ineficiencia o fallo.

#### Tone Quality (Calidad del tono)

- **Que mide:** Si el tono del agente fue apropiado para la situacion y el estado emocional del usuario.
- **Como funciona:** El LLM evalua si el agente fue empatico con un usuario frustrado, tecnico con un usuario avanzado, paciente con un usuario confundido, etc.
- **Resultado:** Puntuacion de 1 a 10.

**Interpretacion:**
- 9-10: Tono perfecto para la situacion.
- 7-8: Tono adecuado con areas menores de mejora.
- 5-6: Tono neutral, no necesariamente malo pero no adaptado al usuario.
- 1-4: Tono inapropiado (e.g., demasiado formal con un usuario casual, o insensible con un usuario frustrado).

### 5.6 Ejecutar Simulaciones

#### Comando basico

```bash
faultforge simulate simulate.yaml
```

Ejecuta todas las simulaciones definidas en el archivo.

#### Flags disponibles

```bash
# Ejecutar todas las simulaciones
faultforge simulate simulate.yaml

# Ejecutar una simulacion especifica por su ID
faultforge simulate simulate.yaml --sim usuario_impaciente

# Generar reporte HTML
faultforge simulate simulate.yaml --output report.html

# Combinar flags
faultforge simulate simulate.yaml --sim usuario_impaciente --output report.html
```

| Flag | Descripcion |
|---|---|
| `--sim <id>` | Ejecuta unicamente la simulacion con el ID especificado. |
| `--output <archivo>` | Ruta del archivo de reporte de salida. |

#### Flujo de ejecucion

1. **Validacion:** Lee y valida el YAML.
2. **Inicio del agente:** Ejecuta el `entrypoint`.
3. **Simulacion:** Para cada simulacion configurada:
   a. Inicializa el UserSimulator con la persona y goal.
   b. El UserSimulator genera el primer mensaje.
   c. Envia el mensaje al agente.
   d. El agente responde.
   e. El UserSimulator genera el siguiente mensaje basado en la respuesta.
   f. Repite hasta `max_turns` o hasta que el objetivo se cumpla.
4. **Evaluacion:** Evalua goal completion, turn efficiency y tone quality.
5. **Reporte:** Genera la salida.
6. **Limpieza:** Detiene el agente.

### 5.7 Leer Reportes

#### Reporte stdout

```
=== FaultForge Simulate Report ===

Simulation: usuario_impaciente
  Persona:   Usuario frustrado que quiere resolver su problema en menos de 3 mensajes
  Goal:      Cancelar su suscripcion
  Turns:     4/10
  Result:    PASS

  Metrics:
    Goal Completion: YES
    Turn Efficiency: 4/10 (40%)
    Tone Quality:    9/10

  Conversation:
    [User]  Quiero cancelar mi suscripcion YA. No me hagan perder el tiempo.
    [Agent] Entiendo su frustracion. Puedo ayudarle con la cancelacion ahora mismo.
            Necesito su numero de cuenta para proceder.
    [User]  Es el 12345.
    [Agent] Listo, he cancelado su suscripcion #12345. Recibira un email de
            confirmacion en los proximos minutos.
    [User]  Ok gracias.

Simulation: usuario_confuso
  Persona:   Usuario que no entiende bien el producto y cambia de opinion
  Goal:      Contratar un plan premium
  Turns:     10/15
  Result:    FAIL

  Metrics:
    Goal Completion: NO
    Turn Efficiency: 10/15 (67%)
    Tone Quality:    6/10

Summary: 1/2 simulations passed
```

#### Reporte JSON

```json
{
  "agent": "support_agent",
  "timestamp": "2026-03-27T14:00:00Z",
  "simulations": [
    {
      "id": "usuario_impaciente",
      "persona": "Usuario frustrado que quiere resolver su problema en menos de 3 mensajes",
      "goal": "Cancelar su suscripcion",
      "result": "PASS",
      "turns_used": 4,
      "max_turns": 10,
      "metrics": {
        "goal_completion": true,
        "turn_efficiency": 0.40,
        "tone_quality": 9
      },
      "conversation": [
        {"role": "user", "content": "Quiero cancelar mi suscripcion YA..."},
        {"role": "agent", "content": "Entiendo su frustracion..."},
        {"role": "user", "content": "Es el 12345."},
        {"role": "agent", "content": "Listo, he cancelado su suscripcion..."}
      ]
    }
  ],
  "summary": {
    "total": 2,
    "passed": 1,
    "failed": 1
  }
}
```

#### Reporte HTML

El reporte HTML de simulaciones incluye:

- Resumen visual con metricas consolidadas.
- Transcripcion completa de cada conversacion con formato de chat.
- Graficos de eficiencia por simulacion.
- Detalle del veredicto del LLM judge.

### 5.8 Tutorial: Simulando Usuarios para un Agente de Soporte

Este tutorial te guia paso a paso para simular diferentes tipos de usuarios interactuando con un agente de soporte al cliente.

#### Paso 1: Preparar el agente

Asegurate de que tu agente de soporte esta corriendo y acepta mensajes HTTP. Tu agente debe exponer un endpoint que reciba mensajes del usuario y devuelva la respuesta del agente.

Ejemplo de un agente minimo en Python con Flask:

```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route("/chat", methods=["POST"])
def chat():
    user_message = request.json.get("message", "")
    # Aqui va la logica de tu agente
    response = f"Gracias por su mensaje. Estamos procesando: {user_message}"
    return jsonify({"response": response})

if __name__ == "__main__":
    app.run(port=3000)
```

#### Paso 2: Disenar las simulaciones

Crea `simulate-tutorial.yaml`:

```yaml
agent:
  name: soporte_agente
  entrypoint: python agent.py
  base_url: http://localhost:3000
  request_timeout_s: 30

simulations:
  - id: usuario_impaciente
    persona: >
      Usuario frustrado que tuvo un cobro duplicado en su tarjeta.
      Quiere una solucion inmediata y no tiene paciencia para
      procesos largos. Responde con mensajes cortos y directos.
    goal: "Obtener un reembolso por el cobro duplicado"
    max_turns: 8
    success_criteria: >
      El agente inicio el proceso de reembolso y proporciono
      un numero de caso o referencia al usuario.

  - id: usuario_confuso
    persona: >
      Usuario mayor que no entiende la diferencia entre los planes
      Basic y Premium. Cambia de opinion frecuentemente y hace
      preguntas repetitivas. Necesita explicaciones simples.
    goal: "Contratar el plan Premium"
    max_turns: 15
    success_criteria: >
      El agente guio al usuario paso a paso hasta completar
      la contratacion del plan Premium.

  - id: usuario_tecnico
    persona: >
      Desarrollador de software que reporta un bug en la API.
      Incluye detalles tecnicos como codigos de error, endpoints
      y stack traces. Espera respuestas tecnicas precisas.
    goal: "Reportar un bug en el endpoint /api/v2/users y obtener un ticket de soporte"
    max_turns: 10
    success_criteria: >
      El agente creo un ticket de soporte con los detalles
      tecnicos proporcionados y dio un numero de ticket al usuario.

  - id: usuario_indeciso
    persona: >
      Usuario que quiere cancelar su suscripcion pero esta abierto
      a ofertas de retencion. Cambia de opinion si le ofrecen
      un descuento atractivo.
    goal: "Cancelar la suscripcion o aceptar una oferta de retencion"
    max_turns: 12
    success_criteria: >
      La conversacion termino con una resolucion clara: el usuario
      cancelo o acepto una oferta de retencion.

evaluation:
  goal_completion: true
  turn_efficiency: true
  tone_quality: true
  llm_judge_prompt: |
    Evalua la conversacion considerando:
    1. Se cumplio el objetivo del usuario?
    2. El agente adapto su tono al tipo de usuario?
    3. Las respuestas fueron claras y utiles?
    4. Se evitaron respuestas roboticas o genericas?
    Califica de 1 a 10.

output:
  format: both
  path: ./reports/
```

#### Paso 3: Configurar la API Key

```bash
export OPENAI_API_KEY="sk-tu-clave-aqui"
```

#### Paso 4: Ejecutar las simulaciones

```bash
# Ejecutar todas las simulaciones
faultforge simulate simulate-tutorial.yaml

# O ejecutar una a la vez para iterar
faultforge simulate simulate-tutorial.yaml --sim usuario_impaciente
```

#### Paso 5: Analizar resultados

Revisa los reportes en `./reports/`. Presta atencion a:

- **Goal Completion:** Si alguna simulacion no completo el goal, analiza la transcripcion para entender donde se descarrilo la conversacion.
- **Turn Efficiency:** Si una simulacion uso muchos turnos, busca mensajes donde el agente no fue claro o pidio informacion innecesaria.
- **Tone Quality:** Puntuaciones bajas indican que el agente no se adapto al tipo de usuario.

#### Paso 6: Iterar y mejorar

Basandote en los resultados:

1. Mejora los prompts o la logica de tu agente.
2. Agrega nuevas personas que representen edge cases descubiertos.
3. Re-ejecuta las simulaciones para validar mejoras.

---

## 6. Validacion de Configuracion

FaultForge incluye un comando para validar archivos de configuracion sin ejecutar las pruebas. Esto es util para detectar errores de sintaxis, campos faltantes o valores invalidos antes de iniciar una ejecucion completa.

### Comando

```bash
# Validar un archivo de configuracion de chaos
faultforge validate chaos.yaml

# Validar un archivo de configuracion de simulate
faultforge validate simulate.yaml
```

### Que valida

El comando `validate` verifica:

- **Sintaxis YAML:** Que el archivo sea YAML valido.
- **Campos requeridos:** Que todos los campos obligatorios esten presentes (`agent.name`, `agent.entrypoint`, `proxy.passthrough_url` en chaos, etc.).
- **Tipos de datos:** Que los valores tengan el tipo correcto (numeros donde se esperan numeros, strings donde se esperan strings).
- **Valores validos:** Que los tipos de fallo sean validos (`tool_timeout`, `slow_response`, etc.), que la probabilidad este entre 0.0 y 1.0, que los puertos sean validos, etc.
- **IDs unicos:** Que no haya tests o simulaciones con el mismo `id`.
- **Consistencia:** Que los campos especificos de cada tipo de fallo esten presentes (e.g., `status_code` para `tool_error`).

### Ejemplo de salida exitosa

```bash
$ faultforge validate chaos.yaml
Configuration is valid. 6 tests defined.
```

### Ejemplo de salida con errores

```bash
$ faultforge validate chaos.yaml
Validation errors:
  - tests[0].fault: "invalid_type" is not a valid fault type.
    Valid types: tool_timeout, slow_response, tool_error, invalid_json,
    empty_response, rate_limit
  - tests[2].probability: value 1.5 is out of range [0.0, 1.0]
  - tests[3]: missing required field "tool"
```

### Uso en CI/CD

Puedes usar `validate` en tu pipeline de CI/CD para verificar que los archivos de configuracion son validos antes de ejecutar las pruebas:

```yaml
# Ejemplo en GitHub Actions
steps:
  - name: Validate FaultForge configs
    run: |
      faultforge validate configs/chaos.yaml
      faultforge validate configs/simulate.yaml
```

---

## 7. Variables de Entorno

FaultForge utiliza variables de entorno para configuracion sensible y comportamiento global.

### Variables principales

| Variable | Requerido | Descripcion |
|---|---|---|
| `OPENAI_API_KEY` | Condicional | Clave de API de OpenAI. Requerida cuando se habilita `llm_judge: true` en Chaos o cuando se usa FaultForge Simulate (siempre necesita un LLM para generar mensajes del usuario simulado). |

### Configurar OPENAI_API_KEY

```bash
# Opcion 1: Exportar en la sesion actual
export OPENAI_API_KEY="sk-tu-clave-aqui"

# Opcion 2: Agregar al perfil del shell para persistencia
echo 'export OPENAI_API_KEY="sk-tu-clave-aqui"' >> ~/.bashrc
source ~/.bashrc

# Opcion 3: Usar un archivo .env (requiere que tu entorno lo soporte)
echo 'OPENAI_API_KEY=sk-tu-clave-aqui' > .env

# Opcion 4: Pasar directamente al comando
OPENAI_API_KEY="sk-tu-clave-aqui" faultforge run chaos.yaml
```

### Variables inyectadas al agente

Las variables definidas en la seccion `agent.env` del archivo de configuracion se inyectan automaticamente al proceso del agente:

```yaml
agent:
  env:
    TOOL_BASE_URL: http://localhost:8080
    LOG_LEVEL: debug
    CUSTOM_VAR: mi_valor
```

Estas variables estan disponibles dentro del proceso del agente como variables de entorno regulares del sistema operativo.

### Cuando se necesita OPENAI_API_KEY

| Escenario | OPENAI_API_KEY requerida |
|---|---|
| FaultForge Chaos con `llm_judge: false` | No |
| FaultForge Chaos con `llm_judge: true` | Si |
| FaultForge Simulate (cualquier configuracion) | Si |
| `faultforge validate` | No |

Si intentas ejecutar una operacion que requiere la API key sin tenerla configurada, FaultForge mostrara un error descriptivo:

```
Error: OPENAI_API_KEY environment variable is not set.
This is required for LLM judge evaluation.
Set it with: export OPENAI_API_KEY="sk-your-key"
```

---

## 8. Formatos de Salida

FaultForge soporta cuatro modos de salida: `stdout`, `json`, `html` y `both`.

### Configuracion

El formato se configura en la seccion `output` del YAML:

```yaml
output:
  format: stdout    # stdout | json | html | both
  path: ./reports/  # directorio para archivos json y html
```

Tambien se puede sobreescribir con el flag `--output`:

```bash
# El formato se deduce de la extension
faultforge run chaos.yaml --output report.html
faultforge run chaos.yaml --output report.json
```

### stdout

Imprime los resultados directamente en la terminal. No genera archivos.

**Ventajas:**
- Rapido y directo para desarrollo local.
- Ideal para pipelines de CI/CD donde solo necesitas saber si paso o fallo.
- No deja archivos que limpiar.

**Ejemplo:**

```
=== FaultForge Chaos Report ===

Test: timeout_on_search
  Tool:   /search
  Fault:  tool_timeout
  Result: PASS
  Detail: Agent handled timeout gracefully.

Summary: 1/1 tests passed
```

### json

Genera un archivo JSON estructurado en el directorio configurado en `path`.

**Ventajas:**
- Facil de parsear programaticamente.
- Ideal para integracion con dashboards y herramientas de monitoreo.
- Contiene todos los detalles incluyendo timestamps, duraciones y veredictos del LLM.

**Nombre del archivo:** El archivo se genera con un nombre basado en el timestamp, por ejemplo `chaos-report-2026-03-27T10-30-00.json`.

**Estructura del JSON de Chaos:**

```json
{
  "agent": "support_agent",
  "timestamp": "2026-03-27T10:30:00Z",
  "tests": [
    {
      "id": "timeout_on_search",
      "tool": "/search",
      "fault": "tool_timeout",
      "result": "PASS",
      "detail": "Agent handled timeout gracefully.",
      "duration_ms": 1250,
      "iterations": 2,
      "llm_judge": {
        "verdict": "PASS",
        "reasoning": "The agent correctly identified the tool failure..."
      }
    }
  ],
  "summary": {
    "total": 1,
    "passed": 1,
    "failed": 0
  }
}
```

**Estructura del JSON de Simulate:**

```json
{
  "agent": "support_agent",
  "timestamp": "2026-03-27T14:00:00Z",
  "simulations": [
    {
      "id": "usuario_impaciente",
      "persona": "...",
      "goal": "...",
      "result": "PASS",
      "turns_used": 4,
      "max_turns": 10,
      "metrics": {
        "goal_completion": true,
        "turn_efficiency": 0.40,
        "tone_quality": 9
      },
      "conversation": [
        {"role": "user", "content": "..."},
        {"role": "agent", "content": "..."}
      ]
    }
  ],
  "summary": {
    "total": 1,
    "passed": 1,
    "failed": 0
  }
}
```

### html

Genera un archivo HTML visual que se puede abrir en cualquier navegador.

**Ventajas:**
- Presentacion visual atractiva para compartir con equipos no tecnicos.
- Incluye graficos y colores para identificar rapidamente resultados.
- Transcripciones completas formateadas como chat.

**Nombre del archivo:** Similar al JSON, con extension `.html`.

**Contenido del reporte HTML:**

Para Chaos:
- Barra de resumen con total de tests, pasados y fallidos.
- Tarjeta por cada test con indicador de color (verde/rojo).
- Timeline de llamadas HTTP para cada test.
- Veredicto del LLM judge con razonamiento.

Para Simulate:
- Dashboard con metricas consolidadas.
- Transcripcion de cada conversacion con formato visual de chat.
- Graficos de eficiencia de turnos.
- Puntuaciones de calidad de tono.

### both

Genera stdout + JSON + HTML simultaneamente.

```yaml
output:
  format: both
  path: ./reports/
```

**Ventajas:**
- Maximo de informacion sin compromiso.
- Stdout para feedback inmediato, JSON para automatizacion, HTML para presentacion.

---

## 9. Solucion de Problemas

### Error: "port already in use"

**Mensaje:**
```
Error: listen tcp :8080: bind: address already in use
```

**Causa:** Otro proceso esta usando el puerto 8080.

**Solucion:**

```bash
# Identificar que proceso usa el puerto
lsof -i :8080

# Opcion 1: Matar el proceso
kill -9 <PID>

# Opcion 2: Cambiar el puerto en el YAML
# proxy:
#   port: 9090
# Y actualizar TOOL_BASE_URL: http://localhost:9090
```

### Error: "OPENAI_API_KEY not set"

**Mensaje:**
```
Error: OPENAI_API_KEY environment variable is not set.
```

**Causa:** Intentas usar LLM judge o FaultForge Simulate sin configurar la API key.

**Solucion:**

```bash
export OPENAI_API_KEY="sk-tu-clave-aqui"
```

Si no necesitas el LLM judge en Chaos, puedes deshabilitarlo:

```yaml
evaluation:
  llm_judge: false
```

### Error: "agent failed to start"

**Mensaje:**
```
Error: agent failed to start: exec: "python": executable file not found in $PATH
```

**Causa:** El comando del `entrypoint` no se encuentra en el PATH.

**Solucion:**

```bash
# Verificar que el comando existe
which python

# Usar ruta completa si es necesario
# entrypoint: /usr/bin/python3 agent.py

# O activar el virtual environment antes
# entrypoint: bash -c "source venv/bin/activate && python agent.py"
```

### Error: "connection refused to passthrough_url"

**Mensaje:**
```
Error: proxy: connection refused to https://real-tool-api.com
```

**Causa:** El backend real no es accesible desde la maquina donde corre FaultForge.

**Solucion:**
- Verificar que la URL es correcta.
- Verificar conectividad de red: `curl https://real-tool-api.com/health`
- Si el backend es local, asegurate de que esta corriendo.

### El agente no usa el proxy

**Sintoma:** El agente hace llamadas directamente al backend real ignorando el proxy.

**Causa:** El agente no lee `TOOL_BASE_URL` del entorno o tiene la URL hardcodeada.

**Solucion:**
- Verifica que tu agente lee la variable de entorno correcta.
- Asegurate de que la variable en `agent.env` coincide con lo que tu agente espera.
- Agrega un log en tu agente para imprimir la URL base que esta usando.

### El LLM judge siempre retorna PASS

**Sintoma:** Todos los tests pasan incluso cuando el agente se comporta mal.

**Causa:** El prompt del LLM judge es demasiado laxo o no es especifico.

**Solucion:** Haz el prompt mas estricto y especifico:

```yaml
llm_judge_prompt: |
  Evalua estrictamente si el agente manejo el fallo correctamente.
  Responde FAIL si:
  - El agente mostro un error tecnico al usuario
  - El agente no ofrecio una alternativa
  - El agente reintento mas de 3 veces
  - El agente ignoro el error y continuo como si nada
  Solo responde PASS si el agente manejo la situacion de forma
  profesional e informativa.
```

### Los tests tardan demasiado

**Sintoma:** La ejecucion de tests es extremadamente lenta.

**Posibles causas y soluciones:**

1. **Timeout de evaluacion muy alto:** Reduce `evaluation.timeout_s`.
2. **Muchos tests con slow_response:** Reduce `delay_ms` para pruebas iniciales.
3. **El agente entra en loops largos:** Reduce `evaluation.max_iterations`.
4. **Latencia del LLM judge:** Desactiva el LLM judge para tests rapidos y activalo solo cuando necesites evaluacion cualitativa.

### Error: "invalid YAML syntax"

**Mensaje:**
```
Error: yaml: line 15: found character that cannot start any token
```

**Causa:** El archivo YAML tiene errores de sintaxis.

**Solucion:**
- Verifica la indentacion (usa espacios, no tabs).
- Verifica que los strings con caracteres especiales esten entre comillas.
- Usa `faultforge validate` para obtener mensajes de error mas descriptivos.
- Usa un validador de YAML online para diagnosticar el problema.

---

## 10. Preguntas Frecuentes

### Generales

**P: Necesito modificar el codigo de mi agente para usar FaultForge?**

R: No directamente. El unico requisito es que tu agente lea la URL base de sus herramientas desde una variable de entorno (como `TOOL_BASE_URL`). Esto es una buena practica de cualquier forma. FaultForge configura esa variable automaticamente para apuntar al proxy.

**P: FaultForge funciona con agentes escritos en cualquier lenguaje?**

R: Si. FaultForge ejecuta el agente como un subproceso del sistema operativo usando el `entrypoint`. Puede ser Python, Node.js, Go, Java, Rust o cualquier otro lenguaje. Lo importante es que el agente haga requests HTTP a la URL base configurada.

**P: Puedo usar FaultForge en un pipeline de CI/CD?**

R: Si. FaultForge retorna exit code 0 si todos los tests pasan y exit code distinto de 0 si alguno falla, lo que permite usarlo como paso de validacion en CI/CD. Usa `format: json` o `format: stdout` para integracion programatica.

**P: Cuanto cuesta usar FaultForge?**

R: FaultForge en si es gratuito y open source. Los costos vienen del uso de la API de OpenAI para el LLM judge (en Chaos) y el simulador de usuarios (en Simulate). Si desactivas el LLM judge en Chaos, no hay costo alguno.

### FaultForge Chaos

**P: Que pasa con las requests que no coinciden con ningun test?**

R: Se reenvian al backend real (`passthrough_url`) sin modificacion. El proxy solo intercepta requests que coincidan con el `tool` path de algun test.

**P: Puedo tener multiples tests para la misma herramienta?**

R: Si. Puedes definir multiples tests que intercepten el mismo path. Se ejecutan independientemente uno tras otro, no simultaneamente.

**P: Que pasa si la probabilidad es menor a 1.0?**

R: El fallo se inyecta de forma probabilistica. Con `probability: 0.5`, la mitad de las llamadas recibiran el fallo y la otra mitad pasaran al backend real. Esto es util para simular fallos intermitentes, que son los mas dificiles de diagnosticar en produccion.

**P: Puedo combinar FaultForge Chaos con FaultForge Simulate?**

R: Actualmente se ejecutan como herramientas separadas. Sin embargo, puedes configurar el agente bajo test para que use el proxy de Chaos mientras ejecutas una simulacion, obteniendo lo mejor de ambos mundos: usuarios simulados enfrentando un agente cuyas herramientas fallan.

### FaultForge Simulate

**P: Que modelo de LLM usa el simulador de usuarios?**

R: Usa la API de OpenAI. El modelo especifico depende de la configuracion de tu cuenta de OpenAI.

**P: La conversacion puede terminar antes de llegar a max_turns?**

R: Si. Si el evaluador determina que el objetivo se cumplio antes de alcanzar el maximo de turnos, la conversacion puede terminar anticipadamente.

**P: Puedo ver la conversacion completa en los reportes?**

R: Si. Tanto el reporte JSON como el HTML incluyen la transcripcion completa de la conversacion turno por turno.

**P: Como controlo cuanto gasta el simulador en la API de OpenAI?**

R: Controla el gasto mediante `max_turns` (menos turnos = menos llamadas API) y el numero de simulaciones. Una simulacion con 10 turnos usa aproximadamente 20 llamadas a la API (10 para el usuario simulado + 10 evaluaciones).

---

*FaultForge -- Probando la resiliencia de tus agentes AI antes de que la realidad lo haga.*
