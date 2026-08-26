package acceptance_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TestSourceUnaryCallOperandsAreOnceAndCausal is a source-only law for every
// executable unary spelling.  A call is deliberately used as each operand so
// a duplicate operand schedule would mint a second, observable Call identity.
func TestSourceUnaryCallOperandsAreOnceAndCausal(t *testing.T) {
	tests := []struct {
		name string
		op   kind.UnaryOp
	}{
		{name: "neg", op: kind.UnaryNeg},
		{name: "not", op: kind.UnaryNot},
		{name: "len", op: kind.UnaryLen},
		{name: "bit-not", op: kind.UnaryBitNot},
	}
	p := parseBindLower(t, `
local function neg() return -negOperand() end
local function logical() return not notOperand() end
local function length() return #lengthOperand() end
local function bitwise() return ~bitwiseOperand() end
`)
	flow := p.Flow()
	unaries := flow.Authored().Operators().Unaries()
	calls := flow.Authored().Calls()
	if got := unaries.Count(); got != len(tests) {
		t.Fatalf("UnaryCount = %d, want %d", got, len(tests))
	}
	if got := calls.Count(); got != len(tests) {
		t.Fatalf("CallCount = %d, want one once-evaluated operand per unary", got)
	}

	seenCalls := make(map[keyspace.Term]struct{}, len(tests))
	for index, want := range tests {
		unary, ok := unaries.At(index)
		if !ok {
			t.Fatalf("missing Unary %d", index)
		}
		owner, op, operand, ok := unaries.Get(unary)
		if !ok || owner == 0 || op != want.op || operand == 0 {
			t.Fatalf("Unary %q = owner %v op %v operand %v ok %v", want.name, owner, op, operand, ok)
		}
		call, ok := calls.At(index)
		if !ok || call != operand {
			t.Fatalf("Unary %q operand = %v; CallAt(%d) = %v/%v, want one authored Call", want.name, operand, index, call, ok)
		}
		if _, duplicate := seenCalls[call]; duplicate {
			t.Fatalf("Unary %q reused Call %v from another authored operand", want.name, call)
		}
		seenCalls[call] = struct{}{}

		operandEntry, ok := flow.Ports().Entry(unary)
		if !ok || operandEntry == 0 {
			t.Fatalf("Unary %q has no exact operand entry", want.name)
		}
		callEntry, ok := flow.Ports().Entry(call)
		if !ok || operandEntry != callEntry {
			t.Fatalf("Unary %q operand entry = %v; Call entry = %v/%v", want.name, operandEntry, callEntry, ok)
		}
		if next := unconditionalSuccessor(t, p, call); next != unary {
			t.Fatalf("Unary %q Call successor = %v, want Unary %v", want.name, next, unary)
		}

		returned := returnOwnedBy(t, p, owner)
		_, values, ok := flow.Authored().Control().Returns().Get(returned)
		if !ok {
			t.Fatalf("Unary %q Return %v has no Values", want.name, returned)
		}
		if result := valueAt(t, p, values, 0); result != unary {
			t.Fatalf("Unary %q Return value = %v, want Unary %v", want.name, result, unary)
		}
		if next := unconditionalSuccessor(t, p, unary); next != values {
			t.Fatalf("Unary %q successor = %v, want parent Return Values %v", want.name, next, values)
		}
	}
}

