package link

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// TestCallOperandsPreservesDirectSourceTuple proves Link exposes exactly the
// sealed Program Call operands, without creating a boundary wrapper or target
// projection vocabulary.
func TestCallOperandsPreservesDirectSourceTuple(t *testing.T) {
	p := source(t, `
local object = {}
object:run(1)
plain(3)
local arithmetic = 1 + 2
`)
	linked := linked(t, testBootContract(t, nil, p), linkproject.Module{Name: "main", Program: p})
	methodCall := methodCall(t, p)
	plainCall := globalCall(t, p, "plain")
	method := callApplicationForCall(t, linked, methodCall)
	plain := callApplicationForCall(t, linked, plainCall)

	_, methodCallee, methodReceiver, methodActuals, methodOK := p.Flow().Authored().Calls().Get(methodCall)
	if !methodOK || methodCallee == 0 || methodReceiver == 0 || methodActuals == 0 {
		t.Fatal("method Program Call")
	}
	methodCalleeValue, methodCalleeOK := linked.Boundary().Calls().Callee(method)
	if !methodCalleeOK || !valueOriginIs(linked, methodCalleeValue, methodCallee) {
		t.Fatalf("method call callee = %v/%t", methodCalleeValue, methodCalleeOK)
	}
	form, receiver, actuals, operandsOK := linked.Boundary().Calls().CallOperands(method)
	if !operandsOK || form != flow.CallFormMethod || !valueOriginIs(linked, receiver, methodReceiver) || !valueOriginIs(linked, actuals, methodActuals) {
		t.Fatalf("method call operands = form:%v receiver:%v actuals:%v ok:%v", form, receiver, actuals, operandsOK)
	}

	_, plainCallee, plainReceiver, plainActuals, plainOK := p.Flow().Authored().Calls().Get(plainCall)
	if !plainOK || plainCallee == 0 || plainReceiver != 0 || plainActuals == 0 {
		t.Fatal("plain Program Call")
	}
	plainCalleeValue, plainCalleeOK := linked.Boundary().Calls().Callee(plain)
	if !plainCalleeOK || !valueOriginIs(linked, plainCalleeValue, plainCallee) {
		t.Fatalf("plain call callee = %v/%t", plainCalleeValue, plainCalleeOK)
	}
	form, receiver, actuals, operandsOK = linked.Boundary().Calls().CallOperands(plain)
	if !operandsOK || form != flow.CallFormPlain || receiver != (linkboundary.Value{}) || !valueOriginIs(linked, actuals, plainActuals) {
		t.Fatalf("plain call operands = form:%v receiver:%v actuals:%v ok:%v", form, receiver, actuals, operandsOK)
	}
	if _, _, _, ok := linked.Boundary().Calls().CallOperands(linkproject.Application{}); ok {
		t.Fatal("zero Application yielded call operands")
	}
	if _, ok := linked.Boundary().Calls().Callee(linkproject.Application{}); ok {
		t.Fatal("zero Application yielded callee")
	}
	applications := linked.Project().Applications()
	var arithmetic linkproject.Application
	for index := 0; index < applications.Count(); index++ {
		application, ok := applications.At(index)
		if !ok {
			t.Fatalf("Project Application %d unavailable", index)
		}
		if _, _, ok := applications.Operators().Arithmetic(application); ok {
			arithmetic = application
			break
		}
	}
	if arithmetic == (linkproject.Application{}) {
		t.Fatal("arithmetic application unavailable")
	}
	if _, ok := linked.Boundary().Calls().Callee(arithmetic); ok {
		t.Fatal("non-Call application yielded callee")
	}
	foreign := linkedForOperands(t, p)
	if _, _, _, ok := foreign.Boundary().Calls().CallOperands(method); ok {
		t.Fatal("foreign Link accepted call Application")
	}
	if _, ok := foreign.Boundary().Calls().Callee(method); ok {
		t.Fatal("foreign Link accepted call Application callee")
	}
}

func linkedForOperands(t testing.TB, p *program.Program) *Link {
	t.Helper()
	return linked(t, testBootContract(t, nil, p), linkproject.Module{Name: "foreign", Program: p})
}

func callApplicationForCall(t testing.TB, linked *Link, call keyspace.Term) linkproject.Application {
	t.Helper()
	calls := linked.Project().Applications().Calls()
	for index := 0; index < calls.Count(); index++ {
		application, ok := calls.At(index)
		if !ok {
			t.Fatal("Project Calls At")
		}
		_, occurrence, ok := linked.Project().Applications().Call(application)
		if ok && occurrence == call {
			return application
		}
	}
	t.Fatalf("Call Application for Program Call %v absent", call)
	return linkproject.Application{}
}

func valueOriginIs(linked *Link, value linkboundary.Value, want keyspace.Term) bool {
	_, got, ok := linked.Boundary().Values().Origin(value)
	return ok && got == want
}

func methodCall(t testing.TB, p *program.Program) keyspace.Term {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok {
			t.Fatal("CallAt")
		}
		_, _, receiver, _, ok := calls.Get(call)
		if ok && receiver != 0 {
			return call
		}
	}
	t.Fatal("method Call absent")
	return 0
}

func globalCall(t testing.TB, p *program.Program, name string) keyspace.Term {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok {
			t.Fatal("CallAt")
		}
		_, callee, receiver, _, ok := calls.Get(call)
		if !ok || receiver != 0 {
			continue
		}
		_, cell, _, ok := p.Flow().Authored().Storage().Reads().Get(callee)
		if !ok {
			continue
		}
		cellKind, body, key, ok := p.Flow().Authored().Storage().Cells().Get(cell)
		literal, literalOK := p.Source().Keys().Exact(key)
		if ok && cellKind == flow.CellGlobal && body == 0 && literalOK && literal.Kind == keyspace.LiteralString && literal.String == name {
			return call
		}
	}
	t.Fatalf("Call to global %q absent", name)
	return 0
}
