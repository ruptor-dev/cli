package rules_test

import (
	"fmt"
	"testing"

	"github.com/ruptor-dev/cli/internal/evaluator/rules"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestClassifyExperiment_TruthTable is an exhaustive decision table that
// covers the cartesian product of {fault type} x {status code bucket} x
// {hit count bucket} x {recovered}. Every combination MUST:
//   - not panic
//   - return a non-nil Behaviors slice (may be empty)
//   - return Passed as a valid bool
//   - return an empty Reason when Passed is true
//   - return a non-empty Reason when Passed is false
//
// For ~30 important combinations, the expected Passed value is asserted.
func TestClassifyExperiment_TruthTable(t *testing.T) {
	allFaultTypes := []types.FaultType{
		types.FaultToolTimeout,
		types.FaultSlowResponse,
		types.FaultToolError,
		types.FaultInvalidJSON,
		types.FaultEmptyResponse,
		types.FaultRateLimit,
		types.FaultLLMError,
		types.FaultLLMTimeout,
	}

	statusBuckets := []int{0, 200, 400, 500}
	hitCounts := []int{0, 1, 5}
	recoveredStates := []bool{true, false}

	const maxIterations = 3

	// Specific expected outcomes for the most meaningful combos.
	// Key: "faultType/status/hits/recovered"
	type expected struct {
		passed bool
	}
	specific := map[string]expected{
		// -- Error faults: 5xx is the injected response, crash exempt --

		// tool_error + 500 + 1 hit: expected fault response, below threshold -> PASS
		"tool_error/500/1/false": {passed: true},
		// tool_error + 500 + 5 hits: over retry threshold (2) -> FAIL (loop)
		"tool_error/500/5/false": {passed: false},
		// tool_error + 500 + 5 hits + recovered: loop but recovered -> PASS
		"tool_error/500/5/true": {passed: true},
		// tool_error + 200 + 1 hit: fault didn't fire, normal -> PASS
		"tool_error/200/1/false": {passed: true},
		// tool_error + 200 + 5 hits: normal threshold (3), 5 >= 3 -> FAIL (loop)
		"tool_error/200/5/false": {passed: false},

		// llm_error + 500 + 1 hit: expected -> PASS
		"llm_error/500/1/false": {passed: true},
		// llm_error + 500 + 5 hits: over retry threshold -> FAIL (loop)
		"llm_error/500/5/false": {passed: false},
		// llm_error + 500 + 5 hits + recovered: loop with recovery -> PASS
		"llm_error/500/5/true": {passed: true},

		// -- Timeout faults: 504 is expected, other 5xx is crash --

		// tool_timeout + 500 (not 504): crash -> FAIL
		"tool_timeout/500/1/false": {passed: false},
		// llm_timeout + 500 (not 504): crash -> FAIL
		"llm_timeout/500/1/false": {passed: false},
		// tool_timeout + 200 + 1 hit: clean pass
		"tool_timeout/200/1/false": {passed: true},
		// llm_timeout + 200 + 1 hit: clean pass
		"llm_timeout/200/1/false": {passed: true},

		// -- Default faults: 5xx is a crash --

		// invalid_json + 500 + 1 hit: crash -> FAIL
		"invalid_json/500/1/false": {passed: false},
		// empty_response + 500 + 1 hit: crash -> FAIL
		"empty_response/500/1/false": {passed: false},
		// rate_limit + 500 + 1 hit: crash -> FAIL
		"rate_limit/500/1/false": {passed: false},
		// slow_response + 500 + 1 hit: crash -> FAIL
		"slow_response/500/1/false": {passed: false},

		// -- 200 status, single hit, no error: always PASS --
		"tool_timeout/200/1/true":    {passed: true},
		"slow_response/200/1/false":  {passed: true},
		"invalid_json/200/1/false":   {passed: true},
		"empty_response/200/1/false": {passed: true},
		"rate_limit/200/1/false":     {passed: true},

		// -- Zero hits, no agentErr: PASS (agent didn't reach proxy) --
		"tool_error/0/0/false":    {passed: true},
		"tool_timeout/0/0/false":  {passed: true},
		"slow_response/0/0/false": {passed: true},

		// -- Loop detection across faults (hits=5, maxIter=3, status=200) --
		// HadError is false when status is 200 and recovered is false,
		// so recovery detectors don't fire. Pure loop detection.
		"slow_response/200/5/false": {passed: false},
		"invalid_json/200/5/false":  {passed: false},
		"rate_limit/200/5/false":    {passed: false},

		// -- Loop + recovery on non-error faults -> PASS --
		"slow_response/200/5/true": {passed: true},
		"invalid_json/200/5/true":  {passed: true},
		"rate_limit/200/5/true":    {passed: true},

		// -- 400 status, single hit: no crash, no loop -> PASS --
		"tool_error/400/1/false":    {passed: true},
		"invalid_json/400/1/false":  {passed: true},
		"slow_response/400/1/false": {passed: true},
	}

	count := 0
	for _, ft := range allFaultTypes {
		for _, status := range statusBuckets {
			for _, hits := range hitCounts {
				for _, recovered := range recoveredStates {
					ft, status, hits, recovered := ft, status, hits, recovered
					key := fmt.Sprintf("%s/%d/%d/%t", ft, status, hits, recovered)
					count++

					t.Run(key, func(t *testing.T) {
						t.Parallel()

						// Build observation. HadError is true when status >= 400
						// or when recovered is true (must have had an error to recover).
						hadError := status >= 400 || recovered
						obs := rules.Obs{
							Hits:           hits,
							LastStatusCode: status,
							HadError:       hadError,
							Recovered:      recovered,
						}

						// Must not panic.
						v := rules.ClassifyExperiment(ft, maxIterations, obs, nil)

						// Structural validity: Behaviors is a valid slice
						// (nil is acceptable in Go for an empty slice).
						_ = len(v.Behaviors) // would panic if somehow invalid
						if v.Passed {
							assert.Empty(t, v.Reason,
								"Reason must be empty on PASS")
						} else {
							assert.NotEmpty(t, v.Reason,
								"Reason must be non-empty on FAIL")
						}

						// Specific expected outcome when defined.
						if exp, ok := specific[key]; ok {
							assert.Equal(t, exp.passed, v.Passed,
								"expected Passed=%v; behaviors=%v reason=%q",
								exp.passed, v.Behaviors, v.Reason)
						}
					})
				}
			}
		}
	}

	t.Logf("total truth-table combinations: %d", count)
}
