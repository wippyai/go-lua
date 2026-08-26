package transaction

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
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type lawInventory struct {
	fence        address.Fence
	relations    map[model.RelationID]uint64
	columns      map[model.ColumnID]uint64
	keys         map[model.KeyID]uint64
	scopes       map[model.ScopeID]uint64
	expressions  map[model.ExpressionID]uint64
	dependencies map[model.DependencyID]uint64
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
func (inventory *lawInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != inventory.denominator {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(inventory.rows, lawContent("denominator-evidence"))
}
func (inventory *lawInventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
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

type lawApplyWorker struct {
	result    outcome.Result
	proposals []binding.Proposal
}

func (worker *lawApplyWorker) Evaluate(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if worker == nil || buffer == nil || !worker.result.Available() {
		return outcome.Result{}
	}
	if worker.result.Code == outcome.Produced {
		for _, proposal := range worker.proposals {
			if !buffer.Append(proposal) {
				return outcome.Result{}
			}
		}
	}
	return worker.result
}

type lawFactory struct {
	operation signature.Signature
	worker    *lawApplyWorker
}

func (factory lawFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || operation.Digest() != factory.operation.Digest() {
		return nil, false
	}
	return lawBinding{operation: factory.operation, worker: factory.worker}, true
}

type lawBinding struct {
	operation signature.Signature
	worker    *lawApplyWorker
}

func (bound lawBinding) Signature() signature.Signature { return bound.operation }
func (bound lawBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return bound.worker, bound.worker != nil
}

type lawFixture struct {
	owner        model.OwnerID
	relation     model.RelationID
	columns      [2]model.ColumnID
	contribution output.ContributionSpec
	typeID       model.TypeID
	row          model.RowID
	otherRow     model.RowID
	denominator  model.DenominatorRef
	dependency   model.DependencyID
	mounted      witness.Mounted
	geometry     geometry.Geometry
	lineage      lineage.Authority
	base         database.Version
	manager      *guard.Manager
	values       [3]binding.ValueToken
	lineages     [3]model.LineageRef
	tokens       [2]binding.CellToken
	scope        witness.Scope
	otherScope   witness.Scope
	signature    signature.Signature
	buffer       *binding.ProposalBuffer
	applyWorker  *lawApplyWorker
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
	return newLawFixtureWithOutputPresence(t, signature.ProducePresent)
}

func newLawFixtureWithOutputPresence(t *testing.T, outputPresence signature.PresenceContract) lawFixture {
	return newLawFixtureWithOutputPresenceAndPublish(t, outputPresence, true)
}

// newLawFixtureWithOutputPresenceAndPublish retains the same sealed semantic
// catalogue while allowing the opaque-key bootstrap law to omit the Publish
// expression. Its signature still admits the declared keyed layout, but no
// checked key operation admits an equality witness for DecodeOnly values.
func newLawFixtureWithOutputPresenceAndPublish(t *testing.T, outputPresence signature.PresenceContract, withPublish bool) lawFixture {
	return newLawFixtureWithOutputPresenceAndPublishAndContribution(t, outputPresence, withPublish, false)
}

func newContributionLawFixture(t *testing.T) lawFixture {
	return newLawFixtureWithOutputPresenceAndPublishAndContribution(t, signature.ProducePresent, true, true)
}

func newLawFixtureWithOutputPresenceAndPublishAndContribution(t *testing.T, outputPresence signature.PresenceContract, withPublish, withContribution bool) lawFixture {
	return newLawFixtureWithOutputPresenceAndPublishAndContributionRows(t, outputPresence, withPublish, withContribution, false)
}

func newLawFixtureWithOutputPresenceAndPublishAndContributionRows(t *testing.T, outputPresence signature.PresenceContract, withPublish, withContribution, withSecondRow bool) lawFixture {
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
	var otherRow model.RowID
	if withSecondRow {
		otherRow = lawID(t, func(value identity.ContentID) (model.RowID, bool) { return model.IssueRowID(relation, value) }, "row-other")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	ref, ok := plan.NewRelationRef(relation)
	if !ok {
		t.Fatal("relation ref")
	}
	dependencyRef := plan.DefineDependencyRef(dependency)
	operation := lawID(t, func(value identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, value) }, "operation")
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	applyExpression := algebra.NewApply([]algebra.Expression{algebra.NewInput(relation)}, algebra.NewApplyContract(
		signature.Identity{Operation: operation, Version: 1}, []algebra.SlotSource{
			algebra.NewSlotSource(0, 0), algebra.NewSlotSource(0, 1),
		}, algebra.OwnerNamed()))
	var expressionValue algebra.Expression = algebra.NewInput(relation)
	if withPublish {
		expressionValue = algebra.NewPublish(applyExpression, algebra.NewPublishContract(relation, key))
	}
	expressionRef := plan.DefineExpressionRef(expression, expressionValue)
	dependencyValue := plan.DefineDependency(dependency, expression, []plan.RelationRef{ref}, []plan.RelationRef{ref}, "self")
	edge := plan.DefineDependencyEdge(dependencyRef, dependencyRef)
	head := plan.DefineWideningHead(dependencyRef, ref)
	scc := plan.DefineSCC([]plan.DependencyRef{dependencyRef}, []plan.DependencyEdge{edge}, plan.DefineRecurrence(plan.Positive, []plan.WideningHead{head}))
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	regionAtom, ok := region.NewAtom(lawContent("region"))
	if !ok {
		t.Fatal("scope region atom")
	}
	scopeRegion, ok := region.FromAtom(regionAtom)
	if !ok {
		t.Fatal("scope region")
	}
	scopeRegionIdentity := scopeRegion.Identity()
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	sealedSignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1}, Fence: signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: relation, Column: columnA, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator},
			{Relation: relation, Column: columnB, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator},
		},
		Outputs:     []signature.Output{{Relation: relation, Column: columnA, Type: typeID, Presence: outputPresence, Denominator: denominator}, {Relation: relation, Column: columnB, Type: typeID, Presence: outputPresence, Denominator: denominator}},
		Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("signature")
	}
	var contribution output.ContributionSpec
	if withContribution {
		capability, capabilityOK := model.NewAscendingCapability(typeID)
		if !capabilityOK {
			t.Fatal("contribution capability")
		}
		contribution, ok = output.Seal(output.Spec{
			Signature: sealedSignature,
			Port:      output.OutputPort{Operation: sealedSignature.Identity(), Column: columnB},
			ValueType: typeID,
			Algebra:   capability,
			Reducer:   output.Contributions,
		})
		if !ok {
			t.Fatal("contribution")
		}
	}
	builder := plan.NewBuilder(schemaID)
	var typeCapability model.TypeCapability
	if outputPresence == signature.ProduceOpaque {
		typeCapability, ok = model.NewDecodeOnlyCapability(typeID)
	} else {
		typeCapability, ok = model.NewAscendingCapability(typeID)
	}
	if !ok || !builder.AddTypeCapability(typeCapability) {
		t.Fatal("type capability")
	}
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{columnA, columnB}, []model.KeyID{key}, scopeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(columnA, typeID)) || !builder.AddColumn(model.DefineColumnSchema(columnB, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{columnA})) || !builder.AddScope(model.DefineScopeSchema(scopeID, nil, scopeRegion)) ||
		!builder.AddScope(model.DefineScopeSchema(otherScopeID, nil, region.True())) || !builder.AddExpression(expressionRef) || !builder.AddSignature(sealedSignature) {
		t.Fatal("schema declarations")
	}
	if withPublish && (!builder.AddDependency(dependencyValue) || !builder.AddSCC(scc)) {
		t.Fatal("recurrence declarations")
	}
	if withContribution && !builder.AddContribution(contribution) {
		t.Fatal("contribution declaration")
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
		denominator: denominator, rows: func() []model.RowID {
			rows := []model.RowID{row}
			if withSecondRow {
				rows = append(rows, otherRow)
			}
			return rows
		}(),
	}
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	var algebraRegistry binding.AlgebraRegistry
	present, presentOK := model.NewPresence(model.Present)
	if !presentOK {
		t.Fatal("present presence")
	}
	if outputPresence.Allows(present) {
		algebraRegistry = lawRegistry{algebra: lawAlgebra{typeID: typeID}}
	}
	applyWorker := &lawApplyWorker{}
	mounted, ok := witness.Specialize(cert, inventory, lawFactory{operation: sealedSignature, worker: applyWorker}, algebraRegistry, lineageFactory)
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
	scopes, ok := cofiber.New(mounted, manager, func(value region.Region) (support.Mask, bool) {
		if value.Identity() == scopeRegionIdentity {
			return partial, true
		}
		if value.IsTrue() {
			return full, true
		}
		return support.Mask{}, false
	})
	if !ok {
		t.Fatal("cofiber authority")
	}
	view, ok := geometry.New(mounted, scopes)
	if !ok {
		t.Fatal("geometry")
	}
	base, ok := database.Bootstrap(mounted, view)
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
		tokens[index], ok = mounted.IssueCell(witnessValue, scope, columnID, row)
		if !ok {
			t.Fatal("token")
		}
		otherTokens[index], ok = mounted.IssueCell(witnessValue, otherScope, columnID, row)
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
	buffer, ok := binding.NewProposalBuffer(sealedSignature, mounted.RuntimeFence(), []binding.DenominatorWitness{witnessValue}, scopeToken, binding.NewOwnerNamedDestination(witnessValue.Relation()))
	if !ok {
		t.Fatal("buffer")
	}
	return lawFixture{owner: owner, relation: relation, columns: [2]model.ColumnID{columnA, columnB}, contribution: contribution, typeID: typeID, row: row, otherRow: otherRow, denominator: denominator, dependency: dependency, mounted: mounted, geometry: view, lineage: lineageAuthority, base: base, manager: manager, values: values, lineages: lineages, tokens: tokens, scope: scope, otherScope: otherScope, signature: sealedSignature, buffer: &buffer, applyWorker: applyWorker}
}

