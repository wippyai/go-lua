package relation_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/analysis/schema/program/publication"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

// TestPlacementSuspensionCorpusKeepsTheVectorAtRouteDerivation proves the
// generated fold ABI receives only scalar route fields. A Value span here
// would recreate the second summary authority this owner correction removes.
func TestPlacementSuspensionCorpusKeepsTheVectorAtRouteDerivation(t *testing.T) {
	for _, family := range relbind.Declared().Families {
		if family.Census != "placement/suspension" {
			continue
		}
		want := []string{"Candidate", "SourceSummary", "Route", "RouteTag", "Selected"}
		if len(family.Inputs) != len(want) {
			t.Fatalf("suspension fold slots=%d, want %d scalar slots", len(family.Inputs), len(want))
		}
		for index, input := range family.Inputs {
			if input.Field != want[index] || input.Delivery != signature.ScalarDelivery {
				t.Fatalf("suspension fold slot %d=%+v, want scalar %s", index, input, want[index])
			}
		}
		return
	}
	t.Fatal("relbind corpus lacks placement/suspension")
}

// TestPlacementSuspensionScalarBridgeCannotReopenSourceVector is a source
// shape law at the owner/binding boundary. Route derivation is the sole
// SummaryVector consumer; the fold, its stateless operation, and the emitted
// decoder may carry only SourceSummary. Keeping this as a structural check
// prevents a later local re-SummarizeSources adapter from silently restoring
// a second summary authority.
func TestPlacementSuspensionScalarBridgeCannotReopenSourceVector(t *testing.T) {
	if reflect.TypeOf(placementrelation.PlacementSuspensionOperation{}).NumField() != 0 {
		t.Fatal("PlacementSuspensionOperation retained solve state")
	}
	_, thisFile, _, callerOK := runtime.Caller(0)
	if !callerOK {
		t.Fatal("resolve suspension binding law source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	paths := []string{
		filepath.Join(root, "domain", "placement", "suspension", "reducer.go"),
		filepath.Join(root, "analysis", "domain", "placement", "relation", "operation_suspension.go"),
		filepath.Join(root, "analysis", "domain", "placement", "relation", "zz_placement_suspension.go"),
	}
	forbidden := []string{"execution.SummaryVector", "SourceVectorAdapter", "SummarizeSources("}
	for _, path := range paths {
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read scalar Suspension bridge %s: %v", path, readErr)
		}
		for _, token := range forbidden {
			if strings.Contains(string(contents), token) {
				t.Fatalf("scalar Suspension bridge %s reopened %s", path, token)
			}
		}
		if !strings.Contains(string(contents), "SourceSummary") {
			t.Fatalf("scalar Suspension bridge %s has no SourceSummary handoff", path)
		}
	}
	if _, statErr := os.Stat(filepath.Join(root, "domain", "placement", "suspension", "source_vector.go")); !os.IsNotExist(statErr) {
		t.Fatalf("removed SourceVectorAdapter source remains: %v", statErr)
	}
}

func suspensionBindingID(t testing.TB, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("placement-suspension-binding-law", []byte(name))
	if !ok {
		t.Fatalf("derive %s identity", name)
	}
	return id
}

func suspensionBindingCandidate(t testing.TB) suspension.MountedSubjectLiveness {
	t.Helper()
	schemaID := suspensionBindingID(t, "schema")
	catalogID, ok := programcatalog.CatalogID(schemaID)
	if !ok {
		t.Fatal("derive Program catalog")
	}
	call, route := suspensionBindingID(t, "call"), suspensionBindingID(t, "route")
	subject := suspensionBindingID(t, "subject")
	boundaryID, boundaryIDOK := lifecycle.SubjectYieldBoundaryIdentity(call, route)
	boundary, boundaryOK := lifecycle.NewSubjectYieldBoundary(boundaryID, call, route, identity.ContentID{}, identity.ContentID{}, 0)
	spanID, spanIDOK := lifecycle.SubjectLivenessSpanIdentity(lifecycle.SubjectLivenessCell, subject, 0, 0)
	span, spanOK := lifecycle.NewSubjectLivenessSpan(spanID, subject, lifecycle.SubjectLivenessCell, 0, 0, lifecycle.SubjectLivenessLive)
	if !boundaryIDOK || !boundaryOK || !spanIDOK || !spanOK {
		t.Fatal("subject liveness rows unavailable")
	}
	frozen, sealed := (publication.Publication{
		Lifecycle: lifecycle.Publication{
			SubjectSpans:      []lifecycle.SubjectLivenessSpan{span},
			SubjectBoundaries: []lifecycle.SubjectYieldBoundary{boundary},
		},
	}).Seal(catalogID, identity.StoreID(43))
	if !sealed {
		t.Fatal("seal lifecycle publication")
	}
	program := programschema.Program{
		Frozen: frozen, ArtifactID: suspensionBindingID(t, "artifact"),
		ProgramID: suspensionBindingID(t, "program"), SchemaID: schemaID,
	}
	state, ok := program.ColdState()
	if !ok {
		t.Fatal("open Program state")
	}
	candidate, ok := lifecycle.RedeemSubjectLiveness(state, 0, suspensionBindingID(t, "mount"), spanID)
	if !ok {
		t.Fatal("redeem subject liveness candidate")
	}
	return candidate
}

