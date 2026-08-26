package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// architectureSourceRoots are the source roots covered by the architecture
// laws.  The roots are deliberately broad: individual laws select their own
// subtree from this inventory, while one parse supplies every consumer.
var architectureSourceRoots = []string{
	"analysis",
	"domain",
	"stdlib",
	"internal",
	"cmd",
}

type architectureSourceInventory struct {
	sources     []w0Source
	parseErrors map[string]error
	walkErrors  map[string]error
}

var (
	architectureSourceInventoryOnce sync.Once
	architectureSources             architectureSourceInventory
)

// architectureSourceInventoryFor builds the test-only source inventory once.
// The ASTs and slices are immutable after initialization; callers only read
// them while applying independent laws.
func architectureSourceInventoryFor(repository string) *architectureSourceInventory {
	architectureSourceInventoryOnce.Do(func() {
		architectureSources = buildArchitectureSourceInventory(repository)
	})
	return &architectureSources
}

func buildArchitectureSourceInventory(repository string) architectureSourceInventory {
	inventory := architectureSourceInventory{
		parseErrors: make(map[string]error),
		walkErrors:  make(map[string]error),
	}
	files := token.NewFileSet()
	for _, root := range architectureSourceRoots {
		directory := filepath.Join(repository, filepath.FromSlash(root))
		if _, err := os.Stat(directory); err != nil {
			inventory.walkErrors[root] = err
			continue
		}
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" {
				return nil
			}
			relative, err := filepath.Rel(repository, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			parsed, err := parser.ParseFile(files, path, nil, 0)
			if err != nil {
				// Keep scanning so a request for another source subtree gets
				// the same parse coverage it had before inventories were shared.
				inventory.parseErrors[relative] = err
				return nil
			}
			inventory.sources = append(inventory.sources, w0Source{path: relative, file: parsed})
			return nil
		})
		if err != nil {
			inventory.walkErrors[root] = err
		}
	}
	sort.Slice(inventory.sources, func(i, j int) bool {
		return inventory.sources[i].path < inventory.sources[j].path
	})
	return inventory
}

// architectureSourcesUnder returns all parsed Go sources below root.  It is
// intentionally independent of testing.T so callers can retain their own
// historical failure wording.
func architectureSourcesUnder(repository, root string) ([]w0Source, error) {
	return architectureSourcesUnderFiltered(repository, root, false)
}

func architectureSourcesUnderSkippingHidden(repository, root string) ([]w0Source, error) {
	return architectureSourcesUnderFiltered(repository, root, true)
}

func architectureSourcesUnderFiltered(repository, root string, skipHidden bool) ([]w0Source, error) {
	directory := filepath.Join(repository, filepath.FromSlash(root))
	if _, err := os.Stat(directory); err != nil {
		return nil, err
	}
	inventory := architectureSourceInventoryFor(repository)
	top := root
	if separator := strings.IndexByte(top, '/'); separator >= 0 {
		top = top[:separator]
	}
	if err := inventory.walkErrors[top]; err != nil {
		return nil, err
	}
	parseErrorPaths := make([]string, 0)
	for path := range inventory.parseErrors {
		if (path == root || strings.HasPrefix(path, root+"/")) && (!skipHidden || !w0SourcePathHasHiddenDirectory(path)) {
			parseErrorPaths = append(parseErrorPaths, path)
		}
	}
	if len(parseErrorPaths) != 0 {
		sort.Strings(parseErrorPaths)
		return nil, inventory.parseErrors[parseErrorPaths[0]]
	}
	sources := make([]w0Source, 0, len(inventory.sources))
	for _, source := range inventory.sources {
		if (source.path == root || strings.HasPrefix(source.path, root+"/")) && (!skipHidden || !w0SourcePathHasHiddenDirectory(source.path)) {
			sources = append(sources, source)
		}
	}
	return sources, nil
}
