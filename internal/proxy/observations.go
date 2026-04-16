package proxy

// Observation accumulates per-test signals the proxy collects while the
// agent exercises a path. The evaluator consumes these at shutdown to
// build a ReliabilityReport.
type Observation struct {
	// TestID mirrors the configured test case ID.
	TestID string
	// Hits counts how many requests matched this test — whether the
	// fault fired or the request passed through.
	Hits int
	// FaultsInjected counts how many of those Hits had a fault applied.
	FaultsInjected int
	// LastStatusCode is the HTTP status code returned on the last
	// matching request. 504 for tool_timeout, 429 for rate_limit, etc.
	LastStatusCode int
	// HadError is true if at least one matching request resulted in a
	// fault response (HTTP >= 400 or timeout).
	HadError bool
}

// Recovered reports whether the observation indicates the agent
// recovered from an injected fault: at least one fault fired, a
// follow-up hit was observed on the same tool path, and the most
// recent response was successful (2xx/3xx). The evaluator consumes
// this as the recovery signal for BehaviorRecoverySuccess.
func (o Observation) Recovered() bool {
	return o.HadError &&
		o.Hits > o.FaultsInjected &&
		o.LastStatusCode >= 200 && o.LastStatusCode < 400
}

// Observations returns a snapshot of per-test observations. Safe to call
// while the proxy is serving; the returned map is a copy.
func (p *Proxy) Observations() map[string]Observation {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make(map[string]Observation, len(p.obs))
	for id, o := range p.obs {
		out[id] = *o
	}
	return out
}

// ObservationSink is the contract for recording per-test proxy
// observations. Both HTTP and MCP handlers write through it; the Proxy
// is the sole implementation.
type ObservationSink interface {
	RecordFault(testID string, statusCode int)
	RecordPassthrough(testID string, statusCode int)
}

// RecordFault updates the observation for a fault-injected request. A
// call to RecordFault is by definition an error — the proxy fired a
// fault on the matched test, regardless of whether the transport-level
// status code reflects it (MCP fault responses use HTTP 200 and carry
// the error in the JSON-RPC envelope).
func (p *Proxy) RecordFault(testID string, statusCode int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	o := p.ensureObs(testID)
	o.Hits++
	o.FaultsInjected++
	o.LastStatusCode = statusCode
	o.HadError = true
}

// RecordPassthrough updates the observation for a matched-but-not-fired
// request that was proxied through. We track only the hit count and
// status code so the evaluator sees whether the path was exercised.
func (p *Proxy) RecordPassthrough(testID string, statusCode int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	o := p.ensureObs(testID)
	o.Hits++
	o.LastStatusCode = statusCode
}

func (p *Proxy) ensureObs(testID string) *Observation {
	if p.obs == nil {
		p.obs = make(map[string]*Observation)
	}
	o, ok := p.obs[testID]
	if !ok {
		o = &Observation{TestID: testID}
		p.obs[testID] = o
	}
	return o
}
