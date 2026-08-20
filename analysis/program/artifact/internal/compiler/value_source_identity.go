package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// ValueSourceIdentityAt is the Artifact construction admission for a
// literal or TypeValue source. It proves Source/Flow/Static ownership while
// the Program is live, then delegates the two owner-neutral source equations
// and lexical-root span fallback to schema/program.
func ValueSourceIdentityAt(input *program.Program, programID identity.ContentID, family keyspace.Family, index int) (sourceID, spanID identity.ContentID, term keyspace.Term, ok bool) {
	if input == nil || !input.Available() || !programID.Available() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	view := input.Flow()
	var owner, target keyspace.Term
	literals := input.Source().Literals()
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
		typeValues := view.Authored().TypeValues()
		term, ok = typeValues.At(index)
		if ok {
			owner, ok = typeValues.Get(term)
		}
		if ok {
			ok = view.Executable().Contains(term)
		}
		if ok {
			target, ok = input.Static().Operands().TypeValues().Target(term)
		}
		if ok {
			ref, refOK := input.Static().StaticTypes().Ref(target)
			ok = refOK && ref.Term() == target
		}
	default:
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	if !ok || term == 0 || owner == 0 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	bodyPath, bodyID, bodyOK := view.BodyContextIDs(owner)
	spanID, direct, spanOK := valueSourceSpan(input, programID, term)
	path, pathOK := view.ValueSourcePath(term)
	code, codeOK := programschema.ValueSourceCode(family)
	if !bodyOK || !bodyPath.Available() || !bodyID.Available() || !spanOK || !pathOK || !path.Available() || !codeOK {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	anchorID, anchorOK := programschema.ValueSourceAnchorIdentity(direct, path)
	if !anchorOK {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	sourceID, sourceOK := programschema.ValueSourceIdentity(code, bodyPath, bodyID, anchorID)
	return sourceID, spanID, term, sourceOK && sourceID.Available() && spanID.Available()
}

func valueSourceSpan(input *program.Program, programID identity.ContentID, term keyspace.Term) (identity.ContentID, bool, bool) {
	spanID, _, _, direct := input.EvaluationSpan(term)
	if direct {
		return spanID, true, spanID.Available()
	}
	root, rootOK := input.Source().Index().Root(term)
	if !rootOK || root == 0 {
		return identity.ContentID{}, false, false
	}
	entryTerm, entryOK := input.Flow().Ports().Entry(root)
	entry, entrySiteOK := input.Flow().Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := input.Flow().FinishSite(term)
	if !finishOK {
		finish, finishOK = input.Flow().FinishSite(root)
	}
	if !entryOK || !entrySiteOK || !finishOK || !entry.Available() || !finish.Available() {
		return identity.ContentID{}, false, false
	}
	spanID, spanOK := programschema.ValueSourceSpanIdentity(programID, root, entry.ContextID(), finish.ContextID())
	return spanID, false, spanOK && spanID.Available()
}

func SourceLiteral(input *program.Program, term keyspace.Term) (keyspace.Family, keyspace.LiteralValue, bool) {
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	index := int(ordinal - 1)
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil:
		issued, _, ok := input.Source().Literals().Nils().At(index)
		return keyspace.FamilyNil, keyspace.LiteralValue{}, ok && issued == term
	case keyspace.FamilyBool:
		issued, _, value, ok := input.Source().Literals().Bools().At(index)
		return keyspace.FamilyBool, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}, ok && issued == term
	case keyspace.FamilyInteger:
		issued, _, value, ok := input.Source().Literals().Integers().At(index)
		return keyspace.FamilyInteger, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, ok && issued == term
	case keyspace.FamilyFloat:
		issued, _, value, ok := input.Source().Literals().Floats().At(index)
		return keyspace.FamilyFloat, keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: value}, ok && issued == term
	case keyspace.FamilyString:
		issued, _, value, ok := input.Source().Literals().Strings().At(index)
		return keyspace.FamilyString, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, ok && issued == term
	default:
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
}
