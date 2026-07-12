package program

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

// relationCatalogGeneration is a run-local authority token. Pointer identity,
// rather than a content digest, prevents routes or cells prepared by one run
// from being paired with an independently prepared snapshot.
type relationCatalogGeneration struct{ freeze sync.Mutex }

// relationCellIdentity is deliberately indivisible: a cell is authority for
// exactly one summary equation compiled from exactly one prepared body.
// Prepared pointer equality prevents two independently prepared bodies with a
// coincidentally equal content digest from being substituted within a run.
type relationCellIdentity struct {
	Cell       transformer.CellRef
	Summary    summary.SummaryKey
	BodyDigest uint64
	Prepared   *body.Static
	Generation *relationCatalogGeneration
}

type relationCatalogEntry struct {
	identity relationCellIdentity
	compiler *transformer.PreparedPlanCompiler
	direct   transformer.DirectCallCatalog
}

// relationConsumerIdentity is the cache-fence authority for one query owner.
// It intentionally has no Cell: consuming an admitted relation does not make
// the caller a relation producer. Prepared pointer equality prevents a route
// index prepared for one body instance from being installed on another body
// with coincidentally equal content.
type relationConsumerIdentity struct {
	Summary    summary.SummaryKey
	BodyDigest uint64
	Prepared   *body.Static
	Generation *relationCatalogGeneration
}

type relationConsumerEntry struct {
	identity relationConsumerIdentity
	direct   transformer.DirectCallCatalog
	active   bool
}

// relationConsumerPolicy is a separate immutable index over every lexical
// query owner. Its point routes contain only targets admitted into the strict
// producer catalog. Ineligible callers may therefore consume exact relations,
// while leaf producers and unrelated owners remain inactive/cacheable.
type relationConsumerPolicy struct {
	entries    []relationConsumerEntry
	byKey      map[summary.SummaryKey]int
	generation *relationCatalogGeneration
}

func (p relationConsumerPolicy) Owners() []relationConsumerIdentity {
	out := make([]relationConsumerIdentity, len(p.entries))
	for i := range p.entries {
		out[i] = p.entries[i].identity
	}
	return out
}

func (p relationConsumerPolicy) Active(owner relationConsumerIdentity) bool {
	entry, ok := p.lookup(owner)
	return ok && entry.active
}

// DirectCalls returns the immutable point routing for the exact owner
// identity. Inactive owners are valid and return an empty catalog; identity
// drift fails closed.
func (p relationConsumerPolicy) DirectCalls(owner relationConsumerIdentity) (transformer.DirectCallCatalog, bool) {
	entry, ok := p.lookup(owner)
	if !ok {
		return transformer.DirectCallCatalog{}, false
	}
	return entry.direct, true
}

func (p relationConsumerPolicy) lookup(owner relationConsumerIdentity) (relationConsumerEntry, bool) {
	index, ok := p.byKey[owner.Summary]
	if !ok || index < 0 || index >= len(p.entries) || p.entries[index].identity != owner {
		return relationConsumerEntry{}, false
	}
	return p.entries[index], true
}

// relationRunCatalog is an immutable run-local preparation product. It is not
// consulted by production solving yet. Order is canonical SummaryKey order;
// maps are private lookup indexes over immutable entries.
type relationRunCatalog struct {
	entries    []relationCatalogEntry
	byKey      map[summary.SummaryKey]int
	consumers  relationConsumerPolicy
	generation *relationCatalogGeneration
}

func (c relationRunCatalog) Entries() []relationCellIdentity {
	out := make([]relationCellIdentity, len(c.entries))
	for i := range c.entries {
		out[i] = c.entries[i].identity
	}
	return out
}

func (c relationRunCatalog) ConsumerPolicy() relationConsumerPolicy { return c.consumers }

// exactLeafActivationSlice is the deliberately narrow first production-shaped
// gate. A producer that itself contains a direct relation call may only reveal
// contextuality while its composed equation is built. Until that composition
// class has a whole-function differential gate, publish exact leaf producers
// only; every owner may still consume those leaves. The original inactive
// catalog remains unchanged for preparation audits.
func (c relationRunCatalog) exactLeafActivationSlice() relationRunCatalog {
	out := relationRunCatalog{
		generation: c.generation,
		byKey:      make(map[summary.SummaryKey]int),
		consumers: relationConsumerPolicy{
			generation: c.generation,
			byKey:      make(map[summary.SummaryKey]int),
		},
	}
	allowed := make(map[transformer.CellRef]struct{})
	for _, entry := range c.entries {
		if len(entry.direct.Cells()) != 0 || relationPreparedHasCalls(entry.identity.Prepared) || len(entry.identity.Prepared.OperationPlan().BoundaryParams()) != 0 {
			continue
		}
		allowed[entry.identity.Cell] = struct{}{}
		out.byKey[entry.identity.Summary] = len(out.entries)
		out.entries = append(out.entries, entry)
	}
	for _, consumer := range c.consumers.entries {
		routes := make(map[cfg.Point]transformer.DirectCallTarget)
		for raw := 0; raw < consumer.direct.PointCount(); raw++ {
			point := cfg.Point(raw)
			target, ok := consumer.direct.Lookup(point)
			if !ok {
				continue
			}
			if _, ok := allowed[target.Cell]; ok {
				routes[point] = target
			}
		}
		direct, err := transformer.NewDirectCallCatalog(consumer.direct.PointCount(), routes)
		if err != nil {
			continue
		}
		consumer.direct = direct
		consumer.active = len(routes) != 0
		out.consumers.byKey[consumer.identity.Summary] = len(out.consumers.entries)
		out.consumers.entries = append(out.consumers.entries, consumer)
	}
	return out
}

