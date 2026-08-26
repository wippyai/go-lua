package suspension_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	targetcompiler "github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	suspension "github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestEvidenceEmptyHeapKeepsZeroWidthFactorAndSummary(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "placement-suspension-evidence-empty-law.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	target, err := targetcompiler.Seal(&declaration.Spec{Semantics: typecontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "placement-suspension-evidence-empty-law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	grammarOK := grammar.Available()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	artifact, artifactFailure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if !receiptOK || !grammarOK || !issuanceOK || artifactFailure.Available() || artifact == nil {
		t.Fatalf("empty suspension evidence artifact grammar=%t failure=%v artifact=%v", grammarOK, artifactFailure, artifact)
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural, structuralOK := composite.StructureVocabulary(receipt)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !shardOK || !moduleOK || !programIDOK || !structuralOK || !lowered || !mountOK {
		t.Fatalf("empty suspension evidence mount shard=%t module=%t program=%t structural=%t lowered=%t mount=%t", shardOK, moduleOK, programIDOK, structuralOK, lowered, mountOK)
	}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	if heapFailure != heapdomain.SealFailureNone || !placementOK || placementSchema.KeyCount() != 0 {
		t.Fatalf("empty suspension evidence authority heap=%v placement=%t keys=%d", heapFailure, placementOK, placementSchema.KeyCount())
	}

	builder := engine.NewSchema()
	semantic, semanticOK := vocabulary.Key("factor/placement-suspension-evidence")
	fold, foldOK := vocabulary.Key("factor/placement-suspension-evidence/summary-coordinatewise")
	fragment, declared := suspension.DeclareEvidenceFactorSchema(builder, semantic, fold)
	sealed, sealedOK := builder.Seal()
	if !semanticOK || !foldOK || !declared || !sealedOK {
		t.Fatal("empty suspension evidence declaration")
	}
	binding := engine.NewSchemaBinding(sealed)
	owner, bound := suspension.BindEvidenceFactorHot(binding, fragment, placementSchema)
	if !bound {
		t.Fatal("empty suspension evidence binding")
	}
	spec, specOK := owner.FactorSpec()
	if !specOK || spec.KeyEnd != 0 || spec.AdmitAt(0, suspension.EvidenceMissing) {
		t.Fatalf("empty suspension evidence width=%d/%t phantom=%t", spec.KeyEnd, specOK, spec.AdmitAt(0, suspension.EvidenceMissing))
	}

	observation := placementdomain.BeginPlacementSummary(placementSchema)
	observed, accumulated := suspension.AccumulatePlacementSummarySuspensionRows(placementSchema, observation, 0, absentEvidenceAt)
	if !accumulated || !placementdomain.EqualPlacementSummary(placementSchema, observed, observation) {
		t.Fatal("empty suspension evidence summary did not preserve the empty Placement observation")
	}
}

// absentEvidenceAt is the row accessor of a delivery that answers no
// coordinate. It stands for the vector a Factor of zero width delivers and for
// the vector a missing predecessor delivers alike: both state a width, and
// neither states a row.
func absentEvidenceAt(int) (suspension.Evidence, bool, bool) {
	var none suspension.Evidence
	return none, false, false
}
