package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/authority"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
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
//
// An expansion states one join per fact its enumeration reads, from the spine
// the chain already carries onto the relation that fact lives in, and the
// operand projection carries those reads into the row the operation is applied
// to. A read the enumeration makes and the join list omits is a frame the
// operation asks for and the plan never delivers, so the reads are stated here
// and nowhere else.
func (access *rawAccess) step(rule schema.Key, candidate relcompile.Name, route schema.Key, bound uint32, reads ...relcompile.Name) relcompile.Name {
	access.t.Helper()
	routeRelation := access.factRelation(heapAxis, route)
	operation := relationName(heapAxis, route+"#expansion")
	access.surfaces.expansion(operation, access.destinationColumn(routeRelation), bound, len(reads), candidate)
	joins := make([]relcompile.JoinSpec, 0, len(reads))
	for _, read := range reads {
		joins = append(joins, access.join(candidate, read))
	}
	access.rules = append(access.rules, relcompile.Rule{
		ID:         access.dependencyOf(rule),
		Expression: access.expressionOf(rule),
		Candidate:  access.relation(candidate),
		Joins:      joins,
		Scope:      access.scopeID(),
		Operand:    access.operandOf(operation, joins),
		Apply:      access.signature(operation),
		Publish:    &relcompile.Publication{Relation: access.relation(routeRelation), Key: access.publicationKey(routeRelation)},
	})
	return routeRelation
}

