package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestExternalCallOperandEvaluatorUsesSealedReachabilityFallback(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	arena := NewArena(reg)
	sym := symbol.ID(71)
	slot := statekey.SymbolValue(sym)
	term := arena.bindEnvironmentSymbol(sym)
	body := &relationProgramBody{relation: Relation{arena: arena}, productDomain: domain}
	const callPoint, sourcePoint, fallbackPoint cfg.Point = 1, 2, 3
	access, err := state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs: []state.TransferInputAccess{
			{},
			{Values: []statekey.Value{slot}, Reachable: true},
			{Values: []statekey.Value{slot}},
		},
		ValueCarry: 0, LaneCarry: 0, DiagnosticCarry: 0, ReachableCarry: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := callpayload.PrepareExternalCallInputProgram(domain, access, []cfg.Point{callPoint, sourcePoint, fallbackPoint}, 0, func(key statekey.Value) (statekey.Value, bool) { return key, true })
	if err != nil {
		t.Fatal(err)
	}
	preferred := typevalue.LiteralString(reg, "preferred")
	fallback := typevalue.LiteralString(reg, "fallback")
	inputs := []state.State{
		state.Reachable(state.State{}),
		domain.Lattice().Bottom(),
		state.Reachable(state.State{}).WriteValue(reg, slot, fallback),
	}
	frame, err := callpayload.BindConcreteExternalCallInputFrame(&program, inputs, make([]callpayload.DiagnosticOutput, len(inputs)))
	if err != nil {
		t.Fatal(err)
	}
	terms := callOutcomeOperandTerms{arguments: []ValueTerm{term}}
	termAccess := []valueAccessTerm{
		{term: term, point: sourcePoint, hasPoint: true},
		{term: term, point: fallbackPoint, hasPoint: true, fallback: true},
	}
	got, err := evaluateExternalCallOperands(body, terms, termAccess, frame, concreteExternalCallDynamicQuery(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Arguments) != 1 || !product.Equal(reg, got.Arguments[0].Value, fallback) {
		t.Fatalf("unreachable preferred operand = %#v, want fallback", got.Arguments)
	}

	inputs[1] = state.Reachable(state.State{}).WriteValue(reg, slot, preferred)
	frame, err = callpayload.BindConcreteExternalCallInputFrame(&program, inputs, make([]callpayload.DiagnosticOutput, len(inputs)))
	if err != nil {
		t.Fatal(err)
	}
	got, err = evaluateExternalCallOperands(body, terms, termAccess, frame, concreteExternalCallDynamicQuery(body))
	if err != nil {
		t.Fatal(err)
	}
	if !product.Equal(reg, got.Arguments[0].Value, preferred) {
		t.Fatalf("reachable preferred operand = %#v, want preferred", got.Arguments[0].Value)
	}

	// Bind the same dense carrier directly, without State. The evaluator is
	// identical because its only inputs are sealed Values/factor wires.
	direct, err := program.BindFrame([]callpayload.ExternalCallInputWireOperands{
		{},
		{Values: []product.Value{preferred}, Reachable: true},
		{Values: []product.Value{fallback}},
	})
	if err != nil {
		t.Fatal(err)
	}
	directGot, err := evaluateExternalCallOperands(body, terms, termAccess, direct, concreteExternalCallDynamicQuery(body))
	if err != nil || !product.Equal(reg, directGot.Arguments[0].Value, got.Arguments[0].Value) {
		t.Fatalf("direct carrier operand = %#v err=%v, concrete adapter = %#v", directGot.Arguments, err, got.Arguments)
	}
}

func TestExternalCallOperandEvaluatorDynamicReadUsesRegisteredFactors(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	arena := NewArena(reg)
	const point cfg.Point = 7
	table := product.Top()
	keyValue := typevalue.LiteralString(reg, "member")
	term := arena.DynamicReadTableValueAt(point, arena.Constant(table), 0, arena.Constant(keyValue))
	body := &relationProgramBody{keys: keys, relation: Relation{arena: arena}, productDomain: domain}
	access, err := externalCallTransferAccess(body, boundaryPrefixStep{
		kind:   boundaryPrefixExternalCall,
		access: []valueAccessTerm{{term: term, point: point, hasPoint: true}},
	}, []cfg.Point{point}, 1, 0, callpayload.CallOutcomeCapability{})
	if err != nil {
		t.Fatal(err)
	}
	program, err := callpayload.PrepareExternalCallInputProgram(domain, access, []cfg.Point{point}, 0, func(key statekey.Value) (statekey.Value, bool) { return key, true })
	if err != nil {
		t.Fatal(err)
	}
	input := state.Reachable(domain.Lattice().Bottom())
	frame, err := callpayload.BindConcreteExternalCallInputFrame(&program, []state.State{input}, []callpayload.DiagnosticOutput{{}})
	if err != nil {
		t.Fatal(err)
	}
	// Repeating the same dynamic term exercises the evaluator-frame projection
	// more than once. Both reads must use the same bound factor projection and
	// remain identical to the canonical concrete binder.
	operandTerms := callOutcomeOperandTerms{arguments: []ValueTerm{term, term}}
	got, err := evaluateExternalCallOperands(body, operandTerms, []valueAccessTerm{{term: term, point: point, hasPoint: true}}, frame, concreteExternalCallDynamicQuery(body))
	if err != nil {
		t.Fatal(err)
	}
	query, err := dynamicValueQuery(body, keys, arena.values[term], []product.Value{table, keyValue})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := domain.ProjectDynamicReadEvidence(query, input)
	if err != nil {
		t.Fatal(err)
	}
	want, exact := enginesourcevalue.ResolveDynamicRead(query, evidence)
	if !exact || len(got.Arguments) != 2 || !product.Equal(reg, got.Arguments[0].Value, want) ||
		!product.Equal(reg, got.Arguments[1].Value, want) {
		t.Fatalf("factor-native dynamic operand = %#v, want %#v exact=%t", got.Arguments, want, exact)
	}

	primary, layout, ok := frame.Primary()
	if !ok {
		t.Fatal("missing primary frame")
	}
	direct, err := program.BindFrame([]callpayload.ExternalCallInputWireOperands{{Factors: primary.Factors()}})
	if err != nil {
		t.Fatal(err)
	}
	directGot, err := evaluateExternalCallOperands(body, operandTerms, []valueAccessTerm{{term: term, point: point, hasPoint: true}}, direct, concreteExternalCallDynamicQuery(body))
	if err != nil || len(directGot.Arguments) != 2 ||
		!product.Equal(reg, directGot.Arguments[0].Value, want) ||
		!product.Equal(reg, directGot.Arguments[1].Value, want) {
		t.Fatalf("direct factor carrier dynamic operand = %#v err=%v, want %#v (layout=%#v)", directGot.Arguments, err, want, layout)
	}
}
