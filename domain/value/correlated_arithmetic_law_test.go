package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/value/arithmetic/resultpolicy"
)

// arithmeticCandidateForLaw states one arithmetic occurrence under one sealed
// result policy. The policy is the whole difference between a closed and an
// open occurrence, so a law names it rather than a schema-wide flag.
func arithmeticCandidateForLaw(schema *Schema, op flowkind.BinaryOp, policy resultpolicy.Policy) BinaryArithmetic {
	return BinaryArithmetic{
		schema:  schema,
		key:     computationKey{module: identity.ContentID{1}, occurrence: identity.ContentID{2}},
		content: identity.ContentID{3},
		op:      op,
		policy:  policy,
	}
}

func TestApplyArithmeticUsesProgramSemanticsAndSealedResultAtoms(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 32}
	for _, literal := range []keyspace.LiteralValue{integer(10), integer(5), integer(0)} {
		schema.exactKeys[literal] = literal
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}) == 0 {
			t.Fatal("source literal atom")
		}
	}
	resultLiteral := integer(15)
	if schema.addAtom(atomRow{kind: atomComputedLiteral, runtime: runtimekind.Number, key: resultLiteral}) == 0 {
		t.Fatal("computed result atom")
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	left := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(10), hasKey: true})
	right := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(5), hasKey: true})
	add := arithmeticCandidateForLaw(schema, flowkind.BinaryAdd, resultpolicy.ClosedImage(integer(10), integer(15)))
	result, ok := schema.ApplyArithmetic(add, left, right)
	scalar, scalarOK := schema.ExactScalar(result)
	literal, literalOK := scalar.Literal()
	if !ok || !scalarOK || !literalOK || literal != resultLiteral {
		t.Fatalf("ApplyArithmetic(10,5,+) = %+v/%v scalar=%+v/%v literal=%+v/%v", result, ok, scalar, scalarOK, literal, literalOK)
	}
	if _, keyOK := (Atom{schema: schema, id: schema.atomForExactArithmetic(resultLiteral)}).ExactKey(); keyOK {
		t.Fatal("computed arithmetic result fabricated a Link key")
	}
	reused, reusedOK := schema.ApplyArithmetic(add, right, right)
	reusedScalar, reusedScalarOK := schema.ExactScalar(reused)
	reusedLiteral, reusedLiteralOK := reusedScalar.Literal()
	if !reusedOK || !reusedScalarOK || !reusedLiteralOK || reusedLiteral != integer(10) {
		t.Fatal("arithmetic result did not reuse an authored literal atom")
	}
	missing, missingOK := schema.ApplyArithmetic(arithmeticCandidateForLaw(schema, flowkind.BinaryMul, resultpolicy.ClosedImage(integer(25))), right, right)
	if missingOK || missing.IsTop() {
		t.Fatal("exact result without a sealed atom did not refuse closed")
	}
	if schema.atomForExactArithmetic(integer(25)) != 0 {
		t.Fatal("unsealed exact result was fabricated")
	}
	mixed, _ := schema.Alternatives(
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(10))},
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(5))},
	)
	mixedResult, mixedResultOK := schema.ApplyArithmetic(add, mixed, right)
	if !mixedResultOK || mixedResult.IsTop() || schema.ValueAtomCount(mixedResult) != 2 {
		t.Fatalf("finite mixed input = %#v/%v atoms=%d, want two concrete results", mixedResult, mixedResultOK, schema.ValueAtomCount(mixedResult))
	}
	mixedLiterals := make(map[int64]struct{}, schema.ValueAtomCount(mixedResult))
	for index := 0; index < schema.ValueAtomCount(mixedResult); index++ {
		atom, atomOK := schema.ValueAtomAt(mixedResult, index)
		singleton, singletonOK := schema.Singleton(atom)
		scalar, scalarOK := schema.ExactScalar(singleton)
		literal, literalOK := scalar.Literal()
		if !atomOK || !singletonOK || !scalarOK || !literalOK || literal.Kind != keyspace.LiteralInteger {
			t.Fatalf("mixed result atom[%d] = %#v/%v singleton=%#v/%v scalar=%#v/%v literal=%#v/%v", index, atom, atomOK, singleton, singletonOK, scalar, scalarOK, literal, literalOK)
		}
		mixedLiterals[literal.Integer] = struct{}{}
	}
	for _, want := range []int64{10, 15} {
		if _, found := mixedLiterals[want]; !found {
			t.Fatalf("mixed result missing %d: %v", want, mixedLiterals)
		}
	}
	strict, strictOK := schema.ApplyArithmetic(add, schema.Bottom(), right)
	if !strictOK || !schema.Equal(strict, schema.Bottom()) {
		t.Fatal("arithmetic over an unreachable operand invented a reachable result")
	}
	trap, trapOK := schema.ApplyArithmetic(arithmeticCandidateForLaw(schema, flowkind.BinaryIDiv, resultpolicy.ClosedImage()), left, wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(0), hasKey: true}))
	if !trapOK || !schema.Equal(trap, schema.Bottom()) {
		t.Fatal("integer division by zero kept a reachable alternative")
	}
	foreign := *schema
	if _, ok := foreign.ApplyArithmetic(add, left, right); ok {
		t.Fatal("foreign equal-content Value owner accepted operands")
	}
	if _, ok := schema.ApplyArithmetic(arithmeticCandidateForLaw(schema, flowkind.BinaryEqual, resultpolicy.OpenImage()), left, right); ok {
		t.Fatal("non-arithmetic operator accepted")
	}
}

