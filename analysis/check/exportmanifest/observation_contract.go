package exportmanifest

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// ObservationContract retains the return and boundary values used to publish
// module exports, function signatures, and provided globals. Export does not
// inspect generic node outputs, edge reachability, or call-outcome witnesses.
func ObservationContract() transformer.ObservationContract {
	return transformer.ObservationClassesV1Contract(
		transformer.ObservationConsumerExportCode,
		transformer.ObservationClassNormalReturn,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointState,
	)
}