func (fixture *lawFixture) batch(t *testing.T, scope witness.Scope, values []int, columns []int) (binding.ProposalBatch, model.LineageRef) {
	return fixture.batchWithPresence(t, scope, values, columns, model.Present)
}

func (fixture *lawFixture) batchWithPresence(t *testing.T, scope witness.Scope, values []int, columns []int, kind model.PresenceKind) (binding.ProposalBatch, model.LineageRef) {
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
	*fixture.buffer, ok = binding.NewProposalBuffer(fixture.signature, fixture.mounted.RuntimeFence(), []binding.DenominatorWitness{witnessValue}, scopeToken, binding.NewOwnerNamedDestination(witnessValue.Relation()))
	if !ok {
		t.Fatal("new batch buffer")
	}
	presence, presenceOK := model.NewPresence(kind)
	if !presenceOK {
		t.Fatal("presence")
	}
	if len(values) == 0 || len(values) != len(columns) {
		t.Fatal("lineage batch shape")
	}
	provenance := fixture.lineages[values[0]]
	for _, valueIndex := range values[1:] {
		joined, joinOK := fixture.lineage.Join(provenance, fixture.lineages[valueIndex])
		if !joinOK {
			t.Fatal("batch lineage")
		}
		provenance = joined
	}
	for index, columnIndex := range columns {
		valueIndex := values[index]
		token := fixture.tokens[columnIndex]
		if scope != fixture.scope {
			token, ok = fixture.mounted.IssueCell(witnessValue, scope, fixture.columns[columnIndex], fixture.row)
			if !ok {
				t.Fatal("issue scoped token")
			}
		}
		value := fixture.values[valueIndex]
		if !presence.Is(model.Present) && !presence.Is(model.AuthenticatedOpaque) {
			value = binding.ValueToken{}
		}
		proposal, issueOK := binding.NewProposal(token, value, presence)
		if !issueOK || !fixture.buffer.Append(proposal) {
			t.Fatal("append")
		}
	}
	batch, ok := fixture.buffer.Seal(outcome.Result{Code: outcome.Produced})
	if !ok {
		t.Fatal("seal")
	}
	return batch, provenance
}

