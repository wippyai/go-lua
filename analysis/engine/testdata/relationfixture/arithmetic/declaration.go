package arithmetic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

const declarationDomain = "analysis/engine/testdata/relationfixture/arithmetic/v1"

// IDs is the immutable logical vocabulary used by the physical and oracle
// sides of the parity fixture.  It is a projection of the declaration, not a
// second registry or a runtime address table.
type IDs struct {
	Owner  model.OwnerID
	Schema model.SchemaID
	Type   model.TypeID
	Scope  model.ScopeID

	Candidate model.RelationID
	Source    model.RelationID
	Output    model.RelationID

	CandidateAddress model.ColumnID
	SourceAddress    model.ColumnID
	SourceLeft       model.ColumnID
	SourceRight      model.ColumnID
	OutputAddress    model.ColumnID
	OutputWrite      model.ColumnID

	CandidateKey model.KeyID
	SourceKey    model.KeyID
	OutputKey    model.KeyID

	SeedCandidate signature.Identity
	SeedSource    signature.Identity
	Arithmetic    signature.Identity

	Expression model.ExpressionID
	Dependency model.DependencyID
	CandidateA model.RowID
	CandidateB model.RowID
	SourceA    model.RowID
	SourceB    model.RowID
	OutputA    model.RowID
	OutputB    model.RowID
}

// Declaration is the exact checked logical artifact consumed by mount.  It
// intentionally retains no worker, physical handle, database root, or domain
// value.  Those are supplied by the sibling fixture layers after checking.
type Declaration struct {
	Schema      plan.ExecutionSchema
	IDs         IDs
	Signatures  []signature.Signature
	Arithmetic  signature.Signature
	Observation identity.ContentID
	Candidates  model.DenominatorRef
	Sources     model.DenominatorRef
	Outputs     model.DenominatorRef
}

func derive(label string) identity.ContentID {
	value, _ := identity.DeriveContentID(declarationDomain, []byte(label))
	return value
}

func arithmeticScopeRegion(t testing.TB) region.Region {
	t.Helper()
	atom, ok := region.NewAtom(derive("scope-atom"))
	if !ok {
		t.Fatal("arithmetic scope atom")
	}
	value, ok := region.FromAtom(atom)
	if !ok {
		t.Fatal("arithmetic scope region")
	}
	return value
}

func issue[T any](t testing.TB, label string, fn func(identity.ContentID) (T, bool)) T {
	t.Helper()
	value, ok := fn(derive(label))
	if !ok {
		t.Fatalf("issue arithmetic identity %s", label)
	}
	return value
}

func arithmeticOutcomes(t testing.TB) outcome.Set {
	t.Helper()
	value, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("arithmetic outcomes")
	}
	return value
}

func scalar(t testing.TB) signature.Delivery {
	t.Helper()
	value, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("arithmetic scalar delivery")
	}
	return value
}

func cardinality(t testing.TB, kind model.CardinalityKind, bound uint32) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(kind, bound)
	if !ok {
		t.Fatalf("arithmetic cardinality %s", kind.String())
	}
	return value
}

func newSignature(t testing.TB, ids IDs, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, size model.Cardinality) signature.Signature {
	t.Helper()
	owner := ids.Owner
	value, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: ids.Schema},
		Inputs:      inputs,
		Outputs:     outputs,
		Cardinality: size,
		Outcomes:    arithmeticOutcomes(t),
	})
	if !ok {
		t.Fatalf("seal arithmetic operation %v", operation)
	}
	return value
}

