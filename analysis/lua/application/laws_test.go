package application

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
)

func TestUnaryLawsAreClosedAndExact(t *testing.T) {
	cases := []struct {
		op   kind.UnaryOp
		want UnaryLaw
	}{
		{kind.UnaryNeg, UnaryLaw{Primitive: PrimitiveNumber, Meta: MetaUnm, MissingFault: FaultArithmetic, Handler: HandlerOrdinaryCall}},
		{kind.UnaryNot, UnaryLaw{Primitive: PrimitiveTruth}},
		{kind.UnaryLen, UnaryLaw{Primitive: PrimitiveLength, Meta: MetaLen, MissingFault: FaultLength, Handler: HandlerOrdinaryCall}},
		{kind.UnaryBitNot, UnaryLaw{Primitive: PrimitiveInteger, Meta: MetaBNot, MissingFault: FaultBitwise, Handler: HandlerOrdinaryCall}},
	}

	for _, test := range cases {
		got, ok := Unary(test.op)
		if !ok || got != test.want {
			t.Fatalf("Unary(%d) = %#v/%v, want %#v/true", test.op, got, ok, test.want)
		}
		assertUnaryLaw(t, got)
	}
	for op := kind.UnaryNeg; op <= kind.UnaryBitNot; op++ {
		if _, ok := Unary(op); !ok {
			t.Fatalf("canonical unary op %d has no law", op)
		}
	}
	if got, ok := Unary(kind.UnaryOp(255)); ok || got != (UnaryLaw{}) {
		t.Fatalf("invalid unary law = %#v/%v, want zero/false", got, ok)
	}
}

func TestBinaryLawsAreClosedAndExact(t *testing.T) {
	cases := []struct {
		op   kind.BinaryOp
		want BinaryLaw
	}{
		{kind.BinaryAdd, BinaryLaw{Primitive: PrimitiveNumber, Meta: MetaAdd, MissingFault: FaultArithmetic, Handler: HandlerOrdinaryCall}},
		{kind.BinarySub, BinaryLaw{Primitive: PrimitiveNumber, Meta: MetaSub, MissingFault: FaultArithmetic, Handler: HandlerOrdinaryCall}},
		{kind.BinaryMul, BinaryLaw{Primitive: PrimitiveNumber, Meta: MetaMul, MissingFault: FaultArithmetic, Handler: HandlerOrdinaryCall}},
		{kind.BinaryDiv, BinaryLaw{Primitive: PrimitiveNumber, Meta: MetaDiv, MissingFault: FaultArithmetic, Handler: HandlerOrdinaryCall}},
		{kind.BinaryIDiv, BinaryLaw{Primitive: PrimitiveNumber, Meta: MetaIDiv, MissingFault: FaultArithmetic, Handler: HandlerOrdinaryCall}},
		{kind.BinaryMod, BinaryLaw{Primitive: PrimitiveNumber, Meta: MetaMod, MissingFault: FaultArithmetic, Handler: HandlerOrdinaryCall}},
		{kind.BinaryPow, BinaryLaw{Primitive: PrimitiveNumber, Meta: MetaPow, MissingFault: FaultArithmetic, Handler: HandlerOrdinaryCall}},
		{kind.BinaryConcat, BinaryLaw{Primitive: PrimitiveConcat, Meta: MetaConcat, MissingFault: FaultConcat, Handler: HandlerOrdinaryCall}},
		{kind.BinaryBitAnd, BinaryLaw{Primitive: PrimitiveInteger, Meta: MetaBand, MissingFault: FaultBitwise, Handler: HandlerOrdinaryCall}},
		{kind.BinaryBitOr, BinaryLaw{Primitive: PrimitiveInteger, Meta: MetaBor, MissingFault: FaultBitwise, Handler: HandlerOrdinaryCall}},
		{kind.BinaryBitXor, BinaryLaw{Primitive: PrimitiveInteger, Meta: MetaBxor, MissingFault: FaultBitwise, Handler: HandlerOrdinaryCall}},
		{kind.BinaryShiftLeft, BinaryLaw{Primitive: PrimitiveInteger, Meta: MetaShl, MissingFault: FaultBitwise, Handler: HandlerOrdinaryCall}},
		{kind.BinaryShiftRight, BinaryLaw{Primitive: PrimitiveInteger, Meta: MetaShr, MissingFault: FaultBitwise, Handler: HandlerOrdinaryCall}},
		{kind.BinaryEqual, BinaryLaw{Primitive: PrimitiveEquality, Meta: MetaEq, Handler: HandlerOrdinaryCall, Truthify: true}},
		{kind.BinaryNotEqual, BinaryLaw{Primitive: PrimitiveEquality, Meta: MetaEq, Handler: HandlerOrdinaryCall, Truthify: true, Negate: true}},
		{kind.BinaryLess, BinaryLaw{Primitive: PrimitiveOrder, Meta: MetaLt, MissingFault: FaultOrder, Handler: HandlerOrdinaryCall, Truthify: true}},
		{kind.BinaryLessEqual, BinaryLaw{Primitive: PrimitiveOrder, Meta: MetaLe, MissingFault: FaultOrder, Handler: HandlerOrdinaryCall, ReverseLessFallback: true, Truthify: true}},
		{kind.BinaryGreater, BinaryLaw{Primitive: PrimitiveOrder, Meta: MetaLt, MissingFault: FaultOrder, Handler: HandlerOrdinaryCall, ReverseOperands: true, Truthify: true}},
		{kind.BinaryGreaterEqual, BinaryLaw{Primitive: PrimitiveOrder, Meta: MetaLe, MissingFault: FaultOrder, Handler: HandlerOrdinaryCall, ReverseOperands: true, ReverseLessFallback: true, Truthify: true}},
	}

	for _, test := range cases {
		got, ok := Binary(test.op)
		if !ok || got != test.want {
			t.Fatalf("Binary(%d) = %#v/%v, want %#v/true", test.op, got, ok, test.want)
		}
		assertBinaryLaw(t, got)
	}
	for op := kind.BinaryAdd; op <= kind.BinaryGreaterEqual; op++ {
		if _, ok := Binary(op); !ok {
			t.Fatalf("canonical binary op %d has no law", op)
		}
	}
	if got, ok := Binary(kind.BinaryOp(255)); ok || got != (BinaryLaw{}) {
		t.Fatalf("invalid binary law = %#v/%v, want zero/false", got, ok)
	}
}

