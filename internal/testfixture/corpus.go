package testfixture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
)

// FrozenLuaFileCount is the exact checked-in Lua fixture denominator. Corpus
// changes require an explicit test-contract update rather than silently
// changing the analysis denominator.
// The two qualified-host-type fixtures state CX-36 end to end: a name the
// target's sealed qualified type index publishes resolves through it, and a
// name it does not publish refuses under its exact authored spelling.
const FrozenLuaFileCount = 1216

// FrozenCorpusProjectCount is the exact checked-in fixture-project
// denominator: the number of distinct project directories LoadCorpus groups
// the frozen Lua files into. Every consumer that pins the fixture-project
// count derives it from this one constant rather than hand-copying the
// number, so the fixture census and its dependent counts change together.
const FrozenCorpusProjectCount = 949

type corpusManifest struct {
	Files []string `json:"files"`
}

// CorpusProject is one closed fixture directory. Its filesystem location is
// intentionally private: consumers can name and seal a project, but cannot
// reinterpret its module inventory through another fixture resolver.
type CorpusProject struct {
	relative  string
	directory string
	files     []string
}

// Corpus is one caller-scoped census of the checked-in fixture corpus. The
// enumeration is performed once per Corpus and belongs to the caller that
// named the repository: this package holds no census state of its own and
// never locates the repository itself.
type Corpus struct {
	projects []CorpusProject
}

func (project CorpusProject) Name() string { return project.relative }

func (project CorpusProject) FileCount() int { return len(project.files) }

func (project CorpusProject) FileAt(index int) (string, bool) {
	if index < 0 || index >= len(project.files) || project.relative == "" {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(project.relative, project.files[index])), true
}

// SourceText reads one declared Lua file of the project. The fixture directory
// stays private: a caller names a file the project declares and never
// reconstructs a corpus path of its own.
func (project CorpusProject) SourceText(file string) ([]byte, error) {
	if project.relative == "" || project.directory == "" {
		return nil, fmt.Errorf("testfixture: unavailable corpus project")
	}
	for _, declared := range project.files {
		if declared != file {
			continue
		}
		text, err := os.ReadFile(filepath.Join(project.directory, declared))
		if err != nil {
			return nil, fmt.Errorf("testfixture: read %s: %w", filepath.ToSlash(filepath.Join(project.relative, declared)), err)
		}
		return text, nil
	}
	return nil, fmt.Errorf("testfixture: fixture %s declares no file %q", project.relative, file)
}

// RepositoryRoot walks up from a caller-supplied directory to the module root
// that holds go.mod. Discovery stays caller-supplied: the caller names its own
// source position, and this package never locates the repository itself.
func RepositoryRoot(fromDir string) (string, error) {
	if fromDir == "" {
		return "", fmt.Errorf("testfixture: empty repository search directory")
	}
	directory, err := filepath.Abs(fromDir)
	if err != nil {
		return "", fmt.Errorf("testfixture: resolve repository search directory %q: %w", fromDir, err)
	}
	for {
		info, statErr := os.Stat(filepath.Join(directory, "go.mod"))
		if statErr == nil && !info.IsDir() {
			return directory, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", fmt.Errorf("testfixture: search repository root above %q: %w", fromDir, statErr)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("testfixture: no module root above %q", fromDir)
		}
		directory = parent
	}
}

// LoadCorpus enumerates every checked-in fixture directory under the named
// repository root in canonical path order. A manifest, when present, must name
// exactly the local Lua files; diagnostics, skip fields, package metadata, and
// outputs are never consulted.
func LoadCorpus(repository string) (*Corpus, error) {
	if repository == "" {
		return nil, fmt.Errorf("testfixture: empty repository root")
	}
	projects, err := loadFrozenCorpusProjects(repository)
	if err != nil {
		return nil, err
	}
	return &Corpus{projects: projects}, nil
}

// Projects returns the whole census as a defensive view.
func (corpus *Corpus) Projects() []CorpusProject {
	return cloneCorpusProjects(corpus.projects)
}

