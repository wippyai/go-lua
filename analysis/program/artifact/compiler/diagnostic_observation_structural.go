package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
)

// copyStructuralMemberConformanceObservationsFailure descends one conformance
// site whose measured value an allocation establishes. Every constructor member
// is itself a value producer with its own occurrence, span, and evidence point,
// and the declaration it is measured against is the member's own node in the
// declared graph, so a member is the same kind of subject as the whole value:
// it reuses the conformance row and the site discriminator names where it sits.
//
// The walk is issuance only. It adds rows to the existing population; no
// judgment is taken here and no verdict is decided at this altitude.
func (compiler *compiler) copyStructuralMemberConformanceObservationsFailure(
	owner, measuredValue, declared identity.ContentID,
	measured keyspace.Term,
) bool {
	if compiler == nil || !compiler.input.Available() {
		return false
	}
	if !owner.Available() || !measuredValue.Available() || !declared.Available() {
		return false
	}
	if keyspace.TermFamily(measured) != keyspace.FamilyTable {
		return true
	}
	declaredTerm, declaredTermOK := compiler.staticTypeTermForID(declared)
	if !declaredTermOK {
		return true
	}
	// The published allocation tree is finite and a member is a distinct row,
	// so the walk terminates on structure alone; the visited set states the
	// bound rather than relying on it.
	visited := make(map[keyspace.Term]struct{}, 4)
	return compiler.admitStructuralMemberObservations(owner, measuredValue, measured, declaredTerm, visited)
}

// admitStructuralMemberObservations measures one allocation's established
// members against the declared node each member's key, index, or map position
// resolves to, and names the required declared fields the constructor's key set
// does not supply.
//
// A field the tail of an open constructor supplies is unknown here, so an open
// constructor contributes its established members and no absence: the tail may
// carry the key the declaration requires. Its known members are established
// whatever the tail holds, so they are measured exactly as a closed
// constructor's are.
func (compiler *compiler) admitStructuralMemberObservations(
	owner, measuredValue identity.ContentID,
	table keyspace.Term,
	declaredTerm keyspace.Term,
	visited map[keyspace.Term]struct{},
) bool {
	if compiler == nil || !owner.Available() || !measuredValue.Available() || table == 0 || visited == nil {
		return false
	}
	if _, seen := visited[table]; seen {
		return true
	}
	visited[table] = struct{}{}
	row, rowOK := compiler.allocationRowForTerm(table)
	if !rowOK {
		return true
	}
	declared, declaredOK := compiler.declaredStructuralTarget(declaredTerm)
	if !declaredOK {
		return true
	}
	established := make(map[keyspace.Key]struct{}, len(row.fields))
	openTail := false
	for index, field := range row.fields {
		if field.finalOpen {
			openTail = true
			continue
		}
		if field.width != 1 {
			continue
		}
		if field.normalizedOK {
			established[field.normalized] = struct{}{}
		}
		memberDeclared, memberDeclaredOK := compiler.declaredStructuralMember(declared, field.kind, field.normalized, field.normalizedOK)
		member, memberRowOK := compiler.valueMemberAt(field.valuesRow, 0)
		if !memberDeclaredOK || field.memberTerm == 0 || !memberRowOK || !member.Available() {
			continue
		}
		memberDeclaredID, memberDeclaredIDOK := compiler.staticTypeNodeIDForTerm(memberDeclared)
		if !memberDeclaredIDOK || !compiler.conformanceObservationAddressable(field.memberTerm) {
			continue
		}
		if !compiler.admitConformanceObservation(diagnosticTypeConformanceSiteMember, owner, member.ID(), memberDeclaredID, field.memberTerm, uint32(index)) {
			return false
		}
		if keyspace.TermFamily(field.memberTerm) != keyspace.FamilyTable {
			continue
		}
		if !compiler.admitStructuralMemberObservations(owner, member.ID(), field.memberTerm, memberDeclared, visited) {
			return false
		}
	}
	if openTail {
		return true
	}
	return compiler.admitAbsentMemberObservations(owner, measuredValue, table, declared, established)
}

// admitAbsentMemberObservations names each required declared field the
// constructor's established key set does not supply. An absence has no value of
// its own, so the row measures the allocation's own value against the missing
// field's node and reports at the constructor.
func (compiler *compiler) admitAbsentMemberObservations(
	owner, measuredValue identity.ContentID,
	table keyspace.Term,
	declared keyspace.Term,
	established map[keyspace.Key]struct{},
) bool {
	view := compiler.input.Static()
	records := view.Types().Records()
	fields := view.Types().Fields()
	_, declaredFieldCount, isRecord := records.Get(declared)
	if !isRecord {
		return true
	}
	for position := 0; position < declaredFieldCount; position++ {
		fieldTerm, fieldTermOK := records.FieldAt(declared, position)
		key, fieldType, optional, shapeOK := fields.Get(fieldTerm)
		if !fieldTermOK || !shapeOK || optional {
			continue
		}
		if _, present := established[key]; present {
			continue
		}
		fieldID, fieldIDOK := compiler.staticTypeNodeIDForTerm(fieldType)
		if !fieldIDOK || !compiler.conformanceObservationAddressable(table) {
			continue
		}
		if !compiler.admitConformanceObservation(diagnosticTypeConformanceSiteMemberAbsent, owner, measuredValue, fieldID, table, uint32(position)) {
			return false
		}
	}
	return true
}