func TestComparisonLawsTruthifyBeforeDirectionAndNegation(t *testing.T) {
	less, _ := Binary(kind.BinaryLess)
	greater, _ := Binary(kind.BinaryGreater)
	lessEqual, _ := Binary(kind.BinaryLessEqual)
	greaterEqual, _ := Binary(kind.BinaryGreaterEqual)
	equal, _ := Binary(kind.BinaryEqual)
	notEqual, _ := Binary(kind.BinaryNotEqual)

	for _, law := range [...]BinaryLaw{less, greater, lessEqual, greaterEqual, equal, notEqual} {
		if !law.Truthify {
			t.Fatalf("comparison law %#v does not truthify its scalar-adjusted handler result", law)
		}
	}
	if less.Meta != MetaLt || less.ReverseOperands || less.ReverseLessFallback {
		t.Fatalf("less-than law = %#v, want forward __lt only", less)
	}
	if greater.Meta != MetaLt || !greater.ReverseOperands || greater.ReverseLessFallback {
		t.Fatalf("greater-than law = %#v, want reversed __lt only", greater)
	}
	if lessEqual.Meta != MetaLe || lessEqual.ReverseOperands || !lessEqual.ReverseLessFallback || lessEqual.Negate {
		t.Fatalf("less-equal law = %#v, want forward __le then reversed not-__lt", lessEqual)
	}
	if greaterEqual.Meta != MetaLe || !greaterEqual.ReverseOperands || !greaterEqual.ReverseLessFallback || greaterEqual.Negate {
		t.Fatalf("greater-equal law = %#v, want reversed __le then reversed not-__lt", greaterEqual)
	}
	if equal.Meta != MetaEq || equal.Negate || notEqual.Meta != MetaEq || !notEqual.Negate {
		t.Fatalf("equality laws = %#v / %#v, want left-then-right __eq and only ~= negated after truthification", equal, notEqual)
	}
}

