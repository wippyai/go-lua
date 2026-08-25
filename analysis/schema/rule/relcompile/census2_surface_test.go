package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// The non-rule half of the W1 census.
//
// A rule declaration carries a Program the compiler reads. A query family, a
// diagnostic observation and the diagnostic-code register carry no Program at
// all: what they declare is a family identity, a subject list, a population and
// a fold contract, and the relational shape their answers are produced by lives
// in hand-written access code. The plans here are authored from those
// declarations exactly as the raw indexed access plans are authored from the
// read chain, so every row states what the declared surface lowers to and names
// the owner statement a residual is waiting on.
//
// Every relation below is an owner statement: the plane installs it under the
// declaration surface that owns it, with the address column its rows are joined
// by and the key they are published under. The census mints no identity.

// The declaration surfaces the non-rule planes are owned by. They are the
// sealed catalog's own kinds, so a query relation is a query-surface entry and
// an observation relation is an observation-surface entry.
const (
	census2QueryOwner       schema.Key = "query"
	census2ObservationOwner schema.Key = "observation"
	census2DiagnosticOwner  schema.Key = "diagnostic"
	census2CompositeOwner   schema.Key = "composite"
	census2ProgramOwner     schema.Key = "program"
)

func census2Entry(surface schema.SurfaceKind, key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: surface, Key: key}
}

// census2Plane is one authored plan under construction: the owner surfaces it
// installs its relations through, the decision scope its derivations are
// observed at, the read port its joined rows are observed at, and the rules it
// has authored so far.
type census2Plane struct {
	t        *testing.T
	surfaces *owners
	scope    relcompile.Name
	port     relcompile.Name
	// root is the surface that installed the plane's first relation. It is the
	// owner the plane's schema identity is issued under, so the identity is a
	// declaration surface's own and never one the census picked.
	root  relcompile.Name
	rules []relcompile.Rule
	// installed is every relation member the plane named, so a row's stated
	// requirements are checked against the plan that states them.
	installed map[schema.Key]bool
}

func newCensus2Plane(t *testing.T) *census2Plane {
	t.Helper()
	surfaces := newOwners(t)
	plane := &census2Plane{t: t, surfaces: surfaces, installed: map[schema.Key]bool{}}
	plane.scope = surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, "census2/decision"))
	plane.port = surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, "census2/port"))
	return plane
}

// relation installs one relation of the plane under the surface its owner
// declares it on.
func (plane *census2Plane) relation(surface schema.SurfaceKind, owner schema.Key, member schema.Key) relcompile.Name {
	plane.t.Helper()
	name := relcompile.NewName(census2Entry(surface, owner), member)
	plane.surfaces.relation(name, plane.scope)
	if !plane.root.Available() {
		plane.root = name
	}
	plane.installed[member] = true
	return name
}

func (plane *census2Plane) query(member schema.Key) relcompile.Name {
	plane.t.Helper()
	return plane.relation(schema.SurfaceKindQuery, census2QueryOwner, member)
}

func (plane *census2Plane) observation(member schema.Key) relcompile.Name {
	plane.t.Helper()
	return plane.relation(schema.SurfaceKindObservation, census2ObservationOwner, member)
}

func (plane *census2Plane) composite(member schema.Key) relcompile.Name {
	plane.t.Helper()
	return plane.relation(schema.SurfaceKindComposite, census2CompositeOwner, member)
}

func (plane *census2Plane) program(member schema.Key) relcompile.Name {
	plane.t.Helper()
	return plane.relation(schema.SurfaceKindComposite, census2ProgramOwner, member)
}

func (plane *census2Plane) axis(owner schema.Key, member schema.Key) relcompile.Name {
	plane.t.Helper()
	return plane.relation(schema.SurfaceKindAxis, owner, member)
}

func (plane *census2Plane) diagnostic(member schema.Key) relcompile.Name {
	plane.t.Helper()
	return plane.relation(schema.SurfaceKindDiagnostic, census2DiagnosticOwner, member)
}

