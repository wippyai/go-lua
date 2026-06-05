package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// ReturnKind classifies the constant value of a return statement for predicate analysis.
// Used to determine if a return is definitively true, false, or dynamic.
type ReturnKind uint8

const (
	ReturnUnknown ReturnKind = iota
	ReturnTrue               // return true
	ReturnFalse              // return false
)

// ConstKind classifies the type of a constant value tracked for type narrowing.
// Constants are tracked to enable literal-based narrowing (e.g., x == "foo").
type ConstKind uint8

const (
	ConstUnknown ConstKind = iota // 0 = zero value = unknown
	ConstString
	ConstInt
	ConstFloat
	ConstBool
	ConstNil
)

// ConstValue represents a constant value (string, int, float, bool, or nil).
type ConstValue struct {
	Kind  ConstKind
	Str   string
	Int   int64
	Float float64
	Bool  bool
}

// ToLiteralType converts a ConstValue to its corresponding literal type.
func (c *ConstValue) ToLiteralType() typ.Type {
	if c == nil {
		return nil
	}
	switch c.Kind {
	case ConstString:
		return typ.LiteralString(c.Str)
	case ConstInt:
		return typ.LiteralInt(c.Int)
	case ConstFloat:
		return typ.LiteralNumber(c.Float)
	case ConstBool:
		return typ.LiteralBool(c.Bool)
	case ConstNil:
		return typ.Nil
	default:
		return nil
	}
}

// IteratorKind describes the type of iteration for iterator variable derivation.
type IteratorKind int

const (
	IterateIndexed IteratorKind = iota // ipairs-style: (integer, element)
	IterateKeyed                       // pairs-style: (key, value)
)

// AssignmentSourceKind names the canonical RHS evidence source for an assignment.
type AssignmentSourceKind uint8

const (
	// AssignmentSourceStatic means the assignment uses the pre-extracted Type.
	AssignmentSourceStatic AssignmentSourceKind = iota
	// AssignmentSourcePath reads another flow path at the assignment point.
	AssignmentSourcePath
	// AssignmentSourceIterator derives a for-loop variable from its iterator source.
	AssignmentSourceIterator
	// AssignmentSourceContainerElement derives a returned container element.
	AssignmentSourceContainerElement
	// AssignmentSourceMapElement derives a dynamic table index read.
	AssignmentSourceMapElement
	// AssignmentSourceLengthIndex derives a t[#t + offset] read under length proof.
	AssignmentSourceLengthIndex
	// AssignmentSourceCallReturn derives a call return from a flow-resolved callee.
	AssignmentSourceCallReturn
	// AssignmentSourceOperator derives an arithmetic, relational, concatenation,
	// or unary operator result from solve-time operand types. Operands that are
	// flow paths re-read their narrowed type at the assignment point; other
	// operands carry their static extraction type. This lets an operator result
	// over a guard-narrowed operand (for example data.amount + 1 where the guard
	// proves data.amount is a number) recover its type after the operand narrows,
	// instead of freezing the extraction-time unknown into the target.
	AssignmentSourceOperator
)

// OperatorOperand is one operand of an AssignmentSourceOperator. When Path has a
// symbol it re-reads the narrowed operand type at the assignment point; otherwise
// Static carries the extraction-time operand type for non-path operands such as
// literals or call results.
type OperatorOperand struct {
	Path   constraint.Path
	Static typ.Type
}

// AssignmentSourceProjectionKind classifies same-source projection evidence
// attached to an assignment source.
type AssignmentSourceProjectionKind uint8

const (
	// AssignmentSourceProjectionNone means transfer should use only the source
	// algebra and target static type.
	AssignmentSourceProjectionNone AssignmentSourceProjectionKind = iota
	// AssignmentSourceProjectionCallable is a callpoint function-value
	// projection, e.g. a graph-local wrapper whose captures are visible at the
	// assignment point.
	AssignmentSourceProjectionCallable
	// AssignmentSourceProjectionCallReturn is a callpoint return-slot
	// projection from the same call expression.
	AssignmentSourceProjectionCallReturn
)

// AssignmentSource is the flow-owned RHS source algebra for assignments.
//
// AST extraction lowers syntax into exactly one source. Transfer evaluates that
// source against the current abstract state, so new solve-time facts do not grow
// extra fields on UnifiedAssignment.
type AssignmentSource struct {
	Kind AssignmentSourceKind

	// ProjectedType is source-owned expression evidence for higher-order values
	// whose meaning depends on callpoint/capture state that a plain path read
	// cannot encode. It is never target annotation evidence.
	ProjectionKind AssignmentSourceProjectionKind
	ProjectedType  typ.Type

	Path constraint.Path

	IteratorKind IteratorKind
	VarIndex     int

	ContainerPath constraint.Path
	MapPath       constraint.Path
	KeySymbol     cfg.SymbolID
	KeyVar        string
	Offset        int64

	CalleePath   constraint.Path
	ReceiverPath constraint.Path
	Method       string
	ReturnIndex  int

	// Operator is the operator token for AssignmentSourceOperator. Operands holds
	// its operand evidence in source order.
	Operator string
	Operands []OperatorOperand
}

