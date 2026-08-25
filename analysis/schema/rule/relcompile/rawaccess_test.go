package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// The raw indexed access plans.
//
// domain/heap/index declares raw-get and raw-set as rule.Spec rows with no
// Program: their read chain, their receiver routes and their heap update live
// in hot Go. The chain itself is fully known - raw-get reads a receiver, then
// a key selected by that receiver, then a call, a heap fact, a pack payload
// and a source value, each selected by the cells the reads before it returned;
// raw-set reads the same chain without the call and ends in a routed heap
// write.
//
// A read whose coordinate is computed from an earlier read's CELL VALUES is
// not an equijoin over authored columns: the coordinate does not exist until
// the earlier cell is known. It is therefore a finite expansion. Each
// dependent read becomes one route relation published by one typed Apply over
// the results it depends on, and the read itself is an ordinary equijoin onto
// that route relation. The chain is a chain of dependencies, not a chain of
// engine forms, and raw-set's write is an authenticated publication into the
// heap denominator its route selected.
const (
	heapAxis  schema.Key = "heap"
	valueAxis schema.Key = "value"
	callAxis  schema.Key = "call"
	packAxis  schema.Key = "pack"

	indexReadCandidates  schema.Key = "heap/index-read-candidates"
	indexWriteCandidates schema.Key = "heap/index-write-candidates"
	valueFacts           schema.Key = "value/facts"
	callFacts            schema.Key = "call/facts"
	heapFacts            schema.Key = "heap/facts"
	packFacts            schema.Key = "pack/facts"
)

func axisRef(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func relationName(axis schema.Key, member schema.Key) relcompile.Name {
	return relcompile.NewName(axisRef(axis), member)
}

// rawAccess builds the registry and the rule set of one raw indexed access
// plan. The route relations and their expansions are owner-issued heap
// members, so the plan names no engine form and mints no identity.
type rawAccess struct {
	t         *testing.T
	surfaces  *owners
	scope     relcompile.Name
	port      relcompile.Name
	rules     []relcompile.Rule
	dependent relcompile.Name
}

func newRawAccess(t *testing.T) *rawAccess {
	t.Helper()
	surfaces := newOwners(t)
	access := &rawAccess{t: t, surfaces: surfaces}
	access.scope = surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, "census/candidate"))
	access.port = surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, "census/port/0"))
	return access
}

// factRelation installs one axis fact relation together with the coordinate
// column its rows are addressed by.
func (access *rawAccess) factRelation(axis schema.Key, member schema.Key) relcompile.Name {
	access.t.Helper()
	name := relationName(axis, member)
	access.surfaces.relation(name, access.scope)
	return name
}

// step adds one dependent read of the chain: a typed expansion over the
// results already known, publishing the route relation the read is then an
// ordinary equijoin onto.
func (access *rawAccess) step(rule schema.Key, candidate relcompile.Name, read relcompile.Name, route schema.Key, bound uint32) relcompile.Name {
	access.t.Helper()
	routeRelation := access.factRelation(heapAxis, route)
	operation := relationName(heapAxis, route+"#expansion")
	access.surfaces.expansion(operation, access.destinationColumn(routeRelation), bound, 1, candidate)
	access.rules = append(access.rules, relcompile.Rule{
		ID:         access.dependencyOf(rule),
		Expression: access.expressionOf(rule),
		Candidate:  access.relation(candidate),
		Joins:      []relcompile.JoinSpec{access.join(candidate, read)},
		Scope:      access.scopeID(),
		Apply:      access.signature(operation),
		Publish:    &relcompile.Publication{Relation: access.relation(routeRelation), Key: access.publicationKey(routeRelation)},
	})
	return routeRelation
}

func (access *rawAccess) destinationColumn(relation relcompile.Name) relcompile.Name {
	access.t.Helper()
	column := relcompile.NewName(relation.Entry, relation.Member+"#address")
	return column
}

// join pairs the source relation's declared address column with the joined
// relation's declared address column. Both sides are owner statements about
// the relations themselves.
func (access *rawAccess) join(source relcompile.Name, joined relcompile.Name) relcompile.JoinSpec {
	access.t.Helper()
	site := relcompile.Site{Rule: "raw-access", Path: "join"}
	left, err := access.surfaces.registry.Addressed(site, source, relcompile.CoordinateAddress)
	if err != nil {
		access.t.Fatalf("resolve address of %v: %v", source, err)
	}
	right, err := access.surfaces.registry.Addressed(site, joined, relcompile.CoordinateAddress)
	if err != nil {
		access.t.Fatalf("resolve address of %v: %v", joined, err)
	}
	return relcompile.JoinSpec{
		Relation:     access.relation(joined),
		LeftColumns:  []model.ColumnID{left},
		RightColumns: []model.ColumnID{right},
		Scope:        access.portScope(),
	}
}

