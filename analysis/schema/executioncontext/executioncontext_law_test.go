package executioncontext_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

func lawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("execution-context-law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

func context(t *testing.T, link, module, actor, representative string) executioncontext.Context {
	t.Helper()
	row, ok := executioncontext.NewContext(lawID(t, link), lawID(t, module), lawID(t, actor), lawID(t, representative))
	if !ok {
		t.Fatalf("context %s", module)
	}
	return row
}

func TestContextIDIsInjectiveAcrossItsTuple(t *testing.T) {
	base := context(t, "link", "module", "actor", "representative")
	variants := []executioncontext.Context{
		context(t, "other-link", "module", "actor", "representative"),
		context(t, "link", "other-module", "actor", "representative"),
		context(t, "link", "module", "other-actor", "representative"),
		context(t, "link", "module", "actor", "other-representative"),
	}
	for index, variant := range variants {
		if !variant.Available() || variant.ID() == base.ID() {
			baseID := base.ID()
			t.Fatalf("tuple coordinate %d collapsed into %x", index, baseID[:4])
		}
	}
}

func TestAliasedRootsQuotientByRepresentative(t *testing.T) {
	linkID := lawID(t, "link")
	moduleID, actorID, representativeID := lawID(t, "module"), lawID(t, "actor"), lawID(t, "representative")
	first, firstOK := executioncontext.NewContext(linkID, moduleID, actorID, representativeID)
	second, secondOK := executioncontext.NewContext(linkID, moduleID, actorID, representativeID)
	if !firstOK || !secondOK || first.ID() != second.ID() {
		t.Fatal("aliased cache instances did not share their representative context")
	}
	rootA, rootAOK := executioncontext.NewRootContext(linkID, lawID(t, "root-a"), first.ID())
	rootB, rootBOK := executioncontext.NewRootContext(linkID, lawID(t, "root-b"), second.ID())
	directory, sealed := executioncontext.Seal(linkID, []executioncontext.Context{first}, []executioncontext.RootContext{rootA, rootB}, nil)
	if !rootAOK || !rootBOK || !sealed || !directory.Available() || directory.ContextCount() != 1 {
		t.Fatal("aliased roots did not seal as one context quotient")
	}
}

func TestDirectoryEnforcesRootTotalityAndEndpointClosure(t *testing.T) {
	linkID := lawID(t, "link")
	left := context(t, "link", "left", "actor", "representative")
	right := context(t, "link", "right", "actor", "representative")
	root, rootOK := executioncontext.NewRootContext(linkID, lawID(t, "left-root"), left.ID())
	if !rootOK {
		t.Fatal("root ingress")
	}
	if _, sealed := executioncontext.Seal(linkID, []executioncontext.Context{left, right}, []executioncontext.RootContext{root}, nil); sealed {
		t.Fatal("unrooted context entered directory")
	}
	transition, transitionOK := executioncontext.NewTransition(linkID, left.ID(), right.ID())
	if !transitionOK {
		t.Fatal("transition")
	}
	if _, sealed := executioncontext.Seal(linkID, []executioncontext.Context{left}, []executioncontext.RootContext{root}, []executioncontext.Transition{transition}); sealed {
		t.Fatal("transition endpoint escaped context directory")
	}
}

func TestDirectorySealIsPermutationDeterministic(t *testing.T) {
	linkID := lawID(t, "link")
	left := context(t, "link", "left", "actor", "representative")
	right := context(t, "link", "right", "actor", "representative")
	leftRoot, _ := executioncontext.NewRootContext(linkID, lawID(t, "left-root"), left.ID())
	rightRoot, _ := executioncontext.NewRootContext(linkID, lawID(t, "right-root"), right.ID())
	forward, _ := executioncontext.NewTransition(linkID, left.ID(), right.ID())
	reverse, _ := executioncontext.NewTransition(linkID, right.ID(), left.ID())
	first, firstOK := executioncontext.Seal(linkID, []executioncontext.Context{left, right}, []executioncontext.RootContext{leftRoot, rightRoot}, []executioncontext.Transition{forward, reverse})
	second, secondOK := executioncontext.Seal(linkID, []executioncontext.Context{right, left}, []executioncontext.RootContext{rightRoot, leftRoot}, []executioncontext.Transition{reverse, forward})
	if !firstOK || !secondOK || first.ContextCount() != second.ContextCount() || first.RootCount() != second.RootCount() || first.TransitionCount() != second.TransitionCount() {
		t.Fatal("permuted directories differ in cardinality")
	}
	for index := 0; index < first.ContextCount(); index++ {
		leftRow, leftOK := first.ContextAt(index)
		rightRow, rightOK := second.ContextAt(index)
		if !leftOK || !rightOK || leftRow.ID() != rightRow.ID() {
			t.Fatalf("context permutation changed row %d", index)
		}
	}
	for index := 0; index < first.RootCount(); index++ {
		leftRow, leftOK := first.RootAt(index)
		rightRow, rightOK := second.RootAt(index)
		if !leftOK || !rightOK || leftRow.ID() != rightRow.ID() {
			t.Fatalf("root permutation changed row %d", index)
		}
	}
	for index := 0; index < first.TransitionCount(); index++ {
		leftRow, leftOK := first.TransitionAt(index)
		rightRow, rightOK := second.TransitionAt(index)
		if !leftOK || !rightOK || leftRow.ID() != rightRow.ID() {
			t.Fatalf("transition permutation changed row %d", index)
		}
	}
}

func TestDirectoryRefusesDuplicateAndConflictingRows(t *testing.T) {
	linkID := lawID(t, "link")
	left := context(t, "link", "left", "actor", "representative")
	right := context(t, "link", "right", "actor", "representative")
	leftRoot, _ := executioncontext.NewRootContext(linkID, lawID(t, "left-root"), left.ID())
	rightRoot, _ := executioncontext.NewRootContext(linkID, lawID(t, "right-root"), right.ID())
	transition, _ := executioncontext.NewTransition(linkID, left.ID(), right.ID())
	if _, sealed := executioncontext.Seal(linkID, []executioncontext.Context{left, left}, []executioncontext.RootContext{leftRoot}, nil); sealed {
		t.Fatal("duplicate context row sealed")
	}
	conflictingRoot, _ := executioncontext.NewRootContext(linkID, leftRoot.AnalysisRootID(), right.ID())
	if _, sealed := executioncontext.Seal(linkID, []executioncontext.Context{left, right}, []executioncontext.RootContext{leftRoot, conflictingRoot}, nil); sealed {
		t.Fatal("conflicting root mapping sealed")
	}
	if _, sealed := executioncontext.Seal(linkID, []executioncontext.Context{left, right}, []executioncontext.RootContext{leftRoot, rightRoot}, []executioncontext.Transition{transition, transition}); sealed {
		t.Fatal("duplicate transition sealed")
	}
	if _, sealed := executioncontext.Seal(linkID, []executioncontext.Context{left, right}, []executioncontext.RootContext{leftRoot, rightRoot}, []executioncontext.Transition{transition}); !sealed {
		t.Fatal("valid scalar directory refused")
	}
}

func TestDirectoryResolvesExactTransitionEndpoints(t *testing.T) {
	linkID := lawID(t, "link")
	left := context(t, "link", "left", "actor", "representative")
	right := context(t, "link", "right", "actor", "representative")
	leftRoot, _ := executioncontext.NewRootContext(linkID, lawID(t, "left-root"), left.ID())
	rightRoot, _ := executioncontext.NewRootContext(linkID, lawID(t, "right-root"), right.ID())
	transition, _ := executioncontext.NewTransition(linkID, left.ID(), right.ID())
	directory, sealed := executioncontext.Seal(linkID, []executioncontext.Context{left, right}, []executioncontext.RootContext{leftRoot, rightRoot}, []executioncontext.Transition{transition})
	if !sealed {
		t.Fatal("directory")
	}
	resolved, resolvedOK := directory.Transition(left.ID(), right.ID())
	if !resolvedOK || resolved.ID() != transition.ID() {
		t.Fatal("exact transition lookup failed")
	}
	if _, found := directory.Transition(right.ID(), left.ID()); found {
		t.Fatal("reverse transition fabricated")
	}
}

func TestDirectoryIssuesOneCanonicalReflexiveTransitionPerContext(t *testing.T) {
	linkID := lawID(t, "reflexive-link")
	left := context(t, "reflexive-link", "left", "actor", "representative")
	right := context(t, "reflexive-link", "right", "actor", "representative")
	leftRoot, leftRootOK := executioncontext.NewRootContext(linkID, lawID(t, "reflexive-left-root"), left.ID())
	rightRoot, rightRootOK := executioncontext.NewRootContext(linkID, lawID(t, "reflexive-right-root"), right.ID())
	cross, crossOK := executioncontext.NewTransition(linkID, left.ID(), right.ID())
	if !leftRootOK || !rightRootOK || !crossOK {
		t.Fatal("construct reflexive law rows")
	}
	directory, sealed := executioncontext.Seal(linkID,
		[]executioncontext.Context{right, left},
		[]executioncontext.RootContext{rightRoot, leftRoot},
		[]executioncontext.Transition{cross})
	if !sealed || !directory.Available() {
		t.Fatal("directory with canonical reflexive rows did not seal")
	}
	if got, want := directory.TransitionCount(), 3; got != want {
		t.Fatalf("transition count = %d, want %d (two reflexive plus one authored cross edge)", got, want)
	}
	seenReflexive := 0
	for index := 0; index < directory.TransitionCount(); index++ {
		row, ok := directory.TransitionAt(index)
		if !ok {
			t.Fatalf("transition %d unavailable", index)
		}
		if row.FromContextID() != row.ToContextID() {
			continue
		}
		seenReflexive++
		canonical, canonicalOK := executioncontext.NewTransition(linkID, row.FromContextID(), row.FromContextID())
		if !canonicalOK || row.ID() != canonical.ID() {
			t.Fatalf("reflexive transition %d is not canonical", index)
		}
	}
	if seenReflexive != directory.ContextCount() {
		t.Fatalf("reflexive transition count = %d, want one per context (%d)", seenReflexive, directory.ContextCount())
	}
	for _, context := range []executioncontext.Context{left, right} {
		row, ok := directory.Transition(context.ID(), context.ID())
		if !ok || row.FromContextID() != context.ID() || row.ToContextID() != context.ID() {
			contextID := context.ID()
			t.Fatalf("missing exact local transition for context %x", contextID[:4])
		}
	}
	resolved, resolvedOK := directory.Transition(left.ID(), right.ID())
	if !resolvedOK || resolved.ID() != cross.ID() {
		t.Fatal("authored cross-context edge was not retained exactly")
	}
}

func TestDirectoryRejectsAuthoredReflexiveTransition(t *testing.T) {
	linkID := lawID(t, "authored-reflexive-link")
	row := context(t, "authored-reflexive-link", "module", "actor", "representative")
	root, rootOK := executioncontext.NewRootContext(linkID, lawID(t, "authored-reflexive-root"), row.ID())
	reflexive, transitionOK := executioncontext.NewTransition(linkID, row.ID(), row.ID())
	if !rootOK || !transitionOK {
		t.Fatal("construct authored reflexive rows")
	}
	if _, sealed := executioncontext.Seal(linkID, []executioncontext.Context{row}, []executioncontext.RootContext{root}, []executioncontext.Transition{reflexive}); sealed {
		t.Fatal("directory accepted an authored reflexive edge instead of issuing its canonical edge")
	}
}

// The activation relation answers where a callable value may be applied, and
// that is not the authored module-call relation. A call value names any
// admitted body, so a trigger reaches a body in a module its own module never
// imports, and it reaches one in the module that imports it. Both directions
// hold whenever the two Contexts execute in one actor, the reflexive pair
// holds for an intra-module body, and the derived row is identity-equal to the
// authored transition wherever the same pair also carries one.
func TestActivationEdgeConnectsEveryContextPairOfOneActor(t *testing.T) {
	linkID := lawID(t, "activation-link")
	importer := context(t, "activation-link", "importer", "actor", "importer-instance")
	imported := context(t, "activation-link", "imported", "actor", "imported-instance")
	sibling := context(t, "activation-link", "sibling", "actor", "sibling-instance")
	roots := []executioncontext.RootContext{
		rootFor(t, linkID, "activation-importer-root", importer),
		rootFor(t, linkID, "activation-imported-root", imported),
		rootFor(t, linkID, "activation-sibling-root", sibling),
	}
	authored, authoredOK := executioncontext.NewTransition(linkID, importer.ID(), imported.ID())
	if !authoredOK {
		t.Fatal("construct the authored module-call edge")
	}
	directory, sealed := executioncontext.Seal(linkID,
		[]executioncontext.Context{importer, imported, sibling}, roots,
		[]executioncontext.Transition{authored})
	if !sealed {
		t.Fatal("seal the activation law directory")
	}

	pairs := []struct {
		name     string
		from, to executioncontext.Context
	}{
		{"along the authored import", importer, imported},
		{"against the authored import", imported, importer},
		{"between siblings the composition never connects", sibling, imported},
		{"into the trigger's own module", sibling, sibling},
	}
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			edge, ok := directory.ActivationEdge(pair.from.ID(), pair.to.ID())
			if !ok || !edge.Available() {
				t.Fatal("an activation route inside one actor was not connected")
			}
			if edge.LinkID() != linkID || edge.FromContextID() != pair.from.ID() || edge.ToContextID() != pair.to.ID() {
				t.Fatal("the activation edge names endpoints other than the route's own")
			}
			canonical, canonicalOK := executioncontext.NewTransition(linkID, pair.from.ID(), pair.to.ID())
			if !canonicalOK || edge.ID() != canonical.ID() {
				t.Fatal("the activation edge is not the canonical row for its endpoints")
			}
		})
	}

	derived, derivedOK := directory.ActivationEdge(importer.ID(), imported.ID())
	if !derivedOK || derived.ID() != authored.ID() {
		t.Fatal("the authored module-call edge and the derived activation edge are not one row")
	}

	// The two relations stay distinct: the authored one still names exactly
	// the one import the composition declared, so an activation route that
	// resolved on the transition relation would refuse the routes above.
	if _, ok := directory.Transition(imported.ID(), importer.ID()); ok {
		t.Fatal("the authored transition relation gained the reverse of an import")
	}
	if _, ok := directory.Transition(sibling.ID(), imported.ID()); ok {
		t.Fatal("the authored transition relation gained a pair the composition never declared")
	}
}

