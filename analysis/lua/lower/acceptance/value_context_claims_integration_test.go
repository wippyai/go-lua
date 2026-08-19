package acceptance_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
)

func TestNonNilValueClaimLowersBeforeMemberAccess(t *testing.T) {
	p := parseBindLower(t, `
local function member(value)
  return (value!).name
end
`)
	flow := p.Flow()
	claims := flow.Authored().Claims()
	if got := claims.Count(); got != 1 {
		t.Fatalf("ValueClaimCount = %d, want 1", got)
	}
	claim, _ := claims.At(0)
	_, operand, claimKind, ok := claims.Get(claim)
	target, targetOK := p.Static().Operands().Claims().Target(claim)
	if !ok || claimKind != kind.ValueClaimNonNil || targetOK || target != 0 {
		t.Fatalf("ValueClaim = operand %v target %v/%v kind %v ok %v, want targetless NonNil", operand, target, targetOK, claimKind, ok)
	}
	if _, _, _, ok := flow.Authored().Storage().Reads().Get(operand); !ok {
		t.Fatalf("non-nil operand %v is not the member base Read", operand)
	}

	function, _ := flow.Authored().Functions().At(0)
	_, body, _, ok := flow.Authored().Functions().Get(function)
	if !ok {
		t.Fatal("missing member Function")
	}
	returned, ok := flow.Authored().Control().Returns().At(0)
	if !ok {
		t.Fatal("missing member Return")
	}
	owner, values, ok := flow.Authored().Control().Returns().Get(returned)
	if !ok || owner != body {
		t.Fatalf("member Return = owner %v ok %v, want body %v", owner, ok, body)
	}
	result := valueAt(t, p, values, 0)
	_, lens, _, ok := flow.Authored().Storage().Reads().Get(result)
	if !ok {
		t.Fatalf("member result %v is not a Read", result)
	}
	_, base, _, _, ok := flow.Authored().Access().Exact().Get(lens)
	if !ok || base != claim {
		t.Fatalf("member Lens base = %v (ok %v), want ValueClaim %v", base, ok, claim)
	}
}

func TestNonNilValueClaimMakesCallResultScalar(t *testing.T) {
	p := parseBindLower(t, `
local function source() return 1 end
local function scalar() return source()! end
`)
	flow := p.Flow()
	claims := flow.Authored().Claims()
	if got := claims.Count(); got != 1 {
		t.Fatalf("ValueClaimCount = %d, want 1", got)
	}
	claim, _ := claims.At(0)
	_, operand, claimKind, ok := claims.Get(claim)
	target, targetOK := p.Static().Operands().Claims().Target(claim)
	if !ok || claimKind != kind.ValueClaimNonNil || targetOK || target != 0 {
		t.Fatalf("ValueClaim = operand %v target %v/%v kind %v ok %v, want targetless NonNil", operand, target, targetOK, claimKind, ok)
	}
	if _, _, _, _, ok := flow.Authored().Calls().Get(operand); !ok {
		t.Fatalf("non-nil operand %v is not a Call", operand)
	}

	functions := flow.Authored().Functions()
	returns := flow.Authored().Control().Returns()
	valuesView := flow.Authored().Values()
	for index := 0; index < functions.Count(); index++ {
		function, _ := functions.At(index)
		_, body, _, functionOK := functions.Get(function)
		if !functionOK {
			t.Fatalf("FunctionAt(%d) is not a Function", index)
		}
		for returnIndex := 0; returnIndex < returns.Count(); returnIndex++ {
			returned, returnOK := returns.At(returnIndex)
			if !returnOK {
				continue
			}
			owner, values, isReturn := returns.Get(returned)
			if !isReturn || owner != body || valuesTail(t, p, values) != 0 {
				continue
			}
			if fixed, _ := valuesView.Len(values); fixed == 1 && valueAt(t, p, values, 0) == claim {
				return
			}
		}
	}
	t.Fatalf("non-nil Call result %v was not returned as one fixed scalar", claim)
}

func TestNestedValueClaimsPreserveExactOrder(t *testing.T) {
	p := parseBindLower(t, `
type Value = number
local value = (1 :: Value)!
`)
	flow := p.Flow()
	claims := flow.Authored().Claims()
	if got := claims.Count(); got != 2 {
		t.Fatalf("ValueClaimCount = %d, want 2", got)
	}
	inner, _ := claims.At(0)
	outer, _ := claims.At(1)
	_, _, innerKind, innerOK := claims.Get(inner)
	_, outerOperand, outerKind, outerOK := claims.Get(outer)
	innerTarget, innerTargetOK := p.Static().Operands().Claims().Target(inner)
	outerTarget, outerTargetOK := p.Static().Operands().Claims().Target(outer)
	if !innerOK || innerKind != kind.ValueClaimTypeColonColon || !innerTargetOK || innerTarget == 0 {
		t.Fatalf("inner ValueClaim = target %v/%v kind %v ok %v, want typed :: claim", innerTarget, innerTargetOK, innerKind, innerOK)
	}
	if !outerOK || outerKind != kind.ValueClaimNonNil || outerTargetOK || outerTarget != 0 || outerOperand != inner {
		t.Fatalf("outer ValueClaim = operand %v target %v/%v kind %v ok %v, want NonNil(inner)", outerOperand, outerTarget, outerTargetOK, outerKind, outerOK)
	}
	if next, ok := flow.Ports().Finish(inner); !ok || next != outer {
		t.Fatalf("inner ValueClaim successor = %v/%v, want outer %v", next, ok, outer)
	}
}

func TestCastExpressionsLowerToTypedValueClaims(t *testing.T) {
	p := parseBindLower(t, `
type Value = number
local value = 1
local asValue = value as Value
local colonValue = value :: Value
local sameValue = value as typeof(value)
`)
	claims := p.Flow().Authored().Claims()
	if got := claims.Count(); got != 3 {
		t.Fatalf("ValueClaimCount = %d, want 3", got)
	}
	kinds := []kind.ValueClaimKind{kind.ValueClaimTypeAs, kind.ValueClaimTypeColonColon, kind.ValueClaimTypeAs}
	for index, wantKind := range kinds {
		claim, ok := claims.At(index)
		if !ok {
			t.Fatalf("missing ValueClaim %d", index)
		}
		owner, operand, claimKind, ok := claims.Get(claim)
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !ok || owner == 0 || operand == 0 || !targetOK || target == 0 || claimKind != wantKind {
			t.Fatalf("ValueClaim %d = owner %v operand %v target %v/%v kind %v ok %v", index, owner, operand, target, targetOK, claimKind, ok)
		}
		if index < 2 {
			state, declaration, _, refOK := p.Static().References().Get(target)
			if !refOK || state != staticrefs.Declaration || declaration == 0 {
				t.Fatalf("ValueClaim %d target = state %v declaration %v ok %v", index, state, declaration, refOK)
			}
			continue
		}
		scope, _, typeOfOK := p.Static().Operators().TypeOfs().Get(target)
		if !typeOfOK || scope != claim {
			t.Fatalf("cast typeof target scope = %v/%v, want %v/true", scope, typeOfOK, claim)
		}
	}
}
