package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func authoredModulePathSpec() declaration.Spec {
	spec := completeBootSpec("Lua 5.3", vocabulary.InitialMutable)
	for index := range spec.InitialRoots {
		switch spec.InitialRoots[index].Identity {
		case "GlobalEnvRoot":
			spec.InitialRoots[index].ModulePath = "std/global"
		case "StringMetatableRoot":
			spec.InitialRoots[index].ModulePath = "std/string"
		}
	}
	return spec
}

func TestInitialRootModulePathRejectsDuplicate(t *testing.T) {
	spec := completeBootSpec("Lua 5.3", vocabulary.InitialMutable)
	spec.InitialRoots[0].ModulePath = "std/shared"
	spec.InitialRoots[1].ModulePath = "std/shared"
	if _, err := testSeal(&spec); err == nil {
		t.Fatal("Seal accepted duplicate non-empty initial-root module paths")
	}
}

func TestInitialRootModulePathIsCanonicalAndContentIdentified(t *testing.T) {
	leftSpec := authoredModulePathSpec()
	rightSpec := authoredModulePathSpec()
	reverseInitialRoots(rightSpec.InitialRoots)

	left := mustSeal(t, leftSpec)
	right := mustSeal(t, rightSpec)
	if left.ContentID() != right.ContentID() {
		t.Fatal("initial-root input order changed ContentID")
	}

	for _, test := range []struct {
		path     string
		identity string
	}{
		{path: "std/global", identity: "GlobalEnvRoot"},
		{path: "std/string", identity: "StringMetatableRoot"},
	} {
		leftRoot, leftOK := left.InitialRootByModulePath(test.path)
		rightRoot, rightOK := right.InitialRootByModulePath(test.path)
		if !leftOK || !rightOK {
			t.Fatalf("module path %q was not resolvable: left=%v right=%v", test.path, leftOK, rightOK)
		}
		leftIdentity, leftIdentityOK := left.InitialRootIdentity(leftRoot)
		rightIdentity, rightIdentityOK := right.InitialRootIdentity(rightRoot)
		if !leftIdentityOK || !rightIdentityOK || leftIdentity != test.identity || rightIdentity != test.identity {
			t.Fatalf("module path %q resolved to %q/%v and %q/%v; want %q", test.path, leftIdentity, leftIdentityOK, rightIdentity, rightIdentityOK, test.identity)
		}
		if got, ok := left.InitialRootModulePath(leftRoot); !ok || got != test.path {
			t.Fatalf("left root module path = %q/%v, want %q", got, ok, test.path)
		}
		if got, ok := right.InitialRootModulePath(rightRoot); !ok || got != test.path {
			t.Fatalf("right root module path = %q/%v, want %q", got, ok, test.path)
		}

		identityRoot, identityOK := left.InitialRootByIdentity(test.identity)
		if !identityOK || identityRoot != leftRoot {
			t.Fatalf("module path %q did not return its exact sorted root: got %d/%v, identity root %d/%v", test.path, leftRoot, leftOK, identityRoot, identityOK)
		}
	}
}

func TestInitialRootModulePathChangesContentID(t *testing.T) {
	baseSpec := authoredModulePathSpec()
	changedSpec := authoredModulePathSpec()
	for index := range changedSpec.InitialRoots {
		if changedSpec.InitialRoots[index].Identity == "GlobalEnvRoot" {
			changedSpec.InitialRoots[index].ModulePath = "std/global/v2"
		}
	}

	base := mustSeal(t, baseSpec)
	changed := mustSeal(t, changedSpec)
	if base.ContentID() == changed.ContentID() {
		t.Fatal("changing only an initial-root module path did not change ContentID")
	}
}

func TestInitialRootModulePathEmptyIsNotResolvable(t *testing.T) {
	contract := mustSeal(t, completeBootSpec("Lua 5.3", vocabulary.InitialMutable))
	if root, ok := contract.InitialRootByModulePath(""); ok || root != 0 {
		t.Fatalf("empty module path resolved to root %d/%v", root, ok)
	}
	if root, ok := contract.InitialRootByModulePath("std/missing"); ok || root != 0 {
		t.Fatalf("unknown module path resolved to root %d/%v", root, ok)
	}
	root, ok := contract.InitialRootAt(0)
	if !ok {
		t.Fatal("missing ordinary initial root")
	}
	if path, pathOK := contract.InitialRootModulePath(root); pathOK || path != "" {
		t.Fatalf("ordinary root reported module path %q/%v", path, pathOK)
	}
}
