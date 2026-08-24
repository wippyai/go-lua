package composite

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func TestQueryAdmissionDispatchesBySealedFamily(t *testing.T) {
	record := mountedRecord(t, "query-admission", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	if len(record.Artifacts) == 0 || !record.Artifacts[0].Available() {
		t.Fatal("sealed mount")
	}
	mount := record.Artifacts[0]
	program := mount.Snapshot.Program()
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	pointCount, pointsPublished := programschema.PointFamily().Count(&program.Frozen, catalog)
	if !program.Available() || !catalogOK || !pointsPublished || pointCount == 0 {
		t.Fatal("fixture issued no sealed points")
	}
	point, pointOK := programschema.PointFamily().At(&program.Frozen, catalog, 0)
	pointID := point.ID()
	if !pointOK || !pointID.Available() {
		t.Fatal("sealed point")
	}
	context := queryContextForMount(t, record.Source.ContextDirectory(), mount.ModuleKey)
	contextID := context.ID()
	summaryID, summaryIDOK := identity.DeriveContentID(querySiteFormula, mount.ModuleKey[:], contextID[:], pointID[:], []byte(QueryFamilyValueSummary))
	exactID, exactIDOK := identity.DeriveContentID(querySiteFormula, mount.ModuleKey[:], contextID[:], pointID[:], []byte(QueryFamilyEffectExact))
	placementID, placementIDOK := identity.DeriveContentID(querySiteFormula, mount.ModuleKey[:], contextID[:], pointID[:], []byte(QueryFamilyPlacementSummary))
	if !summaryIDOK || !exactIDOK || !placementIDOK {
		t.Fatal("query identities")
	}
	summary, summaryOK := bound.QueryAdmission(summaryID, mount.ModuleKey, pointID, QueryFamilyValueSummary, context)
	exact, exactOK := bound.QueryAdmission(exactID, mount.ModuleKey, pointID, QueryFamilyEffectExact, context)
	placement, placementOK := bound.QueryAdmission(placementID, mount.ModuleKey, pointID, QueryFamilyPlacementSummary, context)
	if !summaryOK || !exactOK || !placementOK {
		t.Fatalf("query admission refused: summary=%v exact=%v placement=%v", summaryOK, exactOK, placementOK)
	}
	if summary.ID != summaryID || summary.Mount != mount.ModuleKey || summary.Point != pointID {
		t.Fatal("summary admission lost the sealed site")
	}
	if summary.Context.ID() != context.ID() || exact.Context.ID() != context.ID() || placement.Context.ID() != context.ID() {
		t.Fatal("query admission lost the sealed context")
	}
	if exact.ID != exactID || exact.Mount != mount.ModuleKey || exact.Point != pointID {
		t.Fatal("exact admission lost the sealed site")
	}
	if placement.ID != placementID || placement.Mount != mount.ModuleKey || placement.Point != pointID {
		t.Fatal("placement admission lost the sealed site")
	}
	if bound.PlacementQuery() == nil {
		t.Fatal("sealed Placement query implementation is unavailable")
	}
	if _, ok := bound.QueryAdmission(summaryID, mount.ModuleKey, pointID, "", context); ok {
		t.Fatal("empty family admitted")
	}
}

// This is also the explicit-empty owner law: a published root Return with no
// directly returned Function remains a valid query population.
func TestSelectedQuerySitesExcludeUncalledCallables(t *testing.T) {
	record := mountedRecord(t, "query-sites", `local function dormant(value)
  local retained = value
  return retained
end
return 42`)
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	table, ok := SelectedQuerySites(compilation, record.Artifacts, record.Source.ContextDirectory())
	if !ok || table.Count() == 0 {
		t.Fatal("selected query sites")
	}
	program := record.Artifacts[0].Program
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	rootFunctionCount, rootFunctionsPublished := programschema.ModuleEntryRootFunctionFamily().Count(&program.Frozen, catalog)
	if !program.Available() || !catalogOK || !rootFunctionsPublished || rootFunctionCount != 0 {
		t.Fatalf("explicit-empty root-function owner = count %d/published %t", rootFunctionCount, rootFunctionsPublished)
	}
	selectedFamilies, selectedFamiliesOK := selectedPointQueryIssuance(compilation.catalog)
	if !selectedFamiliesOK {
		t.Fatal("selected-point query issuance")
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
	for index := 0; index < table.Count(); index++ {
		site, siteOK := table.At(index)
		if !siteOK {
			t.Fatalf("selected query site %d is unavailable", index)
		}
		if _, forbidden := callablePoints[site.Point]; forbidden {
			t.Fatal("uncalled callable interior became a query site")
		}
		perPoint[site.Point]++
	}
	// Occurrence spans own authored points. The selected table also closes
	// those points over their canonical WTO regions so declaration-framed
	// stage cuts are queryable; those synthetic points are not an escape into
	// the dormant callable. Every authored root point must still be present.
	for point := range rootPoints {
		if count := perPoint[point]; count != len(selectedFamilies) {
			t.Fatalf("authored root point %v query lanes = %d, want %d", point, count, len(selectedFamilies))
		}
	}
	for point, count := range perPoint {
		if count != len(selectedFamilies) {
			t.Fatalf("selected root-region point %v query lanes = %d, want %d", point, count, len(selectedFamilies))
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
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	table, ok := SelectedQuerySites(compilation, record.Artifacts, record.Source.ContextDirectory())
	if !ok || table.Count() == 0 {
		t.Fatal("selected query sites")
	}
	selectedFamilies, selectedFamiliesOK := selectedPointQueryIssuance(compilation.catalog)
	if !selectedFamiliesOK {
		t.Fatal("selected-point query issuance")
	}
	points := selectedCallableOccurrencePoints(t, record.Artifacts[0].Program)
	callee, sibling := selectedDirectCalleeAndSibling(t, record.Artifacts[0].Program)
	if !callee.Available() || !sibling.Available() {
		t.Fatal("fixture lost the direct callee or its unused sibling")
	}
	if len(points[callee]) == 0 || len(points[sibling]) == 0 {
		t.Fatal("callable bodies published no occurrence points")
	}
	selected := make(map[identity.ContentID]int)
	for index := 0; index < table.Count(); index++ {
		site, siteOK := table.At(index)
		if !siteOK {
			t.Fatalf("selected query site %d is unavailable", index)
		}
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
		if count != len(selectedFamilies) {
			t.Fatalf("selected callee point %v query lanes = %d, want %d", point, count, len(selectedFamilies))
		}
	}
}

// TestSelectedQuerySitesAdmitReturnedCallableInteriors proves that an
// executable callable returned by the module root is selected even when no
// Call row names it. The root-return owner row is the only ingress evidence;
// TestSelectedQuerySitesExcludeUncalledCallables is the paired negative law.
func TestSelectedQuerySitesAdmitReturnedCallableInteriors(t *testing.T) {
	record := mountedRecord(t, "returned-callable", `local function dormant(value)
  local retained = value
  return retained
end
return function(value)
  local retained = value
  return retained
end`)
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	table, ok := SelectedQuerySites(compilation, record.Artifacts, record.Source.ContextDirectory())
	if !ok || table.Count() == 0 {
		t.Fatal("selected query sites")
	}
	selectedFamilies, selectedFamiliesOK := selectedPointQueryIssuance(compilation.catalog)
	if !selectedFamiliesOK {
		t.Fatal("selected-point query issuance")
	}
	points := selectedCallableOccurrencePoints(t, record.Artifacts[0].Program)
	if len(points) == 0 {
		t.Fatal("returned callable published no occurrence points")
	}
	returnedBody, unreturnedBody := returnedAndUnreturnedCallableBodies(t, record.Artifacts[0].Program)
	returnedPoints := points[returnedBody]
	unreturnedPoints := points[unreturnedBody]
	if len(returnedPoints) == 0 || len(unreturnedPoints) == 0 {
		t.Fatal("returned/unreturned callable published no occurrence points")
	}
	selected := make(map[identity.ContentID]int)
	for index := 0; index < table.Count(); index++ {
		site, siteOK := table.At(index)
		if !siteOK {
			t.Fatalf("selected query site %d is unavailable", index)
		}
		if _, forbidden := unreturnedPoints[site.Point]; forbidden {
			t.Fatal("nested/unreturned callable became a query site")
		}
		if _, inside := returnedPoints[site.Point]; inside {
			selected[site.Point]++
		}
	}
	if len(selected) == 0 {
		t.Fatal("returned callable interior is not a query subject")
	}
	for point := range returnedPoints {
		count := selected[point]
		if count != len(selectedFamilies) {
			t.Fatalf("returned callable point %v query lanes = %d, want %d", point, count, len(selectedFamilies))
		}
	}
}

// Missing cold root-function publication and a malformed mounted owner both
// refuse admission; neither may be treated as an empty returned-callable set.
func TestSelectedQuerySitesRejectUnavailableOrMalformedRootFunctionOwner(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		mutate func(testing.TB, *programmount.MountedArtifact)
	}{
		{
			name: "unavailable-root-function-family",
			mutate: func(_ testing.TB, mount *programmount.MountedArtifact) {
				mount.Program.Frozen = snapshot.Frozen{}
			},
		},
		{
			name: "malformed-root-function-position",
			mutate: func(t testing.TB, mount *programmount.MountedArtifact) {
				program := mount.Program
				entry, entryOK := program.ModuleEntryAt(0)
				rootWidth, widthOK := entry.RootWidth()
				rootOffset, rootCount, spanOK := entry.RootFunctionSpan()
				if !entryOK || !widthOK || !spanOK || rootCount == 0 {
					t.Fatal("root-function fixture span")
				}
				corruptRootFunctionPosition(t, mount, int(rootOffset), rootWidth)
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			record := mountedRecord(t, "returned-callable-owner-"+fixture.name, "return function(value) return value end")
			compilation, compilationOK := Build()
			if !compilationOK {
				t.Fatal("compilation unavailable")
			}
			mounts := append([]programmount.MountedArtifact(nil), record.Artifacts...)
			if len(mounts) != 1 {
				t.Fatal("single mount fixture")
			}
			fixture.mutate(t, &mounts[0])
			if fixture.name == "malformed-root-function-position" && !mounts[0].Available() {
				t.Fatal("malformed sidecar fixture failed mount preflight")
			}
			if fixture.name == "unavailable-root-function-family" {
				program := mounts[0].Program
				catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
				if _, published := programschema.ModuleEntryRootFunctionFamily().Count(&program.Frozen, catalog); catalogOK && published {
					t.Fatal("unavailable root-function family remained published")
				}
			}
			if _, ok := SelectedQuerySites(compilation, mounts, record.Source.ContextDirectory()); ok {
				t.Fatal("unavailable or malformed root-function owner was admitted")
			}
		})
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
			compilation, compilationOK := Build()
			if !compilationOK {
				t.Fatal("compilation unavailable")
			}
			table, ok := SelectedQuerySites(compilation, record.Artifacts, record.Source.ContextDirectory())
			if !ok || table.Count() == 0 {
				t.Fatalf("control-fault root has no selected query sites: ok=%t rows=%d", ok, table.Count())
			}
		})
	}
}

func TestSelectedQuerySitesUseTheirOwnerAddressFormula(t *testing.T) {
	record := mountedRecord(t, "query-address", "return 42")
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	table, ok := SelectedQuerySites(compilation, record.Artifacts, record.Source.ContextDirectory())
	if !ok || table.Count() == 0 {
		t.Fatal("selected query sites")
	}
	issued := make(map[schema.Key]struct{})
	for _, family := range QueryIssuance(compilation) {
		issued[family.Family] = struct{}{}
	}
	for index := 0; index < table.Count(); index++ {
		site, siteOK := table.At(index)
		if !siteOK {
			t.Fatalf("selected query site %d is unavailable", index)
		}
		if _, known := issued[site.Family]; !known {
			t.Fatalf("site %d carries unissued family %q", index, site.Family)
		}
		contextID := site.Context.ID()
		want, derived := identity.DeriveContentID(querySiteFormula, site.Mount[:], contextID[:], site.Point[:], []byte(site.Family))
		if !derived || site.ID != want {
			t.Fatalf("site %d address %v is not the owner formula over (%v, %v, %q)", index, site.ID, site.Mount, site.Point, site.Family)
		}
	}
}

func queryContextForMount(t testing.TB, directory executioncontext.Directory, mount identity.ContentID) executioncontext.Context {
	t.Helper()
	if !directory.Available() || !mount.Available() {
		t.Fatal("query context directory or mount is unavailable")
	}
	for index := 0; index < directory.ContextCount(); index++ {
		context, ok := directory.ContextAt(index)
		if ok && context.Available() && context.ModuleKey() == mount {
			return context
		}
	}
	t.Fatalf("no query context for mount %v", mount)
	return executioncontext.Context{}
}

func selectedCallableOccurrencePoints(t *testing.T, program programmount.Program) map[identity.ContentID]map[identity.ContentID]struct{} {
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
		held, callable := points[body]
		if !callable {
			continue
		}
		_, pointCount, pointSpanOK := occurrence.PointSpan()
		for pointIndex := 0; pointIndex < int(pointCount); pointIndex++ {
			point, pointOK := program.OccurrencePointID(index, pointIndex)
			if !pointSpanOK || !pointOK {
				t.Fatal("occurrence point")
			}
			held[point] = struct{}{}
		}
	}
	return points
}

