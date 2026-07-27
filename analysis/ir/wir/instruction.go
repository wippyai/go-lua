// Package wir defines the checker instruction IR: a small, closed instruction
// set lowered from typed-Lua syntax and attached per CFG point.
//
// Each function body lowers to a flat instruction stream; instructions carry
// canonical operands (interned paths, constants, type refs) and a Check
// descriptor for conditions. The instruction set is the stable interface
// between lowering (syntax translation and binding/type resolution) and the
// transfer interpreter (value derivation). Lowering records syntax facts while
// transfer derives value facts from Branch(Check) and Call.
//
// Design targets zero-alloc iteration: instructions are value structs in a flat
// slice, operands are scalar (kind, uint32 index) handles into per-Body intern
// pools, and variadic operand lists reference a shared pool by (start, len)
// range rather than per-instruction slices.
package wir

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// Op is the closed instruction opcode set. The type sublanguage costs zero
// instructions: type declarations resolve at bind/signature time and never
// enter the stream.
type Op uint8

const (
	// OpNoop is a structural no-op occupying a point that carries no value effect.
	OpNoop Op = iota
	// OpEntry marks the function entry point.
	OpEntry
	// OpExit marks the function exit point.
	OpExit
	// OpAssign copies a value operand into a destination path or temp.
	OpAssign
	// OpStaticMemberWrite stores A into the statically known member path in Dst.
	OpStaticMemberWrite
	// OpDynamicIndexWrite stores B into Dst[A] where the key A is dynamic.
	OpDynamicIndexWrite
	// OpDynamicIndexRead computes Dst = A[B] where the table or key is dynamic.
	OpDynamicIndexRead
	// OpMakeTable constructs a table literal into Dst from List entry values.
	OpMakeTable
	// OpBinOp computes Dst = A <Operator> B.
	OpBinOp
	// OpUnOp computes Dst = <Operator> A.
	OpUnOp
	// OpConcat computes Dst = A .. B, or a flattened n-ary concat over List.
	OpConcat
	// OpCall invokes Call.Callee (or Call.Receiver:method) over List args into Results.
	OpCall
	// OpReturn returns the List value operands from the function.
	OpReturn
	// OpBranch selects a CFG edge on the Check descriptor (topology stays in the CFG).
	OpBranch
	// OpIterate is a for-loop header binding Results from the iterator List.
	OpIterate
	// OpClaim asserts a value fact (cast, non-nil assert, annotation, asserts predicate).
	OpClaim
	// OpSelect is a recognized channel-select over case operands in List.
	OpSelect
	// OpLogical computes a short-circuit Dst = A <and|or> B. Unlike OpBinOp it is
	// not a strict operator: transfer derives which operand flows to Dst and the
	// guard narrowing the right operand inherits (truthy/falsy A). See design.md.
	OpLogical
	// OpClosure materializes a function literal into Dst. Func references the
	// nested Body proto; List names the captured (upvalue) operands in bind order.
	OpClosure
)

// Operator selects the arithmetic, relational, or unary operation for OpBinOp
// and OpUnOp. Operators are a small closed enum so no per-instruction operator
// string is stored.
type Operator uint8

const (
	OperatorNone Operator = iota
	// Arithmetic and bitwise binary operators.
	BinAdd
	BinSub
	BinMul
	BinDiv
	BinIDiv
	BinMod
	BinPow
	BinBAnd
	BinBOr
	BinBXor
	BinShl
	BinShr
	// Relational binary operators.
	BinEq
	BinNe
	BinLt
	BinLe
	BinGt
	BinGe
	// Unary operators.
	UnNeg
	UnNot
	UnLen
	UnBNot
	// Short-circuit logical operators for OpLogical.
	LogAnd
	LogOr
)

// IterKind distinguishes numeric and generic for-loop headers for OpIterate.
type IterKind uint8

const (
	IterNumeric IterKind = iota // for i = init, limit, step
	IterGeneric                 // for k, v in f, s, var
)

// ClaimKind identifies the value-fact family a Claim unifies. Claim replaces the
// separate cast / non-nil / annotation / asserts-predicate fact lanes; the
// transfer interpreter applies the narrowing, lowering only records the syntax.
type ClaimKind uint8

const (
	ClaimNone ClaimKind = iota
	// ClaimCast is an unsafe cast: expr as T / expr :: T.
	ClaimCast
	// ClaimAssert is a non-nil assertion: expr!.
	ClaimAssert
	// ClaimAnnotation is a declared variable type: local x: T.
	ClaimAnnotation
	// ClaimAssertsPredicate is a return-position predicate: asserts x is T.
	ClaimAssertsPredicate
)

// AssignKind records the syntactic origin of a root-path write. It is not a
// value fact; transfer uses it to preserve the declaration-vs-reassignment
// distinction without consulting assignment sidecars.
type AssignKind uint8

