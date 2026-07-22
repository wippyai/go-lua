package exportmanifest

import "github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"

// ObservationContract is the immutable full-result-v1 demand owned by module
// export construction.
func ObservationContract() transformer.ObservationContract {
	return transformer.FullResultV1ObservationContract(transformer.ObservationConsumerExportCode)
}