func relationPreparedHasCalls(prepared *body.Static) bool {
	if prepared == nil || prepared.OperationPlan() == nil {
		return true
	}
	plan := prepared.OperationPlan()
	facts := plan.Facts()
	for raw := 0; raw < plan.PointCount(); raw++ {
		if _, ok := facts.CallSiteView(cfg.Point(raw)); ok {
			return true
		}
	}
	return false
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

// Freeze solves every admitted producer as one transaction. No relation is
// observable until every equation is prepared, solved, and certified exact.
func (c relationRunCatalog) Freeze(ctx context.Context) (relationRunSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return relationRunSnapshot{}, err
	}
	if c.generation == nil || c.consumers.generation != c.generation {
		return relationRunSnapshot{}, relationFreezeError{Category: relationFreezeIdentity, Err: fmt.Errorf("catalog has no coherent generation")}
	}
	c.generation.freeze.Lock()
	defer c.generation.freeze.Unlock()
	if err := ctx.Err(); err != nil {
		return relationRunSnapshot{}, err
	}
	cells := make([]transformer.RelationCell, 0, len(c.entries))
	for _, entry := range c.entries {
		if entry.identity.Generation != c.generation || entry.compiler == nil {
			return relationRunSnapshot{}, relationFreezeError{Category: relationFreezeIdentity, Identity: entry.identity, Err: fmt.Errorf("producer identity drift")}
		}
		var (
			equation *transformer.PreparedEquation
			err      error
		)
		if len(entry.direct.Cells()) != 0 {
			equation, err = entry.compiler.DirectEquation(entry.identity.Cell, entry.direct)
		} else {
			equation, err = entry.compiler.Equation(entry.identity.Cell)
		}
		if err != nil {
			return relationRunSnapshot{}, relationFreezeError{Category: relationFreezeEquation, Identity: entry.identity, Err: err}
		}
		cell, err := equation.Cell()
		if err != nil {
			return relationRunSnapshot{}, relationFreezeError{Category: relationFreezeEquation, Identity: entry.identity, Err: err}
		}
		cells = append(cells, cell)
	}
	relations, err := transformer.SolveRelationCells(ctx, cells, transformer.RelationSolveOptions{})
	if err != nil {
		return relationRunSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return relationRunSnapshot{}, err
	}
	identities := make(map[transformer.CellRef]relationCellIdentity, len(c.entries))
	for _, entry := range c.entries {
		relation, ok := relations.Lookup(entry.identity.Cell)
		if !ok {
			return relationRunSnapshot{}, relationFreezeError{Category: relationFreezeIdentity, Identity: entry.identity, Err: fmt.Errorf("solved cell missing")}
		}
		category := relationFreezeCategory("")
		switch {
		case relation.ContextualReason() != "":
			category = relationFreezeContextual
		case relation.Widened():
			category = relationFreezeWidened
		case relation.Rows() == 0:
			category = relationFreezeEmpty
		}
		if category != "" {
			return relationRunSnapshot{}, relationFreezeError{Category: category, Identity: entry.identity, Err: fmt.Errorf("relation rejected: %s", relation.ContextualReason())}
		}
		identities[entry.identity.Cell] = entry.identity
	}
	for _, consumer := range c.consumers.entries {
		if consumer.identity.Generation != c.generation {
			return relationRunSnapshot{}, relationFreezeError{Category: relationFreezeIdentity, Err: fmt.Errorf("consumer identity drift")}
		}
		for point := 0; point < consumer.direct.PointCount(); point++ {
			target, ok := consumer.direct.Lookup(cfg.Point(point))
			if !ok {
				continue
			}
			if _, frozen := identities[target.Cell]; !frozen {
				return relationRunSnapshot{}, relationFreezeError{Category: relationFreezeIdentity, Err: fmt.Errorf("consumer route targets unfrozen cell %v", target.Cell)}
			}
		}
	}
	return relationRunSnapshot{generation: c.generation, relations: relations, identities: identities, consumers: c.consumers}, nil
}

type relationCatalogCandidate struct {
	key      summary.SummaryKey
	fn       *ast.FunctionExpr
	prepared *body.Static
	compiler *transformer.PreparedPlanCompiler
	shape    transformer.Shape
	direct   map[cfg.Point]summary.SummaryKey
}

type relationCatalogOwner struct {
	key      summary.SummaryKey
	fn       *ast.FunctionExpr
	prepared *body.Static
}

