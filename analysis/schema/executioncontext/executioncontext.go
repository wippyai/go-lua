// Package executioncontext owns the Link-scoped execution-context scalar
// directory.  Contexts are deliberately flat identities: the module/cache
// owner admits AnalysisRoot at the boundary, then every later relation carries
// only the resulting Context identity.  No root or owner pointer crosses this
// package's seal.
package executioncontext

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
)

const (
	contextDomain    = "analysis/schema/execution-context/context/v1"
	rootDomain       = "analysis/schema/execution-context/root-context/v1"
	transitionDomain = "analysis/schema/execution-context/transition/v1"
)

// Context is one Link-owned execution context.  Its identity is exactly the
// ordered tuple (LinkID, ModuleKey, ActorID, RepresentativeCacheInstanceID).
// Cache aliases therefore quotient at the representative identity: two roots
// that use different aliased instances but the same representative name one
// Context, while a module, actor, or representative change names another.
//
// The fields are private on purpose.  A row can enter a Directory only through
// NewContext, which computes and authenticates its identity once.
type Context struct {
	id, link, module, actor, representative identity.ContentID
}

// NewContext constructs one authenticated scalar Context.
func NewContext(linkID, moduleKey, actorID, representativeCacheInstanceID identity.ContentID) (Context, bool) {
	if !linkID.Available() || !moduleKey.Available() || !actorID.Available() || !representativeCacheInstanceID.Available() {
		return Context{}, false
	}
	id, ok := ContextIdentity(linkID, moduleKey, actorID, representativeCacheInstanceID)
	if !ok {
		return Context{}, false
	}
	row := Context{id: id, link: linkID, module: moduleKey, actor: actorID, representative: representativeCacheInstanceID}
	return row, row.Available()
}

// ContextIdentity derives the canonical Context ID from the four-coordinate
// tuple.  It is exported for consumers that must authenticate an independently
// carried Context ID without reopening a Context row.
func ContextIdentity(linkID, moduleKey, actorID, representativeCacheInstanceID identity.ContentID) (identity.ContentID, bool) {
	if !linkID.Available() || !moduleKey.Available() || !actorID.Available() || !representativeCacheInstanceID.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(contextDomain, linkID[:], moduleKey[:], actorID[:], representativeCacheInstanceID[:])
}

// Available reports whether the row is a fully authenticated Context.
func (row Context) Available() bool {
	return row.id.Available() && row.link.Available() && row.module.Available() && row.actor.Available() && row.representative.Available() &&
		row.id == mustContextIdentity(row.link, row.module, row.actor, row.representative)
}

func mustContextIdentity(linkID, moduleKey, actorID, representativeCacheInstanceID identity.ContentID) identity.ContentID {
	id, _ := ContextIdentity(linkID, moduleKey, actorID, representativeCacheInstanceID)
	return id
}

// ID returns the authenticated Context identity, or zero for an unavailable
// row.
func (row Context) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row Context) LinkID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.link
}

func (row Context) ModuleKey() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.module
}

func (row Context) ActorID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.actor
}

func (row Context) RepresentativeCacheInstanceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.representative
}

// RootContext is the one ingress row from an AnalysisRoot into a Context.
// Root IDs never participate in Context identity; they are retained only on
// this boundary row so a root can be resolved exactly once by its owner.
type RootContext struct {
	id, link, root, context identity.ContentID
}

// NewRootContext constructs an authenticated AnalysisRoot-to-Context ingress
// row.  The Context's Link ID must be the same Link named by the row.
func NewRootContext(linkID, analysisRootID, contextID identity.ContentID) (RootContext, bool) {
	if !linkID.Available() || !analysisRootID.Available() || !contextID.Available() {
		return RootContext{}, false
	}
	id, ok := identity.DeriveContentID(rootDomain, linkID[:], analysisRootID[:], contextID[:])
	if !ok {
		return RootContext{}, false
	}
	row := RootContext{id: id, link: linkID, root: analysisRootID, context: contextID}
	return row, row.Available()
}

