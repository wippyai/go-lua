package modulecomposition

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// ModuleExportCallableOriginKind identifies the two canonical shapes of an exported
// callable value.  The distinction is semantic: a root function is a sparse
// ModuleEntryRootFunction row, while a member function is the terminal row of
// an exact ModuleEntryMember chain.
type ModuleExportCallableOriginKind uint8

const (
	ModuleExportCallableOriginInvalid ModuleExportCallableOriginKind = iota
	ModuleExportCallableOriginRootFunction
	ModuleExportCallableOriginMember
)

func (kind ModuleExportCallableOriginKind) valid() bool {
	return kind == ModuleExportCallableOriginRootFunction || kind == ModuleExportCallableOriginMember
}

// ModuleExportCallableOrigin is the Link-owned proof that one exported callable value
// came from one exact module-cache transition and one exact closure
// allocation.  The Program remains immutable and shared; this row carries
// only the Link-qualified relation needed to select its contextual origin.
//
// transitionID/fromContextID/toContextID are deliberately all retained.
// bodyContextID is the lexical context of the sealed call target and is not
// the execution context toContextID.  In particular, a consumer must not
// replace one with the other when it builds a contextual Value reference.
type ModuleExportCallableOrigin struct {
	id, key                            identity.ContentID
	link, transitionID                 identity.ContentID
	fromContextID, toContextID         identity.ContentID
	generationID, outcomeID, entryID   identity.ContentID
	exportID, functionID, allocationID identity.ContentID
	bodyID, bodyContextID, formalID    identity.ContentID
	moduleKey, artifactID, programID   identity.ContentID
	kind                               ModuleExportCallableOriginKind
}

// NewModuleExportCallableOriginRoot constructs an origin for an exported root function.
// Every argument is an already-authenticated row from one canonical source;
// the constructor rechecks all cross-family joins before discarding the
// source rows.
func NewModuleExportCallableOriginRoot(
	transition ModuleCallTransition,
	generation InitGeneration,
	outcome InitOutcome,
	mount programmount.Program,
	entry programschema.ModuleEntry,
	root programschema.ModuleEntryRootFunction,
	target calltarget.Target,
	allocation heapallocation.Allocation,
) (ModuleExportCallableOrigin, bool) {
	if !validateCallableOriginBase(transition, generation, outcome, mount, entry, target, allocation) {
		return ModuleExportCallableOrigin{}, false
	}
	entryIndex, entryOK := moduleEntryIndex(mount.Program, entry)
	if !entryOK || !root.Available() || root.EntryID() != entry.ID() {
		return ModuleExportCallableOrigin{}, false
	}
	position := root.Position()
	canonical, canonicalOK := mount.Program.ModuleEntryRootFunctionFor(entryIndex, int(position))
	if !canonicalOK || canonical != root || root.FunctionID() != target.FunctionID() {
		return ModuleExportCallableOrigin{}, false
	}
	return newModuleExportCallableOrigin(
		ModuleExportCallableOriginRootFunction, transition, generation, outcome, mount,
		entry.ID(), root.ID(), root.FunctionID(), target, allocation,
	), true
}

// NewModuleExportCallableOriginMember constructs an origin for the terminal function of
// one exact exported member chain.  members must be in root-to-terminal order;
// the constructor never sorts, searches for, or guesses a chain.
func NewModuleExportCallableOriginMember(
	transition ModuleCallTransition,
	generation InitGeneration,
	outcome InitOutcome,
	mount programmount.Program,
	entry programschema.ModuleEntry,
	members []programschema.ModuleEntryMember,
	target calltarget.Target,
	allocation heapallocation.Allocation,
) (ModuleExportCallableOrigin, bool) {
	if !validateCallableOriginBase(transition, generation, outcome, mount, entry, target, allocation) || len(members) == 0 {
		return ModuleExportCallableOrigin{}, false
	}
	entryIndex, entryOK := moduleEntryIndex(mount.Program, entry)
	if !entryOK || !validateMemberChain(mount.Program, entry, entryIndex, members, target, allocation) {
		return ModuleExportCallableOrigin{}, false
	}
	terminal := members[len(members)-1]
	return newModuleExportCallableOrigin(
		ModuleExportCallableOriginMember, transition, generation, outcome, mount,
		entry.ID(), terminal.ID(), target.FunctionID(), target, allocation,
	), true
}

