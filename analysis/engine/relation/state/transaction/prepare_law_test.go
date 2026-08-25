package transaction_test

import (
	"bytes"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type lawRegion struct{ id identity.ContentID }

func (region lawRegion) Identity() (identity.ContentID, bool) {
	return region.id, region.id.Available()
}
func (region lawRegion) Conjoin(other witness.Region) (witness.Region, bool) {
	if other == nil {
		return nil, false
	}
	id, ok := other.Identity()
	if !ok {
		return nil, false
	}
	// The first declared region is a strict subset of the second; the
	// conjunction therefore remains the first region, matching the physical
	// partial/full masks used by this fixture.
	if region.id == lawContent("region") || id == lawContent("region") {
		return lawRegion{id: lawContent("region")}, true
	}
	return lawRegion{id: id}, true
}
func (region lawRegion) Entails(other witness.Region) bool {
	if other == nil {
		return false
	}
	id, ok := other.Identity()
	if !ok {
		return false
	}
	if region.id == lawContent("region") {
		return id == lawContent("region") || id == lawContent("region-other")
	}
	return id == lawContent("region-other")
}

type lawInventory struct {
	fence        address.Fence
	relations    map[model.RelationID]uint64
	columns      map[model.ColumnID]uint64
	keys         map[model.KeyID]uint64
	scopes       map[model.ScopeID]uint64
	expressions  map[model.ExpressionID]uint64
	dependencies map[model.DependencyID]uint64
	regions      map[model.ScopeID]witness.Region
	denominator  model.DenominatorRef
	rows         []model.RowID
	accesses     []arrangement.Access
}

func (inventory *lawInventory) Fence() address.Fence { return inventory.fence }
func (inventory *lawInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	value, ok := inventory.relations[id]
	return value, ok
}
func (inventory *lawInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	value, ok := inventory.columns[id]
	return value, ok
}
func (inventory *lawInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	value, ok := inventory.keys[id]
	return value, ok
}
func (inventory *lawInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	value, ok := inventory.scopes[id]
	return value, ok
}
func (inventory *lawInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	value, ok := inventory.expressions[id]
	return value, ok
}
func (inventory *lawInventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	value, ok := inventory.dependencies[id]
	return value, ok
}
func (inventory *lawInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, prior := range inventory.accesses {
		if prior.Equal(access) {
			return arrangement.NewHandle(inventory.fence, uint64(index+1))
		}
	}
	inventory.accesses = append(inventory.accesses, access)
	return arrangement.NewHandle(inventory.fence, uint64(len(inventory.accesses)))
}
func (inventory *lawInventory) ScopeRegion(id model.ScopeID) (witness.Region, bool) {
	value, ok := inventory.regions[id]
	return value, ok
}
func (inventory *lawInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != inventory.denominator {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(inventory.rows, lawContent("denominator-evidence"))
}

type lawAlgebra struct{ typeID model.TypeID }

func (algebra lawAlgebra) Type() model.TypeID { return algebra.typeID }
func (algebra lawAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !left.Available() || !right.Available() || left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return binding.ValueToken{}, false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	if bytes.Compare(leftOpaque[:], rightOpaque[:]) >= 0 {
		return left, true
	}
	return right, true
}
func (algebra lawAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return algebra.Join(left, right)
}
func (algebra lawAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	if !left.Available() || !right.Available() || left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	return bytes.Compare(leftOpaque[:], rightOpaque[:]) <= 0
}

type lawRegistry struct{ algebra lawAlgebra }

func (registry lawRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.algebra, registry.algebra.Type() == typeID
}

func (registry lawRegistry) Algebra(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.algebra, registry.algebra.Type() == typeID
}

type lawFactory struct{ operation signature.Signature }

func (factory lawFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || operation.Digest() != factory.operation.Digest() {
		return nil, false
	}
	return lawBinding{operation: factory.operation}, true
}

type lawBinding struct{ operation signature.Signature }

func (bound lawBinding) Signature() signature.Signature { return bound.operation }
func (bound lawBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return nil, false
}

type lawFixture struct {
	owner       model.OwnerID
	relation    model.RelationID
	columns     [2]model.ColumnID
	typeID      model.TypeID
	row         model.RowID
	denominator model.DenominatorRef
	dependency  model.DependencyID
	mounted     witness.Mounted
	geometry    geometry.Geometry
	lineage     lineage.Authority
	base        database.Version
	manager     *guard.Manager
	values      [3]binding.ValueToken
	lineages    [3]model.LineageRef
	tokens      [2]binding.CellToken
	scope       witness.Scope
	otherScope  witness.Scope
	signature   signature.Signature
	buffer      *binding.ProposalBuffer
}

func lawContent(label string) identity.ContentID {
	value, _ := identity.DeriveContentID("relation/state/transaction/law", []byte(label))
	return value
}

func lawID[T any](t *testing.T, issue func(identity.ContentID) (T, bool), label string) T {
	t.Helper()
	value, ok := issue(lawContent(label))
	if !ok {
		t.Fatalf("issue %s", label)
	}
	return value
}

func newLawFixture(t *testing.T) lawFixture {
	t.Helper()
	owner := lawID(t, func(value identity.ContentID) (model.OwnerID, bool) { return model.IssueOwnerID(value) }, "owner")
	lineageOwner := lawID(t, func(value identity.ContentID) (model.OwnerID, bool) { return model.IssueOwnerID(value) }, "lineage-owner")
	relation := lawID(t, func(value identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, value) }, "relation")
	columnA := lawID(t, func(value identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, value) }, "column-a")
	columnB := lawID(t, func(value identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, value) }, "column-b")
	typeID := lawID(t, func(value identity.ContentID) (model.TypeID, bool) { return model.IssueTypeID(owner, value) }, "type")
	key := lawID(t, func(value identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(relation, value) }, "key")
	scopeID := lawID(t, func(value identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, value) }, "scope")
	otherScopeID := lawID(t, func(value identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, value) }, "scope-other")
	expression := lawID(t, func(value identity.ContentID) (model.ExpressionID, bool) {
		return model.IssueExpressionID(owner, value)
	}, "expression")
	dependency := lawID(t, func(value identity.ContentID) (model.DependencyID, bool) {
		return model.IssueDependencyID(owner, value)
	}, "dependency")
	schemaID := lawID(t, func(value identity.ContentID) (model.SchemaID, bool) { return model.IssueSchemaID(owner, value) }, "schema")
	row := lawID(t, func(value identity.ContentID) (model.RowID, bool) { return model.IssueRowID(relation, value) }, "row")
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	ref, ok := plan.NewRelationRef(relation)
	if !ok {
		t.Fatal("relation ref")
	}
	dependencyRef := plan.DefineDependencyRef(dependency)
	expressionRef := plan.DefineExpressionRef(expression, algebra.NewProject(
		algebra.NewInput(relation),
		algebra.NewProjectContract(relation, []algebra.ColumnMapping{
			algebra.NewColumnMapping(columnA, columnA),
			algebra.NewColumnMapping(columnB, columnB),
		}, key),
	))
	dependencyValue := plan.DefineDependency(dependency, expression, []plan.RelationRef{ref}, []plan.RelationRef{ref}, "self")
	edge := plan.DefineDependencyEdge(dependencyRef, dependencyRef)
	head := plan.DefineWideningHead(dependencyRef, ref)
	scc := plan.DefineSCC([]plan.DependencyRef{dependencyRef}, []plan.DependencyEdge{edge}, plan.DefineRecurrence(plan.Positive, []plan.WideningHead{head}))
	operation := lawID(t, func(value identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, value) }, "operation")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	sealedSignature, ok := signature.Seal(signature.Spec{Identity: signature.Identity{Operation: operation, Version: 1}, Fence: signature.Fence{Owner: owner, Schema: schemaID}, Outputs: []signature.Output{{Relation: relation, Column: columnA, Type: typeID, Presence: signature.ProducePresent}, {Relation: relation, Column: columnB, Type: typeID, Presence: signature.ProducePresent}}, Authority: signature.OutputAuthority{Denominator: denominator}, Cardinality: cardinality, Outcomes: outcomes})
	if !ok {
		t.Fatal("signature")
	}
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{columnA, columnB}, []model.KeyID{key}, scopeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(columnA, typeID)) || !builder.AddColumn(model.DefineColumnSchema(columnB, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{columnA})) || !builder.AddScope(model.DefineScopeSchema(scopeID, nil)) ||
		!builder.AddScope(model.DefineScopeSchema(otherScopeID, nil)) || !builder.AddExpression(expressionRef) || !builder.AddDependency(dependencyValue) || !builder.AddSCC(scc) || !builder.AddSignature(sealedSignature) {
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
	addressFence, ok := address.NewFence(schemaID, cert.Digest(), storeID, identity.MountID{0x61}, identity.Generation(1))
	if !ok {
		t.Fatal("address fence")
	}
	inventory := &lawInventory{
		fence: addressFence, relations: map[model.RelationID]uint64{relation: 1},
		columns: map[model.ColumnID]uint64{columnA: 1, columnB: 2}, keys: map[model.KeyID]uint64{key: 1},
		scopes: map[model.ScopeID]uint64{scopeID: 1, otherScopeID: 2}, expressions: map[model.ExpressionID]uint64{expression: 1}, dependencies: map[model.DependencyID]uint64{dependency: 1},
		regions: map[model.ScopeID]witness.Region{scopeID: lawRegion{id: lawContent("region")}, otherScopeID: lawRegion{id: lawContent("region-other")}}, denominator: denominator, rows: []model.RowID{row},
	}
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	mounted, ok := witness.Specialize(cert, inventory, lawFactory{operation: sealedSignature}, lawRegistry{algebra: lawAlgebra{typeID: typeID}}, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("mounted witness")
	}
	scope, ok := mounted.Scope(scopeID)
	if !ok {
		t.Fatal("scope")
	}
	otherScope, ok := mounted.Scope(otherScopeID)
	if !ok {
		t.Fatal("other scope")
	}
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal("guards", err)
	}
	full, ok := support.True(manager)
	if !ok {
		t.Fatal("full mask")
	}
	work := support.New(manager)
	partial, ok := work.Literal(1, true)
	if !ok || !work.Seal() {
		t.Fatal("partial mask")
	}
	scopes, ok := cofiber.New(mounted, manager, func(region witness.Region) (support.Mask, bool) {
		id, ok := region.Identity()
		if !ok {
			return support.Mask{}, false
		}
		if id == lawContent("region") {
			return partial, true
		}
		return full, true
	})
	if !ok {
		t.Fatal("cofiber authority")
	}
	view, ok := geometry.New(mounted, scopes)
	if !ok {
		t.Fatal("geometry")
	}
	initial := make([]column.Version, 0, 2)
	for _, columnID := range []model.ColumnID{columnA, columnB} {
		owned, ok := column.NewColumn(model.DefineColumnSchema(columnID, typeID), mounted.RuntimeFence(), manager)
		if !ok {
			t.Fatal("column")
		}
		initial = append(initial, owned.Initial())
	}
	baseStore, ok := store.NewVersion(mounted, initial)
	if !ok {
		t.Fatal("base store")
	}
	base, ok := database.New(mounted, baseStore, view, store.NewReadScratch(manager))
	if !ok {
		t.Fatal("base aggregate")
	}
	lineageAuthority, ok := mounted.Lineage()
	if !ok {
		t.Fatal("lineage authority")
	}
	valueIDs := [3]identity.ContentID{lawContent("value-low"), lawContent("value-high"), lawContent("value-lower")}
	valueIDs[0][0], valueIDs[1][0], valueIDs[2][0] = 2, 3, 1
	values := [3]binding.ValueToken{}
	for index, valueID := range valueIDs {
		values[index], ok = mounted.IssueValue(typeID, valueID)
		if !ok {
			t.Fatal("value")
		}
	}
	witnessValue, ok := mounted.Denominator(denominator)
	if !ok {
		t.Fatal("denominator witness")
	}
	scopeToken, ok := mounted.ScopeToken(scope)
	if !ok {
		t.Fatal("scope token")
	}
	tokens := [2]binding.CellToken{}
	otherTokens := [2]binding.CellToken{}
	for index, columnID := range []model.ColumnID{columnA, columnB} {
		tokens[index], ok = mounted.IssueCell(denominator, scope, columnID, row)
		if !ok {
			t.Fatal("token")
		}
		otherTokens[index], ok = mounted.IssueCell(denominator, otherScope, columnID, row)
		if !ok {
			t.Fatal("other token")
		}
	}
	lineages := [3]model.LineageRef{}
	for index, label := range []string{"lineage-low", "lineage-high", "lineage-lower"} {
		lineages[index], ok = model.IssueLineageRef(owner, lawContent(label))
		if !ok {
			t.Fatal("lineage")
		}
	}
	buffer, ok := binding.NewProposalBuffer(sealedSignature, mounted.RuntimeFence(), witnessValue, scopeToken)
	if !ok {
		t.Fatal("buffer")
	}
	return lawFixture{owner: owner, relation: relation, columns: [2]model.ColumnID{columnA, columnB}, typeID: typeID, row: row, denominator: denominator, dependency: dependency, mounted: mounted, geometry: view, lineage: lineageAuthority, base: base, manager: manager, values: values, lineages: lineages, tokens: tokens, scope: scope, otherScope: otherScope, signature: sealedSignature, buffer: &buffer}
}

