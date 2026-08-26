// Package suspensionevidence supplies the family-owned target specimen for
// Placement SuspensionEvidence. relationfixture is used only for its
// owner-authenticated Heap payload; target declaration, population, seed,
// mount, solve, and canonical snapshot all go through targetfixture.Build.
package suspensionevidence

import (
	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
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
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	suspensiondomain "github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

const fixtureDomain = "analysis/engine/relation/runtime/testdata/targetfixture/placement/suspension-evidence/v1"

// Fixture retains only the evidence codec and expected owner result beside the
// generic target World. The generic runtime never receives a concrete domain
// Evidence value.
type Fixture struct {
	world    targetfixture.World
	output   model.ColumnID
	evidence *relbindgen.Column[suspensiondomain.Evidence]
	expected suspensiondomain.Evidence
}

// New proves the independent scalar evidence fold. Its source summary is the
// owner-issued scalar consequence of the complete Value vector, so no vector
// leaks into the generic declaration or this fold's input frame.
func New(t targetfixture.Probe) Fixture {
	t.Helper()
	route := ownerRoute(t)
	candidate := ownerCandidate(t)
	ids := newIDs(t)
	codecs := newCodecs(t, ids)
	seed, operation, declaration, input, output := newDeclaration(t, ids)
	columns, ok := placementrelation.NewPlacementSuspensionEvidenceColumns(codecs.route, codecs.candidate, codecs.summary, codecs.evidence, codecs.routeTag)
	if !ok {
		t.Fatal("suspension-evidence typed binding columns")
	}
	factory, ok := placementrelation.BindPlacementSuspensionEvidence(operation, placementrelation.PlacementSuspensionEvidenceOperation{}, columns, ids.identity.Refusal(t, "suspension-evidence"))
	if !ok {
		t.Fatal("suspension-evidence typed binding")
	}
	expected, reduction := suspensiondomain.SuspensionEvidenceFold(candidate, suspensiondomain.SourceSummaryKnown, route, 1, suspensiondomain.EvidenceRefuted)
	if reduction != structure.Concrete {
		t.Fatal("suspension-evidence owner fold")
	}
	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    ids.identity,
		Declaration: declaration,
		Bindings:    []binding.Factory{factory},
		Populations: []targetfixture.Population{
			{Denominator: input, Rows: []model.RowID{ids.inputRow}},
			{Denominator: output, Rows: []model.RowID{ids.outputRow}},
		},
		Scopes: []targetfixture.Scope{{ID: ids.scope, Region: "suspension-evidence"}},
		Initials: []targetfixture.Initial{{
			Operation: seed,
			Scope:     ids.scope,
			Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
				address, addressOK := codecs.coordinate.Encode(issuer, ids.address)
				candidateValue, candidateOK := codecs.candidate.Encode(issuer, candidate)
				summary, summaryOK := codecs.summary.Encode(issuer, suspensiondomain.SourceSummaryKnown)
				routeValue, routeOK := codecs.route.Encode(issuer, route)
				routeTag, routeTagOK := codecs.routeTag.Encode(issuer, uint64(1))
				selected, selectedOK := codecs.evidence.Encode(issuer, suspensiondomain.EvidenceRefuted)
				if !addressOK || !candidateOK || !summaryOK || !routeOK || !routeTagOK || !selectedOK {
					return nil, false
				}
				addressCell, addressCellOK := targetfixture.Opaque(input, ids.inputRow, ids.inputAddress, address)
				candidateCell, candidateCellOK := targetfixture.Opaque(input, ids.inputRow, ids.candidate, candidateValue)
				summaryCell, summaryCellOK := targetfixture.Opaque(input, ids.inputRow, ids.summary, summary)
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
			lattice, latticeOK := placementrelation.NewEvidenceLattice()
			evidenceAlgebra, algebraOK := relbindgen.NewAlgebra[suspensiondomain.Evidence, placementrelation.EvidenceLattice](codecs.evidence, issuer, lattice)
			coordinateEquality, equalityOK := relbindgen.NewEquality(codecs.coordinate, contentEquality{})
			if !latticeOK || !algebraOK || !equalityOK {
				return targetfixture.Registry{}, false
			}
			return targetfixture.Registry{Algebras: []binding.ValueAlgebra{evidenceAlgebra}, Equalities: []binding.ValueEquality{coordinateEquality}}, true
		},
		MountByte: 0xF5,
	})
	return Fixture{world: world, output: ids.outputEvidence, evidence: codecs.evidence, expected: expected}
}