func (plane *census2Plane) id(name relcompile.Name) model.RelationID {
	plane.t.Helper()
	id, err := plane.surfaces.registry.Relation(relcompile.Site{Path: "census2"}, name)
	if err != nil {
		plane.t.Fatalf("resolve relation %v: %v", name, err)
	}
	return id
}

// addressColumn is the column one row of a relation is identified by. It is the
// relation owner's own coordinate declaration, so a join names it on both sides
// rather than a role the compiler infers.
func (plane *census2Plane) addressColumn(name relcompile.Name) relcompile.Name {
	return relcompile.NewName(name.Entry, name.Member+"#address")
}

func (plane *census2Plane) publicationKey(name relcompile.Name) model.KeyID {
	plane.t.Helper()
	key, err := plane.surfaces.registry.PublicationKey(relcompile.Site{Path: "census2"}, plane.addressColumn(name))
	if err != nil {
		plane.t.Fatalf("resolve publication key of %v: %v", name, err)
	}
	return key
}

func (plane *census2Plane) denominator(name relcompile.Name) model.DenominatorRef {
	plane.t.Helper()
	reference, ok := model.NewDenominatorRef(plane.id(name), plane.publicationKey(name))
	if !ok {
		plane.t.Fatalf("construct denominator for %v", name)
	}
	return reference
}

// join pairs one relation's declared address column with another's. Parent,
// occurrence and correspondence reads use this same contract: a correspondence
// relation is an ordinary input joined by the same equijoin.
func (plane *census2Plane) join(source relcompile.Name, joined relcompile.Name) relcompile.JoinSpec {
	plane.t.Helper()
	site := relcompile.Site{Rule: "census2", Path: "join"}
	left, err := plane.surfaces.registry.Addressed(site, source, relcompile.CoordinateAddress)
	if err != nil {
		plane.t.Fatalf("resolve address of %v: %v", source, err)
	}
	right, err := plane.surfaces.registry.Addressed(site, joined, relcompile.CoordinateAddress)
	if err != nil {
		plane.t.Fatalf("resolve address of %v: %v", joined, err)
	}
	scope, scopeErr := plane.surfaces.registry.Scope(site, plane.port)
	if scopeErr != nil {
		plane.t.Fatalf("resolve port scope: %v", scopeErr)
	}
	return relcompile.JoinSpec{
		Relation:     plane.id(joined),
		LeftColumns:  []model.ColumnID{left},
		RightColumns: []model.ColumnID{right},
		Scope:        scope,
	}
}

// completedJoin closes one read over the authenticated denominator its
// declaration materializes absent coordinates through.
func (plane *census2Plane) completedJoin(source relcompile.Name, joined relcompile.Name) relcompile.JoinSpec {
	plane.t.Helper()
	spec := plane.join(source, joined)
	reference := plane.denominator(joined)
	spec.Complete = &reference
	return spec
}

func (plane *census2Plane) scopeID() model.ScopeID {
	plane.t.Helper()
	id, err := plane.surfaces.registry.Scope(relcompile.Site{Path: "census2"}, plane.scope)
	if err != nil {
		plane.t.Fatalf("resolve decision scope: %v", err)
	}
	return id
}

func (plane *census2Plane) portScopeID() model.ScopeID {
	plane.t.Helper()
	id, err := plane.surfaces.registry.Scope(relcompile.Site{Path: "census2"}, plane.port)
	if err != nil {
		plane.t.Fatalf("resolve port scope: %v", err)
	}
	return id
}

func (plane *census2Plane) dependency(key schema.Key) model.DependencyID {
	plane.t.Helper()
	name := relcompile.EntryName(schema.SurfaceKindRule, key)
	plane.surfaces.expression(name.Entry, "")
	id, err := plane.surfaces.registry.Dependency(relcompile.Site{Path: "census2"}, name)
	if err != nil {
		plane.t.Fatalf("resolve dependency %v: %v", name, err)
	}
	return id
}

