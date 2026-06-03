package input

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/domain/numconst"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/predicate"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// LoopAppendLengthFact is a passive, normalized proof produced before
// interpretation and consumed by transfer at Point. It says a loop with one
// unconditional append establishes either:
//
//   - Count > 0: len(TargetKey) >= Count at Point; or
//   - ParamIndex >= 0: len/cardinality(TargetKey) >= len/cardinality(param).
//
// TargetKey is versioned at Point. If the target is reassigned before return,
// summary projection will see a different key and refuse to export the proof.
type LoopAppendLengthFact struct {
	Point      cfg.Point
	TargetRoot cfg.SymbolID
	Target     constraint.Path
	TargetKey  constraint.PathKey
	Count      int64
	ParamIndex int
}

type loopAppendSite struct {
	point  cfg.Point
	target constraint.Path
}

// BuildLoopAppendLengths derives conservative loop-summary facts from CFG shape
// and source syntax. It is deliberately a proof extractor, not a flow pass: when
// the proof obligation is not met exactly, it emits no fact.
func BuildLoopAppendLengths(in Inputs) []LoopAppendLengthFact {
	g := in.Graph
	if g == nil || g.CFG() == nil {
		return nil
	}
	doms := cfganalysis.ComputeImmediateDominatorInfo(g)
	resolver := pathkey.NewResolver(g)
	var facts []LoopAppendLengthFact

	for _, header := range g.RPO() {
		node := g.Node(header)
		if node == nil || !node.LoopPreheaderSet {
			continue
		}
		preheader := node.LoopPreheader
		info, ok := g.Info(preheader).(*cfg.AssignInfo)
		if !ok {
			continue
		}
		body := loopBody(g, header)
		exit, ok := singleLoopExit(g, header, body)
		if !ok {
			continue
		}
		appendSite, ok := singleLoopAppend(g, header, body, doms)
		if !ok {
			continue
		}
		target := unversionedPath(appendSite.target)
		if target.Symbol == 0 {
			continue
		}
		if loopBodyMutatesTarget(g, body, appendSite, target) {
			continue
		}
		targetKey := resolver.KeyAt(exit, target)
		if targetKey == "" {
			continue
		}
		base := LoopAppendLengthFact{
			Point:      exit,
			TargetRoot: target.Symbol,
			Target:     target,
			TargetKey:  targetKey,
			ParamIndex: -1,
		}
		switch {
		case info.NumericFor != nil:
			count, ok := numericForTripCount(info.NumericFor)
			if ok && count > 0 {
				base.Count = count
				facts = append(facts, base)
			}
		case len(info.IterExprs) > 0:
			param, ok := pairsSourceParam(in, preheader, body)
			if ok {
				base.ParamIndex = param
				facts = append(facts, base)
			}
		}
	}
	return compactLoopAppendLengthFacts(facts)
}

func loopBody(g *cfg.Graph, header cfg.Point) map[cfg.Point]bool {
	body := make(map[cfg.Point]bool)
	body[header] = true
	node := g.Node(header)
	preheaderSet := node != nil && node.LoopPreheaderSet
	if preheaderSet {
		body[node.LoopPreheader] = true
	}
	var stack []cfg.Point
	for _, pred := range g.PredecessorsReadOnly(header) {
		if !body[pred] {
			body[pred] = true
			stack = append(stack, pred)
		}
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, pred := range g.PredecessorsReadOnly(p) {
			if !body[pred] {
				body[pred] = true
				stack = append(stack, pred)
			}
		}
	}
	if preheaderSet {
		delete(body, node.LoopPreheader)
	}
	return body
}

func singleLoopExit(g *cfg.Graph, header cfg.Point, body map[cfg.Point]bool) (cfg.Point, bool) {
	var exit cfg.Point
	var exitFrom cfg.Point
	found := false
	for _, p := range sortedBodyPoints(body) {
		for _, succ := range g.SuccessorsReadOnly(p) {
			if body[succ] {
				continue
			}
			if found && (exit != succ || exitFrom != p) {
				return 0, false
			}
			exit = succ
			exitFrom = p
			found = true
		}
	}
	if !found || exitFrom != header {
		return 0, false
	}
	return exit, true
}

