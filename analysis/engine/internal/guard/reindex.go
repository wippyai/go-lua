package guard

// Expr is one immutable Boolean expression over a sealed target Scope. It
// deliberately carries no mutable construction work or caller-owned atom map.
type Expr struct {
	scope Scope
	root  Guard
}

// Expr admits a sealed guard as an expression only when every decision is in
// this exact target scope.
func (s Scope) Expr(root Guard) (Expr, bool) {
	if !s.Contains(root) {
		return Expr{}, false
	}
	return Expr{scope: s, root: root}, true
}

func (e Expr) validFor(scope Scope) bool {
	return e.scope.Same(scope) && scope.Contains(e.root)
}

// Reindex is an immutable total source-coordinate relation. Each source
// decision is either forgotten or mapped to one target-scope Expr. Work sees
// only this sealed relation, never caller atom slices or replacement maps.
type Reindex struct{ value *reindex }

// projectionKind records the only two coordinate actions that can be
// transported without evaluating a target expression: retain the coordinate
// itself, or existentially forget it.  The zero value deliberately means a
// general action; Set must never be inferred to be a projection merely from
// the shape of its expression.
type projectionKind uint8

const (
	projectionGeneral projectionKind = iota
	projectionRetain
	projectionForget
)

type reindex struct {
	manager  *Manager
	source   Scope
	target   Scope
	entries  []reindexEntryRow
	identity bool
	// coordinateIdentity is weaker than identity: every source coordinate is
	// mapped to the identically ranked target coordinate, but source and target
	// may be separately issued Scope identities. Boolean payloads are reusable;
	// a State wrapper is not.
	coordinateIdentity bool
	pureProjection     bool
	sealed             bool
}

type reindexEntry struct {
	// low and high are the target valuations admitting the corresponding
	// source decision value. Set(expr) produces !expr/expr; Forget produces
	// true/true. Keeping both sides makes composition through a forgotten
	// intermediate coordinate exact rather than collapsing it to `expr=true`.
	low        Guard
	high       Guard
	identity   bool
	projection projectionKind
}

// reindexEntryRow keeps one complete source-coordinate action beside its
// immutable Manager rank. Plans retain only source rows, not a Manager-wide
// dense vector, so a plan remains valid if the Manager later grows.
type reindexEntryRow struct {
	rank uint64
	reindexEntry
}

// Source returns the exact input coordinate scope.
func (plan Reindex) Source() Scope {
	if !plan.Valid() {
		return Scope{}
	}
	return plan.value.source
}

// Target returns the exact output coordinate scope.
func (plan Reindex) Target() Scope {
	if !plan.Valid() {
		return Scope{}
	}
	return plan.value.target
}

// Valid reports whether plan is a complete immutable relation issued by its
// Manager. The entries are fixed by source rank and cannot be caller-mutated.
func (plan Reindex) Valid() bool {
	return plan.value != nil && plan.value.manager != nil && plan.value.sealed &&
		plan.value.source.Valid() && plan.value.target.Valid() &&
		plan.value.source.Manager() == plan.value.manager && plan.value.target.Manager() == plan.value.manager &&
		len(plan.value.entries) == len(plan.value.source.value.ranks)
}

// Identity reports the complete proof that this relation preserves every
// coordinate in the same scope. Carrier may reuse an immutable State only on
// this proof; semantic equality of a partial plan is insufficient.
func (plan Reindex) Identity() bool { return plan.Valid() && plan.value.identity }

// CoordinateCount is the width of the source interface: one entry per source
// coordinate, which Seal already proved complete. It is the relation's own
// dimension, not an enumeration of its scope, so a caller can size a
// derivation over the relation without gaining access to atom spellings.
func (plan Reindex) CoordinateCount() int {
	if !plan.Valid() {
		return 0
	}
	return len(plan.value.entries)
}

// CoordinateIdentity proves the relation is the identity function over the
// Boolean payload even when its source and target are distinct issued scopes.
// It authorizes immutable BDD/FDD reuse only; callers must still publish the
// exact target Scope identity.
func (plan Reindex) CoordinateIdentity() bool {
	return plan.Valid() && plan.value.coordinateIdentity
}

