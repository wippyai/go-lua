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
	Final     bool
	Expanded  bool
	Adjusted  bool
	OpenTail  bool

	Func     ast.Expr
	Receiver ast.Expr
	Method   string
	Args     []ast.Expr
	TypeArgs []ast.TypeExpr

	ArgumentSources []sourceprovenance.ASTSource

	CalleePath      path.Path
	HasCalleePath   bool
	ReceiverPath    path.Path
	HasReceiverPath bool
	MethodPath      path.Path
	HasMethodPath   bool

	ResultTargets []CallResultTarget

	CalleeSymbol    symbol.ID
	HasCalleeSymbol bool

	ChannelSelect    ChannelSelectFact
	HasChannelSelect bool
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
	Field  *ast.Field
	Index  int
	Key    ast.Expr
	Value  ast.Expr
	Suffix path.Path
	Source sourceprovenance.ASTSource
}

type ChannelSelectFact struct {
	Call         *ast.FuncCallExpr
	ResultTarget CallResultTarget
	Cases        []ChannelSelectCaseFact
	HasDefault   bool
}

type ChannelSelectCaseFact struct {
	CaseCall       *ast.FuncCallExpr
	ChannelPath    path.Path
	HasChannelPath bool
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
