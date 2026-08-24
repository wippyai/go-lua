package rows

import (
	"crypto/sha256"
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/seal"
)

// TestArtifactScalarTemplateConsumesPrivateBuilderStorageExactlyOnce pins the
// storage transfer: sealing moves the builder's row planes into the template
// rather than copying them, and every handle that shares the builder state is
// closed by that single transfer.
func TestArtifactScalarTemplateConsumesPrivateBuilderStorageExactlyOnce(t *testing.T) {
	spec, copyOfHandle := artifactScalarLawSpec(t)
	pointStorage := &spec.state.Points[0]
	memberStorage := &spec.state.Regions[0].Members[0]
	entryStorage := &spec.state.Bodies[0].Entry[0]
	template, templateOK := NewArtifactScalarTemplate(spec)
	if !templateOK || template == nil {
		t.Fatal("artifact scalar template")
	}
	if &template.points[0] != pointStorage || &template.regions[0].Members[0] != memberStorage || &template.bodies[0].Entry[0] != entryStorage {
		t.Fatal("artifact scalar template copied builder storage")
	}
	if _, ok := copyOfHandle.AddPoint(ArtifactScalarPoint{ID: artifactScalarLawID(7)}); ok || copyOfHandle.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: artifactScalarLawID(4)}) {
		t.Fatal("copied builder handle remained mutable after consumption")
	}
	if second, ok := NewArtifactScalarTemplate(copyOfHandle); ok || second != nil {
		t.Fatal("artifact scalar builder was consumed twice")
	}
}

// artifactScalarLawSpec fills one admissible builder and returns it together
// with a copied handle that shares the same private state.
func TestArtifactScalarTemplateAdmitsRoutedNonNativeRule(t *testing.T) {
	routeFrom, base := artifactScalarLawID(4), artifactScalarLawID(7)
	spec := artifactScalarRoutedRuleSpec(t, routeFrom, base)
	template, ok := NewArtifactScalarTemplate(spec)
	if !ok || template == nil || !template.Available() {
		t.Fatal("local predecessor stage must seal")
	}
	if rule, rowOK := template.RuleAt(0); !rowOK || rule.Native {
		t.Fatalf("local rule receipt=(%+v,%v), want non-native", rule, rowOK)
	}
}

// TestArtifactScalarTemplateRefusesRoutePointThatTheRouteDoesNotReach is the
// nearest negative for the independent route proof. The data input remains a
// valid point; only the claimed route landing is false.
func TestArtifactScalarTemplateRefusesRoutePointThatTheRouteDoesNotReach(t *testing.T) {
	routeFrom := artifactScalarLawID(4)
	spec := artifactScalarRoutedRuleSpec(t, routeFrom, routeFrom)
	if template, ok := NewArtifactScalarTemplate(spec); ok || template != nil {
		t.Fatal("routed rule admitted a route point that its edge does not reach")
	}
}

func artifactScalarRoutedRuleSpec(t testing.TB, input, routePoint identity.ContentID) *ArtifactScalarSpec {
	t.Helper()
	routeFrom, base, stage := artifactScalarLawID(4), artifactScalarLawID(7), artifactScalarLawID(8)
	regionID, bodyID := artifactScalarLawID(5), artifactScalarLawID(6)
	spec, specOK := NewArtifactScalarSpec(artifactScalarLawID(1), artifactScalarLawID(2), artifactScalarLawID(3), ArtifactScalarCapacity{
		Roles: 1, Points: 3, Edges: 1, Regions: 1, Events: 5, Rules: 1, Bodies: 1,
	})
	if !specOK || spec == nil {
		t.Fatal("local predecessor builder")
	}
	_, fromOK := spec.AddPoint(ArtifactScalarPoint{ID: routeFrom, Initial: true})
	_, baseOK := spec.AddPoint(ArtifactScalarPoint{ID: base})
	_, stageOK := spec.AddPoint(ArtifactScalarPoint{ID: stage})
	region, regionOK := spec.AddRegion(ArtifactScalarRegion{ID: regionID, Head: routeFrom, Cyclic: true})
	body, bodyOK := spec.AddBody(ArtifactScalarBody{ID: bodyID})
	if !fromOK || !baseOK || !stageOK || !regionOK || !bodyOK ||
		!spec.AddRegionMember(region, routeFrom) || !spec.AddRegionMember(region, base) || !spec.AddRegionMember(region, stage) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventEnter, Region: regionID}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: routeFrom}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: base}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: stage}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventExit, Region: regionID}) ||
		!spec.AddBodyEntry(body, routeFrom) || !spec.AddBodyExit(body, stage) {
		t.Fatal("local predecessor geometry")
	}
	role, roleOK := spec.DeclareRole(artifactScalarLawID(20))
	if !roleOK || !spec.AddRule(ArtifactScalarRule{
		Role:       role,
		Stage:      schema.Key("test-stage/non-native"),
		Point:      stage,
		Inputs:     [6]identity.ContentID{input},
		InputCount: 1,
		ID:         artifactScalarLawID(21),
		Route:      artifactScalarLawID(22),
		RoutePoint: routePoint,
	}) {
		t.Fatal("local predecessor rule")
	}
	if _, edgeOK := spec.AddEdge(ArtifactScalarEdge{
		ID:    artifactScalarLawID(23),
		From:  routeFrom,
		To:    base,
		Route: artifactScalarLawID(22),
		Arm:   ArtifactStructuralArmLocal,
	}); !edgeOK {
		t.Fatal("predecessor route")
	}
	return spec
}

