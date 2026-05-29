package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/numconst"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ExtractLoopInsertLengths proves a loop whose body appends exactly once per
// iteration to a single accumulator sequence establishes a length lower bound on
// that accumulator at the loop exit. The append it recognizes is either a direct
// table.insert (read from inputs.TableMutatorAssignments) or a border write
// acc[#acc+1]=v (read from the body's index-assignment info), so it runs after
// ExtractTableMutatorAssignments and after assignment extraction.
//
// Two relational forms are proven:
//
//   - Constant trip count: a numeric for with constant integer init/limit/step
//     runs its body exactly N times; the single unconditional non-nil append
//     therefore runs N times, so #target >= N at the loop exit. The bound
//     composes (raises, not resets) with any pre-loop length lower bound.
//
//   - pairs cardinality: a generic for over the builtin pairs(source) where
//     source is a map visits every key exactly once; the single unconditional
//     non-nil append therefore runs once per key, so #target >= (key cardinality
//     of source) at the loop exit. The relation is recorded with Source set; the
//     post-flow producer turns a returned accumulator tied to a parameter into a
//     relational return-length postcondition.
//
// Both forms require: a single exit edge sourced at the header test (no break /
// early return skipping the append), the append dominates every loop latch (it
// runs on every iteration), the accumulator is defined outside the body, and the
// body performs no other length mutation or reassignment of the accumulator. Any
// deviation proves nothing, leaving an indexed read of the accumulator optional.
func ExtractLoopInsertLengths(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc == nil || fc.Graph == nil || inputs == nil {
		return
	}
	c := fc.Graph.CFG()
	if c == nil {
		return
	}

	doms := cfganalysis.ComputeImmediateDominatorInfo(c)

	for pi := range c.Nodes {
		header := cfg.Point(pi)
		node := c.Node(header)
		if node == nil || !node.LoopPreheaderSet {
			continue
		}
		info, ok := fc.Graph.Info(node.LoopPreheader).(*cfg.AssignInfo)
		if !ok {
			continue
		}

		body := naturalLoopBody(c, header)
		exit, ok := singleHeaderExit(c, header, body)
		if !ok {
			continue
		}

		target, ok := singleUnconditionalAppendTarget(c, fc.Graph, inputs, header, body, doms)
		if !ok {
			continue
		}

		switch {
		case info.NumericFor != nil:
			tripCount, ok := numericForTripCount(info.NumericFor)
			if !ok || tripCount <= 0 {
				continue
			}
			inputs.LoopInsertLengths = append(inputs.LoopInsertLengths, flow.LoopInsertLength{
				Point:  exit,
				Target: target,
				Count:  tripCount,
			})
		case len(info.IterExprs) > 0:
			source, ok := pairsMapSource(fc, inputs, node.LoopPreheader, body)
			if !ok {
				continue
			}
			inputs.LoopInsertLengths = append(inputs.LoopInsertLengths, flow.LoopInsertLength{
				Point:  exit,
				Target: target,
				Source: source,
			})
		}
	}
}

// pairsMapSource returns the iterated map path when the loop preheader iterates
// the builtin pairs over a map whose key set the body does not mutate, or
// ok=false otherwise. The cardinality relation #target == keycount(source) holds
// only for the real stdlib pairs (every key visited exactly once) over a finite
// map key set that the loop does not grow or shrink.
func pairsMapSource(fc *core.FlowContext, inputs *flow.Inputs, preheader cfg.Point, body map[cfg.Point]bool) (constraint.Path, bool) {
	info, ok := fc.Graph.Info(preheader).(*cfg.AssignInfo)
	if !ok || len(info.IterExprs) == 0 {
		return constraint.Path{}, false
	}
	if !iteratesBuiltinPairs(info.IterExprs, fc.Graph.Bindings()) {
		return constraint.Path{}, false
	}
	constResolver := predicate.BuildConstResolver(inputs, preheader)
	iterSource := resolve.ExtractIteratorSource(info.IterExprs, preheader, fc.Derived.Synth, fc.Derived.SymResolver, constResolver, fc.Graph.Bindings())
	if iterSource == nil || iterSource.Kind != flow.IterateKeyed || iterSource.Path.Symbol == 0 {
		return constraint.Path{}, false
	}
	if !sourceIsMap(fc, preheader, iterSource.Path) {
		return constraint.Path{}, false
	}
	if bodyMutatesSourceKeys(fc.Graph, inputs, body, iterSource.Path) {
		return constraint.Path{}, false
	}
	return constraint.Path{
		Root:   resolve.RootNameFromBindings(fc.Graph.Bindings(), iterSource.Path.Symbol, iterSource.Path.Root),
		Symbol: iterSource.Path.Symbol,
	}, true
}