type lawMapper func(witness.Region) (support.Mask, bool)

func (mapper lawMapper) Map(region witness.Region) (support.Mask, bool) {
	if mapper == nil {
		return support.Mask{}, false
	}
	return mapper(region)
}

func (fixture *lawFixture) batch(t *testing.T, scope witness.Scope, values []int, columns []int) (binding.ProposalBatch, []transaction.Submission) {
	t.Helper()
	if !fixture.buffer.Reset() && fixture.buffer.Len() != 0 {
		t.Fatal("reset buffer")
	}
	scopeToken, ok := fixture.mounted.ScopeToken(scope)
	if !ok {
		t.Fatal("scope token")
	}
	witnessValue, ok := fixture.mounted.Denominator(fixture.denominator)
	if !ok {
		t.Fatal("witness")
	}
	*fixture.buffer, ok = binding.NewProposalBuffer(fixture.signature, fixture.mounted.RuntimeFence(), witnessValue, scopeToken)
	if !ok {
		t.Fatal("new batch buffer")
	}
	presence, _ := model.NewPresence(model.Present)
	sidecars := make([]transaction.Submission, 0, len(columns))
	for index, columnIndex := range columns {
		valueIndex := values[index]
		token := fixture.tokens[columnIndex]
		if scope != fixture.scope {
			token, ok = fixture.mounted.IssueCell(fixture.denominator, scope, fixture.columns[columnIndex], fixture.row)
			if !ok {
				t.Fatal("issue scoped token")
			}
		}
		proposal, issueOK := binding.NewProposal(token, fixture.values[valueIndex], presence)
		if !issueOK || !fixture.buffer.Append(proposal) {
			t.Fatal("append")
		}
		sidecars = append(sidecars, transaction.Submission{Lineage: fixture.lineages[valueIndex]})
	}
	batch, ok := fixture.buffer.Seal(outcome.Result{Code: outcome.Produced})
	if !ok {
		t.Fatal("seal")
	}
	return batch, sidecars
}

