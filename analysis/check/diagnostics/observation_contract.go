package diagnostics

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

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

// LifecycleResourceObservationContract retains the exit obligation state, the
// reachable call outcomes that explain it, and the boundary states used to
// resolve lifecycle resources. It does not read ordinary node outputs, edges,
// path-value projections, or summary return slots.
func LifecycleResourceObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerDiagnosticLifecycleResource,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEntryExitState,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	)
}

// NilSafetyPresenceObservationContract retains the flow witnesses used for
// nilability, presence, reachability, and mutation-safety diagnostics. It
// deliberately excludes normal-return summaries and generic node outputs.
func NilSafetyPresenceObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerDiagnosticNilSafetyPresence,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEdgeReachability,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
		transformer.ObservationClassPathValue,
	)
}

// TypeAssignmentObservationContract retains the boundary values, return
// slots, and call facts used to prove assignment and callable compatibility.
// It does not retain generic node-output observations.
func TypeAssignmentObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerDiagnosticTypeAssignment,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEntryExitState,
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