func defineIDs(t testing.TB) IDs {
	t.Helper()
	owner := issue(t, "owner", model.IssueOwnerID)
	ids := IDs{Owner: owner}
	ids.Schema = issue(t, "schema", func(id identity.ContentID) (model.SchemaID, bool) { return model.IssueSchemaID(owner, id) })
	ids.Type = issue(t, "opaque-type", func(id identity.ContentID) (model.TypeID, bool) { return model.IssueTypeID(owner, id) })
	ids.Scope = issue(t, "scope", func(id identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, id) })
	ids.Candidate = issue(t, "relation-candidate", func(id identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, id) })
	ids.Source = issue(t, "relation-source", func(id identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, id) })
	ids.Output = issue(t, "relation-output", func(id identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, id) })
	ids.CandidateAddress = issue(t, "candidate/address", func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(ids.Candidate, id) })
	ids.SourceAddress = issue(t, "source/address", func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(ids.Source, id) })
	ids.SourceLeft = issue(t, "source/left", func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(ids.Source, id) })
	ids.SourceRight = issue(t, "source/right", func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(ids.Source, id) })
	ids.OutputAddress = issue(t, "output/address", func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(ids.Output, id) })
	ids.OutputWrite = issue(t, "output/write", func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(ids.Output, id) })
	ids.CandidateKey = issue(t, "key-candidate", func(id identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(ids.Candidate, id) })
	ids.SourceKey = issue(t, "key-source", func(id identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(ids.Source, id) })
	ids.OutputKey = issue(t, "key-output", func(id identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(ids.Output, id) })
	ids.SeedCandidate = signature.Identity{Operation: issue(t, "operation-seed-candidate", func(id identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, id) }), Version: 1}
	ids.SeedSource = signature.Identity{Operation: issue(t, "operation-seed-source", func(id identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, id) }), Version: 1}
	ids.Arithmetic = signature.Identity{Operation: issue(t, "operation-arithmetic", func(id identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, id) }), Version: 1}
	ids.Expression = issue(t, "expression-arithmetic", func(id identity.ContentID) (model.ExpressionID, bool) { return model.IssueExpressionID(owner, id) })
	ids.Dependency = issue(t, "dependency-arithmetic", func(id identity.ContentID) (model.DependencyID, bool) { return model.IssueDependencyID(owner, id) })
	ids.CandidateA = issue(t, "candidate/a", func(id identity.ContentID) (model.RowID, bool) { return model.IssueRowID(ids.Candidate, id) })
	ids.CandidateB = issue(t, "candidate/b", func(id identity.ContentID) (model.RowID, bool) { return model.IssueRowID(ids.Candidate, id) })
	ids.SourceA = issue(t, "source/a", func(id identity.ContentID) (model.RowID, bool) { return model.IssueRowID(ids.Source, id) })
	ids.SourceB = issue(t, "source/b", func(id identity.ContentID) (model.RowID, bool) { return model.IssueRowID(ids.Source, id) })
	ids.OutputA = issue(t, "output/a", func(id identity.ContentID) (model.RowID, bool) { return model.IssueRowID(ids.Output, id) })
	ids.OutputB = issue(t, "output/b", func(id identity.ContentID) (model.RowID, bool) { return model.IssueRowID(ids.Output, id) })
	return ids
}