type suspensionBindingFixture struct {
	place      harness.Place
	operation  signature.Signature
	worker     binding.Worker
	candidate  model.ColumnID
	summary    model.ColumnID
	route      model.ColumnID
	routeTag   model.ColumnID
	selected   model.ColumnID
	output     model.ColumnID
	candidateT model.TypeID
	summaryT   model.TypeID
	routeT     model.TypeID
	routeTagT  model.TypeID
	selectedT  model.TypeID
	candidateC *relbindgen.Column[suspension.MountedSubjectLiveness]
	summaryC   *relbindgen.Column[suspension.SourceSummary]
	routeC     *relbindgen.Column[heapdomain.Key]
	routeTagC  *relbindgen.Column[uint64]
	selectedC  *relbindgen.Column[placementdomain.Fact]
}

func newSuspensionBindingFixture(t *testing.T) suspensionBindingFixture {
	t.Helper()
	place := harness.New(t, "row/suspension-candidate", "row/suspension-route")
	candidateT := place.TypeID(t, "type/suspension-candidate")
	summaryT := place.TypeID(t, "type/suspension-source-summary")
	routeT := place.TypeID(t, "type/heap-candidate")
	routeTagT := place.TypeID(t, "type/placement-route-tag")
	selectedT := place.TypeID(t, "type/placement")
	candidateC := harness.NewColumn[suspension.MountedSubjectLiveness](t, candidateT, "store/suspension-candidate", reserve)
	summaryC := harness.NewColumn[suspension.SourceSummary](t, summaryT, "store/suspension-source-summary", reserve)
	routeC := harness.NewColumn[heapdomain.Key](t, routeT, "store/suspension-route", reserve)
	routeTagC := harness.NewColumn[uint64](t, routeTagT, "store/suspension-route-tag", reserve)
	selectedC := harness.NewColumn[placementdomain.Fact](t, selectedT, "store/suspension-selected", reserve)
	columns, ok := placementrelation.NewPlacementSuspensionColumns(selectedC, routeC, candidateC, summaryC, routeTagC)
	if !ok {
		t.Fatal("suspension owner columns")
	}
	candidateAddress := place.Column(t, "column/suspension-candidate")
	summaryAddress := place.Column(t, "column/suspension-source-summary")
	routeAddress := place.Column(t, "column/suspension-route")
	routeTagAddress := place.Column(t, "column/suspension-route-tag")
	selectedAddress := place.Column(t, "column/suspension-selected")
	outputAddress := place.Column(t, "column/suspension-output")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("suspension cardinality")
	}
	operation := place.Seal(t, "operation/placement-suspension",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateT, place.Denominator),
			harness.ScalarInput(t, place.Relation, summaryAddress, summaryT, place.Denominator),
			harness.ScalarInput(t, place.Relation, routeAddress, routeT, place.Denominator),
			harness.ScalarInput(t, place.Relation, routeTagAddress, routeTagT, place.Denominator),
			harness.ScalarInput(t, place.Relation, selectedAddress, selectedT, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: outputAddress, Type: selectedT, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused,
	)
	factory, ok := placementrelation.BindPlacementSuspension(operation, placementrelation.PlacementSuspensionOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement suspension")
	}
	return suspensionBindingFixture{
		place: place, operation: operation, worker: place.Worker(t, factory, operation),
		candidate: candidateAddress, summary: summaryAddress, route: routeAddress, routeTag: routeTagAddress, selected: selectedAddress, output: outputAddress,
		candidateT: candidateT, summaryT: summaryT, routeT: routeT, routeTagT: routeTagT, selectedT: selectedT,
		candidateC: candidateC, summaryC: summaryC, routeC: routeC, routeTagC: routeTagC, selectedC: selectedC,
	}
}

