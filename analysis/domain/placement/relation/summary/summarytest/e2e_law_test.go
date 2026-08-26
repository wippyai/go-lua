package summarytest

import (
	"testing"

	heapsummary "github.com/wippyai/go-lua/analysis/domain/heap/relation/summary"
	placementsummary "github.com/wippyai/go-lua/analysis/domain/placement/relation/summary"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/relationadmission"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

const e2eDomain = "analysis/domain/placement/relation/summary/summarytest/v1"

// TestAllocationSummaryChildAdmissionSolveSnapshot proves the smallest child
// law at the real admission boundary: a generic declaration is compiled and
// checked, specialized into one witness/geometry mount, solved by the target
// runtime, and redeemed through the canonical snapshot. The Heap span has one
// non-Boot allocation root; suspension is deliberately proven absent rather
// than represented by an invented state value.
func TestAllocationSummaryChildAdmissionSolveSnapshot(t *testing.T) {
	fixture := newSpecimen(t)
	ready, refusal := relationadmission.Admit(fixture.input(t))
	if refusal != nil || !ready.Available() {
		if refusal == nil {
			t.Fatal("allocation-summary admission returned unavailable ready")
		}
		t.Fatalf("allocation-summary admission refused: %v issues=%+v", refusal, refusal.CertificateIssues())
	}
	result, solved := runtime.Solve(ready.Mounted(), ready.Base(), ready.Geometry())
	if !solved || !result.Available() || result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("allocation-summary solve solved=%v available=%v evaluations=%d publications=%d", solved, result.Available(), result.Evaluations(), result.Publications())
	}
	projection, projected := snapshot.Publish(result, ready.Geometry())
	if !projected || !projection.Available() {
		t.Fatal("allocation-summary snapshot publication")
	}
	keys := projection.Keys(fixture.outputFact)
	if len(keys) != 1 || keys[0].Row != fixture.outputRow {
		t.Fatalf("allocation-summary snapshot keys=%v, want one child row %v", keys, fixture.outputRow)
	}
	factCell, status := projection.Read(fixture.outputFact, keys[0])
	if status != canonical.ReadHit || !factCell.Available() || !factCell.Presence.Is(model.Present) {
		t.Fatalf("allocation-summary Fact cell status=%v cell=%#v", status, factCell)
	}
	fact, factOK := fixture.fact.Decode(factCell.Value)
	if !factOK || fact != fixture.expectedFact {
		t.Fatalf("allocation-summary Fact=%#v/%v, want %#v", fact, factOK, fixture.expectedFact)
	}
	evidenceCell, evidenceStatus := projection.Read(fixture.outputEvidence, keys[0])
	if evidenceStatus != canonical.ReadHit || !evidenceCell.Available() || !evidenceCell.Presence.Is(model.Present) {
		t.Fatalf("allocation-summary evidence cell status=%v cell=%#v", evidenceStatus, evidenceCell)
	}
	evidence, evidenceOK := fixture.evidence.Decode(evidenceCell.Value)
	if !evidenceOK || !evidence.Valid() || evidence.OwnerIdentity != fixture.allocationID || !evidence.HasOwnerIdentity {
		t.Fatalf("allocation-summary evidence=%#v/%v, want owner %v", evidence, evidenceOK, fixture.allocationID)
	}
	if evidence.DiesBeforeSuspension != placementdomain.EvidenceAbsent {
		t.Fatalf("allocation-summary suspension evidence=%v, want proven-absent public state", evidence.DiesBeforeSuspension)
	}
}

// TestPlacementSummaryParentAdmissionSolveSnapshot proves the complete
// parent law at the real admission boundary. The parent receives one scalar
// Q.site-id from its direct population child and three distinct slots from a
// single Complete QAllocation child (AllocationID, Fact, Evidence), then
// publishes only Q.placement-schema-id and redeems that exact schema ID.
func TestPlacementSummaryParentAdmissionSolveSnapshot(t *testing.T) {
	fixture := newParentSpecimen(t)
	childOutputs := fixture.operation.Outputs()
	parentInputs := fixture.parentOperation.Inputs()
	if len(childOutputs) != 3 || childOutputs[0].Column != fixture.outputAddress || childOutputs[1].Column != fixture.outputFact || childOutputs[2].Column != fixture.outputEvidence ||
		len(parentInputs) != 4 || !parentInputs[0].Delivery.IsScalar() || parentInputs[0].Presence != signature.RequireOpaque || parentInputs[0].Column != fixture.siteAddress ||
		parentInputs[1].Column != fixture.outputAddress || parentInputs[2].Column != fixture.outputFact || parentInputs[3].Column != fixture.outputEvidence ||
		len(fixture.parentOperation.Outputs()) != 1 || fixture.parentOperation.Outputs()[0].Presence != signature.ProduceOpaque || fixture.parentOperation.Outputs()[0].Column != fixture.siteSchemaID {
		t.Fatal("placement-summary parent ABI did not retain scalar site plus three distinct child slots")
	}
	ready, refusal := relationadmission.Admit(fixture.input(t))
	if refusal != nil || !ready.Available() {
		if refusal == nil {
			t.Fatal("placement-summary parent admission returned unavailable ready")
		}
		initial, hasInitial := refusal.Initial()
		initialName := "unknown"
		if hasInitial {
			switch initial.Operation() {
			case fixture.seedSite.Identity():
				initialName = "site"
			case fixture.seedAllocation.Identity():
				initialName = "allocation"
			case fixture.seedHeap.Identity():
				initialName = "heap"
			case fixture.seedOutputSite.Identity():
				initialName = "output-site"
			}
		}
		t.Fatalf("placement-summary parent admission refused: %v issues=%+v initial=%s/%+v outcome=%+v", refusal, refusal.CertificateIssues(), initialName, initial, refusal.Outcome())
	}
	result, solved := runtime.Solve(ready.Mounted(), ready.Base(), ready.Geometry())
	if !solved || !result.Available() {
		t.Fatalf("placement-summary parent solve solved=%v available=%v evaluations=%d publications=%d", solved, result.Available(), result.Evaluations(), result.Publications())
	}
	projection, projected := snapshot.Publish(result, ready.Geometry())
	if !projected || !projection.Available() {
		t.Fatal("placement-summary parent snapshot publication")
	}
	keys := projection.Keys(fixture.siteSchemaID)
	if len(keys) != len(fixture.siteRows) {
		t.Fatalf("placement-summary parent snapshot keys=%v, want one result per site %v", keys, fixture.siteRows)
	}
	for _, siteRow := range fixture.siteRows {
		var siteKey snapshot.RowKey
		found := false
		for _, key := range keys {
			if key.Row == siteRow {
				siteKey, found = key, true
				break
			}
		}
		if !found {
			t.Fatalf("placement-summary parent omitted site row %v from keys=%v", siteRow, keys)
		}
		cell, status := projection.Read(fixture.siteSchemaID, siteKey)
		if status != canonical.ReadHit || !cell.Available() || !cell.Presence.Is(model.AuthenticatedOpaque) {
			t.Fatalf("placement-summary parent schema ID cell row=%v status=%v cell=%#v", siteRow, status, cell)
		}
		decoded, ok := fixture.schemaIDCodec.Decode(cell.Value)
		if !ok || decoded != fixture.schema.ContentID() {
			t.Fatalf("placement-summary parent schema ID row=%v value=%x/%v, want exact %x", siteRow, decoded, ok, fixture.schema.ContentID())
		}
	}
	childKeys := projection.Keys(fixture.outputFact)
	if len(childKeys) != len(fixture.outputRows) {
		t.Fatalf("placement-summary parent child keys=%v, want one allocation row per site %v", childKeys, fixture.outputRows)
	}
	seenChildRows := make(map[model.RowID]struct{}, len(childKeys))
	for _, key := range childKeys {
		if _, duplicate := seenChildRows[key.Row]; duplicate {
			t.Fatalf("placement-summary parent duplicated child row %v", key.Row)
		}
		seenChildRows[key.Row] = struct{}{}
		cell, status := projection.Read(fixture.outputAddress, key)
		if status != canonical.ReadHit || !cell.Available() || !cell.Presence.Is(model.Present) {
			t.Fatalf("placement-summary parent child allocation cell row=%v status=%v cell=%#v", key.Row, status, cell)
		}
		allocationID, ok := fixture.address.Decode(cell.Value)
		if !ok || allocationID != fixture.allocationID {
			t.Fatalf("placement-summary parent child allocation row=%v value=%x/%v, want %x", key.Row, allocationID, ok, fixture.allocationID)
		}
	}
	for _, outputRow := range fixture.outputRows {
		if _, found := seenChildRows[outputRow]; !found {
			t.Fatalf("placement-summary parent child output omitted canonical QAllocation row %v", outputRow)
		}
	}
}