func TestMissingAndPresentHandlerOutcomesAreDistinct(t *testing.T) {
	if got := HandlerOrdinaryCall.NonCallableFault(); got != FaultNone {
		t.Fatalf("ordinary-call handler non-callable outcome = %d, want ordinary Call", got)
	}
	if got := HandlerDirectFunctionOrReenter.NonCallableFault(); got != FaultNone {
		t.Fatalf("delegating handler non-callable outcome = %d, want reentry/FaultNone", got)
	}
	if got := HandlerDirectFunction.NonCallableFault(); got != FaultCall {
		t.Fatalf("direct-function handler non-callable outcome = %d, want FaultCall", got)
	}

	neg, _ := Unary(kind.UnaryNeg)
	add, _ := Binary(kind.BinaryAdd)
	read, write, call := Read(), Write(), Call()
	if neg.MissingFault != FaultArithmetic || neg.Handler != HandlerOrdinaryCall || neg.Handler.NonCallableFault() != FaultNone {
		t.Fatalf("unary handler distinction = %#v", neg)
	}
	if add.MissingFault != FaultArithmetic || add.Handler != HandlerOrdinaryCall || add.Handler.NonCallableFault() != FaultNone {
		t.Fatalf("binary handler distinction = %#v", add)
	}
	if read.MissingFault != FaultIndex || read.Handler != HandlerDirectFunctionOrReenter || read.Handler.NonCallableFault() != FaultNone {
		t.Fatalf("read handler distinction = %#v", read)
	}
	if write.MissingFault != FaultIndex || write.Handler != HandlerDirectFunctionOrReenter || write.Handler.NonCallableFault() != FaultNone {
		t.Fatalf("write handler distinction = %#v", write)
	}
	if call.MissingFault != FaultCall || call.Handler != HandlerDirectFunction || call.Handler.NonCallableFault() != FaultCall {
		t.Fatalf("call handler distinction = %#v", call)
	}
}

func TestAccessCallAndIteratorLawsAreExact(t *testing.T) {
	read := Read()
	if want := (ReadLaw{Meta: MetaIndex, Handler: HandlerDirectFunctionOrReenter, MissingFault: FaultIndex, MetaArity: 2}); read != want {
		t.Fatalf("Read() = %#v, want %#v", read, want)
	}
	write := Write()
	if want := (WriteLaw{Meta: MetaNewIndex, Handler: HandlerDirectFunctionOrReenter, MissingFault: FaultIndex, MetaArity: 3}); write != want {
		t.Fatalf("Write() = %#v, want %#v", write, want)
	}
	call := Call()
	if want := (CallLaw{Primitive: PrimitiveFunction, Meta: MetaCall, Handler: HandlerDirectFunction, MissingFault: FaultCall, ReceiverPrefix: 1}); call != want {
		t.Fatalf("Call() = %#v, want %#v", call, want)
	}
	iterator := GenericFor()
	if want := (GenericForLaw{HeaderWidth: 3, GeneratorIndex: 0, StateIndex: 1, ControlIndex: 2, ControlResultIndex: 0}); iterator != want {
		t.Fatalf("GenericFor() = %#v, want %#v", iterator, want)
	}

	if !read.Meta.valid() || !read.Handler.valid() || !read.MissingFault.valid() {
		t.Fatalf("invalid read law %#v", read)
	}
	if !write.Meta.valid() || !write.Handler.valid() || !write.MissingFault.valid() {
		t.Fatalf("invalid write law %#v", write)
	}
	if !call.Primitive.valid() || !call.Meta.valid() || !call.Handler.valid() || !call.MissingFault.valid() {
		t.Fatalf("invalid call law %#v", call)
	}
}