func (row RootContext) Available() bool {
	if !row.id.Available() || !row.link.Available() || !row.root.Available() || !row.context.Available() {
		return false
	}
	id, ok := identity.DeriveContentID(rootDomain, row.link[:], row.root[:], row.context[:])
	return ok && id == row.id
}

func (row RootContext) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row RootContext) LinkID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.link
}

// AnalysisRootID returns the root identity carried only by this ingress row.
func (row RootContext) AnalysisRootID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.root
}

func (row RootContext) ContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.context
}

// Transition is an exact source-to-target Context edge.  It intentionally
// carries no AnalysisRoot IDs: roots are ingress coordinates, not runtime
// transition coordinates.
type Transition struct {
	id, link, from, to identity.ContentID
}

// NewTransition constructs one authenticated Context transition.
func NewTransition(linkID, fromContextID, toContextID identity.ContentID) (Transition, bool) {
	if !linkID.Available() || !fromContextID.Available() || !toContextID.Available() {
		return Transition{}, false
	}
	id, ok := identity.DeriveContentID(transitionDomain, linkID[:], fromContextID[:], toContextID[:])
	if !ok {
		return Transition{}, false
	}
	row := Transition{id: id, link: linkID, from: fromContextID, to: toContextID}
	return row, row.Available()
}

func (row Transition) Available() bool {
	if !row.id.Available() || !row.link.Available() || !row.from.Available() || !row.to.Available() {
		return false
	}
	id, ok := identity.DeriveContentID(transitionDomain, row.link[:], row.from[:], row.to[:])
	return ok && id == row.id
}

func (row Transition) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row Transition) LinkID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.link
}

func (row Transition) FromContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.from
}

func (row Transition) ToContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.to
}

// Directory is a frozen scalar Context, root-ingress, and transition
// directory.  Seal copies and canonicalizes all rows; the returned value has
// no owner pointer and exposes no mutator.
type Directory struct {
	link        identity.ContentID
	contexts    []Context
	roots       []RootContext
	transitions []Transition
	sealed      bool
}

