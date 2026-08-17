package program

// This file owns the scalar identity equations for storage and value-source
// rows. It reads the sealed Source/Flow/Static quartet directly and returns
// IDs and Terms; it does not issue transient proof objects or retain rows.

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// EvaluationSpan returns the exact existing Entry/Finish geometry for term.
// The returned spanID is the former Span.ContextID equation; entry and finish
// are the existing endpoint Terms, not synthesized coordinates.
func (program *Program) EvaluationSpan(term keyspace.Term) (spanID identity.ContentID, entry, finish keyspace.Term, ok bool) {
	if !program.scalarIdentityAvailable() || term == 0 {
		return identity.ContentID{}, 0, 0, false
	}
	ports, sites := program.Flow().Ports(), program.Flow().Causal().Sites()
	entry, entryOK := ports.Entry(term)
	finish, finishOK := ports.Finish(term)
	if !entryOK || !finishOK || entry == 0 || finish == 0 {
		return identity.ContentID{}, 0, 0, false
	}
	entrySite, entrySiteOK := sites.ForTerm(entry)
	finishSite, finishSiteOK := sites.ForTerm(finish)
	if !entrySiteOK || !finishSiteOK || !entrySite.Available() || !finishSite.Available() {
		return identity.ContentID{}, 0, 0, false
	}
	entryID, finishID := entrySite.ContextID(), finishSite.ContextID()
	if !entryID.Available() || !finishID.Available() {
		return identity.ContentID{}, 0, 0, false
	}
	spanID = programRoleID("program/transformer/span", program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Uint(uint64(keyspace.TermFamily(term))) == nil &&
			writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil &&
			writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	})
	return spanID, entry, finish, spanID.Available()
}

// StorageCellIDAt returns the former storage Cell ContextID in authored Cell
// order. The Cell term remains available from Flow.Storage.Cells().At.
func (program *Program) StorageCellIDAt(index int) (identity.ContentID, bool) {
	if !program.scalarIdentityAvailable() || index < 0 {
		return identity.ContentID{}, false
	}
	term, ok := program.Flow().Authored().Storage().Cells().At(index)
	if !ok || term == 0 {
		return identity.ContentID{}, false
	}
	id := programRoleID("program/transformer/storage-cell", program.ContentID(), func(writer *framing.Writer) bool {
		return writeProgramTerm(writer, term)
	})
	return id, id.Available()
}

// StorageReadIDAt returns (read identity, exact evaluation span identity,
// authored Read term). Dead or malformed rows remain in the denominator and
// fail closed rather than being compacted into a new one.
func (program *Program) StorageReadIDAt(index int) (readID, spanID identity.ContentID, term keyspace.Term, ok bool) {
	if !program.scalarIdentityAvailable() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	view := program.Flow()
	reads := view.Authored().Storage().Reads()
	term, present := reads.At(index)
	owner, source, _, related := reads.Get(term)
	if !present || !related || term == 0 || owner == 0 || source == 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	if _, _, _, cellOK := view.Authored().Storage().Cells().Get(source); !cellOK {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	bodyPath, bodyID, bodyOK := program.scalarBody(owner)
	spanID, entry, finish, spanOK := program.EvaluationSpan(term)
	if !bodyOK || !spanOK {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	ports, sites := view.Ports(), view.Causal().Sites()
	entryTerm, entryOK := ports.Entry(term)
	finishTerm, finishOK := ports.Finish(term)
	entrySite, entrySiteOK := sites.ForTerm(entryTerm)
	finishSite, finishSiteOK := sites.ForTerm(finishTerm)
	if !entryOK || !finishOK || entryTerm != entry || finishTerm != finish || !entrySiteOK || !finishSiteOK {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	entryID, finishID := entrySite.ContextID(), finishSite.ContextID()
	readID = programRoleID("program/transformer/storage-read", program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(bodyID[:]) == nil &&
			writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	})
	return readID, spanID, term, readID.Available() && spanID.Available()
}

// StorageBindIDAt returns the canonical scalar identity of one executable
// authored Bind row. Source contributes the fixed destination width while
// Flow contributes the Body and evaluation geometry; the bind row itself is
// never retained by Program.
func (program *Program) StorageBindIDAt(index int) (identity.ContentID, bool) {
	if !program.scalarIdentityAvailable() || index < 0 {
		return identity.ContentID{}, false
	}
	view := program.Flow()
	binds := view.Authored().Storage().Binds()
	term, present := binds.At(index)
	owner, values, related := binds.Get(term)
	width, widthOK := program.Source().Binds().Len(term)
	if !present || !related || !widthOK || width < 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, false
	}
	if _, _, valuesOK := view.Authored().Values().Get(values); !valuesOK {
		return identity.ContentID{}, false
	}
	bodyPath, bodyID, bodyOK := program.scalarBody(owner)
	_, entryTerm, finishTerm, spanOK := program.EvaluationSpan(term)
	entry, entryOK := view.Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := view.Causal().Sites().ForTerm(finishTerm)
	if !bodyOK || !spanOK || !entryOK || !finishOK || !entry.Available() || !finish.Available() {
		return identity.ContentID{}, false
	}
	entryID, finishID := entry.ContextID(), finish.ContextID()
	id := programRoleID("program/transformer/storage-bind", program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Count(uint64(width)) == nil && writer.Bytes(bodyID[:]) == nil &&
			writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	})
	return id, id.Available()
}