// A value that crosses an actor boundary is transferred, not applied in place,
// so no activation route connects two actors. A Context the directory never
// sealed connects to nothing at all.
func TestActivationEdgeRefusesAcrossActorsAndOutsideTheDirectory(t *testing.T) {
	linkID := lawID(t, "activation-actor-link")
	local := context(t, "activation-actor-link", "module", "local-actor", "local-instance")
	remote := context(t, "activation-actor-link", "module", "remote-actor", "remote-instance")
	directory, sealed := executioncontext.Seal(linkID,
		[]executioncontext.Context{local, remote},
		[]executioncontext.RootContext{
			rootFor(t, linkID, "activation-local-root", local),
			rootFor(t, linkID, "activation-remote-root", remote),
		}, nil)
	if !sealed {
		t.Fatal("seal the cross-actor law directory")
	}
	if _, ok := directory.ActivationEdge(local.ID(), remote.ID()); ok {
		t.Fatal("an activation route reached another actor")
	}
	if _, ok := directory.ActivationEdge(remote.ID(), local.ID()); ok {
		t.Fatal("an activation route reached another actor")
	}
	foreign := context(t, "activation-actor-link", "foreign", "local-actor", "foreign-instance")
	if _, ok := directory.ActivationEdge(local.ID(), foreign.ID()); ok {
		t.Fatal("an activation route reached a Context this directory never sealed")
	}
}