func buildDeclaration(t testing.TB) Declaration {
	t.Helper()
	ids := defineIDs(t)
	candidateDenominator := mustDenominator(t, ids.Candidate, ids.CandidateKey)
	sourceDenominator := mustDenominator(t, ids.Source, ids.SourceKey)
	outputDenominator := mustDenominator(t, ids.Output, ids.OutputKey)
	outputs := arithmeticOutcomes(t)
	seedCandidate := newSignature(t, ids, ids.SeedCandidate.Operation,
		nil,
		[]signature.Output{{Relation: ids.Candidate, Column: ids.CandidateAddress, Type: ids.Type, Presence: signature.ProducePresent, Denominator: candidateDenominator}},
		cardinality(t, model.BoundedMany, 2))
	seedSource := newSignature(t, ids, ids.SeedSource.Operation,
		nil,
		[]signature.Output{{Relation: ids.Source, Column: ids.SourceAddress, Type: ids.Type, Presence: signature.ProducePresent, Denominator: sourceDenominator}, {Relation: ids.Source, Column: ids.SourceLeft, Type: ids.Type, Presence: signature.ProducePresent, Denominator: sourceDenominator}, {Relation: ids.Source, Column: ids.SourceRight, Type: ids.Type, Presence: signature.ProducePresent, Denominator: sourceDenominator}},
		cardinality(t, model.BoundedMany, 2))
	delivery := scalar(t)
	arithmetic := newSignature(t, ids, ids.Arithmetic.Operation,
		[]signature.Input{
			{Relation: ids.Candidate, Column: ids.CandidateAddress, Type: ids.Type, Presence: signature.RequirePresent, Delivery: delivery, Denominator: candidateDenominator},
			{Relation: ids.Source, Column: ids.SourceLeft, Type: ids.Type, Presence: signature.RequirePresent, Delivery: delivery, Denominator: sourceDenominator},
			{Relation: ids.Source, Column: ids.SourceRight, Type: ids.Type, Presence: signature.RequirePresent, Delivery: delivery, Denominator: sourceDenominator},
		},
		[]signature.Output{{Relation: ids.Output, Column: ids.OutputAddress, Type: ids.Type, Presence: signature.ProducePresent, Denominator: outputDenominator}, {Relation: ids.Output, Column: ids.OutputWrite, Type: ids.Type, Presence: signature.ProducePresent, Denominator: outputDenominator}},
		cardinality(t, model.Optional, 0))
	_ = outputs
	childCandidate := algebra.NewInput(ids.Candidate)
	childSource := algebra.NewInput(ids.Source)
	correspondence := algebra.NewJoinContract([]model.ColumnID{ids.CandidateAddress}, []model.ColumnID{ids.SourceAddress})
	joined := algebra.NewJoin(childCandidate, childSource, correspondence)
	// Join is the sole authority for candidate/source correspondence. Apply
	// consumes one composite tuple, so all three scalar ABI slots are selected
	// from that same matched row rather than forming a Cartesian product and
	// asking the worker to repair it.
	apply := algebra.NewApply([]algebra.Expression{joined}, algebra.NewApplyContract(arithmetic.Identity(), []algebra.SlotSource{
		algebra.NewSlotSource(0, 0),
		algebra.NewSlotSource(0, 2),
		algebra.NewSlotSource(0, 3),
	}, algebra.OwnerNamed()))
	observationCardinality := cardinality(t, model.ExactlyOne, 0)
	observation := algebra.NewObservationContract(
		ids.Dependency,
		arithmetic.Identity(),
		algebra.NewObservationSource(0, 0, 1),
		sourceDenominator,
		algebra.NewObservationOutput(ids.OutputAddress, ids.Type, outputDenominator, observationCardinality),
		algebra.NewObservationOutput(ids.OutputWrite, ids.Type, outputDenominator, observationCardinality),
	)
	expression := algebra.NewPublish(apply, algebra.NewPublishContract(ids.Output, ids.OutputKey))
	relationCandidate, ok := plan.NewRelationRef(ids.Candidate)
	if !ok {
		t.Fatal("candidate relation ref")
	}
	relationSource, ok := plan.NewRelationRef(ids.Source)
	if !ok {
		t.Fatal("source relation ref")
	}
	relationOutput, ok := plan.NewRelationRef(ids.Output)
	if !ok {
		t.Fatal("output relation ref")
	}
	dependencyRef := plan.DefineDependencyRef(ids.Dependency)
	builder := plan.NewBuilder(ids.Schema)
	capability, capabilityOK := model.NewAscendingCapability(ids.Type)
	if !capabilityOK || !builder.AddTypeCapability(capability) {
		t.Fatal("add arithmetic ascending type capability")
	}
	if !builder.AddRelation(model.DefineRelationSchema(ids.Candidate, []model.ColumnID{ids.CandidateAddress}, []model.KeyID{ids.CandidateKey}, ids.Scope)) ||
		!builder.AddRelation(model.DefineRelationSchema(ids.Source, []model.ColumnID{ids.SourceAddress, ids.SourceLeft, ids.SourceRight}, []model.KeyID{ids.SourceKey}, ids.Scope)) ||
		!builder.AddRelation(model.DefineRelationSchema(ids.Output, []model.ColumnID{ids.OutputAddress, ids.OutputWrite}, []model.KeyID{ids.OutputKey}, ids.Scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(ids.CandidateAddress, ids.Type)) ||
		!builder.AddColumn(model.DefineColumnSchema(ids.SourceAddress, ids.Type)) ||
		!builder.AddColumn(model.DefineColumnSchema(ids.SourceLeft, ids.Type)) ||
		!builder.AddColumn(model.DefineColumnSchema(ids.SourceRight, ids.Type)) ||
		!builder.AddColumn(model.DefineColumnSchema(ids.OutputAddress, ids.Type)) ||
		!builder.AddColumn(model.DefineColumnSchema(ids.OutputWrite, ids.Type)) ||
		!builder.AddKey(model.DefineKeySchema(ids.CandidateKey, []model.ColumnID{ids.CandidateAddress})) ||
		!builder.AddKey(model.DefineKeySchema(ids.SourceKey, []model.ColumnID{ids.SourceAddress})) ||
		!builder.AddKey(model.DefineKeySchema(ids.OutputKey, []model.ColumnID{ids.OutputAddress})) ||
		!builder.AddScope(model.DefineScopeSchema(ids.Scope, nil, arithmeticScopeRegion(t))) ||
		!builder.AddSignature(seedCandidate) || !builder.AddSignature(seedSource) || !builder.AddSignature(arithmetic) || !builder.AddObservation(observation) ||
		!builder.AddExpression(plan.DefineExpressionRef(ids.Expression, expression)) ||
		!builder.AddDependency(plan.DefineDependency(ids.Dependency, ids.Expression, []plan.RelationRef{relationCandidate, relationSource}, []plan.RelationRef{relationOutput}, "arithmetic")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{dependencyRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) {
		t.Fatal("build arithmetic declaration")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("seal arithmetic declaration")
	}
	return Declaration{Schema: schema, IDs: ids, Signatures: []signature.Signature{seedCandidate, seedSource, arithmetic}, Arithmetic: arithmetic, Observation: observation.Digest(), Candidates: candidateDenominator, Sources: sourceDenominator, Outputs: outputDenominator}
}

func mustDenominator(t testing.TB, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatalf("denominator %v", relation)
	}
	return value
}