func TestApplyArithmeticEnumeratesFiniteCartesianProduct(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 32}
	for _, literal := range []keyspace.LiteralValue{integer(1), integer(2), integer(10), integer(20)} {
		schema.exactKeys[literal] = literal
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}) == 0 {
			t.Fatal("source literal atom")
		}
	}
	for _, literal := range []keyspace.LiteralValue{integer(11), integer(12), integer(21), integer(22)} {
		if schema.addAtom(atomRow{kind: atomComputedLiteral, runtime: runtimekind.Number, key: literal}) == 0 {
			t.Fatal("computed result atom")
		}
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}

	left, leftOK := schema.Alternatives(
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(1))},
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(2))},
	)
	right, rightOK := schema.Alternatives(
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(10))},
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(20))},
	)
	if !leftOK || !rightOK {
		t.Fatal("finite arithmetic operands")
	}
	result, resultOK := schema.ApplyArithmetic(arithmeticCandidateForLaw(schema, flowkind.BinaryAdd,
		resultpolicy.ClosedImage(integer(11), integer(12), integer(21), integer(22))), left, right)
	if !resultOK || result.IsTop() || schema.ValueAtomCount(result) != 4 {
		t.Fatalf("finite 2x2 arithmetic = %#v/%v atoms=%d, want four concrete atoms", result, resultOK, schema.ValueAtomCount(result))
	}
	got := make(map[int64]struct{}, schema.ValueAtomCount(result))
	for index := 0; index < schema.ValueAtomCount(result); index++ {
		atom, atomOK := schema.ValueAtomAt(result, index)
		singleton, singletonOK := schema.Singleton(atom)
		scalar, scalarOK := schema.ExactScalar(singleton)
		literal, literalOK := scalar.Literal()
		if !atomOK || !singletonOK || !scalarOK || !literalOK || literal.Kind != keyspace.LiteralInteger {
			t.Fatalf("result atom[%d] = %#v/%v singleton=%#v/%v scalar=%#v/%v literal=%#v/%v", index, atom, atomOK, singleton, singletonOK, scalar, scalarOK, literal, literalOK)
		}
		got[literal.Integer] = struct{}{}
	}
	for _, want := range []int64{11, 12, 21, 22} {
		if _, found := got[want]; !found {
			t.Fatalf("finite 2x2 result missing %d: %v", want, got)
		}
	}
}