func TestArtifactScalarTemplateSealsIssuedNativeReceipt(t *testing.T) {
	spec := artifactScalarTwoPointRegion(t)
	role, roleOK := spec.DeclareRole(artifactScalarLawID(20))
	if !roleOK || !spec.AddRule(ArtifactScalarRule{
		Role:   role,
		Stage:  programissuance.StageCallDispatch,
		Point:  artifactScalarLawID(7),
		Inputs: [6]identity.ContentID{artifactScalarLawID(4)}, InputCount: 1,
		ID:     artifactScalarLawID(21),
		Native: true,
	}) {
		t.Fatal("native rule")
	}
	installScalarLawStageTable(t, spec)
	template, templateOK := NewArtifactScalarTemplate(spec)
	if !templateOK || template == nil {
		t.Fatal("native template")
	}
	if spec.state.stagesSet || len(spec.state.stages.Entries(schemaissuance.KindStage)) != 0 {
		t.Fatal("sealed template retained the schema stage table")
	}
	if rule, rowOK := template.RuleAt(0); !rowOK || !rule.Native {
		t.Fatalf("native rule receipt=(%+v,%v), want native", rule, rowOK)
	}
}

func TestArtifactScalarTemplateRefusesNativeRowWithoutStageTable(t *testing.T) {
	spec := artifactScalarTwoPointRegion(t)
	role, roleOK := spec.DeclareRole(artifactScalarLawID(20))
	if !roleOK || !spec.AddRule(ArtifactScalarRule{
		Role: role, Stage: programissuance.StageCallDispatch,
		Point: artifactScalarLawID(7), Inputs: [6]identity.ContentID{artifactScalarLawID(4)}, InputCount: 1,
		ID: artifactScalarLawID(21), Native: true,
	}) {
		t.Fatal("native rule fixture")
	}
	if template, ok := NewArtifactScalarTemplate(spec); ok || template != nil {
		t.Fatal("native row without the canonical stage table was admitted")
	}
}

func TestArtifactScalarTemplateRejectsNativeBackEdge(t *testing.T) {
	spec := artifactScalarTwoPointRegion(t)
	role, roleOK := spec.DeclareRole(artifactScalarLawID(20))
	if !roleOK || !spec.AddRule(ArtifactScalarRule{
		Role:   role,
		Stage:  programissuance.StageCallDispatch,
		Point:  artifactScalarLawID(4),
		Inputs: [6]identity.ContentID{artifactScalarLawID(7)}, InputCount: 1,
		ID:     artifactScalarLawID(21),
		Native: true,
	}) {
		t.Fatal("native back-edge rule")
	}
	installScalarLawStageTable(t, spec)
	if template, ok := NewArtifactScalarTemplate(spec); ok || template != nil {
		t.Fatal("native back-edge must stay refused")
	}
}

type scalarLawEmptySurface struct{ kind schema.SurfaceKind }