func newModuleExportCallableOrigin(
	kind ModuleExportCallableOriginKind,
	transition ModuleCallTransition,
	generation InitGeneration,
	outcome InitOutcome,
	mount programmount.Program,
	entryID, exportID, functionID identity.ContentID,
	target calltarget.Target,
	allocation heapallocation.Allocation,
) ModuleExportCallableOrigin {
	row := ModuleExportCallableOrigin{
		link:          transition.LinkID(),
		transitionID:  transition.TransitionID(),
		fromContextID: transition.FromContextID(),
		toContextID:   transition.ToContextID(),
		generationID:  generation.ID(),
		outcomeID:     outcome.OutcomeID(),
		entryID:       entryID,
		exportID:      exportID,
		functionID:    functionID,
		allocationID:  allocation.ID(),
		bodyID:        target.BodyID(),
		bodyContextID: target.ContextID(),
		formalID:      target.FormalID(),
		moduleKey:     mount.ModuleKey,
		artifactID:    mount.ArtifactID,
		programID:     mount.ProgramID,
		kind:          kind,
	}
	row.key = moduleExportCallableOriginKeyID(row.transitionID, row.allocationID)
	row.id = moduleExportCallableOriginID(row)
	return row
}

// validateCallableOriginBase authenticates all facts shared by root and
// member exports.  The target and allocation are re-read from the sealed
// Program planes, so callers cannot pass a well-formed row from another
// artifact merely because its scalar identities happen to look compatible.
func validateCallableOriginBase(
	transition ModuleCallTransition,
	generation InitGeneration,
	outcome InitOutcome,
	mount programmount.Program,
	entry programschema.ModuleEntry,
	target calltarget.Target,
	allocation heapallocation.Allocation,
) bool {
	if !transition.Available() || !generation.Available() || !outcome.Available() || !mount.Available() ||
		!entry.Available() || !target.Available() || !allocation.Available() {
		return false
	}
	if outcome.Kind() != programschema.OutcomeReturn ||
		transition.LinkID() != generation.LinkID() ||
		transition.GenerationID() != generation.ID() ||
		outcome.GenerationID() != generation.ID() ||
		generation.ModuleKey() != mount.ModuleKey ||
		generation.ArtifactID() != mount.ArtifactID ||
		generation.ProgramID() != mount.ProgramID ||
		outcome.OutcomeID() != entry.ReturnID() {
		return false
	}
	returnOrdinal, ordinalOK := entry.ReturnOrdinal()
	outcomeOrdinal, outcomeOrdinalOK := outcome.ReturnOrdinal()
	if !ordinalOK || !outcomeOrdinalOK || returnOrdinal != outcomeOrdinal {
		return false
	}
	if allocation.Role() != heapallocation.RoleClosure || allocation.Form() != heapallocation.FormEmpty ||
		target.AllocationID() != allocation.ID() {
		return false
	}

	state, stateOK := mount.Program.ColdState()
	callTargets, targetsOK := calltarget.NewView(state)
	allocations, allocationsOK := heapallocation.NewView(state)
	if !stateOK || !targetsOK || !allocationsOK {
		return false
	}
	canonicalAllocation, allocationOK := allocations.AllocationForID(allocation.ID())
	if !allocationOK || canonicalAllocation != allocation {
		return false
	}
	canonicalTarget, targetOK := canonicalTargetForValue(callTargets, target)
	if !targetOK || canonicalTarget != target {
		return false
	}
	canonicalBody, bodyOK := bodyForID(mount.Program, target.BodyID())
	if !bodyOK || !canonicalBody.Callable() || canonicalBody.ContextID() != target.ContextID() {
		return false
	}
	functionID, functionOK := canonicalBody.FunctionContextID()
	formalID, formalOK := canonicalBody.CallFormalID()
	if !functionOK || !formalOK || functionID != target.FunctionID() || formalID != target.FormalID() {
		return false
	}
	boundary, boundaryOK := mount.Program.FunctionBoundaryForBody(target.BodyID())
	return boundaryOK && boundary.BodyContextID() == target.ContextID() && boundary.CallFormalID() == target.FormalID()
}