func singleLoopAppend(g *cfg.Graph, header cfg.Point, body map[cfg.Point]bool, doms *cfganalysis.ImmediateDominators) (loopAppendSite, bool) {
	var appends []loopAppendSite
	for _, p := range sortedBodyPoints(body) {
		if site, ok := tableInsertAppendSite(g, p); ok {
			appends = append(appends, site)
		}
		if site, ok := borderAppendSite(g, p); ok {
			appends = append(appends, site)
		}
	}
	if len(appends) != 1 || appends[0].target.Symbol == 0 {
		return loopAppendSite{}, false
	}
	if !appendDominatesLatches(g, header, body, appends[0].point, doms) {
		return loopAppendSite{}, false
	}
	return appends[0], true
}

func tableInsertAppendSite(g *cfg.Graph, p cfg.Point) (loopAppendSite, bool) {
	for _, call := range g.CallSitesAt(p) {
		if !isTableInsertCallee(call) || call.Call == nil {
			continue
		}
		args := call.Call.Args
		if len(args) < 2 || len(args) > 3 {
			continue
		}
		if isNilLiteralExpr(args[len(args)-1]) {
			continue
		}
		target, ok := staticPathAt(g, p, args[0])
		if !ok || target.Symbol == 0 {
			continue
		}
		return loopAppendSite{point: p, target: unversionedPath(target)}, true
	}
	return loopAppendSite{}, false
}

func borderAppendSite(g *cfg.Graph, p cfg.Point) (loopAppendSite, bool) {
	info, ok := g.Info(p).(*cfg.AssignInfo)
	if !ok || len(info.Targets) != 1 {
		return loopAppendSite{}, false
	}
	target := info.Targets[0]
	if target.Kind != cfg.TargetIndex || target.Base == nil || target.Key == nil {
		return loopAppendSite{}, false
	}
	basePath, ok := staticPathAt(g, p, target.Base)
	if !ok || basePath.Symbol == 0 {
		return loopAppendSite{}, false
	}
	lenPath, offset, ok := lengthIndexPath(g, p, target.Key)
	if !ok || offset != 1 || !samePathNoVersion(basePath, lenPath) {
		return loopAppendSite{}, false
	}
	if len(info.Sources) == 1 && isNilLiteralExpr(info.Sources[0]) {
		return loopAppendSite{}, false
	}
	return loopAppendSite{point: p, target: unversionedPath(basePath)}, true
}

