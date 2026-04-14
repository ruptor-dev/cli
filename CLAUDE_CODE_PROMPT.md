# FaultForge — Prompt de arranque para Claude Code
# Distinguished Engineer Level

---

## Contexto del proyecto

Estamos construyendo **FaultForge**, una herramienta de reliability testing para AI agents
inspirada en Chaos Engineering. El código debe ser de nivel Distinguished Engineer:
production-grade desde el día uno, aunque sea un MVP.

Founder: ~12 años de experiencia en software engineering, infra, Kubernetes, DevOps,
Chaos Engineering. El código debe estar a esa altura.

---

## El problema que resuelve

Los AI agents funcionan en demos pero fallan en producción:
- Tool timeouts sin manejo correcto
- Respuestas JSON inválidas o con schema incorrecto
- Rate limits que causan loops infinitos
- Errores HTTP no manejados
- Recovery logic incorrecta o inexistente

No existe tooling sistemático para probar estos escenarios antes de producción.
FaultForge lo resuelve con un enfoque Chaos Engineering para AI agents.

---

## Dos módulos, un monorepo

### Módulo 1 — FaultForge Chaos
HTTP proxy que intercepta tool calls del agente e inyecta fallos controlados.

```
AI Agent → FaultForge Proxy :8080 → Tool API real
                    ↓
            Fault Injector (registry pattern)
                    ↓
              Evaluator (rule-based + LLM judge)
                    ↓
            ReliabilityReport (stdout + HTML)
```

### Módulo 2 — FaultForge Simulate
Agente simulador que interactúa con el agente bajo prueba evaluando
goal completion y calidad conversacional.

```
UserSimulator → AI Agent bajo prueba → Evaluator → ConversationReport
```

El simulador usa un loop manual orquestado — sin frameworks externos.
Dos llamadas LLM por turno: una para generar el mensaje del usuario simulado,
otra es el agente real respondiendo.

---

## Stack técnico

- **Go 1.22+** — proxy, CLI, evaluator, simulate, report
- **Cobra** — CLI framework
- **gopkg.in/yaml.v3** — config parsing
- **OpenAI API** — LLM judge y user simulator (llamadas HTTP directas, no SDK)
- **slog** (stdlib) — structured logging
- **testify** — assertions en tests
- **Monorepo**: `github.com/faultforge/faultforge`

---

## Estructura del repositorio

El repo ya tiene esta estructura creada. Respétala exactamente:

```
faultforge/
├── cmd/
│   └── faultforge/
│       └── main.go                    # entrypoint, wiring de dependencias
├── internal/
│   ├── proxy/
│   │   ├── proxy.go                   # Server struct, Start/Stop lifecycle
│   │   ├── handler.go                 # http.Handler, routing por path
│   │   └── faults/
│   │       ├── registry.go            # FaultRegistry: registro y lookup
│   │       ├── fault.go               # BaseFault + FaultConfig
│   │       ├── timeout.go
│   │       ├── slow_response.go
│   │       ├── tool_error.go
│   │       ├── invalid_json.go
│   │       ├── empty_response.go
│   │       └── rate_limit.go
│   ├── simulate/
│   │   ├── simulator.go               # Simulator struct, Run() method
│   │   ├── persona.go                 # PersonaPromptBuilder
│   │   └── conversation.go            # Turn, ConversationHistory (ya en pkg/types)
│   ├── evaluator/
│   │   ├── evaluator.go               # Evaluator interfaces
│   │   ├── chaos_evaluator.go
│   │   ├── simulate_evaluator.go
│   │   ├── rules/
│   │   │   ├── loop_detector.go
│   │   │   ├── crash_detector.go
│   │   │   └── recovery_detector.go
│   │   └── llmjudge/
│   │       ├── judge.go               # Judge interface
│   │       ├── openai_judge.go
│   │       └── noop_judge.go          # stub para tests
│   ├── report/
│   │   ├── renderer.go                # Renderer interface
│   │   ├── stdout_renderer.go
│   │   ├── html_renderer.go
│   │   └── json_renderer.go
│   └── config/
│       ├── chaos_config.go
│       ├── simulate_config.go
│       └── loader.go
├── pkg/
│   └── types/
│       ├── fault.go                   # FaultType constants + Fault interface
│       ├── result.go                  # TestResult, SimulationResult, Turn, ConversationHistory
│       ├── report.go                  # ReliabilityReport, ConversationReport
│       └── behavior.go                # DetectedBehavior constants
├── configs/
│   ├── chaos.example.yaml             # YA EXISTE — no modificar
│   └── simulate.example.yaml          # YA EXISTE — no modificar
├── web/
│   ├── chaos_report.html.tmpl
│   └── simulate_report.html.tmpl
├── testdata/
│   ├── chaos_valid.yaml
│   ├── chaos_invalid.yaml
│   └── simulate_valid.yaml
├── .github/workflows/ci.yml           # YA EXISTE — no modificar
├── go.mod                             # YA EXISTE — no modificar
├── Makefile                           # YA EXISTE — no modificar
├── .golangci.yml                      # YA EXISTE — no modificar
├── .gitignore                         # YA EXISTE — no modificar
└── README.md                          # YA EXISTE — no modificar
```

