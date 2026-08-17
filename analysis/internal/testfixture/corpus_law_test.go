package testfixture

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	"github.com/wippyai/go-lua/analysis/library/lualib/targetprofile"
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
	if len(first) != 911 || len(first[0].files) == 0 {
		t.Fatalf("unexpected frozen corpus denominator: projects=%d", len(first))
	}
	wantName, wantFile := first[0].relative, first[0].files[0]
	first[0].relative = "forged/project"
	first[0].files[0] = "forged.lua"

	second := corpus.Projects()
	if len(second) != 911 || second[0].relative != wantName || second[0].files[0] != wantFile {
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

// TestSealCorpusProjectResolvesImportsThroughModuleKeys pins the Import
// admission of the sole fixture Program-to-Link constructor. Every executable
// Import resolves through the Key the Module finalizer derived, which is total
// over sealed Programs: a Key that names no exact string is a broken Program,
// not a fixture shape to skip, so the constructor reports it instead of
// silently dropping the module-cache row Link needs. A request that names no
// sibling module of the project is the one admitted skip.
func TestSealCorpusProjectResolvesImportsThroughModuleKeys(t *testing.T) {
	directory := t.TempDir()
	sources := map[string]string{
		"main.lua":    "local sibling = require(\"sibling\")\nlocal absent = require(\"absent\")\nreturn sibling\n",
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
	contract, err := profile.Contract()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	linked, err := SealCorpusProject(contract, CorpusProject{
		relative: "synthetic/import-admission", directory: directory, files: files,
	})
	if err != nil {
		t.Fatalf("seal synthetic project: %v", err)
	}

	cache := linked.Module().Cache()
	if cache.EntryCount() != 1 {
		t.Fatalf("module cache entries = %d, want exactly the resolved sibling row", cache.EntryCount())
	}
	entry, entryOK := cache.EntryAt(0)
	if !entryOK {
		t.Fatal("module cache entry is unavailable")
	}
	_, from, to, mappingOK := cache.EntryMapping(entry)
	if !mappingOK {
		t.Fatal("module cache entry mapping is unavailable")
	}
	importing := sealedProjectProgram(t, linked, from)
	imported := sealedProjectProgram(t, linked, to)
	if importing.Module().Count() != 2 || imported.Module().Count() != 0 {
		t.Fatalf("module cache row runs %d imports -> %d imports, want the requiring module -> the required module",
			importing.Module().Count(), imported.Module().Count())
	}

	requested := make([]string, 0, importing.Module().Count())
	for index := 0; index < importing.Module().Count(); index++ {
		item, itemOK := importing.Module().ImportAt(index)
		if !itemOK {
			t.Fatalf("Program Import table has no row %d", index)
		}
		row, rowOK := importing.Module().Import(item.Term)
		if !rowOK || !importing.Flow().Executable().Contains(row.Call) {
			t.Fatalf("Program Import %d is not an executable Call", index)
		}
		literal, literalOK := importing.Source().Keys().Exact(row.Key)
		if !literalOK || literal.Kind != keyspace.LiteralString {
			t.Fatalf("Program Import %d Key %v names no exact string", index, row.Key)
		}
		requested = append(requested, literal.String)
	}
	sort.Strings(requested)
	if len(requested) != 2 || requested[0] != "absent" || requested[1] != "sibling" {
		t.Fatalf("resolved Import requests = %v, want the authored [absent sibling]", requested)
	}
}

func sealedProjectProgram(t *testing.T, linked *link.Link, root linkmodule.AnalysisRoot) *program.Program {
	t.Helper()
	shard, _, _, ok := linked.Module().Roots().Mapping(root)
	if !ok {
		t.Fatal("analysis root has no shard mapping")
	}
	mounted, mountedOK := linked.Project().Mounts().Program(shard)
	if !mountedOK || mounted == nil {
		t.Fatal("mounted Program is unavailable")
	}
	return mounted
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
