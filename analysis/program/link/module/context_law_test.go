package module

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestBuildContextDirectoryOwnsReflexiveAndAuthoredEdges(t *testing.T) {
	component := sealModuleFixture(t)
	linkID, linkOK := identity.DeriveContentID("link/module/context-law/link")
	if !linkOK {
		t.Fatal("derive Link identity")
	}
	directory, sealed := component.BuildContextDirectory(linkID)
	if !sealed || !directory.Available() {
		t.Fatal("BuildContextDirectory did not seal")
	}
	if directory.ContextCount() != 2 || directory.RootCount() != 2 {
		t.Fatalf("context geometry contexts=%d roots=%d", directory.ContextCount(), directory.RootCount())
	}
	// One local edge is issued by executioncontext.Seal for each Context;
	// this fixture has one authored cross-context module-composition edge.
	if got, want := directory.TransitionCount(), directory.ContextCount()+1; got != want {
		t.Fatalf("transition count=%d, want %d", got, want)
	}
	reflexive := 0
	cross := 0
	for index := 0; index < directory.TransitionCount(); index++ {
		row, rowOK := directory.TransitionAt(index)
		if !rowOK || !row.Available() || row.LinkID() != linkID {
			t.Fatalf("malformed transition row %d", index)
		}
		if row.FromContextID() == row.ToContextID() {
			reflexive++
		} else {
			cross++
		}
	}
	if reflexive != directory.ContextCount() || cross != 1 {
		t.Fatalf("transition partition reflexive=%d cross=%d, want %d/1", reflexive, cross, directory.ContextCount())
	}
	for index := 0; index < directory.ContextCount(); index++ {
		context, contextOK := directory.ContextAt(index)
		if !contextOK {
			t.Fatalf("context row %d", index)
		}
		transition, transitionOK := directory.Transition(context.ID(), context.ID())
		if !transitionOK || !transition.Available() || transition.FromContextID() != context.ID() || transition.ToContextID() != context.ID() {
			t.Fatalf("missing canonical local edge for context %d", index)
		}
	}
}

func TestBuildContextDirectoryRefusesUnavailableLinkIdentity(t *testing.T) {
	component := sealModuleFixture(t)
	if directory, sealed := component.BuildContextDirectory(identity.ContentID{}); sealed || directory.Available() {
		t.Fatal("BuildContextDirectory accepted an unavailable Link identity")
	}
}
