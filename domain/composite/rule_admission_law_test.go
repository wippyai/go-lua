package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

func TestMountedAdmissionsMatchSealedIngressPlacements(t *testing.T) {
	record := mountedRecord(t, "rule-admission", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding")
	}
	mounted, activations, failed, ok := rules.MountedAdmissions(record.Artifacts)
	if !ok || failed != DiagnosticRuleUnknown {
		t.Fatalf("mounted admissions refused: ok=%v failed=%v", ok, failed)
	}
	type placement struct {
		key        schema.Key
		mount      identity.ContentID
		point      identity.ContentID
		occurrence identity.ContentID
	}
	expected := make(map[placement]int)
	for _, mount := range record.Artifacts {
		if !mount.Available() {
			t.Fatal("sealed mount")
		}
		program := mount.Snapshot.Program()
		count, published := program.RuleOccurrenceCount()
		if !published {
			t.Fatal("cold rule-occurrence family")
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.RuleOccurrenceAt(index)
			ordinal, ordinalOK := row.Occurrence()
			occurrence, occurrenceOK := program.OccurrenceAt(int(ordinal))
			if !rowOK || !ordinalOK || !occurrenceOK || !row.Key().Available() || !row.PointID().Available() || !occurrence.ID().Available() {
				t.Fatalf("sealed placement %d", index)
			}
			expected[placement{row.Key(), mount.ModuleKey, row.PointID(), occurrence.ID()}]++
		}
	}
	if len(expected) == 0 {
		t.Fatal("fixture issued no sealed placements")
	}
	observed := make(map[placement]int, len(mounted)+len(activations))
	for _, row := range mounted {
		observed[placement{capabilityKey(t, bound.Compilation(), rules, row.Capability), row.Mount, row.Point, row.Occurrence}]++
	}
	for _, row := range activations {
		observed[placement{capabilityKey(t, bound.Compilation(), rules, row.Capability), row.Mount, row.Point, row.Occurrence}]++
	}
	if len(observed) != len(expected) {
		t.Fatalf("admissions=%d sealed placements=%d", len(observed), len(expected))
	}
	for key, count := range expected {
		if observed[key] != count {
			t.Fatalf("placement %q/%v missing from sealed-row admissions", key.key, key.occurrence)
		}
	}
}

// TestOwnerRejectedCallShapeIsNeverPlaced states the placement half of the
// declared-admissibility law on one exact shape: a method call is not the
// strict unary plain geometry Value seals a runtime-kind operand for, and the
// subscription declares that requirement, so the artifact carries no
// runtime-kind placement to admit in the first place.
func TestOwnerRejectedCallShapeIsNeverPlaced(t *testing.T) {
	record := mountedRecord(t, "runtime-kind-admission", "local function handle(ch) ch:recv() end")
	bound := materializerBinding(t, record)
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding")
	}
	mounted, _, failed, ok := rules.MountedAdmissions(record.Artifacts)
	if !ok || failed != DiagnosticRuleUnknown {
		t.Fatalf("mounted admissions refused: ok=%v failed=%v", ok, failed)
	}
	const runtimeKind = schema.Key("value-runtime-kind-call")
	placed, admitted := 0, 0
	for _, mount := range record.Artifacts {
		program := mount.Snapshot.Program()
		count, published := program.RuleOccurrenceCount()
		if !published {
			t.Fatal("cold rule-occurrence family")
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.RuleOccurrenceAt(index)
			if rowOK && row.Key() == runtimeKind {
				placed++
			}
		}
	}
	for _, row := range mounted {
		if capabilityKey(t, bound.Compilation(), rules, row.Capability) == runtimeKind {
			admitted++
		}
	}
	if placed != 0 || admitted != 0 {
		t.Fatalf("runtime-kind placements=%d admissions=%d, want placements=0 admissions=0", placed, admitted)
	}
}

func TestLinkAdmissionsWalkDeclaredCatalogs(t *testing.T) {
	record := mountedRecord(t, "link-admission", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding")
	}
	admitted, ok := rules.LinkAdmissions()
	if !ok {
		t.Fatal("link admissions refused")
	}
	expected := 0
	seen := make(map[identity.ContentID]bool, len(admitted))
	for _, key := range LinkKeys(bound.Compilation()) {
		catalog, catalogOK := rules.LinkCatalogByKey(key)
		if !catalogOK {
			t.Fatalf("link catalog %q", key)
		}
		ids := make([]identity.ContentID, catalog.Count())
		for index := range ids {
			id, idOK := catalog.IDAt(index)
			if !idOK {
				t.Fatalf("catalog %q row %d", key, index)
			}
			ids[index] = id
		}
		expected += len(ids)
		for _, id := range ids {
			seen[id] = false
		}
	}
	if len(admitted) != expected {
		t.Fatalf("link admissions=%d declared members=%d", len(admitted), expected)
	}
	for _, row := range admitted {
		key := capabilityKey(t, bound.Compilation(), rules, row.Capability)
		cell, cellOK := rules.cellByKey(key)
		if !row.Capability.Link() || !row.Occurrence.Available() || !cellOK || !cell.Available() {
			t.Fatal("link admission row")
		}
		marked, known := seen[row.Occurrence]
		if !known || marked {
			t.Fatalf("admission occurrence %v is not a unique catalog identity", row.Occurrence)
		}
		seen[row.Occurrence] = true
	}
}