func (access *rawAccess) completed(spec relcompile.JoinSpec, denominator relcompile.Name) relcompile.JoinSpec {
	access.t.Helper()
	site := relcompile.Site{Rule: "raw-access", Path: "join.complete"}
	key, err := access.surfaces.registry.PublicationKey(site, access.destinationColumn(denominator))
	if err != nil {
		access.t.Fatalf("resolve denominator key of %v: %v", denominator, err)
	}
	reference, ok := model.NewDenominatorRef(access.relation(denominator), key)
	if !ok {
		access.t.Fatalf("construct denominator for %v", denominator)
	}
	spec.Complete = &reference
	return spec
}

func (access *rawAccess) relation(name relcompile.Name) model.RelationID {
	access.t.Helper()
	id, err := access.surfaces.registry.Relation(relcompile.Site{Path: "raw-access"}, name)
	if err != nil {
		access.t.Fatalf("resolve relation %v: %v", name, err)
	}
	return id
}

func (access *rawAccess) publicationKey(relation relcompile.Name) model.KeyID {
	access.t.Helper()
	key, err := access.surfaces.registry.PublicationKey(relcompile.Site{Path: "raw-access"}, access.destinationColumn(relation))
	if err != nil {
		access.t.Fatalf("resolve publication key of %v: %v", relation, err)
	}
	return key
}

func (access *rawAccess) signature(operation relcompile.Name) signature.Identity {
	access.t.Helper()
	identity, err := access.surfaces.registry.Signature(relcompile.Site{Path: "raw-access"}, operation)
	if err != nil {
		access.t.Fatalf("resolve signature %v: %v", operation, err)
	}
	return identity
}

func (access *rawAccess) scopeID() model.ScopeID {
	access.t.Helper()
	id, err := access.surfaces.registry.Scope(relcompile.Site{Path: "raw-access"}, access.scope)
	if err != nil {
		access.t.Fatalf("resolve scope: %v", err)
	}
	return id
}

func (access *rawAccess) portScope() model.ScopeID {
	access.t.Helper()
	id, err := access.surfaces.registry.Scope(relcompile.Site{Path: "raw-access"}, access.port)
	if err != nil {
		access.t.Fatalf("resolve port scope: %v", err)
	}
	return id
}

func (access *rawAccess) dependencyOf(key schema.Key) model.DependencyID {
	access.t.Helper()
	name := relcompile.EntryName(schema.SurfaceKindRule, key)
	access.surfaces.expression(name.Entry, "")
	id, err := access.surfaces.registry.Dependency(relcompile.Site{Path: "raw-access"}, name)
	if err != nil {
		access.t.Fatalf("resolve dependency %v: %v", name, err)
	}
	return id
}

func (access *rawAccess) expressionOf(key schema.Key) model.ExpressionID {
	access.t.Helper()
	name := relcompile.EntryName(schema.SurfaceKindRule, key)
	access.surfaces.expression(name.Entry, "")
	id, err := access.surfaces.registry.Expression(relcompile.Site{Path: "raw-access"}, name)
	if err != nil {
		access.t.Fatalf("resolve expression %v: %v", name, err)
	}
	return id
}

// rawGetPlan is the complete raw-get plan: the five dependent route
// expansions of the r0..r5 chain and the reduction that publishes the Value
// result, carrying the receiver's own fact for the rows it does not produce.
func rawGetPlan(t *testing.T) *rawAccess {
	t.Helper()
	access := newRawAccess(t)
	candidates := access.factRelation(heapAxis, indexReadCandidates)
	values := access.factRelation(valueAxis, valueFacts)
	calls := access.factRelation(callAxis, callFacts)
	heaps := access.factRelation(heapAxis, heapFacts)
	packs := access.factRelation(packAxis, packFacts)

	keyRoutes := access.step("raw-get/key-routes", candidates, values, "heap/raw-get-key-routes", 64)
	callRoutes := access.step("raw-get/call-routes", keyRoutes, values, "heap/raw-get-call-routes", 64)
	heapRoutes := access.step("raw-get/heap-routes", callRoutes, calls, "heap/raw-get-heap-routes", 64)
	packRoutes := access.step("raw-get/pack-routes", heapRoutes, heaps, "heap/raw-get-pack-routes", 64)
	sourceRoutes := access.step("raw-get/source-routes", packRoutes, packs, "heap/raw-get-source-routes", 64)

	reduction := relationName(heapAxis, "heap/raw-get-reduction")
	access.surfaces.operation(reduction, access.destinationColumn(values), 3, candidates)
	access.rules = append(access.rules, relcompile.Rule{
		ID:         access.dependencyOf("raw-get/result"),
		Expression: access.expressionOf("raw-get/result"),
		Candidate:  access.relation(candidates),
		Joins: []relcompile.JoinSpec{
			access.join(candidates, values),
			access.join(candidates, sourceRoutes),
			access.join(sourceRoutes, packs),
		},
		Scope:   access.scopeID(),
		Apply:   access.signature(reduction),
		Carry:   &relcompile.CarrySpec{Relation: access.relation(values), Scope: access.portScope()},
		Publish: &relcompile.Publication{Relation: access.relation(values), Key: access.publicationKey(values)},
	})
	return access
}