// TestSourceValueClaimCallOperandsAreOnceScalarAndCausal proves all three
// claim spellings share one direct, erased wrapper protocol. A raw Call is a
// deliberately open producer in list position, so the fixed parent Values
// relation also proves each claim scalar-adjusts its operand.
func TestSourceValueClaimCallOperandsAreOnceScalarAndCausal(t *testing.T) {
	p := parseBindLower(t, `
type Number = number
local function asClaim() return asOperand() as Number end
local function colonClaim() return colonOperand() :: Number end
local function nonNilClaim() return nonNilOperand()! end
`)
	flow := p.Flow()
	calls := flow.Authored().Calls()
	claims := flow.Authored().Claims()
	if calls.Count() != 3 || claims.Count() != 3 {
		t.Fatalf("Calls/ValueClaims = %d/%d, want 3/3", calls.Count(), claims.Count())
	}
	wantKinds := []kind.ValueClaimKind{
		kind.ValueClaimTypeAs,
		kind.ValueClaimTypeColonColon,
		kind.ValueClaimNonNil,
	}
	for index, wantKind := range wantKinds {
		call, _ := calls.At(index)
		claim, _ := claims.At(index)
		owner, operand, claimKind, ok := claims.Get(claim)
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !ok || owner == 0 || operand != call || claimKind != wantKind {
			t.Fatalf("ValueClaim %d = owner %v operand %v target %v/%v kind %v ok %v", index, owner, operand, target, targetOK, claimKind, ok)
		}
		if wantKind == kind.ValueClaimNonNil {
			if targetOK || target != 0 {
				t.Fatalf("NonNil ValueClaim target = %v, want absent", target)
			}
		} else if !targetOK || target == 0 {
			t.Fatalf("typed ValueClaim %d lacks exact static target", index)
		}
		entry, ok := flow.Ports().Entry(claim)
		callEntry, callOK := flow.Ports().Entry(call)
		if !ok || !callOK || entry == 0 || entry != callEntry {
			t.Fatalf("ValueClaim %d operand entry = %v/%v; Call entry = %v/%v", index, entry, ok, callEntry, callOK)
		}
		if next := unconditionalSuccessor(t, p, call); next != claim {
			t.Fatalf("Call %d successor = %v, want ValueClaim %v", index, next, claim)
		}
		returned := returnOwnedBy(t, p, owner)
		_, values, ok := flow.Authored().Control().Returns().Get(returned)
		if !ok || valueAt(t, p, values, 0) != claim || valuesTail(t, p, values) != 0 {
			t.Fatalf("ValueClaim %d Return Values = %v/%v tail %v, want fixed claim %v", index, values, ok, valuesTail(t, p, values), claim)
		}
		if next := unconditionalSuccessor(t, p, claim); next != values {
			t.Fatalf("ValueClaim %d successor = %v, want parent Return Values %v", index, next, values)
		}
	}
}

// TestSourceValueClaimsKeepFalseAndNilOnTheNormalPath makes the absence of a
// runtime proof/guard observable: false and definitely nil both retain their
// direct normal continuation. Nil may be diagnosed later, but lowering does
// not prune it or fabricate a Throw/Outcome at the claim.
func TestSourceValueClaimsKeepFalseAndNilOnTheNormalPath(t *testing.T) {
	p := parseBindLower(t, `
local function claims()
  return false!, nil!, false as boolean, nil :: nil
end
`)
	flow := p.Flow()
	claims := flow.Authored().Claims()
	if claims.Count() != 4 {
		t.Fatalf("ValueClaimCount = %d, want 4", claims.Count())
	}
	for index, want := range []struct {
		kind kind.ValueClaimKind
		nil  bool
	}{
		{kind: kind.ValueClaimNonNil},
		{kind: kind.ValueClaimNonNil, nil: true},
		{kind: kind.ValueClaimTypeAs},
		{kind: kind.ValueClaimTypeColonColon, nil: true},
	} {
		claim, _ := claims.At(index)
		owner, operand, claimKind, ok := claims.Get(claim)
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !ok || owner == 0 || claimKind != want.kind {
			t.Fatalf("ValueClaim %d = owner %v operand %v target %v/%v kind %v ok %v", index, owner, operand, target, targetOK, claimKind, ok)
		}
		if want.kind == kind.ValueClaimNonNil && (targetOK || target != 0) {
			t.Fatalf("NonNil ValueClaim %d has target %v", index, target)
		}
		if want.kind != kind.ValueClaimNonNil && (!targetOK || target == 0) {
			t.Fatalf("typed ValueClaim %d lacks target", index)
		}
		if want.nil {
			if literal, _, literalOK := p.Source().Literals().Nils().At(int(keyspace.TermOrdinal(operand)) - 1); !literalOK || literal != operand {
				t.Fatalf("ValueClaim %d operand %v is not Nil", index, operand)
			}
		} else if literal, _, value, literalOK := p.Source().Literals().Bools().At(int(keyspace.TermOrdinal(operand)) - 1); !literalOK || literal != operand || value {
			t.Fatalf("ValueClaim %d operand %v is not false Bool", index, operand)
		}
		next := unconditionalSuccessor(t, p, claim)
		if _, _, _, isOutcome := flow.Outcomes().Get(next); isOutcome {
			t.Fatalf("ValueClaim %d normal successor %v is an Outcome", index, next)
		}
	}
}

