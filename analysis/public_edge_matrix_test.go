package analysis_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/program/target/profile"
	"github.com/wippyai/go-lua/program/testfixture"
)

func TestPublicAnalyzeSemanticTypeEngineEdgeMatrix(t *testing.T) {
	const fixtureName = "semantic/type-engine-edge-matrix"
	project, err := testfixture.FrozenCorpusProject(fixtureName)
	if err != nil {
		t.Fatalf("fixture %s load: %v", fixtureName, err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatalf("fixture %s target profile: %v", fixtureName, err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatalf("fixture %s Link Seal: %v", fixtureName, err)
	}
	result, status := analysis.Analyze(context.Background(), linked)
	if status != analysis.AnalyzeComplete || result == nil || result.BodyCount() == 0 {
		t.Fatalf("fixture %s Analyze: status=%d result=%v bodies=%d", fixtureName, status, result != nil, result.BodyCount())
	}
}
