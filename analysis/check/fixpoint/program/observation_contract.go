package program

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// SummaryProjectionObservationContract is owned by the summary/publication
// consumer.  Stage 6 deliberately requests the complete full-result-v1
// closure while installing the demand protocol for later scope reduction.
func SummaryProjectionObservationContract() transformer.ObservationContract {
	return transformer.FullResultV1ObservationContract(transformer.ObservationConsumerSummaryProjection)
}