func TestOpenArithmeticPolicyUsesTheSealedNumericAbstraction(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 32}
	schema.exactKeys[integer(1)] = integer(1)
	for _, row := range []atomRow{
		{kind: atomLiteral, runtime: runtimekind.Number, key: integer(1), hasKey: true},
		{kind: atomPrimitive, runtime: runtimekind.Number},
		{kind: atomNaN, runtime: runtimekind.Number},
	} {
		if schema.addAtom(row) == 0 {
			t.Fatalf("seal atom %#v", row)
		}
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	limit := 1 << uint(runtimekind.Count-1)
	schema.forRuntimeKinds = make([]Value, limit)
	numberSet := runtimekind.Bit(runtimekind.Number)
	schema.forRuntimeKinds[int(numberSet)] = schema.canonical(schema.fullRows(func(id uint32) bool {
		return schema.atomKinds(id)&numberSet != 0
	}))
	one := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(1), hasKey: true})
	open := arithmeticCandidateForLaw(schema, flowkind.BinaryAdd, resultpolicy.OpenImage())

	first, firstOK := schema.ApplyArithmetic(open, one, one)
	if !firstOK || first.IsTop() || schema.RuntimeKinds(first) != numberSet {
		t.Fatalf("open 1+1 = %#v/%v kinds=%v, want sealed numeric abstraction", first, firstOK, schema.RuntimeKinds(first))
	}
	second, secondOK := schema.ApplyArithmetic(open, first, one)
	if !secondOK || second.IsTop() || !schema.Equal(first, second) {
		t.Fatalf("open numeric + 1 = %#v/%v, want stable numeric abstraction", second, secondOK)
	}
}

func TestClosedArithmeticPolicyCannotBorrowAnotherOccurrencesAtom(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 32}
	for _, literal := range []keyspace.LiteralValue{integer(1), integer(2)} {
		schema.exactKeys[literal] = literal
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}) == 0 {
			t.Fatal("literal atom")
		}
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	one := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(1), hasKey: true})
	result, ok := schema.ApplyArithmetic(arithmeticCandidateForLaw(schema, flowkind.BinaryAdd, resultpolicy.ClosedImage()), one, one)
	if ok || result.IsTop() {
		t.Fatal("closed occurrence borrowed a globally available exact result atom")
	}
}

func TestArithmeticValueOwnsTheCompleteReductionOutcome(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 32}
	for _, literal := range []keyspace.LiteralValue{integer(10), integer(5)} {
		schema.exactKeys[literal] = literal
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}) == 0 {
			t.Fatal("source literal atom")
		}
	}
	if schema.addAtom(atomRow{kind: atomComputedLiteral, runtime: runtimekind.Number, key: integer(15)}) == 0 {
		t.Fatal("computed result atom")
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	candidate := arithmeticCandidateForLaw(schema, flowkind.BinaryAdd, resultpolicy.ClosedImage(integer(15)))
	left := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(10), hasKey: true})
	right := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(5), hasKey: true})

	result, outcome := ArithmeticValue(candidate, left, right)
	if outcome != structure.Concrete {
		t.Fatalf("ArithmeticValue outcome = %v, want Concrete", outcome)
	}
	scalar, scalarOK := schema.ExactScalar(result)
	literal, literalOK := scalar.Literal()
	if !scalarOK || !literalOK || literal != integer(15) {
		t.Fatalf("ArithmeticValue result = %+v/%v literal=%+v/%v, want integer 15", scalar, scalarOK, literal, literalOK)
	}

	foreign := *schema
	if _, outcome := ArithmeticValue(candidate, Value{schema: &foreign, top: true}, right); outcome != structure.Refuse {
		t.Fatalf("foreign operand outcome = %v, want Refuse", outcome)
	}
}
