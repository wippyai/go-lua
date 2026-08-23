// form_activation.go owns the A form: structural activation transport. Its row
// is one candidate branch of one trigger - the transition the branch runs on,
// the two execution Contexts its endpoints reside in, the two States those
// endpoints occupy, the transport port it instantiates, and the disposition
// the branch settled.
//
// The row is an authentication receipt. A branch is authenticated once, where
// the sealed Link directory, the point layout, and the execution plan are all
// in scope; every later consumer of that branch reads its coordinates off this
// value. That is the whole reason the type exists: the endpoint Context
// assignment and the State pair were previously re-derived at each consumer
// from the transition tuple, so one branch carried several derivations of one
// answer and nothing held them to agreeing.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// ActivationSpec is the complete authenticated tuple of one candidate branch.
// It is a value handoff: the authenticating engine owns the directory and the
// plan, and hands the settled coordinates across as data.
type ActivationSpec struct {
	TransitionID  identity.ContentID
	FromContextID identity.ContentID
	ToContextID   identity.ContentID
	FromContext   contextfiber.ContextOrdinal
	ToContext     contextfiber.ContextOrdinal
	SourcePoint   contextfiber.PointOrdinal
	TargetPoint   contextfiber.PointOrdinal
	SourceState   contextfiber.StateOrdinal
	TargetState   contextfiber.StateOrdinal
	// Port is the transported Factor. FG-6 sealed a Factor named on both sides
	// of the vector as one bidirectional transport, so the port is one name
	// rather than a direction pair.
	Port composition.Key
	// Outcome is the branch's own settled disposition, read from the sealed
	// outcome column of the activation relation.
	Outcome structure.ReductionOutcome
}

// ActivationRow is one sealed activation candidate branch. Its fields are
// private: a consumer can read the authenticated coordinates and cannot mint a
// branch from coordinates no authentication produced.
type ActivationRow struct {
	spec   ActivationSpec
	sealed bool
}

// NewActivationRow seals one authenticated branch. Every coordinate of the
// transition tuple must be present and the disposition must be declared: a
// branch missing any of them was not authenticated, and admitting it would let
// a later consumer read a coordinate nothing decided.
func NewActivationRow(spec ActivationSpec) (ActivationRow, bool) {
	if !spec.TransitionID.Available() || !spec.FromContextID.Available() || !spec.ToContextID.Available() ||
		!spec.Port.Available() || !spec.Outcome.Available() {
		return ActivationRow{}, false
	}
	return ActivationRow{spec: spec, sealed: true}, true
}

// Available reports whether this row is a sealed authentication receipt.
func (row ActivationRow) Available() bool { return row.sealed }

// Transition is the sealed edge the branch runs on.
func (row ActivationRow) Transition() (identity.ContentID, identity.ContentID, identity.ContentID) {
	if !row.sealed {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}
	}
	return row.spec.TransitionID, row.spec.FromContextID, row.spec.ToContextID
}

// Contexts is the settled assignment of the transition's two Contexts to this
// branch's source and target endpoints.
func (row ActivationRow) Contexts() (contextfiber.ContextOrdinal, contextfiber.ContextOrdinal) {
	if !row.sealed {
		return 0, 0
	}
	return row.spec.FromContext, row.spec.ToContext
}

// Points is the branch's endpoint Point pair in the base graph.
func (row ActivationRow) Points() (contextfiber.PointOrdinal, contextfiber.PointOrdinal) {
	if !row.sealed {
		return 0, 0
	}
	return row.spec.SourcePoint, row.spec.TargetPoint
}

// States is the branch's endpoint State pair in the mounted execution plan.
func (row ActivationRow) States() (contextfiber.StateOrdinal, contextfiber.StateOrdinal) {
	if !row.sealed {
		return 0, 0
	}
	return row.spec.SourceState, row.spec.TargetState
}

// Port is the Factor this branch transports across its transition.
func (row ActivationRow) Port() composition.Key {
	if !row.sealed {
		return composition.Key{}
	}
	return row.spec.Port
}

// Outcome is the one disposition this branch settled.
func (row ActivationRow) Outcome() structure.ReductionOutcome {
	if !row.sealed {
		return structure.Refuse
	}
	return row.spec.Outcome
}

// ActivationBranches is one trigger's complete candidate branch set. A trigger
// with no branch is a sealed empty set, not a missing one: the difference
// between "this trigger admits no route" and "this trigger was never
// authenticated" is exactly what the trigger's own disposition states.
type ActivationBranches struct {
	rows   []ActivationRow
	sealed bool
}

// NewActivationBranches seals one trigger's branch set. Every member must be a
// sealed branch, so an unauthenticated row cannot contribute a disposition to
// the trigger's fold.
func NewActivationBranches(rows []ActivationRow) (ActivationBranches, bool) {
	for _, row := range rows {
		if !row.Available() {
			return ActivationBranches{}, false
		}
	}
	return ActivationBranches{rows: append([]ActivationRow(nil), rows...), sealed: true}, true
}

// Available reports whether this set is a sealed trigger branch set.
func (branches ActivationBranches) Available() bool { return branches.sealed }

// Count is the number of authenticated branches this trigger carries.
func (branches ActivationBranches) Count() int {
	if !branches.sealed {
		return 0
	}
	return len(branches.rows)
}

// At addresses one branch by its sealed ordinal.
func (branches ActivationBranches) At(index int) (ActivationRow, bool) {
	if !branches.sealed || index < 0 || index >= len(branches.rows) {
		return ActivationRow{}, false
	}
	return branches.rows[index], true
}

// Outcome is the trigger's own disposition, folded from its branches.
//
// A trigger with no branch settles NoSelection: its admitted bodies are a
// population that exists and the activation relation selected no row of it -
// which is the A form's declared producer obligation for that member. A
// refusing branch is fatal to the trigger, because a transport it declined to
// authenticate is one this trigger cannot publish around. One instantiated
// transport makes the trigger concrete. An authenticated admission of
// unknowing survives when nothing was instantiated, since it is a proved
// claim and not an absence. Branches that all declined leave the population
// present and the selection empty.
func (branches ActivationBranches) Outcome() structure.ReductionOutcome {
	if !branches.sealed || len(branches.rows) == 0 {
		return structure.NoSelection
	}
	settled := structure.NoSelection
	for _, row := range branches.rows {
		switch row.Outcome() {
		case structure.Refuse:
			return structure.Refuse
		case structure.Concrete:
			if settled != structure.Concrete {
				settled = structure.Concrete
			}
		case structure.AuthenticatedOpaque:
			if settled != structure.Concrete {
				settled = structure.AuthenticatedOpaque
			}
		case structure.NoCandidate:
			if settled == structure.NoSelection {
				settled = structure.NoCandidate
			}
		}
	}
	return settled
}