// iteratesBuiltinPairs reports whether the first iterator expression is a direct
// call to the global pairs (no receiver/method, the unshadowed builtin). A custom
// iterator or a __pairs metamethod is not recognized: only the real stdlib pairs
// guarantees each key is visited exactly once, which the cardinality relation
// requires.
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
	symKind, ok := bindings.Kind(sym)
	if !ok || symKind != cfg.SymbolGlobal {
		return false
	}
	name := bindings.Name(sym)
	return name == "" || name == "pairs"
}

// sourceIsMap reports whether the iterated source resolves to a map type at the
// loop preheader. Only a map source has a key cardinality the present-key count
// relation is tied to; an array iterated by pairs would relate the accumulator to
// its present-key count rather than #source, so it is conservatively rejected.
func sourceIsMap(fc *core.FlowContext, p cfg.Point, source constraint.Path) bool {
	if fc.Derived == nil || fc.Derived.SymResolver == nil || source.Symbol == 0 {
		return false
	}
	t, ok := fc.Derived.SymResolver(p, source.Symbol)
	if !ok || t == nil {
		return false
	}
	_, isMap := unwrap.Alias(t).(*typ.Map)
	return isMap
}

// bodyMutatesSourceKeys reports whether the loop body changes the iterated map's
// key set: a keyed write (map mutator), a table.insert into the source, or a
// reassignment of the source symbol. Any of these invalidates the fixed
// per-iteration visit count the cardinality relation depends on.
func bodyMutatesSourceKeys(graph *cfg.Graph, inputs *flow.Inputs, body map[cfg.Point]bool, source constraint.Path) bool {
	for _, mm := range inputs.MapMutatorAssignments {
		if body[mm.Point] && sameTargetSymbol(mm.Target, source) {
			return true
		}
	}
	for _, tm := range inputs.TableMutatorAssignments {
		if body[tm.Point] && sameTargetSymbol(tm.Target, source) {
			return true
		}
	}
	for p := range body {
		info, ok := graph.Info(p).(*cfg.AssignInfo)
		if !ok {
			continue
		}
		for _, t := range info.Targets {
			if t.Kind == cfg.TargetIdent && t.Symbol == source.Symbol {
				return true
			}
			if t.Kind == cfg.TargetIndex && t.BaseSymbol == source.Symbol {
				return true
			}
		}
	}
	return false
}

// numericForTripCount computes the exact iteration count of a numeric for-loop
// from constant integer init/limit/step. An ascending loop (step > 0) runs while
// i <= limit; a descending loop (step < 0) while i >= limit. A non-constant or
// zero step, or a non-constant bound, yields ok=false (no provable count).
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