// declaredStructuralMember resolves the declared node one constructor member is
// measured against. A record names it by the member's normalized key -- the
// same key arena the allocation geometry carries -- an array by its element,
// and a map by its value. A member the declaration does not name is not
// measured here: an unexpected key is a different judgment over the same rows.
func (compiler *compiler) declaredStructuralMember(
	declared keyspace.Term,
	kind flowkind.FieldKind,
	normalized keyspace.Key,
	normalizedOK bool,
) (keyspace.Term, bool) {
	view := compiler.input.Static()
	if element, _, isArray := view.Types().Arrays().Get(declared); isArray {
		if kind != flowkind.FieldList {
			return 0, false
		}
		return element, element != 0
	}
	if _, value, _, isMap := view.Types().Maps().Get(declared); isMap {
		if !normalizedOK {
			return 0, false
		}
		return value, value != 0
	}
	_, fieldCount, isRecord := view.Types().Records().Get(declared)
	if !isRecord || !normalizedOK {
		return 0, false
	}
	fields := view.Types().Fields()
	for position := 0; position < fieldCount; position++ {
		fieldTerm, fieldTermOK := view.Types().Records().FieldAt(declared, position)
		key, fieldType, _, shapeOK := fields.Get(fieldTerm)
		if !fieldTermOK || !shapeOK || key != normalized {
			continue
		}
		return fieldType, fieldType != 0
	}
	return 0, false
}

// declaredStructuralTarget unwraps the declaration wrappers that leave member
// structure unchanged: an alias names its target, a resolved reference names
// its referent, and an optional names its inner type -- a constructor is never
// nil, so an optional declaration measures its members against the inner type.
// The visited set bounds a declaration graph that reaches itself.
func (compiler *compiler) declaredStructuralTarget(term keyspace.Term) (keyspace.Term, bool) {
	view := compiler.input.Static()
	visited := make(map[keyspace.Term]struct{}, 4)
	for term != 0 {
		if _, seen := visited[term]; seen {
			return 0, false
		}
		visited[term] = struct{}{}
		if inner, isOptional := view.Types().Optionals().Get(term); isOptional {
			term = inner
			continue
		}
		if _, target, _, isReference := view.References().Get(term); isReference {
			term = target
			continue
		}
		if _, target, _, _, isAlias := view.Declarations().Aliases().Get(term); isAlias {
			term = target
			continue
		}
		return term, true
	}
	return 0, false
}

// staticTypeNodeIDForTerm issues the published node identity of one declared
// type term. It is the same equation the static graph publishes its nodes
// under, so a member site's declaration addresses the published column.
func (compiler *compiler) staticTypeNodeIDForTerm(term keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || term == 0 {
		return identity.ContentID{}, false
	}
	ref, refOK := compiler.input.Static().StaticTypes().Ref(term)
	if !refOK {
		return identity.ContentID{}, false
	}
	id, idOK := staticquery.TypeReferenceID(compiler.key.ProgramID(), ref)
	return id, idOK && id.Available()
}

// staticTypeTermForID inverts the node identity equation over the published
// type forest. The identity is a function of owner and term alone, so the
// inverse is a function; it is built once per program and read per site.
func (compiler *compiler) staticTypeTermForID(id identity.ContentID) (keyspace.Term, bool) {
	if compiler == nil || !id.Available() {
		return 0, false
	}
	if compiler.staticTypeTermsByID == nil {
		types := compiler.input.Static().StaticTypes()
		index := make(map[identity.ContentID]keyspace.Term, types.Count())
		for position := 0; position < types.Count(); position++ {
			ref, refOK := types.At(position)
			if !refOK {
				continue
			}
			nodeID, nodeOK := staticquery.TypeReferenceID(compiler.key.ProgramID(), ref)
			if !nodeOK || !nodeID.Available() {
				continue
			}
			index[nodeID] = ref.Term()
		}
		compiler.staticTypeTermsByID = index
	}
	term, found := compiler.staticTypeTermsByID[id]
	return term, found && term != 0
}

// conformanceObservationAddressable reports whether the measured term carries
// the span and base evidence a conformance row is minted from. A member the
// program establishes without addressing it is not a subject, so it contributes
// no row rather than a row anchored to nothing.
func (compiler *compiler) conformanceObservationAddressable(measured keyspace.Term) bool {
	if compiler == nil || measured == 0 {
		return false
	}
	location, locationOK := compiler.input.Source().Identity().Span(measured)
	span, spanOK := compiler.input.Span(measured)
	finish, finishOK := span.Finish()
	return locationOK && validDiagnosticSpan(location) && spanOK && compiler.input.OwnsSpan(span) &&
		finishOK && compiler.input.OwnsSite(finish) && len(compiler.pointIDs(finish)) != 0
}
