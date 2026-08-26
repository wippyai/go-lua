// Package suspension supplies the family-owned target specimen for Placement
// Suspension. Its only use of domain/relationfixture is to obtain a genuine
// owner-fenced Heap root whose constructor is private. The target declaration,
// population, seed, mount, solve, and snapshot all remain targetfixture-owned.
package suspension

import (
	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	placementquery "github.com/wippyai/go-lua/analysis/domain/placement/relation/query"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/analysis/schema/program/publication"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	suspensiondomain "github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

const fixtureDomain = "analysis/engine/relation/runtime/testdata/targetfixture/placement/suspension/v1"

// Fixture retains Suspension's typed query contract alongside the generic
// target World. No target runtime authority leaks into the family fixture.
type Fixture struct {
	world     targetfixture.World
	output    model.ColumnID
	placement *relbindgen.Column[placementdomain.Fact]
	expected  placementdomain.Fact
}

// New mounts exactly one genuine liveness candidate and one owner-derived
// SourceSummary scalar. The complete Value vector is deliberately absent from
// this fold frame: canonical SuspensionRoutes consumes it before retaining
// this scalar on the correlated route row.
func New(t targetfixture.Probe) Fixture {
	t.Helper()
	route := ownerRoute(t)
	candidate := ownerCandidate(t)
	ids := newIDs(t)
	codecs := newCodecs(t, ids)
	seed, operation, declaration, input, output := newDeclaration(t, ids)
	columns, ok := placementrelation.NewPlacementSuspensionColumns(codecs.placement, codecs.route, codecs.candidate, codecs.summary, codecs.routeTag)
	if !ok {
		t.Fatal("suspension typed binding columns")
	}
	factory, ok := placementrelation.BindPlacementSuspension(operation, placementrelation.PlacementSuspensionOperation{}, columns, ids.identity.Refusal(t, "suspension"))
	if !ok {
		t.Fatal("suspension typed binding")
	}
	expected, reduction := suspensiondomain.SuspensionFold(candidate, suspensiondomain.SourceSummaryKnown, route, 1, placementdomain.DefaultFact())
	if reduction != structure.Concrete {
		t.Fatal("suspension expected owner fold")
	}
	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    ids.identity,
		Declaration: declaration,
		Bindings:    []binding.Factory{factory},
		Populations: []targetfixture.Population{
			{Denominator: input, Rows: []model.RowID{ids.inputRow}},
			{Denominator: output, Rows: []model.RowID{ids.outputRow}},
		},
		Scopes: []targetfixture.Scope{{ID: ids.scope, Region: "suspension"}},
		Initials: []targetfixture.Initial{{
			Operation: seed,
			Scope:     ids.scope,
			Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
				address, addressOK := codecs.coordinate.Encode(issuer, ids.address)
				candidateValue, candidateOK := codecs.candidate.Encode(issuer, candidate)
				summaryValue, summaryOK := codecs.summary.Encode(issuer, suspensiondomain.SourceSummaryKnown)
				routeValue, routeOK := codecs.route.Encode(issuer, route)
				routeTag, routeTagOK := codecs.routeTag.Encode(issuer, uint64(1))
				selected, selectedOK := codecs.placement.Encode(issuer, placementdomain.DefaultFact())
				if !addressOK || !candidateOK || !summaryOK || !routeOK || !routeTagOK || !selectedOK {
					return nil, false
				}
				addressCell, addressCellOK := targetfixture.Opaque(input, ids.inputRow, ids.inputAddress, address)
				candidateCell, candidateCellOK := targetfixture.Opaque(input, ids.inputRow, ids.candidate, candidateValue)
				summaryCell, summaryCellOK := targetfixture.Opaque(input, ids.inputRow, ids.summary, summaryValue)
				routeCell, routeCellOK := targetfixture.Opaque(input, ids.inputRow, ids.route, routeValue)
				routeTagCell, routeTagCellOK := targetfixture.Opaque(input, ids.inputRow, ids.routeTag, routeTag)
				selectedCell, selectedCellOK := targetfixture.Present(input, ids.inputRow, ids.selected, selected)
				if !addressCellOK || !candidateCellOK || !summaryCellOK || !routeCellOK || !routeTagCellOK || !selectedCellOK {
					return nil, false
				}
				return []targetfixture.Cell{addressCell, candidateCell, summaryCell, routeCell, routeTagCell, selectedCell}, true
			},
		}},
		Authorities: func(issuer binding.Issuer) (targetfixture.Registry, bool) {
			lattice, latticeOK := placementrelation.NewPlacementLattice()
			placementAlgebra, algebraOK := relbindgen.NewAlgebra[placementdomain.Fact, placementrelation.PlacementLattice](codecs.placement, issuer, lattice)
			coordinateEquality, equalityOK := relbindgen.NewEquality(codecs.coordinate, contentEquality{})
			if !latticeOK || !algebraOK || !equalityOK {
				return targetfixture.Registry{}, false
			}
			return targetfixture.Registry{Algebras: []binding.ValueAlgebra{placementAlgebra}, Equalities: []binding.ValueEquality{coordinateEquality}}, true
		},
		MountByte: 0xF4,
	})
	return Fixture{world: world, output: ids.outputFact, placement: codecs.placement, expected: expected}
}

