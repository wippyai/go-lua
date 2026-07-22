package service

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// observationContract is owned by semantic service and IDE query projection.
func observationContract() transformer.ObservationContract {
	return transformer.FullResultV1ObservationContract(transformer.ObservationConsumerServiceIDEQueries)
}
