package program

// This file owns Program's value-source identities. Source literals and Flow
// TypeValues remain in their canonical owners; this root query only joins
// their existing rows to immutable evaluation geometry.

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// ValueSourceCount returns the authored denominator for one literal family or
// FamilyTypeValue. TypeValue includes dead candidates by design.
func (program *Program) ValueSourceCount(family keyspace.Family) int {
	if !program.Available() {
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
	if !program.Available() || index < 0 {
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
	bodyPath, bodyID, bodyOK := program.Flow().BodyContextIDs(owner)
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
	if !program.Available() {
		return identity.ContentID{}, false, false
	}
	root, rootOK := program.Source().Index().Root(term)
	if !rootOK || root == 0 {
		return identity.ContentID{}, false, false
	}
	entryTerm, entryOK := program.Flow().Ports().Entry(root)
	entry, entrySiteOK := program.Flow().Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := program.Flow().FinishSite(term)
	if !finishOK {
		finish, finishOK = program.Flow().FinishSite(root)
	}
	if !entryOK || !entrySiteOK || !finishOK || !entry.Available() || !finish.Available() {
		return identity.ContentID{}, false, false
	}
	spanID = evaluationSpanID(program, root, entry.ContextID(), finish.ContextID())
	return spanID, false, spanID.Available()
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