// Solve executes the production target relation runtime.
func (value Fixture) Solve() (terminal.Result, bool) { return value.world.Solve() }

// Evidence redeems the output through the canonical generic snapshot. There
// is intentionally no fixture-local Evidence query/store: the family codec
// decodes the one generic snapshot Cell after its presence is proved.
func (value Fixture) Evidence(result terminal.Result) (snapshot.Cell, suspensiondomain.Evidence, bool) {
	if value.evidence == nil || !value.evidence.Available() {
		return snapshot.Cell{}, suspensiondomain.EvidenceMissing, false
	}
	projection, ok := value.world.Snapshot(result)
	if !ok || !projection.Available() {
		return snapshot.Cell{}, suspensiondomain.EvidenceMissing, false
	}
	keys := projection.Keys(value.output)
	if len(keys) != 1 {
		return snapshot.Cell{}, suspensiondomain.EvidenceMissing, false
	}
	cell, status := projection.Read(value.output, keys[0])
	if status != canonical.ReadHit || !cell.Available() || !cell.Presence.Is(model.Present) {
		return snapshot.Cell{}, suspensiondomain.EvidenceMissing, false
	}
	evidence, decoded := value.evidence.Decode(cell.Value)
	return cell, evidence, decoded
}

// EvidenceKey exposes the one generic snapshot address paired with Evidence.
// It is a neutral logical probe for consumers outside this family: the
// replacement command needs the row address, while this fixture remains the
// sole authority for selecting the output column and proving its cardinality.
func (value Fixture) EvidenceKey(result terminal.Result) (snapshot.RowKey, bool) {
	if value.evidence == nil || !value.evidence.Available() {
		return snapshot.RowKey{}, false
	}
	projection, ok := value.world.Snapshot(result)
	if !ok || !projection.Available() {
		return snapshot.RowKey{}, false
	}
	keys := projection.Keys(value.output)
	if len(keys) != 1 || !keys[0].Available() {
		return snapshot.RowKey{}, false
	}
	return keys[0], true
}

func (value Fixture) Expected() suspensiondomain.Evidence { return value.expected }

type ids struct {
	identity targetfixture.Identity
	schema   model.SchemaID

	coordinateType model.TypeID
	candidateType  model.TypeID
	summaryType    model.TypeID
	routeType      model.TypeID
	routeTagType   model.TypeID
	evidenceType   model.TypeID
	scope          model.ScopeID

	input  model.RelationID
	output model.RelationID

	inputAddress   model.ColumnID
	candidate      model.ColumnID
	summary        model.ColumnID
	route          model.ColumnID
	routeTag       model.ColumnID
	selected       model.ColumnID
	outputAddress  model.ColumnID
	outputEvidence model.ColumnID

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
	issuer := targetfixture.NewIdentity(t, fixtureDomain)
	value := ids{
		identity:       issuer,
		schema:         issuer.Schema(t, "suspension-evidence"),
		coordinateType: issuer.Type(t, "coordinate"),
		candidateType:  issuer.Type(t, "subject-liveness"),
		summaryType:    issuer.Type(t, "source-summary"),
		routeType:      issuer.Type(t, "heap-route"),
		routeTagType:   issuer.Type(t, "route-tag"),
		evidenceType:   issuer.Type(t, "evidence"),
		scope:          issuer.Scope(t, "suspension-evidence"),
		input:          issuer.Relation(t, "suspension-evidence-input"),
		output:         issuer.Relation(t, "suspension-evidence-output"),
		seed:           issuer.Operation(t, "seed"),
		operation:      issuer.Operation(t, "suspension-evidence"),
		expression:     issuer.Expression(t, "suspension-evidence"),
		dependency:     issuer.Dependency(t, "suspension-evidence"),
	}
	value.inputAddress = issuer.Column(t, value.input, "address")
	value.candidate = issuer.Column(t, value.input, "candidate")
	value.summary = issuer.Column(t, value.input, "source-summary")
	value.route = issuer.Column(t, value.input, "route")
	value.routeTag = issuer.Column(t, value.input, "route-tag")
	value.selected = issuer.Column(t, value.input, "selected")
	value.outputAddress = issuer.Column(t, value.output, "address")
	value.outputEvidence = issuer.Column(t, value.output, "evidence")
	value.inputKey = issuer.Key(t, value.input, "address")
	value.outputKey = issuer.Key(t, value.output, "address")
	value.address = content(t, issuer, "row/suspension-evidence")
	value.inputRow = issuer.RowFromContent(t, value.input, value.address)
	value.outputRow = issuer.RowFromContent(t, value.output, value.address)
	return value
}