// rawSetPlan is the complete raw-set plan: the four dependent route
// expansions of its chain and the cell update, which is an authenticated
// publication into the heap denominator row its route selected rather than a
// write door of its own.
func rawSetPlan(t *testing.T) *rawAccess {
	t.Helper()
	access := newRawAccess(t)
	candidates := access.factRelation(heapAxis, indexWriteCandidates)
	values := access.factRelation(valueAxis, valueFacts)
	heaps := access.factRelation(heapAxis, heapFacts)
	packs := access.factRelation(packAxis, packFacts)

	keyRoutes := access.step("raw-set/key-routes", candidates, values, "heap/raw-set-key-routes", 64)
	heapRoutes := access.step("raw-set/heap-routes", keyRoutes, values, "heap/raw-set-heap-routes", 64)
	packRoutes := access.step("raw-set/pack-routes", heapRoutes, heaps, "heap/raw-set-pack-routes", 64)
	sourceRoutes := access.step("raw-set/source-routes", packRoutes, packs, "heap/raw-set-source-routes", 64)

	update := relationName(heapAxis, "heap/raw-set-cell-update")
	access.surfaces.operation(update, access.destinationColumn(heaps), 3, candidates)
	access.rules = append(access.rules, relcompile.Rule{
		ID:         access.dependencyOf("raw-set/commit"),
		Expression: access.expressionOf("raw-set/commit"),
		Candidate:  access.relation(candidates),
		Joins: []relcompile.JoinSpec{
			access.join(candidates, values),
			access.join(candidates, sourceRoutes),
			access.completed(access.join(sourceRoutes, heaps), heaps),
		},
		Scope:   access.scopeID(),
		Apply:   access.signature(update),
		Carry:   &relcompile.CarrySpec{Relation: access.relation(heaps), Scope: access.portScope()},
		Publish: &relcompile.Publication{Relation: access.relation(heaps), Key: access.publicationKey(heaps)},
	})
	return access
}

func (access *rawAccess) compile(t *testing.T, label schema.Key) planResult {
	t.Helper()
	declaration := access.surfaces.registry.Declaration(access.surfaces.schema())
	declaration.Rules = access.rules
	compiled, err := relcompile.Compile(declaration)
	if err != nil {
		t.Fatalf("compile %s: %v", label, err)
	}
	return planResult{count: len(compiled.Expressions()), sketch: sketch(compiled), certified: certification(compiled)}
}

type planResult struct {
	count     int
	sketch    string
	certified string
}

// rawAccessCensus contributes the two authored raw indexed access rows. Their
// declarations carry no Program today, so the plan is authored here from the
// read chain the hot rule executes.
func rawAccessCensus(t *testing.T) []entry {
	t.Helper()
	get := rawGetPlan(t).compile(t, "raw-get")
	set := rawSetPlan(t).compile(t, "raw-set")
	return []entry{
		{
			Family: "heap/index", Plane: "family", Rule: "raw-get", Status: statusCompiles,
			Sketch: get.sketch, Expressions: get.count, Certified: get.certified,
			Reason: "plan authored from the read chain; the rule declaration carries no Program",
		},
		{
			Family: "heap/index", Plane: "family", Rule: "raw-set", Status: statusCompiles,
			Sketch: set.sketch, Expressions: set.count, Certified: set.certified,
			Reason: "plan authored from the read chain; the rule declaration carries no Program",
		},
	}
}

// TestRawGetChainLowersToExpansionsAndOneReduction states that raw-get's
// dependent six-read chain lowers without an operator switch arm: each read
// whose coordinate depends on an earlier cell is one finite expansion
// publishing a route relation, and the result is one typed reduction.
func TestRawGetChainLowersToExpansionsAndOneReduction(t *testing.T) {
	access := rawGetPlan(t)
	if len(access.rules) != 6 {
		t.Fatalf("raw-get dependencies = %d, want five route expansions and one reduction", len(access.rules))
	}
	result := access.compile(t, "raw-get")
	if result.count != 6 {
		t.Fatalf("raw-get expressions = %d, want one per dependency", result.count)
	}
	for index, declared := range access.rules {
		if declared.Publish == nil {
			t.Fatalf("raw-get dependency %d publishes nothing", index)
		}
		if !declared.Apply.Available() {
			t.Fatalf("raw-get dependency %d names no semantic operation", index)
		}
	}
	final := access.rules[len(access.rules)-1]
	if final.Carry == nil {
		t.Fatal("raw-get's reduction drops the receiver fact it carries")
	}
	if len(final.Joins) != 3 {
		t.Fatalf("raw-get reduction joins = %d, want the receiver, the route chain and its payload", len(final.Joins))
	}
}