// Solve executes the production target relation runtime.
func (value Fixture) Solve() (terminal.Result, bool) { return value.world.Solve() }

// Facts redeems the sole target output through the canonical typed Placement
// query path, without a fixture-local result store.
func (value Fixture) Facts(result terminal.Result) (placementquery.Rows, bool) {
	if value.placement == nil || !value.placement.Available() {
		return placementquery.Rows{}, false
	}
	projection, ok := value.world.Snapshot(result)
	if !ok || !projection.Available() {
		return placementquery.Rows{}, false
	}
	column, ok := placementquery.NewFactColumn(value.output, value.placement)
	if !ok {
		return placementquery.Rows{}, false
	}
	return placementquery.Read(projection, column)
}

func (value Fixture) Expected() placementdomain.Fact { return value.expected }

type ids struct {
	identity targetfixture.Identity
	schema   model.SchemaID

	coordinateType model.TypeID
	candidateType  model.TypeID
	summaryType    model.TypeID
	routeType      model.TypeID
	routeTagType   model.TypeID
	placementType  model.TypeID
	scope          model.ScopeID

	input  model.RelationID
	output model.RelationID

	inputAddress  model.ColumnID
	candidate     model.ColumnID
	summary       model.ColumnID
	route         model.ColumnID
	routeTag      model.ColumnID
	selected      model.ColumnID
	outputAddress model.ColumnID
	outputFact    model.ColumnID

	inputKey  model.KeyID
	outputKey model.KeyID
	inputRow  model.RowID
	outputRow model.RowID
	address   identity.ContentID

	seed       model.OperationID
	operation  model.OperationID
	expression model.ExpressionID
	dependency model.DependencyID
}

func newIDs(t targetfixture.Probe) ids {
	t.Helper()
	identity := targetfixture.NewIdentity(t, fixtureDomain)
	value := ids{
		identity:       identity,
		schema:         identity.Schema(t, "suspension"),
		coordinateType: identity.Type(t, "coordinate"),
		candidateType:  identity.Type(t, "subject-liveness"),
		summaryType:    identity.Type(t, "source-summary"),
		routeType:      identity.Type(t, "heap-route"),
		routeTagType:   identity.Type(t, "route-tag"),
		placementType:  identity.Type(t, "placement-fact"),
		scope:          identity.Scope(t, "suspension"),
		input:          identity.Relation(t, "suspension-input"),
		output:         identity.Relation(t, "placement-output"),
		seed:           identity.Operation(t, "seed"),
		operation:      identity.Operation(t, "suspension"),
		expression:     identity.Expression(t, "suspension"),
		dependency:     identity.Dependency(t, "suspension"),
	}
	value.inputAddress = identity.Column(t, value.input, "address")
	value.candidate = identity.Column(t, value.input, "candidate")
	value.summary = identity.Column(t, value.input, "source-summary")
	value.route = identity.Column(t, value.input, "route")
	value.routeTag = identity.Column(t, value.input, "route-tag")
	value.selected = identity.Column(t, value.input, "selected")
	value.outputAddress = identity.Column(t, value.output, "address")
	value.outputFact = identity.Column(t, value.output, "fact")
	value.inputKey = identity.Key(t, value.input, "address")
	value.outputKey = identity.Key(t, value.output, "address")
	value.address = content(t, identity, "row/suspension")
	value.inputRow = identity.RowFromContent(t, value.input, value.address)
	value.outputRow = identity.RowFromContent(t, value.output, value.address)
	return value
}

type codecs struct {
	coordinate *relbindgen.Column[identity.ContentID]
	candidate  *relbindgen.Column[suspensiondomain.MountedSubjectLiveness]
	summary    *relbindgen.Column[suspensiondomain.SourceSummary]
	route      *relbindgen.Column[heapdomain.Key]
	routeTag   *relbindgen.Column[uint64]
	placement  *relbindgen.Column[placementdomain.Fact]
}