type codecs struct {
	coordinate *relbindgen.Column[identity.ContentID]
	candidate  *relbindgen.Column[suspensiondomain.MountedSubjectLiveness]
	summary    *relbindgen.Column[suspensiondomain.SourceSummary]
	route      *relbindgen.Column[heapdomain.Key]
	routeTag   *relbindgen.Column[uint64]
	evidence   *relbindgen.Column[suspensiondomain.Evidence]
}

func newCodecs(t targetfixture.Probe, ids ids) codecs {
	t.Helper()
	coordinate := newCodec[identity.ContentID](t, ids.identity, ids.coordinateType, "coordinate")
	candidate := newCodec[suspensiondomain.MountedSubjectLiveness](t, ids.identity, ids.candidateType, "candidate")
	summary := newCodec[suspensiondomain.SourceSummary](t, ids.identity, ids.summaryType, "summary")
	route := newCodec[heapdomain.Key](t, ids.identity, ids.routeType, "route")
	routeTag := newCodec[uint64](t, ids.identity, ids.routeTagType, "route-tag")
	evidence := newCodec[suspensiondomain.Evidence](t, ids.identity, ids.evidenceType, "evidence")
	return codecs{coordinate: coordinate, candidate: candidate, summary: summary, route: route, routeTag: routeTag, evidence: evidence}
}

func newCodec[T any](t targetfixture.Probe, issuer targetfixture.Identity, typeID model.TypeID, label string) *relbindgen.Column[T] {
	t.Helper()
	store, ok := relbindgen.NewStore[T](content(t, issuer, "codec/"+label), 1)
	if !ok {
		t.Fatalf("suspension-evidence %s store", label)
	}
	column, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatalf("suspension-evidence %s column", label)
	}
	return column
}

func newDeclaration(t targetfixture.Probe, ids ids) (signature.Signature, signature.Signature, relcompile.Declaration, model.DenominatorRef, model.DenominatorRef) {
	t.Helper()
	input, ok := model.NewDenominatorRef(ids.input, ids.inputKey)
	if !ok {
		t.Fatal("suspension-evidence input denominator")
	}
	output, ok := model.NewDenominatorRef(ids.output, ids.outputKey)
	if !ok {
		t.Fatal("suspension-evidence output denominator")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("suspension-evidence cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("suspension-evidence outcomes")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("suspension-evidence scalar delivery")
	}
	seed := seal(t, ids, ids.seed, nil, []signature.Output{
		{Relation: ids.input, Column: ids.inputAddress, Type: ids.coordinateType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.candidate, Type: ids.candidateType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.summary, Type: ids.summaryType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.route, Type: ids.routeType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.routeTag, Type: ids.routeTagType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.selected, Type: ids.evidenceType, Presence: signature.ProducePresent, Denominator: input},
	}, cardinality, outcomes)
	operation := seal(t, ids, ids.operation, []signature.Input{
		{Relation: ids.input, Column: ids.candidate, Type: ids.candidateType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.summary, Type: ids.summaryType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.route, Type: ids.routeType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.routeTag, Type: ids.routeTagType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.selected, Type: ids.evidenceType, Presence: signature.RequirePresent, Delivery: scalar, Denominator: input},
	}, []signature.Output{{Relation: ids.output, Column: ids.outputEvidence, Type: ids.evidenceType, Presence: signature.ProducePresent, Denominator: output}}, cardinality, outcomes)
	capabilities := []struct {
		typeID model.TypeID
		kind   model.TypeCapabilityKind
	}{
		{ids.coordinateType, model.Equatable},
		{ids.candidateType, model.DecodeOnly},
		{ids.summaryType, model.DecodeOnly},
		{ids.routeType, model.DecodeOnly},
		{ids.routeTagType, model.DecodeOnly},
		{ids.evidenceType, model.Ascending},
	}
	types := make([]model.TypeCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		value, capabilityOK := model.NewTypeCapability(capability.typeID, capability.kind)
		if !capabilityOK {
			t.Fatal("suspension-evidence type capability")
		}
		types = append(types, value)
	}
	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.input, []model.ColumnID{ids.inputAddress, ids.candidate, ids.summary, ids.route, ids.routeTag, ids.selected}, []model.KeyID{ids.inputKey}, ids.scope),
			model.DefineRelationSchema(ids.output, []model.ColumnID{ids.outputAddress, ids.outputEvidence}, []model.KeyID{ids.outputKey}, ids.scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(ids.inputAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.candidate, ids.candidateType),
			model.DefineColumnSchema(ids.summary, ids.summaryType),
			model.DefineColumnSchema(ids.route, ids.routeType),
			model.DefineColumnSchema(ids.routeTag, ids.routeTagType),
			model.DefineColumnSchema(ids.selected, ids.evidenceType),
			model.DefineColumnSchema(ids.outputAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.outputEvidence, ids.evidenceType),
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
			Publish: &relcompile.Publication{Relation: ids.output, Key: ids.outputKey, Columns: []model.ColumnID{ids.outputEvidence}},
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
		t.Fatal("suspension-evidence signature")
	}
	return value
}