// ReindexBuilder is the cold single-use authoring surface for one relation.
// It accepts individual atoms only while sealing; no execution API exposes
// maps, atom slices, or rewrite ordering.
type ReindexBuilder struct {
	manager *Manager
	source  Scope
	target  Scope
	entries []reindexEntry
	set     []bool
	// work is one lazy candidate transaction for all Set/Identity entries.
	// Forget entries need no BDD construction. Keeping this shell open until
	// the complete relation seals lets one builder publish one page set rather
	// than one Work generation per source row.
	work   *Work
	sealed bool
}

// NewReindex starts a cold relation builder. Source and target must be exact
// scopes from this Manager; an empty source legitimately needs no entries.
func (m *Manager) NewReindex(source, target Scope) (*ReindexBuilder, bool) {
	if m == nil || !source.Valid() || !target.Valid() || source.Manager() != m || target.Manager() != m {
		return nil, false
	}
	entries := make([]reindexEntry, len(source.value.ranks))
	return &ReindexBuilder{manager: m, source: source, target: target, entries: entries, set: make([]bool, len(entries))}, true
}

// Forget makes one source coordinate existential. Every source coordinate
// must be assigned exactly once before Seal succeeds.
func (builder *ReindexBuilder) Forget(atom Atom) bool {
	rank, ok := builder.rank(atom)
	if !ok {
		return false
	}
	builder.entries[rank] = reindexEntry{low: builder.manager.True(), high: builder.manager.True(), projection: projectionForget}
	builder.set[rank] = true
	return true
}

// Set maps one source coordinate to an already-sealed target expression.
func (builder *ReindexBuilder) Set(atom Atom, expression Expr) bool {
	rank, ok := builder.rank(atom)
	if !ok || !expression.validFor(builder.target) {
		return false
	}
	if builder.work == nil {
		builder.work = builder.manager.NewWork()
	}
	low := builder.work.Not(expression.root)
	if !builder.work.Open() || !builder.work.owns(low) {
		return false
	}
	builder.entries[rank] = reindexEntry{low: low, high: expression.root, projection: projectionGeneral}
	builder.set[rank] = true
	return true
}

// Identity maps one source atom to the identically named target atom. It is
// valid only when that atom belongs to both scopes.
func (builder *ReindexBuilder) Identity(atom Atom) bool {
	rank, ok := builder.rank(atom)
	if !ok || !builder.target.contains(atom) {
		return false
	}
	if builder.work == nil {
		builder.work = builder.manager.NewWork()
	}
	literal, valid := builder.work.Literal(atom)
	if !valid {
		return false
	}
	negated := builder.work.Not(literal)
	if !builder.work.Open() || !builder.work.owns(negated) {
		return false
	}
	builder.entries[rank] = reindexEntry{low: negated, high: literal, identity: true, projection: projectionRetain}
	builder.set[rank] = true
	return true
}

func (builder *ReindexBuilder) rank(atom Atom) (uint64, bool) {
	if builder == nil || builder.sealed || builder.manager == nil || !builder.source.Valid() {
		return 0, false
	}
	rank, exists := builder.manager.atoms[atom]
	if !exists {
		return 0, false
	}
	index := rankSearch(builder.source.value.ranks, rank)
	return uint64(index), index < len(builder.source.value.ranks) && builder.source.value.ranks[index] == rank && !builder.set[index]
}