type specimen struct {
	identity targetIdentity
	world    relationfixture.Sealed
	schema   placementdomain.Schema

	owner        model.OwnerID
	lineageOwner model.OwnerID
	schemaID     model.SchemaID
	scope        model.ScopeID

	addressType    model.TypeID
	schemaIDType   model.TypeID
	sourceType     model.TypeID
	heapRootType   model.TypeID
	factType       model.TypeID
	suspensionType model.TypeID
	evidenceType   model.TypeID

	site       model.RelationID
	allocation model.RelationID
	heap       model.RelationID
	output     model.RelationID

	siteAddress          model.ColumnID
	siteSchemaID         model.ColumnID
	allocationIDColumn   model.ColumnID
	allocationSource     model.ColumnID
	allocationFact       model.ColumnID
	allocationSuspension model.ColumnID
	allocationSite       model.ColumnID
	heapAddress          model.ColumnID
	heapRoot             model.ColumnID
	outputAddress        model.ColumnID
	outputFact           model.ColumnID
	outputEvidence       model.ColumnID
	outputSite           model.ColumnID

	siteKey           model.KeyID
	allocationKey     model.KeyID
	allocationSiteKey model.KeyID
	heapKey           model.KeyID
	outputKey         model.KeyID

	siteDenominator           model.DenominatorRef
	allocationDenominator     model.DenominatorRef
	allocationSiteDenominator model.DenominatorRef
	heapDenominator           model.DenominatorRef
	outputDenominator         model.DenominatorRef

	siteRow        model.RowID
	siteRow2       model.RowID
	siteRows       []model.RowID
	siteValues     []identity.ContentID
	allocationRow  model.RowID
	allocationRows []model.RowID
	heapRow        model.RowID
	heapRows       []model.RowID
	outputRow      model.RowID
	outputRows     []model.RowID

	seedSite        signature.Signature
	seedAllocation  signature.Signature
	seedHeap        signature.Signature
	seedOutputSite  signature.Signature
	operation       signature.Signature
	parentOperation signature.Signature
	childBinding    binding.Factory
	parentBinding   binding.Factory
	declaration     relcompile.Declaration
	refusal         model.RefusalID
	metadata        heapsummary.AllocationRow
	heapRootValue   heapdomain.Value
	scopeRegion     region.Region

	fact          *relbindgen.Column[placementdomain.Fact]
	evidence      *relbindgen.Column[placementdomain.AllocationEvidence]
	address       *relbindgen.Column[identity.ContentID]
	schemaIDCodec *relbindgen.Column[identity.ContentID]
	source        *relbindgen.Column[heapsummary.Source]
	heapValue     *relbindgen.Column[heapdomain.Value]
	suspension    *relbindgen.Column[suspension.Evidence]

	allocationID identity.ContentID
	expectedFact placementdomain.Fact
}

// targetIdentity keeps all logical test identities owner-issued and scoped to
// this child family. It is test support only; no production declaration is
// modified or reused as a substitute identity.
type targetIdentity struct {
	domain string
	owner  model.OwnerID
}

func newIdentity(t testing.TB, domain string) targetIdentity {
	t.Helper()
	content, ok := identity.DeriveContentID(e2eDomain+"/owner", []byte(domain))
	if !ok {
		t.Fatal("summary-test owner content")
	}
	owner, ok := model.IssueOwnerID(content)
	if !ok {
		t.Fatal("summary-test owner")
	}
	return targetIdentity{domain: domain, owner: owner}
}

func (value targetIdentity) content(t testing.TB, label string) identity.ContentID {
	t.Helper()
	content, ok := identity.DeriveContentID(e2eDomain+"/content", []byte(value.domain), []byte(label))
	if !ok {
		t.Fatalf("summary-test content %q", label)
	}
	return content
}