func (fixture *lawFixture) removalBatch(t *testing.T, scope witness.Scope, columns []int) binding.ProposalBatch {
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
	*fixture.buffer, ok = binding.NewProposalBuffer(fixture.signature, fixture.mounted.RuntimeFence(), []binding.DenominatorWitness{witnessValue}, scopeToken, binding.NewOwnerNamedDestination(witnessValue.Relation()))
	if !ok {
		t.Fatal("new removal buffer")
	}
	for _, columnIndex := range columns {
		token := fixture.tokens[columnIndex]
		if scope != fixture.scope {
			token, ok = fixture.mounted.IssueCell(witnessValue, scope, fixture.columns[columnIndex], fixture.row)
			if !ok {
				t.Fatal("issue scoped removal token")
			}
		}
		proposal, issueOK := binding.NewRemovalProposal(token)
		if !issueOK || !fixture.buffer.Append(proposal) {
			t.Fatal("append removal")
		}
	}
	batch, ok := fixture.buffer.Seal(outcome.Result{Code: outcome.Produced})
	if !ok {
		t.Fatal("seal removal")
	}
	return batch
}

func contributionAddress(t *testing.T, fixture lawFixture, source model.RowID) invocation.InvocationAddress {
	t.Helper()
	scopeToken, ok := fixture.mounted.ScopeToken(fixture.scope)
	if !ok {
		t.Fatal("contribution scope token")
	}
	tuple, ok := invocation.NewTupleSources([]model.RowID{source})
	if !ok {
		t.Fatal("contribution tuple")
	}
	vector, ok := invocation.NewSourceVector([]invocation.TupleSources{tuple})
	if !ok {
		t.Fatal("contribution source vector")
	}
	address, ok := invocation.New(scopeToken, []invocation.SourceVector{vector})
	if !ok {
		t.Fatal("contribution address")
	}
	return address
}

func contributionSide(t *testing.T, fixture lawFixture, value int, lineage int) binding.ContributionSide {
	t.Helper()
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("contribution presence")
	}
	side, ok := binding.NewContributionSide(fixture.values[value], presence, fixture.lineages[lineage])
	if !ok {
		t.Fatal("contribution side")
	}
	return side
}

func contributionTransition(t *testing.T, fixture lawFixture, source model.RowID, before, after binding.ContributionSide) invocation.ContributionTransition {
	t.Helper()
	transition, ok := invocation.NewContributionTransition(fixture.contribution, contributionAddress(t, fixture, source), fixture.tokens[1], fixture.base.Fence(), before, after)
	if !ok {
		t.Fatal("contribution transition")
	}
	return transition
}

// lawSubmissionBatch is deliberately private: these state laws predate the
// application-owned publication door and exercise Prepare with hand-issued
// proposal leases. Keeping the test in package transaction lets the fixture
// seal the same unexported transport shape without reintroducing a public
// spoofable constructor. Production callers must use NewSubmissionBatch with
// a real apply.Application.
func lawSubmissionBatch(t *testing.T, proposals binding.ProposalBatch, lineageValue model.LineageRef, widening witness.WideningPermit, contributions ...invocation.ContributionTransition) SubmissionBatch {
	t.Helper()
	var addressValue invocation.InvocationAddress
	if len(contributions) != 0 {
		addressValue = contributions[0].Address()
	} else if proposals.Available() && proposals.Len() != 0 {
		proposal, ok := proposals.At(0)
		if !ok {
			t.Fatal("proposal address")
		}
		tuple, tupleOK := invocation.NewTupleSources([]model.RowID{proposal.Destination().Row()})
		if !tupleOK {
			t.Fatal("proposal tuple")
		}
		vector, vectorOK := invocation.NewSourceVector([]invocation.TupleSources{tuple})
		if !vectorOK {
			t.Fatal("proposal source vector")
		}
		addressValue, ok = invocation.New(proposal.Destination().Scope(), []invocation.SourceVector{vector})
		if !ok {
			t.Fatal("proposal invocation address")
		}
	}
	return SubmissionBatch{
		proposals:     proposals,
		lineage:       lineageValue,
		widening:      widening,
		contributions: append([]invocation.ContributionTransition(nil), contributions...),
		operation:     proposals.Operation(),
		address:       addressValue,
		sealed:        true,
	}
}

func readStoreParts(t *testing.T, version database.Version, fixture lawFixture, scope witness.Scope, columnIndex int) []store.ReadPart {
	t.Helper()
	witnessValue, ok := fixture.mounted.Denominator(fixture.denominator)
	if !ok {
		t.Fatal("witness")
	}
	token := fixture.tokens[columnIndex]
	if scope != fixture.scope {
		token, ok = fixture.mounted.IssueCell(witnessValue, scope, fixture.columns[columnIndex], fixture.row)
		if !ok {
			t.Fatal("issue read token")
		}
	}
	coordinate, ok := fixture.geometry.Resolve(token)
	if !ok {
		t.Fatal("coordinate")
	}
	scratch := store.NewReadScratch(fixture.manager)
	if scratch == nil {
		t.Fatal("read scratch")
	}
	parts := make([]store.ReadPart, 0, 4)
	completed, valid := version.Store().Read(fixture.columns[columnIndex], coordinate.Dense(), coordinate.Mask(), scratch, func(part store.ReadPart) bool {
		parts = append(parts, part)
		return true
	})
	if !completed || !valid {
		t.Fatal("store read")
	}
	return parts
}