const (
	AssignNone AssignKind = iota
	AssignLocalDeclaration
	AssignOrdinaryRootWrite
)

// OperandKind tags an operand's referenced pool. The zero value OperandNone is
// an absent operand.
type OperandKind uint8

const (
	OperandNone OperandKind = iota
	// OperandPath references Body.paths.
	OperandPath
	// OperandConst references Body.consts.
	OperandConst
	// OperandType references Body.types.
	OperandType
	// OperandTemp is a body-local SSA temp id (an expression result).
	OperandTemp
	// OperandVararg is the Lua vararg value (...).
	OperandVararg
)

// Operand is a scalar handle into a Body intern pool (or a temp id). It holds no
// pointers, so instruction slices never require per-element allocation.
type Operand struct {
	Kind OperandKind
	Ref  uint32
}

// PathRef, ConstRef, TypeRef, CheckRef, and FuncRef are 1-based indices into the
// matching Body intern pool. A zero ref means none (index 0 is a reserved
// sentinel). ExpressionID is an opaque process-local source expression identity
// used only to join WIR metadata back to factflow expression facts during
// migration; it is not a serialized AST pointer.
type (
	PathRef      uint32
	ConstRef     uint32
	TypeRef      uint32
	CheckRef     uint32
	FuncRef      uint32
	ExpressionID uint64
)

// OperandRange is a [Start, Start+Len) window into Body.operandPool for a
// variadic operand list.
type OperandRange struct {
	Start uint32
	Len   uint32
}

// CallInfo carries the callee shape for OpCall.
type CallInfo struct {
	// Callee is the function value for a direct call f(...). None for method calls.
	Callee Operand
	// Receiver is the receiver value for method-call sugar recv:m(...). None otherwise.
	Receiver Operand
	// Method is the method-name string const for method-call sugar. Zero otherwise.
	Method ConstRef
}

// CallContextKind identifies the syntactic context where an OpCall occurs.
type CallContextKind uint8

const (
	CallContextStatement CallContextKind = iota + 1
	CallContextAssignmentSource
	CallContextReturnSource
	CallContextIteratorSource
	CallContextCondition
	CallContextExpressionProducer
)