func capabilityKey(t *testing.T, compilation Compilation, rules *RuleBinding, capability engine.RuleSlotCapability) schema.Key {
	t.Helper()
	state := compilation.catalog
	if state == nil {
		t.Fatal("capability has no compilation environment")
	}
	for _, entry := range state.templates {
		if entry == nil {
			continue
		}
		got, ok := rules.CapabilityByKey(entry.Key())
		if ok && got == capability {
			return entry.Key()
		}
	}
	t.Fatal("capability is not a sealed table role")
	return ""
}

// TestBodylessCallActivationIsFullyAdmitted states the issuance half of the
// declared-placement law for Call's activation lane. The artifact places one
// call-activation occurrence per call site, statically, before any body
// inventory exists; whether that trigger reaches an activatable body is the
// content of its candidate set, not a condition on its issuance. A program
// that declares no body therefore still admits the whole trigger - transport
// vector and application identity included - for every placement it carries.
func TestBodylessCallActivationIsFullyAdmitted(t *testing.T) {
	record := mountedRecord(t, "bodyless-call-activation", "local invoke: fun(): number = nil; local produced = invoke()")
	bound := materializerBinding(t, record)
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding")
	}
	_, activations, failed, ok := rules.MountedAdmissions(record.Artifacts)
	if !ok || failed != DiagnosticRuleUnknown {
		t.Fatalf("mounted admissions refused: ok=%v failed=%v", ok, failed)
	}
	const callActivation = schema.Key("call-activation")
	placed := 0
	for _, mount := range record.Artifacts {
		program := mount.Snapshot.Program()
		count, published := program.RuleOccurrenceCount()
		if !published {
			t.Fatal("cold rule-occurrence family")
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.RuleOccurrenceAt(index)
			if rowOK && row.Key() == callActivation {
				placed++
			}
		}
	}
	if placed == 0 {
		t.Fatal("fixture placed no call-activation occurrence")
	}
	if len(activations) != placed {
		t.Fatalf("call-activation placements=%d admissions=%d", placed, len(activations))
	}
	for index, row := range activations {
		key := capabilityKey(t, bound.Compilation(), rules, row.Capability)
		cell, cellOK := rules.cellByKey(key)
		if row.Transport == nil || !row.Capability.Activation() || !cellOK || !cell.Available() || !row.Application.Available() {
			t.Fatalf("activation %d admitted without its transport vector, activation capability, canonical cell, or application: transport=%t capability=%t cell=%t application=%t",
				index, row.Transport != nil, row.Capability.Activation(), cellOK && cell.Available(), row.Application.Available())
		}
		if len(row.Candidates) != 0 {
			t.Fatalf("activation %d reached %d candidates in a program that declares no body", index, len(row.Candidates))
		}
	}
}

// TestBodylessPlacementQueryRetainsAnAbsentSummary states the Placement query
// half of the no-call law.  A zero-allocation Heap has no public Placement
// rows, but the sealed query family remains an admitted construction lane and
// retains its mount-qualified publication identity.
func TestBodylessPlacementQueryRetainsAnAbsentSummary(t *testing.T) {
	record := mountedRecord(t, "bodyless-placement-query", "local invoke: fun(): number = nil; local produced = invoke()")
	if record.PlacementSchema.KeyCount() != 0 {
		t.Fatalf("bodyless fixture has %d Placement coordinates, want zero", record.PlacementSchema.KeyCount())
	}
	bound := materializerBinding(t, record)
	if bound.PlacementQuery() == nil {
		t.Fatal("bodyless Placement query implementation is unavailable")
	}
	sites, sitesOK := SelectedQuerySites(bound.Compilation(), record.Artifacts)
	if !sitesOK || len(sites) == 0 {
		t.Fatal("bodyless fixture issued no selected query sites")
	}
	placementSites := 0
	for _, site := range sites {
		if site.Family == QueryFamilyPlacementSummary {
			placementSites++
		}
	}
	if placementSites == 0 {
		t.Fatal("bodyless fixture issued no Placement query site")
	}
	if queries, queriesOK := bound.QueryAdmissions(sites); !queriesOK || len(queries) != len(sites) {
		t.Fatalf("bodyless query admissions = %d/%v, sites = %d", len(queries), queriesOK, len(sites))
	}
}
