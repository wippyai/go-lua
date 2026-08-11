package source

import (
	"context"
	"encoding/binary"
	"testing"

	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestCallSourcesKeepPerOccurrencePacks proves the source cut at the
// domain boundary.  Two method calls in one body get different Pack roots;
// each source carries receiver::actuals, and the open actuals tail remains a
// Pack-shaped unresolved tail rather than degrading to an arbitrary scalar.
func TestCallSourcesKeepPerOccurrencePacks(t *testing.T) {
	schema, linked, selected := sourceSchema(t)
	applications := methodApplications(t, linked)
	if len(applications) != 2 {
		t.Fatalf("method calls=%d, want 2", len(applications))
	}

	roots := make(map[packdomain.Root]struct{}, len(applications))
	for index, application := range applications {
		root, ok := schema.CallRoot(application)
		if !ok {
			t.Fatalf("call %d Pack root", index)
		}
		if _, duplicate := roots[root]; duplicate {
			t.Fatal("distinct calls in one Program body shared a Pack root")
		}
		roots[root] = struct{}{}
		source, ok := schema.Source(root)
		if !ok {
			t.Fatalf("call %d has no authored Pack source", index)
		}
		owner := sourceOwner(t, schema)
		gotRoot, fact, ok := result(owner, source)
		if !ok || gotRoot != root || !schema.Admit(root, fact) {
			t.Fatalf("call %d source did not build its complete Pack fact", index)
		}

		assertExactInputPosition(t, schema, linked, application, root, fact, selected, 0)
		assertExactInputPosition(t, schema, linked, application, root, fact, selected, 1)

		tailSelector, ok := schema.InputSelector(selected, target.InputSource{Kind: target.InputSourceValuesVar})
		if !ok {
			t.Fatalf("call %d ValuesVar selector", index)
		}
		tail, ok := schema.ObserveInput(root, fact, tailSelector)
		if !ok || tail.TermCount() != 1 {
			t.Fatalf("call %d ValuesVar collapsed its Pack shape", index)
		}
		var exact []linkboundary.Value
		complete, visited := schema.VisitInputSources(tail, func(value linkboundary.Value) bool {
			exact = append(exact, value)
			return true
		})
		if !visited || complete || len(exact) != 0 {
			t.Fatalf("call %d open ValuesVar was not retained as unresolved Pack tail: complete=%v exact=%d", index, complete, len(exact))
		}
	}
}

// TestRulePublishesOneExactAuthoredPack exercises the production Rule through
// the sole runtime path.  A result is observable only if the rule's own
// derivation checker accepted the exact source descriptor and root binding.
func TestRulePublishesOneExactAuthoredPack(t *testing.T) {
	schema, linked, _ := sourceSchema(t)
	applications := methodApplications(t, linked)
	if len(applications) == 0 {
		t.Fatal("method call")
	}
	root, ok := schema.CallRoot(applications[0])
	if !ok {
		t.Fatal("Pack root")
	}
	sourceOperand, ok := schema.Source(root)
	if !ok {
		t.Fatal("Pack source")
	}

	composition := engine.NewComposition()
	owner, ok := packowner.Declare(composition, sourceKey(1), schema)
	if !ok {
		t.Fatal("Pack owner")
	}
	rule, ok := Declare(composition, sourceKey(2), sourceKey(3), sourceKey(4), owner)
	if !ok || rule == nil {
		t.Fatal("Pack source Rule")
	}
	_, expected, ok := result(owner, sourceOperand)
	if !ok {
		t.Fatal("Pack source result")
	}
	var read engine.QueryRead[engine.OrderedCells[packdomain.Value]]
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: sourceKey(5),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				actual, present, valueOK := cells.At(0)
				return rows == 1 && cellsOK && valueOK && present && schema.Lattice().Equal(actual, expected)
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: sourceKey(6),
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
		var declared bool
		read, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !ok || query == nil || !composition.Seal() {
		t.Fatal("Pack source composition seal")
	}
	ref, ok := owner.Locate(root)
	if !ok {
		t.Fatal("Pack source root ref")
	}
	instance, ok := rule.Instance(sourceOperand)
	if !ok || instance == nil {
		t.Fatal("Pack source Rule instance")
	}
	result := testlaw.Run(context.Background(), testlaw.RuleFixture[packdomain.Value, packdomain.Source, bool]{
		Composition: composition, Instance: instance, Query: query,
		SiteSemantic: sourceKey(7), OccurrenceSemantic: sourceKey(8),
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, ref)
		},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("Pack source runtime law = status:%v available:%v value:%v", result.Status, result.ValueAvailable, result.Value)
	}
}

