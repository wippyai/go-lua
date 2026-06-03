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
// symbol/version plus its leading segment chain. The sym/ver fields let a
// whole-variable redefinition kill all downward demand for that exact version
// (both the root and any of its field/index access paths).
type pathDemandKey struct {
	sym      cfg.SymbolID
	ver      int
	key      constraint.PathKey
	stripped constraint.PathKey
}

type symVer struct {
	sym cfg.SymbolID
	ver int
}

// liveSets holds the per-point access-path demand computed by the backward
// SSA-version liveness solve.
type liveSets struct {
	liveIn map[cfg.Point]map[pathDemandKey]struct{}
}

// ConditionProjector applies the SSA-version relevance abstraction to a
// condition at a CFG point. The product equation solver uses this as the single
// condition-vocabulary bound instead of carrying parallel projection logic.
type ConditionProjector struct {
	live *liveSets
}

// NewConditionProjector computes the liveness demand needed to project
// path-condition facts. A nil or disabled demand returns a projector whose
// Project method is a no-op.
func NewConditionProjector(inputs *Inputs) *ConditionProjector {
	return &ConditionProjector{live: computeLiveSets(inputs)}
}

// Enabled reports whether this projector has real liveness demand.
func (p *ConditionProjector) Enabled() bool {
	return p != nil && p.live.enabled()
}

// Project forgets condition literals whose semantic access paths are dead at
// point. Forgetting weakens the condition, so this is sound for forward
// propagation and bounds acyclic DNF vocabulary growth.
func (p *ConditionProjector) Project(point cfg.Point, cond constraint.Condition) constraint.Condition {
	if !p.Enabled() {
		return cond
	}
	return cond.Project(func(lit constraint.Constraint) bool {
		return literalLive(p.live, point, lit)
	})
}

// enabled reports whether projection should run: only when liveness demand was
// supplied and solved.
func (l *liveSets) enabled() bool {
	return l != nil && l.liveIn != nil
}

// fieldPathLive reports whether an access path is demanded at point p, matching
// on (symbol, segments) and IGNORING the SSA version stamp. Field-presence
// guards and their downstream field reads do not always carry the same version
// stamp (guards version via the visible version at the branch; assignment
// sources may be unversioned), so version-agnostic matching on the field path
// is required. Distinct fields (x.a vs x.b) still differ by segments, so the
// acyclic DNF bound is preserved. Placeholder/empty roots are always live.
func (l *liveSets) fieldPathLive(p cfg.Point, path constraint.Path) bool {
	if l == nil || l.liveIn == nil {
		return true
	}
	if path.Symbol == 0 {
		return true
	}
	set := l.liveIn[p]
	if set == nil {
		return false
	}
	want := strippedKey(path)
	for k := range set {
		if k.sym == path.Symbol && k.stripped == want {
			return true
		}
	}
	return false
}

func demandKeyOf(path constraint.Path) pathDemandKey {
	return pathDemandKey{sym: path.Symbol, ver: path.Version, key: path.Key(), stripped: strippedKey(path)}
}

// strippedKey is the version-agnostic access-path identity: symbol plus segment
// suffix, with the SSA version stamp removed.
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
	for _, p := range rpo {
		liveIn[p] = make(map[pathDemandKey]struct{})
	}

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
		next := make(map[pathDemandKey]struct{}, len(uses[p]))
		for _, succ := range graphSuccessors(g, p) {
			for k := range liveIn[succ] {
				if dset != nil {
					if _, killed := dset[symVer{sym: k.sym, ver: k.ver}]; killed {
						continue
					}
				}
				next[k] = struct{}{}
			}
		}
		for k := range uses[p] {
			next[k] = struct{}{}
		}
		if !sameDemandSet(next, liveIn[p]) {
			liveIn[p] = next
			for _, pred := range preds[p] {
				if !inQueue[pred] {
					worklist = append(worklist, pred)
					inQueue[pred] = true
				}
			}
		}
	}

	return &liveSets{liveIn: liveIn}
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
func literalLive(live *liveSets, p cfg.Point, lit constraint.Constraint) bool {
	paths := constraint.SemanticAffectedPaths(lit)
	if len(paths) == 0 {
		return true
	}
	for _, path := range paths {
		if path.Symbol == 0 {
			// A placeholder/unresolved referenced path is conservatively live.
			return true
		}
		if live.fieldPathLive(p, path) {
			return true
		}
	}
	return false
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