// StorageBindTransferIDAt returns the scalar identity of one fixed Bind
// transfer. Open Values tails have no transfer identity; destination Cell
// admission remains a separate cold join consumed by Artifact.
func (program *Program) StorageBindTransferIDAt(index, position int) (identity.ContentID, bool) {
	if position < 0 {
		return identity.ContentID{}, false
	}
	bindID, bindOK := program.StorageBindIDAt(index)
	if !bindOK || !bindID.Available() {
		return identity.ContentID{}, false
	}
	view := program.Flow()
	binds := view.Authored().Storage().Binds()
	term, present := binds.At(index)
	_, values, related := binds.Get(term)
	width, widthOK := program.Source().Binds().Len(term)
	cellTerm, cellOK := program.Source().Binds().At(term, position)
	_, fixed := view.Authored().Values().Member(values, position)
	if !present || !related || !widthOK || position >= width || !cellOK || !fixed {
		return identity.ContentID{}, false
	}
	if _, _, _, storageCellOK := view.Authored().Storage().Cells().Get(cellTerm); !storageCellOK {
		return identity.ContentID{}, false
	}
	id := programRoleID("program/transformer/storage-bind-transfer", program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Bytes(bindID[:]) == nil && writer.Uint(uint64(position)) == nil
	})
	return id, id.Available()
}

// StorageAssignmentIDAt returns the canonical owner-neutral identity of one
// executable authored assignment. Its identity is structural (Body path,
// assignment path, and destination width), not a transport row index.
func (program *Program) StorageAssignmentIDAt(index int) (identity.ContentID, bool) {
	if !program.scalarIdentityAvailable() || index < 0 {
		return identity.ContentID{}, false
	}
	view := program.Flow()
	assigns := view.Authored().Storage().Assigns()
	term, present := assigns.At(index)
	owner, values, related := assigns.Get(term)
	width, widthOK := assigns.WriteCount(term)
	if !present || !related || !widthOK || width <= 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, false
	}
	if _, _, valuesOK := view.Authored().Values().Get(values); !valuesOK {
		return identity.ContentID{}, false
	}
	bodyPath, _, bodyOK := program.scalarBody(owner)
	assignmentPath, assignmentPathOK := view.StorageAssignmentPath(term)
	if !bodyOK || !assignmentPathOK || !bodyPath.Available() || !assignmentPath.Available() {
		return identity.ContentID{}, false
	}
	id := programSemanticID("program/transformer/storage-assignment", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(assignmentPath[:]) == nil && writer.Count(uint64(width)) == nil
	})
	return id, id.Available()
}