func newSpecimen(t testing.TB) specimen {
	t.Helper()
	world := relationfixture.New(t)
	placementSchema, ok := placementdomain.NewSchema(world.Heap)
	if !ok || placementSchema.KeyCount() != 1 || world.Heap.KeyCount() != 1 {
		t.Fatal("summary-test expected one allocation and one full Heap root")
	}
	key, ok := placementSchema.KeyAt(0)
	if !ok || key.Kind() != heapdomain.RootAllocation {
		t.Fatal("summary-test expected non-Boot allocation root")
	}
	allocationID, ok := key.ContentID()
	if !ok {
		t.Fatal("summary-test allocation identity")
	}
	metadata, ok := heapsummary.NewAllocationRow(world.Heap, key)
	if !ok || !metadata.Valid() {
		t.Fatal("summary-test Heap allocation metadata")
	}

	id := newIdentity(t, "allocation-summary-child")
	owner := id.owner
	lineageOwner := issueOwner(t, id.content(t, "lineage-owner"))
	schemaID := issueSchema(t, owner, id.content(t, "schema"))
	scope := issueScope(t, owner, id.content(t, "scope"))
	// The scope formula is a concrete owner-issued Boolean proposition.  It
	// travels through ScopeSchema and the checked certificate; inventory only
	// resolves physical addresses and denominator evidence.
	scopeAtom, ok := region.NewAtom(id.content(t, "scope/region-atom"))
	if !ok {
		t.Fatal("summary-test scope region atom")
	}
	scopeRegion, ok := region.FromAtom(scopeAtom)
	if !ok {
		t.Fatal("summary-test scope region")
	}
	addressType := issueType(t, owner, id.content(t, "type/address"))
	schemaIDType := issueType(t, owner, id.content(t, "type/placement-schema-id"))
	sourceType := issueType(t, owner, id.content(t, "type/source"))
	heapRootType := issueType(t, owner, id.content(t, "type/heap-root"))
	factType := issueType(t, owner, id.content(t, "type/fact"))
	suspensionType := issueType(t, owner, id.content(t, "type/suspension"))
	evidenceType := issueType(t, owner, id.content(t, "type/evidence"))
	site := issueRelation(t, owner, id.content(t, "relation/q-site"))
	allocation := issueRelation(t, owner, id.content(t, "relation/allocation"))
	heap := issueRelation(t, owner, id.content(t, "relation/heap"))
	output := issueRelation(t, owner, id.content(t, "relation/allocation-summary-child"))

	column := func(relation model.RelationID, label string) model.ColumnID {
		return issueColumn(t, relation, id.content(t, "column/"+label))
	}
	keyID := func(relation model.RelationID, label string) model.KeyID {
		return issueKey(t, relation, id.content(t, "key/"+label))
	}
	siteAddress := column(site, "site-address")
	siteSchemaID := column(site, "placement-schema-id")
	allocationIDColumn := column(allocation, "allocation-id")
	allocationSource := column(allocation, "allocation-source")
	allocationFact := column(allocation, "allocation-fact")
	allocationSuspension := column(allocation, "allocation-suspension")
	allocationSite := column(allocation, "allocation-site-id")
	heapAddress := column(heap, "heap-address")
	heapRoot := column(heap, "heap-root")
	outputAddress := column(output, "output-address")
	outputFact := column(output, "output-fact")
	outputEvidence := column(output, "output-evidence")
	outputSite := column(output, "output-site-id")
	siteKey := keyID(site, "site")
	allocationKey := keyID(allocation, "allocation")
	allocationSiteKey := keyID(allocation, "allocation-site")
	heapKey := keyID(heap, "heap")
	outputKey := keyID(output, "output")
	denom := func(relation model.RelationID, key model.KeyID) model.DenominatorRef {
		result, ok := model.NewDenominatorRef(relation, key)
		if !ok {
			t.Fatal("summary-test denominator")
		}
		return result
	}
	siteDenominator := denom(site, siteKey)
	allocationDenominator := denom(allocation, allocationKey)
	allocationSiteDenominator := denom(allocation, allocationSiteKey)
	heapDenominator := denom(heap, heapKey)
	outputDenominator := denom(output, outputKey)
	siteRow := issueRow(t, site, allocationID)
	// The publication destination is the allocation ID slot, so the source
	// allocation row intentionally carries that same content identity.
	pairContent := func(siteID, allocationID identity.ContentID) identity.ContentID {
		content, ok := identity.DeriveContentID(e2eDomain+"/pair", siteID[:], allocationID[:])
		if !ok {
			t.Fatal("summary-test pair identity")
		}
		return content
	}
	allocationRow := issueRow(t, allocation, pairContent(allocationID, allocationID))
	heapRow := issueRow(t, heap, id.content(t, "row/heap"))
	outputRow := issueRow(t, output, pairContent(allocationID, allocationID))

	address := codec[identity.ContentID](t, id, addressType, "address", 8)
	schemaIDCodec := codec[identity.ContentID](t, id, schemaIDType, "placement-schema-id", 2)
	source := codec[heapsummary.Source](t, id, sourceType, "source", 2)
	heapValue := codec[heapdomain.Value](t, id, heapRootType, "heap-root", 2)
	fact := codec[placementdomain.Fact](t, id, factType, "fact", 4)
	suspension := codec[suspension.Evidence](t, id, suspensionType, "suspension", 2)
	evidence := codec[placementdomain.AllocationEvidence](t, id, evidenceType, "evidence", 2)

	expectedFact := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	operationID := issueOperation(t, owner, id.content(t, "operation/allocation-summary"))
	seedSiteID := issueOperation(t, owner, id.content(t, "operation/seed-site"))
	seedAllocationID := issueOperation(t, owner, id.content(t, "operation/seed-allocation"))
	seedHeapID := issueOperation(t, owner, id.content(t, "operation/seed-heap"))
	seedOutputSiteID := issueOperation(t, owner, id.content(t, "operation/seed-output-site"))
	expressionID := issueExpression(t, owner, id.content(t, "expression/allocation-summary"))
	dependencyID := issueDependency(t, owner, id.content(t, "dependency/allocation-summary"))
	refusalID := issueRefusal(t, owner, id.content(t, "refusal/allocation-summary"))

	seedSite := sealSignature(t, owner, schemaID, seedSiteID, nil, []signature.Output{
		{Relation: site, Column: siteAddress, Type: addressType, Presence: signature.ProduceOpaque, Denominator: siteDenominator},
	}, cardinality(t, model.BoundedMany, 2), outcome.Produced)
	seedAllocation := sealSignature(t, owner, schemaID, seedAllocationID, nil, []signature.Output{
		{Relation: allocation, Column: allocationIDColumn, Type: addressType, Presence: signature.ProducePresent, Denominator: allocationDenominator},
		{Relation: allocation, Column: allocationSource, Type: sourceType, Presence: signature.ProducePresent, Denominator: allocationDenominator},
		{Relation: allocation, Column: allocationFact, Type: factType, Presence: signature.ProducePresent, Denominator: allocationDenominator},
		{Relation: allocation, Column: allocationSuspension, Type: suspensionType, Presence: signature.ProduceAbsent, Denominator: allocationDenominator},
		{Relation: allocation, Column: allocationSite, Type: addressType, Presence: signature.ProduceOpaque, Denominator: allocationDenominator},
	}, cardinality(t, model.BoundedMany, 2), outcome.Produced)
	seedHeap := sealSignature(t, owner, schemaID, seedHeapID, nil, []signature.Output{
		{Relation: heap, Column: heapAddress, Type: addressType, Presence: signature.ProducePresent, Denominator: heapDenominator},
		{Relation: heap, Column: heapRoot, Type: heapRootType, Presence: signature.ProducePresent, Denominator: heapDenominator},
	}, cardinality(t, model.BoundedMany, 2), outcome.Produced)
	seedOutputSite := sealSignature(t, owner, schemaID, seedOutputSiteID, nil, []signature.Output{{
		Relation: output, Column: outputSite, Type: addressType, Presence: signature.ProduceOpaque, Denominator: outputDenominator,
	}}, cardinality(t, model.BoundedMany, 2), outcome.Produced)
	completeAllocation, ok := signature.NewCompleteSpanDelivery(allocationKey)
	if !ok {
		t.Fatal("summary-test allocation complete delivery")
	}
	completeHeap, ok := signature.NewCompleteSpanDelivery(heapKey)
	if !ok {
		t.Fatal("summary-test Heap complete delivery")
	}
	operation := sealSignature(t, owner, schemaID, operationID, []signature.Input{
		{Relation: allocation, Column: allocationIDColumn, Type: addressType, Presence: signature.RequirePresent, Delivery: completeAllocation, Denominator: allocationDenominator},
		{Relation: allocation, Column: allocationSource, Type: sourceType, Presence: signature.RequirePresent, Delivery: completeAllocation, Denominator: allocationDenominator},
		{Relation: heap, Column: heapRoot, Type: heapRootType, Presence: signature.AllowMissing, Delivery: completeHeap, Denominator: heapDenominator},
		{Relation: allocation, Column: allocationFact, Type: factType, Presence: signature.AllowMissing, Delivery: completeAllocation, Denominator: allocationDenominator},
		{Relation: allocation, Column: allocationSuspension, Type: suspensionType, Presence: signature.AllowMissing, Delivery: completeAllocation, Denominator: allocationDenominator},
	}, []signature.Output{
		{Relation: output, Column: outputAddress, Type: addressType, Presence: signature.ProduceOptional, Denominator: outputDenominator},
		{Relation: output, Column: outputFact, Type: factType, Presence: signature.ProduceOptional, Denominator: outputDenominator},
		{Relation: output, Column: outputEvidence, Type: evidenceType, Presence: signature.ProduceOptional, Denominator: outputDenominator},
	}, cardinality(t, model.BoundedMany, 1), outcome.Produced, outcome.Opaque, outcome.Refused)

	declaration := relcompile.Declaration{
		SchemaID: schemaID,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(site, []model.ColumnID{siteAddress, siteSchemaID}, []model.KeyID{siteKey}, scope),
			model.DefineRelationSchema(allocation, []model.ColumnID{allocationIDColumn, allocationSource, allocationFact, allocationSuspension, allocationSite}, []model.KeyID{allocationKey, allocationSiteKey}, scope),
			model.DefineRelationSchema(heap, []model.ColumnID{heapAddress, heapRoot}, []model.KeyID{heapKey}, scope),
			model.DefineRelationSchema(output, []model.ColumnID{outputAddress, outputFact, outputEvidence, outputSite}, []model.KeyID{outputKey}, scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(siteAddress, addressType),
			model.DefineColumnSchema(siteSchemaID, schemaIDType),
			model.DefineColumnSchema(allocationIDColumn, addressType),
			model.DefineColumnSchema(allocationSource, sourceType), model.DefineColumnSchema(allocationFact, factType), model.DefineColumnSchema(allocationSuspension, suspensionType), model.DefineColumnSchema(allocationSite, addressType),
			model.DefineColumnSchema(heapAddress, addressType), model.DefineColumnSchema(heapRoot, heapRootType),
			model.DefineColumnSchema(outputAddress, addressType), model.DefineColumnSchema(outputFact, factType), model.DefineColumnSchema(outputEvidence, evidenceType), model.DefineColumnSchema(outputSite, addressType),
		},
		TypeCapabilities: []model.TypeCapability{
			capability(t, addressType, model.Ascending), capability(t, schemaIDType, model.Ascending), capability(t, sourceType, model.Ascending), capability(t, heapRootType, model.Ascending),
			capability(t, factType, model.Ascending), capability(t, suspensionType, model.Ascending), capability(t, evidenceType, model.Ascending),
		},
		Keys: []model.KeySchema{
			model.DefineKeySchema(siteKey, []model.ColumnID{siteAddress}), model.DefineKeySchema(allocationKey, []model.ColumnID{allocationSite, allocationIDColumn}),
			model.DefineKeySchema(allocationSiteKey, []model.ColumnID{allocationSite}),
			model.DefineKeySchema(heapKey, []model.ColumnID{heapAddress}), model.DefineKeySchema(outputKey, []model.ColumnID{outputSite, outputAddress}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(scope, nil, scopeRegion)},
		Signatures: []signature.Signature{seedSite, seedAllocation, seedHeap, seedOutputSite, operation},
		Initials:   []plan.Initial{plan.DefineInitial(seedSite.Identity(), scope), plan.DefineInitial(seedAllocation.Identity(), scope), plan.DefineInitial(seedHeap.Identity(), scope), plan.DefineInitial(seedOutputSite.Identity(), scope)},
		Rules: []relcompile.Rule{{
			ID: dependencyID, Expression: expressionID, Candidate: allocation, Apply: operation.Identity(),
			ApplyShape: &relcompile.ApplyShape{
				Children: []relcompile.ApplyChild{
					{Candidate: allocation, Scope: scope, Complete: &allocationDenominator},
					{Candidate: heap, Scope: scope, Complete: &heapDenominator},
				},
				Correlation: algebra.NewApplyCorrelation(allocationSiteDenominator, allocationSite, addressType, [][]model.ColumnID{{allocationSite}, {heapAddress}}),
				Slots: []algebra.SlotSource{
					algebra.NewSlotSource(0, 0), algebra.NewSlotSource(0, 1), algebra.NewSlotSource(1, 1),
					algebra.NewSlotSource(0, 2), algebra.NewSlotSource(0, 3),
				},
				Output: algebra.SpanSource(algebra.NewSlotSource(0, 0)),
			},
			Publish: &relcompile.Publication{Relation: output, Key: outputKey, Columns: []model.ColumnID{outputAddress, outputFact, outputEvidence}},
		}},
	}

	columns, ok := placementsummary.NewPlacementSummaryAllocationColumns(address, source, heapValue, fact, suspension, mustOutputColumns(t, address, fact, evidence))
	if !ok {
		t.Fatal("summary-test typed child columns")
	}
	judgment, ok := placementsummary.NewPlacementSummaryAllocationOperation(placementSchema)
	if !ok {
		t.Fatal("summary-test child judgment")
	}
	factory, ok := placementsummary.BindPlacementSummaryAllocation(operation, judgment, columns, refusalID)
	if !ok {
		t.Fatal("summary-test child binding")
	}

	return specimen{
		identity: id, world: world, schema: placementSchema, owner: owner, lineageOwner: lineageOwner, schemaID: schemaID, scope: scope,
		addressType: addressType, schemaIDType: schemaIDType, sourceType: sourceType, heapRootType: heapRootType, factType: factType, suspensionType: suspensionType, evidenceType: evidenceType,
		site: site, allocation: allocation, heap: heap, output: output,
		siteAddress: siteAddress, siteSchemaID: siteSchemaID, allocationIDColumn: allocationIDColumn, allocationSource: allocationSource, allocationFact: allocationFact, allocationSuspension: allocationSuspension, allocationSite: allocationSite,
		heapAddress: heapAddress, heapRoot: heapRoot, outputAddress: outputAddress, outputFact: outputFact, outputEvidence: outputEvidence, outputSite: outputSite,
		siteKey: siteKey, allocationKey: allocationKey, allocationSiteKey: allocationSiteKey, heapKey: heapKey, outputKey: outputKey,
		siteDenominator: siteDenominator, allocationDenominator: allocationDenominator, allocationSiteDenominator: allocationSiteDenominator, heapDenominator: heapDenominator, outputDenominator: outputDenominator,
		siteRow: siteRow, siteRows: []model.RowID{siteRow}, siteValues: []identity.ContentID{allocationID}, allocationRow: allocationRow, allocationRows: []model.RowID{allocationRow}, heapRow: heapRow, heapRows: []model.RowID{heapRow}, outputRow: outputRow, outputRows: []model.RowID{outputRow},
		seedSite: seedSite, seedAllocation: seedAllocation, seedHeap: seedHeap, seedOutputSite: seedOutputSite, operation: operation,
		fact: fact, evidence: evidence, address: address, schemaIDCodec: schemaIDCodec, source: source, heapValue: heapValue, suspension: suspension,
		allocationID: allocationID, expectedFact: expectedFact,
		childBinding: factory,
		declaration:  declaration, refusal: refusalID, metadata: metadata, heapRootValue: world.Heap.Bottom(), scopeRegion: scopeRegion,
	}
}

// newParentSpecimen adds the parent rule to the already sealed child
// specimen. Keeping the child and parent in one declaration is intentional:
// admission must check and mount the heterogeneous Apply against the same
// Complete child relation that publishes the three-slot ABI.
func newParentSpecimen(t testing.TB) specimen {
	t.Helper()
	value := newSpecimen(t)
	// Two independently keyed Q.site rows deliberately point at the same
	// allocation identity. Their RowIDs use the canonical Q row token (the
	// Site key), while the child rows below use the canonical QAllocation pair
	// token. The parent must settle one schema answer per site; a child-row
	// Cartesian expansion would show up as extra site outputs.
	value.siteValues = []identity.ContentID{
		value.identity.content(t, "site/id-1"),
		value.identity.content(t, "site/id-2"),
	}
	value.siteRows = []model.RowID{
		issueRow(t, value.site, value.siteValues[0]),
		issueRow(t, value.site, value.siteValues[1]),
	}
	value.siteRow = value.siteRows[0]
	value.siteRow2 = value.siteRows[1]
	value.allocationRows = make([]model.RowID, len(value.siteValues))
	value.heapRows = make([]model.RowID, len(value.siteValues))
	value.outputRows = make([]model.RowID, len(value.siteValues))
	for index, siteValue := range value.siteValues {
		pair, ok := identity.DeriveContentID(e2eDomain+"/pair", siteValue[:], value.allocationID[:])
		if !ok {
			t.Fatal("summary-test parent pair identity")
		}
		value.allocationRows[index] = issueRow(t, value.allocation, pair)
		value.heapRows[index] = issueRow(t, value.heap, siteValue)
		value.outputRows[index] = issueRow(t, value.output, pair)
		if value.outputRows[index].Content() != pair {
			t.Fatal("summary-test parent output row was not the canonical QAllocation pair row")
		}
	}
	value.allocationRow = value.allocationRows[0]
	value.heapRow = value.heapRows[0]
	value.outputRow = value.outputRows[0]
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("summary-test parent scalar delivery")
	}
	complete, ok := signature.NewCompleteSpanDelivery(value.outputKey)
	if !ok {
		t.Fatal("summary-test parent child complete delivery")
	}
	parentOperationID := issueOperation(t, value.owner, value.identity.content(t, "operation/placement-summary-parent"))
	parentExpressionID := issueExpression(t, value.owner, value.identity.content(t, "expression/placement-summary-parent"))
	parentDependencyID := issueDependency(t, value.owner, value.identity.content(t, "dependency/placement-summary-parent"))
	parentSignature := sealSignature(t, value.owner, value.schemaID, parentOperationID, []signature.Input{
		{Relation: value.site, Column: value.siteAddress, Type: value.addressType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: value.siteDenominator},
		{Relation: value.output, Column: value.outputAddress, Type: value.addressType, Presence: signature.AllowMissing, Delivery: complete, Denominator: value.outputDenominator},
		{Relation: value.output, Column: value.outputFact, Type: value.factType, Presence: signature.AllowMissing, Delivery: complete, Denominator: value.outputDenominator},
		{Relation: value.output, Column: value.outputEvidence, Type: value.evidenceType, Presence: signature.AllowMissing, Delivery: complete, Denominator: value.outputDenominator},
	}, []signature.Output{{
		Relation: value.site, Column: value.siteSchemaID, Type: value.schemaIDType, Presence: signature.ProduceOpaque, Denominator: value.siteDenominator,
	}}, cardinality(t, model.Optional, 0), outcome.Produced, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	parentColumns, ok := placementsummary.NewParentColumns(value.schemaIDCodec)
	if !ok {
		t.Fatal("summary-test parent output columns")
	}
	parentBindingColumns, ok := placementsummary.NewPlacementSummaryParentColumns(
		value.siteAddress, value.addressType, value.address, value.fact, value.evidence, parentColumns,
	)
	if !ok {
		t.Fatal("summary-test parent binding columns")
	}
	judgment, ok := placementsummary.NewPlacementSummaryParentOperation(value.schema)
	if !ok {
		t.Fatal("summary-test parent judgment")
	}
	parentBinding, ok := placementsummary.BindPlacementSummaryParent(parentSignature, judgment, parentBindingColumns, value.refusal)
	if !ok {
		t.Fatal("summary-test parent binding")
	}
	value.parentOperation = parentSignature
	value.parentBinding = parentBinding
	value.declaration.Signatures = append(value.declaration.Signatures, parentSignature)
	value.declaration.Rules = append(value.declaration.Rules, relcompile.Rule{
		ID: parentDependencyID, Expression: parentExpressionID, Candidate: value.site, Apply: parentSignature.Identity(),
		ApplyShape: &relcompile.ApplyShape{
			Children: []relcompile.ApplyChild{
				// The population side is intentionally the direct Input required by
				// the mixed scalar-population+Complete-span ABI. Its row is already
				// fenced by the site denominator; adding a Select/Scope wrapper would
				// turn this back into the rejected all-complete form.
				{Candidate: value.site},
				{Candidate: value.output, Scope: value.scope, Complete: &value.outputDenominator},
			},
			Correlation: algebra.NewApplyCorrelation(value.siteDenominator, value.siteAddress, value.addressType, [][]model.ColumnID{{value.siteAddress}, {value.outputSite}}),
			Slots: []algebra.SlotSource{
				algebra.NewSlotSource(0, 0),
				algebra.NewSlotSource(1, 0), algebra.NewSlotSource(1, 1), algebra.NewSlotSource(1, 2),
			},
			Output: algebra.ScalarSource(algebra.NewSlotSource(0, 0)),
		},
		Publish: &relcompile.Publication{Relation: value.site, Key: value.siteKey, Columns: []model.ColumnID{value.siteSchemaID}},
	})
	return value
}

func issueOwner(t testing.TB, content identity.ContentID) model.OwnerID {
	t.Helper()
	value, ok := model.IssueOwnerID(content)
	if !ok {
		t.Fatal("summary-test owner identity")
	}
	return value
}

func issueSchema(t testing.TB, owner model.OwnerID, content identity.ContentID) model.SchemaID {
	t.Helper()
	value, ok := model.IssueSchemaID(owner, content)
	if !ok {
		t.Fatal("summary-test schema identity")
	}
	return value
}

func issueScope(t testing.TB, owner model.OwnerID, content identity.ContentID) model.ScopeID {
	t.Helper()
	value, ok := model.IssueScopeID(owner, content)
	if !ok {
		t.Fatal("summary-test scope identity")
	}
	return value
}

func issueType(t testing.TB, owner model.OwnerID, content identity.ContentID) model.TypeID {
	t.Helper()
	value, ok := model.IssueTypeID(owner, content)
	if !ok {
		t.Fatal("summary-test type identity")
	}
	return value
}

func issueRelation(t testing.TB, owner model.OwnerID, content identity.ContentID) model.RelationID {
	t.Helper()
	value, ok := model.IssueRelationID(owner, content)
	if !ok {
		t.Fatal("summary-test relation identity")
	}
	return value
}

func issueColumn(t testing.TB, relation model.RelationID, content identity.ContentID) model.ColumnID {
	t.Helper()
	value, ok := model.IssueColumnID(relation, content)
	if !ok {
		t.Fatal("summary-test column identity")
	}
	return value
}

func issueKey(t testing.TB, relation model.RelationID, content identity.ContentID) model.KeyID {
	t.Helper()
	value, ok := model.IssueKeyID(relation, content)
	if !ok {
		t.Fatal("summary-test key identity")
	}
	return value
}

func issueRow(t testing.TB, relation model.RelationID, content identity.ContentID) model.RowID {
	t.Helper()
	value, ok := model.IssueRowID(relation, content)
	if !ok {
		t.Fatal("summary-test row identity")
	}
	return value
}

func issueOperation(t testing.TB, owner model.OwnerID, content identity.ContentID) model.OperationID {
	t.Helper()
	value, ok := model.IssueOperationID(owner, content)
	if !ok {
		t.Fatal("summary-test operation identity")
	}
	return value
}

func issueExpression(t testing.TB, owner model.OwnerID, content identity.ContentID) model.ExpressionID {
	t.Helper()
	value, ok := model.IssueExpressionID(owner, content)
	if !ok {
		t.Fatal("summary-test expression identity")
	}
	return value
}

func issueDependency(t testing.TB, owner model.OwnerID, content identity.ContentID) model.DependencyID {
	t.Helper()
	value, ok := model.IssueDependencyID(owner, content)
	if !ok {
		t.Fatal("summary-test dependency identity")
	}
	return value
}

func issueRefusal(t testing.TB, owner model.OwnerID, content identity.ContentID) model.RefusalID {
	t.Helper()
	value, ok := model.IssueRefusalID(owner, content)
	if !ok {
		t.Fatal("summary-test refusal identity")
	}
	return value
}

func cardinality(t testing.TB, kind model.CardinalityKind, bound uint32) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(kind, bound)
	if !ok {
		t.Fatal("summary-test cardinality")
	}
	return value
}

