package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

func TestQueryAdmissionDispatchesBySealedFamily(t *testing.T) {
	record := mountedRecord(t, "query-admission", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	if len(record.Artifacts) == 0 || !record.Artifacts[0].Available() {
		t.Fatal("sealed mount")
	}
	mount := record.Artifacts[0]
	program := mount.Snapshot.Program()
	catalog, catalogOK := programschema.CatalogID(program.SchemaID)
	pointCount, pointsPublished := programschema.PointFamily().Count(&program.Frozen, catalog)
	if !program.Available() || !catalogOK || !pointsPublished || pointCount == 0 {
		t.Fatal("fixture issued no sealed points")
	}
	point, pointOK := programschema.PointFamily().At(&program.Frozen, catalog, 0)
	pointID := point.ID()
	if !pointOK || !pointID.Available() {
		t.Fatal("sealed point")
	}
	summaryID, summaryIDOK := identity.DeriveContentID("analysis/artifact-query/v1", mount.ModuleKey[:], pointID[:], []byte(QueryFamilyValueSummary))
	exactID, exactIDOK := identity.DeriveContentID("analysis/artifact-query/v1", mount.ModuleKey[:], pointID[:], []byte(QueryFamilyEffectExact))
	if !summaryIDOK || !exactIDOK {
		t.Fatal("query identities")
	}
	summary, summaryOK := bound.QueryAdmission(summaryID, mount.ModuleKey, pointID, QueryFamilyValueSummary)
	exact, exactOK := bound.QueryAdmission(exactID, mount.ModuleKey, pointID, QueryFamilyEffectExact)
	if !summaryOK || !exactOK {
		t.Fatalf("query admission refused: summary=%v exact=%v", summaryOK, exactOK)
	}
	if summary.ID != summaryID || summary.Mount != mount.ModuleKey || summary.Point != pointID {
		t.Fatal("summary admission lost the sealed site")
	}
	if exact.ID != exactID || exact.Mount != mount.ModuleKey || exact.Point != pointID {
		t.Fatal("exact admission lost the sealed site")
	}
	if _, ok := bound.QueryAdmission(summaryID, mount.ModuleKey, pointID, ""); ok {
		t.Fatal("empty family admitted")
	}
}

func TestSelectedQuerySitesExcludeUncalledCallables(t *testing.T) {
	record := mountedRecord(t, "query-sites", `local function dormant(value)
  local retained = value
  return retained
end
return 42`)
	sites, ok := SelectedQuerySites(record.Artifacts)
	if !ok || len(sites) == 0 {
		t.Fatal("selected query sites")
	}
	callable := make(map[identity.ContentID]struct{})
	roots := make(map[identity.ContentID]struct{})
	for _, mount := range record.Artifacts {
		bodyCount, bodiesPublished := mount.Program.BodyCount()
		if !bodiesPublished {
			t.Fatal("cold Body family")
		}
		for index := 0; index < bodyCount; index++ {
			body, bodyOK := mount.Program.BodyAt(index)
			if !bodyOK {
				t.Fatal("sealed body")
			}
			if body.Callable() {
				callable[body.ID()] = struct{}{}
			} else {
				roots[body.ID()] = struct{}{}
			}
		}
	}
	callablePoints := make(map[identity.ContentID]struct{})
	rootPoints := make(map[identity.ContentID]struct{})
	for _, mount := range record.Artifacts {
		program := mount.Snapshot.Program()
		occurrenceCount, occurrencesPublished := program.OccurrenceCount()
		if !occurrencesPublished {
			t.Fatal("cold occurrence family")
		}
		for index := 0; index < occurrenceCount; index++ {
			occurrence, occurrenceOK := program.OccurrenceAt(index)
			body, bodyOK := occurrence.BodyID()
			if !occurrenceOK || !bodyOK {
				continue
			}
			_, pointCount, pointSpanOK := occurrence.PointSpan()
			for pointIndex := 0; pointIndex < int(pointCount); pointIndex++ {
				point, pointOK := program.OccurrencePointID(index, pointIndex)
				if !pointSpanOK || !pointOK {
					t.Fatal("sealed occurrence point")
				}
				if _, held := callable[body]; held {
					callablePoints[point] = struct{}{}
				}
				if _, held := roots[body]; held {
					rootPoints[point] = struct{}{}
				}
			}
		}
	}
	if len(callablePoints) == 0 || len(rootPoints) == 0 {
		t.Fatal("fixture issued no callable/root occurrence points")
	}
	perPoint := make(map[identity.ContentID]int)
	for _, site := range sites {
		if _, forbidden := callablePoints[site.Point]; forbidden {
			t.Fatal("uncalled callable interior became a query site")
		}
		if _, root := rootPoints[site.Point]; !root {
			t.Fatal("query site escaped the non-callable root")
		}
		perPoint[site.Point]++
	}
	for point, count := range perPoint {
		if count != 2 {
			t.Fatalf("root point %v query lanes = %d", point, count)
		}
	}
}

// TestSelectedQuerySitesAdmitOnlyDirectCalleeInteriors proves the selection
// rule at its owner: a direct call from a selected root admits the callee's
// sealed occurrence points, while an uncalled callable sibling remains out of
// the selected-point population.
func TestSelectedQuerySitesAdmitOnlyDirectCalleeInteriors(t *testing.T) {
	record := mountedRecord(t, "selected-callee", `local function dormant(value)
  local retained = value
  return retained
end
local function use(x)
  return x
end
return use(1)`)
	sites, ok := SelectedQuerySites(record.Artifacts)
	if !ok || len(sites) == 0 {
		t.Fatal("selected query sites")
	}
	points := selectedCallableOccurrencePoints(t, record.Artifacts[0].Program, record.Artifacts[0].Snapshot)
	callee, sibling := selectedDirectCalleeAndSibling(t, record.Artifacts[0].Program, record.Artifacts[0].Snapshot)
	if !callee.Available() || !sibling.Available() {
		t.Fatal("fixture lost the direct callee or its unused sibling")
	}
	if len(points[callee]) == 0 || len(points[sibling]) == 0 {
		t.Fatal("callable bodies published no occurrence points")
	}
	selected := make(map[identity.ContentID]int)
	for _, site := range sites {
		if _, forbidden := points[sibling][site.Point]; forbidden {
			t.Fatal("uncalled sibling became a query site")
		}
		if _, inside := points[callee][site.Point]; inside {
			selected[site.Point]++
		}
	}
	if len(selected) == 0 {
		t.Fatal("direct callee interior is not a query subject")
	}
	for point, count := range selected {
		if count != 2 {
			t.Fatalf("selected callee point %v query lanes = %d", point, count)
		}
	}
}

// A control-fault chunk is still a sealed Program root. Query admission keeps
// it in the selected population so diagnostic collection can name the fault
// instead of failing construction at the query boundary.
func TestSelectedQuerySitesAdmitControlFaultRoots(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		source string
	}{
		{name: "break-outside-loop", source: "break -- expect-error"},
		{name: "goto-backward", source: "::start::\ngoto start"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			record := mountedRecord(t, "query-control-fault-"+fixture.name, fixture.source)
			sites, ok := SelectedQuerySites(record.Artifacts)
			if !ok || len(sites) == 0 {
				t.Fatalf("control-fault root has no selected query sites: ok=%t rows=%d", ok, len(sites))
			}
		})
	}
}

