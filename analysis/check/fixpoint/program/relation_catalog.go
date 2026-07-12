package program

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

// relationCellIdentity is deliberately indivisible: a cell is authority for
// exactly one summary equation compiled from exactly one prepared body.
// Prepared pointer equality prevents two independently prepared bodies with a
// coincidentally equal content digest from being substituted within a run.
type relationCellIdentity struct {
	Cell       transformer.CellRef
	Summary    summary.SummaryKey
	BodyDigest uint64
	Prepared   *body.Static
}

type relationCatalogEntry struct {
	identity relationCellIdentity
	compiler *transformer.PreparedPlanCompiler
	direct   transformer.DirectCallCatalog
}

// relationRunCatalog is an immutable run-local preparation product. It is not
// consulted by production solving yet. Order is canonical SummaryKey order;
// maps are private lookup indexes over immutable entries.
type relationRunCatalog struct {
	entries []relationCatalogEntry
	byKey   map[summary.SummaryKey]int
}

func (c relationRunCatalog) Entries() []relationCellIdentity {
	out := make([]relationCellIdentity, len(c.entries))
	for i := range c.entries {
		out[i] = c.entries[i].identity
	}
	return out
}

// DirectCalls requires the complete cell identity, not a free-standing key or
// digest. Identity drift therefore fails closed before point routing is read.
func (c relationRunCatalog) DirectCalls(identity relationCellIdentity) (transformer.DirectCallCatalog, bool) {
	index, ok := c.byKey[identity.Summary]
	if !ok || index < 0 || index >= len(c.entries) || c.entries[index].identity != identity {
		return transformer.DirectCallCatalog{}, false
	}
	return c.entries[index].direct, true
}

type relationCatalogCandidate struct {
	key      summary.SummaryKey
	fn       *ast.FunctionExpr
	prepared *body.Static
	compiler *transformer.PreparedPlanCompiler
	shape    transformer.Shape
	direct   map[cfg.Point]summary.SummaryKey
}

func prepareInactiveRelationCatalog(reg *axis.Registry, bindings *bind.Result, keys programKeys, prepared preparedBodies, rootFn *ast.FunctionExpr) relationRunCatalog {
	if reg == nil || bindings == nil {
		return relationRunCatalog{}
	}
	owners := make([]keyedFunction, 0, len(keys.functions)+1)
	if rootFn != nil {
		owners = append(owners, keyedFunction{funcExpr: rootFn, key: keys.rootKey})
	}
	owners = append(owners, keys.functions...)
	slices.SortFunc(owners, func(a, b keyedFunction) int {
		if a.key.Less(b.key) {
			return -1
		}
		if b.key.Less(a.key) {
			return 1
		}
		return 0
	})

	candidates := make(map[summary.SummaryKey]*relationCatalogCandidate, len(owners))
	for _, owner := range owners {
		if owner.funcExpr == nil || owner.key.Ref.IsZero() {
			continue
		}
		// BindFunction also reports its root as a function origin under the
		// lexical symbol key. The program equation owns that body under rootKey;
		// never publish the same prepared body under both identities.
		if rootFn != nil && owner.funcExpr == rootFn && owner.key != keys.rootKey {
			continue
		}
		if _, exists := candidates[owner.key]; exists {
			continue
		}
		origin, ok := bindings.FunctionOrigin(owner.funcExpr)
		if !ok || origin.Kind == bind.FunctionOriginMethod {
			continue
		}
		static := prepared.function(owner.funcExpr)
		if static == nil || !static.CompositionEligibility().Eligible() {
			continue
		}
		plan := static.OperationPlan()
		if plan == nil {
			continue
		}
		shape := transformer.Shape{Params: uint32(len(plan.BoundaryParams()))}
		compiler, err := transformer.NewPlanCompiler().Prepare(reg, static.Graph(), plan, shape)
		if err != nil || !compiler.EffectFree() {
			continue
		}
		candidate := &relationCatalogCandidate{key: owner.key, fn: owner.funcExpr, prepared: static, compiler: compiler, shape: shape}
		candidate.direct = exactRelationDirectTargets(bindings, keys, static)
		candidates[owner.key] = candidate
	}

	// Any recursive SCC remains wholly on the contextual path. Removing only a
	// recursive edge would leave a cell that appears exact but depends on a
	// fallback solve, so recursive members themselves are excluded.
	for key := range recursiveRelationCandidates(candidates) {
		delete(candidates, key)
	}
	ordered := make([]summary.SummaryKey, 0, len(candidates))
	for key := range candidates {
		ordered = append(ordered, key)
	}
	slices.SortFunc(ordered, func(a, b summary.SummaryKey) int {
		if a.Less(b) {
			return -1
		}
		if b.Less(a) {
			return 1
		}
		return 0
	})

	out := relationRunCatalog{entries: make([]relationCatalogEntry, 0, len(ordered)), byKey: make(map[summary.SummaryKey]int, len(ordered))}
	identities := make(map[summary.SummaryKey]relationCellIdentity, len(ordered))
	for i, key := range ordered {
		candidate := candidates[key]
		identities[key] = relationCellIdentity{Cell: transformer.CellRef{Function: uint64(i + 1)}, Summary: key, BodyDigest: candidate.prepared.IdentityDigest(), Prepared: candidate.prepared}
	}
	for _, key := range ordered {
		candidate := candidates[key]
		routes := make(map[cfg.Point]transformer.DirectCallTarget)
		for point, targetKey := range candidate.direct {
			targetIdentity, ok := identities[targetKey]
			target := candidates[targetKey]
			if !ok || target == nil {
				continue
			}
			routes[point] = transformer.DirectCallTarget{Cell: targetIdentity.Cell, Shape: target.shape}
		}
		direct, err := transformer.NewDirectCallCatalog(candidate.prepared.OperationPlan().PointCount(), routes)
		if err != nil {
			continue
		}
		entry := relationCatalogEntry{identity: identities[key], compiler: candidate.compiler, direct: direct}
		out.byKey[key] = len(out.entries)
		out.entries = append(out.entries, entry)
	}
	return out
}