func lengthIndexPath(g *cfg.Graph, p cfg.Point, key ast.Expr) (constraint.Path, int64, bool) {
	switch e := key.(type) {
	case *ast.UnaryLenOpExpr:
		base, ok := staticPathAt(g, p, e.Expr)
		if !ok || base.Symbol == 0 {
			return constraint.Path{}, 0, false
		}
		return unversionedPath(base), 0, true
	case *ast.ArithmeticOpExpr:
		if e.Operator != "+" && e.Operator != "-" {
			return constraint.Path{}, 0, false
		}
		base, offset, ok := lengthIndexPath(g, p, e.Lhs)
		if !ok {
			return constraint.Path{}, 0, false
		}
		k, ok := numconst.IntConstFromExpr(e.Rhs)
		if !ok {
			return constraint.Path{}, 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		return base, offset + k, true
	default:
		return constraint.Path{}, 0, false
	}
}

func appendDominatesLatches(g *cfg.Graph, header cfg.Point, body map[cfg.Point]bool, appendPoint cfg.Point, doms *cfganalysis.ImmediateDominators) bool {
	latches := 0
	for _, p := range sortedBodyPoints(body) {
		for _, succ := range g.SuccessorsReadOnly(p) {
			if succ != header {
				continue
			}
			latches++
			if !doms.Dominates(appendPoint, p) {
				return false
			}
		}
	}
	return latches > 0
}

func loopBodyMutatesTarget(g *cfg.Graph, body map[cfg.Point]bool, counted loopAppendSite, target constraint.Path) bool {
	for _, p := range sortedBodyPoints(body) {
		if p == counted.point {
			continue
		}
		if pointMutatesRoot(g, p, target.Symbol) {
			return true
		}
	}
	return false
}

func pairsSourceParam(in Inputs, preheader cfg.Point, body map[cfg.Point]bool) (int, bool) {
	g := in.Graph
	info, ok := g.Info(preheader).(*cfg.AssignInfo)
	if !ok || len(info.IterExprs) == 0 || !iteratesBuiltinPairs(info.IterExprs, g.Bindings()) {
		return -1, false
	}
	call, ok := info.IterExprs[0].(*ast.FuncCallExpr)
	if !ok || call == nil || len(call.Args) != 1 {
		return -1, false
	}
	source, ok := staticPathAt(g, preheader, call.Args[0])
	if !ok || source.Symbol == 0 || len(source.Segments) != 0 {
		return -1, false
	}
	paramIndex, ok := paramIndexForSymbol(in.Scope, source.Symbol)
	if !ok || !sourceIsDeclaredMapParam(in.Scope, paramIndex, source.Symbol) {
		return -1, false
	}
	if !paramVersionStillOriginal(g, preheader, source.Symbol, paramIndex) {
		return -1, false
	}
	for _, p := range sortedBodyPoints(body) {
		if pointMutatesRoot(g, p, source.Symbol) {
			return -1, false
		}
	}
	return paramIndex, true
}

func iteratesBuiltinPairs(iterExprs []ast.Expr, bindings *bind.BindingTable) bool {
	if len(iterExprs) == 0 {
		return false
	}
	call, ok := iterExprs[0].(*ast.FuncCallExpr)
	if !ok || call == nil || call.Method != "" || call.Receiver != nil {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident == nil || ident.Value != "pairs" {
		return false
	}
	if bindings == nil {
		return true
	}
	sym, found := bindings.SymbolOf(ident)
	if !found || sym == 0 {
		return true
	}
	kind, ok := bindings.Kind(sym)
	if !ok || kind != basecfg.SymbolGlobal {
		return false
	}
	name := bindings.Name(sym)
	return name == "" || name == "pairs"
}

func pointMutatesRoot(g *cfg.Graph, p cfg.Point, root cfg.SymbolID) bool {
	if root == 0 {
		return false
	}
	if info, ok := g.Info(p).(*cfg.AssignInfo); ok {
		for _, target := range info.Targets {
			switch target.Kind {
			case cfg.TargetIdent:
				if target.Symbol == root {
					return true
				}
			case cfg.TargetField, cfg.TargetIndex:
				if target.BaseSymbol == root {
					return true
				}
			}
		}
	}
	for _, call := range g.CallSitesAt(p) {
		if call == nil || call.Call == nil {
			continue
		}
		for _, arg := range call.Call.Args {
			path, ok := staticPathAt(g, p, arg)
			if ok && path.Symbol == root {
				return true
			}
		}
	}
	return false
}

func staticPathAt(g *cfg.Graph, p cfg.Point, expr ast.Expr) (constraint.Path, bool) {
	if g == nil || expr == nil {
		return constraint.Path{}, false
	}
	constResolver := predicate.BuildConstResolver(nil, p)
	path := domainpath.FromExprWithBindingsAt(expr, constResolver, g.Bindings(), g, p)
	if path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return unversionedPath(path), true
}

func unversionedPath(path constraint.Path) constraint.Path {
	path.Version = 0
	return path
}

func samePathNoVersion(a, b constraint.Path) bool {
	a = unversionedPath(a)
	b = unversionedPath(b)
	return a.Symbol == b.Symbol && slices.Equal(a.Segments, b.Segments)
}

func isTableInsertCallee(info *cfg.CallInfo) bool {
	if info == nil || info.Method != "" || info.CalleeName != "insert" {
		return false
	}
	p := info.CalleePath
	if p.Root != "table" || len(p.Segments) != 1 {
		return false
	}
	seg := p.Segments[0]
	return seg.Kind == constraint.SegmentField && seg.Name == "insert"
}

func isNilLiteralExpr(expr ast.Expr) bool {
	_, ok := expr.(*ast.NilExpr)
	return ok
}

func numericForTripCount(info *cfg.NumericForInfo) (int64, bool) {
	if info == nil {
		return 0, false
	}
	init, ok := numconst.IntConstFromExpr(info.Init)
	if !ok {
		return 0, false
	}
	limit, ok := numconst.IntConstFromExpr(info.Limit)
	if !ok {
		return 0, false
	}
	step := int64(1)
	if info.Step != nil {
		step, ok = numconst.IntConstFromExpr(info.Step)
		if !ok || step == 0 {
			return 0, false
		}
	}
	if step > 0 {
		if limit < init {
			return 0, true
		}
		return (limit-init)/step + 1, true
	}
	if limit > init {
		return 0, true
	}
	return (init-limit)/(-step) + 1, true
}

func paramIndexForSymbol(scope ScopeFacts, sym cfg.SymbolID) (int, bool) {
	for i, param := range scope.ParamSymbols {
		if param == sym {
			return i, true
		}
	}
	return -1, false
}

func sourceIsDeclaredMapParam(scope ScopeFacts, paramIndex int, sym cfg.SymbolID) bool {
	var t typ.Type
	if sym != 0 && scope.DeclaredTypes != nil {
		t = scope.DeclaredTypes[sym]
	}
	if t == nil && paramIndex >= 0 && scope.DeclaredParamTypes != nil {
		t = scope.DeclaredParamTypes[paramIndex]
	}
	_, ok := unwrap.Alias(t).(*typ.Map)
	return ok
}

func paramVersionStillOriginal(g *cfg.Graph, p cfg.Point, sym cfg.SymbolID, paramIndex int) bool {
	if g == nil || sym == 0 {
		return false
	}
	visible := g.VisibleVersion(p, sym)
	if visible.IsZero() {
		return false
	}
	decls := g.ParamDeclPoints()
	if paramIndex < 0 || paramIndex >= len(decls) {
		return true
	}
	decl := g.VisibleVersion(decls[paramIndex], sym)
	if decl.IsZero() {
		return true
	}
	return visible.ID == decl.ID
}

func sortedBodyPoints(body map[cfg.Point]bool) []cfg.Point {
	if len(body) == 0 {
		return nil
	}
	out := make([]cfg.Point, 0, len(body))
	for p := range body {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

func compactLoopAppendLengthFacts(xs []LoopAppendLengthFact) []LoopAppendLengthFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]LoopAppendLengthFact, 0, len(xs))
	for _, fact := range xs {
		if fact.Point == 0 || fact.TargetRoot == 0 || fact.TargetKey == "" {
			continue
		}
		if fact.Count <= 0 && fact.ParamIndex < 0 {
			continue
		}
		fact.Target = unversionedPath(fact.Target)
		out = append(out, fact)
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareLoopAppendLengthFact)
	out = slices.CompactFunc(out, func(a, b LoopAppendLengthFact) bool {
		return compareLoopAppendLengthFact(a, b) == 0
	})
	return out
}

func compareLoopAppendLengthFact(a, b LoopAppendLengthFact) int {
	if c := cmp.Compare(a.Point, b.Point); c != 0 {
		return c
	}
	if c := cmp.Compare(a.TargetRoot, b.TargetRoot); c != 0 {
		return c
	}
	if c := cmp.Compare(a.TargetKey, b.TargetKey); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Count, b.Count); c != 0 {
		return c
	}
	return cmp.Compare(a.ParamIndex, b.ParamIndex)
}
