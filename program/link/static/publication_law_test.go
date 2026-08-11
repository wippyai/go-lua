package static

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/semanticsource"
	"github.com/wippyai/go-lua/program/target"
)

func staticProgram(t testing.TB, name, text string) *programlower.Source {
	t.Helper()
	return &programlower.Source{Name: name, Text: []byte(text)}
}

func lowerStaticProgram(t testing.TB, name, text string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(*staticProgram(t, name, text))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDraftFinalizationFencesQueriesAndPreservesStaticFacets(t *testing.T) {
	provider := lowerStaticProgram(t, "provider.lua", `
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)
	consumer := lowerStaticProgram(t, "consumer.lua", `
local API = require("provider")
type Subject = API.Schema.User
`)
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	projectDraft, err := linkproject.Build(linkproject.Input{Target: contract, Modules: []linkproject.Module{
		{Name: "consumer-a", Program: consumer},
		{Name: "consumer-b", Program: consumer},
		{Name: "provider", Program: provider},
	}})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Project: project}
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	copiedDraft := *draft
	publications, ok := draft.Cold().Publications()
	if !ok || len(publications) != 5 {
		t.Fatalf("static facet publication = %d/%v", len(publications), ok)
	}
	want := map[semanticsource.Token]int{}
	for facet, count := range map[semanticsource.Facet]int{
		0:                                        3,
		semanticsource.FacetLinkStaticResolution: 2,
		semanticsource.FacetLinkStaticExpression: consumer.Static().StaticTypes().Count()*2 + provider.Static().StaticTypes().Count(),
		semanticsource.FacetLinkStaticExport:     1,
		semanticsource.FacetLinkStaticInput:      0,
	} {
		definition, found := semanticsource.Definition(semanticsource.OriginLinkStatic, facet)
		if !found {
			t.Fatalf("missing static facet %d", facet)
		}
		want[definition.Token()] = count
	}
	for _, publication := range publications {
		if got, exists := want[publication.Definition().Token()]; !exists || publication.Count() != got {
			t.Fatalf("unexpected static facet publication %#v", publication)
		}
	}
	wantContent := draft.Cold().ContentID()
	component, err := draft.Finalize()
	if err != nil || component == nil {
		t.Fatalf("finalize = %v/%p", err, component)
	}
	if !wantContent.Available() || component.Cold().ContentID() != wantContent {
		t.Fatal("Static content changed during finalization")
	}
	if _, err := draft.Finalize(); err == nil || draft.Cold().SchemaContentCount() != 0 {
		t.Fatal("draft issued authority after consuming finalization")
	}
	if _, err := copiedDraft.Finalize(); err == nil || copiedDraft.Cold().SchemaContentCount() != 0 {
		t.Fatal("copied draft issued authority after original finalization")
	}
	expressions := component.Expressions()
	firstExpression, firstExpressionOK := expressions.At(0)
	otherDraft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	other, err := otherDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if other.Cold().ContentID() != component.Cold().ContentID() {
		t.Fatal("Static content changed under exact replay")
	}
	if _, accepted := other.Expressions().Reference(firstExpression); !firstExpressionOK || accepted {
		t.Fatal("static expression crossed a finalized component owner fence")
	}
	if expressions.QualifiedCount() != 2 {
		t.Fatalf("qualified duplicate-mounted rows = %d", expressions.QualifiedCount())
	}
	firstConsumer, firstProvider, firstOK := expressions.QualifiedAt(0)
	secondConsumer, secondProvider, secondOK := expressions.QualifiedAt(1)
	firstShard, firstShardOK := expressions.Shard(firstConsumer)
	secondShard, secondShardOK := expressions.Shard(secondConsumer)
	firstReference, firstReferenceOK := expressions.Reference(firstProvider)
	secondReference, secondReferenceOK := expressions.Reference(secondProvider)
	if !firstOK || !secondOK || !firstShardOK || !secondShardOK || firstShard == secondShard ||
		!firstReferenceOK || !secondReferenceOK || firstReference != secondReference {
		t.Fatal("qualified enumeration lost exact duplicate-mounted consumer shard")
	}
}

func TestTypedSemanticSourceViewsReplayAndRetainEmptyFamilies(t *testing.T) {
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	projectDraft, err := linkproject.Build(linkproject.Input{Target: contract, Modules: []linkproject.Module{
		{Name: "main", Program: lowerStaticProgram(t, "main.lua", `return {}`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project})
	if err != nil {
		t.Fatal(err)
	}
	views, ok := draft.Cold().SemanticSourceViews()
	if !ok {
		t.Fatal("sealed Static has no typed semantic-source views")
	}
	if views.Static().Count() != 1 || views.Input().Count() != 0 {
		t.Fatalf("Static/Input view counts = %d/%d, want 1/0", views.Static().Count(), views.Input().Count())
	}
	inputCursor := views.Input().Cursor()
	if _, ok := inputCursor.Next(); ok {
		t.Fatal("empty StaticInput cursor yielded a row")
	}
	first, firstOK := views.Static().DigestAt(0)
	if !firstOK || !first.Available() {
		t.Fatal("Static view omitted its namespace digest")
	}
	replayed, replayOK := draft.Cold().SemanticSourceViews()
	if !replayOK {
		t.Fatal("replayed Static view unavailable")
	}
	second, secondOK := replayed.Static().DigestAt(0)
	if !secondOK || second != first {
		t.Fatal("Static view replay changed its canonical digest")
	}
}