// AssignmentPredecessorID returns the canonical identity of the sealed local
// reverse-commit predecessor for one authored Write. It also returns the
// route identity used by Artifact's scalar row, without exposing any causal
// proof object.
func (program *Program) AssignmentPredecessorID(write keyspace.Term) (id, route identity.ContentID, ok bool) {
	if !program.scalarIdentityAvailable() || write == 0 {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	view := program.Flow()
	finishTerm, finishOK := view.Ports().Finish(write)
	finish, finishSiteOK := view.Causal().Sites().ForTerm(finishTerm)
	successor, successorOK := view.Causal().Successors().AssignmentPredecessor(write)
	identityProof, identityOK := successor.Identity()
	route, routeOK := successor.SemanticID()
	if !finishOK || !finishSiteOK || !successorOK || !identityOK || !routeOK || !finish.Available() || !route.Available() ||
		successor.To != finishTerm || successor.Arm != flow.BoundaryLocal || identityProof.To() != finishTerm ||
		identityProof.Arm() != flow.BoundaryLocal || identityProof.Provenance() != view.Provenance() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	finishID := finish.ContextID()
	digest := identityProof.Digest()
	id = programRoleID("program/transformer/assignment-predecessor", program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Bytes(finishID[:]) == nil && writer.Bytes(route[:]) == nil && writer.Bytes(digest[:]) == nil
	})
	return id, route, id.Available()
}

// StorageWriteTransferIDAt returns the canonical identity of one fixed
// assignment write transfer. It follows the authored assignment range and
// reuses AssignmentPredecessorID for the sealed causal join.
func (program *Program) StorageWriteTransferIDAt(index, position int) (identity.ContentID, bool) {
	if position < 0 {
		return identity.ContentID{}, false
	}
	assignmentID, assignmentOK := program.StorageAssignmentIDAt(index)
	if !assignmentOK || !assignmentID.Available() {
		return identity.ContentID{}, false
	}
	view := program.Flow()
	assigns := view.Authored().Storage().Assigns()
	term, present := assigns.At(index)
	_, values, related := assigns.Get(term)
	width, widthOK := assigns.WriteCount(term)
	writeTerm, writeOK := assigns.WriteAt(term, position)
	actualAssignment, target, writeRelated := view.Authored().Storage().Writes().Get(writeTerm)
	_, fixed := view.Authored().Values().Member(values, position)
	if !present || !related || !widthOK || position >= width || !writeOK || !writeRelated || actualAssignment != term || !fixed {
		return identity.ContentID{}, false
	}
	if _, _, _, cellOK := view.Authored().Storage().Cells().Get(target); !cellOK {
		return identity.ContentID{}, false
	}
	// Assignment writes historically required only their Finish endpoint for
	// transfer identity.  Do not promote the full evaluation-span admission
	// rule here: writes can be authored with no Entry port while still having a
	// valid causal Finish site.
	finishTerm, finishOK := view.Ports().Finish(writeTerm)
	finish, finishOK := view.Causal().Sites().ForTerm(finishTerm)
	predecessorID, _, predecessorOK := program.AssignmentPredecessorID(writeTerm)
	if !finishOK || !finish.Available() || !predecessorOK || !predecessorID.Available() {
		return identity.ContentID{}, false
	}
	finishID := finish.ContextID()
	id := programRoleID("program/transformer/storage-write-transfer", program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Bytes(assignmentID[:]) == nil && writer.Uint(uint64(position)) == nil && writer.Bytes(finishID[:]) == nil &&
			writer.Bytes(predecessorID[:]) == nil
	})
	return id, id.Available()
}

// ValueSourceCount returns the authored denominator for one literal family or
// FamilyTypeValue. TypeValue includes dead candidates by design.
func (program *Program) ValueSourceCount(family keyspace.Family) int {
	if !program.scalarIdentityAvailable() {
		return 0
	}
	literals := program.Source().Literals()
	switch family {
	case keyspace.FamilyNil:
		return literals.Nils().Count()
	case keyspace.FamilyBool:
		return literals.Bools().Count()
	case keyspace.FamilyInteger:
		return literals.Integers().Count()
	case keyspace.FamilyFloat:
		return literals.Floats().Count()
	case keyspace.FamilyString:
		return literals.Strings().Count()
	case keyspace.FamilyTypeValue:
		return program.Flow().Authored().TypeValues().Count()
	default:
		return 0
	}
}

