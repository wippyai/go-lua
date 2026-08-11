// Package application owns the closed Lua 5.3 application rules consumed by
// downstream analysis domains. Program contributes only typed source
// occurrences; this package owns dispatch and type-result semantics without
// inspecting Program storage or allocating Program terms.
package application

import "github.com/wippyai/go-lua/program/flow/kind"

// MetaSlot is the complete non-string metamethod vocabulary needed by source
// application. The eventual interpreter/profile boundary owns the spelling
// of these slots; this package retains the compact semantic identity.
type MetaSlot uint8

const (
	MetaAbsent MetaSlot = iota
	MetaUnm
	MetaBNot
	MetaLen
	MetaAdd
	MetaSub
	MetaMul
	MetaDiv
	MetaIDiv
	MetaMod
	MetaPow
	MetaConcat
	MetaBand
	MetaBor
	MetaBxor
	MetaShl
	MetaShr
	MetaEq
	MetaLt
	MetaLe
	MetaIndex
	MetaNewIndex
	MetaCall
)

func (slot MetaSlot) valid() bool { return slot >= MetaUnm && slot <= MetaCall }

// PrimitiveRule names the exact native operation attempted before a
// metamethod. Number permits Lua numeric conversion (including a numeral
// string); Integer permits Lua's exact integer conversion; Length encodes
// Lua's string, then metamethod, then raw-table order. The operator itself
// supplies the operation-specific result calculation.
type PrimitiveRule uint8

const (
	PrimitiveNone PrimitiveRule = iota
	PrimitiveTruth
	PrimitiveNumber
	PrimitiveInteger
	PrimitiveLength
	PrimitiveConcat
	PrimitiveEquality
	PrimitiveOrder
	PrimitiveFunction
)

func (rule PrimitiveRule) valid() bool {
	return rule >= PrimitiveTruth && rule <= PrimitiveFunction
}

// FaultRule identifies the sole runtime-error class left after all candidates
// declared by a law have failed.  It is source semantics, not a diagnostic.
type FaultRule uint8

const (
	FaultNone FaultRule = iota
	FaultArithmetic
	FaultBitwise
	FaultLength
	FaultConcat
	FaultOrder
	FaultIndex
	FaultCall
)

func (rule FaultRule) valid() bool { return rule >= FaultArithmetic && rule <= FaultCall }

// HandlerRule is the complete Lua 5.3 treatment of a present metamethod slot.
// Operator handlers are invoked through ordinary Call, so a non-function may
// itself resolve __call and recur. Access handlers directly invoke only a function;
// another non-nil value re-enters the same raw access law. Ordinary Call's
// resolved __call slot must itself be a function. MissingFault applies only
// when the slot is absent.
type HandlerRule uint8

const (
	HandlerOrdinaryCall HandlerRule = iota + 1
	HandlerDirectFunctionOrReenter
	HandlerDirectFunction
)

func (rule HandlerRule) valid() bool {
	return rule >= HandlerOrdinaryCall && rule <= HandlerDirectFunction
}

// NonCallableFault returns an immediate fault only where Lua requires the
// resolved slot itself to be a function. Ordinary-call handlers delegate to
// Call and access handlers re-enter, so neither faults at this point.
func (rule HandlerRule) NonCallableFault() FaultRule {
	if rule == HandlerDirectFunction {
		return FaultCall
	}
	return FaultNone
}

// UnaryLaw is the complete fixed dispatch law for one canonical unary op.
// Every unary metamethod uses the structural call shape (x,x), while x is
// evaluated once. That universal ABI is deliberately not duplicated in rows.
// PrimitiveLength is the sole exception to ordinary primitive-first order: it
// takes string length, then consults MetaLen, then takes raw table length.
type UnaryLaw struct {
	Primitive    PrimitiveRule
	Meta         MetaSlot
	MissingFault FaultRule
	Handler      HandlerRule
}

