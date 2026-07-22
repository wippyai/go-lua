package service

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// observationContract is owned by semantic service and IDE query projection.
func observationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerServiceIDEQueries,
		transformer.ObservationClassPointState,
	)
}
