package factor_test

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// An effect occurrence substitutes the whole ValuesVar vocabulary of its
// target operation, position by position: the Target ABI requires exactly one
// argument per declared var. Only the position that is the target's input tail
// names something the effect reads out of the caller's Pack; the remaining
// positions substitute outcome-scoped vars, for which Pack seals no input
// selector because they are not inputs.
//
// These two laws are jointly satisfiable only when the input fence is applied
// at the input-tail position alone. The operations below are the shape the
// standard library produces for pcall, pairs, and tostring: a self effect on
// an operation whose declared vars include an outcome tail.
func outcomeScopedValuesVarSpec(openInput bool) target.Spec {
	any := portableAnyType()
	closed := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	// Var 0 is the input tail when the input is open, otherwise the outcome
	// tail; the second var is always an outcome tail.
	input := vocabulary.ValuesSpec{Fixed: []schematype.Type{any}, Tail: vocabulary.ValuesClosed}
	normal := vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: any}
	vars := uint32(1)
	if openInput {
		input = vocabulary.ValuesSpec{Fixed: []schematype.Type{any}, Tail: vocabulary.ValuesVariable, Var: 0, TailType: any}
		normal = vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 1, TailType: any}
		vars = 2
	}
	self := vocabulary.EffectSpec{
		Target:    1,
		ValueArgs: []vocabulary.ValueFormal{0},
	}
	for index := uint32(0); index < vars; index++ {
		self.ValuesArgs = append(self.ValuesArgs, vocabulary.ValuesVar(index))
	}
	owner := vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"sink"}}},
		ValuesVars: vars,
		Input:      input,
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: kind.OutcomeNormal, Values: normal},
			{Kind: kind.OutcomeThrow, Values: closed},
		},
		Effects: vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{self}, Tail: vocabulary.RowClosed},
	}
	return target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{owner}}
}

// TestSelfEffectBindsOverOutcomeScopedValuesVars proves that an operation
// whose declared ValuesVars include an outcome tail still issues its own
// effect's formal atom and beta binding vector, with an open input tail and
// without one.
func TestSelfEffectBindsOverOutcomeScopedValuesVars(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		openInput bool
	}{
		{"closed input", false},
		{"open input tail", true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newEffectFactorFixture(t, outcomeScopedValuesVarSpec(testCase.openInput), "local function sink(value) return value end\nsink(1)")
			if _, ok := fixture.factor.FormalCallEffectAtom(fixture.mountedCall, fixture.owner, 0); !ok {
				t.Fatal("self effect formal atom over outcome-scoped ValuesVars")
			}
			bindings, ok := fixture.factor.SelectedCallEffectBindings(fixture.root, fixture.mountedCall, fixture.owner)
			if !ok || len(bindings) != 1 {
				t.Fatalf("selected call effect bindings ok=%t count=%d", ok, len(bindings))
			}
			if _, ok := bindings[0].Atom(); !ok {
				t.Fatal("selected call effect binding atom")
			}
		})
	}
}
