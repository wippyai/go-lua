// Package flow provides control flow type propagation and flow-sensitive analysis.
//
// Flow analysis computes the types of variables at each program point by propagating
// type information through the control flow graph. Unlike simple declared types,
// flow types account for assignments, conditionals, and narrowing constraints.
//
// # Usage
//
// Build an Inputs struct from the CFG, declared types, and extracted constraints,
// then call Solve to compute the flow solution:
//
//	inputs := &flow.Inputs{
//		Graph:         cfg,
//		DeclaredTypes: declaredTypes,
//		Assignments:   assignments,
//		EdgeConditions: edgeConditions,
//		TypeKeys:      typeKeys,
//		Decomposer:    decomposer,
//	}
//	solution := flow.Solve(inputs, resolver)
//
// Query the solution for types at specific program points:
//
//	t := solution.TypeAt(point, path)           // Base type lookup
//	narrowed := solution.NarrowedTypeAt(point, path)  // Type with narrowing applied
//	cond := solution.ConditionAt(point)         // Active constraints
//
// # Core Concepts
//
// Inputs: All data needed by the flow solver, including CFG, declared types,
// assignments, conditions, and constraint metadata. Inputs are AST-free and
// deterministic for caching and incremental analysis.
//
// Solution: The result of flow analysis, mapping versioned path keys to their
// narrowed types. Query methods apply point conditions to refine types further.
//
// TypeState: Tracks whether a type at a program point is resolved, pending,
// or conflicted. Used for fixed-point iteration during solving.
//
// SymbolTypes: Maps each CFG point to its per-symbol types, representing the
// complete type state at that point in the program.
//
// # Special Assignments
//
// The solver handles several special assignment patterns:
//
//   - AssignmentSource: Derives RHS values from paths, iterators, calls, map reads,
//     length-index reads, and container element returns
//   - SiblingAssignment: Correlates multi-return values (result, err patterns)
//
// # Subpackages
//
// The flow package has several subpackages:
//
//   - domain: Abstract domain interfaces for constraint solving
//   - join: Type joining operations for phi node merging
//   - numeric: Numeric range and interval tracking
//   - pathkey: Canonical path key resolution for SSA versions
//   - propagate: Condition propagation through the CFG
//
// # Integration
//
// The flow package is used by the type checker to get refined types at each
// program point. The solver is invoked after CFG construction and constraint
// extraction, producing SymbolTypes consumed by narrowing and error reporting.
package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
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
// Used by the flow solver for iterator and container element derivation
// without depending on query/core.
type TypeDecomposer interface {
	ElementType(t typ.Type) typ.Type
	KeyType(t typ.Type) typ.Type
	ValueType(t typ.Type) typ.Type
	EntryValueType(t typ.Type) typ.Type
}

// Inputs bundles all data needed by the flow solver for type propagation.
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
	// Key is "varname@defpoint", value contains constraints to apply when var is truthy/falsy.
	// Example: local _, err = Point:is(data) -> err nil implies HasType{data, Point}
	PredicateLinks map[string]PredicateLink

	// SiblingAssignments tracks variables assigned from the same multi-return call.
	// Key is "varname@defpoint", maps to the sibling group.
	// Used for error return pattern where checking err narrows result.
	SiblingAssignments map[SiblingKey]*SiblingAssignment

	// VariantFieldOrigins records path-origin evidence for discriminated
	// variants produced by effectful calls. For example, a select result variant
	// can state that result.channel aliases timeout and is identified by
	// result.__select_case_id == 1. Branch extraction uses this product to turn
	// runtime identity tests into ordinary discriminator constraints.
	VariantFieldOrigins []VariantFieldOrigin

	// MapMutatorAssignments tracks Lua map writes such as t[k] = v.
	// Direct syntax and replayed interprocedural captured effects both lower to
	// this operator before transfer applies the indexed-write domain law.
	MapMutatorAssignments []MapMutatorAssignment

	// TableMutatorAssignments tracks table.insert-like mutations that widen
	// array element types (including map values that are arrays).
	TableMutatorAssignments []TableMutatorAssignment

	// ContainerMutatorAssignments tracks container mutations (e.g., channel.send)
	// that widen element types via ContainerElementUnion effects.
	ContainerMutatorAssignments []ContainerMutatorAssignment

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

	// LiteralTypes provides function literal types synthesized in the literal phase.
	// Explicit overlay, never merged into DeclaredTypes.
	LiteralTypes map[cfg.SymbolID]typ.Type

	// KeysProvenance tracks variables that contain keys of another table.
	// Key: symbol of variable holding keys (e.g., suite_names)
	// Value: symbol of table the keys came from (e.g., suites)
	// Used to emit KeyOf constraints when iterating over such variables.
	KeysProvenance map[cfg.SymbolID]cfg.SymbolID
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

