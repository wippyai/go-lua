package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

// StaticOperandAt resolves one exact authored operand from the canonical
// Program owners. Claims are transparent, TypeValues retain their static
// target reference, literals retain their exact payload, and fixed-cell reads
// retain the parent-issued Cell identity.
func (program *Program) StaticOperandAt(term keyspace.Term) (programstatic.StaticOperand, bool) {
	if program == nil || !program.Available() || term == 0 {
		return programstatic.StaticOperand{}, false
	}
	return program.Static().StaticOperandAt(term, programstatic.StaticOperandResolver{
		Literal:        program.staticLiteralOperand,
		Claim:          program.staticClaimOperand,
		TypeValue:      program.staticTypeValueOperand,
		RuntimeSubject: program.staticRuntimeSubjectOperand,
	})
}

func (program *Program) staticLiteralOperand(term keyspace.Term) (identity.ContentID, keyspace.LiteralValue, bool) {
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return identity.ContentID{}, keyspace.LiteralValue{}, false
	}
	index := int(ordinal - 1)
	literals := program.Source().Literals()
	var id identity.ContentID
	var literal keyspace.LiteralValue
	var source keyspace.Term
	var ok bool
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil:
		var issued keyspace.Term
		issued, _, ok = literals.Nils().At(index)
		source = issued
	case keyspace.FamilyBool:
		var issued keyspace.Term
		var value bool
		issued, _, value, ok = literals.Bools().At(index)
		literal = keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}
		source = issued
	case keyspace.FamilyInteger:
		var issued keyspace.Term
		var value int64
		issued, _, value, ok = literals.Integers().At(index)
		literal = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
		source = issued
	case keyspace.FamilyFloat:
		var issued keyspace.Term
		var value uint64
		issued, _, value, ok = literals.Floats().At(index)
		literal = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: value}
		source = issued
	case keyspace.FamilyString:
		var issued keyspace.Term
		var value string
		issued, _, value, ok = literals.Strings().At(index)
		if ok {
			literal = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
		}
		source = issued
	default:
		return identity.ContentID{}, keyspace.LiteralValue{}, false
	}
	if !ok || source != term {
		return identity.ContentID{}, keyspace.LiteralValue{}, false
	}
	id, _, issued, ok := program.ValueSourceIDAt(keyspace.TermFamily(term), index)
	return id, literal, ok && issued == term
}

func (program *Program) staticClaimOperand(term keyspace.Term) (keyspace.Term, bool) {
	owner, operand, _, ok := program.Flow().Authored().Claims().Get(term)
	return operand, ok && owner != 0 && operand != 0
}

func (program *Program) staticTypeValueOperand(term keyspace.Term) (identity.ContentID, identity.ContentID, identity.ContentID, bool) {
	typeValues := program.Flow().Authored().TypeValues()
	owner, ownerOK := typeValues.Get(term)
	if !ownerOK || owner == 0 || !program.Flow().Executable().Contains(term) {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	target, targetOK := program.Static().Operands().TypeValues().Target(term)
	ref, refOK := program.Static().StaticTypes().Ref(target)
	id, idOK := programstatic.TypeReferenceID(program.ContentID(), ref)
	if !targetOK || !refOK || ref.Term() != target || !idOK {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	if _, bodyOK := program.Flow().FunctionBoundaries().ForBody(owner); !bodyOK {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	bodyPath, pathOK := program.Flow().BodyPath(owner)
	sourceID, _, source, sourceOK := program.ValueSourceIDAt(keyspace.FamilyTypeValue, int(keyspace.TermOrdinal(term)-1))
	return sourceID, id, bodyPath, pathOK && sourceOK && source == term
}

func (program *Program) staticRuntimeSubjectOperand(term keyspace.Term) (identity.ContentID, identity.ContentID, identity.ContentID, bool) {
	reads := program.Flow().Authored().Storage().Reads()
	owner, source, _, ok := reads.Get(term)
	if !ok || owner == 0 || source == 0 || !program.Flow().Executable().Contains(term) {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	readID, _, readTerm, readOK := program.StorageReadIDAt(int(keyspace.TermOrdinal(term) - 1))
	cellID, cellOK := program.StorageCellID(source)
	cellTerm, cellTermOK := program.Flow().Authored().Storage().Cells().At(int(keyspace.TermOrdinal(source) - 1))
	bodyPath, bodyOK := program.Flow().BodyPath(owner)
	return readID, cellID, bodyPath, readOK && readTerm == term && cellOK && cellTermOK && cellTerm == source && bodyOK
}

// StaticFrontier returns Source's lexical frontier joined to Flow's stable
// Body path. It is a Program relation, not a transport row.
func (program *Program) StaticFrontier(term keyspace.Term) (identity.ContentID, uint32, bool) {
	if program == nil || !program.Available() || term == 0 {
		return identity.ContentID{}, 0, false
	}
	bodyTerm, cursor, ok := program.Source().Index().Frontier(term)
	if !ok || cursor < 0 || uint64(cursor) > uint64(^uint32(0)) {
		return identity.ContentID{}, 0, false
	}
	bodyPath, bodyOK := program.Flow().BodyPath(bodyTerm)
	return bodyPath, uint32(cursor), bodyOK && bodyPath.Available()
}