// Project returns one named fixture directory as a defensive view.
func (corpus *Corpus) Project(name string) (CorpusProject, error) {
	projects := corpus.projects
	index := sort.Search(len(projects), func(index int) bool { return projects[index].relative >= name })
	if index >= len(projects) || projects[index].relative != name {
		return CorpusProject{}, fmt.Errorf("testfixture: missing fixture project %q", name)
	}
	return cloneCorpusProject(projects[index]), nil
}

func loadFrozenCorpusProjects(repository string) ([]CorpusProject, error) {
	root := filepath.Join(repository, "testdata", "fixtures")
	projects := make(map[string]*CorpusProject)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".lua" {
			return nil
		}
		directory := filepath.Dir(path)
		relative, err := filepath.Rel(root, directory)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		project := projects[key]
		if project == nil {
			project = &CorpusProject{relative: key, directory: directory}
			projects[key] = project
		}
		project.files = append(project.files, filepath.Base(path))
		return nil
	}); err != nil {
		return nil, fmt.Errorf("testfixture: walk frozen fixture corpus: %w", err)
	}

	result := make([]CorpusProject, 0, len(projects))
	for _, project := range projects {
		sort.Strings(project.files)
		if err := validateManifest(project); err != nil {
			return nil, err
		}
		result = append(result, *project)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].relative < result[right].relative })
	return result, nil
}

func cloneCorpusProjects(projects []CorpusProject) []CorpusProject {
	result := make([]CorpusProject, len(projects))
	for index, project := range projects {
		result[index] = cloneCorpusProject(project)
	}
	return result
}

func cloneCorpusProject(project CorpusProject) CorpusProject {
	project.files = append([]string(nil), project.files...)
	return project
}

// SealSource is the sole raw-source-to-Link constructor. Its input is a source
// text a caller synthesized or truncated rather than a fixture directory, so it
// seals one module through the ordinary lowerer and Link admission surfaces and
// derives no module-cache ingress. Sealing lives here, with the project seal, so
// a fixture Link and a synthetic Link are built by one construction path.
func SealSource(contract *contract.Contract, name string, text []byte) (*link.Link, error) {
	if contract == nil || !contract.ContentID().Available() || name == "" {
		return nil, fmt.Errorf("testfixture: unavailable source name or canonical target profile")
	}
	sealed, err := lower.Lower(lower.Source{Name: name, Text: text, Types: contract.Types()})
	if err != nil {
		return nil, fmt.Errorf("testfixture: lower %s: %w", name, err)
	}
	return link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: sealed}}})
}