// publish is test orchestration only: production has no compatibility
// path. Laws explicitly exercise Prepare's no-publication contract followed
// by the aggregate root's sole publication operation.
func publish(
	base database.Version,
	view geometry.Geometry,
	readScratch *store.ReadScratch,
	batch transaction.SubmissionBatch,
) (database.Version, database.Delta, bool) {
	prepared, ok := transaction.Prepare(base, view, readScratch, batch)
	if !ok {
		return database.Version{}, database.Delta{}, false
	}
	return database.Commit(prepared)
}

func TestPrepareDoesNotPublishBeforeCommit(t *testing.T) {
	fixture := newLawFixture(t)
	layouts := fixture.mounted.Arrangement().Layouts()
	if len(fixture.base.Layouts()) != len(layouts) || len(fixture.base.Indexes()) != len(layouts) {
		t.Fatalf("aggregate layout census drifted: mounted=%d root-layouts=%d indexes=%d", len(layouts), len(fixture.base.Layouts()), len(fixture.base.Indexes()))
	}
	for position, layout := range layouts {
		if candidate, ok := fixture.base.Index(layout); !ok || !candidate.Available() || !candidate.Layout().Equal(layout) {
			t.Fatalf("aggregate missing mounted layout %d", position)
		}
	}
	batch, sidecars := fixture.batch(t, fixture.scope, []int{0}, []int{0})
	prepared, ok := transaction.Prepare(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: batch, Sidecars: sidecars})
	if !ok || !prepared.Available() || prepared.Empty() || !prepared.Base().Same(fixture.base) {
		t.Fatal("prepare did not retain a private exact candidate")
	}
	if len(prepared.CandidateIndexes()) != len(layouts) || !prepared.SemanticChanged() || !prepared.LineageChanged() {
		t.Fatal("prepared candidate lost exact index roots or semantic/lineage flags")
	}
	if fixture.base.Revision() != 1 {
		t.Fatal("prepare changed the published base")
	}
	next, delta, ok := database.Commit(prepared)
	if !ok || !delta.Available() || !next.SuccessorOf(fixture.base) || !delta.Base().Same(fixture.base) || !delta.Next().Same(next) {
		t.Fatal("commit did not publish the prepared candidate")
	}
	if len(delta.Indexes()) != len(layouts) || !delta.SemanticChanged() || !delta.LineageChanged() {
		t.Fatal("aggregate delta lost complete index successor vector")
	}
	for position, child := range delta.Indexes() {
		layout := layouts[position]
		relevant := false
		for _, id := range append(layout.KeyColumns(), layout.Columns()...) {
			if id == fixture.columns[0] {
				relevant = true
				break
			}
		}
		if !relevant && !child.Empty() {
			t.Fatalf("unrelated layout %d rebuilt for a column-A-only change", position)
		}
	}
}

