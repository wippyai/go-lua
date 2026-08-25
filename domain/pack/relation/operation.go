package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	packdomain "github.com/wippyai/go-lua/domain/pack"
)

// PackSourceOperation is domain/pack's own source seed.
type PackSourceOperation struct{}

// Available reports whether the operation carries its owner mathematics. The
// seed is a package function over a source that carries its own owner, so
// there is no derived state to hold and nothing to be unavailable.
func (PackSourceOperation) Available() bool { return true }

// Evaluate answers one seeded pack source.
func (PackSourceOperation) Evaluate(argument PackSourceArgument, emitter *relbindgen.Emitter[packdomain.Value]) outcome.Code {
	fact, reduction := packdomain.SourceFact(argument.Source)
	return relbindgen.Reduce(emitter, fact, reduction)
}
