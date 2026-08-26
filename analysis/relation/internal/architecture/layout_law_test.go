package architecture_test

import (
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
	{"analysis/relation/schema/region", -1, "region"},
	{"analysis/relation/schema/model", 0, "model"},
	{"analysis/relation/schema/authority", 1, "authority"},
	{"analysis/relation/semantic/outcome", 1, "semantic"},
	{"analysis/relation/semantic/signature", 2, "semantic"},
	{"analysis/relation/semantic/binding", 3, "semantic"},
	{"analysis/relation/semantic/lineage", 4, "semantic"},
	{"analysis/relation/schema/semantic/output", 3, "schema"},
	{"analysis/relation/schema/algebra", 4, "schema"},
	{"analysis/relation/schema/plan", 5, "schema"},
	{"analysis/relation/check/registry", 6, "check"},
	{"analysis/relation/check/typing", 6, "check"},
	{"analysis/relation/check/authority", 6, "check"},
	{"analysis/relation/check/recurrence", 6, "check"},
	{"analysis/schema/rule/relcompile", 6, "relcompile"},
	{"analysis/schema/rule/relbindgen", 6, "relbindgen"},
	{"internal/relationoracle", 6, "oracle"},
	{"analysis/relation/check/certificate", 7, "check"},
	{"analysis/relation/mount/address", 8, "mount"},
	{"analysis/relation/mount/arrangement", 8, "mount"},
	{"analysis/relation/mount/witness", 9, "mount"},
	// State is deliberately split by authority.  Cofiber is the lower
	// physical/logical scope bridge; Geometry consumes that sealed bridge;
	// columns consume Geometry; arrangements and aggregate versions consume
	// columns; database owns initial construction and transaction consumes the
	// immutable roots.  Do not replace this with one broad state altitude: that
	// would allow a transaction to import its own lower-level mutation substrate
	// in the wrong direction unnoticed.
	{"analysis/engine/relation/cofiber", 10, "cofiber"},
	{"analysis/engine/relation/state/geometry", 11, "state-geometry"},
	{"analysis/engine/relation/state/internal/column", 12, "state-storage"},
	{"analysis/engine/relation/state/index", 13, "state-index"},
	{"analysis/engine/relation/state/store", 12, "state-storage"},
	{"analysis/engine/relation/state/database", 14, "state-composition"},
	{"analysis/engine/relation/state/read", 14, "state-composition"},
	{"analysis/engine/relation/state/transaction", 14, "state-composition"},
	// Tuple is the one transient, sealed row/frame ABI. It consumes the
	// cofiber-stable Reader and is redeemed by operators and Apply; it owns no
	// domain policy, query planning, or mutable state.
	{"analysis/engine/relation/tuple", 14, "state-composition"},
	{"analysis/engine/relation/operator", 15, "operator"},
	// Semantic application redeems the transient tuple ABI and is therefore
	// one altitude above it. Publication consumes a sealed Application, so it
	// remains one altitude above Apply rather than importing tuple directly.
	{"analysis/engine/relation/apply", 15, "apply"},
	{"analysis/engine/relation/publish", 16, "publish"},
	{"analysis/engine/relation/solve/fixpoint", 17, "solve"},
	// Evaluator redeems an already-sealed schedule entry and committed root.
	// It is deliberately a sibling of the queue rather than importing it, so
	// runtime can compose queue -> evaluator without a package cycle.
	{"analysis/engine/relation/eval", 17, "evaluator"},
	{"analysis/engine/relation/runtime", 18, "runtime"},
	// Relation admission is the one high-altitude composition seam.  It may
	// consume declaration compilation and sealed runtime/state authorities,
	// but no lower layer may import it.
	{"analysis/program/relationadmission", 19, "admission"},
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
	"analysis/engine/relation/eval",
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
	sources, err := architectureSourcesUnder(root, source.prefix)
	if err != nil {
		t.Fatalf("walk %s: %v", source.prefix, err)
	}
	for _, sourceFile := range sources {
		if strings.HasSuffix(sourceFile.path, "_test.go") {
			continue
		}
		for _, specification := range sourceFile.file.Imports {
			importPath, unquoteErr := strconv.Unquote(specification.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("walk %s: %v", source.prefix, unquoteErr)
			}
			visit(importPath)
		}
	}
}

func controlledImport(importPath string) (string, bool) {
	if !strings.HasPrefix(importPath, modulePath+"/") {
		return "", false
	}
	rel := strings.TrimPrefix(importPath, modulePath+"/")
	return rel, strings.HasPrefix(rel, "analysis/relation/") ||
		strings.HasPrefix(rel, "analysis/engine/relation/") ||
		strings.HasPrefix(rel, "analysis/program/relationadmission") ||
		strings.HasPrefix(rel, "analysis/schema/rule/relcompile") ||
		strings.HasPrefix(rel, "analysis/schema/rule/relbindgen") ||
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