func (plane *census2Plane) expression(key schema.Key) model.ExpressionID {
	plane.t.Helper()
	name := relcompile.EntryName(schema.SurfaceKindRule, key)
	plane.surfaces.expression(name.Entry, "")
	id, err := plane.surfaces.registry.Expression(relcompile.Site{Path: "census2"}, name)
	if err != nil {
		plane.t.Fatalf("resolve expression %v: %v", name, err)
	}
	return id
}

func (plane *census2Plane) signature(operation relcompile.Name) signature.Identity {
	plane.t.Helper()
	identity, err := plane.surfaces.registry.Signature(relcompile.Site{Path: "census2"}, operation)
	if err != nil {
		plane.t.Fatalf("resolve signature %v: %v", operation, err)
	}
	return identity
}

// census2Step is one authored derivation of a plane, in the terms the
// declaration states: the rows it is asked over, the reads it joins, the
// denominator it closes against, the typed operation it applies, the rows it
// carries and the relation it publishes into. A read-only step publishes
// nothing and ends in the relation its consumer reads from the snapshot.
type census2Step struct {
	Key       schema.Key
	Candidate relcompile.Name
	Joins     []relcompile.JoinSpec
	Complete  *relcompile.Name
	Apply     relcompile.Name
	Carry     relcompile.Name
	Publish   relcompile.Name
}

func (plane *census2Plane) add(step census2Step) {
	plane.t.Helper()
	authored := relcompile.Rule{
		ID:         plane.dependency(step.Key),
		Expression: plane.expression(step.Key),
		Candidate:  plane.id(step.Candidate),
		Joins:      step.Joins,
		Scope:      plane.scopeID(),
	}
	if step.Complete != nil {
		reference := plane.denominator(*step.Complete)
		authored.Complete = &reference
	}
	if step.Apply.Available() {
		authored.Apply = plane.signature(step.Apply)
	}
	if step.Publish.Available() {
		authored.Publish = &relcompile.Publication{Relation: plane.id(step.Publish), Key: plane.publicationKey(step.Publish)}
	}
	if step.Carry.Available() {
		authored.Carry = &relcompile.CarrySpec{Relation: plane.id(step.Carry), Scope: plane.portScopeID()}
	}
	plane.rules = append(plane.rules, authored)
}

func (plane *census2Plane) compile(label schema.Key) plan.ExecutionSchema {
	plane.t.Helper()
	owner, err := plane.surfaces.registry.Owner(relcompile.Site{Path: "census2"}, plane.root.Entry)
	if err != nil {
		plane.t.Fatalf("resolve plane owner: %v", err)
	}
	schemaID, ok := model.IssueSchemaID(owner, plane.surfaces.token("schema", relcompile.EntryName(schema.SurfaceKindRule, label)))
	if !ok {
		plane.t.Fatalf("issue schema identity for %s", label)
	}
	declaration := plane.surfaces.registry.Declaration(schemaID)
	declaration.Rules = plane.rules
	compiled, compileErr := relcompile.Compile(declaration)
	if compileErr != nil {
		plane.t.Fatalf("compile %s: %v", label, compileErr)
	}
	return compiled
}