func TestPrepareRemovalPublishesSparseBeforeOnlyDelta(t *testing.T) {
	fixture := newLawFixture(t)
	seedBatch, seedLineage := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	seed, seedDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, seedBatch, seedLineage, witness.WideningPermit{}))
	if !ok || !seedDelta.Available() || !seed.SuccessorOf(fixture.base) {
		t.Fatal("seed publication")
	}
	removalBatch := fixture.removalBatch(t, fixture.scope, []int{1})
	next, delta, ok := publish(seed, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, removalBatch, seedLineage, witness.WideningPermit{}))
	if !ok || !delta.Available() || !next.SuccessorOf(seed) || !delta.Base().Same(seed) || !delta.Next().Same(next) {
		t.Fatal("removal publication")
	}
	change, ok := delta.Change(fixture.columns[1])
	if !ok || !change.Available() || change.Len() != 1 {
		t.Fatalf("removal column change missing: ok=%v len=%d", ok, change.Len())
	}
	entry, ok := change.At(0)
	if !ok {
		t.Fatal("removal change entry")
	}
	beforeValue, beforePresence, beforeOK := entry.Before()
	afterValue, afterPresence, afterOK := entry.After()
	beforeLineage, beforeLineageOK := entry.BeforeLineage()
	afterLineage, afterLineageOK := entry.AfterLineage()
	if !beforeOK || !beforePresence.Is(model.Present) || !beforeValue.Available() || afterOK || afterValue.Available() || afterPresence.Available() || !beforeLineageOK || !beforeLineage.Available() || afterLineageOK || afterLineage.Available() {
		t.Fatal("removal did not retain exact before-only semantic and lineage sides")
	}
	if !entry.SemanticChanged() || !entry.LineageChanged() || !delta.SemanticChanged() || !delta.LineageChanged() {
		t.Fatal("removal change lost semantic or lineage flags")
	}
	if parts := readStoreParts(t, next, fixture, fixture.scope, 1); len(parts) != 0 {
		t.Fatalf("full removal left %d sparse successor partitions", len(parts))
	}
}

func TestPrepareRemovalPreservesDisjointSupportSurvivor(t *testing.T) {
	fixture := newLawFixture(t)
	seedBatch, seedLineage := fixture.batch(t, fixture.otherScope, []int{0}, []int{1})
	seed, seedDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, seedBatch, seedLineage, witness.WideningPermit{}))
	if !ok || !seedDelta.Available() {
		t.Fatal("full-support seed publication")
	}
	removalBatch := fixture.removalBatch(t, fixture.scope, []int{1})
	next, delta, ok := publish(seed, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, removalBatch, seedLineage, witness.WideningPermit{}))
	if !ok || !delta.Available() || !next.SuccessorOf(seed) {
		t.Fatal("partial removal publication")
	}
	witnessValue, ok := fixture.mounted.Denominator(fixture.denominator)
	if !ok {
		t.Fatal("witness")
	}
	fullToken, ok := fixture.mounted.IssueCell(witnessValue, fixture.otherScope, fixture.columns[1], fixture.row)
	if !ok {
		t.Fatal("full-support token")
	}
	fullCoordinate, ok := fixture.geometry.Resolve(fullToken)
	if !ok {
		t.Fatal("full-support coordinate")
	}
	partialCoordinate, ok := fixture.geometry.Resolve(fixture.tokens[1])
	if !ok {
		t.Fatal("partial coordinate")
	}
	split, ok := support.Three(fullCoordinate.Mask(), partialCoordinate.Mask())
	if !ok || support.Empty(split.LeftOnly()) {
		t.Fatal("support survivor")
	}
	parts := readStoreParts(t, next, fixture, fixture.otherScope, 1)
	if len(parts) != 1 || !parts[0].Region().Equal(split.LeftOnly()) || !parts[0].Presence().Is(model.Present) || !parts[0].Value().Same(fixture.values[0]) {
		t.Fatal("partial removal discarded or widened the disjoint survivor")
	}
}

func TestPrepareExplicitProvenAbsentRemainsPresent(t *testing.T) {
	fixture := newLawFixtureWithOutputPresence(t, signature.ProduceOptional)
	batch, provenance := fixture.batchWithPresence(t, fixture.scope, []int{0}, []int{1}, model.ProvenAbsent)
	next, delta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, batch, provenance, witness.WideningPermit{}))
	if !ok || !delta.Available() || !next.SuccessorOf(fixture.base) {
		t.Fatal("explicit absence publication")
	}
	parts := readStoreParts(t, next, fixture, fixture.scope, 1)
	if len(parts) != 1 || !parts[0].Presence().Is(model.ProvenAbsent) || parts[0].Value().Available() {
		t.Fatal("explicit ProvenAbsent was collapsed into sparse undefined")
	}
	change, ok := delta.Change(fixture.columns[1])
	if !ok || change.Len() != 1 {
		t.Fatal("explicit absence change missing")
	}
	entry, ok := change.At(0)
	if !ok {
		t.Fatal("explicit absence entry missing")
	}
	if _, _, beforeOK := entry.Before(); beforeOK {
		t.Fatal("explicit absence fabricated a predecessor cell")
	}
	_, afterPresence, afterOK := entry.After()
	if !afterOK || !afterPresence.Is(model.ProvenAbsent) || !entry.SemanticChanged() || !entry.LineageChanged() {
		t.Fatal("explicit absence successor lost presence or lineage semantics")
	}
}

