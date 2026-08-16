package body

import (
	typecovariant "github.com/wippyai/go-lua/__legacy/analysis/type/covariant"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// luaCovariantWiden preserves the body execution seam while both concrete and
// replacement execution consume the one callback-free type authority.
func luaCovariantWiden(sourceWitness, contract typ.Type, segments []segment.Segment) (typ.Type, [][]segment.Segment, bool) {
	return typecovariant.WidenRecord(sourceWitness, contract, segments)
}
