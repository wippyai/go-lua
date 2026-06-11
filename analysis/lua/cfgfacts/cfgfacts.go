// Package cfgfacts stores Lua sidecar facts for CFG points.
package cfgfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/valuesource"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// LoopKind identifies the structural loop form associated with a CFG point.
type LoopKind uint8

// Loop kind constants represent recognizable loop shapes.
const (
	LoopKindUnknown LoopKind = iota
	LoopKindConditional
	LoopKindNumericFor
	LoopKindGenericFor
)

// TypeDefinitionKind identifies the declaration form associated with a CFG point.
type TypeDefinitionKind uint8

// Type definition kinds describe the concrete declaration form.
const (
	TypeDefinitionUnknown TypeDefinitionKind = iota
	TypeDefinitionAlias
	TypeDefinitionInterface
)

// TypeDefinitionFact describes a type declaration associated with a CFG point.
type TypeDefinitionFact struct {
	Kind TypeDefinitionKind

	Stmt      ast.Stmt
	Type      *ast.TypeDefStmt
	Interface *ast.InterfaceDefStmt
}

// FunctionDefinitionFact describes a function declaration associated with a CFG point.
type FunctionDefinitionFact struct {
	Stmt *ast.FuncDefStmt
	Name *ast.FuncName
	Func *ast.FunctionExpr

	TargetSymbol    symbol.ID
	HasTargetSymbol bool
}

// NumericForRole identifies the structural numeric-for position at a CFG point.
type NumericForRole uint8

// Numeric for roles identify the init/check positions in a numeric-for loop.
const (
	NumericForRoleInit NumericForRole = iota + 1
	NumericForRoleCheck
)

// NumericForFact describes a numeric-for loop point.
type NumericForFact struct {
	Stmt *ast.NumberForStmt
	Role NumericForRole

	Name  string
	Init  ast.Expr
	Limit ast.Expr
	Step  ast.Expr

	Symbol    symbol.ID
	HasSymbol bool
}

// GenericForRole identifies the structural generic-for position at a CFG point.
type GenericForRole uint8

// Generic for roles identify the check/variable positions in a generic-for loop.
const (
	GenericForRoleCheck GenericForRole = iota + 1
	GenericForRoleVariable
)

// NoGenericForVariableIndex marks the check point in a generic-for loop.
const NoGenericForVariableIndex = -1

// GenericForFact describes a generic-for loop point.
type GenericForFact struct {
	Stmt *ast.GenericForStmt
	Role GenericForRole

	Names   []string
	Exprs   []ast.Expr
	Sources []valuesource.Source

	Symbols    []symbol.ID
	HasSymbols bool

	VariableIndex int
}

// LabelFact describes a label point.
type LabelFact struct {
	Stmt *ast.LabelStmt
	Name string
}

// GotoFact describes a goto point.
type GotoFact struct {
	Stmt  *ast.GotoStmt
	Label string
}

// AssignmentFact describes an assignment target.
type AssignmentFact struct {
	Target symbol.ID
}

// LoopFact describes loop structure associated with a CFG point.
type LoopFact struct {
	Kind                 LoopKind
	Vars                 []symbol.ID
	Locals               []symbol.ID
	DirectModifiedOuters []symbol.ID
	Preheader            cfg.Point
	HasPreheader         bool
}

// Metadata stores Lua sidecar facts keyed by CFG point.
type Metadata struct {
	assignments     map[cfg.Point]AssignmentFact
	loops           map[cfg.Point]LoopFact
	typeDefinitions map[cfg.Point]TypeDefinitionFact
	functionDefs    map[cfg.Point]FunctionDefinitionFact
	numericFors     map[cfg.Point]NumericForFact
	genericFors     map[cfg.Point]GenericForFact
	labels          map[cfg.Point]LabelFact
	gotos           map[cfg.Point]GotoFact
}

// Assignment returns the assignment fact for point.
func (m Metadata) Assignment(point cfg.Point) (AssignmentFact, bool) {
	fact, ok := m.assignments[point]
	return fact, ok
}

// SetAssignment records an assignment fact for point.
func (m *Metadata) SetAssignment(point cfg.Point, fact AssignmentFact) {
	if m.assignments == nil {
		m.assignments = make(map[cfg.Point]AssignmentFact)
	}
	m.assignments[point] = fact
}