func (s AssignmentSource) IsZero() bool {
	return s.Kind == AssignmentSourceStatic
}

func (s AssignmentSource) HasPath() bool {
	return s.Kind == AssignmentSourcePath && s.Path.HasSymbol()
}

// UnifiedAssignment describes an assignment in the CFG with typed info.
type UnifiedAssignment struct {
	Point      cfg.Point
	TargetPath constraint.Path
	Type       typ.Type
	Source     AssignmentSource
}

// EdgeCondition ties a DNF condition to a control-flow edge.
type EdgeCondition struct {
	From      cfg.Point
	To        cfg.Point
	Condition constraint.Condition
}

// EdgeNumericConstraint ties numeric constraints to a control-flow edge.
type EdgeNumericConstraint struct {
	From        cfg.Point
	To          cfg.Point
	Constraints []constraint.NumericConstraint
}

// TypeDecomposer extracts element, key, and value types from composite types.
// Used by current transfer for iterator and container element derivation without
// depending on query/core.
type TypeDecomposer interface {
	ElementType(t typ.Type) typ.Type
	KeyType(t typ.Type) typ.Type
	ValueType(t typ.Type) typ.Type
	EntryValueType(t typ.Type) typ.Type
}

// Inputs bundles all AST-free data needed by the flow engine.
// Inputs are AST-free and deterministic, enabling serialization, caching,
// and reproducible analysis across sessions.
type Inputs struct {
	// Graph provides CFG, SSA versioning, and symbol scope information.
	Graph cfg.VersionedGraph

	// Decomposer extracts element/key/value types from composite types.
	Decomposer TypeDecomposer

	// DeclaredTypes maps SymbolID to declared type.
	// All per-variable type data is keyed by SymbolID for proper scope handling.
	DeclaredTypes map[cfg.SymbolID]typ.Type

	// AnnotatedVars tracks variables with explicit type annotations.
	AnnotatedVars map[cfg.SymbolID]bool

	Assignments    []UnifiedAssignment
	ConstValues    map[cfg.SymbolID]map[cfg.Point]*ConstValue
	EdgeConditions []EdgeCondition

	// EdgeNumericConstraints stores numeric constraints from comparison operators.
	// These are fed to theory solvers to detect contradictions.
	EdgeNumericConstraints []EdgeNumericConstraint

	// Type key resolution for HasType constraints.
	TypeKeys map[uint64]typ.Type

	// ReturnKinds classifies return points (true/false/unknown).
	// Populated by checker which has AST access.
	ReturnKinds map[cfg.Point]ReturnKind

	// ReturnConstraints stores constraints extracted from return expressions.
	// For predicate functions, the return expression encodes the constraint.
	// Example: "return type(x) == string" gives OnTrue: HasType{x, string}
	ReturnConstraints map[cfg.Point]ReturnExprConstraints

	// PredicateLinks tracks variables assigned from predicate calls.
	// Keyed by the graph symbol and definition point; value contains constraints
	// to apply when the variable is truthy/falsy.
	// Example: local _, err = Point:is(data) -> err nil implies HasType{data, Point}
	PredicateLinks map[PredicateLinkKey]PredicateLink

	// SiblingAssignments tracks variables assigned from the same multi-return call.
	// Key is "varname@defpoint", maps to the sibling group.
	// Used for error return pattern where checking err narrows result.
	SiblingAssignments map[SiblingKey]*SiblingAssignment

	// VariantFieldOrigins records path-origin evidence for variants produced by
	// effectful calls. For example, a select result can state that result.channel
	// aliases timeout in origin family F, case 1. Branch extraction uses this fact
	// to turn runtime identity tests into first-class variant-case constraints.
	VariantFieldOrigins []VariantFieldOrigin

	// ArrayLiteralLengths tracks sequence constructors that establish a length
	// lower bound for their target at the construction point. A literal {e1..eN}
	// with N positional elements proves #target >= N flow-sensitively, even when
	// the declared type ({number}) erases the static length.
	ArrayLiteralLengths []ArrayLiteralLength

	// LoopInsertLengths tracks constant-trip-count numeric for-loops whose body
	// performs exactly one unconditional non-nil table.insert per iteration into a
	// target sequence. The loop exit then proves #target >= Count, where Count is
	// the loop trip count. Unlike ArrayLiteralLength, the bound is raised (not
	// reset), since it composes with any pre-loop length the target already holds.
	LoopInsertLengths []LoopInsertLength

	// DeadPoints marks CFG points that are unreachable.
	// Used when a terminating function (one that never returns) is called.
	DeadPoints map[cfg.Point]bool

	// ModuleAliases maps symbol IDs to module paths from require() assignments.
	// Example: local http = require("http_client") -> sym(http) -> "http_client"
	ModuleAliases map[cfg.SymbolID]string

	// FunctionAliases maps local symbols to their source function symbols.
	// For patterns like `local f = B`, maps sym(f) -> sym(B).
	// Used for effect propagation through aliases.
	FunctionAliases map[cfg.SymbolID]cfg.SymbolID

	// SiblingTypes provides captured variable types from parent scope.
	// Explicit overlay, never merged into DeclaredTypes.
	SiblingTypes map[cfg.SymbolID]typ.Type

	// LiteralTypes provides function literal types synthesized during literal analysis.
	// Explicit overlay, never merged into DeclaredTypes.
	LiteralTypes map[cfg.SymbolID]typ.Type

	// BindingTypes provides immutable value-binding types, such as the canonical
	// signature of a named/local function binding. These are definition facts, not
	// source annotations, and must never be merged into DeclaredTypes.
	BindingTypes map[cfg.SymbolID]typ.Type

	// KeysProvenance tracks variables that contain keys of another table.
	// Key: symbol of variable holding keys (e.g., suite_names)
	// Value: symbol of table the keys came from (e.g., suites)
	// Used to emit KeyOf constraints when iterating over such variables.
	KeysProvenance map[cfg.SymbolID]cfg.SymbolID

	// ConditionExtraReads records access paths read at a CFG point that the flow
	// inputs do not otherwise encode — notably return-expression reads and call
	// argument reads. The condition-demand builder folds these into the liveness
	// use set so a field guard feeding a return summary is not forgotten.
	ConditionExtraReads map[cfg.Point][]constraint.Path
}