func rootFor(t *testing.T, linkID identity.ContentID, label string, row executioncontext.Context) executioncontext.RootContext {
	t.Helper()
	root, ok := executioncontext.NewRootContext(linkID, lawID(t, label), row.ID())
	if !ok {
		t.Fatalf("root %s", label)
	}
	return root
}

// A module cache is actor-local, so no composition authors a transition
// between two actors. The seal refuses one rather than carrying an edge no
// execution reaches.
func TestDirectoryRejectsAuthoredCrossActorTransition(t *testing.T) {
	linkID := lawID(t, "cross-actor-link")
	local := context(t, "cross-actor-link", "module", "local-actor", "local-instance")
	remote := context(t, "cross-actor-link", "module", "remote-actor", "remote-instance")
	cross, crossOK := executioncontext.NewTransition(linkID, local.ID(), remote.ID())
	if !crossOK {
		t.Fatal("construct the cross-actor edge")
	}
	if _, sealed := executioncontext.Seal(linkID,
		[]executioncontext.Context{local, remote},
		[]executioncontext.RootContext{
			rootFor(t, linkID, "cross-actor-local-root", local),
			rootFor(t, linkID, "cross-actor-remote-root", remote),
		}, []executioncontext.Transition{cross}); sealed {
		t.Fatal("directory accepted a transition between two actors")
	}
}
