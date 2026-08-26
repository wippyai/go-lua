// Package freshbirth is the target-runtime specimen for Placement's
// Value-owned FreshBirth fold.  The package owns the logical declaration and
// its typed codecs; targetfixture owns the certificate, mount, seed handoff,
// solve, and snapshot machinery.
package freshbirth

import (
	"github.com/wippyai/go-lua/analysis/domain/placement/relation"
	placementquery "github.com/wippyai/go-lua/analysis/domain/placement/relation/query"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// IDs is the owner-issued logical vocabulary of this specimen.  Row content
// is supplied by Value's real FreshResultCall key, so the target row is not a
// physical ordinal and the output address is the same logical coordinate.
type IDs struct {
	Schema model.SchemaID
	Owner  model.OwnerID

	CoordinateType model.TypeID
	CandidateType  model.TypeID
	ValueType      model.TypeID
	PlacementType  model.TypeID

	Scope        model.ScopeID
	Candidate    model.RelationID
	Facts        model.RelationID
	Output       model.RelationID
	CandidateKey model.KeyID
	FactsKey     model.KeyID
	OutputKey    model.KeyID

	CandidateAddress model.ColumnID
	CandidatePayload model.ColumnID
	FactsAddress     model.ColumnID
	FactKey          model.ColumnID
	FactValue        model.ColumnID
	OutputAddress    model.ColumnID
	OutputPayload    model.ColumnID

	SeedCandidate          signature.Identity
	SeedFacts              signature.Identity
	seedCandidateSignature signature.Signature
	seedFactsSignature     signature.Signature
	FreshBirth             signature.Signature
	Expression             model.ExpressionID
	Dependency             model.DependencyID
	CandidateDenominator   model.DenominatorRef
	FactsDenominator       model.DenominatorRef
	OutputDenominator      model.DenominatorRef

	CandidateRow model.RowID
	FactsRow     model.RowID
	OutputRow    model.RowID
}

// Columns is the family-owned typed codec set.  Runtime and snapshot layers
// carry only the resulting binding.ValueToken values.
type Columns struct {
	Coordinate           *relbindgen.Column[identity.ContentID]
	FreshResultCandidate *relbindgen.Column[valuedomain.FreshResultCall]
	Value                *relbindgen.Column[valuedomain.Value]
	Placement            *relbindgen.Column[placementdomain.Fact]
}

// Fixture retains only family expectations beside the target World.  The
// World itself contains the sole mounted runtime root.
type Fixture struct {
	World     targetfixture.World
	IDs       IDs
	Columns   Columns
	Candidate valuedomain.FreshResultCall
	Fact      valuedomain.Value
	Expected  placementdomain.Fact
}

type realFreshResult struct {
	candidate valuedomain.FreshResultCall
	fact      valuedomain.Value
}

type coordinateEquality struct{}

func (coordinateEquality) Equal(left, right identity.ContentID) bool { return left == right }

func freshOutcomes(t targetfixture.Probe) outcome.Set {
	t.Helper()
	set, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("freshbirth outcomes")
	}
	return set
}

func freshCardinality(t targetfixture.Probe) model.Cardinality {
	t.Helper()
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("freshbirth cardinality")
	}
	return cardinality
}

func freshDelivery(t targetfixture.Probe) signature.Delivery {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("freshbirth scalar delivery")
	}
	return delivery
}

func freshDenominator(t targetfixture.Probe, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("freshbirth denominator")
	}
	return denominator
}

func opaqueCell(t targetfixture.Probe, denominator model.DenominatorRef, row model.RowID, column model.ColumnID, value binding.ValueToken) targetfixture.Cell {
	t.Helper()
	presence, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		t.Fatal("freshbirth opaque seed presence")
	}
	return targetfixture.Cell{Denominator: denominator, Row: row, Column: column, Value: value, Presence: presence}
}

func sealRealFreshResult(t targetfixture.Probe) realFreshResult {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(contract, "targetfixture_freshbirth.lua", []byte("local co = coroutine.create(function() end)\nreturn co\n"))
	if err != nil {
		t.Fatalf("seal source: %v", err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	mounted, mountedOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !mountedOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mounted})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{mounted}), []programmount.MountedArtifact{mounted}, structural)
	freshCount := 0
	if values != nil {
		freshCount = values.FreshResultCallCount()
	}
	if heapFailure != heap.SealFailureNone || valueFailure != valuedomain.SealFailureNone || freshCount == 0 {
		t.Fatalf("seal schemas heap=%s value=%s fresh=%d", heapFailure, valueFailure, freshCount)
	}
	candidate, candidateOK := values.FreshResultCallAt(0)
	key, keyOK := candidate.Key()
	fact, factOK := values.FreshResultFact(key, materialization.Recent)
	if !candidateOK || !keyOK || !factOK || !values.OwnsFreshResultCall(candidate) {
		t.Fatal("sealed fresh-result operand")
	}
	return realFreshResult{candidate: candidate, fact: fact}
}