func TestPrepareNormalizesGuardedOverlapAndNoOpSharesBase(t *testing.T) {
	fixture := newLawFixture(t)
	first, firstSidecars := fixture.batch(t, fixture.scope, []int{0}, []int{0})
	seed, seedDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: first, Sidecars: firstSidecars})
	if !ok || !seedDelta.Available() {
		t.Fatal("seed")
	}
	second, secondSidecars := fixture.batch(t, fixture.otherScope, []int{2}, []int{0})
	next, delta, ok := publish(seed, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: second, Sidecars: secondSidecars})
	if !ok || !delta.Available() || !next.SuccessorOf(seed) {
		t.Fatal("overlap apply")
	}
	fullToken, ok := fixture.mounted.IssueCell(fixture.denominator, fixture.otherScope, fixture.columns[0], fixture.row)
	if !ok {
		t.Fatal("full-scope token")
	}
	coordinate, ok := fixture.geometry.Resolve(fullToken)
	if !ok {
		t.Fatal("coordinate")
	}
	readScratch := store.NewReadScratch(fixture.manager)
	columnScratch := column.NewReadScratch(fixture.manager)
	parts := 0
	completed, valid := next.Store().Column(fixture.columns[0])
	if !valid {
		t.Fatal("column")
	}
	completedOK, valid := completed.Read(coordinate.Dense(), coordinate.Mask(), columnScratch, func(part column.ReadPart) bool { parts++; return true })
	if !completedOK || !valid || parts < 2 {
		t.Fatalf("multi-terminal read completed:%v valid:%v parts:%d", completedOK, valid, parts)
	}
	noOp, noDelta, ok := publish(next, fixture.geometry, readScratch, transaction.SubmissionBatch{Proposals: second, Sidecars: secondSidecars})
	if !ok || !noOp.Same(next) || noDelta.Available() {
		t.Logf("noop: ok=%v noOp=%v next=%v delta=%v", ok, noOp.Available(), next.Available(), noDelta.Available())
		t.Fatal("no-op did not share exact base")
	}
}

