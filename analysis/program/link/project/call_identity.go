package project

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
)

// callIdentityAt is the pre-Artifact Project admission join. Project must
// recognize executable authored Calls before an Artifact exists, so it
// delegates to Program's own canonical Call identity join rather than
// retaining a Project-local equation.
func callIdentityAt(input *program.Program, index int) (identity.ContentID, bool) {
	identities, ok := input.CallIdentityAt(index)
	return identities.Call, ok
}
