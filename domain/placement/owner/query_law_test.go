package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	placementquery "github.com/wippyai/go-lua/domain/placement/query"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
)

func placementQueryGeometryTypes(t *testing.T) structure.GeometryTypes {
	t.Helper()
	entries, entriesOK := structure.Collect(structure.RelationGeometrySpecs())
	if !entriesOK {
		t.Fatal("canonical relation geometry entries")
	}
	types, typesOK := structure.RelationGeometryTypes(entries)
	if !typesOK {
		t.Fatal("canonical relation geometry types")
	}
	return types
}

func placementQueryResultTypes(t *testing.T) placementquery.SummaryResultTypes {
	t.Helper()
	var ownerContent identity.ContentID
	ownerContent[0] = 0xf1
	owner, ownerOK := model.IssueOwnerID(ownerContent)
	issue := func(seed byte) model.TypeID {
		var content identity.ContentID
		content[0] = seed
		value, ok := model.IssueTypeID(owner, content)
		if !ok {
			t.Fatalf("summary result type %d", seed)
		}
		return value
	}
	if !ownerOK {
		t.Fatal("summary result owner")
	}
	return placementquery.SummaryResultTypes{
		AllocationID: issue(1),
		Fact:         issue(2),
		Evidence:     issue(3),
		SchemaID:     issue(4),
	}
}

func TestPlacementSummaryQuerySpecOwnsCanonicalFamilyAndGeometry(t *testing.T) {
	spec := placementquery.QuerySpec(placementQueryGeometryTypes(t), placementQueryResultTypes(t))
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

func TestPlacementSummaryQueryDeclarationIsExactAndGeometryRequired(t *testing.T) {
	types := placementQueryGeometryTypes(t)
	resultTypes := placementQueryResultTypes(t)
	spec := placementquery.QuerySpec(types, resultTypes)
	if !spec.Declaration.Available() {
		t.Fatal("placement-summary declaration unavailable")
	}
	relations := spec.Declaration.Relations()
	columns := spec.Declaration.Columns()
	keys := spec.Declaration.Keys()
	scopes := spec.Declaration.Scopes()
	denominators := spec.Declaration.Denominators()
	if len(relations) != 2 || len(columns) != 9 || len(keys) != 2 || len(scopes) != 2 || len(denominators) != 2 {
		t.Fatalf("authority shape relations/columns/keys/scopes/denominators = %d/%d/%d/%d/%d, want 2/9/2/2/2",
			len(relations), len(columns), len(keys), len(scopes), len(denominators))
	}
	if columns[0].Type != types.Site || columns[1].Type != types.Mount || columns[2].Type != types.Context || columns[3].Type != types.Point {
		t.Fatal("Q columns do not preserve geometry authority")
	}
	if columns[4].Name != "placement-schema-id" || columns[4].Relation != "Q" || columns[4].Type != resultTypes.SchemaID ||
		columns[5].Name != "allocation-site-id" || columns[5].Relation != "QAllocation" || columns[5].Type != types.Site ||
		columns[6].Name != "allocation-id" || columns[6].Type != resultTypes.AllocationID ||
		columns[7].Name != "placement-fact" || columns[7].Type != resultTypes.Fact ||
		columns[8].Name != "allocation-evidence" || columns[8].Type != resultTypes.Evidence {
		t.Fatalf("Placement summary result columns = %+v", columns[4:])
	}
	if relations[1].Name != "QAllocation" || relations[1].Scope != "QAllocationScope" || relations[1].Publication != "site-allocation-key" ||
		len(relations[1].Addressing) != 2 || keys[1].Name != "site-allocation-key" || len(keys[1].Columns) != 2 ||
		keys[1].Columns[0] != "allocation-site-id" || keys[1].Columns[1] != "allocation-id" ||
		scopes[1].Name != "QAllocationScope" || len(scopes[1].Dimensions) != 1 || scopes[1].Dimensions[0] != "allocation-site-id" ||
		denominators[1].Name != "Q/allocation" || denominators[1].Relation != "QAllocation" || denominators[1].Key != "site-allocation-key" {
		t.Fatalf("Placement summary allocation authority mismatch relation=%+v key=%+v scope=%+v denominator=%+v", relations[1], keys[1], scopes[1], denominators[1])
	}
	changed := types
	changed.Site, changed.Mount = changed.Mount, changed.Site
	changedSpec := placementquery.QuerySpec(changed, resultTypes)
	if !changedSpec.Declaration.Available() || changedSpec.Declaration.Digest() == spec.Declaration.Digest() {
		t.Fatal("placement-summary declaration digest ignored geometry change")
	}
	if unavailable := placementquery.QuerySpec(structure.GeometryTypes{}, resultTypes); unavailable.Declaration.Available() || unavailable.Family.Available() {
		t.Fatal("placement-summary query accepted unavailable geometry")
	}
	if unavailable := placementquery.QuerySpec(types, placementquery.SummaryResultTypes{}); unavailable.Declaration.Available() || unavailable.Family.Available() {
		t.Fatal("placement-summary query accepted unavailable result types")
	}
	colliding := resultTypes
	colliding.AllocationID = types.Site
	if duplicate := placementquery.QuerySpec(types, colliding); duplicate.Declaration.Available() || duplicate.Family.Available() {
		t.Fatal("placement-summary query accepted a payload TypeID aliased to geometry")
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
		Semantic:   semantic,
		Freezer:    freezer,
		Population: query.PopulationKindSelectedPoint,
		Subjects:   coldSubjects,
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
		Semantic:   semantic,
		Freezer:    freezer,
		Population: query.PopulationKindSelectedPoint,
		Subjects:   query.NewSubjects(nil),
	}); declaredOK || declared != nil {
		t.Fatal("placement query admitted without its declared subject")
	}
}
