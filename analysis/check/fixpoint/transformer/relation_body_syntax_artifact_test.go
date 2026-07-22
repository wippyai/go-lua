package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestFreezeRelationProgramExtractsSealedPerBodySyntaxArtifacts(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("relation-body-syntax-artifacts"))
	first := lexicalidentity.RootBody(namespace)
	second := lexicalidentity.FunctionBody(namespace, 1)
	firstUnit := formalTemplateFreezeUnit(t, first)
	secondUnit := formalTemplateFreezeUnit(t, second)
	topology := testAcyclicCallTopology(t, first, second)

	program, err := FreezeRelationProgram([]RelationProgramUnit{secondUnit, firstUnit}, topology)
	if err != nil {
		t.Fatal(err)
	}
	if len(program.syntax) != 2 || !program.syntax[0].valid() || !program.syntax[1].valid() {
		t.Fatalf("syntax artifacts = %#v, want two sealed body artifacts", program.syntax)
	}
	if program.syntax[0].relation.code == program.syntax[1].relation.code ||
		program.syntax[0].relation.arena == program.syntax[1].relation.arena ||
		program.syntax[0].relation.effects == program.syntax[1].relation.effects {
		t.Fatal("distinct body syntax artifacts share local syntax ownership")
	}
	beforeCode := program.syntax[0].relation.code
	beforeRoot := program.syntax[0].relation.root
	_ = program.bodies[0].definitionFrames
	if program.syntax[0].relation.code != beforeCode || program.syntax[0].relation.root != beforeRoot ||
		!program.syntax[0].relation.code.sealed || !program.syntax[0].relation.arena.Sealed() || !program.syntax[0].relation.effects.Sealed() {
		t.Fatal("post-artifact SCC/link workspace mutation reached local syntax")
	}
}
