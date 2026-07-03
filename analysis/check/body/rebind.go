package body

// RebindBoundaryProviders installs the call-boundary providers derived from
// config onto an already solved result and clears lazy readmodel caches that may
// have observed the previous providers. Fixed-point materialization uses this
// when it reuses a solved transfer whose summary dependencies are unchanged but
// whose live summary reader/provider closures must point at the current pass.
func RebindBoundaryProviders(result *Result, prepared *Static, config SolveConfig) *Result {
	if result == nil || prepared == nil {
		return result
	}
	typeValues := prepared.solveTypeValues(config)
	signatureArgumentType := prepared.signatureArgumentTypeProvider(config, typeValues)
	result.callOutcome = prepared.callOutcomeProvider(config, typeValues, signatureArgumentType)
	result.signatureArg = signatureArgumentType
	result.typeValues = typeValues
	result.queries.reset()
	return result
}