func returnedAndUnreturnedCallableBodies(t testing.TB, program programmount.Program) (identity.ContentID, identity.ContentID) {
	t.Helper()
	callableByFunction := make(map[identity.ContentID]identity.ContentID)
	bodyCount, bodiesPublished := program.BodyCount()
	if !bodiesPublished {
		t.Fatal("cold Body family")
	}
	for index := 0; index < bodyCount; index++ {
		body, bodyOK := program.BodyAt(index)
		function, functionOK := body.FunctionContextID()
		if !bodyOK {
			t.Fatal("body row")
		}
		if functionOK {
			callableByFunction[function] = body.ID()
		}
	}
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	rootFunctionCount, rootFunctionsPublished := programschema.ModuleEntryRootFunctionFamily().Count(&program.Frozen, catalog)
	if !catalogOK || !rootFunctionsPublished {
		t.Fatal("root-function owner")
	}
	returned := make(map[identity.ContentID]struct{})
	for index := 0; index < rootFunctionCount; index++ {
		child, childOK := program.ModuleEntryRootFunctionAt(index)
		if !childOK || !child.Available() {
			t.Fatal("root-function row")
		}
		body, bodyOK := callableByFunction[child.FunctionID()]
		if !bodyOK {
			t.Fatal("root-function body")
		}
		returned[body] = struct{}{}
	}
	if len(returned) != 1 {
		t.Fatalf("returned callable body count = %d, want 1", len(returned))
	}
	var returnedBody identity.ContentID
	for body := range returned {
		returnedBody = body
	}
	var unreturnedBody identity.ContentID
	for function := range callableByFunction {
		candidate := callableByFunction[function]
		if candidate == returnedBody {
			continue
		}
		if unreturnedBody.Available() {
			t.Fatal("fixture has multiple unreturned callable bodies")
		}
		unreturnedBody = candidate
	}
	if !returnedBody.Available() || !unreturnedBody.Available() {
		t.Fatal("fixture lost returned or unreturned callable")
	}
	return returnedBody, unreturnedBody
}