// Instruction is a single wir operation. Field meaning is selected by Op; unused
// slots are zero. The struct is a flat value stored in Body.instrs.
type Instruction struct {
	Op    Op
	Point cfg.Point

	// Dst is the value destination for producing instructions:
	//   OpAssign/OpDynamicIndexRead/OpBinOp/OpUnOp/OpConcat/OpMakeTable/OpClaim/OpSelect: result
	//   OpStaticMemberWrite: the member path written
	//   OpDynamicIndexWrite: the container path written
	Dst Operand

	// A and B are fixed operand slots (see each Op's doc comment).
	A Operand
	B Operand

	// List is the variadic operand window (call args, return values, table
	// entries, n-ary concat operands, iterator sources, select cases).
	List OperandRange

	// TableEntries is the statically-addressable constructor entry window for
	// OpMakeTable. It is analysis metadata; List remains the runtime value list.
	TableEntries TableEntryRange

	// StaticStringKeysComplete reports that every constructor field was
	// statically classifiable, so the string-key subset of TableEntries is a
	// closed key set. Array/int entries may still exist and are ignored by
	// string-key dispatch consumers.
	StaticStringKeysComplete bool

	// DynamicSuffix carries static member/index segments after the dynamic key
	// for OpDynamicIndexWrite targets such as t[k].field.
	DynamicSuffix SegmentRange

	// DynamicTargetDisplay and DynamicValueDisplay retain the authored surface
	// for a dynamic write diagnostic. They are source metadata only: the
	// equation still derives validity exclusively from the closed heap facts.
	DynamicTargetDisplay string
	DynamicValueDisplay  string

	// ImpliedChecks is the normalized leaf-check window for OpBranch compound
	// conditions. Check carries the direct condition when one exists; this range
	// carries edge-specific leaves proven by and/or/not structure.
	ImpliedChecks ImpliedCheckRange

	// AssignmentSnapshot is the first write point of the Lua assignment
	// statement that owns this instruction. Lua evaluates all right-hand sides
	// before any target write, so every target in the statement reads from the
	// same predecessor boundary.
	AssignmentSnapshot    cfg.Point
	HasAssignmentSnapshot bool

	// DiffConstraints is the normalized difference-logic descriptor window for
	// OpBranch conditions. It carries syntax-derived linear relations; transfer
	// decides the factflow projection.
	DiffConstraints BranchDiffConstraintRange

	// Results is the variadic destination window (multi-value call results,
	// for-loop variables).
	Results OperandRange

	Operator Operator  // OpBinOp / OpUnOp / OpLogical selector
	Iter     IterKind  // OpIterate numeric/generic
	Claim    ClaimKind // OpClaim family
	Assign   AssignKind
	Type     TypeRef // OpClaim target, OpMakeTable declared type, OpCall type receiver (0 = none)
	// ClaimTypeDisplay and ClaimSourceDisplay preserve authored presentation
	// identities for OpClaim. They are diagnostic provenance only: Type remains
	// the sole resolved type authority and A remains the sole value authority.
	// In particular, an imported alias and a materialized call/member expression
	// must not be reconstructed from a structural type or a temporary name.
	ClaimTypeDisplay          string
	ClaimSourceDisplay        string
	ClaimSourceMethodSelector bool
	Check                     CheckRef // normalized condition descriptor for OpBranch/OpCall/boolean OpBinOp (0 = none)
	Func                      FuncRef  // OpClosure nested proto (0 = none)
	Call                      CallInfo // OpCall shape
	ExprID                    ExpressionID

	CallContext  CallContextKind
	CallFinal    bool
	CallExpanded bool
	CallAdjusted bool
	CallOpenTail bool
	// CallConditionNegated is true when a condition call is the operand of unary
	// `not`, so the CFG true edge corresponds to the call's falsy result.
	CallConditionNegated bool
	CallSpan             Span
	CalleeSpan           Span
	CallArgs             CallArgumentMetaRange
	CallTypeArgs         TypeRefRange
	ReturnValues         ReturnValueMetaRange
	ConcatOperands       ConcatOperandMetaRange

	// ExprSpan is source-only metadata for the expression lowered by this
	// instruction when downstream diagnostics need the whole expression range
	// rather than a child operand span.
	ExprSpan Span

	// TargetSpan is source-only metadata for the assignment target written by this
	// instruction. It lets post-solve diagnostics render write locations from WIR
	// facts without retaining assignment AST sidecars.
	TargetSpan Span

	// DeclaredSpan is the declared type in an annotation claim.  It is separate
	// from TargetSpan because assignment failures point at the assigned source,
	// while the asserted contract is best explained at its type annotation.
	DeclaredSpan Span

	// ContainerSpan is source-only metadata for the container being assigned
	// through by member/index writes. It lets diagnostics point at the nil-risk
	// receiver separately from the full assignment target.
	ContainerSpan Span

	// JoinSpan is the source anchor of the point where a branch's arms rejoin:
	// the closing `end` of the authored if statement. Diagnostics that explain
	// what a value carries past a merge anchor there rather than at the
	// condition, which ExprSpan already owns.
	JoinSpan Span

	// SelectDefault marks an OpSelect that carries a default (non-blocking) case.
	SelectDefault bool

	// ListSpread marks that the final List operand expands to all runtime values it
	// produces (an open multi-value tail): `return f()`, `g(a, h())`, `f(...)`. The
	// preceding List operands are the exact static arity; only the tail is open.
	// This is the information a codegen backend needs to emit a multret-forwarding
	// arg/return without re-deriving arity.
	ListSpread bool

	// ResultSpread marks an OpCall whose produced result count is dynamic (multret,
	// not truncated to Results.Len): a call in tail position of an arg/return list.
	// Results names the explicitly bound head destinations; ResultSpread says the
	// count is open beyond them.
	ResultSpread bool

	// TailIndex is the array index the head value of an OpMakeTable's expanding
	// final positional field occupies. ListSpread states that the tail is open;
	// TailIndex states where it starts, so a consumer that resolves the tail
	// producer's result count sizes the expansion exactly instead of discarding
	// the constructor inventory. Zero when the constructor has no such field.
	TailIndex int
}

// AssignmentSourceOperand returns the operand whose value is written by an
// assignment-like instruction. It centralizes the opcode layout so consumers do
// not need to know that dynamic-index stores write B while direct/static stores
// and claims (casts, non-nil asserts) write A.
func (i Instruction) AssignmentSourceOperand() (Operand, bool) {
	switch i.Op {
	case OpAssign, OpStaticMemberWrite, OpClaim:
		if i.A.Kind != OperandNone {
			return i.A, true
		}
	case OpDynamicIndexWrite:
		if i.B.Kind != OperandNone {
			return i.B, true
		}
	}
	return Operand{}, false
}

// WritesAssignmentPoint reports whether the instruction writes a value at its
// CFG point for assignment lowering purposes.
func (i Instruction) WritesAssignmentPoint() bool {
	switch i.Op {
	case OpAssign, OpDynamicIndexRead, OpMakeTable, OpBinOp, OpUnOp, OpConcat, OpClaim, OpSelect, OpLogical, OpClosure:
		return i.Dst.Kind != OperandNone
	case OpStaticMemberWrite, OpDynamicIndexWrite:
		return true
	default:
		return false
	}
}