// operandOf projects the joined row onto the relation the operation reads.
//
// The operation's signature names that relation, and it declares one column
// per read the rule makes, so the column at position i is defined by the read
// at position i. Every column is defined exactly once, which is what makes the
// Apply's input a column of its own child rather than a column the plan hoped
// would be there.
func (access *rawAccess) operandOf(operation relcompile.Name, joins []relcompile.JoinSpec) *relcompile.Operand {
	access.t.Helper()
	site := relcompile.Site{Rule: "raw-access", Path: "operand"}
	name := relcompile.NewName(operation.Entry, operation.Member+"#operand")
	relation, err := access.surfaces.registry.Relation(site, name)
	if err != nil {
		access.t.Fatalf("resolve operand relation %v: %v", name, err)
	}
	key, err := access.surfaces.registry.RelationKeyOf(site, relation)
	if err != nil {
		access.t.Fatalf("resolve operand key of %v: %v", name, err)
	}
	targets, err := access.surfaces.registry.RelationColumns(site, relation)
	if err != nil {
		access.t.Fatalf("resolve operand columns of %v: %v", name, err)
	}
	if len(targets) != len(joins) {
		access.t.Fatalf("operand %v declares %d columns and the rule makes %d reads", name, len(targets), len(joins))
	}
	mappings := make([]relcompile.ColumnMapping, 0, len(targets))
	for index, target := range targets {
		mappings = append(mappings, relcompile.ColumnMapping{Source: joins[index].RightColumns[0], Target: target})
	}
	return &relcompile.Operand{Relation: relation, Key: key, Columns: mappings}
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

	keyRoutes := access.step("raw-get/key-routes", candidates, "heap/raw-get-key-routes", 64, values)
	callRoutes := access.step("raw-get/call-routes", keyRoutes, "heap/raw-get-call-routes", 64, values)
	heapRoutes := access.step("raw-get/heap-routes", callRoutes, "heap/raw-get-heap-routes", 64, calls)
	// The pack expansion enumerates the payloads one selected route carries
	// under a key selector, and the selectors are the ones the read candidate
	// resolves to under the value its key selected. So it reads the heap fact
	// the route selected, the candidate whose key geometry decides whether the
	// selection is read at all, and the key value the selectors project from.
	// The key route states the coordinate and never the value at it, so it is
	// not the relation this read observes.
	packRoutes := access.step("raw-get/pack-routes", heapRoutes, "heap/raw-get-pack-routes", 64, heaps, candidates, values)
	sourceRoutes := access.step("raw-get/source-routes", packRoutes, "heap/raw-get-source-routes", 64, packs)

	reduction := relationName(heapAxis, "heap/raw-get-reduction")
	// The reduction re-enumerates the receiver's calls and rooted routes and
	// looks up the fact each one selected, so its frame is the value, call,
	// heap, source and pack selections together.
	reductionJoins := []relcompile.JoinSpec{
		access.join(candidates, values),
		access.join(candidates, calls),
		access.join(candidates, heaps),
		access.join(candidates, sourceRoutes),
		access.join(sourceRoutes, packs),
	}
	access.surfaces.operation(reduction, access.destinationColumn(values), len(reductionJoins), candidates)
	access.rules = append(access.rules, relcompile.Rule{
		ID:         access.dependencyOf("raw-get/result"),
		Expression: access.expressionOf("raw-get/result"),
		Candidate:  access.relation(candidates),
		Joins:      reductionJoins,
		Scope:      access.scopeID(),
		Operand:    access.operandOf(reduction, reductionJoins),
		Apply:      access.signature(reduction),
		Carry:      &relcompile.CarrySpec{Relation: access.relation(values), Scope: access.portScope()},
		Publish:    &relcompile.Publication{Relation: access.relation(values), Key: access.publicationKey(values)},
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

	keyRoutes := access.step("raw-set/key-routes", candidates, "heap/raw-set-key-routes", 64, values)
	heapRoutes := access.step("raw-set/heap-routes", keyRoutes, "heap/raw-set-heap-routes", 64, values)
	// The write side's pack expansion is the same enumeration over the same
	// three reads: the heap fact the route selected, the write candidate whose
	// key geometry decides whether the key selection is read, and the key value
	// the selectors project from.
	packRoutes := access.step("raw-set/pack-routes", heapRoutes, "heap/raw-set-pack-routes", 64, heaps, candidates, values)
	sourceRoutes := access.step("raw-set/source-routes", packRoutes, "heap/raw-set-source-routes", 64, packs)

	update := relationName(heapAxis, "heap/raw-set-cell-update")
	// The cell update writes the payload each source names into the route it
	// selected, so its frame is the value, pack and source selections together
	// with the heap denominator row it is authenticated against.
	updateJoins := []relcompile.JoinSpec{
		access.join(candidates, values),
		access.join(candidates, packs),
		access.join(candidates, sourceRoutes),
		access.completed(access.join(sourceRoutes, heaps), heaps),
	}
	access.surfaces.operation(update, access.destinationColumn(heaps), len(updateJoins), candidates)
	access.rules = append(access.rules, relcompile.Rule{
		ID:         access.dependencyOf("raw-set/commit"),
		Expression: access.expressionOf("raw-set/commit"),
		Candidate:  access.relation(candidates),
		Joins:      updateJoins,
		Scope:      access.scopeID(),
		Operand:    access.operandOf(update, updateJoins),
		Apply:      access.signature(update),
		Carry:      &relcompile.CarrySpec{Relation: access.relation(heaps), Scope: access.portScope()},
		Publish:    &relcompile.Publication{Relation: access.relation(heaps), Key: access.publicationKey(heaps)},
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
	if len(final.Joins) != 5 {
		t.Fatalf("raw-get reduction joins = %d, want the receiver, the call and heap fact selections, the route chain and its payload", len(final.Joins))
	}
}

// TestEveryRawAccessOperationReadsARowItsPlanBuilds states the closure of the
// plan gap. An operation is applied to the relation its signature names, so
// the rule that applies it owes a projection that defines every column of that
// relation, and it owes one read per column. A rule that applied its operation
// to the joined row would be asking for a column no child of the Apply carries,
// which is a frame the plan never delivered.
func TestEveryRawAccessOperationReadsARowItsPlanBuilds(t *testing.T) {
	for _, plan := range []struct {
		label  schema.Key
		access *rawAccess
	}{{"raw-get", rawGetPlan(t)}, {"raw-set", rawSetPlan(t)}} {
		for index, declared := range plan.access.rules {
			if declared.Operand == nil {
				t.Fatalf("%s dependency %d applies an operation to a row its plan never builds", plan.label, index)
			}
			if len(declared.Operand.Columns) != len(declared.Joins) {
				t.Fatalf("%s dependency %d projects %d columns from %d reads", plan.label, index, len(declared.Operand.Columns), len(declared.Joins))
			}
			defined := map[model.ColumnID]bool{}
			for _, mapping := range declared.Operand.Columns {
				if defined[mapping.Target] {
					t.Fatalf("%s dependency %d defines one operand column twice", plan.label, index)
				}
				defined[mapping.Target] = true
			}
		}
	}
}

// TestEveryRawAccessPlanCertifies states that both authored plans pass the
// independent checker on everything the plan itself states. The scope-order
// finding is the census harness's own port scope, carried by every family in
// the matrix, so it is not a statement about raw access.
func TestEveryRawAccessPlanCertifies(t *testing.T) {
	for _, plan := range []struct {
		label  schema.Key
		access *rawAccess
	}{{"raw-get", rawGetPlan(t)}, {"raw-set", rawSetPlan(t)}} {
		declaration := plan.access.surfaces.registry.Declaration(plan.access.surfaces.schema())
		declaration.Rules = plan.access.rules
		compiled, err := relcompile.Compile(declaration)
		if err != nil {
			t.Fatalf("compile %s: %v", plan.label, err)
		}
		_, refusal := certificate.Check(compiled)
		for _, issue := range refusal.Issues() {
			if issue.Pass == certificate.PassAuthority && issue.Code == uint16(authority.CodeInvalidScopeOrder) {
				continue
			}
			t.Errorf("%s: %s[%d] at %s: %s", plan.label, issue.Pass, issue.Code, issue.Path, issue.Detail)
		}
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
