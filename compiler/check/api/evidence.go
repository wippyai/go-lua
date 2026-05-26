package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// FlowEvidence is the abstract-interpreter event stream that later checker
// phases reduce with solved, narrowed expression types.
type FlowEvidence struct {
	Calls               []CallEvidence
	Returns             []ReturnEvidence
	Assignments         []AssignmentEvidence
	Branches            []BranchEvidence
	NormalExit          NormalExitEvidence
	IdentifierUses      []IdentifierUseEvidence
	FieldDefaults       []FieldDefaultEvidence
	ParameterUses       []ParameterUseEvidence
	FreshTableLiterals  []FreshTableLiteralEvidence
	FunctionDefinitions []FunctionDefinitionEvidence
	EscapedFunctions    []FunctionEscapeEvidence
	LocalTypePredicates []LocalTypePredicateEvidence
	CapturedFields      []CapturedFieldEvidence
	CapturedContainers  []CapturedContainerEvidence
}

// IsZero reports whether no abstract-interpreter event evidence has been
// materialized for a graph.
func (e FlowEvidence) IsZero() bool {
	return len(e.Calls) == 0 &&
		len(e.Returns) == 0 &&
		len(e.Assignments) == 0 &&
		len(e.Branches) == 0 &&
		!e.NormalExit.Valid &&
		len(e.IdentifierUses) == 0 &&
		len(e.FieldDefaults) == 0 &&
		len(e.ParameterUses) == 0 &&
		len(e.FreshTableLiterals) == 0 &&
		len(e.FunctionDefinitions) == 0 &&
		len(e.EscapedFunctions) == 0 &&
		len(e.LocalTypePredicates) == 0 &&
		len(e.CapturedFields) == 0 &&
		len(e.CapturedContainers) == 0
}

// CallOrigin classifies the graph event that owns a call expression.
type CallOrigin uint8

const (
	CallOriginExpression CallOrigin = iota
	CallOriginStatement
	CallOriginAssignment
	CallOriginReturn
	CallOriginBranch
)

// CallEvidence records a call site discovered by the abstract interpreter.
type CallEvidence struct {
	Point            cfg.Point
	Info             *cfg.CallInfo
	Origin           CallOrigin
	CalleeType       typ.Type
	ExpectedArgs     []typ.Type
	ExpectedVariadic typ.Type
}

// ExpectedArgType returns the contextual type inferred for argument idx.
func (e CallEvidence) ExpectedArgType(idx int) typ.Type {
	if idx < 0 {
		return nil
	}
	if idx < len(e.ExpectedArgs) {
		return e.ExpectedArgs[idx]
	}
	return e.ExpectedVariadic
}

// ReturnEvidence records a return point discovered by the abstract interpreter.
type ReturnEvidence struct {
	Point cfg.Point
	Info  *cfg.ReturnInfo
}

// AssignmentEvidence records an assignment point discovered by the abstract
// interpreter.
type AssignmentEvidence struct {
	Point cfg.Point
	Info  *cfg.AssignInfo
}

// BranchEvidence records a branch point discovered by the abstract interpreter.
type BranchEvidence struct {
	Point cfg.Point
	Info  *cfg.BranchInfo
}

// NormalExitEvidence records the graph exit point that represents implicit
// normal return from the function body.
type NormalExitEvidence struct {
	Point cfg.Point
	Valid bool
}

// IdentifierUseEvidence records an identifier expression read by one graph
// event. Definition targets are not uses; assignment sources, call operands,
// return expressions, branch conditions, and structured assignment bases/keys
// are.
type IdentifierUseEvidence struct {
	Point cfg.Point
	Expr  *ast.IdentExpr
}

// FieldDefaultEvidence records an `x.field or default` expression.
type FieldDefaultEvidence struct {
	Point  cfg.Point
	Target cfg.SymbolID
	Field  string
	Value  ast.Expr
}

// FreshTableLiteralEvidence records that at Point, Symbol's visible value is a
// fresh table literal assigned at AssignmentPoint and not yet exposed through
// an alias, call, return, function definition, or structured mutation along the
// unique predecessor chain.
type FreshTableLiteralEvidence struct {
	Point           cfg.Point
	Symbol          cfg.SymbolID
	Version         cfg.Version
	Table           *ast.TableExpr
	AssignmentPoint cfg.Point
}

// FunctionDefinitionEvidence records a nested function definition and its
// resolved source-level identity.
type FunctionDefinitionEvidence struct {
	Nested  cfg.NestedFunc
	FuncDef *cfg.FuncDefInfo
	Name    string
	Symbol  cfg.SymbolID
	IsLocal bool
}

// FunctionEscapeEvidence records a local function value published through a
// global/field/indexed position that can run outside the defining graph.
type FunctionEscapeEvidence struct {
	Point  cfg.Point
	Symbol cfg.SymbolID
}

// LocalTypePredicateEvidence records a local function that returns a builtin
// runtime type predicate for one of its parameters.
type LocalTypePredicateEvidence struct {
	Symbol     cfg.SymbolID
	ParamName  string
	ParamIndex int
	Kind       string
}

// ParameterUseEvidence records how a function body demands one parameter.
type ParameterUseEvidence struct {
	Symbol cfg.SymbolID
	Whole  bool
	Fields []string
}

// CapturedFieldEvidence records a direct field write to a captured symbol.
type CapturedFieldEvidence struct {
	Point       cfg.Point
	Target      cfg.SymbolID
	Field       string
	TargetPath  constraint.Path
	Value       ast.Expr
	ValueType   typ.Type
	ValueSource flow.AssignmentSource
}

// CapturedContainerEvidence records an element mutation on a captured symbol.
type CapturedContainerEvidence struct {
	Point         cfg.Point
	Target        cfg.SymbolID
	Segments      []constraint.Segment
	KeyPath       constraint.Path
	KeyType       typ.Type
	ValueMode     flow.MapMutationValueMode
	ValuePath     constraint.Path
	ValueType     typ.Type
	ValueTemplate flow.ValueTemplate
	Kind          ContainerMutationKind
}