func exactRelationDirectTargets(bindings *bind.Result, keys programKeys, static *body.Static) map[cfg.Point]summary.SummaryKey {
	plan := static.OperationPlan()
	if plan == nil {
		return nil
	}
	facts := plan.Facts()
	out := make(map[cfg.Point]summary.SummaryKey)
	for raw := 0; raw < plan.PointCount(); raw++ {
		point := cfg.Point(raw)
		site, ok := facts.CallSiteView(point)
		if !ok || site.CalleeSymbol() == 0 || site.CalleeMemberAccess() || site.MethodName() != "" || site.TypeArgCount() != 0 {
			continue
		}
		if _, ok := site.ReceiverPath(); ok {
			continue
		}
		if _, ok := site.ReceiverSource(); ok {
			continue
		}
		if _, ok := site.MethodPath(); ok {
			continue
		}
		calleePath := site.CalleePathRef()
		if calleePath.Symbol != site.CalleeSymbol() || len(calleePath.Segments) != 0 {
			continue
		}
		if !functionTargetCanUseDirectSymbolKey(bindings, site.CalleeSymbol()) || len(bindings.WriteIdents(site.CalleeSymbol())) != 0 {
			continue
		}
		key, ok := keys.targetKeys[site.CalleeSymbol()]
		if !ok {
			continue
		}
		out[point] = key
	}
	return out
}

func recursiveRelationCandidates(candidates map[summary.SummaryKey]*relationCatalogCandidate) map[summary.SummaryKey]struct{} {
	index, next := make(map[summary.SummaryKey]int), 0
	low := make(map[summary.SummaryKey]int)
	onStack := make(map[summary.SummaryKey]bool)
	stack := make([]summary.SummaryKey, 0, len(candidates))
	recursive := make(map[summary.SummaryKey]struct{})
	var visit func(summary.SummaryKey)
	visit = func(key summary.SummaryKey) {
		index[key], low[key] = next, next
		next++
		stack = append(stack, key)
		onStack[key] = true
		for _, dependency := range candidates[key].direct {
			if candidates[dependency] == nil {
				continue
			}
			if _, seen := index[dependency]; !seen {
				visit(dependency)
				if low[dependency] < low[key] {
					low[key] = low[dependency]
				}
			} else if onStack[dependency] && index[dependency] < low[key] {
				low[key] = index[dependency]
			}
		}
		if low[key] != index[key] {
			return
		}
		component := make([]summary.SummaryKey, 0, 1)
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, last)
			if last == key {
				break
			}
		}
		cyclic := len(component) > 1
		if !cyclic {
			for _, dependency := range candidates[key].direct {
				if dependency == key {
					cyclic = true
					break
				}
			}
		}
		if cyclic {
			for _, member := range component {
				recursive[member] = struct{}{}
			}
		}
	}
	ordered := make([]summary.SummaryKey, 0, len(candidates))
	for key := range candidates {
		ordered = append(ordered, key)
	}
	slices.SortFunc(ordered, func(a, b summary.SummaryKey) int {
		if a.Less(b) {
			return -1
		}
		if b.Less(a) {
			return 1
		}
		return 0
	})
	for _, key := range ordered {
		if _, seen := index[key]; !seen {
			visit(key)
		}
	}
	return recursive
}