// BinaryLaw is the complete fixed dispatch law for one canonical binary op.
// ReverseOperands applies to both primitive and Meta. Binary Meta lookup
// always tries the first effective operand and then the second, and never
// requires handler identity. Comparison metamethod results are structurally
// scalar-adjusted before Truthify and Negate; scalar adjustment is universal
// and therefore not duplicated in rows. ReverseLessFallback applies only
// after the declared handler is absent and means not (y < x) for effective x,y.
type BinaryLaw struct {
	Primitive           PrimitiveRule
	Meta                MetaSlot
	MissingFault        FaultRule
	Handler             HandlerRule
	ReverseOperands     bool
	ReverseLessFallback bool
	Truthify            bool
	Negate              bool
}

// ReadLaw specifies ordinary Lens read behavior.  A raw read is always
// attempted first.  An absent raw key on a table is nil; a non-table with no
// usable __index faults. A non-callable non-nil handler re-enters this same
// law, which must be represented with Mu rather than bounded host recursion.
type ReadLaw struct {
	Meta         MetaSlot
	Handler      HandlerRule
	MissingFault FaultRule
	MetaArity    uint8
}

// WriteLaw specifies ordinary Lens write behavior.  A present raw table key
// is written directly; an absent table key and a non-table may dispatch
// through __newindex.  Delegation re-enters this same law without a bound.
type WriteLaw struct {
	Meta         MetaSlot
	Handler      HandlerRule
	MissingFault FaultRule
	MetaArity    uint8
}

// CallLaw specifies ordinary Call behavior.  A function is invoked directly.
// Otherwise __call must itself be a function and receives the original callee
// before the authored actual list.
type CallLaw struct {
	Primitive      PrimitiveRule
	Meta           MetaSlot
	Handler        HandlerRule
	MissingFault   FaultRule
	ReceiverPrefix uint8
}

// GenericForLaw specifies Lua's generic-for protocol without creating a
// synthetic Call.  Header values are exactly generator,state,control.  On
// every iteration the generator uses the ordinary CallLaw with state/control
// actuals. The value at ControlResultIndex terminates on nil; otherwise it
// replaces hidden control before loop Cells adjust the remaining result pack.
type GenericForLaw struct {
	HeaderWidth        uint8
	GeneratorIndex     uint8
	StateIndex         uint8
	ControlIndex       uint8
	ControlResultIndex uint8
}

// Unary returns the one closed law for op.  Unsupported values have no law.
func Unary(op kind.UnaryOp) (UnaryLaw, bool) {
	switch op {
	case kind.UnaryNeg:
		return unaryMeta(PrimitiveNumber, MetaUnm, FaultArithmetic), true
	case kind.UnaryNot:
		return UnaryLaw{Primitive: PrimitiveTruth}, true
	case kind.UnaryLen:
		return unaryMeta(PrimitiveLength, MetaLen, FaultLength), true
	case kind.UnaryBitNot:
		return unaryMeta(PrimitiveInteger, MetaBNot, FaultBitwise), true
	default:
		return UnaryLaw{}, false
	}
}

func unaryMeta(primitive PrimitiveRule, meta MetaSlot, missingFault FaultRule) UnaryLaw {
	return UnaryLaw{
		Primitive: primitive, Meta: meta, MissingFault: missingFault,
		Handler: HandlerOrdinaryCall,
	}
}