func TestPreparePermutationAndMultiColumnAtomicity(t *testing.T) {
	fixture := newLawFixture(t)
	forward, forwardSidecars := fixture.batch(t, fixture.scope, []int{0, 1}, []int{0, 1})
	first, firstDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: forward, Sidecars: forwardSidecars})
	if !ok || !firstDelta.Available() {
		t.Fatal("forward multi-column apply")
	}
	reverse, reverseSidecars := fixture.batch(t, fixture.scope, []int{1, 0}, []int{1, 0})
	second, secondDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: reverse, Sidecars: reverseSidecars})
	if !ok || !secondDelta.Available() || !first.SuccessorOf(fixture.base) || !second.SuccessorOf(fixture.base) {
		t.Fatal("permuted apply")
	}
	left, right := firstDelta.ChangedColumnIDs(), secondDelta.ChangedColumnIDs()
	if len(left) != 2 || len(right) != 2 || left[0] != right[0] || left[1] != right[1] {
		t.Fatal("column delta order changed under permutation")
	}
	invalid, invalidSidecars := fixture.batch(t, fixture.scope, []int{0, 1}, []int{0, 1})
	// A valid first proposal is paired with an invalid second proof sidecar;
	// no first-column candidate may publish before the whole batch validates.
	invalidSidecars[1].Lineage = model.LineageRef{}
	unchanged, noDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: invalid, Sidecars: invalidSidecars})
	if ok || noDelta.Available() || unchanged.Available() {
		t.Fatal("invalid second candidate changed aggregate outcome")
	}
}