// corruptRootFunctionPosition is test-only hostile publication setup. It
// changes one row in a fresh fixture while preserving the Frozen metadata that
// authenticates the mount, so SelectedQuerySites reaches its sidecar-row
// validation rather than failing the mount preflight. Production publications
// remain immutable; no runtime code uses this escape hatch.
func corruptRootFunctionPosition(t testing.TB, mount *programmount.MountedArtifact, index int, position uint32) {
	t.Helper()
	if mount == nil || !mount.Program.Available() {
		t.Fatal("malformed fixture program")
	}
	frozen := reflect.ValueOf(&mount.Program.Frozen).Elem()
	publication := frozen.FieldByName("publication")
	columns := writableTestValue(t, publication.FieldByName("columns"))
	slot := int(programcatalog.ModuleEntryRootFunction().Slot())
	if slot < 0 || slot >= columns.Len() {
		t.Fatalf("root-function slot %d outside frozen columns", slot)
	}
	column := columns.Index(slot)
	if column.IsNil() {
		t.Fatal("root-function column unavailable")
	}
	columnValue := column.Elem().Elem()
	values := writableTestValue(t, columnValue.FieldByName("values"))
	if index < 0 || index >= values.Len() {
		t.Fatalf("root-function row %d outside column", index)
	}
	row := values.Index(index)
	positionField := writableTestValue(t, row.FieldByName("position"))
	positionField.SetUint(uint64(position))
}

func writableTestValue(t testing.TB, value reflect.Value) reflect.Value {
	t.Helper()
	if !value.IsValid() || !value.CanAddr() {
		t.Fatal("test publication field is unavailable")
	}
	return reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
}

func selectedDirectCalleeAndSibling(t *testing.T, program programmount.Program) (identity.ContentID, identity.ContentID) {
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
	callCount, callsPublished := program.CallCount()
	if !callsPublished {
		t.Fatal("cold Call family")
	}
	for index := 0; index < callCount; index++ {
		call, callOK := program.CallAt(index)
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
