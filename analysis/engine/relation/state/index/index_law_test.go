package index_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
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
)

type indexLawInventory struct {
	fence       address.Fence
	relation    model.RelationID
	denominator model.DenominatorRef
	rows        []model.RowID
	columns     map[model.ColumnID]uint64
	keys        map[model.KeyID]uint64
	scopes      map[model.ScopeID]uint64
	expressions map[model.ExpressionID]uint64
	accesses    []arrangement.Access
}

func (inventory *indexLawInventory) Fence() address.Fence { return inventory.fence }
func (inventory *indexLawInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	return 1, id == inventory.relation
}
func (inventory *indexLawInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	value, ok := inventory.columns[id]
	return value, ok
}
func (inventory *indexLawInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	value, ok := inventory.keys[id]
	return value, ok
}
func (inventory *indexLawInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	value, ok := inventory.scopes[id]
	return value, ok
}
func (inventory *indexLawInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	value, ok := inventory.expressions[id]
	return value, ok
}
func (inventory *indexLawInventory) ResolveDependency(model.DependencyID) (uint64, bool) {
	return 0, false
}
func (inventory *indexLawInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, prior := range inventory.accesses {
		if prior.Equal(access) {
			return arrangement.NewHandle(inventory.fence, uint64(index+1))
		}
	}
	inventory.accesses = append(inventory.accesses, access)
	return arrangement.NewHandle(inventory.fence, uint64(len(inventory.accesses)))
}
func (inventory *indexLawInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != inventory.denominator {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(inventory.rows, indexLawContent("denominator-evidence"))
}
func (inventory *indexLawInventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}

type indexLawAlgebra struct{ typeID model.TypeID }

func (algebra indexLawAlgebra) Type() model.TypeID { return algebra.typeID }
func (algebra indexLawAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !left.Available() || !right.Available() || left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return binding.ValueToken{}, false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	if bytes.Compare(leftOpaque[:], rightOpaque[:]) >= 0 {
		return left, true
	}
	return right, true
}
func (algebra indexLawAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return algebra.Join(left, right)
}
func (algebra indexLawAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	if !left.Available() || !right.Available() || left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	return bytes.Compare(leftOpaque[:], rightOpaque[:]) <= 0
}

type indexLawAlgebraRegistry struct{ algebra indexLawAlgebra }

func (registry indexLawAlgebraRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.algebra, registry.algebra.Type() == typeID
}

type indexLawFactory struct{ operation signature.Signature }

func (factory indexLawFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || operation.Digest() != factory.operation.Digest() {
		return nil, false
	}
	return indexLawBinding{operation: operation}, true
}

type indexLawBinding struct{ operation signature.Signature }

func (value indexLawBinding) Signature() signature.Signature { return value.operation }
func (value indexLawBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return nil, false
}

func indexLawVisit(index.Match) bool { return true }

type indexLawFixture struct {
	owner       model.OwnerID
	relation    model.RelationID
	columns     [2]model.ColumnID
	typeID      model.TypeID
	payloadType model.TypeID
	mounted     witness.Mounted
	layout      arrangement.Layout
	state       store.Version
	root        database.Version
	roots       map[store.Version]database.Version
	geometry    geometry.Geometry
	manager     *guard.Manager
	mask        support.Mask
	initial     [2]column.Version
}

func indexLawContent(label string) identity.ContentID {
	value, _ := identity.DeriveContentID("relation/state/index/law", []byte(label))
	return value
}

func indexLawID[T any](t *testing.T, issue func(identity.ContentID) (T, bool), label string) T {
	t.Helper()
	value, ok := issue(indexLawContent(label))
	if !ok {
		t.Fatalf("issue %s", label)
	}
	return value
}

func newIndexLawFixture(t *testing.T) indexLawFixture {
	return newIndexLawFixtureWithTypes(t, 2, false)
}

func newIndexLawFixtureWithKeyWidth(t *testing.T, keyWidth int) indexLawFixture {
	return newIndexLawFixtureWithTypes(t, keyWidth, false)
}

func newIndexLawFixtureWithTypes(t *testing.T, keyWidth int, distinctPayload bool) indexLawFixture {
	t.Helper()
	if keyWidth != 1 && keyWidth != 2 {
		t.Fatalf("key width = %d", keyWidth)
	}
	owner := indexLawID(t, func(value identity.ContentID) (model.OwnerID, bool) { return model.IssueOwnerID(value) }, "owner")
	relation := indexLawID(t, func(value identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, value) }, "relation")
	columnA := indexLawID(t, func(value identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, value) }, "column-a")
	columnB := indexLawID(t, func(value identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, value) }, "column-b")
	typeID := indexLawID(t, func(value identity.ContentID) (model.TypeID, bool) { return model.IssueTypeID(owner, value) }, "type")
	payloadType := typeID
	if distinctPayload {
		payloadType = indexLawID(t, func(value identity.ContentID) (model.TypeID, bool) { return model.IssueTypeID(owner, value) }, "payload-type")
	}
	key := indexLawID(t, func(value identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(relation, value) }, "key")
	scope := indexLawID(t, func(value identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, value) }, "scope")
	schemaID := indexLawID(t, func(value identity.ContentID) (model.SchemaID, bool) { return model.IssueSchemaID(owner, value) }, "schema")
	expressionID := indexLawID(t, func(value identity.ContentID) (model.ExpressionID, bool) {
		return model.IssueExpressionID(owner, value)
	}, "expression")
	operation := indexLawID(t, func(value identity.ContentID) (model.OperationID, bool) {
		return model.IssueOperationID(owner, value)
	}, "operation")
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	row := indexLawID(t, func(value identity.ContentID) (model.RowID, bool) {
		return model.IssueRowID(relation, value)
	}, "row")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("delivery")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	sealedSignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{{
			Relation: relation, Column: columnA, Type: typeID, Presence: signature.RequirePresent,
			Delivery: delivery, Denominator: denominator,
		}},
		Outputs:     []signature.Output{{Relation: relation, Column: columnA, Type: typeID, Presence: signature.ProducePresent, Denominator: denominator}},
		Cardinality: cardinality,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("signature")
	}
	builder := plan.NewBuilder(schemaID)
	typeCapability, capabilityOK := model.NewAscendingCapability(typeID)
	if !capabilityOK || !builder.AddTypeCapability(typeCapability) {
		t.Fatal("type capability")
	}
	if distinctPayload {
		payloadCapability, payloadCapabilityOK := model.NewDecodeOnlyCapability(payloadType)
		if !payloadCapabilityOK || !builder.AddTypeCapability(payloadCapability) {
			t.Fatal("payload type capability")
		}
	}
	keyColumns := []model.ColumnID{columnA}
	if keyWidth == 2 {
		keyColumns = append(keyColumns, columnB)
	}
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{columnA, columnB}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(columnA, typeID)) || !builder.AddColumn(model.DefineColumnSchema(columnB, payloadType)) ||
		!builder.AddKey(model.DefineKeySchema(key, keyColumns)) || !builder.AddScope(model.DefineScopeSchema(scope, nil, region.True())) ||
		!builder.AddExpression(plan.DefineExpressionRef(expressionID, algebra.NewGroup(algebra.NewInput(relation), algebra.NewGroupContract(key, cardinality)))) ||
		!builder.AddSignature(sealedSignature) {
		t.Fatal("schema declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema build")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("certificate: %v", refusal)
	}
	storeID, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store identity")
	}
	addressFence, ok := address.NewFence(schemaID, cert.Digest(), storeID, identity.MountID{0x72}, identity.Generation(1))
	if !ok {
		t.Fatal("address fence")
	}
	inventory := &indexLawInventory{
		fence: addressFence, relation: relation,
		denominator: denominator, rows: []model.RowID{row},
		columns: map[model.ColumnID]uint64{columnA: 1, columnB: 2},
		keys:    map[model.KeyID]uint64{key: 1}, scopes: map[model.ScopeID]uint64{scope: 1},
		expressions: map[model.ExpressionID]uint64{expressionID: 1},
	}
	lineageOwner := indexLawID(t, func(value identity.ContentID) (model.OwnerID, bool) { return model.IssueOwnerID(value) }, "lineage-owner")
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	mounted, ok := witness.Specialize(cert, inventory, indexLawFactory{operation: sealedSignature}, indexLawAlgebraRegistry{algebra: indexLawAlgebra{typeID: typeID}}, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("mounted witness")
	}
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	mask, ok := support.True(manager)
	if !ok {
		t.Fatal("support mask")
	}
	var layout arrangement.Layout
	for _, candidate := range mounted.Arrangement().Layouts() {
		if len(candidate.KeyColumns()) == keyWidth {
			layout = candidate
			break
		}
	}
	if !layout.Available() {
		t.Fatal("keyed layout")
	}
	cofiberAuthority, ok := cofiber.New(mounted, manager, func(value region.Region) (support.Mask, bool) {
		if !value.IsTrue() {
			return support.Mask{}, false
		}
		return support.True(manager)
	})
	if !ok || !cofiberAuthority.Available() {
		t.Fatal("cofiber authority")
	}
	geometryValue, ok := geometry.New(mounted, cofiberAuthority)
	if !ok || !geometryValue.Available() {
		t.Fatal("geometry")
	}
	root, ok := database.Bootstrap(mounted, geometryValue)
	if !ok || !root.Available() {
		t.Fatal("database bootstrap")
	}
	state := root.Store()
	var initial [2]column.Version
	initial[0], ok = state.Column(columnA)
	if !ok || !initial[0].Available() {
		t.Fatal("initial column A")
	}
	initial[1], ok = state.Column(columnB)
	if !ok || !initial[1].Available() {
		t.Fatal("initial column B")
	}
	return indexLawFixture{owner: owner, relation: relation, columns: [2]model.ColumnID{columnA, columnB}, typeID: typeID, payloadType: payloadType, mounted: mounted, layout: layout, state: state, root: root, roots: map[store.Version]database.Version{state: root}, geometry: geometryValue, manager: manager, mask: mask, initial: initial}
}