func TestSelectedQuerySitesUseTheirOwnerAddressFormula(t *testing.T) {
	record := mountedRecord(t, "query-address", "return 42")
	sites, ok := SelectedQuerySites(record.Artifacts)
	if !ok || len(sites) == 0 {
		t.Fatal("selected query sites")
	}
	issued := make(map[schema.Key]struct{})
	for _, family := range QueryIssuance() {
		issued[family.Family] = struct{}{}
	}
	for index, site := range sites {
		if _, known := issued[site.Family]; !known {
			t.Fatalf("site %d carries unissued family %q", index, site.Family)
		}
		want, derived := identity.DeriveContentID(querySiteFormula, site.Mount[:], site.Point[:], []byte(site.Family))
		if !derived || site.ID != want {
			t.Fatalf("site %d address %v is not the owner formula over (%v, %v, %q)", index, site.ID, site.Mount, site.Point, site.Family)
		}
	}
}

func selectedCallableOccurrencePoints(t *testing.T, program programmount.Program, snapshot *ingress.Snapshot) map[identity.ContentID]map[identity.ContentID]struct{} {
	t.Helper()
	points := make(map[identity.ContentID]map[identity.ContentID]struct{})
	bodyCount, bodiesPublished := program.BodyCount()
	if !bodiesPublished {
		t.Fatal("cold Body family")
	}
	for index := 0; index < bodyCount; index++ {
		body, ok := program.BodyAt(index)
		if !ok || !body.Callable() {
			continue
		}
		points[body.ID()] = make(map[identity.ContentID]struct{})
	}
	canonical := snapshot.Program()
	occurrenceCount, occurrencesPublished := canonical.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("cold occurrence family")
	}
	for index := 0; index < occurrenceCount; index++ {
		occurrence, occurrenceOK := canonical.OccurrenceAt(index)
		body, bodyOK := occurrence.BodyID()
		if !occurrenceOK || !bodyOK {
			continue
		}
		held, callable := points[body]
		if !callable {
			continue
		}
		_, pointCount, pointSpanOK := occurrence.PointSpan()
		for pointIndex := 0; pointIndex < int(pointCount); pointIndex++ {
			point, pointOK := canonical.OccurrencePointID(index, pointIndex)
			if !pointSpanOK || !pointOK {
				t.Fatal("occurrence point")
			}
			held[point] = struct{}{}
		}
	}
	return points
}

func selectedDirectCalleeAndSibling(t *testing.T, program programmount.Program, snapshot *ingress.Snapshot) (identity.ContentID, identity.ContentID) {
	t.Helper()
	rootBodies := make(map[identity.ContentID]struct{})
	callable := make(map[identity.ContentID]struct{})
	bodyCount, bodiesPublished := program.BodyCount()
	if !bodiesPublished {
		t.Fatal("cold Body family")
	}
	for index := 0; index < bodyCount; index++ {
		body, ok := program.BodyAt(index)
		if !ok {
			t.Fatal("body row")
		}
		if body.Callable() {
			callable[body.ID()] = struct{}{}
		} else {
			rootBodies[body.ID()] = struct{}{}
		}
	}
	var callee identity.ContentID
	for index := 0; index < snapshot.CallCount(); index++ {
		call, callOK := snapshot.CallAt(index)
		target, targetOK := call.DirectTargetBody()
		if !callOK || !targetOK {
			continue
		}
		if _, root := rootBodies[call.BodyID()]; !root {
			continue
		}
		if _, known := callable[target]; !known {
			t.Fatal("direct target is not a callable body")
		}
		callee = target
		break
	}
	var sibling identity.ContentID
	for body := range callable {
		if body != callee {
			sibling = body
			break
		}
	}
	return callee, sibling
}