func TestRemovalBufferRejectsForeignDenominatorAndScope(t *testing.T) {
	fixture := newLawFixture(t)
	witnessValue, ok := fixture.mounted.Denominator(fixture.denominator)
	if !ok {
		t.Fatal("witness")
	}
	scopeToken, ok := fixture.mounted.ScopeToken(fixture.scope)
	if !ok {
		t.Fatal("scope token")
	}
	newBuffer := func() binding.ProposalBuffer {
		buffer, bufferOK := binding.NewProposalBuffer(fixture.signature, fixture.mounted.RuntimeFence(), []binding.DenominatorWitness{witnessValue}, scopeToken, binding.NewOwnerNamedDestination(fixture.relation))
		if !bufferOK {
			t.Fatal("buffer")
		}
		return buffer
	}
	foreignKey := lawID(t, func(value identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(fixture.relation, value) }, "foreign-removal-key")
	foreignRef, ok := model.NewDenominatorRef(fixture.relation, foreignKey)
	if !ok {
		t.Fatal("foreign denominator")
	}
	membership, ok := binding.NewMembershipView(fixture.relation, []model.RowID{fixture.row})
	if !ok {
		t.Fatal("foreign membership")
	}
	issuer, ok := binding.NewIssuer(fixture.mounted.RuntimeFence())
	if !ok {
		t.Fatal("foreign issuer")
	}
	foreignWitness, ok := issuer.IssueDenominator(foreignRef, membership, lawContent("foreign-removal-witness"))
	if !ok {
		t.Fatal("foreign witness")
	}
	foreignToken, ok := issuer.IssueCell(foreignWitness, scopeToken, fixture.columns[1], fixture.row)
	if !ok {
		t.Fatal("foreign denominator token")
	}
	foreignDenominatorProposal, ok := binding.NewRemovalProposal(foreignToken)
	if !ok {
		t.Fatal("foreign denominator proposal")
	}
	denominatorBuffer := newBuffer()
	if denominatorBuffer.Append(foreignDenominatorProposal) {
		t.Fatal("foreign denominator removal crossed destination buffer")
	}
	foreignScopeToken, ok := fixture.mounted.IssueCell(witnessValue, fixture.otherScope, fixture.columns[1], fixture.row)
	if !ok {
		t.Fatal("foreign scope token")
	}
	foreignScopeProposal, ok := binding.NewRemovalProposal(foreignScopeToken)
	if !ok {
		t.Fatal("foreign scope proposal")
	}
	scopeBuffer := newBuffer()
	if scopeBuffer.Append(foreignScopeProposal) {
		t.Fatal("foreign scope removal crossed destination buffer")
	}
}

// publish is test orchestration only: production has no compatibility
// path. Laws explicitly exercise Prepare's no-publication contract followed
// by the aggregate root's sole publication operation.
func publish(
	base database.Version,
	view geometry.Geometry,
	readScratch *store.ReadScratch,
	batch SubmissionBatch,
) (database.Version, database.Delta, bool) {
	prepared, ok := Prepare(base, view, readScratch, batch)
	if !ok {
		return database.Version{}, database.Delta{}, false
	}
	return database.Commit(prepared)
}

// A transition is not a second publication channel.  This fixture's mounted
// plan deliberately has no contribution declaration, so even a structurally
// valid transition paired with the exact live proposal must refuse before
// ordinary aggregate admission.  A contribution-enabled fixture exercises
// the positive path in the contribution reduction vertical.
func TestPrepareContributionRequiresMountedDeclaration(t *testing.T) {
	fixture := newLawFixture(t)
	capability, ok := model.NewAscendingCapability(fixture.typeID)
	if !ok {
		t.Fatal("capability")
	}
	port := output.OutputPort{Operation: fixture.signature.Identity(), Column: fixture.columns[0]}
	spec, ok := output.Seal(output.Spec{Signature: fixture.signature, Port: port, ValueType: fixture.typeID, Algebra: capability, Reducer: output.Contributions})
	if !ok {
		t.Fatal("contribution spec")
	}
	scopeToken, ok := fixture.mounted.ScopeToken(fixture.scope)
	if !ok {
		t.Fatal("scope token")
	}
	tuple, ok := invocation.NewTupleSources([]model.RowID{fixture.row})
	if !ok {
		t.Fatal("tuple sources")
	}
	vector, ok := invocation.NewSourceVector([]invocation.TupleSources{tuple})
	if !ok {
		t.Fatal("source vector")
	}
	address, ok := invocation.New(scopeToken, []invocation.SourceVector{vector})
	if !ok {
		t.Fatal("invocation address")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	after, ok := binding.NewContributionSide(fixture.values[0], presence, fixture.lineages[0])
	if !ok {
		t.Fatal("after side")
	}
	transition, ok := invocation.NewContributionTransition(spec, address, fixture.tokens[0], fixture.base.Fence(), binding.NoContributionSide(), after)
	if !ok {
		t.Fatal("transition")
	}
	batch, provenance := fixture.batch(t, fixture.scope, []int{0}, []int{0})
	prepared, accepted := Prepare(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, batch, provenance, witness.WideningPermit{}, transition))
	if accepted || prepared.Available() {
		t.Fatal("undeclared contribution crossed the transaction boundary")
	}
}

func TestPrepareContributionPublishesProducerAndAggregateAtomically(t *testing.T) {
	fixture := newContributionLawFixture(t)
	batch, lineage := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	after := contributionSide(t, fixture, 0, 0)
	transition := contributionTransition(t, fixture, fixture.row, binding.NoContributionSide(), after)
	next, delta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, batch, lineage, witness.WideningPermit{}, transition))
	if !ok || !delta.Available() || !next.SuccessorOf(fixture.base) {
		t.Fatal("contribution publication")
	}
	if len(delta.AffectedContributionTargets()) != 1 || len(next.ContributionState().Rows()) != 1 {
		t.Fatal("producer contribution was not atomically published")
	}
	parts := readStoreParts(t, next, fixture, fixture.scope, 1)
	if len(parts) != 1 || !parts[0].Presence().Is(model.Present) || !parts[0].Value().Same(fixture.values[0]) {
		t.Fatal("derived aggregate was not published with producer state")
	}
}