// VariantFieldOrigin links a variant field value to the source path it aliases.
// Target.Field aliases Source when Target.DiscriminatorField equals
// DiscriminatorValue. The abstract interpreter owns this relational evidence;
// the constraint solver consumes the lowered discriminator constraints.
type VariantFieldOrigin struct {
	Target             constraint.Path
	Field              string
	Source             constraint.Path
	DiscriminatorField string
	DiscriminatorValue *typ.Literal
}

// MapMutationValueMode describes how a map mutation affects the value slot.
type MapMutationValueMode uint8

const (
	// MapMutationValueWrite models t[k] = v: the observed slot value can be v.
	MapMutationValueWrite MapMutationValueMode = iota
	// MapMutationValueUpdate models t[k].field = v: v updates the existing
	// slot shape instead of replacing the whole slot.
	MapMutationValueUpdate
)

// MapMutatorAssignment describes a Lua map write t[k] = v.
// KeyVar stores the variable name if key is an identifier; KeyType is resolved
// during flow solving when flow-narrowed key evidence is available.
type MapMutatorAssignment struct {
	Point     cfg.Point
	Target    constraint.Path // Map path being mutated
	ValueMode MapMutationValueMode
	KeyVar    string          // Variable name if key is an identifier
	KeySymbol cfg.SymbolID    // Symbol ID for the key variable (for SSA-aware lookup)
	KeyType   typ.Type        // Optional explicit key type (overrides KeySymbol lookup)
	ValuePath constraint.Path // Path to value expression for flow-resolved type lookup
	ValueType typ.Type        // Static value type if ValuePath doesn't resolve
	Value     ValueTemplate   // Flow-resolved slots inside a static table value
}

// IndexWriteQuery identifies the solved transfer proof for a dynamic index
// write. The query is AST-free; syntax lowering records MapMutatorAssignment
// products, and diagnostics/projectors consume this product instead of
// reconstructing write provenance.
type IndexWriteQuery struct {
	Point     cfg.Point
	Target    constraint.Path
	KeySymbol cfg.SymbolID
	KeyType   typ.Type
	ValuePath constraint.Path
}

// TableMutatorAssignment describes table.insert-like mutations that widen
// array element types. If KeySymbol/KeyType is set, the target is treated as
// a map index (e.g., suites[suite]) and the map value's array element type is widened.
type TableMutatorAssignment struct {
	Point     cfg.Point
	Target    constraint.Path // Base table path (or base map path for indexed targets)
	KeyVar    string          // Variable name if key is an identifier
	KeySymbol cfg.SymbolID    // Symbol ID for the key variable (for SSA-aware lookup)
	KeyType   typ.Type        // Optional explicit key type (overrides KeySymbol lookup)
	ValuePath constraint.Path // Path to value expression for flow-resolved type lookup
	ValueType typ.Type        // Static value type if ValuePath doesn't resolve
	Value     ValueTemplate   // Flow-resolved slots inside a static table value
}

// ContainerMutatorAssignment describes container mutations (channel.send, etc.)
// that widen element types. Uses the ContainerElementUnion effect pattern from specs.
type ContainerMutatorAssignment struct {
	Point     cfg.Point
	Target    constraint.Path // Container path (symbol-only, e.g., channel variable)
	ValuePath constraint.Path // Path to value expression for flow-resolved type lookup
	ValueType typ.Type        // Static value type if ValuePath doesn't resolve
	Value     ValueTemplate   // Flow-resolved slots inside a static table value
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