func canonicalTargetForValue(view calltarget.View, wanted calltarget.Target) (calltarget.Target, bool) {
	if !view.Available() || !wanted.Available() {
		return calltarget.Target{}, false
	}
	count, published := view.Count()
	if !published {
		return calltarget.Target{}, false
	}
	var found calltarget.Target
	for index := 0; index < count; index++ {
		candidate, held := view.At(index)
		if !held || candidate != wanted {
			continue
		}
		if found.Available() {
			return calltarget.Target{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

func bodyForID(program programschema.Program, id identity.ContentID) (programschema.Body, bool) {
	if !program.Available() || !id.Available() {
		return programschema.Body{}, false
	}
	count, published := program.BodyCount()
	if !published {
		return programschema.Body{}, false
	}
	var found programschema.Body
	for index := 0; index < count; index++ {
		candidate, held := program.BodyAt(index)
		if !held || candidate.ID() != id {
			continue
		}
		if found.Available() {
			return programschema.Body{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

func moduleEntryIndex(program programschema.Program, wanted programschema.ModuleEntry) (int, bool) {
	if !program.Available() || !wanted.Available() {
		return 0, false
	}
	count, published := program.ModuleEntryCount()
	if !published {
		return 0, false
	}
	index := -1
	for candidateIndex := 0; candidateIndex < count; candidateIndex++ {
		candidate, held := program.ModuleEntryAt(candidateIndex)
		if !held || !candidate.Available() {
			return 0, false
		}
		if candidate.ID() != wanted.ID() {
			continue
		}
		if index >= 0 || candidate != wanted {
			return 0, false
		}
		index = candidateIndex
	}
	return index, index >= 0
}

func validateMemberChain(
	program programschema.Program,
	entry programschema.ModuleEntry,
	entryIndex int,
	members []programschema.ModuleEntryMember,
	target calltarget.Target,
	allocation heapallocation.Allocation,
) bool {
	if len(members) == 0 || !program.Available() || !entry.Available() || !target.Available() || !allocation.Available() {
		return false
	}
	state, stateOK := program.ColdState()
	allocations, allocationsOK := heapallocation.NewView(state)
	if !stateOK || !allocationsOK {
		return false
	}
	rootWidth, widthOK := entry.RootWidth()
	if !widthOK {
		return false
	}
	position := members[0].Position()
	if uint64(position) >= uint64(rootWidth) {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, len(members))
	for index, member := range members {
		if !member.Available() || member.EntryID() != entry.ID() || member.Position() != position {
			return false
		}
		if _, duplicate := seen[member.ID()]; duplicate {
			return false
		}
		seen[member.ID()] = struct{}{}
		canonical, canonicalOK := canonicalMemberAt(program, entry, entryIndex, member)
		if !canonicalOK || canonical != member {
			return false
		}
		table, tableOK := allocations.AllocationForID(member.TableID())
		if !tableOK || table.Role() != heapallocation.RoleTable || table.ID() != member.TableID() {
			return false
		}
		if index == 0 {
			if member.ParentID() != member.TableID() {
				return false
			}
		} else {
			previous := members[index-1]
			if hasMemberValue(previous) || member.ParentID() != previous.FieldID() {
				return false
			}
		}
	}
	terminal := members[len(members)-1]
	valueID, valueOK := terminal.ValueID()
	return valueOK && valueID == target.FunctionID()
}

func canonicalMemberAt(program programschema.Program, entry programschema.ModuleEntry, entryIndex int, wanted programschema.ModuleEntryMember) (programschema.ModuleEntryMember, bool) {
	offset, count, spanOK := entry.MemberSpan()
	if !spanOK {
		return programschema.ModuleEntryMember{}, false
	}
	var found programschema.ModuleEntryMember
	for childIndex := uint32(0); childIndex < count; childIndex++ {
		candidate, held := program.ModuleEntryMemberAt(int(offset + childIndex))
		if !held || !candidate.Available() || candidate.EntryID() != entry.ID() {
			return programschema.ModuleEntryMember{}, false
		}
		if candidate.ID() != wanted.ID() {
			continue
		}
		if found.Available() || candidate != wanted {
			return programschema.ModuleEntryMember{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

// hasMemberValue is an explicit shape predicate. It avoids treating the zero
// value as an implicit wildcard while validating an intermediate chain row.
func hasMemberValue(row programschema.ModuleEntryMember) bool {
	_, ok := row.ValueID()
	return ok
}

func (row ModuleExportCallableOrigin) Available() bool {
	return row.id.Available() && row.key.Available() && row.link.Available() && row.transitionID.Available() &&
		row.fromContextID.Available() && row.toContextID.Available() && row.generationID.Available() && row.outcomeID.Available() &&
		row.entryID.Available() && row.exportID.Available() && row.functionID.Available() && row.allocationID.Available() &&
		row.bodyID.Available() && row.bodyContextID.Available() && row.formalID.Available() && row.moduleKey.Available() &&
		row.artifactID.Available() && row.programID.Available() && row.kind.valid() &&
		row.key == moduleExportCallableOriginKeyID(row.transitionID, row.allocationID) && row.id == moduleExportCallableOriginID(row)
}

func (row ModuleExportCallableOrigin) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}

// ConsumerKey is the exact lookup key for a contextual callable.  Two actor
// transitions may point at the same immutable closure allocation; their keys
// remain distinct because the full transition identity is retained.
func (row ModuleExportCallableOrigin) ConsumerKey() identity.ContentID {
	if row.Available() {
		return row.key
	}
	return identity.ContentID{}
}

func (row ModuleExportCallableOrigin) LinkID() identity.ContentID { return row.scalar(row.link) }
func (row ModuleExportCallableOrigin) TransitionID() identity.ContentID {
	return row.scalar(row.transitionID)
}
func (row ModuleExportCallableOrigin) FromContextID() identity.ContentID {
	return row.scalar(row.fromContextID)
}
func (row ModuleExportCallableOrigin) ToContextID() identity.ContentID {
	return row.scalar(row.toContextID)
}
func (row ModuleExportCallableOrigin) GenerationID() identity.ContentID {
	return row.scalar(row.generationID)
}
func (row ModuleExportCallableOrigin) OutcomeID() identity.ContentID {
	return row.scalar(row.outcomeID)
}
func (row ModuleExportCallableOrigin) EntryID() identity.ContentID  { return row.scalar(row.entryID) }
func (row ModuleExportCallableOrigin) ExportID() identity.ContentID { return row.scalar(row.exportID) }
func (row ModuleExportCallableOrigin) FunctionID() identity.ContentID {
	return row.scalar(row.functionID)
}
func (row ModuleExportCallableOrigin) AllocationID() identity.ContentID {
	return row.scalar(row.allocationID)
}
func (row ModuleExportCallableOrigin) BodyID() identity.ContentID { return row.scalar(row.bodyID) }
func (row ModuleExportCallableOrigin) BodyContextID() identity.ContentID {
	return row.scalar(row.bodyContextID)
}
func (row ModuleExportCallableOrigin) FormalID() identity.ContentID { return row.scalar(row.formalID) }
func (row ModuleExportCallableOrigin) ModuleKey() identity.ContentID {
	return row.scalar(row.moduleKey)
}
func (row ModuleExportCallableOrigin) ArtifactID() identity.ContentID {
	return row.scalar(row.artifactID)
}
func (row ModuleExportCallableOrigin) ProgramID() identity.ContentID {
	return row.scalar(row.programID)
}
func (row ModuleExportCallableOrigin) Kind() ModuleExportCallableOriginKind {
	if row.Available() {
		return row.kind
	}
	return ModuleExportCallableOriginInvalid
}

func (row ModuleExportCallableOrigin) scalar(value identity.ContentID) identity.ContentID {
	if row.Available() {
		return value
	}
	return identity.ContentID{}
}
