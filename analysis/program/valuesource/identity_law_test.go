package valuesource

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func valueSourceLawProgram(t *testing.T) *program.Program {
	t.Helper()
	input, err := lower.Lower(lower.Source{Name: "value-source-law.lua", Text: []byte(`
type Shape = { field: number }
local typed: Shape = { field = 1 }
local n = nil
local b = true
local i = 7
local f = 1.5
local s = "value"
return Shape(typed), number(i), n, b, i, f, s
`)})
	if err != nil {
		t.Fatal(err)
	}
	if input == nil || !input.Available() {
		t.Fatal("lowered Program unavailable")
	}
	return input
}

func TestCountAndIdentityAtMatchTheCanonicalValueSourcePreimages(t *testing.T) {
	input := valueSourceLawProgram(t)
	families := []keyspace.Family{
		keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyTypeValue,
	}
	for _, family := range families {
		count := Count(input, family)
		if count == 0 {
			t.Fatalf("available ValueSource family %d has an empty denominator", family)
		}
		for index := 0; index < count; index++ {
			sourceID, spanID, term, ok := IdentityAt(input, family, index)
			if !ok || !sourceID.Available() || !spanID.Available() || term == 0 {
				t.Fatalf("IdentityAt(%d, %d) = %v/%v/%08x/%v", family, index, sourceID, spanID, uint32(term), ok)
			}
			owner := valueSourceOwner(t, input, family, index, term)
			bodyPath, bodyID, bodyOK := input.Flow().BodyContextIDs(owner)
			path, pathOK := input.Flow().SemanticTermPath(term)
			code, codeOK := valueSourceCode(family)
			_, _, _, direct := input.EvaluationSpan(term)
			anchorID, anchorOK := valueSourceAnchorIdentity(direct, path)
			wantSource, sourceOK := valueSourceIdentity(code, bodyPath, bodyID, anchorID)
			if !bodyOK || !pathOK || !codeOK || !anchorOK || !sourceOK || sourceID != wantSource {
				t.Fatalf("source preimage drift for family %d index %d", family, index)
			}
			if direct {
				wantSpan, _, _, spanOK := input.EvaluationSpan(term)
				if !spanOK || spanID != wantSpan {
					t.Fatalf("direct span drift for family %d index %d", family, index)
				}
				continue
			}
			root, rootOK := input.Source().Index().Root(term)
			entryTerm, entryOK := input.Flow().Ports().Entry(root)
			entry, entrySiteOK := input.Flow().Causal().Sites().ForTerm(entryTerm)
			finish, finishOK := input.Flow().FinishSite(term)
			if !finishOK {
				finish, finishOK = input.Flow().FinishSite(root)
			}
			wantSpan, spanOK := valueSourceSpanIdentity(input.ContentID(), root, entry.ContextID(), finish.ContextID())
			if !rootOK || !entryOK || !entrySiteOK || !finishOK || !spanOK || spanID != wantSpan {
				t.Fatalf("fallback span preimage drift for family %d index %d", family, index)
			}
		}
	}
}

func TestCountAndIdentityAtFailClosed(t *testing.T) {
	input := valueSourceLawProgram(t)
	if Count(nil, keyspace.FamilyString) != 0 || Count(input, keyspace.FamilyInvalid) != 0 {
		t.Fatal("invalid ValueSource denominator was admitted")
	}
	if source, span, term, ok := IdentityAt(nil, keyspace.FamilyString, 0); ok || source.Available() || span.Available() || term != 0 {
		t.Fatalf("nil IdentityAt = %v/%v/%08x/%v", source, span, uint32(term), ok)
	}
	for _, test := range []struct {
		family keyspace.Family
		index  int
	}{
		{keyspace.FamilyInvalid, 0},
		{keyspace.FamilyString, -1},
		{keyspace.FamilyString, Count(input, keyspace.FamilyString)},
	} {
		if source, span, term, ok := IdentityAt(input, test.family, test.index); ok || source.Available() || span.Available() || term != 0 {
			t.Fatalf("invalid IdentityAt(%d, %d) = %v/%v/%08x/%v", test.family, test.index, source, span, uint32(term), ok)
		}
	}
}

func valueSourceOwner(t *testing.T, input *program.Program, family keyspace.Family, index int, want keyspace.Term) keyspace.Term {
	t.Helper()
	literals := input.Source().Literals()
	var term, owner keyspace.Term
	var ok bool
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
		term, ok = input.Flow().Authored().TypeValues().At(index)
		if ok {
			owner, ok = input.Flow().Authored().TypeValues().Get(term)
		}
	}
	if !ok || term != want || owner == 0 {
		t.Fatalf("ValueSource owner unavailable for family %d index %d", family, index)
	}
	return owner
}
