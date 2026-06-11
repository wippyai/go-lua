// Package semantics extracts AST semantic facts owned by cfgbuild statement points.
package semantics

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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

type Result struct {
	function *ast.FunctionExpr

	localAssignments    map[cfg.Point]LocalAssignmentFact
	ordinaryAssignments map[cfg.Point]OrdinaryAssignmentFact
	calls               map[cfg.Point]CallFact
	returns             map[cfg.Point]ReturnFact
	objectLiterals      map[ast.Expr]ObjectLiteralFact
	branches            map[cfg.Point]BranchConditionFact
	meta                cfgfacts.Metadata
}

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

	CalleePath      path.Path
	HasCalleePath   bool
	ReceiverPath    path.Path
	HasReceiverPath bool
	MethodPath      path.Path
	HasMethodPath   bool

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
	Field  *ast.Field
	Index  int
	Key    ast.Expr
	Value  ast.Expr
	Suffix path.Path
	Source sourceprovenance.ASTSource
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

func (r *Result) ObjectLiteral(expr ast.Expr) (ObjectLiteralFact, bool) {
	if r == nil || expr == nil {
		return ObjectLiteralFact{}, false
	}
	fact, ok := r.objectLiterals[expr]
	if !ok {
		return ObjectLiteralFact{}, false
	}
	return copyObjectLiteralFact(fact), true
}

func (r *Result) BranchCondition(point cfg.Point) (BranchConditionFact, bool) {
	if r == nil {
		return BranchConditionFact{}, false
	}
	fact, ok := r.branches[point]
	if !ok {
		return BranchConditionFact{}, false
	}
	return copyBranchConditionFact(fact), true
}

func (r *Result) TypeDefinition(point cfg.Point) (cfgfacts.TypeDefinitionFact, bool) {
	if r == nil {
		return cfgfacts.TypeDefinitionFact{}, false
	}
	return r.meta.TypeDefinition(point)
}

func (r *Result) FunctionDefinition(point cfg.Point) (cfgfacts.FunctionDefinitionFact, bool) {
	if r == nil {
		return cfgfacts.FunctionDefinitionFact{}, false
	}
	return r.meta.FunctionDefinition(point)
}

func (r *Result) NumericFor(point cfg.Point) (cfgfacts.NumericForFact, bool) {
	if r == nil {
		return cfgfacts.NumericForFact{}, false
	}
	return r.meta.NumericFor(point)
}

func (r *Result) GenericFor(point cfg.Point) (cfgfacts.GenericForFact, bool) {
	if r == nil {
		return cfgfacts.GenericForFact{}, false
	}
	return r.meta.GenericFor(point)
}

func (r *Result) Label(point cfg.Point) (cfgfacts.LabelFact, bool) {
	if r == nil {
		return cfgfacts.LabelFact{}, false
	}
	return r.meta.Label(point)
}

func (r *Result) Goto(point cfg.Point) (cfgfacts.GotoFact, bool) {
	if r == nil {
		return cfgfacts.GotoFact{}, false
	}
	return r.meta.Goto(point)
}

