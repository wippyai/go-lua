package modulecomposition

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// ModuleCallTransition is the Link-lifetime join for one module import call
// and the exact execution-context edge through which that call is admitted.
//
// The constructor takes the canonical CacheIngress, its authenticated target
// InitGeneration, the exact ModuleImport row from the source mounted Program,
// and the exact execution-context Transition. Once admitted, the row retains
// only scalar identities; the mounted Program and either source row are not
// retained as mutable or reusable state.
type ModuleCallTransition struct {
	id, link, cacheIngressID, sourceModuleKey identity.ContentID
	sourcePointID, returnPointID              identity.ContentID
	artifactID, programID, importID, callID   identity.ContentID
	generationID, transitionID                identity.ContentID
	fromContextID, toContextID                identity.ContentID
}

// NewModuleCallTransition constructs one canonical module-call transition.
// The InitGeneration must be an authenticated generation for the exact cache
// ingress and target module.  The ModuleImport must be the exact row published
// by mount.Program.  The cache ingress must name that import, source mount,
// and Link, while the transition must name the same Link and both exact cache
// endpoints.
//
// The constructor intentionally accepts rows rather than raw identities: a
// caller cannot supply a second import, mount, cache, or endpoint geometry for
// the same resulting row.
func NewModuleCallTransition(cache CacheIngress, generation InitGeneration, mount programmount.Program, importRow programschema.ModuleImport, transition executioncontext.Transition) (ModuleCallTransition, bool) {
	if !cache.Available() || !generation.Available() || !mount.Available() || !importRow.Available() || !transition.Available() {
		return ModuleCallTransition{}, false
	}
	if generation.CacheIngressID() != cache.ID() ||
		generation.LinkID() != cache.LinkID() ||
		generation.ModuleKey() != cache.TargetModuleKey() {
		return ModuleCallTransition{}, false
	}
	if !moduleImportInProgram(mount.Program, importRow) {
		return ModuleCallTransition{}, false
	}
	sourcePointID, sourcePointOK := moduleCallDispatchPoint(mount.Program, importRow.CallID())
	returnPointID, returnPointOK := moduleCallReturnPoint(mount.Program, importRow.CallID())
	if !sourcePointOK || !returnPointOK {
		return ModuleCallTransition{}, false
	}
	request, requestOK := moduleRequestForImport(mount.Program, importRow, cache.RequestID())
	if !requestOK {
		return ModuleCallTransition{}, false
	}
	resolved, resolvedOK := NewResolvedImport(cache.LinkID(), mount, request, cache.TargetModuleKey())
	if !resolvedOK || resolved.ID() != cache.ImportID() {
		return ModuleCallTransition{}, false
	}
	if cache.LinkID() != transition.LinkID() ||
		cache.SourceModuleKey() != mount.ModuleKey ||
		cache.FromContextID() != transition.FromContextID() ||
		cache.ToContextID() != transition.ToContextID() {
		return ModuleCallTransition{}, false
	}

	row := ModuleCallTransition{
		link:            cache.LinkID(),
		cacheIngressID:  cache.ID(),
		sourceModuleKey: mount.ModuleKey,
		sourcePointID:   sourcePointID,
		returnPointID:   returnPointID,
		artifactID:      mount.ArtifactID,
		programID:       mount.ProgramID,
		importID:        importRow.ID(),
		callID:          importRow.CallID(),
		generationID:    generation.ID(),
		transitionID:    transition.ID(),
		fromContextID:   transition.FromContextID(),
		toContextID:     transition.ToContextID(),
	}
	row.id = moduleCallTransitionID(row)
	return row, row.Available()
}