func capability(t testing.TB, typeID model.TypeID, kind model.TypeCapabilityKind) model.TypeCapability {
	t.Helper()
	value, ok := model.NewTypeCapability(typeID, kind)
	if !ok {
		t.Fatal("summary-test type capability")
	}
	return value
}

func sealSignature(t testing.TB, owner model.OwnerID, schema model.SchemaID, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, card model.Cardinality, codes ...outcome.Code) signature.Signature {
	t.Helper()
	accepted, ok := outcome.NewSet(codes...)
	if !ok {
		t.Fatal("summary-test outcomes")
	}
	value, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schema}, Inputs: inputs, Outputs: outputs,
		Cardinality: card, Outcomes: accepted,
	})
	if !ok {
		t.Fatal("summary-test signature")
	}
	return value
}

func codec[T any](t testing.TB, id targetIdentity, typeID model.TypeID, label string, reserve int) *relbindgen.Column[T] {
	t.Helper()
	store, ok := relbindgen.NewStore[T](id.content(t, "codec/"+label), reserve)
	if !ok {
		t.Fatalf("summary-test %s store", label)
	}
	column, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatalf("summary-test %s column", label)
	}
	return column
}

func mustOutputColumns(t testing.TB, allocationID *relbindgen.Column[identity.ContentID], fact *relbindgen.Column[placementdomain.Fact], evidence *relbindgen.Column[placementdomain.AllocationEvidence]) placementsummary.AllocationColumns {
	t.Helper()
	columns, ok := placementsummary.NewAllocationColumnsWithID(allocationID, fact, evidence)
	if !ok {
		t.Fatal("summary-test output columns")
	}
	return columns
}