---

## Tipos compartidos — implementar EXACTAMENTE así en pkg/types/

```go
// pkg/types/fault.go
package types

import "net/http"

type FaultType string

const (
    FaultToolTimeout    FaultType = "tool_timeout"
    FaultSlowResponse   FaultType = "slow_response"
    FaultToolError      FaultType = "tool_error"
    FaultInvalidJSON    FaultType = "invalid_json"
    FaultEmptyResponse  FaultType = "empty_response"
    FaultRateLimit      FaultType = "rate_limit"
)

type Fault interface {
    Type() FaultType
    Inject(w http.ResponseWriter, r *http.Request) error
}
```

```go
// pkg/types/behavior.go
package types

type DetectedBehavior string

const (
    BehaviorCrash           DetectedBehavior = "crash"
    BehaviorInfiniteLoop    DetectedBehavior = "infinite_loop"
    BehaviorRecoverySuccess DetectedBehavior = "recovery_success"
    BehaviorRecoveryFailed  DetectedBehavior = "recovery_failed"
    BehaviorHallucination   DetectedBehavior = "hallucination"
    BehaviorFallbackUsed    DetectedBehavior = "fallback_used"
    BehaviorTimeout         DetectedBehavior = "timeout"
)
```

```go
// pkg/types/result.go
package types

import "time"

type TestResult struct {
    TestID            string
    FaultType         FaultType
    Tool              string
    Passed            bool
    DetectedBehaviors []DetectedBehavior
    LLMJudgeVerdict   string
    LLMJudgeReason    string
    DurationMs        int64
    Error             string
}

type SimulationResult struct {
    SimulationID string
    Persona      string
    Goal         string
    GoalReached  bool
    TurnCount    int
    MaxTurns     int
    QualityScore int
    Issues       []string
    DurationMs   int64
}

type Turn struct {
    Role      string
    Content   string
    Timestamp time.Time
}

type ConversationHistory struct {
    Turns []Turn
}

func (h *ConversationHistory) Add(role, content string) {
    h.Turns = append(h.Turns, Turn{
        Role:      role,
        Content:   content,
        Timestamp: time.Now(),
    })
}

func (h *ConversationHistory) ToOpenAIMessages() []map[string]string {
    msgs := make([]map[string]string, 0, len(h.Turns))
    for _, t := range h.Turns {
        msgs = append(msgs, map[string]string{
            "role":    t.Role,
            "content": t.Content,
        })
    }
    return msgs
}
```

```go
// pkg/types/report.go
package types

import "time"

type ReliabilityReport struct {
    AgentName  string
    RunAt      time.Time
    TotalTests int
    Passed     int
    Failed     int
    Score      int
    Results    []TestResult
}

type ConversationReport struct {
    AgentName   string
    RunAt       time.Time
    TotalSims   int
    GoalReached int
    AvgScore    float64
    Results     []SimulationResult
}
```

---

## Interfaces críticas — implementar EXACTAMENTE estas firmas