// ValueSourceIDAt returns (source identity, exact evaluation span identity,
// source term). It preserves the old ValueSourceOccurrence code: nil/bool/
// integer/float/string use codes 1..5 and TypeValue uses code 6.
func (program *Program) ValueSourceIDAt(family keyspace.Family, index int) (sourceID, spanID identity.ContentID, term keyspace.Term, ok bool) {
	if !program.scalarIdentityAvailable() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	var owner, target keyspace.Term
	literals := program.Source().Literals()
	switch family {
	case keyspace.FamilyNil:
		term, owner, ok = literals.Nils().At(index)
	case keyspace.FamilyBool:
		term, owner, _, ok = literals.Bools().At(index)
	case keyspace.FamilyInteger:
		term, owner, _, ok = literals.Integers().At(index)
	case keyspace.FamilyFloat:
		term, owner, _, ok = literals.Floats().At(index)
	case keyspace.FamilyString:
		term, owner, _, ok = literals.Strings().At(index)
	case keyspace.FamilyTypeValue:
		typeValues := program.Flow().Authored().TypeValues()
		term, ok = typeValues.At(index)
		if ok {
			owner, ok = typeValues.Get(term)
		}
		if ok {
			ok = program.Flow().Executable().Contains(term)
		}
		if ok {
			target, ok = program.Static().Operands().TypeValues().Target(term)
		}
		if ok {
			ref, refOK := program.Static().StaticTypes().Ref(target)
			ok = refOK && ref.Term() == target
		}
	default:
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	if !ok || term == 0 || owner == 0 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	bodyPath, bodyID, bodyOK := program.scalarBody(owner)
	spanID, direct, spanOK := program.valueSourceSpan(term)
	path, pathOK := program.Flow().ValueSourcePath(term)
	code := valueSourceCode(family)
	if !bodyOK || !spanOK || !pathOK || !path.Available() || code == 0 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	anchorID := programSemanticID("program/transformer/value-source-anchor", func(writer *framing.Writer) bool {
		return writer.Bool(direct) == nil && writer.Bytes(path[:]) == nil
	})
	if !anchorID.Available() {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	sourceID = programSemanticID("program/transformer/value-source-occurrence", func(writer *framing.Writer) bool {
		return writer.Uint(uint64(code)) == nil && writer.Bytes(bodyPath[:]) == nil &&
			writer.Bytes(bodyID[:]) == nil && writer.Bytes(anchorID[:]) == nil
	})
	return sourceID, spanID, term, sourceID.Available() && spanID.Available()
}

// valueSourceSpan chooses Source's direct span or its sealed lexical root,
// exactly once, matching the old ValueSourceAnchor rule.
func (program *Program) valueSourceSpan(term keyspace.Term) (identity.ContentID, bool, bool) {
	spanID, _, _, direct := program.EvaluationSpan(term)
	if direct {
		return spanID, true, spanID.Available()
	}
	if !program.scalarIdentityAvailable() {
		return identity.ContentID{}, false, false
	}
	root, rootOK := program.Source().Index().Root(term)
	if !rootOK || root == 0 {
		return identity.ContentID{}, false, false
	}
	spanID, _, _, rootOK = program.EvaluationSpan(root)
	return spanID, false, rootOK && spanID.Available()
}

func (program *Program) scalarIdentityAvailable() bool {
	if program == nil || program.source == nil || program.flow == nil || program.static == nil || program.module == nil || !program.id.Available() {
		return false
	}
	sourceID := program.source.Cold().ContentID()
	flowID := program.flow.ContentID()
	staticID := program.static.Cold().ContentID()
	moduleID := program.module.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return false
	}
	provenance := program.flow.View().Provenance()
	return provenance.Source == sourceID && provenance.Flow == flowID && provenance.Static == staticID && provenance.Module == moduleID
}

func (program *Program) scalarBody(owner keyspace.Term) (identity.ContentID, identity.ContentID, bool) {
	if !program.scalarIdentityAvailable() || owner == 0 {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	view := program.Flow()
	body, ok := view.FunctionBoundaries().ForBody(owner)
	if !ok || !body.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	path, pathOK := view.BodyPath(owner)
	context := body.ContextID()
	return path, context, pathOK && path.Available() && context.Available()
}

func valueSourceCode(family keyspace.Family) uint8 {
	if family == keyspace.FamilyTypeValue {
		return 6
	}
	if family >= keyspace.FamilyNil && family <= keyspace.FamilyString {
		return uint8(family)
	}
	return 0
}

func writeProgramTerm(writer *framing.Writer, term keyspace.Term) bool {
	return writer != nil && keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0 &&
		writer.Uint(uint64(keyspace.TermFamily(term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil
}