// sealOperation installs one sealed semantic signature under the delivery and
// cardinality the declaration states. Delivery is what makes a query fold a
// grouped reduction rather than a per-row judgment: a complete-span input is
// the whole set of rows one grouping key selects, so the grouping a family
// folds under is a term of the sealed signature and never a shape the engine
// infers from the plan.
func (plane *census2Plane) sealOperation(name relcompile.Name, destination relcompile.Name, delivery signature.Delivery, cardinality model.Cardinality) relcompile.Name {
	plane.t.Helper()
	site := relcompile.Site{Path: "census2.operation"}
	plane.surfaces.owner(name.Entry)
	if !plane.surfaces.once("operation", name) {
		return name
	}
	if err := plane.surfaces.registry.InstallOperation(name, plane.surfaces.token("operation", name)); err != nil {
		plane.t.Fatalf("install operation %v: %v", name, err)
	}
	operation, err := plane.surfaces.registry.Operation(site, name)
	if err != nil {
		plane.t.Fatalf("resolve operation %v: %v", name, err)
	}
	owner, ownerErr := plane.surfaces.registry.Owner(site, name.Entry)
	if ownerErr != nil {
		plane.t.Fatalf("resolve operation owner %v: %v", name, ownerErr)
	}
	schemaID, schemaOK := model.IssueSchemaID(owner, plane.surfaces.token("schema", relcompile.Name{Entry: name.Entry}))
	if !schemaOK {
		plane.t.Fatalf("issue schema identity for %v", name)
	}
	destinationColumn := plane.addressColumn(destination)
	column, columnErr := plane.surfaces.registry.Column(site, destinationColumn)
	if columnErr != nil {
		plane.t.Fatalf("resolve destination %v: %v", destinationColumn, columnErr)
	}
	reference := plane.denominator(destination)
	valueType, typeErr := plane.surfaces.registry.ColumnType(site, destinationColumn)
	if typeErr != nil {
		plane.t.Fatalf("resolve destination type %v: %v", destinationColumn, typeErr)
	}
	accepted, acceptedOK := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !acceptedOK {
		plane.t.Fatal("construct outcome set")
	}
	sealed, sealedOK := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{{
			Relation: column.Relation(), Column: column, Type: valueType,
			Presence: signature.AllowMissing, Delivery: delivery, Denominator: reference,
		}},
		Outputs: []signature.Output{{
			Relation: column.Relation(), Column: column, Type: valueType,
			Presence: signature.ProducePresent,
		}},
		Authority:   signature.OutputAuthority{Denominator: reference},
		Cardinality: cardinality,
		Outcomes:    accepted,
	})
	if !sealedOK {
		plane.t.Fatalf("seal signature for %v", name)
	}
	if err := plane.surfaces.registry.InstallSignature(name, sealed); err != nil {
		plane.t.Fatalf("install signature %v: %v", name, err)
	}
	return name
}

// foldOperation is a grouped reduction: the complete span of the rows one
// grouping key selects, answered by exactly one row.
func (plane *census2Plane) foldOperation(name relcompile.Name, destination relcompile.Name, grouping relcompile.Name) relcompile.Name {
	plane.t.Helper()
	delivery, ok := signature.NewCompleteSpanDelivery(plane.publicationKey(grouping))
	if !ok {
		plane.t.Fatal("construct complete span delivery")
	}
	return plane.sealOperation(name, destination, delivery, plane.exactlyOne())
}

// exactOperation is a scalar judgment answered by exactly one row: the shape of
// an exact query family and of a diagnostic observation.
func (plane *census2Plane) exactOperation(name relcompile.Name, destination relcompile.Name) relcompile.Name {
	plane.t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		plane.t.Fatal("construct scalar delivery")
	}
	return plane.sealOperation(name, destination, delivery, plane.exactlyOne())
}

func (plane *census2Plane) exactlyOne() model.Cardinality {
	plane.t.Helper()
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		plane.t.Fatal("construct exactly-one cardinality")
	}
	return cardinality
}

// census2Rendered is one authored plan's compiled shape.
type census2Rendered struct {
	count  int
	sketch string
}

func (plane *census2Plane) rendered(label schema.Key) census2Rendered {
	plane.t.Helper()
	compiled := plane.compile(label)
	return census2Rendered{count: len(compiled.Expressions()), sketch: sketch(compiled)}
}

// census2Ref names one relation as a completion denominator.
func census2Ref(name relcompile.Name) *relcompile.Name { return &name }