```go
// internal/proxy/faults/registry.go
type FaultConfig struct {
    Type        types.FaultType
    DelayMs     int
    StatusCode  int
    Body        string
    Payload     string
    Probability float64
    RetryAfterS int
}

type FaultFactory func(cfg FaultConfig) (types.Fault, error)

type FaultRegistry struct {
    faults map[types.FaultType]FaultFactory
}

func NewFaultRegistry() *FaultRegistry // registra los 6 faults en el constructor
func (r *FaultRegistry) Register(ft types.FaultType, factory FaultFactory)
func (r *FaultRegistry) Build(ft types.FaultType, cfg FaultConfig) (types.Fault, error)
// Build retorna error descriptivo si ft no está registrado — NUNCA switch/case en el handler
```

```go
// internal/evaluator/llmjudge/judge.go
type Judge interface {
    EvaluateChaos(ctx context.Context, prompt, agentBehavior string) (verdict, reason string, err error)
    EvaluateConversation(ctx context.Context, prompt string, history *types.ConversationHistory) (score int, issues []string, err error)
}
// NoopJudge implementa Judge retornando "SKIPPED" — usar cuando llm_judge: false o en tests
```

```go
// internal/report/renderer.go
type Renderer interface {
    RenderChaos(report *types.ReliabilityReport) error
    RenderSimulate(report *types.ConversationReport) error
}
// NewRendererFromFormat(format, path string) Renderer
// format: "stdout" | "json" | "html" | "both" (stdout + html)
```

```go
// internal/simulate/simulator.go
type LLMClient interface {
    Complete(ctx context.Context, messages []map[string]string, model string) (string, error)
}

type Simulator struct {
    llmClient  LLMClient
    httpClient *http.Client
    logger     *slog.Logger
}

func (s *Simulator) Run(ctx context.Context, sim config.Simulation) (*types.SimulationResult, error)
// loop: generateUserMessage → callAgent → goalReached → repeat
// Estado = ConversationHistory en memoria, sin persistencia
```

---

## Patrones Distinguished Engineer — obligatorios en TODO el código

### Error wrapping con contexto
```go
// NUNCA:  return err
// SIEMPRE:
return fmt.Errorf("proxy: injecting %s on %s: %w", fault.Type(), path, err)
```

### Structured logging con slog — inyectado como dependencia
```go
// Configurar una vez en main.go, pasar como dependencia. NUNCA logger global.
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

// En cada struct que necesite logging:
type Proxy struct { logger *slog.Logger }

// Loggear con campos estructurados — NUNCA fmt.Printf en código de producción:
logger.Info("fault injected", "fault_type", fault.Type(), "tool", path, "duration_ms", elapsed.Milliseconds())
```

### Context en toda operación I/O — sin excepción
```go
func (s *Simulator) Run(ctx context.Context, sim config.Simulation) (*types.SimulationResult, error)
func (j *OpenAIJudge) EvaluateChaos(ctx context.Context, ...) (string, string, error)
func (p *Proxy) Start(ctx context.Context) error
```

### Options pattern para constructores con múltiples parámetros
```go
type ProxyOption func(*Proxy)

func WithLogger(l *slog.Logger) ProxyOption  { return func(p *Proxy) { p.logger = l } }
func WithTimeout(d time.Duration) ProxyOption { return func(p *Proxy) { p.timeout = d } }

func NewProxy(cfg *config.ProxyConfig, registry *faults.FaultRegistry, opts ...ProxyOption) *Proxy
```

### Config validation con errors.Join (Go 1.20+)
```go
func (c *ChaosConfig) Validate() error {
    var errs []error
    if c.Agent.Name == "" { errs = append(errs, errors.New("agent.name is required")) }
    if c.Proxy.Port == 0  { errs = append(errs, errors.New("proxy.port is required")) }
    for i, t := range c.Tests {
        if t.Probability < 0 || t.Probability > 1 {
            errs = append(errs, fmt.Errorf("tests[%d].probability must be 0-1", i))
        }
    }
    return errors.Join(errs...)
}
```

### Graceful shutdown en main.go
```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := app.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
    logger.Error("unexpected shutdown", "error", err)
    os.Exit(1)
}
```