func TestLawVocabulariesAreClosed(t *testing.T) {
	for slot := MetaUnm; slot <= MetaCall; slot++ {
		if !slot.valid() {
			t.Fatalf("meta slot %d is not valid", slot)
		}
	}
	if MetaAbsent.valid() || MetaSlot(MetaCall+1).valid() {
		t.Fatal("meta slot vocabulary admits a sentinel or extension")
	}
	for rule := PrimitiveTruth; rule <= PrimitiveFunction; rule++ {
		if !rule.valid() {
			t.Fatalf("primitive rule %d is not valid", rule)
		}
	}
	if PrimitiveNone.valid() || PrimitiveRule(PrimitiveFunction+1).valid() {
		t.Fatal("primitive rule vocabulary admits a sentinel or extension")
	}
	for rule := FaultArithmetic; rule <= FaultCall; rule++ {
		if !rule.valid() {
			t.Fatalf("fault rule %d is not valid", rule)
		}
	}
	if FaultNone.valid() || FaultRule(FaultCall+1).valid() {
		t.Fatal("fault rule vocabulary admits a sentinel or extension")
	}
	for rule := HandlerOrdinaryCall; rule <= HandlerDirectFunction; rule++ {
		if !rule.valid() {
			t.Fatalf("handler rule %d is not valid", rule)
		}
	}
	if HandlerRule(0).valid() || HandlerRule(HandlerDirectFunction+1).valid() {
		t.Fatal("handler rule vocabulary admits a sentinel or extension")
	}
}

var lawAllocationSink uint64

func TestLawLookupAllocatesNothing(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		var sink uint64
		for op := kind.UnaryNeg; op <= kind.UnaryBitNot; op++ {
			law, ok := Unary(op)
			if !ok {
				panic("canonical unary law absent")
			}
			sink += uint64(law.Primitive) + uint64(law.Meta) + uint64(law.MissingFault) + uint64(law.Handler)
		}
		for op := kind.BinaryAdd; op <= kind.BinaryGreaterEqual; op++ {
			law, ok := Binary(op)
			if !ok {
				panic("canonical binary law absent")
			}
			sink += uint64(law.Primitive) + uint64(law.Meta) + uint64(law.MissingFault) + uint64(law.Handler)
			if law.ReverseOperands {
				sink++
			}
			if law.ReverseLessFallback {
				sink++
			}
			if law.Truthify {
				sink++
			}
			if law.Negate {
				sink++
			}
		}
		read, write, call, iterator := Read(), Write(), Call(), GenericFor()
		sink += uint64(read.Meta + MetaSlot(read.MetaArity))
		sink += uint64(write.Meta + MetaSlot(write.MetaArity))
		sink += uint64(call.Meta + MetaSlot(call.ReceiverPrefix))
		sink += uint64(iterator.HeaderWidth + iterator.GeneratorIndex + iterator.StateIndex + iterator.ControlIndex + iterator.ControlResultIndex)
		lawAllocationSink = sink
	})
	if allocs != 0 {
		t.Fatalf("law lookup allocations = %v, want 0", allocs)
	}
}

func assertUnaryLaw(t *testing.T, law UnaryLaw) {
	t.Helper()
	if !law.Primitive.valid() {
		t.Fatalf("invalid unary primitive %#v", law)
	}
	if law.Meta == MetaAbsent {
		if law.MissingFault != FaultNone || law.Handler != 0 {
			t.Fatalf("metamethod-free unary law has meta state %#v", law)
		}
		return
	}
	if !law.Meta.valid() || !law.MissingFault.valid() || law.Handler != HandlerOrdinaryCall {
		t.Fatalf("invalid unary metamethod law %#v", law)
	}
}

func assertBinaryLaw(t *testing.T, law BinaryLaw) {
	t.Helper()
	if !law.Primitive.valid() || !law.Meta.valid() || !law.Handler.valid() {
		t.Fatalf("invalid binary law %#v", law)
	}
	if law.MissingFault != FaultNone && !law.MissingFault.valid() {
		t.Fatalf("invalid binary missing fault %#v", law)
	}
	if law.ReverseLessFallback && law.Meta != MetaLe {
		t.Fatalf("non-__le law declares reverse-less fallback %#v", law)
	}
	if law.Negate && law.Meta != MetaEq {
		t.Fatalf("non-__eq law declares negation %#v", law)
	}
}
