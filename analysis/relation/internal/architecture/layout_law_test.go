package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/wippyai/go-lua"

type altitude struct {
	prefix string
	rank   int
	owner  string
}

// These are altitude boundaries, not a frozen package inventory. Teams may add
// child packages below an owned prefix; those children inherit its altitude.
var altitudes = []altitude{
	{"analysis/relation/schema/model", 0, "model"},
	{"analysis/relation/semantic/outcome", 1, "semantic"},
	{"analysis/relation/semantic/signature", 2, "semantic"},
	{"analysis/relation/semantic/binding", 3, "semantic"},
	{"analysis/relation/semantic/lineage", 4, "semantic"},
	{"analysis/relation/schema/algebra", 4, "schema"},
	{"analysis/relation/schema/plan", 5, "schema"},
	{"analysis/relation/check/registry", 6, "check"},
	{"analysis/relation/check/typing", 6, "check"},
	{"analysis/relation/check/authority", 6, "check"},
	{"analysis/relation/check/recurrence", 6, "check"},
	{"analysis/schema/rule/relcompile", 6, "relcompile"},
	{"analysis/schema/rule/relbindgen", 6, "relbindgen"},
	// The relation input bundle publishes composition-supplied decision
	// scopes against the dense rule catalog. It names model identities and
	// nothing below the checker, so it sits with the other declaration-side
	// owners rather than in mount.
	{"analysis/schema/rule/relinput", 6, "relinput"},
	{"internal/relationoracle", 6, "oracle"},
	{"analysis/relation/check/certificate", 7, "check"},
	{"analysis/relation/mount/address", 8, "mount"},
	{"analysis/relation/mount/arrangement", 8, "mount"},
	// The input-scope projection reads a sealed bundle and resolves no
	// physical coordinate, so it consumes no other mount package.
	{"analysis/relation/mount/inputscope", 8, "mount"},
	{"analysis/relation/mount/witness", 9, "mount"},
	// State is deliberately split by authority. Geometry and recurrence are
	// mount-derived value layers; columns consume geometry; arrangements and
	// aggregate versions consume columns; bootstrap and transaction consume
	// those immutable roots. Do not replace this with one broad state altitude:
	// that would allow a transaction or bootstrap package to import its own
	// lower-level mutation substrate in the wrong direction unnoticed.
	{"analysis/engine/relation/state/geometry", 10, "state-geometry"},
	{"analysis/engine/relation/state/recurrence", 10, "state-recurrence"},
	{"analysis/engine/relation/state/internal/column", 11, "state-column"},
	{"analysis/engine/relation/state/index", 12, "state-index"},
	{"analysis/engine/relation/state/store", 12, "state-store"},
	{"analysis/engine/relation/state/bootstrap", 13, "state-bootstrap"},
	{"analysis/engine/relation/state/transaction", 13, "state-transaction"},
	{"analysis/engine/relation/operator", 14, "operator"},
	{"analysis/engine/relation/publish", 14, "publish"},
	{"analysis/engine/relation/solve/dependency", 14, "solve"},
	{"analysis/engine/relation/apply", 15, "apply"},
	{"analysis/engine/relation/solve/fixpoint", 16, "solve"},
	{"analysis/engine/relation/runtime", 17, "runtime"},
}

var emptyAggregationRoots = []string{
	"analysis/relation",
	"analysis/relation/schema",
	"analysis/relation/semantic",
	"analysis/relation/check",
	"analysis/relation/mount",
	"analysis/engine/relation",
	"analysis/engine/relation/state",
	"analysis/engine/relation/operator",
	"analysis/engine/relation/solve",
}

func TestAggregationRootsContainNoProductionGo(t *testing.T) {
	root := repositoryRoot(t)
	for _, rel := range emptyAggregationRoots {
		entries, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
				t.Errorf("aggregation root %s contains production file %s; create an owned child package", rel, entry.Name())
			}
		}
	}
}

func TestNewArchitectureImportsOnlyDownward(t *testing.T) {
	root := repositoryRoot(t)
	for _, source := range altitudes {
		walkPackageImports(t, root, source, func(importPath string) {
			rel, controlled := controlledImport(importPath)
			if !controlled {
				return
			}
			target, ok := altitudeFor(rel)
			if !ok {
				t.Errorf("%s imports unowned new-architecture package %s; register its subsystem altitude", source.prefix, rel)
				return
			}
			if target.rank > source.rank {
				t.Errorf("upward import: %s (rank %d) -> %s (rank %d)", source.prefix, source.rank, rel, target.rank)
			}
			if target.rank == source.rank && target.owner != source.owner {
				t.Errorf("sideways import across owned subsystems: %s -> %s", source.prefix, rel)
			}
		})
	}
}