// Seal validates and freezes a complete Link directory.  It refuses malformed
// rows, duplicate/conflicting roots or transitions, unrooted contexts,
// transition endpoints outside the context set, authored reflexive
// transitions, and rows from another Link.  The reflexive edge for every
// sealed Context is issued here as part of the canonical directory; callers
// author only cross-context edges.  This makes the relation total for local
// execution without making a consumer infer a current context or enumerate
// sibling contexts.  Sorting by scalar identities makes the sealed result
// independent of input permutation.
func Seal(linkID identity.ContentID, contexts []Context, roots []RootContext, transitions []Transition) (Directory, bool) {
	if !linkID.Available() || len(contexts) == 0 || len(roots) == 0 {
		return Directory{}, false
	}

	contextRows := append([]Context(nil), contexts...)
	rootRows := append([]RootContext(nil), roots...)
	transitionRows := append([]Transition(nil), transitions...)
	for _, row := range contextRows {
		if !row.Available() || row.LinkID() != linkID {
			return Directory{}, false
		}
	}
	for _, row := range rootRows {
		if !row.Available() || row.LinkID() != linkID {
			return Directory{}, false
		}
	}
	for _, row := range transitionRows {
		if !row.Available() || row.LinkID() != linkID {
			return Directory{}, false
		}
	}

	sort.Slice(contextRows, func(i, j int) bool { return lessID(contextRows[i].ID(), contextRows[j].ID()) })
	for i := 1; i < len(contextRows); i++ {
		if contextRows[i-1].ID() == contextRows[i].ID() {
			return Directory{}, false
		}
	}

	sort.Slice(rootRows, func(i, j int) bool {
		left, right := rootRows[i].AnalysisRootID(), rootRows[j].AnalysisRootID()
		if left != right {
			return lessID(left, right)
		}
		return lessID(rootRows[i].ID(), rootRows[j].ID())
	})
	for i := 1; i < len(rootRows); i++ {
		// A root is a total function into exactly one Context.  Distinct row
		// identities with the same root therefore signal a conflict.
		if rootRows[i-1].AnalysisRootID() == rootRows[i].AnalysisRootID() {
			return Directory{}, false
		}
	}

	// Every root must resolve to one sealed Context, and every Context must
	// have at least one ingress root.  The latter is the root-totality law:
	// no free-floating Context can enter the directory.
	rooted := make(map[identity.ContentID]struct{}, len(rootRows))
	contextAt := make(map[identity.ContentID]Context, len(contextRows))
	for _, row := range contextRows {
		contextAt[row.ID()] = row
	}
	for _, row := range rootRows {
		if _, ok := contextAt[row.ContextID()]; !ok {
			return Directory{}, false
		}
		rooted[row.ContextID()] = struct{}{}
	}
	if len(rooted) != len(contextRows) {
		return Directory{}, false
	}

	sort.Slice(transitionRows, func(i, j int) bool { return lessID(transitionRows[i].ID(), transitionRows[j].ID()) })
	for i := 1; i < len(transitionRows); i++ {
		if transitionRows[i-1].ID() == transitionRows[i].ID() {
			return Directory{}, false
		}
	}
	for _, row := range transitionRows {
		if _, ok := contextAt[row.FromContextID()]; !ok {
			return Directory{}, false
		}
		if _, ok := contextAt[row.ToContextID()]; !ok {
			return Directory{}, false
		}
		// Reflexive transitions are not authored data.  They are the one
		// canonical local edge issued below, so accepting one here would make
		// the source of that edge ambiguous and would permit duplicate
		// authorities at the directory boundary.
		if row.FromContextID() == row.ToContextID() {
			return Directory{}, false
		}
		// A module cache is actor-local, so the composition that authors these
		// edges never crosses an actor.  Admitting one would put a transition
		// in the relation that no execution reaches and would break the
		// containment of the authored relation in the activation relation.
		if contextAt[row.FromContextID()].ActorID() != contextAt[row.ToContextID()].ActorID() {
			return Directory{}, false
		}
	}

	// Every sealed Context has exactly one local execution edge.  Issue these
	// rows from the already-authenticated Context IDs, then sort the complete
	// relation once more so its public order remains permutation-independent.
	for _, context := range contextRows {
		transition, ok := NewTransition(linkID, context.ID(), context.ID())
		if !ok {
			return Directory{}, false
		}
		transitionRows = append(transitionRows, transition)
	}
	sort.Slice(transitionRows, func(i, j int) bool { return lessID(transitionRows[i].ID(), transitionRows[j].ID()) })

	return Directory{link: linkID, contexts: contextRows, roots: rootRows, transitions: transitionRows, sealed: true}, true
}