func prepareInactiveRelationCatalog(reg *axis.Registry, bindings *bind.Result, keys programKeys, prepared preparedBodies, rootFn *ast.FunctionExpr) relationRunCatalog {
	if reg == nil || bindings == nil {
		return relationRunCatalog{}
	}
	owners := relationCatalogOwners(keys, prepared, rootFn)
	slices.SortFunc(owners, func(a, b relationCatalogOwner) int {
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
		if owner.fn == nil || owner.key.Ref.IsZero() {
			continue
		}
		// BindFunction also reports its root as a function origin under the
		// lexical symbol key. The program equation owns that body under rootKey;
		// never publish the same prepared body under both identities.
		if _, exists := candidates[owner.key]; exists {
			continue
		}
		origin, ok := bindings.FunctionOrigin(owner.fn)
		if !ok || origin.Kind == bind.FunctionOriginMethod {
			continue
		}
		static := owner.prepared
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
		candidate := &relationCatalogCandidate{key: owner.key, fn: owner.fn, prepared: static, compiler: compiler, shape: shape}
		candidate.direct = exactRelationDirectTargets(bindings, keys, owner.key, static)
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

	out := relationRunCatalog{
		entries: make([]relationCatalogEntry, 0, len(ordered)),
		byKey:   make(map[summary.SummaryKey]int, len(ordered)),
		consumers: relationConsumerPolicy{
			entries: make([]relationConsumerEntry, 0, len(owners)),
			byKey:   make(map[summary.SummaryKey]int, len(owners)),
		},
	}
	out.generation = &relationCatalogGeneration{}
	out.consumers.generation = out.generation
	identities := make(map[summary.SummaryKey]relationCellIdentity, len(ordered))
	for i, key := range ordered {
		candidate := candidates[key]
		identities[key] = relationCellIdentity{Cell: transformer.CellRef{Function: uint64(i + 1)}, Summary: key, BodyDigest: candidate.prepared.IdentityDigest(), Prepared: candidate.prepared, Generation: out.generation}
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
	for _, owner := range owners {
		if owner.prepared == nil || owner.key.Ref.IsZero() {
			continue
		}
		plan := owner.prepared.OperationPlan()
		if plan == nil {
			continue
		}
		routes := make(map[cfg.Point]transformer.DirectCallTarget)
		for point, targetKey := range exactRelationDirectTargets(bindings, keys, owner.key, owner.prepared) {
			targetIdentity, ok := identities[targetKey]
			target := candidates[targetKey]
			if !ok || target == nil {
				continue
			}
			routes[point] = transformer.DirectCallTarget{Cell: targetIdentity.Cell, Shape: target.shape}
		}
		direct, err := transformer.NewDirectCallCatalog(plan.PointCount(), routes)
		if err != nil {
			continue
		}
		identity := relationConsumerIdentity{Summary: owner.key, BodyDigest: owner.prepared.IdentityDigest(), Prepared: owner.prepared, Generation: out.generation}
		entry := relationConsumerEntry{identity: identity, direct: direct, active: len(routes) != 0}
		out.consumers.byKey[owner.key] = len(out.consumers.entries)
		out.consumers.entries = append(out.consumers.entries, entry)
	}
	return out
}

// relationCatalogOwners returns every non-context query owner exactly once.
// Chunk roots have no FunctionExpr but do have a prepared body; RunFunction's
// root is owned by rootKey rather than its lexical symbol key.
func relationCatalogOwners(keys programKeys, prepared preparedBodies, rootFn *ast.FunctionExpr) []relationCatalogOwner {
	owners := make([]relationCatalogOwner, 0, len(keys.functions)+1)
	if rootFn == nil {
		owners = append(owners, relationCatalogOwner{key: keys.rootKey, prepared: prepared.root})
	} else {
		owners = append(owners, relationCatalogOwner{key: keys.rootKey, fn: rootFn, prepared: prepared.function(rootFn)})
	}
	seen := map[summary.SummaryKey]struct{}{keys.rootKey: {}}
	for _, fn := range keys.functions {
		if fn.funcExpr == nil {
			continue
		}
		// BindFunction reports rootFn under its lexical symbol as well. It is
		// still one query owner, under rootKey.
		if rootFn != nil && fn.funcExpr == rootFn {
			continue
		}
		if _, ok := seen[fn.key]; ok {
			continue
		}
		seen[fn.key] = struct{}{}
		owners = append(owners, relationCatalogOwner{key: fn.key, fn: fn.funcExpr, prepared: prepared.function(fn.funcExpr)})
	}
	return owners
}

func exactRelationDirectTargets(bindings *bind.Result, keys programKeys, owner summary.SummaryKey, static *body.Static) map[cfg.Point]summary.SummaryKey {
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
		// A context-specialized summary is the canonical legacy meaning for this
		// site. A base lexical relation must never preempt it merely because both
		// happen to project equal results in the current run.
		if expr, ok := site.Expr(); ok && expr != 0 {
			if _, contextual := keys.contexts.CallContextKey(owner, expr); contextual {
				continue
			}
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