func TestPrepareContributionSelectiveRemovalPreservesSiblingAggregate(t *testing.T) {
	fixture := newContributionLawFixture(t)
	producerB := lawID(t, func(value identity.ContentID) (model.RowID, bool) { return model.IssueRowID(fixture.relation, value) }, "producer-b")
	firstBatch, firstLineage := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	first := contributionTransition(t, fixture, fixture.row, binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	firstVersion, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, firstBatch, firstLineage, witness.WideningPermit{}, first))
	if !ok {
		t.Fatal("first contribution")
	}
	secondBatch, secondLineage := fixture.batch(t, fixture.scope, []int{1}, []int{1})
	second := contributionTransition(t, fixture, producerB, binding.NoContributionSide(), contributionSide(t, fixture, 1, 1))
	secondVersion, _, ok := publish(firstVersion, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, secondBatch, secondLineage, witness.WideningPermit{}, second))
	if !ok {
		t.Fatal("second contribution")
	}
	removalBatch := fixture.removalBatch(t, fixture.scope, []int{1})
	removal := contributionTransition(t, fixture, fixture.row, contributionSide(t, fixture, 0, 0), binding.NoContributionSide())
	next, delta, ok := publish(secondVersion, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, removalBatch, fixture.lineages[0], witness.WideningPermit{}, removal))
	if !ok || !delta.Available() || len(next.ContributionState().Rows()) != 1 {
		t.Fatal("selective removal")
	}
	parts := readStoreParts(t, next, fixture, fixture.scope, 1)
	if len(parts) != 1 || !parts[0].Presence().Is(model.Present) || !parts[0].Value().Same(fixture.values[1]) {
		t.Fatal("selective removal discarded sibling aggregate")
	}
}

func TestPrepareContributionReplacementReplacesExactAggregate(t *testing.T) {
	fixture := newContributionLawFixture(t)
	seedBatch, seedLineage := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	seed := contributionTransition(t, fixture, fixture.row, binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	seedVersion, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, seedBatch, seedLineage, witness.WideningPermit{}, seed))
	if !ok {
		t.Fatal("seed contribution")
	}
	replacementBatch, replacementLineage := fixture.batch(t, fixture.scope, []int{1}, []int{1})
	replacement := contributionTransition(t, fixture, fixture.row, contributionSide(t, fixture, 0, 0), contributionSide(t, fixture, 1, 1))
	next, delta, ok := publish(seedVersion, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, replacementBatch, replacementLineage, witness.WideningPermit{}, replacement))
	if !ok || !delta.Available() || len(next.ContributionState().Rows()) != 1 {
		t.Fatal("replacement contribution")
	}
	parts := readStoreParts(t, next, fixture, fixture.scope, 1)
	if len(parts) != 1 || !parts[0].Presence().Is(model.Present) || !parts[0].Value().Same(fixture.values[1]) {
		t.Fatal("replacement did not replace aggregate")
	}
}

func TestPrepareContributionLowerReplacementPublishesContributionOnly(t *testing.T) {
	fixture := newContributionLawFixture(t)
	producerB := lawID(t, func(value identity.ContentID) (model.RowID, bool) { return model.IssueRowID(fixture.relation, value) }, "producer-b-lower-replacement")

	// The fixture's ascending algebra orders these values by their first byte:
	// value[1] is the surviving high producer, value[0] is lower, and value[2]
	// is lower still.  Keep all producer lineages equal so this test isolates
	// the semantic aggregate no-op from lineage-only churn.
	highBatch, highLineage := fixture.batch(t, fixture.scope, []int{1}, []int{1})
	high := contributionTransition(t, fixture, producerB, binding.NoContributionSide(), contributionSide(t, fixture, 1, 0))
	highVersion, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, highBatch, highLineage, witness.WideningPermit{}, high))
	if !ok {
		t.Fatal("high producer")
	}

	lowBatch, lowLineage := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	low := contributionTransition(t, fixture, fixture.row, binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	lowVersion, _, ok := publish(highVersion, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, lowBatch, lowLineage, witness.WideningPermit{}, low))
	if !ok {
		t.Fatal("lower producer")
	}
	targets := lowVersion.ContributionState().Targets()
	if len(targets) != 1 {
		t.Fatalf("contribution targets=%d, want one", len(targets))
	}
	priorRows := lowVersion.ContributionState().RowsFor(targets[0])
	if len(priorRows) != 2 {
		t.Fatalf("producer rows before replacement=%d, want two", len(priorRows))
	}
	priorStore := lowVersion.Store()
	priorIndexes := lowVersion.Indexes()
	priorContribution := lowVersion.ContributionState()

	// Replace only the lower producer.  The high sibling keeps the reduced
	// value exactly unchanged, so no column or arrangement successor is legal.
	replacementBatch, replacementLineage := fixture.batch(t, fixture.scope, []int{2}, []int{1})
	replacement := contributionTransition(t, fixture, fixture.row, contributionSide(t, fixture, 0, 0), contributionSide(t, fixture, 2, 0))
	prepared, ok := Prepare(lowVersion, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, replacementBatch, replacementLineage, witness.WideningPermit{}, replacement))
	if !ok || !prepared.Available() || prepared.Empty() {
		t.Fatal("lower replacement did not retain a contribution-only candidate")
	}
	next, delta, ok := database.Commit(prepared)
	if !ok || !delta.Available() || !next.SuccessorOf(lowVersion) || !delta.Base().Same(lowVersion) || !delta.Next().Same(next) {
		t.Fatal("lower replacement did not publish one atomic database successor")
	}
	if next.ContributionState().Same(priorContribution) || !next.ContributionState().SuccessorOf(priorContribution) {
		t.Fatal("producer contribution root did not advance")
	}
	if !next.Store().Same(priorStore) {
		t.Fatal("semantically unchanged aggregate rebuilt the store root")
	}
	indexes := next.Indexes()
	if len(indexes) != len(priorIndexes) {
		t.Fatalf("index root count=%d, want %d", len(indexes), len(priorIndexes))
	}
	for position := range indexes {
		if !indexes[position].Same(priorIndexes[position]) {
			t.Fatalf("index root %d changed for contribution-only replacement", position)
		}
	}
	if len(delta.ChangedColumnIDs()) != 0 || delta.SemanticChanged() || delta.LineageChanged() || delta.Source().Available() || len(delta.Indexes()) != 0 {
		t.Fatal("contribution-only publication fabricated store or index deltas")
	}
	rows := next.ContributionState().RowsFor(targets[0])
	if len(rows) != 2 {
		t.Fatalf("producer rows after replacement=%d, want two", len(rows))
	}
	haveHigh, haveLower := false, false
	for _, row := range rows {
		haveHigh = haveHigh || row.Value.Same(fixture.values[1])
		haveLower = haveLower || row.Value.Same(fixture.values[2])
	}
	if !haveHigh || !haveLower {
		t.Fatal("producer replacement did not retain high sibling and lower successor")
	}
}