// Seal freezes one complete source relation. It rejects omitted source atoms
// rather than giving them an accidental identity or forget interpretation.
func (builder *ReindexBuilder) Seal() (Reindex, bool) {
	if builder == nil || builder.sealed || builder.manager == nil {
		return Reindex{}, false
	}
	for index := range builder.source.value.ranks {
		if !builder.set[index] {
			return Reindex{}, false
		}
	}
	if builder.work != nil {
		builder.work.Seal()
		if !builder.work.Published() {
			builder.sealed = true
			builder.work.Close()
			return Reindex{}, false
		}
	}
	builder.sealed = true
	entries := make([]reindexEntryRow, len(builder.entries))
	for index, entry := range builder.entries {
		entries[index] = reindexEntryRow{rank: builder.source.value.ranks[index], reindexEntry: entry}
	}
	coordinateIdentity := len(builder.source.value.ranks) == len(builder.target.value.ranks)
	if coordinateIdentity {
		for index, entry := range entries {
			if !entry.identity || builder.source.value.ranks[index] != builder.target.value.ranks[index] {
				coordinateIdentity = false
				break
			}
		}
	}
	identity := coordinateIdentity && builder.source.Same(builder.target)
	pureProjection := true
	for _, entry := range entries {
		if entry.projection == projectionGeneral {
			pureProjection = false
			break
		}
	}
	plan := Reindex{value: &reindex{manager: builder.manager, source: builder.source, target: builder.target, entries: entries, identity: identity, coordinateIdentity: coordinateIdentity, pureProjection: pureProjection, sealed: true}}
	if !plan.Valid() {
		return Reindex{}, false
	}
	builder.entries = nil
	builder.set = nil
	return plan, true
}

// IdentityReindex seals the unique complete identity relation for scope.
func (m *Manager) IdentityReindex(scope Scope) (Reindex, bool) {
	builder, ok := m.NewReindex(scope, scope)
	if !ok {
		return Reindex{}, false
	}
	for _, rank := range scope.value.ranks {
		if !builder.Identity(m.atom(rank)) {
			return Reindex{}, false
		}
	}
	return builder.Seal()
}

// ReindexAction is one opaque entry exposed only to symbolic storage that
// must interpret the already-sealed relation. It never exposes plan storage
// or a caller-authored atom map.
type ReindexAction struct {
	low  Guard
	high Guard
}

// ProjectionAction is the O(1)-validity, scalar lookup surface for a pure
// projection.  It intentionally exposes only the coordinate classification;
// general relational regions remain behind ReindexAction.
type ProjectionAction struct{ kind projectionKind }

// RetainsCoordinate reports that the source decision is the same target
// coordinate and can be rebuilt directly without an ITE.
func (action ProjectionAction) RetainsCoordinate() bool { return action.kind == projectionRetain }

// ForgetsCoordinate reports that both source fibers reach the same target
// region and therefore must be combined by the caller's existential law.
func (action ProjectionAction) ForgetsCoordinate() bool { return action.kind == projectionForget }

// Low and High return the sealed target regions admitting source false and
// source true respectively. They preserve the plan's relational meaning
// through composition while exposing no plan table or source enumeration.
func (action ReindexAction) Low() (Guard, bool) {
	return action.low, action.low.manager != nil
}

func (action ReindexAction) High() (Guard, bool) {
	return action.high, action.high.manager != nil
}

func (plan Reindex) action(atom Atom) (ReindexAction, bool) {
	if !plan.validAction() {
		return ReindexAction{}, false
	}
	rank, exists := plan.value.manager.atoms[atom]
	if !exists {
		return ReindexAction{}, false
	}
	index := rankSearch(plan.value.source.value.ranks, rank)
	if index >= len(plan.value.entries) || plan.value.entries[index].rank != rank {
		return ReindexAction{}, false
	}
	entry := plan.value.entries[index]
	return ReindexAction{low: entry.low, high: entry.high}, true
}

// validAction is the allocation-free structural proof needed by the scalar
// Action lookup. Full plan validation intentionally remains on cold sealing
// and Work entry, where target expressions are checked through DAG-aware
// Scope.Contains.
func (plan Reindex) validAction() bool {
	if plan.value == nil || plan.value.manager == nil || !plan.value.source.Valid() || !plan.value.target.Valid() || plan.value.source.Manager() != plan.value.manager || plan.value.target.Manager() != plan.value.manager || len(plan.value.entries) != len(plan.value.source.value.ranks) {
		return false
	}
	for index, row := range plan.value.entries {
		if row.rank != plan.value.source.value.ranks[index] || !plan.value.manager.validSealed(row.low) || !plan.value.manager.validSealed(row.high) {
			return false
		}
	}
	return true
}

// Action returns one already-sealed source-coordinate action. It exists for
// typed symbolic storage that must execute a Reindex; callers still cannot
// inspect plan entries collectively or provide a replacement map at runtime.
func (plan Reindex) Action(atom Atom) (ReindexAction, bool) { return plan.action(atom) }