type contentEquality struct{}

func (contentEquality) Equal(left, right identity.ContentID) bool { return left == right }

func content(t targetfixture.Probe, issuer targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := issuer.Content(label)
	if !ok {
		t.Fatalf("suspension-evidence content %q", label)
	}
	return value
}

func ownerRoute(t targetfixture.Probe) heapdomain.Key {
	t.Helper()
	world := relationfixture.New(t)
	if !world.Root.Valid() || world.Root.Kind() != heapdomain.RootAllocation {
		t.Fatal("suspension-evidence owner route factory")
	}
	return world.Root
}

func ownerCandidate(t targetfixture.Probe) suspensiondomain.MountedSubjectLiveness {
	t.Helper()
	derive := func(label string) identity.ContentID {
		value, ok := identity.DeriveContentID("placement-suspension-evidence-target-fixture/v1", []byte(label))
		if !ok {
			t.Fatalf("suspension-evidence candidate %s", label)
		}
		return value
	}
	schemaID := derive("schema")
	catalogID, ok := programcatalog.CatalogID(schemaID)
	if !ok {
		t.Fatal("suspension-evidence candidate catalog")
	}
	call, route, subject := derive("call"), derive("route"), derive("subject")
	boundaryID, boundaryIDOK := lifecycle.SubjectYieldBoundaryIdentity(call, route)
	boundary, boundaryOK := lifecycle.NewSubjectYieldBoundary(boundaryID, call, route, identity.ContentID{}, identity.ContentID{}, 0)
	spanID, spanIDOK := lifecycle.SubjectLivenessSpanIdentity(lifecycle.SubjectLivenessCell, subject, 0, 0)
	span, spanOK := lifecycle.NewSubjectLivenessSpan(spanID, subject, lifecycle.SubjectLivenessCell, 0, 0, lifecycle.SubjectLivenessLive)
	if !boundaryIDOK || !boundaryOK || !spanIDOK || !spanOK {
		t.Fatal("suspension-evidence candidate lifecycle rows")
	}
	frozen, sealed := (publication.Publication{Lifecycle: lifecycle.Publication{
		SubjectSpans:      []lifecycle.SubjectLivenessSpan{span},
		SubjectBoundaries: []lifecycle.SubjectYieldBoundary{boundary},
	}}).Seal(catalogID, identity.StoreID(48))
	if !sealed {
		t.Fatal("suspension-evidence candidate publication")
	}
	program := programschema.Program{Frozen: frozen, ArtifactID: derive("artifact"), ProgramID: derive("program"), SchemaID: schemaID}
	state, stateOK := program.ColdState()
	if !stateOK {
		t.Fatal("suspension-evidence candidate state")
	}
	candidate, candidateOK := lifecycle.RedeemSubjectLiveness(state, 0, derive("mount"), spanID)
	if !candidateOK || !candidate.Available() {
		t.Fatal("suspension-evidence candidate")
	}
	return candidate
}
