package carrier

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// Scope is one Composition-issued finite guard-coordinate interface. States
// carry it explicitly so that equal-looking roots from different recurrence
// boundaries cannot be compared, merged, or contributed together.
type Scope struct {
	composition *Composition
	guard       guard.Scope
}

func (scope Scope) validFor(composition *Composition) bool {
	return composition != nil && scope.composition == composition && scope.guard.Valid() && scope.guard.Manager() == composition.guards
}

func (scope Scope) same(other Scope) bool {
	return scope.validFor(scope.composition) && scope.composition == other.composition && scope.guard.Same(other.guard)
}

// Same proves exact issued-scope identity. It is intentionally stronger than
// equal coordinate membership: separate boundary interfaces stay distinct.
func (scope Scope) Same(other Scope) bool { return scope.same(other) }

// Valid reports whether this is a live immutable scope issued by a sealed
// Composition. The coordinate members remain private to guard.Scope.
func (scope Scope) Valid() bool { return scope.validFor(scope.composition) }

// Expr seals one already-built support BDD as an expression over this Scope.
// It is a cold plan-construction boundary; execution receives only ReindexPlan.
func (scope Scope) Expr(region support.Mask) (Expr, bool) {
	if !scope.Valid() || !region.Valid() || region.Manager() != scope.composition.guards {
		return Expr{}, false
	}
	root, ok := region.Guard()
	if !ok {
		return Expr{}, false
	}
	expression, ok := scope.guard.Expr(root)
	if !ok {
		return Expr{}, false
	}
	return Expr{scope: scope, value: expression}, true
}

// Expr is an opaque target Boolean expression used only while a ReindexPlan
// is sealed. It exposes neither BDD storage nor coordinate substitutions.
type Expr struct {
	scope Scope
	value guard.Expr
}

// ReindexPlan is a Composition-owned immutable total source-to-target
// coordinate relation. It is the only reindex input accepted by Work.
type ReindexPlan struct {
	composition *Composition
	relation    guard.Reindex
}

// Valid reports whether this is a sealed complete carrier relation.  It is
// read-only provenance for the equation/runtime boundary; execution still
// validates exact source/target ownership before transporting a State.
func (plan ReindexPlan) Valid() bool { return plan.validFor(plan.composition) }

// CoordinateIdentity reports whether this sealed relation preserves every
// ranked Boolean coordinate even if source and target are distinct issued
// Scope identities.
func (plan ReindexPlan) CoordinateIdentity() bool { return plan.coordinateIdentity() }

// SelfComposable reports that this relation's target interface is its own
// source interface, so it can be composed with itself and its powers stay
// relations over one Scope. It is the exact structural admission a closure
// basis needs; the Scope objects themselves remain private to Composition.
func (plan ReindexPlan) SelfComposable() bool {
	return plan.validFor(plan.composition) && plan.target().same(plan.source())
}

// CoordinateCount is the width of this relation's source interface. It is the
// relation's own dimension, published so a derivation over the relation can
// be sized without enumerating any Scope.
func (plan ReindexPlan) CoordinateCount() int {
	if !plan.validFor(plan.composition) {
		return 0
	}
	return plan.relation.CoordinateCount()
}

// RelationIdentity is the collision-resistant canonical identity of this
// exact coordinate relation. It forwards the sole guard representation, so
// two plans that relate the same coordinates the same way share one identity
// however they were composed. It is the key a closure basis dedupes on.
func (plan ReindexPlan) RelationIdentity() (guard.FormulaID, bool) {
	if !plan.validFor(plan.composition) {
		return guard.FormulaID{}, false
	}
	return plan.relation.RelationIdentity()
}

func (plan ReindexPlan) validFor(composition *Composition) bool {
	return composition != nil && plan.composition == composition && plan.relation.Valid() && plan.relation.Source().Manager() == composition.guards && plan.relation.Target().Manager() == composition.guards
}

func (plan ReindexPlan) source() Scope {
	if !plan.validFor(plan.composition) {
		return Scope{}
	}
	return Scope{composition: plan.composition, guard: plan.relation.Source()}
}

func (plan ReindexPlan) target() Scope {
	if !plan.validFor(plan.composition) {
		return Scope{}
	}
	return Scope{composition: plan.composition, guard: plan.relation.Target()}
}

func (plan ReindexPlan) identity() bool {
	return plan.validFor(plan.composition) && plan.relation.Identity()
}

func (plan ReindexPlan) coordinateIdentity() bool {
	return plan.validFor(plan.composition) && plan.relation.CoordinateIdentity()
}

// ReindexBuilder is the cold carrier wrapper over guard's relation builder.
// Individual atoms are accepted only during sealing, never by Work.Reindex.
type ReindexBuilder struct {
	composition *Composition
	source      Scope
	target      Scope
	builder     *guard.ReindexBuilder
}

// Scope returns Composition's complete initial coordinate interface.
func (composition *Composition) Scope() Scope {
	if composition == nil || composition.guards == nil {
		return Scope{}
	}
	// A copied Composition is a distinct carrier owner in adversarial tests.
	// Its inherited field still names the original owner, so reissue the one
	// complete scope wrapper without altering the shared guard universe.
	if !composition.scope.validFor(composition) {
		return Scope{composition: composition, guard: composition.guards.AllScope()}
	}
	return composition.scope
}