// PureProjection proves that every source coordinate is either retained as
// itself or existentially forgotten.  The proof is computed at Seal and the
// hot read is structural and allocation-free; it never rescans the relation.
func (plan Reindex) PureProjection() bool {
	return plan.Valid() && plan.value.pureProjection
}

// ProjectionAction returns the scalar projection classification for one
// source atom. Unlike Action, it does not call validAction: the sealed proof
// makes validity O(1), followed by a source-row binary search.
func (plan Reindex) ProjectionAction(atom Atom) (ProjectionAction, bool) {
	if !plan.PureProjection() {
		return ProjectionAction{}, false
	}
	rank, exists := plan.value.manager.atoms[atom]
	if !exists {
		return ProjectionAction{}, false
	}
	index := rankSearch(plan.value.source.value.ranks, rank)
	if index >= len(plan.value.entries) || plan.value.entries[index].rank != rank {
		return ProjectionAction{}, false
	}
	action := ProjectionAction{kind: plan.value.entries[index].projection}
	if !action.RetainsCoordinate() && !action.ForgetsCoordinate() {
		return ProjectionAction{}, false
	}
	return action, true
}

// Reindex applies plan simultaneously to root. Each source branch is gated by
// its sealed relational target region then unioned, which is ITE for ordinary
// substitution and existential OR for Forget. The complete source-scope proof
// rejects an unscoped decision rather than silently retaining it.
func (w *Work) Reindex(root Guard, plan Reindex) (Guard, bool) {
	if !w.Valid(root) || !plan.Valid() || plan.value.manager != w.manager {
		return Guard{}, false
	}
	if isTerminal(root) {
		return root, true
	}
	if plan.CoordinateIdentity() {
		// A coordinate-identical relation is the one zero-copy path. It still
		// needs the candidate-aware source-scope proof: separately issued
		// source/target Scopes may have the same coordinate payload, while an
		// out-of-source Guard must never be retained through either identity.
		if !w.scopeContains(plan.Source(), root) {
			return Guard{}, false
		}
		return root, true
	}
	// Pure projections are the common carrier relation: every source
	// coordinate is either retained or existentially forgotten.  The
	// projection traversal below is also the complete source-scope proof, so
	// do not first fold the root and then traverse it again.  Retained
	// coordinates can be rebuilt directly; forgotten coordinates only need the
	// exact OR of their already-transported branches.  General Set relations
	// retain the relational traversal below, including its explicit source
	// proof, because their target expressions need the full ITE operation.
	if plan.PureProjection() {
		return w.reindexProjection(root, plan)
	}
	if !w.scopeContains(plan.Source(), root) {
		return Guard{}, false
	}
	resolved := make(map[Guard]Guard)
	stack := []unaryFrame{{guard: root}}
	for len(stack) != 0 {
		frame := &stack[len(stack)-1]
		if result, done := resolvedGuard(frame.guard, resolved); done {
			_ = result
			stack = stack[:len(stack)-1]
			continue
		}
		n := w.node(frame.guard)
		switch frame.phase {
		case 0:
			frame.phase = 1
			if _, done := resolvedGuard(n.low, resolved); !done {
				stack = append(stack, unaryFrame{guard: n.low})
			}
		case 1:
			frame.phase = 2
			if _, done := resolvedGuard(n.high, resolved); !done {
				stack = append(stack, unaryFrame{guard: n.high})
			}
		default:
			low, _ := resolvedGuard(n.low, resolved)
			high, _ := resolvedGuard(n.high, resolved)
			action, valid := plan.action(w.manager.atom(n.rank))
			if !valid {
				return Guard{}, false
			}
			lowRegion, lowOK := action.Low()
			highRegion, highOK := action.High()
			if !lowOK || !highOK {
				return Guard{}, false
			}
			low = w.applyNode(andOperation, lowRegion, low)
			high = w.applyNode(andOperation, highRegion, high)
			resolved[frame.guard] = w.applyNode(orOperation, low, high)
			stack = stack[:len(stack)-1]
		}
	}
	return resolved[root], true
}

