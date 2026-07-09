package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type allocationLifetime struct {
	BirthPoint           cfg.Point
	BirthSpan            SourceSpan
	HasBirthSpan         bool
	DiesBeforeSuspension bool
}

func (r *Result) allocationLifetimes() map[identity.ID]allocationLifetime {
	sites := r.collectAllocationLifetimeSites()
	if len(sites) == 0 {
		return nil
	}
	suspensions := r.suspensionPoints()
	reach := normalReachabilityQuery{result: r, memo: make(map[normalReachabilityKey]bool)}
	out := make(map[identity.ID]allocationLifetime, len(sites))
	for id, site := range sites {
		out[id] = allocationLifetime{
			BirthPoint:           site.birthPoint,
			BirthSpan:            site.birthSpan,
			HasBirthSpan:         site.hasBirthSpan,
			DiesBeforeSuspension: r.allocationDiesBeforeSuspension(site, suspensions, &reach),
		}
	}
	return out
}

// AllocationDiesBeforeSuspension reads the solved lifetime fact for id.
func (r *Result) AllocationDiesBeforeSuspension(id identity.ID) (bool, bool) {
	lifetime, ok := r.allocationLifetimes()[id]
	return lifetime.DiesBeforeSuspension, ok
}

type allocationLifetimeSite struct {
	birthPoint   cfg.Point
	birthSpan    SourceSpan
	hasBirthSpan bool
	uses         map[cfg.Point]struct{}
	escaped      bool
	captured     bool
}

func (r *Result) collectAllocationLifetimeSites() map[identity.ID]*allocationLifetimeSite {
	if r == nil || r.registry == nil || r.Graph() == nil {
		return nil
	}
	byID := make(map[identity.ID]*allocationLifetimeSite)
	byExpr := make(map[factflow.ExprRef]*allocationLifetimeSite)
	r.facts.ForEachObjectLiteral(func(ref factflow.ExprRef, lit factflow.ObjectLiteralView) bool {
		id, ok := lit.Identity()
		if !ok || id == (identity.ID{}) {
			return true
		}
		site := byID[id]
		if site == nil {
			site = &allocationLifetimeSite{uses: make(map[cfg.Point]struct{})}
			byID[id] = site
		}
		byExpr[ref] = site
		if span, ok := lit.Span(); ok && !site.hasBirthSpan {
			site.birthSpan = sourceSpanFromFactflow(span)
			site.hasBirthSpan = true
		}
		return true
	})
	if len(byID) == 0 {
		return nil
	}
	collector := allocationLifetimeCollector{result: r, byExpr: byExpr, byID: byID}
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		collector.collectPoint(point)
	}
	collector.markCaptured()
	collector.markEscaped()
	return byID
}

type allocationLifetimeCollector struct {
	result *Result
	byExpr map[factflow.ExprRef]*allocationLifetimeSite
	byID   map[identity.ID]*allocationLifetimeSite
}

func (c allocationLifetimeCollector) collectPoint(point cfg.Point) {
	r := c.result
	if assignment, ok := r.RootAssignment(point); ok {
		c.markSourceUse(point, assignment.Source())
	}
	if assignment, ok := r.PathAssignment(point); ok {
		c.markPathUse(point, assignment.TargetPathRef().ParentView())
		c.markSourceUse(point, assignment.Source())
	}
	if invalidation, ok := r.PathDescendantInvalidation(point); ok {
		c.markPathUse(point, invalidation.ContainerPathRef())
		if table, key, _, ok := invalidation.DynamicTargetRef(); ok {
			c.markPathUse(point, table)
			c.markSourceUse(point, key)
		}
	}
	if write, ok := r.DynamicIndexWrite(point); ok {
		c.markPathUse(point, write.TablePathRef())
		if keyPath, ok := write.KeyPathRef(); ok {
			c.markPathUse(point, keyPath)
		}
		if valuePath, ok := write.ValuePathRef(); ok {
			c.markPathUse(point, valuePath)
		}
		c.markSourceUse(point, write.KeySource())
		c.markSourceUse(point, write.Source())
	}
	if source, ok := r.facts.BranchConditionSource(point); ok {
		c.markSourceUse(point, source)
	}
	if site, ok := r.CallSiteView(point); ok {
		c.markPathUse(point, site.CalleePathRef())
		if receiver, ok := site.ReceiverPath(); ok {
			c.markPathUse(point, receiver)
		}
		if receiver, ok := site.ReceiverSource(); ok {
			c.markSourceUse(point, receiver)
		}
		site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
			c.markSourceUse(point, source)
			return true
		})
	}
	for _, selectFact := range r.ChannelSelects(point) {
		if p, ok := selectFact.ResultPath(); ok {
			c.markPathUse(point, p)
		}
		if p, ok := selectFact.CasePath(); ok {
			c.markPathUse(point, p)
		}
		if value, ok := selectFact.PayloadValue(); ok {
			c.markValueUse(point, value)
		}
	}
	if sources, ok := r.ReturnValueSources(point); ok {
		for _, source := range sources {
			c.markSourceUse(point, source)
		}
	}
}