// ReturnExprConstraints holds constraints extracted from a return expression.
//
// OnReturn facts hold whenever the function returns normally from this
// expression. OnTrue and OnFalse facts are correlated with the truthiness of the
// returned value at callers.
type ReturnExprConstraints struct {
	OnReturn constraint.Condition
	OnTrue   constraint.Condition
	OnFalse  constraint.Condition
}

// PredicateLinkKey identifies a variable assigned from a predicate call.
//
// Symbol is the graph-local variable identity; DefPoint is the assignment point
// where that variable received the predicate result. This is analysis-facing
// evidence, so it uses typed identities instead of the old "name@point" string
// encoding.
type PredicateLinkKey struct {
	Symbol   cfg.SymbolID
	DefPoint cfg.Point
}

// PredicateLink stores predicate constraints for a variable assigned from a predicate call.
// Example: local ok = Point:is(data) -> OnTruthy contains HasType{data, Point}
type PredicateLink struct {
	OnTruthy constraint.Condition
	OnFalsy  constraint.Condition
}

// ReturnCorrelation describes a correlated (value, error) pair in a multi-return.
// Derived from effect.ErrorReturn on the callee's spec.
type ReturnCorrelation struct {
	ValueIndex int
	ErrorIndex int
}

// GuardedTypeCorrelation describes branch-sensitive sibling narrowing:
// when guard return at GuardIndex is truthy/falsy (per GuardOnTruthy),
// target return at TargetIndex narrows to TargetType.
type GuardedTypeCorrelation struct {
	GuardIndex    int
	TargetIndex   int
	GuardOnTruthy bool
	TargetType    typ.Type
}

// SiblingAssignment tracks variables assigned from the same multi-return call.
// Used for error return pattern: `local result, err = call()` where checking err narrows result.
type SiblingAssignment struct {
	Symbols             []cfg.SymbolID           // Symbol IDs in order (primary identity)
	Names               []string                 // Variable names (for constraint path construction)
	Types               []typ.Type               // Declared types for each variable
	Correlations        []ReturnCorrelation      // Inverse correlations (ErrorReturn): value nil <-> error non-nil
	CoCorrelations      []ReturnCorrelation      // Same-direction correlations (CorrelatedReturn): all nil or all non-nil
	GuardedCorrelations []GuardedTypeCorrelation // Branch-sensitive type narrowing from guard/result relations
}

