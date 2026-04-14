# SKILL: Ruptor Testing Patterns

Use this skill when writing tests for the CLI.

## Quick reference

| Test type | Location | Build tag | Command | External I/O |
|-----------|----------|-----------|---------|-------------|
| Unit | *_test.go | none | make test | None |
| Integration | *_inttest.go | none | make test-int | httptest server only |
| E2E | *_e2e_test.go | e2e | make test-e2e | Real LLM, real API |

## Unit test template (fault type)

```go
func TestTimeoutFault_Inject_AppliesDelayAtFullProbability(t *testing.T) {
    tests := []struct {
        name        string
        delayMs     int
        probability float64
        wantFault   bool
    }{
        {"full probability applies fault", 100, 1.0, true},
        {"zero probability passes through", 100, 0.0, false},
        {"half probability is probabilistic", 100, 0.5, nil}, // tested separately
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            fault := &TimeoutFault{}
            req := &types.ProxyRequest{
                Config: types.FaultConfig{
                    DelayMs:     tt.delayMs,
                    Probability: tt.probability,
                },
            }

            start := time.Now()
            resp, err := fault.Inject(context.Background(), req, passThroughNext)

            if tt.wantFault {
                assert.Greater(t, time.Since(start), time.Duration(tt.delayMs)*time.Millisecond)
            }
            // etc.
        })
    }
}
```

## Integration test template (proxy)

```go
func TestProxy_Integration_TimeoutFault(t *testing.T) {
    // Real HTTP server simulating the tool
    toolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(10 * time.Millisecond) // fast response
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
    }))
    defer toolServer.Close()

    // Proxy configured to inject timeout
    proxy := NewProxy(ProxyConfig{
        Port:          0, // auto-assign
        PassthroughURL: toolServer.URL,
        Tests: []TestConfig{{
            Fault:       "tool_timeout",
            DelayMs:     5000,
            Probability: 1.0,
        }},
    })

    // Start proxy
    addr, err := proxy.Start(context.Background())
    require.NoError(t, err)
    defer proxy.Stop()

    // Call through proxy — should timeout
    client := &http.Client{Timeout: 1 * time.Second}
    _, err = client.Get("http://" + addr + "/search")
    assert.ErrorIs(t, err, context.DeadlineExceeded)
}
```

## Naming convention

```
Test{Component}_{Scenario}_{ExpectedBehavior}

TestTimeoutFault_Inject_AppliesDelayAtFullProbability
TestProxy_Integration_TimeoutFault_ReturnsDeadlineExceeded  
TestEvaluator_LLMJudge_ReturnsLowScoreOnEmptyResponse
TestSimulator_MaxTurnsRespected_WhenAgentNeverCompletes
TestReportingClient_RetryWithJitter_RecoversAfterTransientFailure
```

## Mocking the LLM client

```go
type mockLLMClient struct {
    response string
    err      error
    calls    int
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
    m.calls++
    return m.response, m.err
}
```

## Test fixtures (testdata/)

Persona files: testdata/personas/frustrated_user.yaml
Chaos configs: testdata/chaos/timeout_example.yaml
Simulate configs: testdata/simulate/support_agent_example.yaml
Expected reports: testdata/reports/expected_*.json (for report format tests)

## What NOT to mock

Never mock the DB in tests (platform only — use testcontainers).
Never mock the HTTP server in integration tests — use httptest.NewServer.
Never mock time.Now() — use clock injection if time matters.
