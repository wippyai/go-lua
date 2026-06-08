package propagate

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// Demand seeds the backward SSA-version liveness solve. The caller (which can
// enumerate real read/def sites) records, per CFG point, the access paths read
// there (Uses) and the root versions (re)defined there (Defs). Propagate runs
// the standard backward dataflow over these to compute the live access-path
// demand at every point, then forgets condition facts whose referenced access
// paths are all dead.
//
// Uses are keyed at access-path granularity (root version plus leading
// segments) so that distinct field discriminants on a shared live root die
// after their own guard regions. Defs are keyed by (symbol, version): a
// whole-variable redefinition kills downward demand for the prior version.
type Demand struct {
	// Uses maps a point to the access paths read at (or whose narrowing applies
	// at) that point.
	Uses map[cfg.Point][]constraint.Path
	// Defs maps a point to the root symbol versions defined there.
	Defs map[cfg.Point][]cfg.Version
}

// pathDemandKey is the access-path identity a use records: the root
// symbol/version plus its leading segment chain with the SSA version stored
// outside the stripped path key. A whole-variable redefinition kills all
// downward demand for that exact version, both the root and its field/index
// prefixes. This is also the lookup key for projection; no secondary path index
// is needed.
type pathDemandKey struct {
	sym      cfg.SymbolID
	ver      int
	stripped constraint.PathKey
}

type symVer struct {
	sym cfg.SymbolID
	ver int
}

type edgeKey struct {
	from cfg.Point
	to   cfg.Point
}

type phiProvider interface {
	PhiNodes() []cfg.PhiNode
}

// liveSets holds the per-point access-path demand computed by the backward
// SSA-version liveness solve.
type liveSets struct {
	liveIn  map[cfg.Point]map[pathDemandKey]struct{}
	liveOut map[cfg.Point]map[pathDemandKey]struct{}
}

// ConditionProjector applies the SSA-version relevance abstraction to a
// condition at a CFG point. The product equation solver uses this as the single
// condition-vocabulary bound instead of carrying parallel projection logic.
type ConditionProjector struct {
	live  *liveSets
	cache map[projectionCacheKey][]projectionCacheEntry
}

type projectionCacheKey struct {
	point cfg.Point
	hash  uint64
	out   bool
}

type projectionCacheEntry struct {
	condition constraint.Condition
	projected constraint.Condition
}

// NewConditionProjector computes the liveness demand needed to project
// path-condition facts. A nil or disabled demand returns a projector whose
// Project method is a no-op.
func NewConditionProjector(inputs *Inputs) *ConditionProjector {
	live := computeLiveSets(inputs)
	p := &ConditionProjector{live: live}
	if live.enabled() {
		p.cache = make(map[projectionCacheKey][]projectionCacheEntry)
	}
	return p
}

// Enabled reports whether this projector has real liveness demand.
func (p *ConditionProjector) Enabled() bool {
	return p != nil && p.live.enabled()
}

// Project forgets condition literals whose semantic access paths are dead at
// point. Forgetting weakens the condition, so this is sound for forward
// propagation and bounds acyclic DNF vocabulary growth.
func (p *ConditionProjector) Project(point cfg.Point, cond constraint.Condition) constraint.Condition {
	return p.project(point, cond, false)
}

// ProjectOut forgets condition literals using demand live after point. Point
// cells in the equation solver hold post-transfer state, so assignment defs at
// the point must not kill facts that are needed by successors.
func (p *ConditionProjector) ProjectOut(point cfg.Point, cond constraint.Condition) constraint.Condition {
	return p.project(point, cond, true)
}

func (p *ConditionProjector) project(point cfg.Point, cond constraint.Condition, out bool) constraint.Condition {
	if !p.Enabled() {
		return cond
	}
	key := projectionCacheKey{point: point, hash: cond.Hash(), out: out}
	for _, entry := range p.cache[key] {
		if entry.condition.Equals(cond) {
			return entry.projected
		}
	}
	projected := cond.Project(func(lit constraint.Constraint) bool {
		return literalLive(p.live, point, lit, out)
	})
	p.cache[key] = append(p.cache[key], projectionCacheEntry{
		condition: cond,
		projected: projected,
	})
	return projected
}

// enabled reports whether projection should run: only when liveness demand was
// supplied and solved.
func (l *liveSets) enabled() bool {
	return l != nil && l.liveIn != nil
}

