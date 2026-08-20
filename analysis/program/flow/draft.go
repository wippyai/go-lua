package flow

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
)

// Draft is a construction-only Flow capability.  It has no public query,
// commit, or finalizer operation.  Copies share the private authored
// lifecycle; only Assemble can claim and consume it.
type Draft struct{ authored *authored.Draft }

// Build validates and copies the complete authored Flow vocabulary into one
// construction Draft.  Derived Outcome identities, exact-key joins,
// activation, evaluation ports, and all analysis projections are absent from
// this input and are created only by Assemble.
func Build(input authored.Input) (*Draft, error) {
	owner, err := authored.Build(input)
	if err != nil {
		return nil, err
	}
	return &Draft{authored: owner}, nil
}

// Abort terminally discards an unclaimed authored Flow Draft. The root
// artifact rebuild uses this only when a later sibling Build fails, before
// Assemble has had a chance to claim Flow. Keeping the operation here avoids
// leaving an earlier construction capability live across a failed rebuild.
func (draft *Draft) claim() (authored.Finalizer, error) {
	if draft == nil || draft.authored == nil {
		return authored.Finalizer{}, errors.New("program/flow: nil or consumed Draft")
	}
	return draft.authored.Finalizer()
}
