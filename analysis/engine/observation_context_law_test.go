package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// observationContextClone keeps the graph and sealed binding fixed while
// replacing only the committed Link context plane. It is deliberately a
// test-only construction: production never mutates a CommittedProgram after
// construction. The clone lets this law exercise the admission boundary with
// two exact Context rows naming the same mounted module.
func observationContextClone(t testing.TB, source *CommittedProgram, contexts executioncontext.Directory) *CommittedProgram {
	t.Helper()
	if source == nil || !source.valid() || !contexts.Available() {
		t.Fatal("observation context clone inputs")
	}
	index, indexOK := contextfiber.New(contexts, source.graph.PointCount(), source.relation.Generation())
	layout, layoutOK := contextfiber.NewLayoutForGraph(index, contexts, source.pointOwners, source.relation.Generation(), source.graph)
	if !indexOK || !layoutOK {
		t.Fatal("observation context clone plane")
	}
	clone := *source
	clone.self = &clone
	clone.contexts = contexts
	clone.contextIndex = index
	clone.contextLayout = layout
	clone.pointOwners = append([]contextfiber.PointOwner(nil), source.pointOwners...)
	if !clone.sealProgramAdmission() {
		t.Fatal("observation context clone admission")
	}
	return &clone
}

func twoObservationContexts(t testing.TB, source executioncontext.Directory, module identity.ContentID) (executioncontext.Directory, executioncontext.Context, executioncontext.Context) {
	t.Helper()
	if !source.Available() || !module.Available() {
		t.Fatal("observation context directory inputs")
	}
	first, firstOK := source.ContextAt(0)
	if !firstOK || !first.Available() || first.ModuleKey() != module {
		t.Fatal("observation first context")
	}
	second, secondOK := executioncontext.NewContext(source.LinkID(), module, observationContextID("second-actor"), observationContextID("second-representative"))
	linkID := source.LinkID()
	secondContextID := second.ID()
	rootID, rootIDOK := identity.DeriveContentID("analysis/engine/observation-context-root/v1", linkID[:], secondContextID[:])
	root, rootOK := executioncontext.NewRootContext(linkID, rootID, second.ID())
	if !secondOK || !rootIDOK || !rootOK {
		t.Fatal("observation second context")
	}
	contexts := make([]executioncontext.Context, 0, source.ContextCount()+1)
	roots := make([]executioncontext.RootContext, 0, source.RootCount()+1)
	for index := 0; index < source.ContextCount(); index++ {
		row, rowOK := source.ContextAt(index)
		if !rowOK {
			t.Fatal("observation source context")
		}
		contexts = append(contexts, row)
	}
	for index := 0; index < source.RootCount(); index++ {
		row, rowOK := source.RootAt(index)
		if !rowOK {
			t.Fatal("observation source root")
		}
		roots = append(roots, row)
	}
	contexts = append(contexts, second)
	roots = append(roots, root)
	directory, directoryOK := executioncontext.Seal(source.LinkID(), contexts, roots, nil)
	if !directoryOK {
		t.Fatal("observation two-context directory")
	}
	canonicalFirst, firstCanonicalOK := directory.Context(first.ID())
	canonicalSecond, secondCanonicalOK := directory.Context(second.ID())
	if !firstCanonicalOK || !secondCanonicalOK {
		t.Fatal("observation canonical contexts")
	}
	return directory, canonicalFirst, canonicalSecond
}

func observationContextID(label string) identity.ContentID {
	id, _ := identity.DeriveContentID("analysis/engine/observation-context-id/v1", []byte(label))
	return id
}

