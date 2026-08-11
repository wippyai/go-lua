package activation

import (
	"crypto/sha256"
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

var activationOperation = target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"call_activation_test_operation"}}

type activationFixture struct {
	source      *link.Link
	program     *program.Program
	algebra     *calldomain.Algebra
	key         calldomain.Key
	application linkproject.Application
	bodyTargets []calldomain.Target
	external    calldomain.Target
}

func newActivationFixture(t testing.TB, name string) activationFixture {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(`
local function first() return 1 end
local function second() return 2 end
local function invoke(value) return value() end
invoke(first)
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{activationOperation},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	algebra, ok := calldomain.New(source)
	if !ok {
		t.Fatal("call algebra")
	}
	calls := source.Project().Applications().Calls()
	if calls.Count() == 0 {
		t.Fatal("call application")
	}
	application, ok := calls.At(0)
	if !ok {
		t.Fatal("call application row")
	}
	key, ok := algebra.KeyForApplication(application)
	if !ok {
		t.Fatal("call key")
	}
	var bodyTargets []calldomain.Target
	var external calldomain.Target
	for index := 0; index < algebra.SupportCount(key); index++ {
		candidate, candidateOK := algebra.SupportTargetAt(key, index)
		if !candidateOK {
			t.Fatal("call support target")
		}
		if _, body := candidate.Body(); body {
			bodyTargets = append(bodyTargets, candidate)
		}
	}
	contractValue, ok := source.Boundary().Target()
	if !ok {
		t.Fatal("target contract")
	}
	operation, ok := contractValue.Lookup(activationOperation)
	if !ok {
		t.Fatal("operation")
	}
	seed, ok := source.Boundary().Seeds().ForOperation(operation)
	if !ok {
		t.Fatal("operation seed")
	}
	external, ok = algebra.TargetForSeed(seed)
	if !ok || len(bodyTargets) < 2 {
		t.Fatalf("fixture targets: bodies=%d external=%v", len(bodyTargets), ok)
	}
	return activationFixture{source: source, program: p, algebra: algebra, key: key, application: application, bodyTargets: bodyTargets, external: external}
}

func activationSemantic(value byte) engine.SemanticKey {
	digest := sha256.Sum256([]byte{value})
	key, _ := engine.NewSemanticKey(digest, 1)
	return key
}

func sourceRoutes(fixture activationFixture) []route {
	routes := make([]route, len(fixture.bodyTargets))
	for index, target := range fixture.bodyTargets {
		body, _ := target.Body()
		routes[index] = route{body: body, target: activationSemantic(byte(index + 1)), endpoint: activationSemantic(byte(index + 101))}
	}
	return routes
}

func publicSourceRoutes(fixture activationFixture) []Route {
	private := sourceRoutes(fixture)
	routes := make([]Route, len(private))
	for index, item := range private {
		routes[index] = Route{Body: item.body, Target: item.target, Endpoint: item.endpoint}
	}
	return routes
}

func TestBodySelectorSelectedKnownAndExternalUnselected(t *testing.T) {
	fixture := newActivationFixture(t, "call_activation_selected")
	source := &Source{bodies: fixture.algebra.Bodies(), routes: sourceRoutes(fixture)}
	bodyTarget := fixture.bodyTargets[0]
	value, ok := fixture.algebra.DispatchValue(fixture.key, []calldomain.Target{bodyTarget, fixture.external}, false)
	if !ok {
		t.Fatal("dispatch value")
	}
	count := 0
	if !source.visitRoutes(value, func(item route) bool {
		count++
		body, bodyOK := bodyTarget.Body()
		return bodyOK && item.body.Same(body)
	}) {
		t.Fatal("selected body traversal")
	}
	if count != 1 {
		t.Fatalf("selected body count=%d", count)
	}
}

func TestBodySelectorTopSelectsEveryCanonicalBody(t *testing.T) {
	fixture := newActivationFixture(t, "call_activation_top")
	source := &Source{bodies: fixture.algebra.Bodies(), routes: sourceRoutes(fixture)}
	count := 0
	if !source.visitRoutes(fixture.algebra.Top(), func(item route) bool {
		if !item.body.Valid() {
			return false
		}
		count++
		return true
	}) {
		t.Fatal("top body traversal")
	}
	if count != fixture.algebra.Bodies().Count() {
		t.Fatalf("top selected %d bodies, want %d", count, fixture.algebra.Bodies().Count())
	}
}

func TestBodySelectorRejectsForeignCallOwner(t *testing.T) {
	fixture := newActivationFixture(t, "call_activation_foreign_left")
	foreign := newActivationFixture(t, "call_activation_foreign_right")
	source := &Source{bodies: fixture.algebra.Bodies(), routes: sourceRoutes(fixture)}
	foreignTarget := foreign.bodyTargets[0]
	value, ok := foreign.algebra.DispatchValue(foreign.key, []calldomain.Target{foreignTarget}, false)
	if !ok {
		t.Fatal("foreign dispatch value")
	}
	if source.visitRoutes(value, func(route) bool { return true }) {
		t.Fatal("foreign Call owner crossed body activation seam")
	}
	if _, ok := source.routeFor(func() calldomain.Body { body, _ := foreignTarget.Body(); return body }()); ok {
		t.Fatal("foreign Body resolved through local route cursor")
	}
}

func TestPrepareKeepsActivationRulePrivateAndFencesAssemblyReuse(t *testing.T) {
	fixture := newActivationFixture(t, "call_activation_prepare")
	composition := engine.NewComposition()
	owner, ownerOK := callowner.Declare(composition, activationSemantic(210), fixture.algebra)
	if !ownerOK || owner == nil {
		t.Fatal("Call owner")
	}
	_, queryOK := engine.DeclareQuery[bool](composition, engine.QuerySpec[bool]{
		Semantic: activationSemantic(216),
		Project:  func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: activationSemantic(217),
			Freeze:   func(value bool) bool { return value },
			Clone:    func(value bool) bool { return value },
			Equal:    func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		_, readOK := engine.QueryReadFrom(query, owner.ExactRead())
		return readOK
	})
	if !queryOK {
		t.Fatal("activation preparation query")
	}
	source, sourceOK := Declare(composition, Spec{
		Link: fixture.source, Calls: owner,
		Semantic: activationSemantic(211), Family: activationSemantic(212), Admission: activationSemantic(213),
		Routes: publicSourceRoutes(fixture),
	})
	if !sourceOK || source == nil {
		t.Fatal("body activation source")
	}
	if !composition.Seal() {
		t.Fatal("composition seal")
	}
	assembly := engine.NewSourceAssembly(composition)
	scope, scopeOK := assembly.Scope()
	truth, truthOK := assembly.TrueExpr()
	site, siteOK := assembly.Site(activationSemantic(214), scope, truth, true)
	occurrence, occurrenceOK := assembly.At(site)
	entity := activationSemantic(215)
	prepared, preparedOK := source.Prepare(assembly, occurrence, entity)
	if !scopeOK || !truthOK || !siteOK || !occurrenceOK || !preparedOK || prepared.Available() {
		t.Fatal("late-bound preparation before source seal")
	}
	foreign := engine.NewSourceAssembly(composition)
	if _, foreignOK := source.Prepare(foreign, occurrence, entity); foreignOK {
		t.Fatal("foreign SourceAssembly accepted")
	}
	if !assembly.Seal() || !prepared.Available() {
		t.Fatal("prepared activation did not publish with source seal")
	}
	if _, reused := source.Prepare(assembly, occurrence, entity); reused {
		t.Fatal("sealed SourceAssembly accepted activation reuse")
	}
}

func TestStageUsesOwnerIssuedCarryForm(t *testing.T) {
	fixture := newActivationFixture(t, "call_activation_carry")
	composition := engine.NewComposition()
	owner, ownerOK := callowner.Declare(composition, activationSemantic(220), fixture.algebra)
	if !ownerOK || owner == nil {
		t.Fatal("Call owner")
	}
	_, queryOK := engine.DeclareQuery[bool](composition, engine.QuerySpec[bool]{
		Semantic: activationSemantic(221),
		Project:  func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: activationSemantic(222),
			Freeze:   func(value bool) bool { return value },
			Clone:    func(value bool) bool { return value },
			Equal:    func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		_, readOK := engine.QueryReadFrom(query, owner.ExactRead())
		return readOK
	})
	if !queryOK {
		t.Fatal("Carry test query")
	}
	source, sourceOK := Declare(composition, Spec{
		Link: fixture.source, Calls: owner,
		Semantic: activationSemantic(223), Family: activationSemantic(224), Admission: activationSemantic(225),
		Routes: publicSourceRoutes(fixture),
	})
	if !sourceOK || source == nil || !composition.Seal() {
		t.Fatal("body activation declaration")
	}
	assembly := engine.NewSourceAssembly(composition)
	scope, scopeOK := assembly.Scope()
	truth, truthOK := assembly.TrueExpr()
	sourceSite, sourceSiteOK := assembly.Site(activationSemantic(226), scope, truth, true)
	targetSite, targetSiteOK := assembly.Site(activationSemantic(227), scope, truth, true)
	if !scopeOK || !truthOK || !sourceSiteOK || !targetSiteOK {
		t.Fatal("Carry endpoints")
	}
	carry := owner.Carry()
	entries := make([]Entry, len(fixture.bodyTargets))
	for index, target := range fixture.bodyTargets {
		body, bodyOK := target.Body()
		if !bodyOK {
			t.Fatal("body target")
		}
		entries[index] = Entry{
			Body: body, Target: activationSemantic(byte(index + 1)), Endpoint: activationSemantic(byte(index + 101)),
			FactorEdges: []engine.ActivationFactorEdge{{
				SourceSite: sourceSite, TargetSite: targetSite, Factor: carry, Provenance: activationSemantic(250),
			}},
		}
	}
	session, staged := source.Stage(assembly, entries)
	if !staged || session == nil {
		t.Fatal("Carry-backed activation stage")
	}
	if !assembly.Seal() {
		t.Fatal("Carry-backed source seal")
	}
	plan, finalized := session.Finalize()
	if !finalized || plan == nil || plan.EndpointCount() != len(entries) {
		t.Fatal("Carry-backed activation finalization")
	}
}
