package signature

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// OutputAuthority is the mounted denominator witness required for the output
// relation declared by the signature's ordered Outputs. Destination columns
// are intentionally not duplicated here.
type OutputAuthority struct {
	Denominator model.DenominatorRef
}

func (authority OutputAuthority) Available() bool {
	return authority.Denominator.Available()
}
