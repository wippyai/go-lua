package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	placementquery "github.com/wippyai/go-lua/domain/placement/query"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
)

func TestPlacementSummaryQuerySpecOwnsCanonicalFamilyAndGeometry(t *testing.T) {
	spec := placementquery.QuerySpec()
	if spec.Family != placementdomain.SummaryResultFamily {
		t.Fatalf("family = %q, want %q", spec.Family, placementdomain.SummaryResultFamily)
	}
	if spec.Semantic != "semantic/query/placement-summary" || spec.Codec != "semantic/query-result/placement-summary" {
		t.Fatalf("query identities = %q/%q", spec.Semantic, spec.Codec)
	}
	if spec.Fold != query.FoldGeneral || spec.Contract != "semantic/query/placement-summary" {
		t.Fatalf("fold contract = %q/%q", spec.Fold, spec.Contract)
	}
	if len(spec.Subjects) != 3 || spec.Subjects[0] != "placement" || spec.Subjects[1] != "heap" || spec.Subjects[2] != "placement-suspension-evidence" {
		t.Fatalf("subjects = %v, want [placement heap placement-suspension-evidence]", spec.Subjects)
	}
	if spec.Population != query.PopulationSelectedPoint || spec.Projection != query.ProjectionExact {
		t.Fatalf("geometry = %q/%q", spec.Population, spec.Projection)
	}
}

func TestPlacementSummaryQueryDeclaresAndBindsItsHeterogeneousForm(t *testing.T) {
	heapSchema := placementHeapFixture(t)
	placementSchema, ok := placementdomain.NewSchema(heapSchema)
	if !ok {
		t.Fatal("placement schema")
	}

	builder := engine.NewSchema()
	factor, factorOK := vocabulary.Key("factor/placement")
	fold, foldOK := vocabulary.Key("factor/placement/summary-coordinatewise")
	heapFactor, heapFactorOK := vocabulary.Key("factor/heap")
	heapSummary, heapSummaryOK := vocabulary.Key("factor/heap/summary-complete")
	evidenceFactor, evidenceFactorOK := vocabulary.Key("factor/placement/suspension-evidence")
	evidenceSummary, evidenceSummaryOK := vocabulary.Key("factor/placement/suspension-evidence/summary")
	semantic, semanticOK := vocabulary.Key("query/placement-summary")
	freezer, freezerOK := vocabulary.Key("query-result/placement-summary")
	if !factorOK || !foldOK || !heapFactorOK || !heapSummaryOK || !evidenceFactorOK || !evidenceSummaryOK || !semanticOK || !freezerOK {
		t.Fatal("placement heterogeneous query roles")
	}
	fragment, fragmentOK := placementowner.DeclareSchema(builder, factor, fold)
	if !fragmentOK || fragment == nil {
		t.Fatal("placement factor declaration")
	}
	heapFragment, heapFragmentOK := heapowner.DeclareSchema(builder, heapFactor, heapSummary)
	if !heapFragmentOK || heapFragment == nil {
		t.Fatal("heap factor declaration")
	}
	evidenceFragment, evidenceFragmentOK := placementsuspension.DeclareEvidenceFactorSchema(builder, evidenceFactor, evidenceSummary)
	if !evidenceFragmentOK || evidenceFragment == nil {
		t.Fatal("suspension evidence factor declaration")
	}
	coldSubjects := query.NewSubjects(map[schema.Key]axis.Cell{
		"placement":                     axis.NewCell(fragment),
		"heap":                          axis.NewCell(heapFragment),
		"placement-suspension-evidence": axis.NewCell(evidenceFragment),
	})
	queryFragment, queryOK := placementquery.DeclareQuery(builder, query.Declaration{
		Semantic: semantic,
		Freezer:  freezer,
		Subjects: coldSubjects,
	})
	if !queryOK || queryFragment == nil || !queryFragment.Available() {
		t.Fatal("placement query declaration")
	}
	sealed, sealOK := builder.Seal()
	if !sealOK || sealed == nil {
		t.Fatal("placement query schema")
	}
	binding := engine.NewSchemaBinding(sealed)
	heapOwner, heapOwnerOK := heapowner.BindHot(binding, heapFragment, heapSchema)
	if !heapOwnerOK {
		t.Fatal("heap hot owner")
	}
	owner, ownerOK := placementowner.BindHot(binding, fragment, placementSchema)
	if !ownerOK {
		t.Fatal("placement hot owner")
	}
	evidenceOwner, evidenceOwnerOK := placementsuspension.BindEvidenceFactorHot(binding, evidenceFragment, placementSchema)
	if !evidenceOwnerOK {
		t.Fatal("suspension evidence hot owner")
	}
	hotSubjects := query.NewSubjects(map[schema.Key]axis.Cell{
		"placement":                     axis.NewBoundCell(owner),
		"heap":                          axis.NewBoundCell(heapOwner),
		"placement-suspension-evidence": axis.NewBoundCell(evidenceOwner),
	})
	if !placementquery.BindQuery(binding, query.Binding[*placementquery.SummaryQueryFragment]{
		Fragment: queryFragment,
		Subjects: hotSubjects,
	}) {
		t.Fatal("placement query binding")
	}
	if !binding.Seal() {
		t.Fatal("placement query binding seal")
	}
	if implementation, recovered := placementquery.RecoverQuery(binding, query.Sealed[*placementquery.SummaryQueryFragment]{Fragment: queryFragment}); !recovered || implementation == nil {
		t.Fatal("placement query recovery")
	}
}

func TestPlacementSummaryQueryRequiresItsPlacementAndHeapSubjects(t *testing.T) {
	builder := engine.NewSchema()
	factor, factorOK := vocabulary.Key("factor/placement")
	fold, foldOK := vocabulary.Key("factor/placement/summary-coordinatewise")
	heapFactor, heapFactorOK := vocabulary.Key("factor/heap")
	heapSummary, heapSummaryOK := vocabulary.Key("factor/heap/summary-complete")
	evidenceFactor, evidenceFactorOK := vocabulary.Key("factor/placement/suspension-evidence")
	evidenceSummary, evidenceSummaryOK := vocabulary.Key("factor/placement/suspension-evidence/summary")
	semantic, semanticOK := vocabulary.Key("query/placement-summary")
	freezer, freezerOK := vocabulary.Key("query-result/placement-summary")
	if !factorOK || !foldOK || !heapFactorOK || !heapSummaryOK || !evidenceFactorOK || !evidenceSummaryOK || !semanticOK || !freezerOK {
		t.Fatal("placement heterogeneous query roles")
	}
	fragment, fragmentOK := placementowner.DeclareSchema(builder, factor, fold)
	if !fragmentOK || fragment == nil {
		t.Fatal("placement factor declaration")
	}
	heapFragment, heapFragmentOK := heapowner.DeclareSchema(builder, heapFactor, heapSummary)
	if !heapFragmentOK || heapFragment == nil {
		t.Fatal("heap factor declaration")
	}
	evidenceFragment, evidenceFragmentOK := placementsuspension.DeclareEvidenceFactorSchema(builder, evidenceFactor, evidenceSummary)
	if !evidenceFragmentOK || evidenceFragment == nil {
		t.Fatal("suspension evidence factor declaration")
	}
	if declared, declaredOK := placementquery.DeclareQuery(builder, query.Declaration{
		Semantic: semantic,
		Freezer:  freezer,
		Subjects: query.NewSubjects(nil),
	}); declaredOK || declared != nil {
		t.Fatal("placement query admitted without its declared subject")
	}
}