func TestPrepareContributionExactNoopSharesDatabaseRoot(t *testing.T) {
	fixture := newContributionLawFixture(t)
	seedBatch, seedLineage := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	seed := contributionTransition(t, fixture, fixture.row, binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	seedVersion, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, seedBatch, seedLineage, witness.WideningPermit{}, seed))
	if !ok {
		t.Fatal("seed contribution")
	}
	noopBatch, noopLineage := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	noop := contributionTransition(t, fixture, fixture.row, binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	prepared, ok := Prepare(seedVersion, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, noopBatch, noopLineage, witness.WideningPermit{}, noop))
	if !ok {
		t.Fatal("exact contribution noop was refused")
	}
	next, delta, ok := database.Commit(prepared)
	if !ok || !next.Same(seedVersion) || delta.Available() {
		t.Fatal("exact contribution noop advanced a database root")
	}
}

func TestPrepareMixedOrdinaryAndContributionUsesOnePublication(t *testing.T) {
	fixture := newContributionLawFixture(t)
	batch, lineage := fixture.batch(t, fixture.scope, []int{2, 0}, []int{0, 1})
	transition := contributionTransition(t, fixture, fixture.row, binding.NoContributionSide(), contributionSide(t, fixture, 0, 0))
	next, delta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, batch, lineage, witness.WideningPermit{}, transition))
	if !ok || !delta.Available() || !next.SuccessorOf(fixture.base) || len(delta.ChangedColumnIDs()) != 2 {
		t.Fatal("mixed publication was not atomic")
	}
	left := readStoreParts(t, next, fixture, fixture.scope, 0)
	right := readStoreParts(t, next, fixture, fixture.scope, 1)
	if len(left) != 1 || !left[0].Value().Same(fixture.values[2]) || len(right) != 1 || !right[0].Value().Same(fixture.values[0]) {
		t.Fatal("mixed ordinary and contribution values diverged")
	}
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
	batch, provenance := fixture.batch(t, fixture.scope, []int{0}, []int{0})
	prepared, ok := Prepare(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, batch, provenance, witness.WideningPermit{}))
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
		// The zero-width relation directory is a derived membership index.
		// It has no key/payload columns to enumerate, but any semantic update
		// in its relation changes the owner-issued row directory.
		relevant := len(layout.KeyColumns()) == 0 && len(layout.Columns()) == 0 && layout.Access().Relation() == fixture.relation
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
	first, firstProvenance := fixture.batch(t, fixture.scope, []int{0}, []int{0})
	seed, seedDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, first, firstProvenance, witness.WideningPermit{}))
	if !ok || !seedDelta.Available() {
		t.Fatal("seed")
	}
	second, secondProvenance := fixture.batch(t, fixture.otherScope, []int{2}, []int{0})
	next, delta, ok := publish(seed, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, second, secondProvenance, witness.WideningPermit{}))
	if !ok || !delta.Available() || !next.SuccessorOf(seed) {
		t.Fatal("overlap apply")
	}
	witnessValue, ok := fixture.mounted.Denominator(fixture.denominator)
	if !ok {
		t.Fatal("full denominator witness")
	}
	fullToken, ok := fixture.mounted.IssueCell(witnessValue, fixture.otherScope, fixture.columns[0], fixture.row)
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
	noOp, noDelta, ok := publish(next, fixture.geometry, readScratch, lawSubmissionBatch(t, second, secondProvenance, witness.WideningPermit{}))
	if !ok || !noOp.Same(next) || noDelta.Available() {
		t.Fatal("no-op did not share exact base")
	}
}

func TestPreparePermutationAndMultiColumnAtomicity(t *testing.T) {
	fixture := newLawFixture(t)
	forward, forwardProvenance := fixture.batch(t, fixture.scope, []int{0, 1}, []int{0, 1})
	first, firstDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, forward, forwardProvenance, witness.WideningPermit{}))
	if !ok || !firstDelta.Available() {
		t.Fatal("forward multi-column apply")
	}
	reverse, reverseProvenance := fixture.batch(t, fixture.scope, []int{1, 0}, []int{1, 0})
	second, secondDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, reverse, reverseProvenance, witness.WideningPermit{}))
	if !ok || !secondDelta.Available() || !first.SuccessorOf(fixture.base) || !second.SuccessorOf(fixture.base) {
		t.Fatal("permuted apply")
	}
	left, right := firstDelta.ChangedColumnIDs(), secondDelta.ChangedColumnIDs()
	if len(left) != 2 || len(right) != 2 || left[0] != right[0] || left[1] != right[1] {
		t.Fatal("column delta order changed under permutation")
	}
	invalid, _ := fixture.batch(t, fixture.scope, []int{0, 1}, []int{0, 1})
	// A valid proposal batch paired with unavailable application provenance
	// must refuse before any candidate reaches the state transaction.
	unchanged, noDelta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, invalid, model.LineageRef{}, witness.WideningPermit{}))
	if ok || noDelta.Available() || unchanged.Available() {
		t.Fatal("invalid second candidate changed aggregate outcome")
	}
}

