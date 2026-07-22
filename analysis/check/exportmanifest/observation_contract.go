package exportmanifest

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// ObservationContract retains the return and boundary values used to publish
// module exports, function signatures, and provided globals. Function
// signatures may transitively inspect call-outcome witnesses; export does not
// inspect generic node outputs or edge reachability.
func ObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerExportCode,
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassNormalReturn,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointState,
	)
}
