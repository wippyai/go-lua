package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

// StaticOperandKind is the closed disposition vocabulary copied into a
// ProgramArtifact static-input row. It is deliberately separate from Link's
// Boundary value algebra: Link later supplies the mounted value for a
// RuntimeSubject through its scalar identity.
type StaticOperandKind uint8

const (
	StaticOperandInvalid StaticOperandKind = iota
	StaticOperandKnown
	StaticOperandRuntimeSubject
	StaticOperandTypeValue
)

// StaticOperand is a seal-time proof of the exact scalar operand behind a
// TypeOf/annotation input. No authored term crosses this API.
type StaticOperand struct {
	kind      StaticOperandKind
	id        identity.ContentID
	literal   keyspace.LiteralValue
	reference identity.ContentID
	subject   identity.ContentID
	body      identity.ContentID
}

func (operand StaticOperand) Kind() StaticOperandKind         { return operand.kind }
func (operand StaticOperand) ID() identity.ContentID          { return operand.id }
func (operand StaticOperand) Literal() keyspace.LiteralValue  { return operand.literal }
func (operand StaticOperand) ReferenceID() identity.ContentID { return operand.reference }
func (operand StaticOperand) SubjectID() identity.ContentID   { return operand.subject }
func (operand StaticOperand) BodyPathID() identity.ContentID  { return operand.body }

// StaticOperandAt resolves one exact authored operand from the canonical
// Program owners. Claims are transparent, TypeValues retain their static
// target reference, literals retain their exact payload, and fixed-cell reads
// retain the parent-issued Cell identity.
func (program *Program) StaticOperandAt(term keyspace.Term) (StaticOperand, bool) {
	if program == nil || !program.ContentID().Available() || term == 0 {
		return StaticOperand{}, false
	}
	return program.staticOperandAt(term, make(map[keyspace.Term]struct{}))
}

func (program *Program) staticOperandAt(term keyspace.Term, seen map[keyspace.Term]struct{}) (StaticOperand, bool) {
	if _, duplicate := seen[term]; duplicate {
		return StaticOperand{}, false
	}
	seen[term] = struct{}{}
	literals := program.Source().Literals()
	if ordinal := keyspace.TermOrdinal(term); ordinal != 0 {
		switch keyspace.TermFamily(term) {
		case keyspace.FamilyNil:
			if value, _, ok := literals.Nils().At(int(ordinal - 1)); ok && value == term {
				id, _, source, sourceOK := program.ValueSourceIDAt(keyspace.FamilyNil, int(ordinal-1))
				return StaticOperand{kind: StaticOperandKnown, id: id, literal: keyspace.LiteralValue{}}, sourceOK && source == term
			}
		case keyspace.FamilyBool:
			if value, _, payload, ok := literals.Bools().At(int(ordinal - 1)); ok && value == term {
				id, _, source, sourceOK := program.ValueSourceIDAt(keyspace.FamilyBool, int(ordinal-1))
				return StaticOperand{kind: StaticOperandKnown, id: id, literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: payload}}, sourceOK && source == term
			}
		case keyspace.FamilyInteger:
			if value, _, payload, ok := literals.Integers().At(int(ordinal - 1)); ok && value == term {
				id, _, source, sourceOK := program.ValueSourceIDAt(keyspace.FamilyInteger, int(ordinal-1))
				return StaticOperand{kind: StaticOperandKnown, id: id, literal: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: payload}}, sourceOK && source == term
			}
		case keyspace.FamilyFloat:
			if value, _, payload, ok := literals.Floats().At(int(ordinal - 1)); ok && value == term {
				id, _, source, sourceOK := program.ValueSourceIDAt(keyspace.FamilyFloat, int(ordinal-1))
				return StaticOperand{kind: StaticOperandKnown, id: id, literal: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: payload}}, sourceOK && source == term
			}
		case keyspace.FamilyString:
			if value, _, payload, ok := literals.Strings().At(int(ordinal - 1)); ok && value == term {
				id, _, source, sourceOK := program.ValueSourceIDAt(keyspace.FamilyString, int(ordinal-1))
				return StaticOperand{kind: StaticOperandKnown, id: id, literal: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: payload}}, sourceOK && source == term
			}
		}
	}
	claims := program.Flow().Authored().Claims()
	if owner, operand, _, ok := claims.Get(term); ok && owner != 0 && operand != 0 {
		return program.staticOperandAt(operand, seen)
	}
	typeValues := program.Flow().Authored().TypeValues()
	if owner, ok := typeValues.Get(term); ok && owner != 0 && program.Flow().Executable().Contains(term) {
		target, targetOK := program.Static().Operands().TypeValues().Target(term)
		ref, refOK := program.Static().StaticTypes().Ref(target)
		id, idOK := programstatic.TypeReferenceID(program.ContentID(), ref)
		if targetOK && refOK && ref.Term() == target && idOK {
			_, bodyOK := program.Flow().FunctionBoundaries().ForBody(owner)
			bodyPath, pathOK := program.Flow().BodyPath(owner)
			if bodyOK && pathOK {
				sourceID, _, source, sourceOK := program.ValueSourceIDAt(keyspace.FamilyTypeValue, int(keyspace.TermOrdinal(term)-1))
				return StaticOperand{kind: StaticOperandTypeValue, id: sourceID, reference: id, body: bodyPath}, sourceOK && source == term
			}
		}
		return StaticOperand{}, false
	}
	reads := program.Flow().Authored().Storage().Reads()
	if owner, source, _, ok := reads.Get(term); ok && owner != 0 && source != 0 && program.Flow().Executable().Contains(term) {
		readID, _, readTerm, readOK := program.StorageReadIDAt(int(keyspace.TermOrdinal(term) - 1))
		cellID, cellOK := program.StorageCellID(source)
		cellTerm, cellTermOK := program.Flow().Authored().Storage().Cells().At(int(keyspace.TermOrdinal(source) - 1))
		bodyPath, bodyOK := program.Flow().BodyPath(owner)
		if readOK && readTerm == term && cellOK && cellTermOK && cellTerm == source && bodyOK && bodyPath.Available() {
			return StaticOperand{kind: StaticOperandRuntimeSubject, id: readID, subject: cellID, body: bodyPath}, true
		}
	}
	return StaticOperand{}, false
}

// StaticFrontier returns Source's lexical frontier joined to Flow's stable
// Body path. It is a Program relation, not a transport row.
func (program *Program) StaticFrontier(term keyspace.Term) (identity.ContentID, uint32, bool) {
	if program == nil || term == 0 {
		return identity.ContentID{}, 0, false
	}
	bodyTerm, cursor, ok := program.Source().Index().Frontier(term)
	if !ok || cursor < 0 || uint64(cursor) > uint64(^uint32(0)) {
		return identity.ContentID{}, 0, false
	}
	bodyPath, bodyOK := program.Flow().BodyPath(bodyTerm)
	return bodyPath, uint32(cursor), bodyOK && bodyPath.Available()
}
