package modulecomposition

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// ModuleReturnStateEdge is one authenticated return transport from an exact
// module initialization Return outcome point back to the importing call's
// dispatch point.  It is deliberately not callable-specific: the engine may
// transport the complete PointState across this one edge.
type ModuleReturnStateEdge struct {
	id, link                                                identity.ContentID
	callTransitionID, generationID, outcomeID               identity.ContentID
	outcomePointID, callerReturnPointID, returnTransitionID identity.ContentID
	fromContextID, toContextID                              identity.ContentID
	returnModuleKey, callerModuleKey                        identity.ContentID
}

// NewModuleReturnStateEdge joins one authenticated module-call transition to
// one Return OutcomePoint of its exact initialization generation.  The return
// context edge is the directory's derived same-actor activation edge from
// callee to caller; it is not a caller-provided reverse transition.
func NewModuleReturnStateEdge(
	call ModuleCallTransition,
	generation InitGeneration,
	outcome InitOutcome,
	mount programmount.Program,
	point programschema.OutcomePoint,
	directory executioncontext.Directory,
) (ModuleReturnStateEdge, bool) {
	if !call.Available() || !generation.Available() || !outcome.Available() || !mount.Available() || !point.Available() || !directory.Available() ||
		call.LinkID() != generation.LinkID() || call.LinkID() != outcome.LinkID() || call.LinkID() != directory.LinkID() ||
		call.GenerationID() != generation.ID() || outcome.GenerationID() != generation.ID() || outcome.Kind() != programschema.OutcomeReturn ||
		!generationMountMatches(generation, mount) {
		return ModuleReturnStateEdge{}, false
	}
	body, bodyOK := mount.Program.EntryBody()
	if !bodyOK || body.ID() != generation.BodyID() {
		return ModuleReturnStateEdge{}, false
	}
	canonicalOutcome, outcomeOK := programOutcomeByID(mount.Program, body, outcome.OutcomeID())
	if !outcomeOK || canonicalOutcome.Kind() != programschema.OutcomeReturn || !outcomePointInProgram(mount.Program, canonicalOutcome, point) {
		return ModuleReturnStateEdge{}, false
	}
	forward, forwardOK := directory.Transition(call.FromContextID(), call.ToContextID())
	returnEdge, returnOK := directory.ActivationEdge(call.ToContextID(), call.FromContextID())
	from, fromOK := directory.Context(call.ToContextID())
	to, toOK := directory.Context(call.FromContextID())
	if !forwardOK || !returnOK || !fromOK || !toOK || forward.ID() != call.TransitionID() ||
		from.ModuleKey() != generation.ModuleKey() || to.ModuleKey() != call.SourceModuleKey() {
		return ModuleReturnStateEdge{}, false
	}
	row := ModuleReturnStateEdge{
		link:                call.LinkID(),
		callTransitionID:    call.ID(),
		generationID:        generation.ID(),
		outcomeID:           outcome.OutcomeID(),
		outcomePointID:      point.PointID(),
		callerReturnPointID: call.ReturnPointID(),
		returnTransitionID:  returnEdge.ID(),
		fromContextID:       from.ID(),
		toContextID:         to.ID(),
		returnModuleKey:     generation.ModuleKey(),
		callerModuleKey:     call.SourceModuleKey(),
	}
	row.id = moduleReturnStateEdgeID(row)
	return row, row.Available()
}

func outcomePointInProgram(program programschema.Program, outcome programschema.Outcome, wanted programschema.OutcomePoint) bool {
	if !program.Available() || !outcome.Available() || !wanted.Available() {
		return false
	}
	count, published := program.OutcomeCount()
	if !published {
		return false
	}
	index := -1
	for candidateIndex := 0; candidateIndex < count; candidateIndex++ {
		candidate, held := program.OutcomeAt(candidateIndex)
		if !held || !candidate.Available() {
			return false
		}
		if candidate.ID() != outcome.ID() {
			continue
		}
		if index >= 0 || candidate != outcome {
			return false
		}
		index = candidateIndex
	}
	if index < 0 {
		return false
	}
	found := false
	for pointIndex := 0; pointIndex < outcome.PointCount(); pointIndex++ {
		candidate, held := program.OutcomePointFor(index, pointIndex)
		if !held || !candidate.Available() {
			return false
		}
		if candidate.ID() != wanted.ID() {
			continue
		}
		if found || candidate != wanted {
			return false
		}
		found = true
	}
	return found
}

func (row ModuleReturnStateEdge) Available() bool {
	return row.id.Available() && row.link.Available() && row.callTransitionID.Available() && row.generationID.Available() && row.outcomeID.Available() &&
		row.outcomePointID.Available() && row.callerReturnPointID.Available() && row.returnTransitionID.Available() &&
		row.fromContextID.Available() && row.toContextID.Available() && row.returnModuleKey.Available() && row.callerModuleKey.Available() &&
		row.id == moduleReturnStateEdgeID(row)
}

func (row ModuleReturnStateEdge) ID() identity.ContentID     { return row.scalar(row.id) }
func (row ModuleReturnStateEdge) LinkID() identity.ContentID { return row.scalar(row.link) }
func (row ModuleReturnStateEdge) CallTransitionID() identity.ContentID {
	return row.scalar(row.callTransitionID)
}
func (row ModuleReturnStateEdge) GenerationID() identity.ContentID {
	return row.scalar(row.generationID)
}
func (row ModuleReturnStateEdge) OutcomeID() identity.ContentID { return row.scalar(row.outcomeID) }
func (row ModuleReturnStateEdge) OutcomePointID() identity.ContentID {
	return row.scalar(row.outcomePointID)
}
func (row ModuleReturnStateEdge) CallerReturnPointID() identity.ContentID {
	return row.scalar(row.callerReturnPointID)
}
func (row ModuleReturnStateEdge) ReturnTransitionID() identity.ContentID {
	return row.scalar(row.returnTransitionID)
}
func (row ModuleReturnStateEdge) FromContextID() identity.ContentID {
	return row.scalar(row.fromContextID)
}
func (row ModuleReturnStateEdge) ToContextID() identity.ContentID { return row.scalar(row.toContextID) }
func (row ModuleReturnStateEdge) ReturnModuleKey() identity.ContentID {
	return row.scalar(row.returnModuleKey)
}
func (row ModuleReturnStateEdge) CallerModuleKey() identity.ContentID {
	return row.scalar(row.callerModuleKey)
}

func (row ModuleReturnStateEdge) scalar(value identity.ContentID) identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return value
}