func (value specimen) input(t testing.TB) relationadmission.Input {
	t.Helper()
	lineageFactory, ok := lineage.NewFactory(value.lineageOwner)
	if !ok {
		t.Fatal("summary-test lineage factory")
	}
	geometryFactory, ok := targetfixture.NewGeometryFactory(value.scopeRegion.Identity())
	if !ok {
		t.Fatal("summary-test geometry factory")
	}
	factories := []binding.Factory{value.childBinding}
	if value.parentBinding != nil {
		factories = append(factories, value.parentBinding)
	}
	siteCells := make([]seedCell, 0, len(value.siteRows))
	for index, row := range value.siteRows {
		if index >= len(value.siteValues) {
			t.Fatal("summary-test site rows and values diverged")
		}
		siteValue := value.siteValues[index]
		siteCells = append(siteCells,
			seedCell{column: value.siteAddress, row: row, presence: model.AuthenticatedOpaque, encode: func(issuer binding.Issuer) (binding.ValueToken, bool) {
				return value.address.Encode(issuer, siteValue)
			}},
		)
	}
	allocationCells := make([]seedCell, 0, len(value.allocationRows)*5)
	heapCells := make([]seedCell, 0, len(value.heapRows)*2)
	outputSiteCells := make([]seedCell, 0, len(value.outputRows))
	for index, allocationRow := range value.allocationRows {
		if index >= len(value.siteValues) {
			t.Fatal("summary-test allocation rows and site values diverged")
		}
		siteValue := value.siteValues[index]
		allocationCells = append(allocationCells,
			seedCell{column: value.allocationIDColumn, row: allocationRow, presence: model.Present, encode: func(issuer binding.Issuer) (binding.ValueToken, bool) {
				return value.address.Encode(issuer, value.metadata.ID())
			}},
			seedCell{column: value.allocationSource, row: allocationRow, presence: model.Present, encode: func(issuer binding.Issuer) (binding.ValueToken, bool) {
				return value.source.Encode(issuer, value.metadata.Source)
			}},
			seedCell{column: value.allocationFact, row: allocationRow, presence: model.Present, encode: func(issuer binding.Issuer) (binding.ValueToken, bool) {
				return value.fact.Encode(issuer, value.expectedFact)
			}},
			seedCell{column: value.allocationSuspension, row: allocationRow, presence: model.ProvenAbsent},
			seedCell{column: value.allocationSite, row: allocationRow, presence: model.AuthenticatedOpaque, encode: func(issuer binding.Issuer) (binding.ValueToken, bool) {
				return value.address.Encode(issuer, siteValue)
			}},
		)
	}
	for index, heapRow := range value.heapRows {
		if index >= len(value.siteValues) {
			t.Fatal("summary-test heap rows and site values diverged")
		}
		siteValue := value.siteValues[index]
		heapCells = append(heapCells,
			seedCell{column: value.heapAddress, row: heapRow, presence: model.Present, encode: func(issuer binding.Issuer) (binding.ValueToken, bool) {
				return value.address.Encode(issuer, siteValue)
			}},
			seedCell{column: value.heapRoot, row: heapRow, presence: model.Present, encode: func(issuer binding.Issuer) (binding.ValueToken, bool) {
				return value.heapValue.Encode(issuer, value.heapRootValue)
			}},
		)
	}
	for index, outputRow := range value.outputRows {
		if index >= len(value.siteValues) {
			t.Fatal("summary-test output rows and site values diverged")
		}
		siteValue := value.siteValues[index]
		outputSiteCells = append(outputSiteCells, seedCell{column: value.outputSite, row: outputRow, presence: model.AuthenticatedOpaque, encode: func(issuer binding.Issuer) (binding.ValueToken, bool) {
			return value.address.Encode(issuer, siteValue)
		}})
	}
	factories = append(factories,
		seedFactory{operation: value.seedSite, cells: siteCells},
		seedFactory{operation: value.seedAllocation, cells: allocationCells},
		seedFactory{operation: value.seedHeap, cells: heapCells},
		seedFactory{operation: value.seedOutputSite, cells: outputSiteCells},
	)
	return relationadmission.Input{
		Declaration: value.declaration,
		Inventory:   specimenInventoryFactory{specimen: value},
		Bindings:    specimenBindingFactory{factories: factories},
		Algebras: authorityRegistry{
			address: value.address, schemaID: value.schemaIDCodec, source: value.source, heapValue: value.heapValue, fact: value.fact,
			suspension: value.suspension, evidence: value.evidence,
		},
		Lineage:  lineageFactory,
		Geometry: geometryFactory,
	}
}