func (fixture indexLawFixture) value(t *testing.T, label string) binding.ValueToken {
	t.Helper()
	value, ok := fixture.mounted.IssueValue(fixture.typeID, indexLawContent(label))
	if !ok {
		t.Fatal("issue value")
	}
	return value
}

func (fixture indexLawFixture) payloadValue(t *testing.T, label string) binding.ValueToken {
	t.Helper()
	value, ok := fixture.mounted.IssueValue(fixture.payloadType, indexLawContent(label))
	if !ok {
		t.Fatal("issue payload value")
	}
	return value
}

func (fixture indexLawFixture) cell(t *testing.T, value binding.ValueToken) column.Cell {
	t.Helper()
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	cell, ok := column.NewCell(value, presence)
	if !ok {
		t.Fatal("cell")
	}
	return cell
}

func (fixture indexLawFixture) publish(t *testing.T, base store.Version, columns [2]column.Version, values [2]binding.ValueToken, lineages [2]string) (store.Version, store.Delta, [2]column.Version) {
	t.Helper()
	deltas := make([]column.Delta, len(columns))
	var nextColumns [2]column.Version
	for index := range columns {
		lineageRef, ok := model.IssueLineageRef(fixture.owner, indexLawContent(lineages[index]))
		if !ok {
			t.Fatal("lineage")
		}
		update, ok := column.NewUpdate(geometry.Key(0), fixture.mask, fixture.cell(t, values[index]), lineageRef)
		if !ok {
			t.Fatal("update")
		}
		nextColumns[index], deltas[index], ok = columns[index].Next(update)
		if !ok {
			t.Fatal("column successor")
		}
	}
	prepared, ok := store.Prepare(base, deltas...)
	if !ok || !prepared.Available() {
		t.Fatal("aggregate prepare")
	}
	scratch := store.NewReadScratch(fixture.manager)
	baseDatabase, ok := fixture.roots[base]
	if !ok || !baseDatabase.Available() || !baseDatabase.Store().Same(base) {
		t.Fatal("foreign store base")
	}
	databasePrepared, ok := database.Prepare(baseDatabase, prepared, scratch, baseDatabase.ContributionDirectory(), baseDatabase.ContributionState(), nil)
	if !ok || !databasePrepared.Available() {
		t.Fatal("database prepare")
	}
	nextDatabase, delta, ok := database.Commit(databasePrepared)
	if !ok || !nextDatabase.Available() || !delta.Available() {
		t.Fatal("aggregate successor")
	}
	fixture.roots[nextDatabase.Store()] = nextDatabase
	return nextDatabase.Store(), delta.Source(), nextColumns
}