func (c allocationLifetimeCollector) markSourceUse(point cfg.Point, source factflow.ValueSource) {
	c.markSourceUseSeen(point, source, nil)
}

func (c allocationLifetimeCollector) markSourceUseSeen(point cfg.Point, source factflow.ValueSource, seen map[factflow.ExprRef]struct{}) {
	if source.HasExpr {
		if site := c.byExpr[source.ExprRef]; site != nil {
			if site.birthPoint == 0 {
				site.birthPoint = point
				if source.HasSourcePoint && source.SourcePoint != 0 {
					site.birthPoint = source.SourcePoint
				}
			}
			site.uses[point] = struct{}{}
		}
		if _, already := seen[source.ExprRef]; !already {
			if seen == nil {
				seen = make(map[factflow.ExprRef]struct{})
			}
			seen[source.ExprRef] = struct{}{}
			if lit, ok := c.result.ObjectLiteralView(source.ExprRef); ok {
				lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
					c.markSourceUseSeen(point, entry.Source(), seen)
					return true
				})
				if listSource, ok := lit.ListElementSource(); ok {
					c.markSourceUseSeen(point, listSource, seen)
				}
			}
		}
	}
	if p, ok := c.result.valueSourcePath(source); ok {
		c.markPathUse(point, p)
	}
	if value, ok := c.result.SourceValueBeforeBoundary(point, source); ok {
		c.markValueUse(point, value)
	}
}

func (c allocationLifetimeCollector) markPathUse(point cfg.Point, p pathdom.Path) {
	if p.IsEmpty() {
		return
	}
	if parent := p.ParentView(); !parent.IsEmpty() {
		c.markPathValueUse(point, parent)
	}
	c.markPathValueUse(point, p)
}

func (c allocationLifetimeCollector) markPathValueUse(point cfg.Point, p pathdom.Path) {
	value, ok := c.result.PathValueBeforeBoundary(point, p)
	if !ok {
		return
	}
	c.markValueUse(point, value)
}

func (c allocationLifetimeCollector) markValueUse(point cfg.Point, value product.Value) {
	id, ok := identityvalue.ExactID(c.result.registry, value)
	if !ok {
		return
	}
	if site := c.byID[id]; site != nil {
		site.uses[point] = struct{}{}
	}
}

func (c allocationLifetimeCollector) markCaptured() {
	c.result.ForEachClosureCaptureFact(func(fact ClosureCaptureFact) bool {
		if !fact.HasIdentity {
			return true
		}
		if site := c.byID[fact.Identity]; site != nil {
			site.captured = true
		}
		return true
	})
}

func (c allocationLifetimeCollector) markEscaped() {
	exit, ok := c.result.ExitState()
	if !ok {
		for _, site := range c.byID {
			site.escaped = true
		}
		return
	}
	for id, site := range c.byID {
		site.escaped = exit.ReadPlacement(id) != placement.Stack
	}
}

func (r *Result) suspensionPoints() []cfg.Point {
	if r == nil || r.Graph() == nil {
		return nil
	}
	var out []cfg.Point
	for _, point := range r.Graph().RPO() {
		if r.PointMaySuspend(point) {
			out = append(out, point)
		}
	}
	return out
}

func (r *Result) allocationDiesBeforeSuspension(
	site *allocationLifetimeSite,
	suspensions []cfg.Point,
	reach *normalReachabilityQuery,
) bool {
	if site == nil || site.birthPoint == 0 || site.escaped || site.captured || len(site.uses) == 0 {
		return false
	}
	for _, suspension := range suspensions {
		if !reach.canReach(site.birthPoint, suspension) {
			continue
		}
		for use := range site.uses {
			if reach.canReach(suspension, use) {
				return false
			}
		}
	}
	return true
}

type normalReachabilityKey struct {
	from cfg.Point
	to   cfg.Point
}

type normalReachabilityQuery struct {
	result *Result
	memo   map[normalReachabilityKey]bool
}

func (q *normalReachabilityQuery) canReach(from, to cfg.Point) bool {
	if from == 0 || to == 0 || q == nil || q.result == nil || q.result.Graph() == nil {
		return false
	}
	if from == to {
		return true
	}
	key := normalReachabilityKey{from: from, to: to}
	if got, ok := q.memo[key]; ok {
		return got
	}
	seen := map[cfg.Point]struct{}{from: {}}
	stack := []cfg.Point{from}
	for len(stack) != 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, succ := range cfg.SuccessorsReadOnly(q.result.Graph(), cur) {
			if _, ok := seen[succ]; ok {
				continue
			}
			if !q.result.PointNormallyReachable(succ) || !q.result.EdgeCanCompleteNormally(cur, succ) {
				continue
			}
			if succ == to {
				q.memo[key] = true
				return true
			}
			seen[succ] = struct{}{}
			stack = append(stack, succ)
		}
	}
	q.memo[key] = false
	return false
}
