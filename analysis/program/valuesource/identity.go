// Package valuesource owns the live Program join for literal and TypeValue
// source identities. It is shared by Artifact construction and Link boundary
// sealing without depending on either consumer.
package valuesource

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Count returns the authored denominator for one ValueSource family.
func Count(input *program.Program, family keyspace.Family) int {
	if input == nil || !input.Available() {
		return 0
	}
	literals := input.Source().Literals()
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
		return input.Flow().Authored().TypeValues().Count()
	default:
		return 0
	}
}

// IdentityAt proves Source, Flow, and Static ownership for one literal or
// TypeValue source then issues its exact source and span identities. Derived
// spans are fenced by the live Program's canonical ContentID.
func IdentityAt(input *program.Program, family keyspace.Family, index int) (sourceID, spanID identity.ContentID, term keyspace.Term, ok bool) {
	if input == nil || !input.Available() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	programID := input.ContentID()
	if !programID.Available() {
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
	spanID, direct, spanOK := span(input, programID, term)
	path, pathOK := view.SemanticTermPath(term)
	code, codeOK := valueSourceCode(family)
	if !bodyOK || !bodyPath.Available() || !bodyID.Available() || !spanOK || !pathOK || !path.Available() || !codeOK {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	anchorID, anchorOK := valueSourceAnchorIdentity(direct, path)
	if !anchorOK {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	sourceID, sourceOK := valueSourceIdentity(code, bodyPath, bodyID, anchorID)
	return sourceID, spanID, term, sourceOK && sourceID.Available() && spanID.Available()
}

func span(input *program.Program, programID identity.ContentID, term keyspace.Term) (identity.ContentID, bool, bool) {
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
	spanID, spanOK := valueSourceSpanIdentity(programID, root, entry.ContextID(), finish.ContextID())
	return spanID, false, spanOK && spanID.Available()
}