// Binary returns the one closed law for op.  Every binary law first tries its
// Primitive rule.  On primitive failure, Meta lookup checks the first
// effective operand and then the second; it never requires handler identity.
func Binary(op kind.BinaryOp) (BinaryLaw, bool) {
	switch op {
	case kind.BinaryAdd:
		return numericBinary(MetaAdd), true
	case kind.BinarySub:
		return numericBinary(MetaSub), true
	case kind.BinaryMul:
		return numericBinary(MetaMul), true
	case kind.BinaryDiv:
		return numericBinary(MetaDiv), true
	case kind.BinaryIDiv:
		return numericBinary(MetaIDiv), true
	case kind.BinaryMod:
		return numericBinary(MetaMod), true
	case kind.BinaryPow:
		return numericBinary(MetaPow), true
	case kind.BinaryConcat:
		return binaryMeta(PrimitiveConcat, MetaConcat, FaultConcat), true
	case kind.BinaryBitAnd:
		return integerBinary(MetaBand), true
	case kind.BinaryBitOr:
		return integerBinary(MetaBor), true
	case kind.BinaryBitXor:
		return integerBinary(MetaBxor), true
	case kind.BinaryShiftLeft:
		return integerBinary(MetaShl), true
	case kind.BinaryShiftRight:
		return integerBinary(MetaShr), true
	case kind.BinaryEqual:
		return comparisonMeta(PrimitiveEquality, MetaEq, FaultNone, false, false, false), true
	case kind.BinaryNotEqual:
		return comparisonMeta(PrimitiveEquality, MetaEq, FaultNone, false, false, true), true
	case kind.BinaryLess:
		return comparisonMeta(PrimitiveOrder, MetaLt, FaultOrder, false, false, false), true
	case kind.BinaryLessEqual:
		return comparisonMeta(PrimitiveOrder, MetaLe, FaultOrder, false, true, false), true
	case kind.BinaryGreater:
		return comparisonMeta(PrimitiveOrder, MetaLt, FaultOrder, true, false, false), true
	case kind.BinaryGreaterEqual:
		return comparisonMeta(PrimitiveOrder, MetaLe, FaultOrder, true, true, false), true
	default:
		return BinaryLaw{}, false
	}
}

func numericBinary(meta MetaSlot) BinaryLaw {
	return binaryMeta(PrimitiveNumber, meta, FaultArithmetic)
}

func integerBinary(meta MetaSlot) BinaryLaw {
	return binaryMeta(PrimitiveInteger, meta, FaultBitwise)
}

func binaryMeta(primitive PrimitiveRule, meta MetaSlot, missingFault FaultRule) BinaryLaw {
	return BinaryLaw{Primitive: primitive, Meta: meta, MissingFault: missingFault, Handler: HandlerOrdinaryCall}
}

func comparisonMeta(
	primitive PrimitiveRule,
	meta MetaSlot,
	missingFault FaultRule,
	reverseOperands bool,
	reverseLessFallback bool,
	negate bool,
) BinaryLaw {
	return BinaryLaw{
		Primitive: primitive, Meta: meta, MissingFault: missingFault, Handler: HandlerOrdinaryCall,
		ReverseOperands: reverseOperands, ReverseLessFallback: reverseLessFallback,
		Truthify: true, Negate: negate,
	}
}

// Read returns the one source-level dynamic access law.  Table constructor
// fields use raw initialization and therefore intentionally do not use this.
func Read() ReadLaw {
	return ReadLaw{Meta: MetaIndex, Handler: HandlerDirectFunctionOrReenter, MissingFault: FaultIndex, MetaArity: 2}
}

// Write returns the one source-level dynamic store law.  Table constructor
// fields use raw initialization and therefore intentionally do not use this.
func Write() WriteLaw {
	return WriteLaw{Meta: MetaNewIndex, Handler: HandlerDirectFunctionOrReenter, MissingFault: FaultIndex, MetaArity: 3}
}

// Call returns the one source-level call law.
func Call() CallLaw {
	return CallLaw{Primitive: PrimitiveFunction, Meta: MetaCall, Handler: HandlerDirectFunction, MissingFault: FaultCall, ReceiverPrefix: 1}
}

// GenericFor returns the fixed generic iterator protocol.
func GenericFor() GenericForLaw {
	return GenericForLaw{
		HeaderWidth: 3, GeneratorIndex: 0, StateIndex: 1, ControlIndex: 2, ControlResultIndex: 0,
	}
}
