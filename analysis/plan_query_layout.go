package analysis

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/domain/composite"
)

// QueryResultLayout returns the sealed layout one query family's answers were
// detached under in this plan. A consumer opens a published cell against it,
// so the declaration a payload is read by is the same one the compilation
// sealed and wrote it under, never a second copy kept beside the reader.
func (plan *Plan) QueryResultLayout(family schema.Key) (*plane.Sealed, bool) {
	state, leased := plan.acquire()
	if !leased {
		return nil, false
	}
	defer state.releaseLease()
	return composite.QueryResultLayout(state.compilation, family)
}