type seedCell struct {
	column   model.ColumnID
	row      model.RowID
	presence model.PresenceKind
	encode   func(binding.Issuer) (binding.ValueToken, bool)
}

type seedFactory struct {
	operation signature.Signature
	cells     []seedCell
}

func (factory seedFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || operation.Digest() != factory.operation.Digest() {
		return nil, false
	}
	return seedBinding{operation: factory.operation, cells: factory.cells}, true
}

type seedBinding struct {
	operation signature.Signature
	cells     []seedCell
}

func (value seedBinding) Signature() signature.Signature { return value.operation }

func (value seedBinding) NewWorker(fence binding.Fence) (binding.Worker, bool) {
	return seedWorker{operation: value.operation, fence: fence, cells: value.cells}, value.operation.Available() && fence.Available()
}

type seedWorker struct {
	operation signature.Signature
	fence     binding.Fence
	cells     []seedCell
}

func (worker seedWorker) Evaluate(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if buffer == nil || !frame.Available() || frame.Len() != 0 || !worker.operation.Available() || buffer.Signature().Digest() != worker.operation.Digest() {
		return outcome.Result{}
	}
	issuer, ok := binding.NewIssuer(worker.fence)
	if !ok {
		return outcome.Result{}
	}
	for _, cell := range worker.cells {
		output, outputOK := worker.operation.OutputFor(cell.column.Relation(), cell.column)
		witness, witnessOK := buffer.DestinationWitness(output.Denominator)
		row, rowOK := model.IssueRowID(output.Relation, cell.row.Content())
		if !outputOK || !witnessOK || !rowOK || row.Relation() != cell.row.Relation() {
			return outcome.Result{}
		}
		destination, destinationOK := issuer.IssueCell(witness, buffer.Scope(), cell.column, row)
		var value binding.ValueToken
		if cell.presence == model.Present || cell.presence == model.AuthenticatedOpaque {
			if cell.encode == nil {
				return outcome.Result{}
			}
			value, ok = cell.encode(issuer)
			if !ok {
				return outcome.Result{}
			}
		}
		presence, presenceOK := model.NewPresence(cell.presence)
		proposal, proposalOK := binding.NewProposal(destination, value, presence)
		if !destinationOK || !presenceOK || !proposalOK || !buffer.Append(proposal) {
			return outcome.Result{}
		}
	}
	return outcome.Result{Code: outcome.Produced}
}

