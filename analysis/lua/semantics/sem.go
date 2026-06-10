// Package semantics extracts AST semantic facts owned by cfgbuild statement points.
package semantics

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

var (
	ErrNoCFG         = errors.New("semantics: missing cfg")
	ErrPointMismatch = errors.New("semantics: statement point count mismatch")
)

type BranchKind uint8

const (
	BranchUnknown BranchKind = iota
	BranchIf
	BranchWhile
	BranchRepeat
)

type TypeDefinitionKind uint8

const (
	TypeDefinitionUnknown TypeDefinitionKind = iota
	TypeDefinitionAlias
	TypeDefinitionInterface
)

type Result struct {
	function *ast.FunctionExpr

	localAssignments    map[cfg.Point]LocalAssignmentFact
	ordinaryAssignments map[cfg.Point]OrdinaryAssignmentFact
	calls               map[cfg.Point]CallFact
	returns             map[cfg.Point]ReturnFact
	branches            map[cfg.Point]BranchConditionFact
	typeDefinitions     map[cfg.Point]TypeDefinitionFact
	functionDefinitions map[cfg.Point]FunctionDefinitionFact
	numericFors         map[cfg.Point]NumericForFact
	genericFors         map[cfg.Point]GenericForFact
	labels              map[cfg.Point]LabelFact
	gotos               map[cfg.Point]GotoFact
}

type LocalAssignmentFact struct {
	Stmt  *ast.LocalAssignStmt
	Index int

	Name string
	Type ast.TypeExpr
	Expr ast.Expr

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

	Symbol    symbol.ID
	HasSymbol bool

	Lhs []ast.Expr
	Rhs []ast.Expr
}

type CallFact struct {
	Stmt *ast.FuncCallStmt
	Call *ast.FuncCallExpr

	Func     ast.Expr
	Receiver ast.Expr
	Method   string
	Args     []ast.Expr
	TypeArgs []ast.TypeExpr

	CalleeSymbol    symbol.ID
	HasCalleeSymbol bool
}

type ReturnFact struct {
	Stmt  *ast.ReturnStmt
	Exprs []ast.Expr
}

type BranchConditionFact struct {
	Kind BranchKind

	Stmt      ast.Stmt
	If        *ast.IfStmt
	While     *ast.WhileStmt
	Repeat    *ast.RepeatStmt
	Condition ast.Expr
}

type TypeDefinitionFact struct {
	Kind TypeDefinitionKind

	Stmt      ast.Stmt
	Type      *ast.TypeDefStmt
	Interface *ast.InterfaceDefStmt
}

type FunctionDefinitionFact struct {
	Stmt *ast.FuncDefStmt
	Name *ast.FuncName
	Func *ast.FunctionExpr

	TargetSymbol    symbol.ID
	HasTargetSymbol bool
}

type NumericForRole uint8

const (
	NumericForRoleInit NumericForRole = iota + 1
	NumericForRoleCheck
)

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

type GenericForRole uint8

const (
	GenericForRoleCheck GenericForRole = iota + 1
	GenericForRoleVariable
)

const NoGenericForVariableIndex = -1

type GenericForFact struct {
	Stmt *ast.GenericForStmt
	Role GenericForRole

	Names []string
	Exprs []ast.Expr

	Symbols    []symbol.ID
	HasSymbols bool

	VariableIndex int
}

type LabelFact struct {
	Stmt *ast.LabelStmt
	Name string
}

type GotoFact struct {
	Stmt  *ast.GotoStmt
	Label string
}

func ExtractChunk(stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) (*Result, error) {
	if built == nil || built.Graph == nil {
		return nil, ErrNoCFG
	}
	r := newResult(nil)
	if err := r.extractStmts(stmts, bindings, built); err != nil {
		return nil, err
	}
	return r, nil
}

func ExtractFunction(fn *ast.FunctionExpr, bindings *bind.Result, built *cfgbuild.Result) (*Result, error) {
	if built == nil || built.Graph == nil {
		return nil, ErrNoCFG
	}
	r := newResult(fn)
	if fn != nil {
		if err := r.extractStmts(fn.Stmts, bindings, built); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Result) Function() *ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return r.function
}

func (r *Result) LocalAssignment(point cfg.Point) (LocalAssignmentFact, bool) {
	if r == nil {
		return LocalAssignmentFact{}, false
	}
	fact, ok := r.localAssignments[point]
	if !ok {
		return LocalAssignmentFact{}, false
	}
	return copyLocalAssignmentFact(fact), true
}