func newResult(fn *ast.FunctionExpr) *Result {
	return &Result{
		function:            fn,
		localAssignments:    make(map[cfg.Point]LocalAssignmentFact),
		ordinaryAssignments: make(map[cfg.Point]OrdinaryAssignmentFact),
		calls:               make(map[cfg.Point]CallFact),
		returns:             make(map[cfg.Point]ReturnFact),
		objectLiterals:      make(map[ast.Expr]ObjectLiteralFact),
		branches:            make(map[cfg.Point]BranchConditionFact),
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
		return r.extractReturn(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.DoBlockStmt:
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.IfStmt:
		if err := r.extractBranch(stmt, BranchIf, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		if err := r.extractStmts(stmt.Then, bindings, built); err != nil {
			return err
		}
		return r.extractStmts(stmt.Else, bindings, built)
	case *ast.WhileStmt:
		if err := r.extractBranch(stmt, BranchWhile, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.RepeatStmt:
		if err := r.extractStmts(stmt.Stmts, bindings, built); err != nil {
			return err
		}
		return r.extractBranch(stmt, BranchRepeat, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt))
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
	calls := topLevelValueListCalls(stmt.Exprs)
	if len(points) != len(calls)+len(stmt.Names) {
		return ErrPointMismatch
	}
	targets := localResultTargets(stmt, bindings)
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextAssignmentSource, stmt.Exprs, call.index, call.call, bindings, targets)
	}
	assignPoints := points[len(calls):]
	sources := assignmentValueSources(stmt.Exprs, len(stmt.Names), callPointsByExprIndex(calls, points))
	exprs := copyExprs(stmt.Exprs)
	types := copyTypeExprs(stmt.Types)
	r.extractObjectLiterals(stmt.Exprs)
	for i, name := range stmt.Names {
		id, hasSymbol := symbol.ID(0), false
		if bindings != nil {
			id, hasSymbol = bindings.LocalSymbolAt(stmt, i)
		}
		r.localAssignments[assignPoints[i]] = LocalAssignmentFact{
			Stmt:      stmt,
			Index:     i,
			Name:      name,
			Type:      typeAt(stmt.Types, i),
			Expr:      exprAt(stmt.Exprs, i),
			Source:    sources[i],
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
	calls := topLevelValueListCalls(stmt.Rhs)
	if len(points) != len(calls)+len(stmt.Lhs) {
		return ErrPointMismatch
	}
	targets := ordinaryResultTargets(stmt, bindings)
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextAssignmentSource, stmt.Rhs, call.index, call.call, bindings, targets)
	}
	assignPoints := points[len(calls):]
	sources := assignmentValueSources(stmt.Rhs, len(stmt.Lhs), callPointsByExprIndex(calls, points))
	lhs := copyExprs(stmt.Lhs)
	rhs := copyExprs(stmt.Rhs)
	r.extractObjectLiterals(stmt.Rhs)
	for i, target := range stmt.Lhs {
		id, hasSymbol := symbol.ID(0), false
		if ident, ok := target.(*ast.IdentExpr); ok && bindings != nil {
			id, hasSymbol = bindings.SymbolOf(ident)
		}
		targetPath, hasPath := pathexpr.Resolve(target, bindings)
		r.ordinaryAssignments[assignPoints[i]] = OrdinaryAssignmentFact{
			Stmt:      stmt,
			Index:     i,
			Target:    target,
			Value:     exprAt(stmt.Rhs, i),
			Source:    sources[i],
			Symbol:    id,
			HasSymbol: hasSymbol && id != 0,
			Path:      targetPath,
			HasPath:   hasPath,
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
	if len(points) != 1 {
		return ErrPointMismatch
	}
	r.calls[points[0]] = buildCallFact(stmt, stmt, CallContextStatement, []ast.Expr{call}, 0, call, bindings, nil)
	return nil
}

func (r *Result) extractReturn(stmt *ast.ReturnStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls := topLevelValueListCalls(stmt.Exprs)
	if len(points) != len(calls)+1 {
		return ErrPointMismatch
	}
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextReturnSource, stmt.Exprs, call.index, call.call, bindings, nil)
	}
	returnPoint := points[len(calls)]
	r.returns[returnPoint] = ReturnFact{
		Stmt:    stmt,
		Exprs:   copyExprs(stmt.Exprs),
		Sources: returnValueSources(stmt.Exprs, callPointsByExprIndex(calls, points)),
	}
	return nil
}

func (r *Result) extractBranch(stmt ast.Stmt, kind BranchKind, condition ast.Expr, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls := topLevelValueListCalls([]ast.Expr{condition})
	if len(points) != len(calls)+1 {
		return ErrPointMismatch
	}
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextCondition, []ast.Expr{condition}, call.index, call.call, bindings, nil)
	}
	branchPoint := points[len(calls)]
	fact := BranchConditionFact{
		Kind:      kind,
		Stmt:      stmt,
		Condition: condition,
		Source:    conditionValueSource(condition, callPointsByExprIndex(calls, points)),
		Check:     branchcond.Normalize(condition, bindings),
	}
	switch stmt := stmt.(type) {
	case *ast.IfStmt:
		fact.If = stmt
	case *ast.WhileStmt:
		fact.While = stmt
	case *ast.RepeatStmt:
		fact.Repeat = stmt
	}
	r.branches[branchPoint] = fact
	return nil
}

func (r *Result) extractTypeDef(stmt *ast.TypeDefStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 1 {
		return ErrPointMismatch
	}
	r.meta.SetTypeDefinition(points[0], cfgfacts.TypeDefinitionFact{
		Kind: cfgfacts.TypeDefinitionAlias,
		Stmt: stmt,
		Type: stmt,
	})
	return nil
}