func TestPrepareWidenLineageAndStaleLeaseLaws(t *testing.T) {
	fixture := newLawFixture(t)
	seed, seedSidecars := fixture.batch(t, fixture.scope, []int{0}, []int{0})
	base, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: seed, Sidecars: seedSidecars})
	if !ok {
		t.Fatal("seed")
	}
	widen, widenSidecars := fixture.batch(t, fixture.scope, []int{1}, []int{0})
	permit, permitOK := fixture.mounted.Widening(fixture.dependency, fixture.relation)
	if !permitOK || !permit.Available() {
		t.Fatal("widening permit")
	}
	widened, widenDelta, ok := publish(base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: widen, Sidecars: widenSidecars, Widening: permit})
	if !ok || !widenDelta.Available() || !widened.SuccessorOf(base) {
		t.Fatal("authorized widening refused")
	}
	foreignDependency := lawID(t, func(value identity.ContentID) (model.DependencyID, bool) {
		return model.IssueDependencyID(fixture.owner, value)
	}, "foreign-dependency")
	if _, ok := fixture.mounted.Widening(foreignDependency, fixture.relation); ok {
		t.Fatal("foreign widening permit was issued")
	}
	lineageBatch, lineageSidecars := fixture.batch(t, fixture.scope, []int{0}, []int{0})
	lineageSidecars[0].Lineage = fixture.lineages[1]
	lineageNext, lineageDelta, ok := publish(base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: lineageBatch, Sidecars: lineageSidecars})
	if !ok || !lineageDelta.Available() || len(lineageDelta.SemanticColumnIDs()) != 0 || len(lineageDelta.LineageColumnIDs()) != 1 || !lineageNext.SuccessorOf(base) {
		t.Fatal("lineage-only successor missing")
	}
	for position, child := range lineageDelta.Indexes() {
		if !child.Empty() || !lineageNext.Indexes()[position].SuccessorOf(base.Indexes()[position]) {
			t.Fatalf("lineage-only update rebuilt or lost index root %d", position)
		}
	}
	stale, staleSidecars := fixture.batch(t, fixture.scope, []int{1}, []int{0})
	if !fixture.buffer.Reset() {
		t.Fatal("reset")
	}
	if _, _, ok := publish(base, fixture.geometry, store.NewReadScratch(fixture.manager), transaction.SubmissionBatch{Proposals: stale, Sidecars: staleSidecars}); ok {
		t.Fatal("stale proposal batch accepted")
	}
}