func TestIndexScanLookupEquivalenceAndWarmBorrow(t *testing.T) {
	fixture := newIndexLawFixture(t)
	values := [2]binding.ValueToken{fixture.value(t, "value-a"), fixture.value(t, "value-b")}
	state, _, _ := fixture.publish(t, fixture.state, fixture.initial, values, [2]string{"lineage-a", "lineage-b"})
	scratch := store.NewReadScratch(fixture.manager)
	arrangementValue, ok := index.New(fixture.mounted, state, fixture.layout, fixture.mask, scratch)
	if !ok || !arrangementValue.Available() {
		t.Fatal("construct immutable index")
	}
	borrowed, ok := arrangementValue.Borrow()
	if !ok {
		t.Fatal("borrow index")
	}
	type result struct {
		key    geometry.Key
		region guard.FormulaID
	}
	scan := make([]result, 0)
	completed, valid := borrowed.Scan(func(match index.Match) bool {
		if match.Relation() != fixture.relation {
			t.Fatal("scan relation identity changed")
		}
		logical, rowOK := fixture.mounted.RowAt(match.Relation(), int(match.Key()))
		if !rowOK || logical != match.Row() {
			t.Fatal("scan row directory inverse changed")
		}
		if index, indexOK := fixture.mounted.RowIndex(match.Relation(), match.Row()); !indexOK || index != int(match.Key()) {
			t.Fatal("scan row directory round trip changed")
		}
		identityValue, identityOK := match.Region().Identity()
		if !identityOK {
			t.Fatal("scan region identity")
		}
		scan = append(scan, result{key: match.Key(), region: identityValue})
		return true
	})
	if !completed || !valid || len(scan) != 1 || scan[0].key != geometry.Key(0) {
		t.Fatalf("scan=(%v,%v), results=%v", completed, valid, scan)
	}
	lookup := make([]result, 0, len(scan))
	completed, valid = borrowed.Lookup(values[:], func(match index.Match) bool {
		if match.Relation() != fixture.relation {
			t.Fatal("lookup relation identity changed")
		}
		logical, rowOK := fixture.mounted.RowAt(match.Relation(), int(match.Key()))
		if !rowOK || logical != match.Row() {
			t.Fatal("lookup row directory inverse changed")
		}
		identityValue, identityOK := match.Region().Identity()
		if !identityOK {
			t.Fatal("lookup region identity")
		}
		lookup = append(lookup, result{key: match.Key(), region: identityValue})
		return true
	})
	if !completed || !valid || len(lookup) != len(scan) || lookup[0] != scan[0] {
		t.Fatalf("lookup=%v/%v %v, scan=%v", completed, valid, lookup, scan)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if completed, valid := borrowed.Lookup(values[:], indexLawVisit); !completed || !valid {
			t.Fatal("warm lookup")
		}
	}); allocs != 0 {
		t.Fatalf("warm borrowed lookup allocated %v times", allocs)
	}
}

