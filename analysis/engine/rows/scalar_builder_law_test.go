package rows

import (
	"crypto/sha256"
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
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
func TestArtifactScalarTemplateAdmitsLocalPredecessorStage(t *testing.T) {
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
		Role:  role,
		Stage: ArtifactRuleStageLocal,
		Point: stage,
		Input: base,
		ID:    artifactScalarLawID(21),
		Route: artifactScalarLawID(22),
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
	template, ok := NewArtifactScalarTemplate(spec)
	if !ok || template == nil || !template.Available() {
		t.Fatal("local predecessor stage must seal")
	}
	if native, rowOK := template.RuleNativeAt(0); !rowOK || native {
		t.Fatalf("local rule native projection=(%v,%v), want (false,true)", native, rowOK)
	}
}

func TestArtifactScalarTemplateSealsDeclaredNativeProjection(t *testing.T) {
	spec := artifactScalarTwoPointRegion(t)
	if !spec.InstallStageLaws(artifactCallStageLaws()) {
		t.Fatal("call stage laws")
	}
	role, roleOK := spec.DeclareRole(artifactScalarLawID(20))
	if !roleOK || !spec.AddRule(ArtifactScalarRule{
		Role:  role,
		Stage: ArtifactRuleStageIssued3,
		Point: artifactScalarLawID(7),
		Input: artifactScalarLawID(4),
		ID:    artifactScalarLawID(21),
	}) {
		t.Fatal("native rule")
	}
	template, templateOK := NewArtifactScalarTemplate(spec)
	if !templateOK || template == nil {
		t.Fatal("native template")
	}
	if native, rowOK := template.RuleNativeAt(0); !rowOK || !native {
		t.Fatalf("native rule projection=(%v,%v), want (true,true)", native, rowOK)
	}
	if _, rowOK := template.RuleNativeAt(1); rowOK {
		t.Fatal("out-of-range native projection")
	}
}

func TestArtifactScalarTemplateRejectsCallStageBackEdge(t *testing.T) {
	spec := artifactScalarTwoPointRegion(t)
	if !spec.InstallStageLaws(artifactCallStageLaws()) {
		t.Fatal("call stage laws")
	}
	role, roleOK := spec.DeclareRole(artifactScalarLawID(20))
	if !roleOK || !spec.AddRule(ArtifactScalarRule{
		Role:  role,
		Stage: ArtifactRuleStageIssued3,
		Point: artifactScalarLawID(4),
		Input: artifactScalarLawID(7),
		ID:    artifactScalarLawID(21),
	}) {
		t.Fatal("call back-edge rule")
	}
	if template, ok := NewArtifactScalarTemplate(spec); ok || template != nil {
		t.Fatal("call-stage back-edge must stay refused")
	}
}

func TestArtifactScalarTemplateEnforcesDeclaredStagePredecessor(t *testing.T) {
	spec := artifactScalarTwoPointRegion(t)
	if !spec.InstallStageLaws(artifactCallStageLaws()) {
		t.Fatal("call stage laws")
	}
	role, roleOK := spec.DeclareRole(artifactScalarLawID(20))
	if !roleOK || !spec.AddRule(ArtifactScalarRule{
		Role:  role,
		Stage: ArtifactRuleStageIssued4,
		Point: artifactScalarLawID(7),
		Input: artifactScalarLawID(4),
		ID:    artifactScalarLawID(21),
	}) {
		t.Fatal("summary without dispatch predecessor")
	}
	if template, ok := NewArtifactScalarTemplate(spec); ok || template != nil {
		t.Fatal("summary stage must require the declared dispatch predecessor")
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

func artifactCallStageLaws() []ArtifactStageLaw {
	return []ArtifactStageLaw{
		{Stage: ArtifactRuleStageIssued3, Native: true},
		{Stage: ArtifactRuleStageIssued4, Native: true, Predecessor: ArtifactRuleStageIssued3},
		{Stage: ArtifactRuleStageIssued5, Native: true, Predecessor: ArtifactRuleStageIssued4},
	}
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

// TestScalarRowsDoNotNameDomainCallStages is the rows floor: schema structure
// owns Call spelling. This package carries opaque issued ordinals.
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
