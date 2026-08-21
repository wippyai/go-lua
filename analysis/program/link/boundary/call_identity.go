package boundary

import (
	"github.com/wippyai/go-lua/analysis/program"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// boundaryCallIdentitiesAt is the narrow pre-Artifact join required while
// Boundary seals its mounted semantic value directory. It delegates to
// Program's own canonical Call identity join rather than retaining a
// Boundary-local identity table.
func boundaryCallIdentitiesAt(input *program.Program, index int) (programschema.CallIdentitySet, bool) {
	return input.CallIdentityAt(index)
}