func TestIndexRejectsForeignTokenAndStaleStoreDelta(t *testing.T) {
	fixture := newIndexLawFixture(t)
	values := [2]binding.ValueToken{fixture.value(t, "value-a"), fixture.value(t, "value-b")}
	state, sourceDelta, _ := fixture.publish(t, fixture.state, fixture.initial, values, [2]string{"lineage-a", "lineage-b"})
	scratch := store.NewReadScratch(fixture.manager)
	arrangementValue, ok := index.New(fixture.mounted, state, fixture.layout, fixture.mask, scratch)
	if !ok {
		t.Fatal("construct index")
	}
	if _, _, ok := arrangementValue.Next(sourceDelta, scratch); ok {
		t.Fatal("stale aggregate delta accepted")
	}
	foreignFence, ok := binding.NewFence(fixture.mounted.RuntimeFence().Schema(), identity.MountID{0x73}, fixture.mounted.RuntimeFence().Generation())
	if !ok {
		t.Fatal("foreign fence")
	}
	foreignIssuer, ok := binding.NewIssuer(foreignFence)
	if !ok {
		t.Fatal("foreign issuer")
	}
	foreign, ok := foreignIssuer.IssueValue(fixture.typeID, indexLawContent("foreign-value"))
	if !ok {
		t.Fatal("foreign value")
	}
	borrowed, ok := arrangementValue.Borrow()
	if !ok {
		t.Fatal("borrow")
	}
	if completed, valid := borrowed.Lookup([]binding.ValueToken{foreign, values[1]}, func(index.Match) bool { return true }); completed || valid {
		t.Fatal("foreign token accepted")
	}
	if _, ok := index.New(witness.Mounted{}, state, fixture.layout, fixture.mask, scratch); ok {
		t.Fatal("zero mounted witness accepted")
	}
}