func (fixture suspensionBindingFixture) evaluate(t *testing.T, candidate suspension.MountedSubjectLiveness, summary suspension.SourceSummary, route heapdomain.Key, routeTag uint64, selected placementdomain.Fact, candidatePresent bool) (outcome.Result, binding.ProposalBatch, bool) {
	t.Helper()
	candidateToken, ok := fixture.candidateC.Encode(fixture.place.Issuer, candidate)
	if !ok {
		t.Fatal("encode suspension candidate")
	}
	summaryToken, ok := fixture.summaryC.Encode(fixture.place.Issuer, summary)
	if !ok {
		t.Fatal("encode suspension source summary")
	}
	routeToken, ok := fixture.routeC.Encode(fixture.place.Issuer, route)
	if !ok {
		t.Fatal("encode suspension route")
	}
	routeTagToken, ok := fixture.routeTagC.Encode(fixture.place.Issuer, routeTag)
	if !ok {
		t.Fatal("encode suspension route tag")
	}
	selectedToken, ok := fixture.selectedC.Encode(fixture.place.Issuer, selected)
	if !ok {
		t.Fatal("encode suspension selected fact")
	}
	row := fixture.place.Rows[0]
	routeRow := fixture.place.Rows[1]
	slots := []binding.Slot{
		harness.ScalarSlot(t, func() binding.Cell {
			if candidatePresent {
				return fixture.place.Cell(t, fixture.candidate, row, fixture.candidateT, candidateToken)
			}
			return fixture.place.AbsentCell(t, fixture.candidate, row, fixture.candidateT)
		}()),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.summary, routeRow, fixture.summaryT, summaryToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.route, routeRow, fixture.routeT, routeToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeTag, routeRow, fixture.routeTagT, routeTagToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.selected, routeRow, fixture.selectedT, selectedToken)),
	}
	buffer := fixture.place.BufferAt(t, fixture.operation, fixture.place.Rows[1])
	result := fixture.worker.Evaluate(fixture.place.Frame(t, slots...), buffer)
	batch, sealed := buffer.Seal(result)
	return result, batch, sealed
}

func TestPlacementSuspensionBindsRealScalarFoldAtOneRouteRow(t *testing.T) {
	world := relationfixture.New(t)
	candidate := suspensionBindingCandidate(t)
	selected := placementdomain.DefaultFact()
	want, reduction := suspension.SuspensionFold(candidate, suspension.SourceSummaryKnown, world.Root, 1, selected)
	if reduction != structure.Concrete {
		t.Fatalf("real SuspensionFold reduction = %v, want Concrete", reduction)
	}
	fixture := newSuspensionBindingFixture(t)
	result, batch, sealed := fixture.evaluate(t, candidate, suspension.SourceSummaryKnown, world.Root, 1, selected, true)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("suspension binding = %v sealed=%t rows=%d, want one produced row", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != fixture.place.Rows[1] || proposal.Destination().Column() != fixture.output {
		t.Fatal("suspension binding did not publish at final selected Factor row")
	}
	if proposal.Destination().Row() == fixture.place.Rows[0] {
		t.Fatal("suspension binding published at candidate row instead of selected Factor row")
	}
	published, ok := fixture.selectedC.Decode(proposal.Value())
	if !ok || !placementdomain.EqualFact(published, want) {
		t.Fatalf("suspension fact = %#v, want SuspensionFold %#v", published, want)
	}
}

func TestPlacementSuspensionRefusesNearestInvalidEvidence(t *testing.T) {
	world := relationfixture.New(t)
	candidate := suspensionBindingCandidate(t)
	fixture := newSuspensionBindingFixture(t)
	checkRefused := func(name string, candidatePresent bool, route heapdomain.Key) {
		t.Helper()
		result, batch, sealed := fixture.evaluate(t, candidate, suspension.SourceSummaryKnown, route, 1, placementdomain.DefaultFact(), candidatePresent)
		if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
			t.Fatalf("%s = %v sealed=%t rows=%d, want refused without rows", name, result.Code, sealed, batch.Len())
		}
	}
	checkRefused("invalid route key", true, heapdomain.Key{})
	checkRefused("absent candidate", false, world.Root)
}
