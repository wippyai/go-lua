package semantics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
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

func (r *Result) ChannelSelect(point cfg.Point) (ChannelSelectFact, bool) {
	if r == nil {
		return ChannelSelectFact{}, false
	}
	fact, ok := r.calls[point]
	if !ok || !fact.HasChannelSelect {
		return ChannelSelectFact{}, false
	}
	return copyChannelSelectFact(fact.ChannelSelect), true
}

func (r *Result) ChannelSelects() []ChannelSelectFact {
	if r == nil {
		return nil
	}
	var points []cfg.Point
	for point, fact := range r.calls {
		if fact.HasChannelSelect {
			points = append(points, point)
		}
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i] < points[j]
	})
	out := make([]ChannelSelectFact, 0, len(points))
	for _, point := range points {
		out = append(out, copyChannelSelectFact(r.calls[point].ChannelSelect))
	}
	return out
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
	fact.ContainerPath = copyPath(fact.ContainerPath)
	fact.Lhs = copyExprs(fact.Lhs)
	fact.Rhs = copyExprs(fact.Rhs)
	return fact
}

func copyCallFact(fact CallFact) CallFact {
	fact.Args = copyExprs(fact.Args)
	fact.TypeArgs = copyTypeExprs(fact.TypeArgs)
	fact.ArgumentSources = copyValueSources(fact.ArgumentSources)
	fact.CalleePath = copyPath(fact.CalleePath)
	fact.ReceiverPath = copyPath(fact.ReceiverPath)
	fact.MethodPath = copyPath(fact.MethodPath)
	fact.ResultTargets = copyResultTargets(fact.ResultTargets)
	fact.ChannelSelect = copyChannelSelectFact(fact.ChannelSelect)
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

func copyChannelSelectFact(fact ChannelSelectFact) ChannelSelectFact {
	fact.ResultTarget = copyResultTarget(fact.ResultTarget)
	fact.Cases = copyChannelSelectCases(fact.Cases)
	return fact
}

func copyChannelSelectCases(in []ChannelSelectCaseFact) []ChannelSelectCaseFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChannelSelectCaseFact, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].ChannelPath = copyPath(in[i].ChannelPath)
	}
	return out
}

func copyValueSources(in []sourceprovenance.ASTSource) []sourceprovenance.ASTSource {
	if len(in) == 0 {
		return nil
	}
	out := make([]sourceprovenance.ASTSource, len(in))
	copy(out, in)
	return out
}

func copyResultTargets(in []CallResultTarget) []CallResultTarget {
	if len(in) == 0 {
		return nil
	}
	out := make([]CallResultTarget, len(in))
	for i := range in {
		out[i] = copyResultTarget(in[i])
	}
	return out
}

func copyResultTarget(target CallResultTarget) CallResultTarget {
	target.Path = copyPath(target.Path)
	return target
}