// reindexProjection transports a pure retain/forget relation in one complete
// postorder traversal.  Looking up the action for every visited source node
// simultaneously proves that the input root is contained by plan.Source;
// there is no separate scope fold.  A retained action preserves the source
// decision rank and therefore uses the canonical node constructor directly.
// A forgotten action is existential closure, exactly low OR high.
func (w *Work) reindexProjection(root Guard, plan Reindex) (Guard, bool) {
	resolved := make(map[Guard]Guard)
	stack := []unaryFrame{{guard: root}}
	for len(stack) != 0 {
		if !w.Live() {
			return Guard{}, false
		}
		frame := &stack[len(stack)-1]
		if _, done := resolvedGuard(frame.guard, resolved); done {
			stack = stack[:len(stack)-1]
			continue
		}
		n := w.node(frame.guard)
		action, valid := plan.ProjectionAction(w.manager.atom(n.rank))
		if !valid {
			// The input mentions a source coordinate not admitted by this
			// complete relation. No candidate is published by this method;
			// the support owner discards its enclosing transaction.
			return Guard{}, false
		}
		switch frame.phase {
		case 0:
			frame.phase = 1
			if _, done := resolvedGuard(n.low, resolved); !done {
				stack = append(stack, unaryFrame{guard: n.low})
			}
		case 1:
			frame.phase = 2
			if _, done := resolvedGuard(n.high, resolved); !done {
				stack = append(stack, unaryFrame{guard: n.high})
			}
		default:
			low, _ := resolvedGuard(n.low, resolved)
			high, _ := resolvedGuard(n.high, resolved)
			var result Guard
			if action.RetainsCoordinate() {
				result = w.nodeOrExisting(n.rank, low, high, frame.guard, Guard{})
			} else if action.ForgetsCoordinate() {
				result = w.applyNode(orOperation, low, high)
			} else {
				return Guard{}, false
			}
			if !w.Live() {
				return Guard{}, false
			}
			resolved[frame.guard] = result
			stack = stack[:len(stack)-1]
		}
	}
	return resolved[root], true
}

func (w *Work) scopeContains(scope Scope, root Guard) bool {
	if !scope.Valid() || scope.Manager() != w.manager || !w.Valid(root) {
		return false
	}
	completed, valid := w.Fold(root, func(_ Guard, view Decomposition) bool {
		return view.Terminal || scope.contains(view.Atom)
	})
	return completed && valid
}

// ComposeReindex seals tau o rho. It is a cold operation used by the equation
// binder; execution still receives one immutable relation. Both relational
// source-value regions from rho are transformed through tau in one candidate
// transaction before the composite is exposed.
func (m *Manager) ComposeReindex(first, second Reindex) (Reindex, bool) {
	if m == nil || !first.Valid() || !second.Valid() || first.value.manager != m || second.value.manager != m || !first.Target().Same(second.Source()) {
		return Reindex{}, false
	}
	work := m.NewWork()
	entries := make([]reindexEntryRow, len(first.value.entries))
	for index, firstEntry := range first.value.entries {
		entry := firstEntry.reindexEntry
		low, ok := work.Reindex(entry.low, second)
		if !ok {
			work.Discard()
			return Reindex{}, false
		}
		high, ok := work.Reindex(entry.high, second)
		if !ok {
			work.Discard()
			return Reindex{}, false
		}
		entries[index] = reindexEntryRow{rank: firstEntry.rank, reindexEntry: reindexEntry{low: low, high: high, projection: projectionGeneral}}
	}
	work.Seal()
	// Both derived proofs follow Seal exactly. Coordinate identity composes
	// because the shared intermediate scope forces the outer rank vectors to
	// agree coordinate by coordinate; full identity additionally needs the
	// composite to return to its one issued source scope.
	coordinateIdentity := first.CoordinateIdentity() && second.CoordinateIdentity()
	identity := coordinateIdentity && first.Source().Same(second.Target())
	if coordinateIdentity {
		for index := range entries {
			entries[index].identity = true
		}
	}
	result := Reindex{value: &reindex{manager: m, source: first.Source(), target: second.Target(), entries: entries, identity: identity, coordinateIdentity: coordinateIdentity, sealed: true}}
	if !result.Valid() {
		return Reindex{}, false
	}
	return result, true
}