// TestSourceValueClaimsScalarAdjustVarargOperands makes the second open
// producer explicit. Even though ... can remain a final open Values tail,
// every claim consumes exactly one scalar and the enclosing return remains a
// fixed three-value pack.
func TestSourceValueClaimsScalarAdjustVarargOperands(t *testing.T) {
	p := parseBindLower(t, `
local function claims(...)
  return (...) as number, (...) :: number, (...)!
end
`)
	flow := p.Flow()
	varargs := flow.Authored().Storage().Varargs()
	claims := flow.Authored().Claims()
	if varargs.Count() != 3 || claims.Count() != 3 {
		t.Fatalf("Varargs/ValueClaims = %d/%d, want 3/3", varargs.Count(), claims.Count())
	}
	returned, ok := flow.Authored().Control().Returns().At(0)
	if !ok {
		t.Fatal("missing return")
	}
	_, values, ok := flow.Authored().Control().Returns().Get(returned)
	if !ok || valuesTail(t, p, values) != 0 {
		t.Fatalf("claim return Values = %v/%v tail %v, want fixed", values, ok, valuesTail(t, p, values))
	}
	if fixed, ok := flow.Authored().Values().Len(values); !ok || fixed != 3 {
		t.Fatalf("claim return fixed length = %d/%v, want 3", fixed, ok)
	}
	for index, wantKind := range []kind.ValueClaimKind{
		kind.ValueClaimTypeAs,
		kind.ValueClaimTypeColonColon,
		kind.ValueClaimNonNil,
	} {
		claim, _ := claims.At(index)
		_, operand, claimKind, ok := claims.Get(claim)
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !ok || claimKind != wantKind {
			t.Fatalf("ValueClaim %d = operand %v target %v/%v kind %v ok %v", index, operand, target, targetOK, claimKind, ok)
		}
		if _, _, varargOK := varargs.Get(operand); !varargOK {
			t.Fatalf("ValueClaim %d operand %v is not Vararg", index, operand)
		}
		if wantKind == kind.ValueClaimNonNil {
			if targetOK || target != 0 {
				t.Fatalf("NonNil ValueClaim %d target = %v, want absent", index, target)
			}
		} else if !targetOK || target == 0 {
			t.Fatalf("typed ValueClaim %d lacks target", index)
		}
		if result := valueAt(t, p, values, index); result != claim {
			t.Fatalf("return fixed value %d = %v, want ValueClaim %v", index, result, claim)
		}
		if next := unconditionalSuccessor(t, p, operand); next != claim {
			t.Fatalf("Vararg %d successor = %v, want ValueClaim %v", index, next, claim)
		}
	}
}

// TestSourceValueClaimSpansPreserveAuthoredExpression proves the claims retain
// the exact source extent of each spelling. In particular, postfix ! owns its
// authored token rather than inheriting only the operand's end position.
func TestSourceValueClaimSpansPreserveAuthoredExpression(t *testing.T) {
	p := parseBindLower(t, `return false!, false as boolean, false :: boolean`)
	wants := []source.Span{
		{File: "fixture.lua", StartLine: 1, StartCol: 8, EndLine: 1, EndCol: 13},
		{File: "fixture.lua", StartLine: 1, StartCol: 16, EndLine: 1, EndCol: 31},
		{File: "fixture.lua", StartLine: 1, StartCol: 34, EndLine: 1, EndCol: 49},
	}
	claims := p.Flow().Authored().Claims()
	if got := claims.Count(); got != len(wants) {
		t.Fatalf("ValueClaimCount = %d, want %d", got, len(wants))
	}
	for index, want := range wants {
		claim, ok := claims.At(index)
		if !ok {
			t.Fatalf("missing ValueClaim %d", index)
		}
		if span, ok := p.Source().Identity().Span(claim); !ok || span != want {
			t.Fatalf("Span(ValueClaim %d) = %#v/%v, want %#v", index, span, ok, want)
		}
	}
}

func returnOwnedBy(t *testing.T, p *program.Program, owner keyspace.Term) keyspace.Term {
	t.Helper()
	returns := p.Flow().Authored().Control().Returns()
	var found keyspace.Term
	for index := 0; index < returns.Count(); index++ {
		returned, ok := returns.At(index)
		if !ok {
			continue
		}
		returnOwner, _, ok := returns.Get(returned)
		if !ok || returnOwner != owner {
			continue
		}
		if found != 0 {
			t.Fatalf("Body %v has multiple Returns %v and %v", owner, found, returned)
		}
		found = returned
	}
	if found == 0 {
		t.Fatalf("Body %v has no Return", owner)
	}
	return found
}