func TestIndexStableCoordinateMutationRefusesAtomically(t *testing.T) {
	fixture := newIndexLawFixture(t)
	firstValues := [2]binding.ValueToken{fixture.value(t, "value-a"), fixture.value(t, "value-b")}
	state, _, columns := fixture.publish(t, fixture.state, fixture.initial, firstValues, [2]string{"lineage-a", "lineage-b"})
	scratch := store.NewReadScratch(fixture.manager)
	arrangementValue, ok := index.New(fixture.mounted, state, fixture.layout, fixture.mask, scratch)
	if !ok || !arrangementValue.Available() {
		t.Fatal("construct index")
	}
	secondValues := [2]binding.ValueToken{fixture.value(t, "value-c"), fixture.value(t, "value-d")}
	deltas := make([]column.Delta, len(columns))
	for position := range columns {
		lineageRef, lineageOK := model.IssueLineageRef(fixture.owner, indexLawContent("stable-mutation-lineage-"+string(rune('a'+position))))
		if !lineageOK {
			t.Fatal("lineage reference")
		}
		update, updateOK := column.NewUpdate(geometry.Key(0), fixture.mask, fixture.cell(t, secondValues[position]), lineageRef)
		if !updateOK {
			t.Fatal("stable-coordinate update")
		}
		_, deltas[position], updateOK = columns[position].Next(update)
		if !updateOK || !deltas[position].Available() {
			t.Fatal("stable-coordinate column successor")
		}
	}
	prepared, ok := store.Prepare(state, deltas...)
	if !ok || !prepared.Available() {
		t.Fatal("stable-coordinate source prepare")
	}
	baseDatabase, ok := fixture.roots[state]
	if !ok || !baseDatabase.Available() || !baseDatabase.Store().Same(state) {
		t.Fatal("stable-coordinate database base")
	}
	candidate, ok := database.Prepare(baseDatabase, prepared, scratch, baseDatabase.ContributionDirectory(), baseDatabase.ContributionState(), nil)
	if ok || candidate.Available() {
		t.Fatal("stable-coordinate mutation crossed database publication door")
	}
	if !arrangementValue.Source().Same(state) || !arrangementValue.Available() {
		t.Fatal("stable-coordinate refusal damaged predecessor index")
	}
	count := 0
	if completed, valid := arrangementValue.Lookup(firstValues[:], func(match index.Match) bool {
		count++
		if match.Key() != geometry.Key(0) || match.Relation() != fixture.relation {
			t.Fatal("predecessor posting changed after refusal")
		}
		return true
	}); !completed || !valid || count != 1 {
		t.Fatalf("predecessor lookup after refusal=(%v,%v), count=%d", completed, valid, count)
	}
	if len(fixture.roots) != 2 {
		t.Fatalf("refusal published a database successor: roots=%d", len(fixture.roots))
	}
}

