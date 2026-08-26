package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	staticdomain "github.com/wippyai/go-lua/domain/static"
)

// StaticTransferOperation is domain/static's own type-fact transfer, which
// carries a fact from the coordinate it was observed at to the one it is
// stored at.
type StaticTransferOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (StaticTransferOperation) Available() bool { return true }

// Evaluate answers one static type-fact transfer.
func (StaticTransferOperation) Evaluate(argument StaticTransferArgument, emitter *relbindgen.Emitter[staticdomain.TypeFact]) outcome.Code {
	fact, reduction := staticdomain.IdentityTypeFact(argument.Source)
	return relbindgen.Reduce(emitter, fact, reduction)
}