func assertExactInputPosition(t testing.TB, schema *packdomain.Schema, linked *link.Link, application linkproject.Application, root packdomain.Root, fact packdomain.Value, operation target.Operation, ordinal uint32) {
	t.Helper()
	selector, ok := schema.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: ordinal})
	if !ok {
		t.Fatalf("formal %d selector", ordinal)
	}
	observation, ok := schema.ObserveInput(root, fact, selector)
	if !ok || observation.ScalarCount() != 1 {
		t.Fatalf("formal %d scalar observation", ordinal)
	}
	var exact []linkboundary.Value
	complete, visited := schema.VisitInputSources(observation, func(value linkboundary.Value) bool {
		exact = append(exact, value)
		return true
	})
	want, wantOK := directMethodInputPosition(linked, application, ordinal)
	if !visited || !complete || len(exact) != 1 || !wantOK || want == (linkboundary.Value{}) || exact[0] != want {
		t.Fatalf("formal %d lost exact endpoint: complete=%v exact=%d", ordinal, complete, len(exact))
	}
}

func sourceSchema(t testing.TB) (*packdomain.Schema, *link.Link, target.Operation) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "pack_source_rule.lua", Text: []byte(`
local function many(...) return ... end
local object = {}
object:send(1, many(2))
object:send(3, many(4))
`)})
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"selected"}}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings:   []target.BindingSpec{binding},
		ValuesVars: 1,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: target.ValuesVariable, Var: 0},
		Outcomes:   []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:    target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	selected, ok := contract.Lookup(binding)
	if !ok {
		t.Fatal("selected Target operation")
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_source_rule", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := packdomain.Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}
	return schema, linked, selected
}

func methodApplications(t testing.TB, linked *link.Link) []linkproject.Application {
	t.Helper()
	var applications []linkproject.Application
	project := linked.Project()
	projectApplications := project.Applications()
	calls := projectApplications.Calls()
	for index := 0; index < calls.Count(); index++ {
		application, ok := calls.At(index)
		if !ok {
			t.Fatal("CallApplicationAt")
		}
		shard, callTerm, callOK := projectApplications.Call(application)
		p, programOK := project.Mounts().Program(shard)
		if !callOK || !programOK || p == nil {
			continue
		}
		_, _, receiver, _, operandsOK := p.Flow().Authored().Calls().Get(callTerm)
		if operandsOK && receiver != 0 {
			applications = append(applications, application)
		}
	}
	return applications
}

func directMethodInputPosition(linked *link.Link, application linkproject.Application, ordinal uint32) (linkboundary.Value, bool) {
	project := linked.Project()
	shard, callTerm, callOK := project.Applications().Call(application)
	p, programOK := project.Mounts().Program(shard)
	if !callOK || !programOK || p == nil {
		return linkboundary.Value{}, false
	}
	_, _, receiverTerm, actuals, operandsOK := p.Flow().Authored().Calls().Get(callTerm)
	if !operandsOK || receiverTerm == 0 {
		return linkboundary.Value{}, false
	}
	if ordinal == 0 {
		return linked.Boundary().Values().Of(shard, receiverTerm)
	}
	term, valueOK := p.Flow().Authored().Values().Member(actuals, int(ordinal-1))
	if !valueOK {
		return linkboundary.Value{}, false
	}
	value, linkedOK := linked.Boundary().Values().Of(shard, term)
	return value, linkedOK
}

func sourceOwner(t testing.TB, schema *packdomain.Schema) *packowner.Owner {
	t.Helper()
	composition := engine.NewComposition()
	owner, ok := packowner.Declare(composition, sourceKey(91), schema)
	if !ok {
		t.Fatal("Pack owner")
	}
	return owner
}

func sourceKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("Pack source semantic key")
	}
	return key
}
