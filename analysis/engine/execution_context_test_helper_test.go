package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// explicitTestContextDirectory builds the exact execution-context directory
// owned by a mounted-program test.  The owner, mounted modules, actor, and
// representative are all supplied by the fixture; this helper never invents
// a fallback context or derives one from the production declaration.
func explicitTestContextDirectory(t testing.TB, owner identity.ContentID, modules []identity.ContentID, actorID, representativeID identity.ContentID) executioncontext.Directory {
	t.Helper()
	if !owner.Available() || len(modules) == 0 || !actorID.Available() || !representativeID.Available() {
		t.Fatal("explicit execution-context directory inputs")
	}
	contexts := make([]executioncontext.Context, 0, len(modules))
	roots := make([]executioncontext.RootContext, 0, len(modules))
	seenModules := make(map[identity.ContentID]struct{}, len(modules))
	for _, moduleID := range modules {
		if !moduleID.Available() {
			t.Fatal("explicit execution-context module")
		}
		if _, duplicate := seenModules[moduleID]; duplicate {
			t.Fatal("duplicate explicit execution-context module")
		}
		seenModules[moduleID] = struct{}{}
		row, rowOK := executioncontext.NewContext(owner, moduleID, actorID, representativeID)
		rootID, rootIDOK := identity.DeriveContentID("analysis/engine/test-context-root/v1", owner[:], moduleID[:], actorID[:], representativeID[:])
		root, rootOK := executioncontext.NewRootContext(owner, rootID, row.ID())
		if !rowOK || !rootIDOK || !rootOK {
			t.Fatal("explicit execution-context row")
		}
		contexts = append(contexts, row)
		roots = append(roots, root)
	}
	directory, sealed := executioncontext.Seal(owner, contexts, roots, nil)
	if !sealed || !directory.Available() {
		t.Fatal("explicit execution-context seal")
	}
	return directory
}

// explicitTestContext resolves one exact module context from a fixture's
// sealed directory. Query admissions must carry this typed row; callers never
// synthesize a context identity or rely on directory position.
func explicitTestContext(t testing.TB, directory executioncontext.Directory, module identity.ContentID) executioncontext.Context {
	t.Helper()
	if !directory.Available() || !module.Available() {
		t.Fatal("explicit execution-context lookup inputs")
	}
	for index := 0; index < directory.ContextCount(); index++ {
		row, ok := directory.ContextAt(index)
		if ok && row.Available() && row.ModuleKey() == module {
			return row
		}
	}
	t.Fatalf("explicit execution-context module %x not found", module[:4])
	return executioncontext.Context{}
}
