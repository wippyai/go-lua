package paramhints

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

type paramUse struct {
	whole  bool
	fields map[string]struct{}
}

// ProjectHintsToParamUse trims structured call-site hints to the surface the
// function body actually reads from each unannotated parameter. Hints are
// evidence for analyzing a helper, not a promise that every unused field on the
// first argument shape is part of that helper's public contract.
func ProjectHintsToParamUse(graph *cfg.Graph, fn *ast.FunctionExpr, hints []typ.Type) []typ.Type {
	if graph == nil || fn == nil || len(hints) == 0 {
		return hints
	}

	uses := collectParamUses(graph, fn)
	if len(uses) == 0 {
		return hints
	}

	var out []typ.Type
	for idx, slot := range graph.ParamSlotsReadOnly() {
		if slot.Symbol == 0 || idx < 0 || idx >= len(hints) {
			continue
		}
		hint := hints[idx]
		if hint == nil {
			continue
		}
		projected := projectHintToUse(hint, uses[slot.Symbol])
		if typ.TypeEquals(hint, projected) {
			continue
		}
		if out == nil {
			out = make([]typ.Type, len(hints))
			copy(out, hints)
		}
		out[idx] = projected
	}

	if out == nil {
		return hints
	}
	return out
}

// ProjectSignatureToParamUse completes a function signature's parameter slots
// against the fields the function body reads. Unlike ProjectHintsToParamUse it
// does not trim unused fields: a function fact is already a canonical signature
// observation, and same-body analysis only needs to ensure demanded fields are
// present even when the parameter is also used as a whole value.
func ProjectSignatureToParamUse(graph *cfg.Graph, fn *ast.FunctionExpr, sig *typ.Function) *typ.Function {
	if sig == nil || len(sig.Params) == 0 {
		return sig
	}
	uses := collectParamUses(graph, fn)
	if len(uses) == 0 {
		return sig
	}
	projected := make([]typ.Type, len(sig.Params))
	changed := false
	for idx, slot := range graph.ParamSlotsReadOnly() {
		if idx < 0 || idx >= len(sig.Params) || slot.Symbol == 0 {
			continue
		}
		use := uses[slot.Symbol]
		if len(use.fields) == 0 {
			continue
		}
		completed, ok := completeTypeWithFields(sig.Params[idx].Type, use.fields)
		if !ok || completed == nil {
			continue
		}
		projected[idx] = completed
		if !typ.TypeEquals(sig.Params[idx].Type, completed) {
			changed = true
		}
	}
	if !changed {
		return sig
	}

	builder := typ.Func().ReserveParams(len(sig.Params))
	for _, tp := range sig.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for i, p := range sig.Params {
		paramType := p.Type
		if i < len(projected) && projected[i] != nil {
			paramType = projected[i]
		}
		if p.Optional {
			builder = builder.OptParam(p.Name, paramType)
		} else {
			builder = builder.Param(p.Name, paramType)
		}
	}
	if sig.Variadic != nil {
		builder = builder.Variadic(sig.Variadic)
	}
	if len(sig.Returns) > 0 {
		builder = builder.Returns(sig.Returns...)
	}
	if sig.Effects != nil {
		builder = builder.Effects(sig.Effects)
	}
	if sig.Spec != nil {
		builder = builder.Spec(sig.Spec)
	}
	if sig.Refinement != nil {
		builder = builder.WithRefinement(sig.Refinement)
	}
	return builder.Build()
}

func completeTypeWithFields(t typ.Type, fields map[string]struct{}) (typ.Type, bool) {
	if t == nil || len(fields) == 0 {
		return t, false
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		completed, ok := completeTypeWithFields(v.Target, fields)
		if !ok {
			return t, false
		}
		return completed, true
	case *typ.Optional:
		inner, ok := completeTypeWithFields(v.Inner, fields)
		if !ok {
			return t, false
		}
		return typ.NewOptional(inner), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			completed, ok := completeTypeWithFields(member, fields)
			if !ok {
				members = append(members, member)
				continue
			}
			if !typ.TypeEquals(member, completed) {
				changed = true
			}
			members = append(members, completed)
		}
		if !changed {
			return t, false
		}
		return typ.NewUnion(members...), true
	case *typ.Record:
		return completeRecordWithFields(v, fields), true
	default:
		return t, false
	}
}

func completeRecordWithFields(r *typ.Record, fields map[string]struct{}) typ.Type {
	builder := typ.NewRecord()
	if r.Open {
		builder.SetOpen(true)
	}
	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}
	for _, field := range r.Fields {
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
			builder.Field(field.Name, field.Type)
		}
	}
	if r.HasMapComponent() {
		builder.MapComponent(r.MapKey, r.MapValue)
	}

	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if r.GetField(name) != nil {
			continue
		}
		if r.HasMapComponent() && subtype.IsSubtype(typ.LiteralString(name), r.MapKey) {
			mapValue := r.MapValue
			if mapValue == nil {
				mapValue = typ.Unknown
			}
			builder.OptField(name, mapValue)
			continue
		}
		if !r.Open {
			builder.Field(name, typ.Nil)
		}
	}
	return builder.Build()
}

