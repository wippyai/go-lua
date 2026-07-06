package semantics

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type BranchKind uint8

const (
	BranchUnknown BranchKind = iota
	BranchIf
	BranchWhile
	BranchRepeat
	BranchShortCircuit
)

type CallContextKind uint8

const (
	CallContextUnknown CallContextKind = iota
	CallContextStatement
	CallContextAssignmentSource
	CallContextReturnSource
	CallContextIteratorSource
	CallContextCondition
	CallContextExpressionProducer
)

type CallResultTargetKind uint8

const (
	CallResultTargetUnknown CallResultTargetKind = iota
	CallResultTargetLocalAssignment
	CallResultTargetOrdinaryAssignment
	CallResultTargetReturn
	CallResultTargetExpression
)

const NoCallResultIndex = -1

type LocalAssignmentFact struct {
	Stmt  *ast.LocalAssignStmt
	Index int

	Name   string
	Type   ast.TypeExpr
	Expr   ast.Expr
	Source sourceprovenance.ASTSource

	Symbol    symbol.ID
	HasSymbol bool

	Exprs []ast.Expr
	Types []ast.TypeExpr
}

type OrdinaryAssignmentFact struct {
	Stmt  *ast.AssignStmt
	Index int

	Target ast.Expr
	Value  ast.Expr
	Source sourceprovenance.ASTSource

	Symbol    symbol.ID
	HasSymbol bool
	Path      path.Path
	HasPath   bool

	ContainerPath    path.Path
	HasContainerPath bool

	Lhs []ast.Expr
	Rhs []ast.Expr
}

type CallFact struct {
	Stmt       *ast.FuncCallStmt
	SourceStmt ast.Stmt
	Context    CallContextKind

	Call      *ast.FuncCallExpr
	ExprIndex int
	// ConditionNegated is true when this condition call selects the branch
	// through `not call(...)` rather than `call(...)`.
	ConditionNegated bool
	Final            bool
	Expanded         bool
	Adjusted         bool
	OpenTail         bool

	Func     ast.Expr
	Receiver ast.Expr
	Method   string
	Args     []ast.Expr
	TypeArgs []ast.TypeExpr

	ArgumentSources []sourceprovenance.ASTSource
	CallSpan        SourceSpan
	CalleeSpan      SourceSpan
	ArgumentSpans   []SourceSpan
	ArgumentLabels  []string

	CalleePath         path.Path
	HasCalleePath      bool
	CalleeMemberAccess bool
	ReceiverPath       path.Path
	HasReceiverPath    bool
	MethodPath         path.Path
	HasMethodPath      bool

	ReceiverSource    sourceprovenance.ASTSource
	HasReceiverSource bool

	ResultTargets []CallResultTarget

	CalleeSymbol    symbol.ID
	HasCalleeSymbol bool
}

type ReturnFact struct {
	Stmt    *ast.ReturnStmt
	Exprs   []ast.Expr
	Sources []sourceprovenance.ASTSource
}

type ObjectLiteralFact struct {
	Expr    ast.Expr
	Table   *ast.TableExpr
	Entries []ObjectEntryFact
}

type ObjectEntryFact struct {
	Field      *ast.Field
	Index      int
	Key        ast.Expr
	Value      ast.Expr
	ValueSpan  SourceSpan
	ValueLabel string
	Suffix     path.Path
	Source     sourceprovenance.ASTSource
}

// SourceSpan is a syntax-free source range carried by semantic facts for
// downstream consumers that must not inspect AST nodes.
type SourceSpan struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

type BranchConditionFact struct {
	Kind BranchKind

	Stmt      ast.Stmt
	If        *ast.IfStmt
	While     *ast.WhileStmt
	Repeat    *ast.RepeatStmt
	Condition ast.Expr
	Source    sourceprovenance.ASTSource
	Check     branchcond.Check
}

type CallResultTarget struct {
	Kind        CallResultTargetKind
	Index       int
	ResultIndex int

	Local  *ast.LocalAssignStmt
	Assign *ast.AssignStmt
	Return *ast.ReturnStmt

	Name   string
	Target ast.Expr

	Symbol    symbol.ID
	HasSymbol bool
	Path      path.Path
	HasPath   bool

	OpenTail bool
}