func moduleRequestForImport(program programschema.Program, importRow programschema.ModuleImport, requestID identity.ContentID) (programschema.ModuleRequest, bool) {
	if !program.Available() || !importRow.Available() || !requestID.Available() {
		return programschema.ModuleRequest{}, false
	}
	count, published := program.ModuleRequestCount()
	if !published {
		return programschema.ModuleRequest{}, false
	}
	var found programschema.ModuleRequest
	for index := 0; index < count; index++ {
		candidate, held := program.ModuleRequestAt(index)
		if !held || !candidate.Available() {
			return programschema.ModuleRequest{}, false
		}
		if candidate.ID() != requestID {
			continue
		}
		if found.Available() || candidate.ImportID() != importRow.ID() {
			return programschema.ModuleRequest{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

// Available reports whether the row is a complete, self-authenticated scalar
// join.  Membership in the mounted Program and agreement with the source
// rows are authenticated by NewModuleCallTransition before the Program is
// discarded; the identity equation protects the sealed scalar row thereafter.
func (row ModuleCallTransition) Available() bool {
	return row.id.Available() && row.link.Available() && row.cacheIngressID.Available() &&
		row.sourceModuleKey.Available() && row.sourcePointID.Available() && row.returnPointID.Available() && row.artifactID.Available() && row.programID.Available() &&
		row.importID.Available() && row.callID.Available() && row.generationID.Available() && row.transitionID.Available() &&
		row.fromContextID.Available() && row.toContextID.Available() &&
		row.id == moduleCallTransitionID(row)
}

func (row ModuleCallTransition) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) LinkID() identity.ContentID {
	if row.Available() {
		return row.link
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) CacheIngressID() identity.ContentID {
	if row.Available() {
		return row.cacheIngressID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) SourceModuleKey() identity.ContentID {
	if row.Available() {
		return row.sourceModuleKey
	}
	return identity.ContentID{}
}

// SourcePointID is the reusable Program point issued by the canonical
// StageCallDispatch placement for this import's semantic call occurrence.
// It is authenticated during construction from the sealed source Program;
// callers cannot select a role stage or reconstruct a point from CallID.
func (row ModuleCallTransition) SourcePointID() identity.ContentID {
	if row.Available() {
		return row.sourcePointID
	}
	return identity.ContentID{}
}

// ReturnPointID is the reusable Program point issued by the canonical
// StageCallEffect placement. Return-state transports terminate at the
// post-call continuation so callee Heap, Placement, and Effect state enters
// the caller's ordinary successor flow.
func (row ModuleCallTransition) ReturnPointID() identity.ContentID {
	if row.Available() {
		return row.returnPointID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) ArtifactID() identity.ContentID {
	if row.Available() {
		return row.artifactID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) ProgramID() identity.ContentID {
	if row.Available() {
		return row.programID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) ImportID() identity.ContentID {
	if row.Available() {
		return row.importID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) CallID() identity.ContentID {
	if row.Available() {
		return row.callID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) GenerationID() identity.ContentID {
	if row.Available() {
		return row.generationID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) TransitionID() identity.ContentID {
	if row.Available() {
		return row.transitionID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) FromContextID() identity.ContentID {
	if row.Available() {
		return row.fromContextID
	}
	return identity.ContentID{}
}

func (row ModuleCallTransition) ToContextID() identity.ContentID {
	if row.Available() {
		return row.toContextID
	}
	return identity.ContentID{}
}

// moduleImportInProgram authenticates the complete ModuleImport value, not
// merely its ID.  A duplicate ID or a same-ID forged row is therefore never
// admitted as belonging to a mounted Program.
func moduleImportInProgram(program programschema.Program, wanted programschema.ModuleImport) bool {
	if !program.Available() || !wanted.Available() {
		return false
	}
	count, published := program.ModuleImportCount()
	if !published {
		return false
	}
	found := false
	for index := 0; index < count; index++ {
		candidate, held := program.ModuleImportAt(index)
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

// moduleCallDispatchPoint derives the sole source reusable point for one
// semantic call. The Program's RuleOccurrence rows are authoritative: only
// the canonical call-dispatch stage names the module-call boundary. Multiple
// roles may publish that stage, but they must all agree on the exact point;
// distinct dispatch points are an ambiguity and refuse the join.
func moduleCallDispatchPoint(program programschema.Program, callID identity.ContentID) (identity.ContentID, bool) {
	return moduleCallStagePoint(program, callID, programissuance.StageCallDispatch)
}

func moduleCallReturnPoint(program programschema.Program, callID identity.ContentID) (identity.ContentID, bool) {
	return moduleCallStagePoint(program, callID, programissuance.StageCallEffect)
}

func moduleCallStagePoint(program programschema.Program, callID identity.ContentID, stage schema.Key) (identity.ContentID, bool) {
	if !program.Available() || !callID.Available() {
		return identity.ContentID{}, false
	}
	ordinal, ordinalOK := program.OccurrenceOrdinalForID(programschema.OccurrenceCall, callID)
	count, published := program.RuleOccurrenceCount()
	if !ordinalOK || !published {
		return identity.ContentID{}, false
	}
	var point identity.ContentID
	found := false
	for index := 0; index < count; index++ {
		placement, held := program.RuleOccurrenceAt(index)
		occurrence, occurrenceOK := placement.Occurrence()
		if !held || !placement.Available() || !occurrenceOK {
			return identity.ContentID{}, false
		}
		if int(occurrence) != ordinal || placement.Stage() != stage {
			continue
		}
		candidate := placement.PointID()
		if !candidate.Available() {
			return identity.ContentID{}, false
		}
		if found && point != candidate {
			return identity.ContentID{}, false
		}
		point, found = candidate, true
	}
	return point, found
}