func collectParamUses(graph *cfg.Graph, fn *ast.FunctionExpr) map[cfg.SymbolID]paramUse {
	paramSymbols := make(map[cfg.SymbolID]struct{})
	for _, slot := range graph.ParamSlotsReadOnly() {
		if slot.Symbol == 0 {
			continue
		}
		paramSymbols[slot.Symbol] = struct{}{}
	}
	if len(paramSymbols) == 0 {
		return nil
	}

	collector := paramUseCollector{
		bindings:               graph.Bindings(),
		paramSymbols:           paramSymbols,
		currentFunctionSymbols: currentFunctionSymbols(graph, fn),
		uses:                   make(map[cfg.SymbolID]paramUse),
	}
	for _, stmt := range fn.Stmts {
		collector.stmt(stmt)
	}
	return collector.uses
}

type paramUseCollector struct {
	bindings               *bind.BindingTable
	paramSymbols           map[cfg.SymbolID]struct{}
	currentFunctionSymbols map[cfg.SymbolID]struct{}
	uses                   map[cfg.SymbolID]paramUse
}

func (c *paramUseCollector) stmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for _, lhs := range s.Lhs {
			c.lvalue(lhs)
		}
		for _, rhs := range s.Rhs {
			c.expr(rhs)
		}
	case *ast.LocalAssignStmt:
		for _, expr := range s.Exprs {
			c.expr(expr)
		}
	case *ast.FuncCallStmt:
		c.expr(s.Expr)
	case *ast.DoBlockStmt:
		c.stmts(s.Stmts)
	case *ast.WhileStmt:
		c.condition(s.Condition)
		c.stmts(s.Stmts)
	case *ast.RepeatStmt:
		c.stmts(s.Stmts)
		c.condition(s.Condition)
	case *ast.IfStmt:
		c.condition(s.Condition)
		c.stmts(s.Then)
		c.stmts(s.Else)
	case *ast.NumberForStmt:
		c.expr(s.Init)
		c.expr(s.Limit)
		c.expr(s.Step)
		c.stmts(s.Stmts)
	case *ast.GenericForStmt:
		for _, expr := range s.Exprs {
			c.expr(expr)
		}
		c.stmts(s.Stmts)
	case *ast.FuncDefStmt:
		if s.Name != nil {
			c.expr(s.Name.Func)
			c.expr(s.Name.Receiver)
		}
		if s.Func != nil {
			c.stmts(s.Func.Stmts)
		}
	case *ast.ReturnStmt:
		for _, expr := range s.Exprs {
			c.expr(expr)
		}
	}
}

func (c *paramUseCollector) stmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		c.stmt(stmt)
	}
}

func (c *paramUseCollector) condition(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if c.isParamIdent(e) {
			return
		}
	case *ast.UnaryNotOpExpr:
		if ident, ok := e.Expr.(*ast.IdentExpr); ok && c.isParamIdent(ident) {
			return
		}
		c.condition(e.Expr)
		return
	case *ast.RelationalOpExpr:
		if isNilLiteral(e.Lhs) && c.isParamExpr(e.Rhs) {
			return
		}
		if isNilLiteral(e.Rhs) && c.isParamExpr(e.Lhs) {
			return
		}
	case *ast.LogicalOpExpr:
		c.condition(e.Lhs)
		c.condition(e.Rhs)
		return
	}
	c.expr(expr)
}

func (c *paramUseCollector) expr(expr ast.Expr) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		c.whole(e)
	case *ast.AttrGetExpr:
		if c.pathUse(expr) {
			return
		}
		c.expr(e.Object)
		c.expr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			c.expr(field.Key)
			c.expr(field.Value)
		}
	case *ast.FuncCallExpr:
		c.call(e)
	case *ast.LogicalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.RelationalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.StringConcatOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryNotOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryLenOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		c.expr(e.Expr)
	case *ast.FunctionExpr:
		c.stmts(e.Stmts)
	case *ast.CastExpr:
		c.expr(e.Expr)
	case *ast.NonNilAssertExpr:
		c.expr(e.Expr)
	}
}

func (c *paramUseCollector) call(call *ast.FuncCallExpr) {
	if call == nil {
		return
	}
	recursive := c.isDirectRecursiveCall(call)
	if call.Method != "" {
		if recv := flowpath.FromExprWithBindings(call.Receiver, nil, c.bindings); c.isParamPath(recv) {
			c.field(recv.Symbol, firstFieldOrMethod(recv, call.Method))
		} else {
			c.expr(call.Receiver)
		}
	} else if callee := flowpath.FromExprWithBindings(call.Func, nil, c.bindings); c.isParamPath(callee) {
		if len(callee.Segments) == 0 {
			c.markWhole(callee.Symbol)
		} else {
			c.field(callee.Symbol, segmentFieldName(callee.Segments[0]))
		}
	} else {
		c.expr(call.Func)
	}

	for _, arg := range call.Args {
		if recursive && c.isParamExpr(arg) {
			continue
		}
		c.expr(arg)
	}
}

