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

// Recovered returns true when the observation indicates the agent
// retried after a fault and eventually received a successful response.
func (o Observation) Recovered() bool {
	return o.HadError && o.Hits > o.FaultsInjected &&
		o.LastStatusCode >= 200 && o.LastStatusCode < 400
}

// ResetObservation clears the accumulated stats for a single test ID.
// The orchestrator calls this at the start of each experiment so the
// evaluator sees only the current experiment's activity rather than
// the whole run's — necessary because the proxy's first-match test
// dispatch routes every request on a given tool to the first test
// that declared that tool, regardless of which experiment is active.
func (p *Proxy) ResetObservation(testID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.obs, testID)
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

// RecordFault updates the observation for a fault-injected request.
//
// A call to RecordFault is by definition an error — the proxy fired a
// fault on the matched test, regardless of the transport-level status
// code. For HTTP mode this is a tautology (every injected fault returns
// >= 400 or 0); for MCP mode the old status-code guard would have
// mis-classified the JSON-RPC-over-HTTP-200 pattern, because MCP fault
// responses ride on HTTP 200 and carry the error in the JSON-RPC
// envelope. We therefore set HadError unconditionally here and let the
// caller (HTTP handler or MCP handler via the types.ObservationSink
// interface) decide when a fault actually fired.
//
// This method satisfies the types.ObservationSink contract — the MCP
// handler invokes it through that interface to keep HTTP and MCP
// observations in a single map.
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
//
// This method satisfies the types.ObservationSink contract — see
// RecordFault for context.
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
