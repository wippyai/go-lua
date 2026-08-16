package assembly

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// ControlFault records one binder-produced fault with its direct Body owner.
func (c *Collector) ControlFault(span source.Span, owner Term, kind source.ControlFaultKind, label, blocker Term) Term {
	if !validOwner(c, owner) || !validControlFaultTerms(c, kind, label, blocker) {
		if c != nil && c.err == nil && !c.terminal {
			c.fail(errors.New("program/lower/collector: invalid control fault"))
		}
		return 0
	}
	term := c.mint(keyspace.FamilyControlFault, span)
	if term == 0 {
		return 0
	}
	c.source.AddFault(source.ControlFault{Owner: owner, Kind: kind, Label: label, Blocker: blocker})
	return term
}

// validControlFaultTerms mirrors Source's closed fault grammar at the
// construction boundary. Rejecting malformed label/blocker combinations here
// keeps a bad row from surviving until Source.Build, where it would otherwise
// become a late, less-local failure.
func validControlFaultTerms(c *Collector, kind source.ControlFaultKind, label, blocker Term) bool {
	if c == nil || kind < source.ControlFaultDuplicateLabel || kind > source.ControlFaultBreakOutsideLoop {
		return false
	}
	if label != 0 && !validFamilyTerm(c, label, keyspace.FamilyLabel) {
		return false
	}
	if blocker != 0 && !validFamilyTerm(c, blocker, keyspace.FamilyCell) {
		return false
	}
	switch kind {
	case source.ControlFaultDuplicateLabel:
		return label != 0 && blocker == 0
	case source.ControlFaultUndefinedGoto, source.ControlFaultBreakOutsideLoop:
		return label == 0 && blocker == 0
	case source.ControlFaultGotoEntersLocal:
		return label != 0 && blocker != 0
	default:
		return false
	}
}