func (c *paramUseCollector) isDirectRecursiveCall(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method != "" || len(c.currentFunctionSymbols) == 0 {
		return false
	}
	callee := flowpath.FromExprWithBindings(call.Func, nil, c.bindings)
	if callee.Symbol == 0 || len(callee.Segments) != 0 {
		return false
	}
	_, ok := c.currentFunctionSymbols[callee.Symbol]
	return ok
}

func (c *paramUseCollector) lvalue(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if c.pathUse(expr) {
			return
		}
		c.expr(e.Object)
		c.expr(e.Key)
	default:
		c.expr(expr)
	}
}

func (c *paramUseCollector) pathUse(expr ast.Expr) bool {
	p := flowpath.FromExprWithBindings(expr, nil, c.bindings)
	if !c.isParamPath(p) {
		return false
	}
	if len(p.Segments) == 0 {
		c.markWhole(p.Symbol)
		return true
	}
	c.field(p.Symbol, segmentFieldName(p.Segments[0]))
	return true
}

func (c *paramUseCollector) whole(expr ast.Expr) {
	if c.bindings == nil || expr == nil {
		return
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return
	}
	sym, ok := c.bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return
	}
	if _, isParam := c.paramSymbols[sym]; !isParam {
		return
	}
	c.markWhole(sym)
}

func (c *paramUseCollector) isParamExpr(expr ast.Expr) bool {
	ident, ok := expr.(*ast.IdentExpr)
	return ok && c.isParamIdent(ident)
}

func (c *paramUseCollector) isParamIdent(ident *ast.IdentExpr) bool {
	if c.bindings == nil || ident == nil {
		return false
	}
	sym, ok := c.bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return false
	}
	_, ok = c.paramSymbols[sym]
	return ok
}

func isNilLiteral(expr ast.Expr) bool {
	_, ok := expr.(*ast.NilExpr)
	return ok
}

func (c *paramUseCollector) isParamPath(p constraint.Path) bool {
	if p.IsEmpty() || p.Symbol == 0 {
		return false
	}
	_, ok := c.paramSymbols[p.Symbol]
	return ok
}

func (c *paramUseCollector) markWhole(sym cfg.SymbolID) {
	use := c.uses[sym]
	use.whole = true
	c.uses[sym] = use
}

func (c *paramUseCollector) field(sym cfg.SymbolID, name string) {
	if name == "" {
		c.markWhole(sym)
		return
	}
	use := c.uses[sym]
	if use.fields == nil {
		use.fields = make(map[string]struct{}, 1)
	}
	use.fields[name] = struct{}{}
	c.uses[sym] = use
}

func firstFieldOrMethod(p constraint.Path, method string) string {
	if len(p.Segments) == 0 {
		return method
	}
	return segmentFieldName(p.Segments[0])
}

func currentFunctionSymbols(graph *cfg.Graph, fn *ast.FunctionExpr) map[cfg.SymbolID]struct{} {
	if graph == nil || fn == nil {
		return nil
	}
	syms := make(map[cfg.SymbolID]struct{}, 1)
	if bindings := graph.Bindings(); bindings != nil {
		if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
			syms[sym] = struct{}{}
		}
	}
	for _, localFn := range graph.LocalFunctionAssignments() {
		if localFn.Func == fn && localFn.Symbol != 0 {
			syms[localFn.Symbol] = struct{}{}
		}
	}
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if info != nil && info.FuncExpr == fn && info.Symbol != 0 {
			syms[info.Symbol] = struct{}{}
		}
	})
	if len(syms) == 0 {
		return nil
	}
	return syms
}

func segmentFieldName(seg constraint.Segment) string {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return seg.Name
	default:
		return ""
	}
}

func projectHintToUse(hint typ.Type, use paramUse) typ.Type {
	if hint == nil || use.whole {
		return hint
	}
	if len(use.fields) == 0 {
		return nil
	}
	projected, ok := projectTypeToFields(hint, use.fields)
	if !ok {
		return hint
	}
	return projected
}

func projectTypeToFields(t typ.Type, fields map[string]struct{}) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return projectTypeToFields(v.Target, fields)
	case *typ.Optional:
		inner, ok := projectTypeToFields(v.Inner, fields)
		if !ok {
			return t, false
		}
		return typ.NewOptional(inner), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			projected, ok := projectTypeToFields(member, fields)
			if !ok {
				return t, false
			}
			members = append(members, projected)
		}
		return typ.NewUnion(members...), true
	case *typ.Record:
		return projectRecordToFields(v, fields), true
	default:
		return t, false
	}
}

func projectRecordToFields(r *typ.Record, fields map[string]struct{}) typ.Type {
	builder := typ.NewRecord().SetOpen(true)
	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field := r.GetField(name)
		if field == nil {
			if r.HasMapComponent() && subtype.IsSubtype(typ.LiteralString(name), r.MapKey) {
				mapValue := r.MapValue
				if mapValue == nil {
					mapValue = typ.Unknown
				}
				builder.OptField(name, mapValue)
			} else if !r.Open {
				builder.Field(name, typ.Nil)
			}
			continue
		}
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
			builder.Field(field.Name, field.Type)
		}
	}
	return builder.Build()
}