type specimenBindingFactory struct{ factories []binding.Factory }

func (factory specimenBindingFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || len(factory.factories) == 0 {
		return nil, false
	}
	var result binding.Binding
	for _, candidate := range factory.factories {
		if candidate == nil {
			return nil, false
		}
		bound, ok := candidate.Bind(operation)
		if !ok {
			continue
		}
		if bound == nil || result != nil {
			return nil, false
		}
		result = bound
	}
	return result, result != nil
}

type specimenInventoryFactory struct{ specimen specimen }

func (factory specimenInventoryFactory) Bind(cert certificate.Certificate) (witness.Inventory, bool) {
	if !cert.Available() || cert.SchemaID() != factory.specimen.schemaID {
		return nil, false
	}
	storeID, ok := identity.IssueStore()
	if !ok {
		return nil, false
	}
	fence, ok := address.NewFence(cert.SchemaID(), cert.Digest(), storeID, identity.MountID{0xD4}, identity.Generation(1))
	if !ok {
		return nil, false
	}
	rows := map[model.DenominatorRef][]model.RowID{
		factory.specimen.siteDenominator:           append([]model.RowID(nil), factory.specimen.siteRows...),
		factory.specimen.allocationDenominator:     append([]model.RowID(nil), factory.specimen.allocationRows...),
		factory.specimen.allocationSiteDenominator: append([]model.RowID(nil), factory.specimen.allocationRows...),
		factory.specimen.heapDenominator:           append([]model.RowID(nil), factory.specimen.heapRows...),
		factory.specimen.outputDenominator:         append([]model.RowID(nil), factory.specimen.outputRows...),
	}
	return &specimenInventory{certificate: cert, fence: fence, rows: rows, specimen: factory.specimen}, true
}

type specimenInventory struct {
	certificate certificate.Certificate
	fence       address.Fence
	rows        map[model.DenominatorRef][]model.RowID
	accesses    []arrangement.Access
	specimen    specimen
}

func (inventory *specimenInventory) Fence() address.Fence { return inventory.fence }