// Loop returns the loop fact for point.
func (m Metadata) Loop(point cfg.Point) (LoopFact, bool) {
	fact, ok := m.loops[point]
	if !ok {
		return LoopFact{}, false
	}
	return copyLoopFact(fact), true
}

// SetLoop records a loop fact for point.
func (m *Metadata) SetLoop(point cfg.Point, fact LoopFact) {
	if m.loops == nil {
		m.loops = make(map[cfg.Point]LoopFact)
	}
	m.loops[point] = copyLoopFact(fact)
}

// TypeDefinition returns the type-definition fact for point.
func (m Metadata) TypeDefinition(point cfg.Point) (TypeDefinitionFact, bool) {
	fact, ok := m.typeDefinitions[point]
	return fact, ok
}

// SetTypeDefinition records a type-definition fact for point.
func (m *Metadata) SetTypeDefinition(point cfg.Point, fact TypeDefinitionFact) {
	if m.typeDefinitions == nil {
		m.typeDefinitions = make(map[cfg.Point]TypeDefinitionFact)
	}
	m.typeDefinitions[point] = fact
}

// FunctionDefinition returns the function-definition fact for point.
func (m Metadata) FunctionDefinition(point cfg.Point) (FunctionDefinitionFact, bool) {
	fact, ok := m.functionDefs[point]
	return fact, ok
}

// SetFunctionDefinition records a function-definition fact for point.
func (m *Metadata) SetFunctionDefinition(point cfg.Point, fact FunctionDefinitionFact) {
	if m.functionDefs == nil {
		m.functionDefs = make(map[cfg.Point]FunctionDefinitionFact)
	}
	m.functionDefs[point] = fact
}

// NumericFor returns the numeric-for fact for point.
func (m Metadata) NumericFor(point cfg.Point) (NumericForFact, bool) {
	fact, ok := m.numericFors[point]
	return fact, ok
}

// SetNumericFor records a numeric-for fact for point.
func (m *Metadata) SetNumericFor(point cfg.Point, fact NumericForFact) {
	if m.numericFors == nil {
		m.numericFors = make(map[cfg.Point]NumericForFact)
	}
	m.numericFors[point] = fact
}

// GenericFor returns the generic-for fact for point.
func (m Metadata) GenericFor(point cfg.Point) (GenericForFact, bool) {
	fact, ok := m.genericFors[point]
	if !ok {
		return GenericForFact{}, false
	}
	return copyGenericForFact(fact), true
}

// SetGenericFor records a generic-for fact for point.
func (m *Metadata) SetGenericFor(point cfg.Point, fact GenericForFact) {
	if m.genericFors == nil {
		m.genericFors = make(map[cfg.Point]GenericForFact)
	}
	m.genericFors[point] = copyGenericForFact(fact)
}

// Label returns the label fact for point.
func (m Metadata) Label(point cfg.Point) (LabelFact, bool) {
	fact, ok := m.labels[point]
	return fact, ok
}

// SetLabel records a label fact for point.
func (m *Metadata) SetLabel(point cfg.Point, fact LabelFact) {
	if m.labels == nil {
		m.labels = make(map[cfg.Point]LabelFact)
	}
	m.labels[point] = fact
}

// Goto returns the goto fact for point.
func (m Metadata) Goto(point cfg.Point) (GotoFact, bool) {
	fact, ok := m.gotos[point]
	return fact, ok
}

// SetGoto records a goto fact for point.
func (m *Metadata) SetGoto(point cfg.Point, fact GotoFact) {
	if m.gotos == nil {
		m.gotos = make(map[cfg.Point]GotoFact)
	}
	m.gotos[point] = fact
}

func copyLoopFact(fact LoopFact) LoopFact {
	fact.Vars = append([]symbol.ID(nil), fact.Vars...)
	fact.Locals = append([]symbol.ID(nil), fact.Locals...)
	fact.DirectModifiedOuters = append([]symbol.ID(nil), fact.DirectModifiedOuters...)
	return fact
}

func copyGenericForFact(fact GenericForFact) GenericForFact {
	fact.Names = append([]string(nil), fact.Names...)
	fact.Exprs = append([]ast.Expr(nil), fact.Exprs...)
	fact.Sources = append([]valuesource.Source(nil), fact.Sources...)
	fact.Symbols = append([]symbol.ID(nil), fact.Symbols...)
	return fact
}
