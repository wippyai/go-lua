package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
)

func TestFlowCandidatesClassifyAuthoredOperatorFamilies(t *testing.T) {
	p := parseBindLower(t, "\nlocal function candidates(a, b, ...)\n  local unaryNeg = -a\n  local unaryNot = not a\n  local unaryLen = #a\n  local unaryBit = ~a\n  local add = a + b\n  local sub = a - b\n  local mul = a * b\n  local div = a / b\n  local idiv = a // b\n  local mod = a % b\n  local pow = a ^ b\n  local concat = a .. b\n  local band = a & b\n  local bor = a | b\n  local bxor = a ~ b\n  local shl = a << b\n  local shr = a >> b\n  local equal = a == b\n  local notEqual = a ~= b\n  local less = a < b\n  local lessEqual = a <= b\n  local greater = a > b\n  local greaterEqual = a >= b\n  local got = a[b]\n  a[b] = b\n  a:b(b, ...)\n  return a(b, ...)\nend\n")
	candidates := p.Flow().Candidates()
	unary := candidates.Unary()
	binary := candidates.Binary()
	access := candidates.Access()
	if unary.NumericCount() != 2 || unary.LengthCount() != 1 {
		t.Fatalf("Unary candidates numeric/length = %d/%d, want 2/1", unary.NumericCount(), unary.LengthCount())
	}
	if binary.ArithmeticCount() != 7 || binary.BitwiseCount() != 5 || binary.ConcatCount() != 1 || binary.EqualityCount() != 2 || binary.OrderCount() != 4 {
		t.Fatalf("Binary candidates = arithmetic %d bitwise %d concat %d equality %d order %d", binary.ArithmeticCount(), binary.BitwiseCount(), binary.ConcatCount(), binary.EqualityCount(), binary.OrderCount())
	}
	if access.GetCount() != 1 || access.SetCount() != 1 {
		t.Fatalf("Access candidates get/set = %d/%d, want 1/1", access.GetCount(), access.SetCount())
	}
	neg, _ := unary.NumericAt(0)
	_, op, _, negOK := p.Flow().Authored().Operators().Unaries().Get(neg)
	if !negOK || op != kind.UnaryNeg {
		t.Fatalf("first numeric candidate Unary op = %v/%v, want neg", op, negOK)
	}
	add, _ := binary.ArithmeticAt(0)
	_, addOp, _, _, addOK := p.Flow().Authored().Operators().Binaries().Get(add)
	if !addOK || addOp != kind.BinaryAdd {
		t.Fatalf("first arithmetic candidate op = %v/%v, want add", addOp, addOK)
	}
}

func TestFlowCandidatesDoNotClassifyStaticAuthoredRows(t *testing.T) {
	p := parseBindLower(t, "type Snapshot = typeof(function(a, b) return -a, #a, a + b, a[b], a(b) end)")
	candidates := p.Flow().Candidates()
	if candidates.Unary().NumericCount() != 0 || candidates.Binary().ArithmeticCount() != 0 || candidates.Access().GetCount() != 0 {
		t.Fatalf("static rows entered runtime candidates: unary=%d binary=%d access=%d", candidates.Unary().NumericCount(), candidates.Binary().ArithmeticCount(), candidates.Access().GetCount())
	}
	for index := 0; index < p.Flow().Authored().Operators().Unaries().Count(); index++ {
		term, _ := p.Flow().Authored().Operators().Unaries().At(index)
		if !p.Flow().Containment().Static(term) {
			t.Fatalf("static Unary %v escaped static containment", term)
		}
	}
	for index := 0; index < p.Flow().Authored().Operators().Binaries().Count(); index++ {
		term, _ := p.Flow().Authored().Operators().Binaries().At(index)
		if !p.Flow().Containment().Static(term) {
			t.Fatalf("static Binary %v escaped static containment", term)
		}
	}
	for index := 0; index < p.Flow().Authored().Calls().Count(); index++ {
		term, _ := p.Flow().Authored().Calls().At(index)
		if !p.Flow().Containment().Static(term) {
			t.Fatalf("static Call %v escaped static containment", term)
		}
	}
}