// TestRawSetChainEndsInAnAuthenticatedHeapPublication states that raw-set's
// dependent chain lowers the same way and that its write is a publication
// into the heap denominator its route selected, never a second write door.
func TestRawSetChainEndsInAnAuthenticatedHeapPublication(t *testing.T) {
	access := rawSetPlan(t)
	if len(access.rules) != 5 {
		t.Fatalf("raw-set dependencies = %d, want four route expansions and one cell update", len(access.rules))
	}
	commit := access.rules[len(access.rules)-1]
	completed := 0
	for _, join := range commit.Joins {
		if join.Complete != nil {
			completed++
		}
	}
	if completed != 1 {
		t.Fatalf("raw-set completions = %d, want the heap denominator the update is authenticated against", completed)
	}
	if commit.Publish == nil {
		t.Fatal("raw-set's cell update publishes nothing")
	}
	result := access.compile(t, "raw-set")
	if result.count != 5 {
		t.Fatalf("raw-set expressions = %d, want one per dependency", result.count)
	}
}

// expansion installs one finite expansion signature: a bounded output span
// over the destination's own denominator, which is what makes a value-
// dependent route emission expressible without a callback.
func (surfaces *owners) expansion(name relcompile.Name, destination relcompile.Name, bound uint32, reads int, candidate relcompile.Name) {
	surfaces.t.Helper()
	surfaces.owner(name.Entry)
	if !surfaces.once("operation", name) {
		return
	}
	if err := surfaces.registry.InstallOperation(name, surfaces.token("operation", name)); err != nil {
		surfaces.t.Fatalf("install operation %v: %v", name, err)
	}
	operation, err := surfaces.registry.Operation(relcompile.Site{Path: "census.expansion"}, name)
	if err != nil {
		surfaces.t.Fatalf("resolve operation %v: %v", name, err)
	}
	owner, err := surfaces.registry.Owner(relcompile.Site{Path: "census.expansion"}, name.Entry)
	if err != nil {
		surfaces.t.Fatalf("resolve operation owner %v: %v", name, err)
	}
	schemaID := surfaces.schema()
	site := relcompile.Site{Path: "census.expansion"}
	// The rows an expansion publishes are computed from the results it reads,
	// so it is applied to the same kind of projected operand row a fold is.
	operand := candidate
	first := surfaces.coordinateName(candidate, relcompile.CoordinateAddress)
	if reads != 0 {
		operand = relcompile.NewName(name.Entry, name.Member+"#operand")
		first = surfaces.operandRelation(operand, reads)
	}
	reference := surfaces.denominatorOf(operand)
	column, err := surfaces.registry.Column(site, first)
	if err != nil {
		surfaces.t.Fatalf("resolve operand column %v: %v", first, err)
	}
	delivery, ok := signature.NewBoundedSpanDelivery(bound, reference.Key())
	if !ok {
		surfaces.t.Fatal("construct bounded span delivery")
	}
	valueType, err := surfaces.registry.ColumnType(site, first)
	if err != nil {
		surfaces.t.Fatalf("resolve destination type %v: %v", destination, err)
	}
	cardinality, ok := model.NewCardinality(model.BoundedMany, bound)
	if !ok {
		surfaces.t.Fatal("construct bounded cardinality")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		surfaces.t.Fatal("construct outcome set")
	}
	sealed, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{{
			Relation: reference.Relation(), Column: column, Type: valueType,
			Presence: signature.AllowMissing, Delivery: delivery, Denominator: reference,
		}},
		// An expansion publishes rows, and a row is whole: the operation yields
		// every column the relation it lands in declares, so the publication
		// of its result owes nothing the operation did not produce.
		Outputs:     surfaces.destinationOutputs(surfaces.relationOf(destination)),
		Authority:   signature.OutputAuthority{Denominator: surfaces.denominatorOf(surfaces.relationOf(destination))},
		Cardinality: cardinality,
		Outcomes:    accepted,
	})
	if !ok {
		surfaces.t.Fatalf("seal expansion signature for %v", name)
	}
	if err := surfaces.registry.InstallSignature(name, sealed); err != nil {
		surfaces.t.Fatalf("install signature %v: %v", name, err)
	}
}