// SiblingKey uniquely identifies a variable in a sibling assignment by SymbolID+SSA version.
type SiblingKey struct {
	Symbol    cfg.SymbolID
	VersionID int
}

// VariantFieldOrigin links a variant field value to the source path it aliases
// through a first-class finite provenance family/case axis.
type VariantFieldOrigin struct {
	Target       constraint.Path
	Field        string
	Source       constraint.Path
	OriginFamily uint64
	CaseIndex    int
}

// IndexWriteReadQuery identifies a solved dynamic-index readback proof with
// explicit solver context and normalized address-domain admission evidence.
// TargetPath and KeyPath remain as provenance views for reducers that derive
// readback from key-array/value-origin facts instead of the heavy admission lane.
type IndexWriteReadQuery struct {
	Point      cfg.Point
	View       PathReadView
	Admission  IndexWriteAddressQuery
	TargetPath constraint.Path
	KeyPath    constraint.Path
}

// IndexWriteReadQueryFromPaths normalizes source paths at the caller boundary
// into the address-domain query consumed by flow facts.
func IndexWriteReadQueryFromPaths(
	point cfg.Point,
	view PathReadView,
	target constraint.Path,
	keyPath constraint.Path,
	keyType typ.Type,
	valuePath constraint.Path,
) (IndexWriteReadQuery, bool) {
	targetAddr, ok := StableAddressOfPath(target)
	if !ok {
		return IndexWriteReadQuery{}, false
	}
	query := IndexWriteReadQuery{
		Point:      point,
		View:       view,
		Admission:  IndexWriteAddressQuery{Target: targetAddr},
		TargetPath: target,
		KeyPath:    keyPath,
	}
	if !keyPath.IsEmpty() {
		keyAddr, ok := StableAddressOfPath(keyPath)
		if ok {
			query.Admission.KeyPath = keyAddr
			query.Admission.HasKeyPath = true
		}
	}
	if !valuePath.IsEmpty() {
		valueAddr, ok := StableAddressOfPath(valuePath)
		if ok {
			query.Admission.ValuePath = valueAddr
			query.Admission.HasValuePath = true
		}
	}
	if !typ.IsAbsentOrUnknown(keyType) {
		query.Admission.KeyValue = product.FromType(keyType)
	}
	return query, true
}

// ArrayLiteralLength records a sequence constructor's proven length lower bound.
// Count is the number of leading positional elements whose count is statically
// certain (a trailing multi-value element such as a call or vararg is excluded,
// since it may expand to zero values). The numeric component seeds #Target >= Count
// at Point, the same length-proof channel table.insert raises.
type ArrayLiteralLength struct {
	Point  cfg.Point
	Target constraint.Path
	Count  int64
}

// LoopInsertLength records a loop's proven append count for a target sequence,
// established at the loop exit Point.
//
// A constant-trip-count numeric for-loop fixes Count, the number of times the
// body's single unconditional non-nil append runs; the numeric component raises
// #Target >= Count, which composes with any pre-loop length lower bound.
//
// A pairs(Source) loop instead ties the count to the iterated map's key
// cardinality: each iteration appends exactly once and pairs visits every entry
// once, so #Target >= (key cardinality of Source) at exit. Source carries the
// iterated map path; Count is 0 for this relational form, since the bound is the
// source's cardinality rather than a constant. The post-flow producer reads a
// returned slot whose accumulator has a Source path equal to a parameter and
// emits the relational return-length postcondition len(ret) >= len(param).
type LoopInsertLength struct {
	Point  cfg.Point
	Target constraint.Path
	Count  int64
	Source constraint.Path
}

// ValueTemplate records flow-owned source slots inside an extracted value.
//
// Extraction lowers syntax to a static ValueType plus these AST-free source
// slots. Transfer evaluates the slots against the current abstract state, so
// table literals embedded in mutator calls do not freeze pre-solve types for
// fields that come from locals, parameters, or other flow paths.
type ValueTemplate struct {
	Slots []ValueTemplateSlot
}

type ValueTemplateSlot struct {
	Segments []constraint.Segment
	Source   AssignmentSource
}

// TypeState tracks the resolution progress of a type during fixed-point iteration.
// The solver uses this to detect convergence and handle cyclic dependencies.
type TypeState uint8

const (
	StateUnknown    TypeState = iota // Type not yet resolved
	StateResolved                    // Type fully resolved
	StatePending                     // Type resolution in progress
	StateConflicted                  // Conflicting type assignments
)

// TypedValue pairs a type with its resolution state.
type TypedValue struct {
	Type  typ.Type
	State TypeState
}

// SymbolTypes is the per-point symbol type map.
type SymbolTypes = map[cfg.Point]map[cfg.SymbolID]TypedValue
