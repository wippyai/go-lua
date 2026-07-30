package program

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// SummaryProjectionObservationContract is owned by the summary/publication
// consumer. Summary-only program callers need no complete point-state
// publication; mixed consumer sets canonically widen at the transformer
// boundary.
func SummaryProjectionObservationContract() transformer.ObservationContract {
	return transformer.SummaryV1ObservationContract()
}
