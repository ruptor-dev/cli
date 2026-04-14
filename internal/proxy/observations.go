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

// recordFault updates the observation for a fault-injected request.
func (p *Proxy) recordFault(testID string, statusCode int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	o := p.ensureObs(testID)
	o.Hits++
	o.FaultsInjected++
	o.LastStatusCode = statusCode
	if statusCode >= 400 || statusCode == 0 {
		// statusCode == 0 represents "no status written" (e.g. timeout
		// fault that closes the socket or returns 504 without body).
		o.HadError = true
	}
}

// recordPassthrough updates the observation for a matched-but-not-fired
// request that was proxied through. We track only the hit count and
// status code so the evaluator sees whether the path was exercised.
func (p *Proxy) recordPassthrough(testID string, statusCode int) {
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