func (surface scalarLawEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (scalarLawEmptySurface) Entries() []schema.Entry          { return nil }
func (scalarLawEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func installScalarLawStageTable(t testing.TB, spec *ArtifactScalarSpec) {
	t.Helper()
	entries, entriesOK := programissuance.Entries()
	if !entriesOK {
		t.Fatal("Program issuance entries")
	}
	builder := seal.NewBuilder()
	builder.Register(scalarLawEmptySurface{schema.SurfaceKindStructure})
	builder.Register(scalarLawEmptySurface{schema.SurfaceKindAxis})
	builder.Register(schemaissuance.NewSurface(entries))
	for kind := schema.SurfaceKindRule; kind <= schema.SurfaceKindObservation; kind++ {
		builder.Register(scalarLawEmptySurface{kind})
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil || !sealed.Available() {
		t.Fatal("Program issuance seal")
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindIssuance)
	table, tableOK := schemaissuance.NewTable(view)
	if !viewOK || !tableOK || !spec.InstallStageTable(table) {
		t.Fatal("Program stage table installation")
	}
}

func artifactScalarTwoPointRegion(t testing.TB) *ArtifactScalarSpec {
	t.Helper()
	first, second, regionID, bodyID := artifactScalarLawID(4), artifactScalarLawID(7), artifactScalarLawID(5), artifactScalarLawID(6)
	spec, ok := NewArtifactScalarSpec(artifactScalarLawID(1), artifactScalarLawID(2), artifactScalarLawID(3), ArtifactScalarCapacity{
		Roles: 1, Points: 2, Edges: 1, Regions: 1, Events: 4, Rules: 1, Bodies: 1,
	})
	if !ok || spec == nil {
		t.Fatal("two-point builder")
	}
	if _, ok := spec.AddPoint(ArtifactScalarPoint{ID: first, Initial: true}); !ok {
		t.Fatal("first point")
	}
	if _, ok := spec.AddPoint(ArtifactScalarPoint{ID: second}); !ok {
		t.Fatal("second point")
	}
	region, regionOK := spec.AddRegion(ArtifactScalarRegion{ID: regionID, Head: first, Cyclic: true})
	body, bodyOK := spec.AddBody(ArtifactScalarBody{ID: bodyID})
	if !regionOK || !bodyOK ||
		!spec.AddRegionMember(region, first) || !spec.AddRegionMember(region, second) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventEnter, Region: regionID}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: first}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: second}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventExit, Region: regionID}) ||
		!spec.AddBodyEntry(body, first) || !spec.AddBodyExit(body, second) {
		t.Fatal("two-point region")
	}
	return spec
}

func artifactScalarLawSpec(t testing.TB) (*ArtifactScalarSpec, *ArtifactScalarSpec) {
	t.Helper()
	artifactID, program, schema := artifactScalarLawID(1), artifactScalarLawID(2), artifactScalarLawID(3)
	pointID, regionID, bodyID := artifactScalarLawID(4), artifactScalarLawID(5), artifactScalarLawID(6)
	spec, ok := NewArtifactScalarSpec(artifactID, program, schema, ArtifactScalarCapacity{Points: 1, Regions: 1, Events: 3, Bodies: 1})
	if !ok || spec == nil {
		t.Fatal("artifact scalar builder")
	}
	handle := *spec
	copyOfHandle := &handle
	point, pointOK := spec.AddPoint(ArtifactScalarPoint{ID: pointID, Initial: true})
	region, regionOK := copyOfHandle.AddRegion(ArtifactScalarRegion{ID: regionID, Head: pointID})
	body, bodyOK := spec.AddBody(ArtifactScalarBody{ID: bodyID})
	if !pointOK || point != 0 || !regionOK || region != 0 || !bodyOK || body != 0 ||
		!spec.AddRegionMember(region, pointID) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventEnter, Region: regionID}) ||
		!copyOfHandle.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: pointID}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventExit, Region: regionID}) ||
		!spec.AddBodyEntry(body, pointID) || !spec.AddBodyExit(body, pointID) {
		t.Fatal("artifact scalar rows")
	}
	return spec, copyOfHandle
}

func artifactScalarLawID(value byte) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte{0xA5, value}))
}

// TestScalarRowsDoNotNameDomainCallStages is the rows floor: the issuance
// schema owns stage keys and this package interprets only their declared laws.
func TestScalarRowsDoNotNameDomainCallStages(t *testing.T) {
	src, err := os.ReadFile("scalar_rows.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, name := range []string{"CallDispatch", "CallSummary", "CallEffect"} {
		if strings.Contains(body, name) {
			t.Errorf("rows names domain Call stage %s", name)
		}
	}
}