func newCodecs(t targetfixture.Probe, ids ids) codecs {
	t.Helper()
	coordinate := newCodec[identity.ContentID](t, ids.identity, ids.coordinateType, "coordinate")
	candidate := newCodec[suspensiondomain.MountedSubjectLiveness](t, ids.identity, ids.candidateType, "candidate")
	summary := newCodec[suspensiondomain.SourceSummary](t, ids.identity, ids.summaryType, "source-summary")
	route := newCodec[heapdomain.Key](t, ids.identity, ids.routeType, "route")
	routeTag := newCodec[uint64](t, ids.identity, ids.routeTagType, "route-tag")
	placement := newCodec[placementdomain.Fact](t, ids.identity, ids.placementType, "placement")
	return codecs{coordinate: coordinate, candidate: candidate, summary: summary, route: route, routeTag: routeTag, placement: placement}
}

func newCodec[T any](t targetfixture.Probe, issuer targetfixture.Identity, typeID model.TypeID, label string) *relbindgen.Column[T] {
	t.Helper()
	store, ok := relbindgen.NewStore[T](content(t, issuer, "codec/"+label), 1)
	if !ok {
		t.Fatalf("suspension %s store", label)
	}
	column, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatalf("suspension %s column", label)
	}
	return column
}

func newDeclaration(t targetfixture.Probe, ids ids) (signature.Signature, signature.Signature, relcompile.Declaration, model.DenominatorRef, model.DenominatorRef) {
	t.Helper()
	input, ok := model.NewDenominatorRef(ids.input, ids.inputKey)
	if !ok {
		t.Fatal("suspension input denominator")
	}
	output, ok := model.NewDenominatorRef(ids.output, ids.outputKey)
	if !ok {
		t.Fatal("suspension output denominator")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("suspension cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("suspension outcomes")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("suspension scalar delivery")
	}
	seed := seal(t, ids, ids.seed, nil, []signature.Output{
		{Relation: ids.input, Column: ids.inputAddress, Type: ids.coordinateType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.candidate, Type: ids.candidateType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.summary, Type: ids.summaryType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.route, Type: ids.routeType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.routeTag, Type: ids.routeTagType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.selected, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: input},
	}, cardinality, outcomes)
	operation := seal(t, ids, ids.operation, []signature.Input{
		{Relation: ids.input, Column: ids.candidate, Type: ids.candidateType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.summary, Type: ids.summaryType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.route, Type: ids.routeType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.routeTag, Type: ids.routeTagType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.selected, Type: ids.placementType, Presence: signature.RequirePresent, Delivery: scalar, Denominator: input},
	}, []signature.Output{{Relation: ids.output, Column: ids.outputFact, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: output}}, cardinality, outcomes)
	capabilities := []struct {
		typeID model.TypeID
		kind   model.TypeCapabilityKind
	}{
		{ids.coordinateType, model.Equatable},
		{ids.candidateType, model.DecodeOnly},
		{ids.summaryType, model.DecodeOnly},
		{ids.routeType, model.DecodeOnly},
		{ids.routeTagType, model.DecodeOnly},
		{ids.placementType, model.Ascending},
	}
	types := make([]model.TypeCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		value, capabilityOK := model.NewTypeCapability(capability.typeID, capability.kind)
		if !capabilityOK {
			t.Fatal("suspension type capability")
		}
		types = append(types, value)
	}
	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.input, []model.ColumnID{ids.inputAddress, ids.candidate, ids.summary, ids.route, ids.routeTag, ids.selected}, []model.KeyID{ids.inputKey}, ids.scope),
			model.DefineRelationSchema(ids.output, []model.ColumnID{ids.outputAddress, ids.outputFact}, []model.KeyID{ids.outputKey}, ids.scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(ids.inputAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.candidate, ids.candidateType),
			model.DefineColumnSchema(ids.summary, ids.summaryType),
			model.DefineColumnSchema(ids.route, ids.routeType),
			model.DefineColumnSchema(ids.routeTag, ids.routeTagType),
			model.DefineColumnSchema(ids.selected, ids.placementType),
			model.DefineColumnSchema(ids.outputAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.outputFact, ids.placementType),
		},
		TypeCapabilities: types,
		Keys: []model.KeySchema{
			model.DefineKeySchema(ids.inputKey, []model.ColumnID{ids.inputAddress}),
			model.DefineKeySchema(ids.outputKey, []model.ColumnID{ids.outputAddress}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(ids.scope, nil, region.True())},
		Signatures: []signature.Signature{seed, operation},
		Rules: []relcompile.Rule{{
			ID: ids.dependency, Expression: ids.expression, Candidate: ids.input,
			ApplySlots: []relcompile.ReadOccurrence{
				relcompile.CandidateOccurrence(), relcompile.CandidateOccurrence(), relcompile.CandidateOccurrence(), relcompile.CandidateOccurrence(), relcompile.CandidateOccurrence(),
			},
			Scope:   ids.scope,
			Apply:   operation.Identity(),
			Output:  algebra.ScalarSource(algebra.NewSlotSource(0, 5)),
			Publish: &relcompile.Publication{Relation: ids.output, Key: ids.outputKey, Columns: []model.ColumnID{ids.outputFact}},
		}},
	}
	return seed, operation, declaration, input, output
}

