package testfixture

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// corpusLawRepositoryRoot locates the module root from this source file, so the
// census is independent of the working directory a test runs in.
func corpusLawRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("testfixture source location unavailable")
	}
	repository, err := RepositoryRoot(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func TestFrozenCorpusCatalogReturnsDefensiveViews(t *testing.T) {
	corpus, err := LoadCorpus(corpusLawRepositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	first := corpus.Projects()
	if len(first) != FrozenCorpusProjectCount || len(first[0].files) == 0 {
		t.Fatalf("unexpected frozen corpus denominator: projects=%d", len(first))
	}
	wantName, wantFile := first[0].relative, first[0].files[0]
	first[0].relative = "forged/project"
	first[0].files[0] = "forged.lua"

	second := corpus.Projects()
	if len(second) != FrozenCorpusProjectCount || second[0].relative != wantName || second[0].files[0] != wantFile {
		t.Fatal("caller mutation changed the loaded corpus catalog")
	}
	project, err := corpus.Project(wantName)
	if err != nil {
		t.Fatal(err)
	}
	project.files[0] = "forged-again.lua"
	replayed, err := corpus.Project(wantName)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.files[0] != wantFile {
		t.Fatal("named lookup exposed the stored file slice")
	}
}

// TestSealCorpusProjectResolvesImportsThroughModuleKeys pins the canonical
// Link-lifetime composition publication. Every executable authored request to
// a declared or mounted module is represented by a sealed ResolvedImport and
// a CacheIngress. A request naming a module that is neither declared nor
// mounted is refused by the module-request admission gate at seal time: the
// gate must never be weakened to fabricate a mount or cache row for it.
func TestSealCorpusProjectResolvesImportsThroughModuleKeys(t *testing.T) {
	directory := t.TempDir()
	sources := map[string]string{
		"main.lua":    "local sibling = require(\"sibling\")\nreturn sibling\n",
		"sibling.lua": "return {}\n",
	}
	files := make([]string, 0, len(sources))
	for name, text := range sources {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, name)
	}
	sort.Strings(files)
	contract, err := StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	linked, err := SealCorpusProject(contract, CorpusProject{
		relative: "synthetic/import-admission", directory: directory, files: files,
	})
	if err != nil {
		t.Fatalf("seal synthetic project: %v", err)
	}

	published, axes := sealedCompositionSnapshot(t, linked)
	cacheCount, cacheCountOK := snapshot.MemberCountAtAxis(&published, axes.cache)
	if !cacheCountOK || cacheCount != 1 {
		t.Fatalf("module cache rows = %d/%v, want exactly the resolved sibling row", cacheCount, cacheCountOK)
	}
	cacheKey, cacheKeyOK := snapshot.MemberAtAxis(&published, axes.cache, 0)
	if !cacheKeyOK {
		t.Fatal("module cache row key is unavailable")
	}
	cacheEntry, cacheEntryOK := modulecomposition.CacheAt(&published, axes.cache, cacheKey)
	if !cacheEntryOK {
		t.Fatal("module cache row is unavailable")
	}
	importing, importingOK := programmount.Mounted(&published, axes.mount, cacheEntry.SourceModuleKey())
	imported, importedOK := programmount.Mounted(&published, axes.mount, cacheEntry.TargetModuleKey())
	if !importingOK || !importedOK {
		t.Fatal("module cache row names no published source/target mounts")
	}
	importingImports, importingPublished := importing.ModuleImportCount()
	importedImports, importedPublished := imported.ModuleImportCount()
	if !importingPublished || !importedPublished || importingImports != 1 || importedImports != 0 {
		t.Fatalf("module cache row runs %d imports -> %d imports, want the requiring module -> the required module",
			importingImports, importedImports)
	}

	requested := make([]string, 0, importingImports)
	for index := 0; index < importingImports; index++ {
		_, itemOK := importing.ModuleImportAt(index)
		if !itemOK {
			t.Fatalf("Program Import table has no row %d", index)
		}
		request, requestOK := importing.ModuleRequestFor(index)
		projectKey, projectKeyOK := linked.Project().Keys().ForMounted(cacheEntry.SourceModuleKey(), request.Key())
		literal, literalOK := linked.Project().Keys().Exact(projectKey)
		if !requestOK || !projectKeyOK || !literalOK || literal.Kind != keyspace.LiteralString {
			t.Fatalf("Program Import %d request names no exact string", index)
		}
		requested = append(requested, literal.String)
	}
	if len(requested) != 1 || requested[0] != "sibling" {
		t.Fatalf("resolved Import requests = %v, want the authored [sibling]", requested)
	}
}

// TestSealCorpusProjectRefusesUndeclaredModuleRequests pins the negative half
// of the module-request admission gate: a require naming a module that is
// neither a declared host module nor a mounted sibling is refused by name at
// seal time, and no Link is produced for it.
func TestSealCorpusProjectRefusesUndeclaredModuleRequests(t *testing.T) {
	directory := t.TempDir()
	source := "local absent = require(\"absent\")\nreturn absent\n"
	if err := os.WriteFile(filepath.Join(directory, "main.lua"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	contract, err := StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	_, err = SealCorpusProject(contract, CorpusProject{
		relative: "synthetic/import-admission-refusal", directory: directory, files: []string{"main.lua"},
	})
	if err == nil {
		t.Fatal("seal synthetic project: want refusal of the undeclared module request, got nil error")
	}
	if !strings.Contains(err.Error(), "absent") ||
		!strings.Contains(err.Error(), "neither a declared host module nor a mounted module") {
		t.Fatalf("seal error = %q, want refusal naming %q as neither a declared host module nor a mounted module", err, "absent")
	}
}

type moduleSnapshotAxes struct {
	mount   snapshot.Axis[identity.ContentID, programmount.Program]
	imports snapshot.Axis[identity.ContentID, modulecomposition.ResolvedImport]
	cache   snapshot.Axis[identity.ContentID, modulecomposition.CacheIngress]
}

func sealedCompositionSnapshot(t *testing.T, linked *link.Link) (snapshot.Snapshot, moduleSnapshotAxes) {
	t.Helper()
	plan, status, diagnostics := analysis.CompileWithDiagnostics(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile canonical composition: status=%v diagnostics=%+v", status, diagnostics)
	}
	publication, publicationOK := plan.Publication()
	if !publicationOK {
		_ = plan.Close()
		t.Fatal("canonical composition Publication is unavailable")
	}
	mount, mountOK := analysiscatalog.ProjectAxis[identity.ContentID, programmount.Program](publication, programmount.OutputKey)
	imports, importsOK := analysiscatalog.ProjectAxis[identity.ContentID, modulecomposition.ResolvedImport](publication, modulecomposition.ImportOutputKey)
	cache, cacheOK := analysiscatalog.ProjectAxis[identity.ContentID, modulecomposition.CacheIngress](publication, modulecomposition.CacheOutputKey)
	published, publishedOK := plan.Snapshot()
	if !publishedOK {
		_ = plan.Close()
		t.Fatal("canonical composition Snapshot is unavailable")
	}
	_ = plan.Close()
	if !mountOK || !importsOK || !cacheOK || mount.SchemaID != published.Schema() || imports.SchemaID != published.Schema() || cache.SchemaID != published.Schema() {
		t.Fatal("canonical mount/module-composition axes are unavailable")
	}
	return published, moduleSnapshotAxes{mount: mount, imports: imports, cache: cache}
}

// TestFrozenLuaFileCountIsTheCorpusDenominator holds the declared release
// denominator against the enumeration it names.
func TestFrozenLuaFileCountIsTheCorpusDenominator(t *testing.T) {
	corpus, err := LoadCorpus(corpusLawRepositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, project := range corpus.Projects() {
		total += project.FileCount()
	}
	if total != FrozenLuaFileCount {
		t.Fatalf("checked-in Lua corpus has %d files; FrozenLuaFileCount declares %d", total, FrozenLuaFileCount)
	}
}