// SealScope issues one finite coordinate interface before evaluator work
// begins. Raw atoms are confined to this cold declaration boundary.
func (composition *Composition) SealScope(atoms []guard.Atom) (Scope, bool) {
	if composition == nil || composition.guards == nil {
		return Scope{}, false
	}
	composition.scopeMu.Lock()
	defer composition.scopeMu.Unlock()
	if composition.workOpened {
		return Scope{}, false
	}
	sealed, ok := composition.guards.SealScope(atoms)
	if !ok {
		return Scope{}, false
	}
	return Scope{composition: composition, guard: sealed}, true
}

// NewReindex starts a cold total relation between two scopes issued by this
// Composition. Its complete plan must be sealed before evaluator work opens.
func (composition *Composition) NewReindex(source, target Scope) (*ReindexBuilder, bool) {
	return composition.newReindex(source, target, false)
}

// NewRuntimeReindex seals one additional immutable relation after evaluator
// work has opened.  It is intentionally narrower than the ordinary cold
// declaration surface: source and target must already be exact Scopes issued
// by this Composition, and the builder can name only atoms already owned by
// its fixed guard Manager.  It neither issues a Scope nor changes the carrier
// shape, operation catalog, or guard universe.
//
// Runtime topology overlays use this only to lower a selected equation
// boundary whose endpoints were already present in the sealed base graph.
// Callers still receive an ordinary immutable ReindexPlan; Work does not gain
// an authoring API.
func (composition *Composition) NewRuntimeReindex(source, target Scope) (*ReindexBuilder, bool) {
	return composition.newReindex(source, target, true)
}

func (composition *Composition) newReindex(source, target Scope, runtime bool) (*ReindexBuilder, bool) {
	if composition == nil || !source.validFor(composition) || !target.validFor(composition) {
		return nil, false
	}
	composition.scopeMu.Lock()
	defer composition.scopeMu.Unlock()
	if composition.workOpened && !runtime {
		return nil, false
	}
	builder, ok := composition.guards.NewReindex(source.guard, target.guard)
	if !ok {
		return nil, false
	}
	return &ReindexBuilder{composition: composition, source: source, target: target, builder: builder}, true
}

func (builder *ReindexBuilder) Forget(atom guard.Atom) bool {
	return builder != nil && builder.builder != nil && builder.builder.Forget(atom)
}

func (builder *ReindexBuilder) Set(atom guard.Atom, expression Expr) bool {
	return builder != nil && builder.builder != nil && expression.scope.same(builder.target) && builder.builder.Set(atom, expression.value)
}

func (builder *ReindexBuilder) Identity(atom guard.Atom) bool {
	return builder != nil && builder.builder != nil && builder.builder.Identity(atom)
}

// Seal freezes the complete relation. Its source/target scopes are retained
// in the plan and re-proved by Work before it begins any Factor work.
func (builder *ReindexBuilder) Seal() (ReindexPlan, bool) {
	if builder == nil || builder.composition == nil || builder.builder == nil {
		return ReindexPlan{}, false
	}
	relation, ok := builder.builder.Seal()
	builder.builder = nil
	if !ok {
		return ReindexPlan{}, false
	}
	plan := ReindexPlan{composition: builder.composition, relation: relation}
	if !plan.validFor(builder.composition) || !plan.source().same(builder.source) || !plan.target().same(builder.target) {
		return ReindexPlan{}, false
	}
	return plan, true
}

// IdentityReindex seals the complete no-coordinate-change proof for scope.
func (composition *Composition) IdentityReindex(scope Scope) (ReindexPlan, bool) {
	if composition == nil || !scope.validFor(composition) {
		return ReindexPlan{}, false
	}
	composition.scopeMu.Lock()
	defer composition.scopeMu.Unlock()
	if composition.workOpened {
		return ReindexPlan{}, false
	}
	relation, ok := composition.guards.IdentityReindex(scope.guard)
	if !ok {
		return ReindexPlan{}, false
	}
	return ReindexPlan{composition: composition, relation: relation}, true
}

// ComposeReindex seals the exact composed relation for two adjoining plans.
func (composition *Composition) ComposeReindex(first, second ReindexPlan) (ReindexPlan, bool) {
	return composition.composeReindex(first, second, false)
}

// ComposeRuntimeReindex seals one additional composed relation after
// evaluator work has opened. It is the compose-side counterpart of
// NewRuntimeReindex and is narrower for the same reason: both operands must
// already be immutable relations issued by this Composition over already
// issued Scopes, so their composite names no new Scope, atom, operation, or
// carrier shape. Callers still receive an ordinary immutable ReindexPlan;
// Work gains no authoring API.
//
// A closure basis over a settled recurrence boundary uses this to build the
// powers of relations the sealed base graph already carries.
func (composition *Composition) ComposeRuntimeReindex(first, second ReindexPlan) (ReindexPlan, bool) {
	return composition.composeReindex(first, second, true)
}

func (composition *Composition) composeReindex(first, second ReindexPlan, runtime bool) (ReindexPlan, bool) {
	if composition == nil || !first.validFor(composition) || !second.validFor(composition) || !first.target().same(second.source()) {
		return ReindexPlan{}, false
	}
	composition.scopeMu.Lock()
	defer composition.scopeMu.Unlock()
	if composition.workOpened && !runtime {
		return ReindexPlan{}, false
	}
	relation, ok := composition.guards.ComposeReindex(first.relation, second.relation)
	if !ok {
		return ReindexPlan{}, false
	}
	return ReindexPlan{composition: composition, relation: relation}, true
}