func (inventory *specimenInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	for index, relation := range inventory.certificate.Relations() {
		if relation.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	for index, column := range inventory.certificate.Columns() {
		if column.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	for index, key := range inventory.certificate.Keys() {
		if key.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	for index, scope := range inventory.certificate.Scopes() {
		if scope.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	for index, expression := range inventory.certificate.Expressions() {
		if expression.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	for index, dependency := range inventory.certificate.Dependencies() {
		if dependency.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (inventory *specimenInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, prior := range inventory.accesses {
		if prior.Equal(access) {
			return arrangement.NewHandle(inventory.fence, uint64(index+1))
		}
	}
	inventory.accesses = append(inventory.accesses, access)
	return arrangement.NewHandle(inventory.fence, uint64(len(inventory.accesses)))
}

func (inventory *specimenInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	rows, ok := inventory.rows[ref]
	if !ok {
		return witness.DenominatorEvidence{}, false
	}
	relation := ref.Relation().Content()
	key := ref.Key().Content()
	evidence, ok := identity.DeriveContentID(e2eDomain+"/denominator", relation[:], key[:])
	if !ok {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(rows, evidence)
}

func (inventory *specimenInventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}

// ResolvePartition supplies the explicit Q-site-to-child postings required by
// the correlated Apply. The child rows are already bounded to their sealed
// denominators; this method only snapshots that owner evidence for mount
// specialization, where it is reissued under the runtime fence.
func (inventory *specimenInventory) ResolvePartition(partition certificate.CorrelationPartition) (map[model.RowID]witness.DenominatorEvidence, bool) {
	if inventory == nil || !partition.Available() {
		return nil, false
	}
	population := partition.Population()
	child := partition.Child()
	var populationRows []model.RowID
	switch population {
	case inventory.specimen.siteDenominator:
		populationRows = inventory.specimen.siteRows
	case inventory.specimen.allocationSiteDenominator:
		populationRows = inventory.specimen.allocationRows
	default:
		return nil, false
	}
	var childRows []model.RowID
	switch child {
	case inventory.specimen.allocationDenominator:
		childRows = inventory.specimen.allocationRows
	case inventory.specimen.heapDenominator:
		childRows = inventory.specimen.heapRows
	case inventory.specimen.outputDenominator:
		childRows = inventory.specimen.outputRows
	default:
		return nil, false
	}
	if len(childRows) == 0 {
		return nil, false
	}
	digest := partition.Digest()
	evidence, ok := identity.DeriveContentID(e2eDomain+"/partition", digest[:])
	if !ok {
		return nil, false
	}
	result := make(map[model.RowID]witness.DenominatorEvidence, len(populationRows))
	for index, populationRow := range populationRows {
		childIndex := index
		if childIndex >= len(childRows) {
			return nil, false
		}
		posting, postingOK := witness.NewDenominatorEvidence([]model.RowID{childRows[childIndex]}, evidence)
		if !postingOK {
			return nil, false
		}
		result[populationRow] = posting
	}
	return result, true
}

// The only key in this specimen is an owner-issued ContentID. The generic
// relation runtime needs equality to join the Q site to the two source
// relations, but no ascent authority is semantically requested by this rule.
// This test-only authority decodes through the same owner codec and projects
// equality; it does not infer identity from a token or physical slot.
type authorityRegistry struct {
	address    *relbindgen.Column[identity.ContentID]
	schemaID   *relbindgen.Column[identity.ContentID]
	source     *relbindgen.Column[heapsummary.Source]
	heapValue  *relbindgen.Column[heapdomain.Value]
	fact       *relbindgen.Column[placementdomain.Fact]
	suspension *relbindgen.Column[suspension.Evidence]
	evidence   *relbindgen.Column[placementdomain.AllocationEvidence]
}

func (registry authorityRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	switch {
	case registry.address != nil && registry.address.Type() == typeID:
		return typedAuthority[identity.ContentID]{column: registry.address, equal: func(left, right identity.ContentID) bool { return left == right }}, true
	case registry.schemaID != nil && registry.schemaID.Type() == typeID:
		return typedAuthority[identity.ContentID]{column: registry.schemaID, equal: func(left, right identity.ContentID) bool { return left == right }}, true
	case registry.source != nil && registry.source.Type() == typeID:
		return typedAuthority[heapsummary.Source]{column: registry.source, equal: func(left, right heapsummary.Source) bool { return left == right }}, true
	case registry.heapValue != nil && registry.heapValue.Type() == typeID:
		return typedAuthority[heapdomain.Value]{column: registry.heapValue, equal: heapdomain.Equal}, true
	case registry.fact != nil && registry.fact.Type() == typeID:
		return typedAuthority[placementdomain.Fact]{column: registry.fact, equal: func(left, right placementdomain.Fact) bool { return left == right }}, true
	case registry.suspension != nil && registry.suspension.Type() == typeID:
		return typedAuthority[suspension.Evidence]{column: registry.suspension, equal: func(left, right suspension.Evidence) bool { return left == right }}, true
	case registry.evidence != nil && registry.evidence.Type() == typeID:
		return typedAuthority[placementdomain.AllocationEvidence]{column: registry.evidence, equal: func(left, right placementdomain.AllocationEvidence) bool { return left == right }}, true
	default:
		return nil, false
	}
}

func (registry authorityRegistry) ResolveEquality(typeID model.TypeID) (binding.ValueEquality, bool) {
	authority, ok := registry.Resolve(typeID)
	if !ok {
		return nil, false
	}
	equality, ok := authority.(binding.ValueEquality)
	return equality, ok
}

// typedAuthority is a fixture-local exact-value authority for columns that
// are only published once in this law. It decodes through the owner codec and
// never compares or joins raw token identities. The Placement operation's
// own mathematics remains the production binding; this authority only
// satisfies the runtime's monotone-cell contract for the non-repeated seeds.
type typedAuthority[T any] struct {
	column *relbindgen.Column[T]
	equal  func(T, T) bool
}

func (authority typedAuthority[T]) Type() model.TypeID {
	if authority.column == nil {
		return model.TypeID{}
	}
	return authority.column.Type()
}

func (authority typedAuthority[T]) values(left, right binding.ValueToken) (T, T, bool) {
	var zero T
	if authority.column == nil {
		return zero, zero, false
	}
	leftValue, leftOK := authority.column.Decode(left)
	rightValue, rightOK := authority.column.Decode(right)
	return leftValue, rightValue, leftOK && rightOK
}

func (authority typedAuthority[T]) Equal(left, right binding.ValueToken) bool {
	leftValue, rightValue, ok := authority.values(left, right)
	return ok && authority.equal != nil && authority.equal(leftValue, rightValue)
}

func (authority typedAuthority[T]) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !authority.Equal(left, right) {
		return binding.ValueToken{}, false
	}
	return left, true
}

func (authority typedAuthority[T]) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return authority.Join(left, right)
}

func (authority typedAuthority[T]) LessOrEqual(left, right binding.ValueToken) bool {
	return authority.Equal(left, right)
}