func (r *Result) extractInterfaceDef(stmt *ast.InterfaceDefStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	r.meta.SetTypeDefinition(points[0], cfgfacts.TypeDefinitionFact{
		Kind:      cfgfacts.TypeDefinitionInterface,
		Stmt:      stmt,
		Interface: stmt,
	})
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
	if bindings != nil {
		id, hasSymbol = bindings.FuncDefTargetSymbol(stmt)
	}
	r.meta.SetFunctionDefinition(points[0], cfgfacts.FunctionDefinitionFact{
		Stmt:            stmt,
		Name:            stmt.Name,
		Func:            stmt.Func,
		TargetSymbol:    id,
		HasTargetSymbol: hasSymbol,
	})
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
	fact := cfgfacts.NumericForFact{
		Stmt:      stmt,
		Name:      stmt.Name,
		Init:      stmt.Init,
		Limit:     stmt.Limit,
		Step:      stmt.Step,
		Symbol:    id,
		HasSymbol: hasSymbol && id != 0,
	}
	initFact := fact
	initFact.Role = cfgfacts.NumericForRoleInit
	checkFact := fact
	checkFact.Role = cfgfacts.NumericForRoleCheck
	r.meta.SetNumericFor(points[0], initFact)
	r.meta.SetNumericFor(points[1], checkFact)
	return nil
}

func (r *Result) extractGenericFor(stmt *ast.GenericForStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls := topLevelValueListCalls(stmt.Exprs)
	if len(points) != len(calls)+1+len(stmt.Names) {
		return ErrPointMismatch
	}
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextIteratorSource, stmt.Exprs, call.index, call.call, bindings, nil)
	}
	var symbols []symbol.ID
	if bindings != nil {
		symbols = bindings.GenericForSymbols(stmt)
	}
	fact := cfgfacts.GenericForFact{
		Stmt:          stmt,
		Names:         copyStrings(stmt.Names),
		Exprs:         copyExprs(stmt.Exprs),
		Sources:       copyValueSources(iteratorValueSources(stmt.Exprs, callPointsByExprIndex(calls, points))),
		Symbols:       copySymbols(symbols),
		HasSymbols:    completeSymbols(symbols, len(stmt.Names)),
		VariableIndex: cfgfacts.NoGenericForVariableIndex,
	}
	checkFact := fact
	checkFact.Role = cfgfacts.GenericForRoleCheck
	r.meta.SetGenericFor(points[len(calls)], checkFact)
	for i, point := range points[len(calls)+1 : len(calls)+1+len(stmt.Names)] {
		varFact := fact
		varFact.Role = cfgfacts.GenericForRoleVariable
		varFact.VariableIndex = i
		r.meta.SetGenericFor(point, varFact)
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
	r.meta.SetLabel(points[0], cfgfacts.LabelFact{
		Stmt: stmt,
		Name: stmt.Name,
	})
	return nil
}

func (r *Result) extractGoto(stmt *ast.GotoStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	r.meta.SetGoto(points[0], cfgfacts.GotoFact{
		Stmt:  stmt,
		Label: stmt.Label,
	})
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
	fact.Path = copyPath(fact.Path)
	fact.Lhs = copyExprs(fact.Lhs)
	fact.Rhs = copyExprs(fact.Rhs)
	return fact
}

func copyCallFact(fact CallFact) CallFact {
	fact.Args = copyExprs(fact.Args)
	fact.TypeArgs = copyTypeExprs(fact.TypeArgs)
	fact.CalleePath = copyPath(fact.CalleePath)
	fact.ReceiverPath = copyPath(fact.ReceiverPath)
	fact.MethodPath = copyPath(fact.MethodPath)
	fact.ResultTargets = copyResultTargets(fact.ResultTargets)
	return fact
}

func copyReturnFact(fact ReturnFact) ReturnFact {
	fact.Exprs = copyExprs(fact.Exprs)
	fact.Sources = copyValueSources(fact.Sources)
	return fact
}

func copyObjectLiteralFact(fact ObjectLiteralFact) ObjectLiteralFact {
	fact.Entries = copyObjectEntries(fact.Entries)
	return fact
}

func copyObjectEntries(in []ObjectEntryFact) []ObjectEntryFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]ObjectEntryFact, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Suffix = copyPath(in[i].Suffix)
	}
	return out
}