func (r *Result) OrdinaryAssignment(point cfg.Point) (OrdinaryAssignmentFact, bool) {
	if r == nil {
		return OrdinaryAssignmentFact{}, false
	}
	fact, ok := r.ordinaryAssignments[point]
	if !ok {
		return OrdinaryAssignmentFact{}, false
	}
	return copyOrdinaryAssignmentFact(fact), true
}

func (r *Result) Call(point cfg.Point) (CallFact, bool) {
	if r == nil {
		return CallFact{}, false
	}
	fact, ok := r.calls[point]
	if !ok {
		return CallFact{}, false
	}
	return copyCallFact(fact), true
}

func (r *Result) Return(point cfg.Point) (ReturnFact, bool) {
	if r == nil {
		return ReturnFact{}, false
	}
	fact, ok := r.returns[point]
	if !ok {
		return ReturnFact{}, false
	}
	return copyReturnFact(fact), true
}

func (r *Result) BranchCondition(point cfg.Point) (BranchConditionFact, bool) {
	if r == nil {
		return BranchConditionFact{}, false
	}
	fact, ok := r.branches[point]
	return fact, ok
}

func (r *Result) TypeDefinition(point cfg.Point) (TypeDefinitionFact, bool) {
	if r == nil {
		return TypeDefinitionFact{}, false
	}
	fact, ok := r.typeDefinitions[point]
	return fact, ok
}

func (r *Result) FunctionDefinition(point cfg.Point) (FunctionDefinitionFact, bool) {
	if r == nil {
		return FunctionDefinitionFact{}, false
	}
	fact, ok := r.functionDefinitions[point]
	return fact, ok
}

func (r *Result) NumericFor(point cfg.Point) (NumericForFact, bool) {
	if r == nil {
		return NumericForFact{}, false
	}
	fact, ok := r.numericFors[point]
	return fact, ok
}

func (r *Result) GenericFor(point cfg.Point) (GenericForFact, bool) {
	if r == nil {
		return GenericForFact{}, false
	}
	fact, ok := r.genericFors[point]
	if !ok {
		return GenericForFact{}, false
	}
	return copyGenericForFact(fact), true
}

func (r *Result) Label(point cfg.Point) (LabelFact, bool) {
	if r == nil {
		return LabelFact{}, false
	}
	fact, ok := r.labels[point]
	return fact, ok
}

func (r *Result) Goto(point cfg.Point) (GotoFact, bool) {
	if r == nil {
		return GotoFact{}, false
	}
	fact, ok := r.gotos[point]
	return fact, ok
}

func newResult(fn *ast.FunctionExpr) *Result {
	return &Result{
		function:            fn,
		localAssignments:    make(map[cfg.Point]LocalAssignmentFact),
		ordinaryAssignments: make(map[cfg.Point]OrdinaryAssignmentFact),
		calls:               make(map[cfg.Point]CallFact),
		returns:             make(map[cfg.Point]ReturnFact),
		branches:            make(map[cfg.Point]BranchConditionFact),
		typeDefinitions:     make(map[cfg.Point]TypeDefinitionFact),
		functionDefinitions: make(map[cfg.Point]FunctionDefinitionFact),
		numericFors:         make(map[cfg.Point]NumericForFact),
		genericFors:         make(map[cfg.Point]GenericForFact),
		labels:              make(map[cfg.Point]LabelFact),
		gotos:               make(map[cfg.Point]GotoFact),
	}
}

func (r *Result) extractStmts(stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) error {
	for _, stmt := range stmts {
		if err := r.extractStmt(stmt, bindings, built); err != nil {
			return err
		}
	}
	return nil
}

