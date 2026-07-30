package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestSealRelationProgramWorldReleasesConsumedWorldProgram(t *testing.T) {
	graph := cfg.New()
	graph.AddEdge(graph.Entry(), graph.Exit(), false)
	plan := operationplan.New(graph, factflow.FactsInput{})
	prepared, err := NewPlanCompiler().Prepare(standard.Registry(), graph, plan, Shape{})
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.builder.arena.bindLexicalOwner(lexicalidentity.StableLexicalBodyID{1}) {
		t.Fatal("bind lexical owner")
	}
	if err := prepared.sealAmbientEnvironment(nil); err != nil {
		t.Fatal(err)
	}
	if err := prepared.freezeRelationProgramWorld(programCallSurface{}); err != nil {
		t.Fatal(err)
	}
	if !prepared.worldBase.valid(true) {
		t.Fatal("freeze did not retain the WorldProgram until reduction")
	}
	if err := prepared.reduceRelationProgramWorldUnsealed(); err != nil {
		t.Fatal(err)
	}
	if err := prepared.sealRelationProgramSyntax(); err != nil {
		t.Fatal(err)
	}
	if err := prepared.sealRelationProgramWorld(); err != nil {
		t.Fatal(err)
	}
	if prepared.worldBase.terms != nil || prepared.worldBase.effects != nil || prepared.worldBase.arena != nil ||
		prepared.worldBase.root != 0 || prepared.worldBase.publication.points != nil || prepared.worldBase.publication.edges != nil {
		t.Fatal("sealed compiler retained the consumed WorldProgram")
	}
	if prepared.codeBase == nil || prepared.rootBase == 0 {
		t.Fatal("releasing WorldProgram also released canonical relation code")
	}
}