func TestPreparePresenceConflictIsAtomic(t *testing.T) {
	fixture := newLawFixture(t)
	// An ordinary present proposal is invalid against an authenticated proven
	// absence in column B; the valid column A proposal must not publish alone.
	manager := fixture.manager
	mask, ok := support.True(manager)
	if !ok {
		t.Fatal("mask")
	}
	absent, ok := model.NewPresence(model.ProvenAbsent)
	if !ok {
		t.Fatal("absence")
	}
	absentCell, ok := column.NewCell(binding.ValueToken{}, absent)
	if !ok {
		t.Fatal("absent cell")
	}
	lineage, ok := model.IssueLineageRef(fixture.owner, lawContent("absent-lineage"))
	if !ok {
		t.Fatal("absence lineage")
	}
	coordinate, ok := fixture.geometry.Resolve(fixture.tokens[1])
	if !ok {
		t.Fatal("column B coordinate")
	}
	update, ok := column.NewUpdate(coordinate.Dense(), mask, absentCell, lineage)
	if !ok {
		t.Fatal("absence update")
	}
	columnB, ok := fixture.base.Store().Column(fixture.columns[1])
	if !ok {
		t.Fatal("column B")
	}
	seededColumn, seededDelta, ok := columnB.Next(update)
	if !ok || !seededDelta.Available() {
		t.Fatal("seed absence")
	}
	seededPrepared, ok := store.Prepare(fixture.base.Store(), seededDelta)
	var seededStore store.Version
	var aggregateDelta store.Delta
	if ok {
		seededStore, aggregateDelta, ok = store.Commit(seededPrepared)
	}
	if !ok || !aggregateDelta.Available() {
		t.Fatal("seed aggregate")
	}
	seeded, ok := database.New(fixture.mounted, seededStore, fixture.geometry, store.NewReadScratch(manager))
	if !ok {
		t.Fatal("seed aggregate root")
	}
	_ = seededColumn
	batch, sidecars := fixture.batch(t, fixture.scope, []int{0, 1}, []int{0, 1})
	if next, delta, ok := publish(seeded, fixture.geometry, store.NewReadScratch(manager), transaction.SubmissionBatch{Proposals: batch, Sidecars: sidecars}); ok || delta.Available() || next.Available() {
		t.Fatal("presence conflict published partial aggregate")
	}
	if !seeded.Same(seeded) {
		t.Fatal("seed aggregate identity was not stable")
	}
}

func TestLineageJoinAuthorityIsPermutationStable(t *testing.T) {
	fixture := newLawFixture(t)
	values := append([]model.LineageRef(nil), fixture.lineages[:]...)
	sort.Slice(values, func(left, right int) bool {
		l, r := values[left].Content(), values[right].Content()
		return bytes.Compare(l[:], r[:]) < 0
	})
	if values[0] == values[1] || values[1] == values[2] {
		t.Fatal("lineage fixture not distinct")
	}
	// The public Prepare path folds these sidecars in canonical order; this law
	// keeps the fake authority explicitly associative/commutative/idempotent.
	left, _ := fixture.lineage.Join(values[0], values[1])
	left, _ = fixture.lineage.Join(left, values[2])
	right, _ := fixture.lineage.Join(values[2], values[0])
	right, _ = fixture.lineage.Join(right, values[1])
	if left != right {
		t.Fatal("lineage authority was not permutation stable")
	}
}