func TestIndexNonKeyPayloadLeavesKeyPostingsStable(t *testing.T) {
	fixture := newIndexLawFixtureWithKeyWidth(t, 1)
	firstValues := [2]binding.ValueToken{fixture.value(t, "key-value"), fixture.value(t, "payload-a")}
	state, _, columns := fixture.publish(t, fixture.state, fixture.initial, firstValues, [2]string{"key-lineage", "payload-lineage-a"})
	scratch := store.NewReadScratch(fixture.manager)
	arrangementValue, ok := index.New(fixture.mounted, state, fixture.layout, fixture.mask, scratch)
	if !ok || !arrangementValue.Available() || arrangementValue.Layout().KeyWidth() != 1 {
		t.Fatal("construct non-key payload index")
	}
	secondValues := [2]binding.ValueToken{firstValues[0], fixture.value(t, "payload-b")}
	nextState, sourceDelta, _ := fixture.publish(t, state, columns, secondValues, [2]string{"key-lineage-next", "payload-lineage-b"})
	nextIndex, indexDelta, ok := arrangementValue.Next(sourceDelta, scratch)
	if !ok || !nextIndex.Available() || !indexDelta.Available() || !indexDelta.Empty() || indexDelta.Len() != 0 || !nextIndex.Source().Same(nextState) || !nextIndex.SuccessorOf(arrangementValue) {
		t.Fatal("non-key payload changed the keyed posting root")
	}
	count := 0
	if completed, valid := nextIndex.Lookup([]binding.ValueToken{firstValues[0]}, func(match index.Match) bool {
		count++
		if match.Key() != geometry.Key(0) || match.Relation() != fixture.relation {
			t.Fatal("non-key payload changed mounted coordinate")
		}
		return true
	}); !completed || !valid || count != 1 {
		t.Fatalf("non-key payload lookup=(%v,%v), count=%d", completed, valid, count)
	}
}

func TestIndexKeyedLayoutDoesNotRequirePayloadEquality(t *testing.T) {
	fixture := newIndexLawFixtureWithTypes(t, 1, true)
	if _, ok := fixture.mounted.Equality(fixture.payloadType); ok {
		t.Fatal("decode-only payload unexpectedly owns equality")
	}
	values := [2]binding.ValueToken{fixture.value(t, "key-value"), fixture.payloadValue(t, "payload-value")}
	state, _, _ := fixture.publish(t, fixture.state, fixture.initial, values, [2]string{"key-lineage", "payload-lineage"})
	value, ok := index.New(fixture.mounted, state, fixture.layout, fixture.mask, store.NewReadScratch(fixture.manager))
	if !ok || !value.Available() {
		t.Fatal("keyed layout refused a delivered payload with no equality authority")
	}
}

func TestIndexUnkeyedUsesOnlyDeclaredDeliveredColumns(t *testing.T) {
	fixture := newIndexLawFixture(t)
	values := [2]binding.ValueToken{fixture.value(t, "value-a"), fixture.value(t, "value-b")}
	state, _, _ := fixture.publish(t, fixture.state, fixture.initial, values, [2]string{"lineage-a", "lineage-b"})
	var layout arrangement.Layout
	for _, candidate := range fixture.mounted.Arrangement().Layouts() {
		if len(candidate.KeyColumns()) == 0 && len(candidate.Columns()) != 0 {
			layout = candidate
			break
		}
	}
	if !layout.Available() {
		t.Fatal("unkeyed delivered layout")
	}
	scratch := store.NewReadScratch(fixture.manager)
	arrangementValue, ok := index.New(fixture.mounted, state, layout, fixture.mask, scratch)
	if !ok || !arrangementValue.Available() {
		t.Fatal("construct unkeyed index")
	}
	count := 0
	if completed, valid := arrangementValue.Scan(func(match index.Match) bool {
		count++
		if match.Key() != geometry.Key(0) || match.Relation() != fixture.relation {
			t.Fatal("unkeyed posting fabricated a coordinate")
		}
		return true
	}); !completed || !valid || count != 1 {
		t.Fatalf("unkeyed scan=(%v,%v), count=%d", completed, valid, count)
	}
}