// fieldPathLive reports whether an access path is demanded at point p, matching
// on the same symbol, SSA version, and field/index path. Transfer and condition
// extraction stamp mutable access-path facts at their source point; projection
// must not let a guard over x@old.f survive just because x@new.f is live after a
// redefinition. Prefix demand still preserves same-version sibling discriminants:
// reading x@v.value demands x@v, so a FieldEquals{x@v,"kind"} literal remains
// live while the same version's root is demanded. Placeholder/empty roots are
// always live.
func (l *liveSets) fieldPathLive(p cfg.Point, path constraint.Path, out bool) bool {
	if l == nil || l.liveIn == nil {
		return true
	}
	if path.Symbol == 0 {
		return true
	}
	set := l.liveIn[p]
	if out {
		set = l.liveOut[p]
	}
	if set == nil {
		return false
	}
	_, ok := set[pathDemandKey{sym: path.Symbol, ver: path.Version, stripped: strippedKey(path)}]
	return ok
}

func demandKeyOf(path constraint.Path) pathDemandKey {
	return pathDemandKey{sym: path.Symbol, ver: path.Version, stripped: strippedKey(path)}
}

// strippedKey is the access-path suffix identity: symbol plus segment suffix,
// with the SSA version stored separately in pathDemandKey.
func strippedKey(path constraint.Path) constraint.PathKey {
	bare := constraint.Path{Root: path.Root, Symbol: path.Symbol, Segments: path.Segments}
	return bare.Key()
}

// computeLiveSets runs the standard backward liveness dataflow over the CFG:
//
//	live-in[p]  = uses[p] ∪ (live-out[p] \ defs[p])
//	live-out[p] = ∪ live-in[succ]
//
// It depends only on the CFG/SSA structure and the supplied demand (read/def
// sites), never on the value-domain fixpoint, and is computed once.
func computeLiveSets(inputs *Inputs) *liveSets {
	if inputs == nil || inputs.Graph == nil || inputs.Demand == nil {
		return &liveSets{}
	}
	g := inputs.Graph
	rpo := g.RPO()
	if len(rpo) == 0 {
		return &liveSets{}
	}

	uses := make(map[cfg.Point]map[pathDemandKey]struct{}, len(rpo))
	for p, paths := range inputs.Demand.Uses {
		set := uses[p]
		if set == nil {
			set = make(map[pathDemandKey]struct{}, len(paths))
			uses[p] = set
		}
		for _, path := range paths {
			if path.Symbol == 0 {
				continue
			}
			// Reading an access path demands every prefix: reading x.a.b requires
			// x and x.a as well, so a guard on any prefix (HasType{x.a}, Truthy{x})
			// stays relevant to a deeper read. Distinct sibling fields (x.a vs x.b)
			// still differ by segments, preserving the acyclic DNF bound.
			for i := len(path.Segments); i >= 0; i-- {
				prefix := constraint.Path{
					Root:     path.Root,
					Symbol:   path.Symbol,
					Version:  path.Version,
					Segments: path.Segments[:i],
				}
				set[demandKeyOf(prefix)] = struct{}{}
			}
		}
	}

	defs := make(map[cfg.Point]map[symVer]struct{}, len(inputs.Demand.Defs))
	for p, versions := range inputs.Demand.Defs {
		for _, v := range versions {
			if v.Symbol == 0 || v.ID == 0 {
				continue
			}
			set := defs[p]
			if set == nil {
				set = make(map[symVer]struct{})
				defs[p] = set
			}
			set[symVer{sym: v.Symbol, ver: v.ID}] = struct{}{}
		}
	}

	liveIn := make(map[cfg.Point]map[pathDemandKey]struct{}, len(rpo))
	liveOut := make(map[cfg.Point]map[pathDemandKey]struct{}, len(rpo))
	liveScratch := make(map[cfg.Point]map[pathDemandKey]struct{}, len(rpo))
	phiEdgeDemand := buildPhiEdgeDemandRenames(g)

	// Worklist over the backward dataflow: a point is re-evaluated only when one
	// of its successors' live-in changed, driven by a once-built predecessor map.
	// This computes the same least fixpoint as a round-robin sweep but with
	// near-linear work instead of O(passes x points), which matters on large
	// deeply-nested functions where the round-robin form is super-linear.
	preds := make(map[cfg.Point][]cfg.Point, len(rpo))
	for _, p := range rpo {
		for _, succ := range graphSuccessors(g, p) {
			preds[succ] = append(preds[succ], p)
		}
	}

	// Seed in reverse-RPO (successors before predecessors) so a single sweep
	// propagates fully for reducible CFGs; back-edges re-queue predecessors.
	worklist := make([]cfg.Point, len(rpo))
	for i, p := range rpo {
		worklist[len(rpo)-1-i] = p
	}
	inQueue := make(map[cfg.Point]bool, len(rpo))
	for _, p := range worklist {
		inQueue[p] = true
	}

	for len(worklist) > 0 {
		p := worklist[0]
		worklist = worklist[1:]
		inQueue[p] = false

		dset := defs[p]
		out := liveOut[p]
		if out == nil {
			out = make(map[pathDemandKey]struct{})
		} else {
			clearDemandSet(out)
		}
		for _, succ := range graphSuccessors(g, p) {
			renames := phiEdgeDemand[edgeKey{from: p, to: succ}]
			for k := range liveIn[succ] {
				k = renamePhiDemand(k, renames)
				if k.sym == 0 || k.ver == 0 {
					continue
				}
				out[k] = struct{}{}
			}
		}
		liveOut[p] = out

		old := liveIn[p]
		next := liveScratch[p]
		if next == nil {
			next = make(map[pathDemandKey]struct{}, len(out)+len(uses[p]))
		} else {
			clearDemandSet(next)
		}
		for k := range out {
			if dset != nil {
				if _, killed := dset[symVer{sym: k.sym, ver: k.ver}]; killed {
					continue
				}
			}
			next[k] = struct{}{}
		}
		for k := range uses[p] {
			next[k] = struct{}{}
		}
		if !sameDemandSet(next, old) {
			liveIn[p] = next
			liveScratch[p] = old
			for _, pred := range preds[p] {
				if !inQueue[pred] {
					worklist = append(worklist, pred)
					inQueue[pred] = true
				}
			}
		}
	}

	return &liveSets{
		liveIn:  liveIn,
		liveOut: liveOut,
	}
}