### Tabla de tests — obligatorio, no tests repetitivos
```go
func TestTimeoutFault_Inject(t *testing.T) {
    tests := []struct {
        name       string
        delayMs    int
        wantStatus int
    }{
        {"standard timeout", 30000, http.StatusGatewayTimeout},
        {"zero delay", 0, http.StatusGatewayTimeout},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) { /* ... */ })
    }
}
```

### Stubs en tests — no librerías de mocking externas
```go
type stubLLMClient struct {
    responses []string
    callCount int
}
func (s *stubLLMClient) Complete(_ context.Context, _ []map[string]string, _ string) (string, error) {
    resp := s.responses[s.callCount%len(s.responses)]
    s.callCount++
    return resp, nil
}
```

### TODO markers — solo dos tipos
```go
// TODO(v2): descripción de mejora futura
// FIXME: descripción de problema conocido e intencional
```

---

## Orden estricto de implementación

Después de cada paso: `go build ./...` debe pasar. `go test ./...` debe pasar.

```
Paso 1: pkg/types/           → fault.go, behavior.go, result.go, report.go
Paso 2: internal/config/     → chaos_config.go, simulate_config.go, loader.go + tests
Paso 3: internal/proxy/faults/ → registry.go, fault.go, 6 implementaciones + tests
Paso 4: internal/proxy/      → proxy.go, handler.go + tests con httptest
Paso 5: internal/evaluator/  → rules/, llmjudge/ (noop primero), evaluator.go + tests
Paso 6: internal/simulate/   → simulator.go, persona.go + tests con stubLLMClient
Paso 7: internal/report/     → renderer.go, stdout, json, html + tests
Paso 8: cmd/faultforge/      → main.go con Cobra, wiring completo de dependencias
Paso 9: web/                 → chaos_report.html.tmpl, simulate_report.html.tmpl
Paso 10: testdata/           → chaos_valid.yaml, chaos_invalid.yaml, simulate_valid.yaml
```

---

## Tests requeridos por módulo (coverage mínima 80%)

```
internal/proxy/faults/timeout_test.go         tabla de tests, httptest.ResponseRecorder
internal/proxy/faults/slow_response_test.go   verifica delay real con time.Since
internal/proxy/faults/invalid_json_test.go    verifica que el body es JSON inválido
internal/proxy/faults/registry_test.go        register, build, unknown type error
internal/config/loader_test.go                valid yaml, invalid yaml, missing required fields
internal/evaluator/rules/loop_detector_test.go   N < max → ok, N > max → BehaviorInfiniteLoop
internal/evaluator/rules/crash_detector_test.go  5xx → BehaviorCrash, 2xx → no behavior
internal/simulate/simulator_test.go           stubLLMClient, goal reached en turno N, max turns
internal/report/stdout_renderer_test.go       output a bytes.Buffer, verifica campos clave
pkg/types/result_test.go                      ConversationHistory.Add(), ToOpenAIMessages()
```

---

## CLI esperado — Cobra thin wrappers, lógica en internal/

```bash
faultforge run chaos.yaml
faultforge run chaos.yaml --output report.html
faultforge run chaos.yaml --test timeout_on_search

faultforge simulate simulate.yaml
faultforge simulate simulate.yaml --output report.html
faultforge simulate simulate.yaml --sim usuario_impaciente

faultforge validate chaos.yaml
faultforge version
```

Cada comando Cobra es un thin wrapper que:
1. Parsea flags
2. Llama `config.Load*()`
3. Construye dependencias (registry, judge, evaluator, renderer)
4. Llama al runner/simulator
5. Retorna error si algo falla — Cobra lo imprime y sale con código 1

Toda la lógica vive en `internal/`. `cmd/faultforge/main.go` es solo wiring.

---

## Principios finales

- **Distinguished Engineer no es perfeccionismo** — es claridad y mantenibilidad.
  Código que cualquier Go engineer senior entiende en 5 minutos.
- **Interfaces sobre implementaciones** en toda llamada entre capas.
- **Zero dependencias externas innecesarias** — si la stdlib lo resuelve, usarla.
- **Errores con contexto siempre**: `fmt.Errorf("component: operation: %w", err)`
- **Logging estructurado siempre** — nunca `fmt.Printf` en producción.
- **Context en toda operación I/O** — sin excepción.
- **MVP no significa código desechable** — significa scope reducido con calidad alta.
