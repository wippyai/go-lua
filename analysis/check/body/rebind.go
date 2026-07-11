package body

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// RebindBoundaryProviders installs the call-boundary providers derived from
// config onto an already solved result and clears lazy readmodel caches that may
// have observed the previous providers. Fixed-point materialization uses this
// when it reuses a solved transfer whose summary dependencies are unchanged but
// whose live summary reader/provider closures must point at the current pass.
func RebindBoundaryProviders(result *Result, prepared *Static, config SolveConfig) (*Result, error) {
	if result == nil || prepared == nil {
		return result, nil
	}
	typeValues := prepared.solveTypeValues(config)
	signatureArgumentType := prepared.signatureArgumentTypeProvider(config, typeValues)
	result.callOutcome = prepared.callOutcomeProvider(config, typeValues, signatureArgumentType)
	result.signatureArg = signatureArgumentType
	result.typeValues = typeValues
	result.queries.reset()
	// Captured outputs were validated against the prior provider set. A rebind
	// may change summary reads without rerunning the body worklist, so only the
	// deterministic seal projection is sound here.
	result.capturedNodeOutputs = nil
	result.observation = ObservationStats{}
	// Rebinding changes closures consulted by boundary projection. The cached
	// transfer solution is valid only when its tracked summary reads are valid,
	// but PublishedFacts must still be rebuilt against the current closures.
	if err := result.sealObservationsContext(config.Context); err != nil {
		return nil, errors.Join(solve.ErrCanceled, err)
	}
	return result, nil
}
