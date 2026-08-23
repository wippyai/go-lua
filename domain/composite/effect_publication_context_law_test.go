package composite

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

func effectPublicationContextID(t testing.TB, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("domain/composite/effect-publication-context-law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive context-law identity %q", label)
	}
	return id
}

func effectPublicationContextDirectory(t testing.TB, link identity.ContentID, contexts ...executioncontext.Context) executioncontext.Directory {
	t.Helper()
	if !link.Available() || len(contexts) == 0 {
		t.Fatal("context-law directory inputs")
	}
	roots := make([]executioncontext.RootContext, 0, len(contexts))
	for index, context := range contexts {
		if !context.Available() || context.LinkID() != link {
			t.Fatal("context-law context")
		}
		rootID := effectPublicationContextID(t, "root/"+strconv.Itoa(index))
		root, ok := executioncontext.NewRootContext(link, rootID, context.ID())
		if !ok {
			t.Fatal("context-law root")
		}
		roots = append(roots, root)
	}
	directory, ok := executioncontext.Seal(link, contexts, roots, nil)
	if !ok || !directory.Available() {
		t.Fatal("seal context-law directory")
	}
	return directory
}

func TestMountedEffectContextsExpandsEveryExactSameModuleContext(t *testing.T) {
	link := effectPublicationContextID(t, "link")
	mount := effectPublicationContextID(t, "module")
	first, firstOK := executioncontext.NewContext(link, mount, effectPublicationContextID(t, "actor/first"), effectPublicationContextID(t, "representative/first"))
	second, secondOK := executioncontext.NewContext(link, mount, effectPublicationContextID(t, "actor/second"), effectPublicationContextID(t, "representative/second"))
	other, otherOK := executioncontext.NewContext(link, effectPublicationContextID(t, "other-module"), effectPublicationContextID(t, "actor/other"), effectPublicationContextID(t, "representative/other"))
	if !firstOK || !secondOK || !otherOK {
		t.Fatal("construct context-law rows")
	}
	directory := effectPublicationContextDirectory(t, link, first, second, other)
	got, ok := directory.ContextsForModule(mount)
	if !ok || len(got) != 2 {
		t.Fatalf("mounted effect contexts = %d/%t, want two exact rows", len(got), ok)
	}
	if got[0].ID() == got[1].ID() || got[0].ModuleKey() != mount || got[1].ModuleKey() != mount {
		t.Fatal("same-module effect contexts collapsed or changed module")
	}
	// Directory sealing canonicalizes the rows. Expansion must preserve that
	// order so the observation inventory is deterministic across callers.
	for index := range got {
		want, wantOK := directory.Context(got[index].ID())
		if !wantOK || want != got[index] {
			t.Fatalf("expanded context %d is not the canonical directory row", index)
		}
	}
	reversed := effectPublicationContextDirectory(t, link, other, second, first)
	gotReversed, reversedOK := reversed.ContextsForModule(mount)
	if !reversedOK || len(gotReversed) != len(got) {
		t.Fatal("permuted context directory changed expansion cardinality")
	}
	for index := range got {
		if gotReversed[index].ID() != got[index].ID() {
			t.Fatal("context expansion depended on input directory order")
		}
	}
}

func TestMountedEffectContextsPreservesOneContextAndRefusesMissingContext(t *testing.T) {
	link := effectPublicationContextID(t, "one-link")
	mount := effectPublicationContextID(t, "one-module")
	context, contextOK := executioncontext.NewContext(link, mount, effectPublicationContextID(t, "one-actor"), effectPublicationContextID(t, "one-representative"))
	foreign, foreignOK := executioncontext.NewContext(link, effectPublicationContextID(t, "foreign-module"), effectPublicationContextID(t, "foreign-actor"), effectPublicationContextID(t, "foreign-representative"))
	if !contextOK || !foreignOK {
		t.Fatal("construct one-context rows")
	}
	directory := effectPublicationContextDirectory(t, link, context, foreign)
	got, ok := directory.ContextsForModule(mount)
	if !ok || len(got) != 1 || got[0] != context {
		t.Fatalf("one-context expansion = %d/%t, want the exact context", len(got), ok)
	}
	if _, ok := directory.ContextsForModule(effectPublicationContextID(t, "absent-module")); ok {
		t.Fatal("mismatched module acquired an effect context")
	}
	if _, ok := (executioncontext.Directory{}).ContextsForModule(mount); ok {
		t.Fatal("zero context directory acquired an effect context")
	}
	if _, ok := directory.ContextsForModule(identity.ContentID{}); ok {
		t.Fatal("zero mount acquired an effect context")
	}
}
