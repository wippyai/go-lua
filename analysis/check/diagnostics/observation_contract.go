package diagnostics

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// DiscriminatedUnionObservationContract declares the only retained analysis
// surfaces read while proving discriminated-union exhaustiveness, including
// call evidence and path values reached through optional/discriminant checks.
// Syntax and declaration metadata remain prepared-body inputs.
func DiscriminatedUnionObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerDiagnosticDiscriminatedUnion,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	)
}

// LifecycleResourceObservationContract retains the exit obligation state, the
// reachable call outcomes that explain it, and the boundary node outputs used
// to resolve lifecycle resources. It does not read edge or summary-return
// projections.
func LifecycleResourceObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerDiagnosticLifecycleResource,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEntryExitState,
		transformer.ObservationClassNodeOutput,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	)
}

// NilSafetyPresenceObservationContract retains the flow witnesses used for
// nilability, presence, reachability, mutation-safety diagnostics, and their
// boundary-node outputs. It deliberately excludes normal-return summaries.
func NilSafetyPresenceObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerDiagnosticNilSafetyPresence,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEdgeReachability,
		transformer.ObservationClassNodeOutput,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
		transformer.ObservationClassPathValue,
	)
}

// TypeAssignmentObservationContract retains the boundary values, return
// slots, call facts, and node outputs used to prove assignment and callable
// compatibility.
func TypeAssignmentObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerDiagnosticTypeAssignment,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEntryExitState,
		transformer.ObservationClassNodeOutput,
		transformer.ObservationClassNormalReturn,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
		transformer.ObservationClassPathValue,
	)
}

// ObservationContracts is the canonical diagnostic demand inventory.
func ObservationContracts() []transformer.ObservationContract {
	return []transformer.ObservationContract{
		DiscriminatedUnionObservationContract(),
		LifecycleResourceObservationContract(),
		NilSafetyPresenceObservationContract(),
		TypeAssignmentObservationContract(),
	}
}
