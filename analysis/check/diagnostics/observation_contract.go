package diagnostics

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// RemainingObservationContract owns the full result closure still needed by
// diagnostic families that have not yet crossed the stage-8 boundary. It is
// deliberately not a fallback: callers enumerate it beside each migrated
// family's exact contract.
func RemainingObservationContract() transformer.ObservationContract {
	return transformer.FullResultV1ObservationContract(transformer.ObservationConsumerDiagnosticRuleFamily)
}

// DiscriminatedUnionObservationContract declares the only retained analysis
// surfaces read while proving discriminated-union exhaustiveness. Syntax and
// declaration metadata remain prepared-body inputs and are not observations.
func DiscriminatedUnionObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerDiagnosticDiscriminatedUnion,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	)
}

// ObservationContracts is the canonical diagnostic demand inventory. Keep the
// remaining full closure explicit until each legacy family has its own commit.
func ObservationContracts() []transformer.ObservationContract {
	return []transformer.ObservationContract{
		RemainingObservationContract(),
		DiscriminatedUnionObservationContract(),
	}
}