func TestIndexRelationScanEnumeratesMountedOwnerDirectory(t *testing.T) {
	fixture := newIndexLawFixture(t)
	values := [2]binding.ValueToken{fixture.value(t, "directory-value-a"), fixture.value(t, "directory-value-b")}
	state, _, _ := fixture.publish(t, fixture.state, fixture.initial, values, [2]string{"directory-lineage-a", "directory-lineage-b"})
	var layout arrangement.Layout
	for _, candidate := range fixture.mounted.Arrangement().Layouts() {
		if !candidate.Access().Key().Available() && len(candidate.Columns()) == 0 {
			layout = candidate
			break
		}
	}
	if !layout.Available() || layout.KeyWidth() != 0 || len(layout.KeyColumns()) != 0 || len(layout.Columns()) != 0 {
		t.Fatal("relation scan layout")
	}
	arrangementValue, ok := index.New(fixture.mounted, state, layout, fixture.mask, store.NewReadScratch(fixture.manager))
	if !ok || !arrangementValue.Available() {
		t.Fatal("construct relation scan index")
	}
	count := 0
	completed, valid := arrangementValue.Scan(func(match index.Match) bool {
		count++
		if match.Relation() != fixture.relation {
			t.Fatal("relation scan crossed owner relation")
		}
		expected, expectedOK := fixture.mounted.RowAt(fixture.relation, int(match.Key()))
		if !expectedOK || expected != match.Row() || !match.Region().Equal(fixture.mask) {
			t.Fatal("relation scan did not redeem directory row/cofiber")
		}
		return true
	})
	if !completed || !valid || count != 1 {
		t.Fatalf("relation scan=(%v,%v), count=%d", completed, valid, count)
	}
	lookupCount := 0
	if completed, valid := arrangementValue.Lookup(nil, func(match index.Match) bool { lookupCount++; return true }); !completed || !valid || lookupCount != count {
		t.Fatalf("zero-vector lookup=(%v,%v), count=%d", completed, valid, lookupCount)
	}
}

func TestIndexLineageOnlyDeltaSharesTrieRoot(t *testing.T) {
	fixture := newIndexLawFixture(t)
	values := [2]binding.ValueToken{fixture.value(t, "value-a"), fixture.value(t, "value-b")}
	state, _, columns := fixture.publish(t, fixture.state, fixture.initial, values, [2]string{"lineage-a", "lineage-b"})
	scratch := store.NewReadScratch(fixture.manager)
	arrangementValue, ok := index.New(fixture.mounted, state, fixture.layout, fixture.mask, scratch)
	if !ok {
		t.Fatal("construct index")
	}
	var lineageDeltas = make([]column.Delta, 0, len(columns))
	for index := range columns {
		lineageRef, lineageOK := model.IssueLineageRef(fixture.owner, indexLawContent("lineage-only-"+string(rune('a'+index))))
		if !lineageOK {
			t.Fatal("lineage-only reference")
		}
		update, updateOK := column.NewUpdate(geometry.Key(0), fixture.mask, fixture.cell(t, values[index]), lineageRef)
		if !updateOK {
			t.Fatal("lineage-only update")
		}
		nextColumn, delta, nextOK := columns[index].Next(update)
		if !nextOK || !delta.Available() || delta.Empty() || delta.Len() != 1 || !nextColumn.Available() {
			t.Fatal("lineage-only column delta")
		}
		lineageDeltas = append(lineageDeltas, delta)
	}
	prepared, ok := store.Prepare(state, lineageDeltas...)
	if !ok || !prepared.Available() {
		t.Fatal("lineage-only prepare")
	}
	// The external index law cannot redeem a lower store candidate. Build and
	// publish the complete database root so the source delta comes from the
	// canonical publication door.
	scratch = store.NewReadScratch(fixture.manager)
	baseDatabase, ok := fixture.roots[state]
	if !ok || !baseDatabase.Available() || !baseDatabase.Store().Same(state) {
		t.Fatal("database base")
	}
	databasePrepared, ok := database.Prepare(baseDatabase, prepared, scratch, baseDatabase.ContributionDirectory(), baseDatabase.ContributionState(), nil)
	if !ok || !databasePrepared.Available() {
		t.Fatal("database prepare")
	}
	nextDatabase, databaseDelta, ok := database.Commit(databasePrepared)
	nextState, sourceDelta := nextDatabase.Store(), databaseDelta.Source()
	if !ok || !sourceDelta.Available() || len(sourceDelta.SemanticColumnIDs()) != 0 {
		t.Fatal("lineage-only aggregate delta")
	}
	nextIndex, indexDelta, ok := arrangementValue.Next(sourceDelta, scratch)
	if !ok || !indexDelta.Available() || !indexDelta.Empty() || !nextIndex.Available() || !nextIndex.SuccessorOf(arrangementValue) || !nextIndex.Source().Same(nextState) {
		t.Fatal("lineage-only index rebuilt or lost source successor")
	}
}
