package semantic

import (
	"fmt"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"golang.org/x/tools/go/packages"
)

// typedImpactPatterns projects complete cheap package metadata into the exact
// typed roots needed by this collection. Metadata never becomes semantic
// evidence; it only prevents a whole-workspace type load.
func typedImpactPatterns(root string, loaded []*packages.Package, requests []SymbolRequest, scope loadScope) ([]string, error) {
	graph := metadataGraph{nodes: map[string]metadataNode{}, reverse: map[string]map[string]bool{}, owners: map[string]map[string]bool{}}
	for _, pkg := range allPackages(loaded) {
		if isSyntheticTestMain(pkg) {
			continue
		}
		inside, err := metadataInsideRoot(root, pkg)
		if err != nil {
			return nil, err
		}
		if !inside {
			continue
		}
		base, err := metadataBasePath(pkg)
		if err != nil {
			return nil, err
		}
		node := graph.nodes[base]
		if node.imports == nil {
			node.imports = map[string]bool{}
		}
		for _, file := range metadataFiles(pkg) {
			relative, err := filepath.Rel(root, file)
			if err != nil || relative == "" || relative == "." || !pathInsideRoot(root, file) {
				return nil, fmt.Errorf("metadata package has invalid workspace file: %s", file)
			}
			if graph.owners[relative] == nil {
				graph.owners[relative] = map[string]bool{}
			}
			graph.owners[relative][base] = true
		}
		for importPath, dependency := range pkg.Imports {
			dependencyBase, err := metadataImportBase(importPath, dependency)
			if err != nil {
				return nil, err
			}
			node.imports[dependencyBase] = true
		}
		graph.nodes[base] = node
	}
	for importer, node := range graph.nodes {
		for imported := range node.imports {
			if graph.nodes[imported].imports == nil {
				continue
			}
			if graph.reverse[imported] == nil {
				graph.reverse[imported] = map[string]bool{}
			}
			graph.reverse[imported][importer] = true
		}
	}
	return graph.patterns(requests, scope)
}

type metadataGraph struct {
	nodes   map[string]metadataNode
	reverse map[string]map[string]bool
	owners  map[string]map[string]bool
}

type metadataNode struct {
	imports map[string]bool
}

func (graph metadataGraph) patterns(requests []SymbolRequest, scope loadScope) ([]string, error) {
	selected := map[string]bool{}
	pending := make([]string, 0)
	for _, request := range requests {
		owner, exported, err := symbolOwnerAndExported(request.Object)
		if err != nil {
			return nil, err
		}
		if graph.nodes[owner].imports == nil {
			return nil, fmt.Errorf("metadata lacks request owner package: %s", owner)
		}
		if !selected[owner] {
			selected[owner] = true
		}
		if request.Impact && exported {
			pending = append(pending, owner)
		}
	}
	for _, path := range scope.Files {
		owners := graph.owners[path]
		if len(owners) != 1 {
			return nil, fmt.Errorf("metadata lacks unique affected package owner: %s", path)
		}
		for owner := range owners {
			selected[owner] = true
			if scope.ExpandFileOwners {
				pending = append(pending, owner)
			}
		}
	}
	for _, owner := range scope.RemovedSurfaceOwners {
		if graph.nodes[owner].imports == nil {
			continue
		}
		selected[owner] = true
		pending = append(pending, owner)
	}
	for len(pending) != 0 {
		owner := pending[0]
		pending = pending[1:]
		for importer := range graph.reverse[owner] {
			if !selected[importer] {
				selected[importer] = true
				pending = append(pending, importer)
			}
		}
	}
	patterns := make([]string, 0, len(selected))
	for owner := range selected {
		patterns = append(patterns, owner)
	}
	sort.Strings(patterns)
	if len(patterns) == 0 {
		return nil, fmt.Errorf("metadata produced no typed package roots")
	}
	return patterns, nil
}

func metadataInsideRoot(root string, pkg *packages.Package) (bool, error) {
	if pkg == nil {
		return false, fmt.Errorf("metadata contains nil package")
	}
	files := metadataFiles(pkg)
	if len(files) == 0 {
		return false, nil
	}
	inside := 0
	for _, file := range files {
		if pathInsideRoot(root, file) {
			inside++
		}
	}
	if inside == 0 {
		return false, nil
	}
	if inside != len(files) {
		return false, fmt.Errorf("metadata package mixes workspace and external files: %s", pkg.PkgPath)
	}
	return true, nil
}

func metadataFiles(pkg *packages.Package) []string {
	if len(pkg.CompiledGoFiles) != 0 {
		return pkg.CompiledGoFiles
	}
	return pkg.GoFiles
}

func metadataBasePath(pkg *packages.Package) (string, error) {
	if pkg == nil || pkg.PkgPath == "" {
		return "", fmt.Errorf("metadata package has no import path")
	}
	if pkg.ForTest != "" {
		return pkg.ForTest, nil
	}
	return pkg.PkgPath, nil
}

func metadataImportBase(importPath string, dependency *packages.Package) (string, error) {
	if dependency == nil {
		if importPath == "" {
			return "", fmt.Errorf("metadata has empty unresolved import")
		}
		return importPath, nil
	}
	return metadataBasePath(dependency)
}

func isSyntheticTestMain(pkg *packages.Package) bool {
	return pkg != nil && pkg.ForTest != "" && pkg.Name == "main" && pkg.PkgPath != pkg.ForTest
}

// removedExportedSourceOwners identifies public surfaces removed from the
// target state. They are optional because an isolated retired package may no
// longer exist after the cut; any surviving importer is already a metadata
// failure. Relocation targets remain ordinary target requests instead.
func removedExportedSourceOwners(intent cutplan.Intent) ([]string, error) {
	objects, err := cutplan.ImpactObjects(intent)
	if err != nil {
		return nil, err
	}
	requirements, err := cutplan.ResolutionRequirements(intent)
	if err != nil {
		return nil, err
	}
	roles := make(map[string]cutplan.ObjectRole, len(requirements))
	for _, requirement := range requirements {
		roles[requirement.Object.Object] = requirement.Role
	}
	seen := map[string]bool{}
	for _, object := range objects {
		if roles[object.Object] != cutplan.ObjectSource {
			continue
		}
		owner, exported, err := symbolOwnerAndExported(object)
		if err != nil {
			return nil, err
		}
		if exported {
			seen[owner] = true
		}
	}
	result := make([]string, 0, len(seen))
	for owner := range seen {
		result = append(result, owner)
	}
	sort.Strings(result)
	return result, nil
}

func symbolOwnerAndExported(value cutplan.SymbolRef) (string, bool, error) {
	object := value.Object
	owner, member, found := strings.Cut(object, "#")
	if !found || owner == "" || member == "" {
		return "", false, fmt.Errorf("invalid canonical impact object: %q", object)
	}
	separator := strings.LastIndex(member, ":")
	if separator < 0 || separator == len(member)-1 {
		return "", false, fmt.Errorf("invalid canonical impact terminal: %q", object)
	}
	terminal := member[separator+1:]
	if !token.IsIdentifier(terminal) {
		return "", false, fmt.Errorf("invalid canonical impact terminal: %q", object)
	}
	first, _ := utf8.DecodeRuneInString(terminal)
	return owner, unicode.IsUpper(first), nil
}