func lessID(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

// Available reports whether the directory passed all seal laws.
func (directory Directory) Available() bool {
	return directory.sealed && directory.link.Available() && len(directory.contexts) != 0 && len(directory.roots) != 0
}

func (directory Directory) LinkID() identity.ContentID {
	if !directory.Available() {
		return identity.ContentID{}
	}
	return directory.link
}

func (directory Directory) ContextCount() int {
	if !directory.Available() {
		return 0
	}
	return len(directory.contexts)
}

func (directory Directory) ContextAt(index int) (Context, bool) {
	if !directory.Available() || index < 0 || index >= len(directory.contexts) {
		return Context{}, false
	}
	return directory.contexts[index], true
}

func (directory Directory) Context(id identity.ContentID) (Context, bool) {
	if !directory.Available() || !id.Available() {
		return Context{}, false
	}
	for _, row := range directory.contexts {
		if row.ID() == id {
			return row, true
		}
	}
	return Context{}, false
}

func (directory Directory) RootCount() int {
	if !directory.Available() {
		return 0
	}
	return len(directory.roots)
}

func (directory Directory) RootAt(index int) (RootContext, bool) {
	if !directory.Available() || index < 0 || index >= len(directory.roots) {
		return RootContext{}, false
	}
	return directory.roots[index], true
}

// RootContext resolves the exact ingress row for one AnalysisRoot identity.
func (directory Directory) RootContext(rootID identity.ContentID) (RootContext, bool) {
	if !directory.Available() || !rootID.Available() {
		return RootContext{}, false
	}
	for _, row := range directory.roots {
		if row.AnalysisRootID() == rootID {
			return row, true
		}
	}
	return RootContext{}, false
}

// ContextForRoot resolves a root directly to its Context while keeping the
// root itself at the ingress boundary.
func (directory Directory) ContextForRoot(rootID identity.ContentID) (Context, bool) {
	root, ok := directory.RootContext(rootID)
	if !ok {
		return Context{}, false
	}
	return directory.Context(root.ContextID())
}

func (directory Directory) TransitionCount() int {
	if !directory.Available() {
		return 0
	}
	return len(directory.transitions)
}

func (directory Directory) TransitionAt(index int) (Transition, bool) {
	if !directory.Available() || index < 0 || index >= len(directory.transitions) {
		return Transition{}, false
	}
	return directory.transitions[index], true
}

// Transition resolves one exact source-to-target edge. Multiple outgoing
// targets from one source are legal, so callers provide both endpoints.
func (directory Directory) Transition(fromID, toID identity.ContentID) (Transition, bool) {
	if !directory.Available() || !fromID.Available() || !toID.Available() {
		return Transition{}, false
	}
	for _, row := range directory.transitions {
		if row.FromContextID() == fromID && row.ToContextID() == toID {
			return row, true
		}
	}
	return Transition{}, false
}

// ActivationEdge resolves the execution-context edge one activation route
// occupies: the application is performed in the source Context and enters a
// body whose module resides in the target Context.
//
// This is not the module-call relation. A module-call transition is authored
// by one import: it names the exact require that instantiates a module cache.
// An activation is a plain application of a callable value, and a callable
// admitted into an actor may be applied at any call site that actor executes -
// a callback handed down to an imported library, a value re-exported through
// a third module, or an opaque call value that names no body at all. The
// activation relation is therefore exactly co-residence in one actor of one
// Link, and the edge is derived from the two authenticated Contexts rather
// than materialized: the relation is quadratic in Context count while the
// authored transition relation is not.
//
// Contexts in different actors are never activation-connected. A value that
// crosses an actor boundary is transferred, not applied in place, so a route
// between two actors names an application no execution reaches.
//
// The derived edge is identity-equal to the authored row whenever the same
// pair also carries a module-call transition, because both derive the row
// from the same Link and endpoint pair. The authored relation is a
// subrelation of this one and neither becomes a second authority.
func (directory Directory) ActivationEdge(fromID, toID identity.ContentID) (Transition, bool) {
	from, fromOK := directory.Context(fromID)
	to, toOK := directory.Context(toID)
	if !fromOK || !toOK || from.ActorID() != to.ActorID() {
		return Transition{}, false
	}
	return NewTransition(directory.link, from.ID(), to.ID())
}

// ContextsForModule resolves every sealed Context one module holds. Directory
// order is canonical, so the result is permutation-independent and no first
// or default Context is selected. A module with no Context is not a caller
// error to recover from: the boolean states that the directory has nothing to
// say about it.
func (directory Directory) ContextsForModule(moduleKey identity.ContentID) ([]Context, bool) {
	if !directory.Available() || !moduleKey.Available() {
		return nil, false
	}
	rows := make([]Context, 0, len(directory.contexts))
	for _, row := range directory.contexts {
		if row.ModuleKey() != moduleKey {
			continue
		}
		rows = append(rows, row)
	}
	return rows, len(rows) != 0
}