func TestPrepareWidenLineageAndStaleLeaseLaws(t *testing.T) {
	fixture := newLawFixture(t)
	// Column A is the declared stable coordinate; recurrence widening applies
	// to payload column B so the law exercises ascent rather than a forbidden
	// stable-key replacement.
	seed, seedProvenance := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	permit, permitOK := fixture.mounted.Widening(fixture.dependency, fixture.relation)
	if !permitOK || !permit.Available() {
		t.Fatal("widening permit")
	}
	// A recurrence permit authorizes a later widening; it is not a command to
	// widen an empty cell. The first ascent must seed the terminal lattice
	// value through the ordinary join/initialization path.
	initial, initialDelta, initialOK := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, seed, seedProvenance, permit))
	if !initialOK || !initialDelta.Available() || !initial.SuccessorOf(fixture.base) {
		t.Fatal("initial recurrence ascent rejected a valid widening authorization")
	}
	base, _, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, seed, seedProvenance, witness.WideningPermit{}))
	if !ok {
		t.Fatal("seed")
	}
	widen, widenProvenance := fixture.batch(t, fixture.scope, []int{1}, []int{1})
	widened, widenDelta, ok := publish(base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, widen, widenProvenance, permit))
	if !ok || !widenDelta.Available() || !widened.SuccessorOf(base) {
		t.Fatal("authorized widening refused")
	}
	foreignDependency := lawID(t, func(value identity.ContentID) (model.DependencyID, bool) {
		return model.IssueDependencyID(fixture.owner, value)
	}, "foreign-dependency")
	if _, ok := fixture.mounted.Widening(foreignDependency, fixture.relation); ok {
		t.Fatal("foreign widening permit was issued")
	}
	lineageBatch, _ := fixture.batch(t, fixture.scope, []int{0}, []int{1})
	lineageNext, lineageDelta, ok := publish(base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, lineageBatch, fixture.lineages[1], witness.WideningPermit{}))
	if !ok || !lineageDelta.Available() || len(lineageDelta.SemanticColumnIDs()) != 0 || len(lineageDelta.LineageColumnIDs()) != 1 || !lineageNext.SuccessorOf(base) {
		t.Fatal("lineage-only successor missing")
	}
	for position, child := range lineageDelta.Indexes() {
		if !child.Empty() || !lineageNext.Indexes()[position].SuccessorOf(base.Indexes()[position]) {
			t.Fatalf("lineage-only update rebuilt or lost index root %d", position)
		}
	}
	stale, staleProvenance := fixture.batch(t, fixture.scope, []int{1}, []int{1})
	if !fixture.buffer.Reset() {
		t.Fatal("reset")
	}
	if _, _, ok := publish(base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, stale, staleProvenance, witness.WideningPermit{})); ok {
		t.Fatal("stale proposal batch accepted")
	}
}

func opaqueKeyedBootstrapFixture(t *testing.T) lawFixture {
	t.Helper()
	fixture := newLawFixtureWithOutputPresenceAndPublish(t, signature.ProduceOpaque, false)
	if requirements := fixture.mounted.AlgebraRequirements(); len(requirements) != 0 {
		t.Fatalf("opaque-only schema requested an algebra: %+v", requirements)
	}
	if _, ok := fixture.mounted.Algebra(fixture.typeID); ok {
		t.Fatal("opaque-only schema mounted an algebra")
	}
	if _, ok := fixture.mounted.Equality(fixture.typeID); ok {
		t.Fatal("opaque-only schema mounted an equality witness")
	}
	return fixture
}

func TestOpaqueKeyedBootstrapSealsEmptyRootWithoutEquality(t *testing.T) {
	fixture := opaqueKeyedBootstrapFixture(t)
	var keyed arrangement.Layout
	for _, candidate := range fixture.mounted.Arrangement().Layouts() {
		if len(candidate.KeyColumns()) != 0 {
			keyed = candidate
			break
		}
	}
	if !keyed.Available() {
		t.Fatal("opaque fixture lost its declared keyed layout")
	}
	root, ok := fixture.base.Index(keyed)
	if !ok || !root.Available() || root.RowCount() != 0 {
		t.Fatalf("empty opaque keyed root=(%v,%v), rows=%d", ok, root.Available(), root.RowCount())
	}
}

func TestOpaqueFirstKeyedMaterializationRefusesWithoutEquality(t *testing.T) {
	fixture := opaqueKeyedBootstrapFixture(t)
	batch, provenance := fixture.batchWithPresence(t, fixture.scope, []int{1}, []int{0}, model.AuthenticatedOpaque)
	next, delta, ok := publish(fixture.base, fixture.geometry, store.NewReadScratch(fixture.manager), lawSubmissionBatch(t, batch, provenance, witness.WideningPermit{}))
	if ok || next.Available() || delta.Available() {
		t.Fatal("opaque keyed materialization crossed the missing equality authority")
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
	var seeded database.Version
	if ok {
		var databasePrepared database.Prepared
		databasePrepared, ok = database.Prepare(fixture.base, seededPrepared, store.NewReadScratch(manager), fixture.base.ContributionDirectory(), fixture.base.ContributionState(), nil)
		if ok {
			seeded, _, ok = database.Commit(databasePrepared)
		}
	}
	if !ok || !seeded.Available() {
		t.Fatal("seed aggregate")
	}
	_ = seededColumn
	batch, provenance := fixture.batch(t, fixture.scope, []int{0, 1}, []int{0, 1})
	if next, delta, ok := publish(seeded, fixture.geometry, store.NewReadScratch(manager), lawSubmissionBatch(t, batch, provenance, witness.WideningPermit{})); ok || delta.Available() || next.Available() {
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
	// The public Prepare path redeems one application provenance; this law
	// keeps the mounted authority explicitly associative/commutative/idempotent.
	left, _ := fixture.lineage.Join(values[0], values[1])
	left, _ = fixture.lineage.Join(left, values[2])
	right, _ := fixture.lineage.Join(values[2], values[0])
	right, _ = fixture.lineage.Join(right, values[1])
	if left != right {
		t.Fatal("lineage authority was not permutation stable")
	}
}