func clearDemandSet(set map[pathDemandKey]struct{}) {
	for k := range set {
		delete(set, k)
	}
}

func buildPhiEdgeDemandRenames(g Graph) map[edgeKey]map[symVer]int {
	pg, ok := g.(phiProvider)
	if !ok {
		return nil
	}
	phis := pg.PhiNodes()
	if len(phis) == 0 {
		return nil
	}
	out := make(map[edgeKey]map[symVer]int)
	for _, phi := range phis {
		if phi.Target.Symbol == 0 || phi.Target.ID == 0 {
			continue
		}
		for _, op := range phi.Operands {
			if op.Version.Symbol == 0 || op.Version.ID == 0 {
				continue
			}
			if op.Version.Symbol != phi.Target.Symbol {
				continue
			}
			edge := edgeKey{from: op.From, to: phi.Point}
			renames := out[edge]
			if renames == nil {
				renames = make(map[symVer]int)
				out[edge] = renames
			}
			renames[symVer{sym: phi.Target.Symbol, ver: phi.Target.ID}] = op.Version.ID
		}
	}
	return out
}

func renamePhiDemand(k pathDemandKey, renames map[symVer]int) pathDemandKey {
	if len(renames) == 0 {
		return k
	}
	operandVer, ok := renames[symVer{sym: k.sym, ver: k.ver}]
	if !ok {
		return k
	}
	k.ver = operandVer
	return k
}

// literalLive reports whether a condition literal must be retained by
// projection at point p. The rule is kind-agnostic: a literal is forgettable
// iff ALL of its semantically referenced access paths are dead at p (no read at
// or after p on any of them). It is retained when ANY referenced path is live.
//
// constraint.SemanticAffectedPaths enumerates every access path a literal's
// truth value depends on, including the container root for field/index
// discriminants (e.g. FieldEquals{x,"kind",lit} → [x, x.kind]). Keeping the
// literal while the root x is live preserves discriminated-union narrowing: a
// downstream read of a sibling field x.code still observes the variant
// selection. A literal that narrows only a single field (HasType{x.f, T},
// Truthy{x.f}) references just [x.f], so it dies once x.f is no longer read —
// which is exactly what bounds a straight-line chain of independent guards over
// distinct fields, regardless of guard kind.
//
// Forgetting only ever weakens (γ(c) ⊆ γ(Project(c))); a still-live referenced
// path keeps the literal, so projection can never unsoundly narrow a downstream
// query. Reassignment kill is handled separately by KillRedefinedConditions.
func literalLive(live *liveSets, p cfg.Point, lit constraint.Constraint, out bool) bool {
	visited := false
	isLive := false
	constraint.VisitSemanticAffectedPaths(lit, func(path constraint.Path) bool {
		visited = true
		if path.Symbol == 0 {
			// A placeholder/unresolved referenced path is conservatively live.
			isLive = true
			return true
		}
		if live.fieldPathLive(p, path, out) {
			isLive = true
			return true
		}
		return false
	})
	if !visited {
		return true
	}
	return isLive
}

func sameDemandSet(a, b map[pathDemandKey]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