func seal(t targetfixture.Probe, ids ids, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, cardinality model.Cardinality, outcomes outcome.Set) signature.Signature {
	t.Helper()
	value, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: ids.identity.Owner(), Schema: ids.schema},
		Inputs:      inputs,
		Outputs:     outputs,
		Cardinality: cardinality,
		Outcomes:    outcomes,
	})
	if !ok || !value.Available() {
		t.Fatal("suspension signature")
	}
	return value
}

type contentEquality struct{}

func (contentEquality) Equal(left, right identity.ContentID) bool { return left == right }

func content(t targetfixture.Probe, issuer targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := issuer.Content(label)
	if !ok {
		t.Fatalf("suspension content %q", label)
	}
	return value
}

// ownerRoute is the narrowly permitted private-constructor bridge. It obtains
// only the owner-authenticated Heap root; targetfixture.Build remains the sole
// source of target identities, relations, rows, mount, and snapshot state.
func ownerRoute(t targetfixture.Probe) heapdomain.Key {
	t.Helper()
	world := relationfixture.New(t)
	if !world.Root.Valid() || world.Root.Kind() != heapdomain.RootAllocation {
		t.Fatal("suspension owner payload factory")
	}
	return world.Root
}

func ownerCandidate(t targetfixture.Probe) suspensiondomain.MountedSubjectLiveness {
	t.Helper()
	derive := func(label string) identity.ContentID {
		value, ok := identity.DeriveContentID("placement-suspension-target-fixture/v1", []byte(label))
		if !ok {
			t.Fatalf("suspension candidate %s", label)
		}
		return value
	}
	schemaID := derive("schema")
	catalogID, ok := programcatalog.CatalogID(schemaID)
	if !ok {
		t.Fatal("suspension candidate catalog")
	}
	call, route, subject := derive("call"), derive("route"), derive("subject")
	boundaryID, boundaryIDOK := lifecycle.SubjectYieldBoundaryIdentity(call, route)
	boundary, boundaryOK := lifecycle.NewSubjectYieldBoundary(boundaryID, call, route, identity.ContentID{}, identity.ContentID{}, 0)
	spanID, spanIDOK := lifecycle.SubjectLivenessSpanIdentity(lifecycle.SubjectLivenessCell, subject, 0, 0)
	span, spanOK := lifecycle.NewSubjectLivenessSpan(spanID, subject, lifecycle.SubjectLivenessCell, 0, 0, lifecycle.SubjectLivenessLive)
	if !boundaryIDOK || !boundaryOK || !spanIDOK || !spanOK {
		t.Fatal("suspension candidate lifecycle rows")
	}
	frozen, sealed := (publication.Publication{Lifecycle: lifecycle.Publication{
		SubjectSpans:      []lifecycle.SubjectLivenessSpan{span},
		SubjectBoundaries: []lifecycle.SubjectYieldBoundary{boundary},
	}}).Seal(catalogID, identity.StoreID(47))
	if !sealed {
		t.Fatal("suspension candidate publication")
	}
	program := programschema.Program{Frozen: frozen, ArtifactID: derive("artifact"), ProgramID: derive("program"), SchemaID: schemaID}
	state, stateOK := program.ColdState()
	if !stateOK {
		t.Fatal("suspension candidate state")
	}
	candidate, candidateOK := lifecycle.RedeemSubjectLiveness(state, 0, derive("mount"), spanID)
	if !candidateOK || !candidate.Available() {
		t.Fatal("suspension candidate")
	}
	return candidate
}