func TestHeterogeneousObservationBindsDistinctStateRowsForSameModuleContexts(t *testing.T) {
	fixture := newHeterogeneousQueryLawFixture(t)
	first := fixture.observation.Context
	directory, canonicalFirst, second := twoObservationContexts(t, fixture.program.contexts, fixture.observation.Mount)
	if canonicalFirst.ID() != first.ID() || canonicalFirst.ModuleKey() != second.ModuleKey() {
		t.Fatal("observation context canonicalization")
	}
	clone := observationContextClone(t, fixture.program, directory)
	secondID := heterogeneousQueryLawID(19)
	secondObservation, secondOK := NewHeterogeneousObservationAdmission(
		fixture.implementation, secondID, fixture.observation.Role, fixture.observation.Mount,
		fixture.observation.Point, fixture.observation.Occurrence, second,
	)
	if !secondOK {
		t.Fatal("second heterogeneous observation admission")
	}
	solver, failure, sealed := clone.Seal([]ProgramObservationAdmission{fixture.observation, secondObservation})
	if !sealed || solver == nil {
		t.Fatalf("two-context heterogeneous observation seal failure=%v", failure)
	}
	if len(solver.runtime.program.observationTable) != 2 {
		t.Fatalf("observation rows=%d, want 2", len(solver.runtime.program.observationTable))
	}
	left, right := solver.runtime.program.observationTable[0], solver.runtime.program.observationTable[1]
	if left.state == right.state {
		t.Fatalf("same-module contexts collapsed to state row %d", left.state)
	}
	leftCell, leftOK := clone.contextLayout.StateAt(left.state)
	rightCell, rightOK := clone.contextLayout.StateAt(right.state)
	leftContext, leftContextOK := leftCell.ContextOrdinal()
	rightContext, rightContextOK := rightCell.ContextOrdinal()
	if !leftOK || !rightOK || !leftContextOK || !rightContextOK || leftContext == rightContext {
		t.Fatalf("state cells lost exact contexts: left=%v/%v right=%v/%v", leftContext, leftContextOK, rightContext, rightContextOK)
	}
}

func TestObservationAdmissionRefusesZeroForeignAndMismatchedContexts(t *testing.T) {
	fixture := newHeterogeneousQueryLawFixture(t)
	directory, _, _ := twoObservationContexts(t, fixture.program.contexts, fixture.observation.Mount)
	foreignLink, foreignOK := executioncontext.NewContext(observationContextID("foreign-link"), fixture.observation.Mount, observationContextID("foreign-actor"), observationContextID("foreign-representative"))
	foreignLinkID := foreignLink.LinkID()
	foreignContextID := foreignLink.ID()
	foreignRootID, foreignRootIDOK := identity.DeriveContentID("analysis/engine/observation-context-root/v1", foreignLinkID[:], foreignContextID[:])
	foreignRoot, foreignRootOK := executioncontext.NewRootContext(foreignLink.LinkID(), foreignRootID, foreignLink.ID())
	_, foreignDirectoryOK := executioncontext.Seal(foreignLink.LinkID(), []executioncontext.Context{foreignLink}, []executioncontext.RootContext{foreignRoot}, nil)
	if !foreignOK || !foreignRootIDOK || !foreignRootOK || !foreignDirectoryOK {
		t.Fatal("foreign observation context fixture")
	}
	mismatched, mismatchedOK := executioncontext.NewContext(fixture.program.contexts.LinkID(), observationContextID("other-module"), observationContextID("mismatched-actor"), observationContextID("mismatched-representative"))
	programLinkID := fixture.program.contexts.LinkID()
	mismatchedContextID := mismatched.ID()
	mismatchedRootID, mismatchedRootIDOK := identity.DeriveContentID("analysis/engine/observation-context-root/v1", programLinkID[:], mismatchedContextID[:])
	mismatchedRoot, mismatchedRootOK := executioncontext.NewRootContext(programLinkID, mismatchedRootID, mismatched.ID())
	_, mismatchedDirectoryOK := executioncontext.Seal(programLinkID, []executioncontext.Context{mismatched}, []executioncontext.RootContext{mismatchedRoot}, nil)
	if !mismatchedOK || !mismatchedRootIDOK || !mismatchedRootOK || !mismatchedDirectoryOK {
		t.Fatal("mismatched observation context fixture")
	}
	tests := []struct {
		name      string
		context   executioncontext.Context
		directory executioncontext.Directory
		wantSeal  bool
	}{
		{name: "zero", context: executioncontext.Context{}, directory: directory},
		{name: "foreign-link", context: foreignLink, directory: directory},
		{name: "mismatched-module", context: mismatched, directory: directory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			admission, admitted := NewHeterogeneousObservationAdmission(
				fixture.implementation, heterogeneousQueryLawID(30), fixture.observation.Role,
				fixture.observation.Mount, fixture.observation.Point, fixture.observation.Occurrence, test.context,
			)
			if test.context.Available() && !admitted {
				t.Fatal("available context rejected before committed-directory admission")
			}
			if !test.context.Available() && admitted {
				t.Fatal("zero context admitted")
			}
			if !admitted {
				return
			}
			clone := observationContextClone(t, fixture.program, test.directory)
			_, failure, sealed := clone.Seal([]ProgramObservationAdmission{admission})
			if sealed || !failure.Available() {
				t.Fatalf("invalid observation context crossed seal: sealed=%t failure=%v", sealed, failure)
			}
		})
	}
}