func issueContent(t targetfixture.Probe, owner targetfixture.Identity, label string, issue func(identity.ContentID) (identity.ContentID, bool)) identity.ContentID {
	t.Helper()
	content, ok := owner.Content(label)
	if !ok {
		t.Fatalf("freshbirth content %q", label)
	}
	value, ok := issue(content)
	if !ok {
		t.Fatalf("freshbirth issued content %q", label)
	}
	return value
}

func sealSignature(t targetfixture.Probe, owner model.OwnerID, schema model.SchemaID, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, authority model.DenominatorRef) signature.Signature {
	t.Helper()
	value, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: schema},
		Inputs:      inputs,
		Outputs:     outputs,
		Cardinality: freshCardinality(t),
		Outcomes:    freshOutcomes(t),
	})
	if !ok {
		t.Fatalf("freshbirth signature %v", operation)
	}
	return value
}

func buildDeclaration(t targetfixture.Probe, owner targetfixture.Identity, candidateContent identity.ContentID) (IDs, relcompile.Declaration) {
	t.Helper()
	ownerID := owner.Owner()
	schemaID := owner.Schema(t, "placement/freshbirth")
	scope := owner.Scope(t, "placement/freshbirth")
	candidateRelation := owner.Relation(t, "value/fresh-result/candidates")
	factsRelation := owner.Relation(t, "value/fresh-result/facts")
	outputRelation := owner.Relation(t, "placement/facts")
	candidateAddress := owner.Column(t, candidateRelation, "address")
	candidatePayload := owner.Column(t, candidateRelation, "fresh-result-candidate")
	factsAddress := owner.Column(t, factsRelation, "address")
	factKey := owner.Column(t, factsRelation, "fresh-result-key")
	factValue := owner.Column(t, factsRelation, "fresh-result-fact")
	outputAddress := owner.Column(t, outputRelation, "address")
	outputPayload := owner.Column(t, outputRelation, "placement-fact")
	candidateKey := owner.Key(t, candidateRelation, "candidate")
	factsKey := owner.Key(t, factsRelation, "facts")
	outputKey := owner.Key(t, outputRelation, "output")
	coordinateType := owner.Type(t, "value/coordinate")
	candidateType := owner.Type(t, "value/fresh-result-candidate")
	valueType := owner.Type(t, "value/fact")
	placementType := owner.Type(t, "placement/fact")
	seedCandidateOperation := owner.Operation(t, "seed/candidate")
	seedFactsOperation := owner.Operation(t, "seed/facts")
	freshBirthOperation := owner.Operation(t, "placement/freshbirth")
	expressionID := issueContent(t, owner, "expression/placement/freshbirth", func(content identity.ContentID) (identity.ContentID, bool) { return content, true })
	dependencyID := issueContent(t, owner, "dependency/placement/freshbirth", func(content identity.ContentID) (identity.ContentID, bool) { return content, true })
	// ExpressionID and DependencyID are nominal wrappers over the same
	// owner-issued content only at this handoff; the constructors retain the
	// separate model kinds and ownership fences.
	expression, ok := model.IssueExpressionID(ownerID, expressionID)
	if !ok {
		t.Fatal("freshbirth expression")
	}
	dependency, ok := model.IssueDependencyID(ownerID, dependencyID)
	if !ok {
		t.Fatal("freshbirth dependency")
	}

	candidateDenominator := freshDenominator(t, candidateRelation, candidateKey)
	factsDenominator := freshDenominator(t, factsRelation, factsKey)
	outputDenominator := freshDenominator(t, outputRelation, outputKey)
	delivery := freshDelivery(t)
	seedCandidate := sealSignature(t, ownerID, schemaID, seedCandidateOperation, nil, []signature.Output{
		{Relation: candidateRelation, Column: candidateAddress, Type: coordinateType, Presence: signature.ProduceOpaque, Denominator: candidateDenominator},
		{Relation: candidateRelation, Column: candidatePayload, Type: candidateType, Presence: signature.ProduceOpaque, Denominator: candidateDenominator},
	}, candidateDenominator)
	seedFacts := sealSignature(t, ownerID, schemaID, seedFactsOperation, nil, []signature.Output{
		{Relation: factsRelation, Column: factsAddress, Type: coordinateType, Presence: signature.ProduceOpaque, Denominator: factsDenominator},
		{Relation: factsRelation, Column: factKey, Type: coordinateType, Presence: signature.ProduceOpaque, Denominator: factsDenominator},
		{Relation: factsRelation, Column: factValue, Type: valueType, Presence: signature.ProduceOpaque, Denominator: factsDenominator},
	}, factsDenominator)
	freshBirth := sealSignature(t, ownerID, schemaID, freshBirthOperation, []signature.Input{
		{Relation: candidateRelation, Column: candidatePayload, Type: candidateType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: candidateDenominator},
		{Relation: factsRelation, Column: factValue, Type: valueType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: factsDenominator},
	}, []signature.Output{{Relation: outputRelation, Column: outputPayload, Type: placementType, Presence: signature.ProducePresent, Denominator: outputDenominator}}, outputDenominator)

	capabilities := make([]model.TypeCapability, 0, 4)
	for _, item := range []struct {
		typeID model.TypeID
		kind   model.TypeCapabilityKind
	}{
		{coordinateType, model.Equatable},
		{candidateType, model.DecodeOnly},
		{valueType, model.DecodeOnly},
		{placementType, model.Ascending},
	} {
		capability, capabilityOK := model.NewTypeCapability(item.typeID, item.kind)
		if !capabilityOK {
			t.Fatal("freshbirth type capability")
		}
		capabilities = append(capabilities, capability)
	}

	declaration := relcompile.Declaration{
		SchemaID: schemaID,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(candidateRelation, []model.ColumnID{candidateAddress, candidatePayload}, []model.KeyID{candidateKey}, scope),
			model.DefineRelationSchema(factsRelation, []model.ColumnID{factsAddress, factKey, factValue}, []model.KeyID{factsKey}, scope),
			model.DefineRelationSchema(outputRelation, []model.ColumnID{outputAddress, outputPayload}, []model.KeyID{outputKey}, scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(candidateAddress, coordinateType),
			model.DefineColumnSchema(candidatePayload, candidateType),
			model.DefineColumnSchema(factsAddress, coordinateType),
			model.DefineColumnSchema(factKey, coordinateType),
			model.DefineColumnSchema(factValue, valueType),
			model.DefineColumnSchema(outputAddress, coordinateType),
			model.DefineColumnSchema(outputPayload, placementType),
		},
		TypeCapabilities: capabilities,
		Keys: []model.KeySchema{
			model.DefineKeySchema(candidateKey, []model.ColumnID{candidateAddress}),
			model.DefineKeySchema(factsKey, []model.ColumnID{factsAddress}),
			model.DefineKeySchema(outputKey, []model.ColumnID{outputAddress}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(scope, nil, region.True())},
		Signatures: []signature.Signature{seedCandidate, seedFacts, freshBirth},
		Rules: []relcompile.Rule{{
			ID:         dependency,
			Expression: expression,
			Candidate:  candidateRelation,
			Joins: []relcompile.JoinSpec{{
				Relation:     factsRelation,
				LeftColumns:  []model.ColumnID{candidateAddress},
				RightColumns: []model.ColumnID{factKey},
				Scope:        scope,
			}},
			ApplySlots: []relcompile.ReadOccurrence{relcompile.CandidateOccurrence(), relcompile.JoinOccurrence(0)},
			Scope:      scope,
			Apply:      freshBirth.Identity(),
			Output:     algebra.ScalarSource(algebra.NewSlotSource(0, 1)),
			Carry:      &relcompile.CarrySpec{Relation: outputRelation, Scope: scope, Columns: []model.ColumnID{outputPayload}},
			Publish:    &relcompile.Publication{Relation: outputRelation, Key: outputKey, Columns: []model.ColumnID{outputPayload}},
		}},
	}
	assertOutputGeometry(t, freshBirth, declaration.Rules[0], outputRelation, outputPayload, outputDenominator)

	ids := IDs{
		Schema: schemaID, Owner: ownerID,
		CoordinateType: coordinateType, CandidateType: candidateType, ValueType: valueType, PlacementType: placementType,
		Scope: scope, Candidate: candidateRelation, Facts: factsRelation, Output: outputRelation,
		CandidateKey: candidateKey, FactsKey: factsKey, OutputKey: outputKey,
		CandidateAddress: candidateAddress, CandidatePayload: candidatePayload,
		FactsAddress: factsAddress, FactKey: factKey, FactValue: factValue,
		OutputAddress: outputAddress, OutputPayload: outputPayload,
		SeedCandidate: seedCandidate.Identity(), SeedFacts: seedFacts.Identity(), FreshBirth: freshBirth,
		Expression: expression, Dependency: dependency,
		seedCandidateSignature: seedCandidate, seedFactsSignature: seedFacts,
		CandidateDenominator: candidateDenominator, FactsDenominator: factsDenominator, OutputDenominator: outputDenominator,
	}
	ids.CandidateRow = owner.RowFromContent(t, candidateRelation, candidateContent)
	ids.FactsRow = owner.RowFromContent(t, factsRelation, candidateContent)
	ids.OutputRow = owner.RowFromContent(t, outputRelation, candidateContent)
	return ids, declaration
}

// assertOutputGeometry is FreshBirth's owner law: the candidate row is the
// exact destination identity, and the sealed one-row placement output uses
// the candidate payload's scalar source. No physical row or cardinality
// convention may replace this declaration.
func assertOutputGeometry(t targetfixture.Probe, operation signature.Signature, rule relcompile.Rule, outputRelation model.RelationID, outputColumn model.ColumnID, outputDenominator model.DenominatorRef) {
	t.Helper()
	outputs := operation.Outputs()
	if len(outputs) != 1 || outputs[0].Relation != outputRelation || outputs[0].Column != outputColumn || outputs[0].Denominator != outputDenominator {
		t.Fatal("freshbirth output relation geometry")
	}
	if !operation.Cardinality().Available() || operation.Cardinality().Kind() != model.ExactlyOne {
		t.Fatal("freshbirth output cardinality geometry")
	}
	source, ok := rule.Output.Source()
	if !rule.Output.IsScalarSource() || !ok || source != algebra.NewSlotSource(0, 1) {
		t.Fatalf("freshbirth output source=%v/%t, want scalar child 0 cell 1", source, ok)
	}
	if input, inputOK := operation.InputAt(0); !inputOK || !input.Delivery.IsScalar() {
		t.Fatal("freshbirth output source input is not scalar")
	}
}

func newColumns(t targetfixture.Probe, owner targetfixture.Identity, ids IDs) Columns {
	t.Helper()
	newTag := func(label string) identity.ContentID {
		value, ok := owner.Content("codec/" + label)
		if !ok {
			t.Fatalf("freshbirth codec tag %q", label)
		}
		return value
	}
	coordinateStore, ok := relbindgen.NewStore[identity.ContentID](newTag("coordinate"), 2)
	if !ok {
		t.Fatal("freshbirth coordinate store")
	}
	coordinate, ok := relbindgen.NewColumn(ids.CoordinateType, coordinateStore)
	if !ok {
		t.Fatal("freshbirth coordinate column")
	}
	candidateStore, ok := relbindgen.NewStore[valuedomain.FreshResultCall](newTag("candidate"), 1)
	if !ok {
		t.Fatal("freshbirth candidate store")
	}
	candidate, ok := relbindgen.NewColumn(ids.CandidateType, candidateStore)
	if !ok {
		t.Fatal("freshbirth candidate column")
	}
	valueStore, ok := relbindgen.NewStore[valuedomain.Value](newTag("value"), 1)
	if !ok {
		t.Fatal("freshbirth value store")
	}
	value, ok := relbindgen.NewColumn(ids.ValueType, valueStore)
	if !ok {
		t.Fatal("freshbirth value column")
	}
	placementStore, ok := relbindgen.NewStore[placementdomain.Fact](newTag("placement"), 1)
	if !ok {
		t.Fatal("freshbirth placement store")
	}
	placement, ok := relbindgen.NewColumn(ids.PlacementType, placementStore)
	if !ok {
		t.Fatal("freshbirth placement column")
	}
	return Columns{Coordinate: coordinate, FreshResultCandidate: candidate, Value: value, Placement: placement}
}

// New constructs one complete target specimen.  The real Value operand is
// sealed by the family helper below; no synthetic candidate or default value
// is admitted into the semantic path. An optional mount byte lets tests
// exercise the runtime fence without changing the logical declaration.
func New(t targetfixture.Probe, mountBytes ...byte) Fixture {
	t.Helper()
	if len(mountBytes) > 1 {
		t.Fatalf("freshbirth: at most one mount byte, got %d", len(mountBytes))
	}
	mountByte := byte(0xF7)
	if len(mountBytes) == 1 {
		mountByte = mountBytes[0]
	}
	owner := targetfixture.NewIdentity(t, "placement/freshbirth/v1")
	real := sealRealFreshResult(t)
	coordinate, ok := real.candidate.KeyID()
	if !ok {
		t.Fatal("freshbirth candidate coordinate")
	}
	ids, declaration := buildDeclaration(t, owner, coordinate)
	columns := newColumns(t, owner, ids)
	refusal := owner.Refusal(t, "placement/freshbirth/refused")
	factory, ok := relation.BindPlacementFreshBirth(ids.FreshBirth, relation.PlacementFreshBirthOperation{}, relation.PlacementFreshBirthColumns{
		Value:                columns.Value,
		Placement:            columns.Placement,
		FreshResultCandidate: columns.FreshResultCandidate,
	}, refusal)
	if !ok {
		t.Fatal("freshbirth generated binding")
	}
	coordinateValue := coordinate
	initials := []targetfixture.Initial{
		{
			Operation: signatureFor(ids, true),
			Scope:     ids.Scope,
			Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
				coordinateToken, coordinateOK := columns.Coordinate.Encode(issuer, coordinateValue)
				candidateToken, candidateOK := columns.FreshResultCandidate.Encode(issuer, real.candidate)
				if !coordinateOK || !candidateOK {
					return nil, false
				}
				return []targetfixture.Cell{
					opaqueCell(t, ids.CandidateDenominator, ids.CandidateRow, ids.CandidateAddress, coordinateToken),
					opaqueCell(t, ids.CandidateDenominator, ids.CandidateRow, ids.CandidatePayload, candidateToken),
				}, true
			},
		},
		{
			Operation: signatureFor(ids, false),
			Scope:     ids.Scope,
			Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
				coordinateToken, coordinateOK := columns.Coordinate.Encode(issuer, coordinateValue)
				valueToken, valueOK := columns.Value.Encode(issuer, real.fact)
				if !coordinateOK || !valueOK {
					return nil, false
				}
				return []targetfixture.Cell{
					opaqueCell(t, ids.FactsDenominator, ids.FactsRow, ids.FactsAddress, coordinateToken),
					opaqueCell(t, ids.FactsDenominator, ids.FactsRow, ids.FactKey, coordinateToken),
					opaqueCell(t, ids.FactsDenominator, ids.FactsRow, ids.FactValue, valueToken),
				}, true
			},
		},
	}
	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    owner,
		Declaration: declaration,
		Bindings:    []binding.Factory{factory},
		Populations: []targetfixture.Population{
			{Denominator: ids.CandidateDenominator, Rows: []model.RowID{ids.CandidateRow}},
			{Denominator: ids.FactsDenominator, Rows: []model.RowID{ids.FactsRow}},
			{Denominator: ids.OutputDenominator, Rows: []model.RowID{ids.OutputRow}},
		},
		Scopes:   []targetfixture.Scope{{ID: ids.Scope, Region: "freshbirth"}},
		Initials: initials,
		Authorities: func(issuer binding.Issuer) (targetfixture.Registry, bool) {
			lattice, latticeOK := relation.NewPlacementLattice()
			algebra, algebraOK := relbindgen.NewAlgebra(columns.Placement, issuer, lattice)
			equality, equalityOK := relbindgen.NewEquality(columns.Coordinate, coordinateEquality{})
			if !latticeOK || !algebraOK || !equalityOK {
				return targetfixture.Registry{}, false
			}
			return targetfixture.Registry{
				Algebras:   []binding.ValueAlgebra{algebra},
				Equalities: []binding.ValueEquality{equality},
			}, true
		},
		MountByte: mountByte,
	})
	return Fixture{World: world, IDs: ids, Columns: columns, Candidate: real.candidate, Fact: real.fact, Expected: placementdomain.DefaultFact()}
}

// Solve executes FreshBirth through the target runtime.
func (value Fixture) Solve() (terminal.Result, bool) { return value.World.Solve() }

// Facts redeems FreshBirth's output through the canonical typed Placement query.
func (value Fixture) Facts(result terminal.Result) (placementquery.Rows, bool) {
	projection, ok := value.World.Snapshot(result)
	if !ok || !projection.Available() || value.Columns.Placement == nil {
		return placementquery.Rows{}, false
	}
	column, ok := placementquery.NewFactColumn(value.IDs.OutputPayload, value.Columns.Placement)
	if !ok {
		return placementquery.Rows{}, false
	}
	return placementquery.Read(projection, column)
}

func signatureFor(ids IDs, candidate bool) signature.Signature {
	if candidate {
		return ids.seedCandidateSignature
	}
	return ids.seedFactsSignature
}