func (r *Result) extractStmt(stmt ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) error {
	switch stmt := stmt.(type) {
	case nil:
		return nil
	case *ast.AssignStmt:
		return r.extractAssign(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.LocalAssignStmt:
		return r.extractLocalAssign(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.FuncCallStmt:
		return r.extractCall(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.ReturnStmt:
		return r.extractReturn(stmt, built.StmtPoints.PointsFor(stmt))
	case *ast.DoBlockStmt:
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.IfStmt:
		if err := r.extractBranch(stmt, BranchIf, stmt.Condition, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		if err := r.extractStmts(stmt.Then, bindings, built); err != nil {
			return err
		}
		return r.extractStmts(stmt.Else, bindings, built)
	case *ast.WhileStmt:
		if err := r.extractBranch(stmt, BranchWhile, stmt.Condition, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.RepeatStmt:
		if err := r.extractStmts(stmt.Stmts, bindings, built); err != nil {
			return err
		}
		return r.extractBranch(stmt, BranchRepeat, stmt.Condition, built.StmtPoints.PointsFor(stmt))
	case *ast.NumberForStmt:
		if err := r.extractNumberFor(stmt, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.GenericForStmt:
		if err := r.extractGenericFor(stmt, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.FuncDefStmt:
		return r.extractFunctionDefinition(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.LabelStmt:
		return r.extractLabel(stmt, built.StmtPoints.PointsFor(stmt))
	case *ast.GotoStmt:
		return r.extractGoto(stmt, built.StmtPoints.PointsFor(stmt))
	case *ast.TypeDefStmt:
		return r.extractTypeDef(stmt, built.StmtPoints.PointsFor(stmt))
	case *ast.InterfaceDefStmt:
		return r.extractInterfaceDef(stmt, built.StmtPoints.PointsFor(stmt))
	default:
		return nil
	}
}

func (r *Result) extractLocalAssign(stmt *ast.LocalAssignStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < len(stmt.Names) {
		return ErrPointMismatch
	}
	exprs := copyExprs(stmt.Exprs)
	types := copyTypeExprs(stmt.Types)
	for i, name := range stmt.Names {
		id, hasSymbol := symbol.ID(0), false
		if bindings != nil {
			id, hasSymbol = bindings.LocalSymbolAt(stmt, i)
		}
		r.localAssignments[points[i]] = LocalAssignmentFact{
			Stmt:      stmt,
			Index:     i,
			Name:      name,
			Type:      typeAt(stmt.Types, i),
			Expr:      exprAt(stmt.Exprs, i),
			Symbol:    id,
			HasSymbol: hasSymbol && id != 0,
			Exprs:     exprs,
			Types:     types,
		}
	}
	return nil
}

func (r *Result) extractAssign(stmt *ast.AssignStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < len(stmt.Lhs) {
		return ErrPointMismatch
	}
	lhs := copyExprs(stmt.Lhs)
	rhs := copyExprs(stmt.Rhs)
	for i, target := range stmt.Lhs {
		id, hasSymbol := symbol.ID(0), false
		if ident, ok := target.(*ast.IdentExpr); ok && bindings != nil {
			id, hasSymbol = bindings.SymbolOf(ident)
		}
		r.ordinaryAssignments[points[i]] = OrdinaryAssignmentFact{
			Stmt:      stmt,
			Index:     i,
			Target:    target,
			Value:     exprAt(stmt.Rhs, i),
			Symbol:    id,
			HasSymbol: hasSymbol && id != 0,
			Lhs:       lhs,
			Rhs:       rhs,
		}
	}
	return nil
}

func (r *Result) extractCall(stmt *ast.FuncCallStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	call, ok := stmt.Expr.(*ast.FuncCallExpr)
	if !ok {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	id, hasSymbol := symbol.ID(0), false
	if ident, ok := call.Func.(*ast.IdentExpr); ok && bindings != nil {
		id, hasSymbol = bindings.SymbolOf(ident)
	}
	r.calls[points[0]] = CallFact{
		Stmt:            stmt,
		Call:            call,
		Func:            call.Func,
		Receiver:        call.Receiver,
		Method:          call.Method,
		Args:            copyExprs(call.Args),
		TypeArgs:        copyTypeExprs(call.TypeArgs),
		CalleeSymbol:    id,
		HasCalleeSymbol: hasSymbol && id != 0,
	}
	return nil
}

func (r *Result) extractReturn(stmt *ast.ReturnStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 1 {
		return ErrPointMismatch
	}
	r.returns[points[0]] = ReturnFact{
		Stmt:  stmt,
		Exprs: copyExprs(stmt.Exprs),
	}
	return nil
}

func (r *Result) extractBranch(stmt ast.Stmt, kind BranchKind, condition ast.Expr, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 1 {
		return ErrPointMismatch
	}
	fact := BranchConditionFact{
		Kind:      kind,
		Stmt:      stmt,
		Condition: condition,
	}
	switch stmt := stmt.(type) {
	case *ast.IfStmt:
		fact.If = stmt
	case *ast.WhileStmt:
		fact.While = stmt
	case *ast.RepeatStmt:
		fact.Repeat = stmt
	}
	r.branches[points[0]] = fact
	return nil
}

func (r *Result) extractTypeDef(stmt *ast.TypeDefStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 1 {
		return ErrPointMismatch
	}
	r.typeDefinitions[points[0]] = TypeDefinitionFact{
		Kind: TypeDefinitionAlias,
		Stmt: stmt,
		Type: stmt,
	}
	return nil
}

func (r *Result) extractInterfaceDef(stmt *ast.InterfaceDefStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	r.typeDefinitions[points[0]] = TypeDefinitionFact{
		Kind:      TypeDefinitionInterface,
		Stmt:      stmt,
		Interface: stmt,
	}
	return nil
}

func (r *Result) extractFunctionDefinition(stmt *ast.FuncDefStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	id, hasSymbol := symbol.ID(0), false
	if stmt.Name != nil {
		if ident, ok := stmt.Name.Func.(*ast.IdentExpr); ok && bindings != nil {
			id, hasSymbol = bindings.SymbolOf(ident)
		}
	}
	r.functionDefinitions[points[0]] = FunctionDefinitionFact{
		Stmt:            stmt,
		Name:            stmt.Name,
		Func:            stmt.Func,
		TargetSymbol:    id,
		HasTargetSymbol: hasSymbol && id != 0,
	}
	return nil
}

func (r *Result) extractNumberFor(stmt *ast.NumberForStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 2 {
		return ErrPointMismatch
	}
	id, hasSymbol := symbol.ID(0), false
	if bindings != nil {
		id, hasSymbol = bindings.NumForSymbol(stmt)
	}
	fact := NumericForFact{
		Stmt:      stmt,
		Name:      stmt.Name,
		Init:      stmt.Init,
		Limit:     stmt.Limit,
		Step:      stmt.Step,
		Symbol:    id,
		HasSymbol: hasSymbol && id != 0,
	}
	initFact := fact
	initFact.Role = NumericForRoleInit
	checkFact := fact
	checkFact.Role = NumericForRoleCheck
	r.numericFors[points[0]] = initFact
	r.numericFors[points[1]] = checkFact
	return nil
}

func (r *Result) extractGenericFor(stmt *ast.GenericForStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 1+len(stmt.Names) {
		return ErrPointMismatch
	}
	var symbols []symbol.ID
	if bindings != nil {
		symbols = bindings.GenericForSymbols(stmt)
	}
	fact := GenericForFact{
		Stmt:          stmt,
		Names:         copyStrings(stmt.Names),
		Exprs:         copyExprs(stmt.Exprs),
		Symbols:       copySymbols(symbols),
		HasSymbols:    completeSymbols(symbols, len(stmt.Names)),
		VariableIndex: NoGenericForVariableIndex,
	}
	checkFact := fact
	checkFact.Role = GenericForRoleCheck
	r.genericFors[points[0]] = checkFact
	for i, point := range points[1 : 1+len(stmt.Names)] {
		varFact := fact
		varFact.Role = GenericForRoleVariable
		varFact.VariableIndex = i
		r.genericFors[point] = varFact
	}
	return nil
}

func (r *Result) extractLabel(stmt *ast.LabelStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	r.labels[points[0]] = LabelFact{
		Stmt: stmt,
		Name: stmt.Name,
	}
	return nil
}

func (r *Result) extractGoto(stmt *ast.GotoStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	r.gotos[points[0]] = GotoFact{
		Stmt:  stmt,
		Label: stmt.Label,
	}
	return nil
}

func exprAt(exprs []ast.Expr, index int) ast.Expr {
	if index < 0 || index >= len(exprs) {
		return nil
	}
	return exprs[index]
}

func typeAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}

func copyExprs(in []ast.Expr) []ast.Expr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.Expr, len(in))
	copy(out, in)
	return out
}

func copyTypeExprs(in []ast.TypeExpr) []ast.TypeExpr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.TypeExpr, len(in))
	copy(out, in)
	return out
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copySymbols(in []symbol.ID) []symbol.ID {
	if len(in) == 0 {
		return nil
	}
	out := make([]symbol.ID, len(in))
	copy(out, in)
	return out
}

func completeSymbols(symbols []symbol.ID, want int) bool {
	if len(symbols) != want {
		return false
	}
	for _, id := range symbols {
		if id == 0 {
			return false
		}
	}
	return true
}

func copyLocalAssignmentFact(fact LocalAssignmentFact) LocalAssignmentFact {
	fact.Exprs = copyExprs(fact.Exprs)
	fact.Types = copyTypeExprs(fact.Types)
	return fact
}

func copyOrdinaryAssignmentFact(fact OrdinaryAssignmentFact) OrdinaryAssignmentFact {
	fact.Lhs = copyExprs(fact.Lhs)
	fact.Rhs = copyExprs(fact.Rhs)
	return fact
}

func copyCallFact(fact CallFact) CallFact {
	fact.Args = copyExprs(fact.Args)
	fact.TypeArgs = copyTypeExprs(fact.TypeArgs)
	return fact
}

func copyReturnFact(fact ReturnFact) ReturnFact {
	fact.Exprs = copyExprs(fact.Exprs)
	return fact
}

func copyGenericForFact(fact GenericForFact) GenericForFact {
	fact.Names = copyStrings(fact.Names)
	fact.Exprs = copyExprs(fact.Exprs)
	fact.Symbols = copySymbols(fact.Symbols)
	return fact
}