// naturalLoopBody returns the natural-loop body of header: header plus every node
// that reaches a latch (a back-edge predecessor of header) without passing
// through the preheader. The preheader is the loop-entry boundary, never a body
// member, so the backward walk excludes the pre-loop initializer and any
// enclosing loop's body.
func naturalLoopBody(c *basecfg.CFG, header cfg.Point) map[cfg.Point]bool {
	body := make(map[cfg.Point]bool)
	body[header] = true
	node := c.Node(header)
	preheaderSet := node != nil && node.LoopPreheaderSet
	if preheaderSet {
		body[node.LoopPreheader] = true
	}
	var stack []cfg.Point
	for _, pred := range c.PredecessorsReadOnly(header) {
		if !body[pred] {
			body[pred] = true
			stack = append(stack, pred)
		}
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, pred := range c.PredecessorsReadOnly(p) {
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

// singleHeaderExit returns the loop's unique exit point when the body leaves to
// non-body code through exactly one edge, and that edge is sourced at the header
// test. A second exit edge (break, early return inside the body) means the count
// is not guaranteed on every leaving path, so ok=false.
func singleHeaderExit(c *basecfg.CFG, header cfg.Point, body map[cfg.Point]bool) (cfg.Point, bool) {
	var exit cfg.Point
	var exitFrom cfg.Point
	found := false
	for p := range body {
		for _, succ := range c.SuccessorsReadOnly(p) {
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

// appendSite is one length-increasing append the body performs into a sequence:
// either a direct table.insert or a border write acc[#acc+1]=v, identified by its
// CFG point and target path.
type appendSite struct {
	point  cfg.Point
	target constraint.Path
}

// singleUnconditionalAppendTarget returns the target of the body's sole append
// when the body performs exactly one non-nil append (a direct-sequence
// table.insert or a border write acc[#acc+1]=v) that dominates every loop latch
// (runs on every iteration), the target is defined outside the body, and no other
// body mutation changes the target's length. Any deviation yields ok=false (no
// provable per-iteration count).
func singleUnconditionalAppendTarget(
	c *basecfg.CFG,
	graph *cfg.Graph,
	inputs *flow.Inputs,
	header cfg.Point,
	body map[cfg.Point]bool,
	doms *cfganalysis.ImmediateDominators,
) (constraint.Path, bool) {
	var appends []appendSite
	for _, tm := range inputs.TableMutatorAssignments {
		if body[tm.Point] && isDirectSequenceAppend(tm) {
			appends = append(appends, appendSite{point: tm.Point, target: tm.Target})
		}
	}
	for p := range body {
		if site, ok := borderAppendSite(graph, p); ok {
			appends = append(appends, site)
		}
	}
	if len(appends) != 1 || appends[0].target.Symbol == 0 {
		return constraint.Path{}, false
	}
	append0 := appends[0]

	// The append must run on every iteration: it dominates each latch (back-edge
	// predecessor of the header). A conditional append (inside an if) dominates no
	// latch, so its count is not the iteration count.
	if !insertDominatesLatches(c, header, body, append0.point, doms) {
		return constraint.Path{}, false
	}

	// The target must not be reshaped elsewhere in the body: a second append, a
	// table.remove, a different index/map write, or a reassignment voids the exact
	// count. The counted append itself is excluded.
	if bodyHasOtherTargetMutation(graph, inputs, body, append0) {
		return constraint.Path{}, false
	}

	// The target must be defined before the loop (a body-local sequence is not the
	// caller-visible accumulator and its exit length is irrelevant). A body
	// assignment to the target symbol is the reassignment case already rejected.
	return append0.target, true
}

// isDirectSequenceAppend reports whether tm is a positive-length-delta non-nil
// table.insert(t, v) into a direct sequence (not a keyed/map target).
func isDirectSequenceAppend(tm flow.TableMutatorAssignment) bool {
	if tm.LengthDelta <= 0 || tm.KeySymbol != 0 || tm.KeyType != nil {
		return false
	}
	if len(tm.Target.Segments) != 0 {
		return false
	}
	v := tm.ValueType
	if v != nil && (v.Kind() == kind.Nil || unwrap.IsOptionalLike(v)) {
		return false
	}
	return true
}

// borderAppendSite recognizes a border write acc[#acc+1]=v at p: a single
// index-assignment whose target indexes a direct-sequence accumulator by the
// expression #acc + 1 (the sequence border + 1, which appends one element). The
// assigned value must not be a nil literal, since assigning nil to the border
// writes no element and does not grow the sequence. Returns the accumulator path
// and ok=true on a match.
func borderAppendSite(graph *cfg.Graph, p cfg.Point) (appendSite, bool) {
	info, ok := graph.Info(p).(*cfg.AssignInfo)
	if !ok || len(info.Targets) != 1 {
		return appendSite{}, false
	}
	target := info.Targets[0]
	if target.Kind != cfg.TargetIndex || target.BaseSymbol == 0 || target.Base == nil || target.Key == nil {
		return appendSite{}, false
	}
	basePath, lenOffset, ok := lengthIndexPath(target.Key, graph.Bindings())
	if !ok || lenOffset != 1 {
		return appendSite{}, false
	}
	if basePath.Symbol != target.BaseSymbol {
		return appendSite{}, false
	}
	if len(info.Sources) == 1 && isNilLiteralExpr(info.Sources[0]) {
		return appendSite{}, false
	}
	return appendSite{
		point: p,
		target: constraint.Path{
			Root:   resolve.RootNameFromBindings(graph.Bindings(), target.BaseSymbol, target.BaseName),
			Symbol: target.BaseSymbol,
		},
	}, true
}

// lengthIndexPath resolves an index key of the form #base (offset 0) or #base + k
// to the indexed base path and the constant offset, matching the border-append
// key #acc + 1. Any other key shape yields ok=false.
func lengthIndexPath(key ast.Expr, bindings *bind.BindingTable) (constraint.Path, int64, bool) {
	switch e := key.(type) {
	case *ast.UnaryLenOpExpr:
		basePath := path.FromExprWithBindings(e.Expr, nil, bindings)
		if basePath.IsEmpty() || basePath.Symbol == 0 {
			return constraint.Path{}, 0, false
		}
		return basePath, 0, true
	case *ast.ArithmeticOpExpr:
		if e.Operator != "+" && e.Operator != "-" {
			return constraint.Path{}, 0, false
		}
		basePath, offset, ok := lengthIndexPath(e.Lhs, bindings)
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
		return basePath, offset + k, true
	default:
		return constraint.Path{}, 0, false
	}
}

// isNilLiteralExpr reports whether expr is the nil literal.
func isNilLiteralExpr(expr ast.Expr) bool {
	_, ok := expr.(*ast.NilExpr)
	return ok
}

// insertDominatesLatches reports whether the insert point dominates every loop
// latch (a body node with a back edge to this header), i.e. it executes on every
// iteration before control returns to the header.
func insertDominatesLatches(c *basecfg.CFG, header cfg.Point, body map[cfg.Point]bool, insert cfg.Point, doms *cfganalysis.ImmediateDominators) bool {
	latches := 0
	for p := range body {
		for _, succ := range c.SuccessorsReadOnly(p) {
			if succ != header {
				continue
			}
			// p -> header is a back edge; p is a latch.
			latches++
			if !doms.Dominates(insert, p) {
				return false
			}
		}
	}
	return latches > 0
}

// bodyHasOtherTargetMutation reports whether the body changes the target's length
// or identity through anything other than the single counted append: a second
// table mutator on the target, a map/index write on the target, or a
// reassignment of the target symbol inside the body. The counted append's own
// point is excluded (a border write acc[#acc+1]=v is itself a map mutator and an
// index assignment, so its point must not count as another mutation).
func bodyHasOtherTargetMutation(graph *cfg.Graph, inputs *flow.Inputs, body map[cfg.Point]bool, counted appendSite) bool {
	target := counted.target
	for _, tm := range inputs.TableMutatorAssignments {
		if !body[tm.Point] || tm.Point == counted.point {
			continue
		}
		if sameTargetSymbol(tm.Target, target) {
			return true
		}
	}
	for _, mm := range inputs.MapMutatorAssignments {
		if !body[mm.Point] || mm.Point == counted.point {
			continue
		}
		if sameTargetSymbol(mm.Target, target) {
			return true
		}
	}
	// A reassignment of the target symbol inside the body redefines the sequence,
	// so the per-iteration append no longer accumulates onto one container. The
	// counted append's own index-assignment point is excluded.
	for p := range body {
		if p == counted.point {
			continue
		}
		info, ok := graph.Info(p).(*cfg.AssignInfo)
		if !ok {
			continue
		}
		for _, t := range info.Targets {
			if t.Kind == cfg.TargetIdent && t.Symbol == target.Symbol {
				return true
			}
		}
	}
	return false
}

func sameTargetSymbol(a, b constraint.Path) bool {
	return a.Symbol != 0 && a.Symbol == b.Symbol
}
