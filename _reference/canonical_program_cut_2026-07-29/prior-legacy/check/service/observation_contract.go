package service

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// observationContract is owned by semantic service and IDE query projection.
// Semantic projection walks call, edge, and path read models to construct
// query records, so its closure is every retained class except normal returns.
func observationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerServiceIDEQueries,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEdgeReachability,
		transformer.ObservationClassEntryExitState,
		transformer.ObservationClassNodeOutput,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	)
}