func TestGenericArchitectureExcludesDomainsAndOldExecutionProtocol(t *testing.T) {
	root := repositoryRoot(t)
	for _, source := range altitudes {
		walkPackageImports(t, root, source, func(importPath string) {
			rel := strings.TrimPrefix(importPath, modulePath+"/")
			switch {
			case strings.HasPrefix(rel, "domain/"):
				t.Errorf("generic package %s imports domain implementation %s", source.prefix, rel)
			case strings.HasPrefix(rel, "analysis/engine/execution"):
				t.Errorf("new architecture %s imports old execution protocol %s", source.prefix, rel)
			case strings.HasPrefix(rel, "analysis/engine/internal/carrier") ||
				strings.HasPrefix(rel, "analysis/engine/internal/factbinding") ||
				strings.HasPrefix(rel, "analysis/engine/internal/demand") ||
				strings.HasPrefix(rel, "analysis/engine/internal/equation") ||
				strings.HasPrefix(rel, "analysis/engine/internal/linkexecutionplan") ||
				strings.HasPrefix(rel, "analysis/engine/internal/executioncatalog"):
				t.Errorf("new architecture %s imports rejected old protocol package %s", source.prefix, rel)
			}
		})
	}
}

func TestLogicalLayerDoesNotImportPhysicalEngine(t *testing.T) {
	root := repositoryRoot(t)
	for _, source := range altitudes {
		if !strings.HasPrefix(source.prefix, "analysis/relation/") {
			continue
		}
		walkPackageImports(t, root, source, func(importPath string) {
			if strings.HasPrefix(importPath, modulePath+"/analysis/engine/") {
				t.Errorf("logical package %s imports physical engine package %s", source.prefix, importPath)
			}
		})
	}
}

func TestMountConsumesOnlyTheOpaqueCertificateBoundary(t *testing.T) {
	root := repositoryRoot(t)
	for _, source := range altitudes {
		if !strings.HasPrefix(source.prefix, "analysis/relation/mount/") {
			continue
		}
		walkPackageImports(t, root, source, func(importPath string) {
			if strings.HasPrefix(importPath, modulePath+"/analysis/relation/check/registry") ||
				strings.HasPrefix(importPath, modulePath+"/analysis/relation/check/typing") ||
				strings.HasPrefix(importPath, modulePath+"/analysis/relation/check/authority") ||
				strings.HasPrefix(importPath, modulePath+"/analysis/relation/check/recurrence") {
				t.Errorf("mount package %s bypasses the opaque certificate through %s", source.prefix, importPath)
			}
		})
	}
}

func TestNewCoreHasNoThirdPartyDependency(t *testing.T) {
	root := repositoryRoot(t)
	for _, source := range altitudes {
		walkPackageImports(t, root, source, func(importPath string) {
			if strings.Contains(strings.Split(importPath, "/")[0], ".") &&
				!strings.HasPrefix(importPath, modulePath+"/") {
				t.Errorf("new core package %s imports third-party package %s", source.prefix, importPath)
			}
		})
	}
}

func TestReferenceOracleCannotInheritProductionAssumptions(t *testing.T) {
	root := repositoryRoot(t)
	source, _ := altitudeFor("internal/relationoracle")
	walkPackageImports(t, root, source, func(importPath string) {
		rel := strings.TrimPrefix(importPath, modulePath+"/")
		if strings.HasPrefix(rel, "analysis/engine/relation") || strings.HasPrefix(rel, "analysis/relation/mount") || strings.HasPrefix(rel, "analysis/relation/check") {
			t.Errorf("reference oracle imports production planning/execution package %s", rel)
		}
	})
}

func walkPackageImports(t *testing.T, root string, source altitude, visit func(string)) {
	t.Helper()
	directory := filepath.Join(root, source.prefix)
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range file.Imports {
			value, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			visit(value)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", source.prefix, err)
	}
}

func controlledImport(importPath string) (string, bool) {
	if !strings.HasPrefix(importPath, modulePath+"/") {
		return "", false
	}
	rel := strings.TrimPrefix(importPath, modulePath+"/")
	return rel, strings.HasPrefix(rel, "analysis/relation/") ||
		strings.HasPrefix(rel, "analysis/engine/relation/") ||
		strings.HasPrefix(rel, "analysis/schema/rule/relcompile") ||
		strings.HasPrefix(rel, "analysis/schema/rule/relbindgen") ||
		strings.HasPrefix(rel, "analysis/schema/rule/relinput") ||
		strings.HasPrefix(rel, "internal/relationoracle")
}

func altitudeFor(path string) (altitude, bool) {
	best := altitude{}
	found := false
	for _, candidate := range altitudes {
		if path == candidate.prefix || strings.HasPrefix(path, candidate.prefix+"/") {
			if !found || len(candidate.prefix) > len(best.prefix) {
				best = candidate
				found = true
			}
		}
	}
	return best, found
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod not found")
		}
		directory = parent
	}
}