// SealCorpusProject is the sole fixture Project-to-Link constructor. It uses
// the ordinary lowerer and Link admission surfaces and derives only the exact
// actor/cache rows required by the project's sealed Program imports.
func SealCorpusProject(contract *contract.Contract, project CorpusProject) (*link.Link, error) {
	if contract == nil || !contract.ContentID().Available() || project.relative == "" || project.directory == "" || len(project.files) == 0 {
		return nil, fmt.Errorf("testfixture: unavailable corpus project or canonical target profile")
	}
	modules := make([]linkproject.Module, 0, len(project.files))
	programs := make(map[string]*program.Program, len(project.files))
	for _, file := range project.files {
		module := strings.TrimSuffix(file, ".lua")
		path := filepath.ToSlash(filepath.Join(project.relative, file))
		if module == "" {
			return nil, fmt.Errorf("testfixture: fixture %s has an empty module identity", path)
		}
		if _, duplicate := programs[module]; duplicate {
			return nil, fmt.Errorf("testfixture: fixture %s has duplicate module identity %q", project.relative, module)
		}
		source, err := os.ReadFile(filepath.Join(project.directory, file))
		if err != nil {
			return nil, fmt.Errorf("testfixture: read %s: %w", path, err)
		}
		// The fixture project is the source-root boundary. Diagnostics and
		// manifests use project-relative file names; the corpus storage path is
		// test infrastructure and must not enter Program identity or findings.
		sealed, err := lower.Lower(lower.Source{Name: filepath.ToSlash(file), Text: source, Types: contract.Types()})
		if err != nil {
			return nil, fmt.Errorf("testfixture: lower %s: %w", path, err)
		}
		programs[module] = sealed
		modules = append(modules, linkproject.Module{Name: module, Program: sealed})
	}

	aliases := make([]linkmodule.ModuleCacheAliasClassSpec, 0, len(modules))
	roots := make([]linkmodule.AnalysisRootSpec, 0, len(modules))
	for _, module := range modules {
		instance := "fixture-module:" + module.Name
		aliases = append(aliases, linkmodule.ModuleCacheAliasClassSpec{
			Actor: "fixture", Instances: []string{instance}, Representative: instance,
		})
		roots = append(roots, linkmodule.AnalysisRootSpec{
			Name: "fixture-root:" + module.Name, Module: module.Name, Actor: "fixture", Instance: instance,
		})
	}
	entries := make([]linkmodule.ModuleCacheEntrySpec, 0)
	for _, module := range modules {
		imports := module.Program.Flow().Authored().Imports()
		for index := 0; index < imports.Count(); index++ {
			item, ok := imports.ImportAt(index)
			if !ok {
				return nil, fmt.Errorf("testfixture: fixture %s %s has malformed Program Import table", project.relative, module.Name)
			}
			row, ok := imports.Get(item.Term)
			if !ok || row.Call == 0 {
				return nil, fmt.Errorf("testfixture: fixture %s %s has malformed Program Import", project.relative, module.Name)
			}
			// Project admits an Import application exactly when its Call is
			// executable. The authored Request is joined to Source's canonical
			// String literal and exact-key quotient; the derived Artifact Module
			// row is produced later by compilation and is not needed to describe
			// this fixture's Link ingress.
			if !module.Program.Flow().Executable().Contains(row.Call) {
				continue
			}
			_, _, requestedName, requestedStringOK := module.Program.Source().Literals().Strings().At(int(keyspace.TermOrdinal(row.Request) - 1))
			requestedKey, requestKeyOK := module.Program.Source().Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: requestedName})
			requested, requestedOK := module.Program.Source().Keys().Exact(requestedKey)
			if !requestedStringOK || !requestKeyOK || !requestedOK || requested.Kind != keyspace.LiteralString {
				return nil, fmt.Errorf("testfixture: fixture %s %s Import has non-string request", project.relative, module.Name)
			}
			name := requested.String
			if _, registered := programs[name]; !registered {
				continue
			}
			entries = append(entries, linkmodule.ModuleCacheEntrySpec{
				Module: module.Name, Import: item.Term,
				FromRoot: "fixture-root:" + module.Name, ToRoot: "fixture-root:" + name,
			})
		}
	}
	return link.Seal(&link.Spec{
		Target: contract, Modules: modules,
		Module: linkmodule.Spec{Actors: []linkmodule.ActorSpec{{Name: "fixture"}}, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries},
	})
}

func validateManifest(project *CorpusProject) error {
	data, err := os.ReadFile(filepath.Join(project.directory, "manifest.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("testfixture: read fixture manifest %s: %w", project.relative, err)
	}
	var manifest corpusManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("testfixture: decode fixture manifest %s: %w", project.relative, err)
	}
	if len(manifest.Files) == 0 {
		return nil
	}
	declared := append([]string(nil), manifest.Files...)
	sort.Strings(declared)
	if len(declared) != len(project.files) {
		return fmt.Errorf("testfixture: fixture manifest %s declares %d Lua modules; filesystem census has %d", project.relative, len(declared), len(project.files))
	}
	for index, name := range declared {
		if filepath.Base(name) != name || filepath.Ext(name) != ".lua" || name != project.files[index] {
			return fmt.Errorf("testfixture: fixture manifest %s module inventory is not the exact local Lua census", project.relative)
		}
	}
	return nil
}
