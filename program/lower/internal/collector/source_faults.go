package collector

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// SourceFaults owns binder-rejected source evidence. A fault is still an
// authored Source occurrence, but it can never become a legal Flow transfer.
type SourceFaults struct{ collector *Collector }

// Faults selects Source's control-fault leaf.
func (r SourceRoot) Faults() SourceFaults { return SourceFaults{collector: r.collector} }

// ControlFault records one binder-produced fault with its direct Body owner.
func (f SourceFaults) ControlFault(span source.Span, owner Term, kind source.ControlFaultKind, label, blocker Term) Term {
	c := f.collector
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
	c.source.faults = append(c.source.faults, source.ControlFault{Owner: owner, Kind: kind, Label: label, Blocker: blocker})
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
