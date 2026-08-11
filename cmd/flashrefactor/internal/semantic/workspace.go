package semantic

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"golang.org/x/tools/go/packages"
)

// Workspace is one borrowed, immutable semantic load. Its indexes and source
// bytes are immutable after construction. AST and go/types nodes are borrowed:
// a renderer may clone and mutate a node, but must never expect TypesInfo to
// describe newly-created nodes. Existing node identities remain valid for the
// lifetime of this Workspace.
type Workspace struct {
	root      string
	fset      *token.FileSet
	packages  []WorkspacePackage
	files     []WorkspaceFile
	fileIndex map[string][]int
	objects   map[string][]workspaceObject
	imports   map[string][]resolvedImport
	loaded    []*packages.Package
}

// WorkspacePackage is the typed identity of one loaded package variant.
type WorkspacePackage struct {
	ID      string
	Path    string
	Name    string
	Imports []string
	Types   *types.Package
	Info    *types.Info
}

// WorkspaceFile is one exact loaded source file. Path is repository-relative;
// Source is copied at collection time so the evidence cannot observe later I/O.
type WorkspaceFile struct {
	Path        string
	PackageID   string
	PackagePath string
	AST         *ast.File
	Source      []byte
}

type workspaceObject struct {
	object     types.Object
	definition cutplan.Position
	packageID  string
}

// resolvedImport keeps one typed import object with the exact source spelling
// that produced it. The semantic declaration name and raw alias stay distinct:
// the effective Go identifier is derived by consumers, never stored here.
type resolvedImport struct {
	object *types.PkgName
	ref    cutplan.ImportRef
}

func buildWorkspace(root string, fset *token.FileSet, loaded []*packages.Package, overlay map[string][]byte, excludedRoots ...string) (*Workspace, error) {
	workspace := &Workspace{
		root: root, fset: fset, fileIndex: map[string][]int{},
		objects: map[string][]workspaceObject{}, imports: map[string][]resolvedImport{},
	}
	for _, pkg := range loaded {
		// go/packages returns a typed transitive graph. Dependencies are needed
		// by Go while type-checking the declared workspace, but their syntax is
		// not refactor authority. In particular, compiled dependency syntax may
		// live in GOCACHE. Admit only complete package variants whose every
		// source file is physically within this workspace root.
		if !packageInsideWorkspace(root, fset, pkg, excludedRoots...) {
			continue
		}
		workspace.loaded = append(workspace.loaded, pkg)
		if pkg.TypesInfo == nil || pkg.Types == nil {
			continue
		}
		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}
		sort.Strings(imports)
		workspace.packages = append(workspace.packages, WorkspacePackage{ID: pkg.ID, Path: pkg.PkgPath, Name: pkg.Name, Imports: imports, Types: pkg.Types, Info: pkg.TypesInfo})
		if err := workspace.addPackageObjects(pkg); err != nil {
			return nil, err
		}
		if err := workspace.addPackageFiles(pkg, overlay); err != nil {
			return nil, err
		}
	}
	sort.Slice(workspace.packages, func(i, j int) bool { return workspace.packages[i].ID < workspace.packages[j].ID })
	sort.Slice(workspace.files, func(i, j int) bool {
		if workspace.files[i].Path != workspace.files[j].Path {
			return workspace.files[i].Path < workspace.files[j].Path
		}
		return workspace.files[i].PackageID < workspace.files[j].PackageID
	})
	workspace.fileIndex = map[string][]int{}
	for index, file := range workspace.files {
		workspace.fileIndex[file.Path] = append(workspace.fileIndex[file.Path], index)
	}
	return workspace, nil
}

func packageInsideWorkspace(root string, fset *token.FileSet, pkg *packages.Package, excludedRoots ...string) bool {
	if pkg == nil || fset == nil || len(pkg.Syntax) == 0 {
		return false
	}
	for _, file := range pkg.Syntax {
		if file == nil {
			return false
		}
		filename := fset.PositionFor(file.Pos(), false).Filename
		if filename == "" {
			return false
		}
		full, _, err := workspacePath(root, filename, true)
		if err != nil || insideExcludedRoot(full, excludedRoots) {
			return false
		}
	}
	return true
}

func insideExcludedRoot(path string, roots []string) bool {
	for _, root := range roots {
		if root == "" {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return true
		}
	}
	return false
}

func (workspace *Workspace) addPackageObjects(pkg *packages.Package) error {
	owners := packageFieldOwners(pkg)
	for identifier, object := range pkg.TypesInfo.Defs {
		if object == nil {
			continue
		}
		ref, err := symbolRef(object, owners[object])
		if err != nil {
			continue // Objects outside the cutplan vocabulary cannot enter its index.
		}
		definition, err := tokenSitePosition(workspace.root, workspace.fset, pkg.ID, identifier.Pos(), cutplan.SiteDeclaration)
		if err != nil {
			continue
		}
		workspace.objects[ref.Object] = appendWorkspaceObject(workspace.objects[ref.Object], workspaceObject{object: object, definition: definition, packageID: pkg.ID})
	}
	return nil
}

func appendWorkspaceObject(values []workspaceObject, candidate workspaceObject) []workspaceObject {
	// The same physical declaration may be type-checked in several package
	// variants. Keep every variant: the package ID is part of the lockable
	// semantic-site identity, so coalescing here would erase authority.
	return append(values, candidate)
}

func (workspace *Workspace) addPackageFiles(pkg *packages.Package, overlay map[string][]byte) error {
	for _, file := range pkg.Syntax {
		absolute := workspace.fset.PositionFor(file.Pos(), false).Filename
		if absolute == "" {
			return fmt.Errorf("loaded package %s has unpositioned syntax", pkg.ID)
		}
		full, relative, err := workspacePath(workspace.root, absolute, true)
		if err != nil {
			return err
		}
		source, found := overlay[full]
		if found {
			source = append([]byte(nil), source...)
		} else {
			source, err = os.ReadFile(full)
			if err != nil {
				return fmt.Errorf("read loaded source %s: %w", relative, err)
			}
		}
		entry := WorkspaceFile{Path: relative, PackageID: pkg.ID, PackagePath: pkg.PkgPath, AST: file, Source: source}
		workspace.files = append(workspace.files, entry)
		imports, importErr := resolveFileImports(file, pkg.TypesInfo)
		if importErr != nil {
			return fmt.Errorf("resolve imports for %s: %w", relative, importErr)
		}
		workspace.imports[workspaceImportKey(relative, pkg.ID)] = imports
	}
	return nil
}

func resolveFileImports(file *ast.File, info *types.Info) ([]resolvedImport, error) {
	if file == nil || info == nil {
		return nil, fmt.Errorf("missing typed import syntax")
	}
	result := make([]resolvedImport, 0, len(file.Imports))
	for _, specification := range file.Imports {
		if specification == nil || specification.Path == nil {
			return nil, fmt.Errorf("malformed import specification")
		}
		object, _ := info.Implicits[specification].(*types.PkgName)
		if object == nil && specification.Name != nil {
			object, _ = info.Defs[specification.Name].(*types.PkgName)
		}
		if object == nil || object.Imported() == nil {
			return nil, fmt.Errorf("unresolved import %s", specification.Path.Value)
		}
		path, err := strconv.Unquote(specification.Path.Value)
		if err != nil || path == "" || path != object.Imported().Path() {
			return nil, fmt.Errorf("import path does not match resolved package")
		}
		alias := ""
		if specification.Name != nil {
			alias = specification.Name.Name
			if alias == "." || alias == "_" {
				return nil, fmt.Errorf("dot and blank imports are outside semantic import authority")
			}
		}
		name := object.Imported().Name()
		if name == "" || name == "_" {
			return nil, fmt.Errorf("resolved import has unusable package clause")
		}
		result = append(result, resolvedImport{object: object, ref: cutplan.ImportRef{Path: path, Name: name, Alias: alias}})
	}
	return result, nil
}

// Files returns deterministic shallow copies of all loaded source variants.
func (workspace *Workspace) Files() []WorkspaceFile {
	if workspace == nil {
		return nil
	}
	return append([]WorkspaceFile(nil), workspace.files...)
}

// FileSet returns the borrowed token.FileSet shared by every AST, type object,
// and source position in this Workspace.
func (workspace *Workspace) FileSet() *token.FileSet {
	if workspace == nil {
		return nil
	}
	return workspace.fset
}

// Packages returns deterministic shallow copies of all typed package variants.
func (workspace *Workspace) Packages() []WorkspacePackage {
	if workspace == nil {
		return nil
	}
	return append([]WorkspacePackage(nil), workspace.packages...)
}

// File returns the exact loaded source by repo path. Multiple non-identical
// package variants are an ambiguity, never an arbitrary choice.
func (workspace *Workspace) File(path string) (WorkspaceFile, error) {
	if workspace == nil {
		return WorkspaceFile{}, fmt.Errorf("nil semantic workspace")
	}
	// A virtual collection owns immutable source bytes after its disposable
	// shadow has been cleaned, so canonical lookup validates path containment
	// without requiring that transient file to remain on disk.
	_, relative, err := workspacePath(workspace.root, path, false)
	if err != nil {
		return WorkspaceFile{}, err
	}
	indexes := workspace.fileIndex[relative]
	if len(indexes) == 0 {
		return WorkspaceFile{}, fmt.Errorf("loaded source file not found: %s", relative)
	}
	index, err := workspace.canonicalFileIndex(relative, indexes)
	if err != nil {
		return WorkspaceFile{}, err
	}
	return workspace.files[index], nil
}

func (workspace *Workspace) canonicalFileIndex(path string, indexes []int) (int, error) {
	if len(indexes) == 1 {
		return indexes[0], nil
	}
	base := make([]int, 0, 1)
	for _, index := range indexes {
		file := workspace.files[index]
		if !isTestSource(path) && file.PackageID == file.PackagePath {
			base = append(base, index)
		}
	}
	if len(base) == 1 {
		return base[0], nil
	}
	if len(base) > 1 {
		return 0, fmt.Errorf("loaded source file has multiple base package variants: %s", path)
	}
	return 0, fmt.Errorf("loaded source file is ambiguous across package variants: %s", path)
}

func isTestSource(path string) bool {
	base := filepath.Base(path)
	return len(base) > len("_test.go") && base[len(base)-len("_test.go"):] == "_test.go"
}

// PackageForFile returns the exact typed package variant that owns file.
func (workspace *Workspace) PackageForFile(file WorkspaceFile) (WorkspacePackage, error) {
	if workspace == nil {
		return WorkspacePackage{}, fmt.Errorf("nil semantic workspace")
	}
	for _, candidate := range workspace.packages {
		if candidate.ID == file.PackageID {
			return candidate, nil
		}
	}
	return WorkspacePackage{}, fmt.Errorf("typed package not found for %s", file.Path)
}

// Object resolves the deterministic renderer authority for one reviewed
// identity. Resolver evidence keeps every package variant, but rendering uses
// the base package for ordinary files and the exact test variant for test
// files. Any remaining choice is real ambiguity and fails closed.
func (workspace *Workspace) Object(ref cutplan.SymbolRef) (types.Object, error) {
	object, found, err := workspace.LookupObject(ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("resolved object not found: %s", ref.Object)
	}
	return object, nil
}

// LookupObject resolves one exact canonical object without conflating absence
// with resolver ambiguity. A prospective cut uses this to prove that a target
// identity is genuinely new; callers must still treat any returned error as a
// hard semantic failure.
func (workspace *Workspace) LookupObject(ref cutplan.SymbolRef) (types.Object, bool, error) {
	if workspace == nil {
		return nil, false, fmt.Errorf("nil semantic workspace")
	}
	if len(workspace.objects[ref.Object]) == 0 {
		return nil, false, nil
	}
	value, err := workspace.canonicalObject(ref)
	if err != nil {
		return nil, false, err
	}
	return value.object, true, nil
}

// ObjectForFile resolves ref in the exact type universe visible to file.
//
// Declaration work uses Object's canonical owner selection. A route is
// different: a consumer may be an internal or external test package whose
// typed import refers to a distinct package instance. This method therefore
// accepts only the local package variant or an object whose package pointer is
// exactly the resolved PkgName import for that file. It never falls back to a
// package-path string comparison. Zero or multiple visible candidates are a
// semantic ambiguity, not an invitation for a renderer to guess.
func (workspace *Workspace) ObjectForFile(ref cutplan.SymbolRef, file WorkspaceFile) (types.Object, error) {
	if workspace == nil {
		return nil, fmt.Errorf("nil semantic workspace")
	}
	if file.Path == "" || file.PackageID == "" {
		return nil, fmt.Errorf("object projection needs an exact workspace file")
	}
	if !workspace.hasFileVariant(file.Path, file.PackageID) {
		return nil, fmt.Errorf("object projection file is not loaded: %s (%s)", file.Path, file.PackageID)
	}
	values := workspace.objects[ref.Object]
	if len(values) == 0 {
		return nil, fmt.Errorf("resolved object not found: %s", ref.Object)
	}
	visible := make(map[types.Object]struct{}, len(values))
	for _, candidate := range values {
		if candidate.object != nil && candidate.packageID == file.PackageID {
			visible[candidate.object] = struct{}{}
		}
	}
	imports := workspace.imports[workspaceImportKey(file.Path, file.PackageID)]
	for _, imported := range imports {
		if imported.object == nil || imported.object.Imported() == nil {
			return nil, fmt.Errorf("file %s has an incomplete resolved import", file.Path)
		}
		for _, candidate := range values {
			if candidate.object != nil && candidate.object.Pkg() == imported.object.Imported() {
				visible[candidate.object] = struct{}{}
			}
		}
	}
	if len(visible) == 0 {
		return nil, fmt.Errorf("resolved object %s is not visible in %s (%s)", ref.Object, file.Path, file.PackageID)
	}
	if len(visible) != 1 {
		return nil, fmt.Errorf("resolved object %s is ambiguous in %s (%s): %d visible variants", ref.Object, file.Path, file.PackageID, len(visible))
	}
	for object := range visible {
		return object, nil
	}
	panic("unreachable visible object projection")
}

func (workspace *Workspace) hasFileVariant(path, packageID string) bool {
	for _, index := range workspace.fileIndex[path] {
		if index < 0 || index >= len(workspace.files) {
			continue
		}
		if workspace.files[index].PackageID == packageID {
			return true
		}
	}
	return false
}

func (workspace *Workspace) canonicalObject(ref cutplan.SymbolRef) (workspaceObject, error) {
	if workspace == nil {
		return workspaceObject{}, fmt.Errorf("nil semantic workspace")
	}
	values := workspace.objects[ref.Object]
	if len(values) == 0 {
		return workspaceObject{}, fmt.Errorf("resolved object not found: %s", ref.Object)
	}
	for _, value := range values[1:] {
		if value.definition.Path != values[0].definition.Path || value.definition.Offset != values[0].definition.Offset || value.definition.Line != values[0].definition.Line || value.definition.Column != values[0].definition.Column {
			return workspaceObject{}, fmt.Errorf("resolved object is ambiguous: %s", ref.Object)
		}
	}
	indexes := workspace.fileIndex[values[0].definition.Path]
	index, err := workspace.canonicalFileIndex(values[0].definition.Path, indexes)
	if err != nil {
		return workspaceObject{}, fmt.Errorf("canonical object %s: %w", ref.Object, err)
	}
	packageID := workspace.files[index].PackageID
	candidates := make([]workspaceObject, 0, 1)
	for _, value := range values {
		if value.packageID == packageID {
			candidates = append(candidates, value)
		}
	}
	if len(candidates) != 1 {
		return workspaceObject{}, fmt.Errorf("resolved object is ambiguous in canonical package variant %s: %s", packageID, ref.Object)
	}
	return candidates[0], nil
}

// ImportPkgName resolves the exact import object used in one package variant.
func (workspace *Workspace) ImportPkgName(file WorkspaceFile, importPath string) (*types.PkgName, error) {
	if workspace == nil {
		return nil, fmt.Errorf("nil semantic workspace")
	}
	var object *types.PkgName
	for _, candidate := range workspace.imports[workspaceImportKey(file.Path, file.PackageID)] {
		if candidate.ref.Path != importPath {
			continue
		}
		if object != nil {
			return nil, fmt.Errorf("import %s is ambiguous in %s", importPath, file.Path)
		}
		object = candidate.object
	}
	if object == nil {
		return nil, fmt.Errorf("import %s is not resolved in %s", importPath, file.Path)
	}
	return object, nil
}

func workspaceImportKey(path, packageID string) string { return path + "\x00" + packageID }

func (workspace *Workspace) resolve(requests []SymbolRequest) ([]cutplan.ObjectEvidence, error) {
	result := make([]cutplan.ObjectEvidence, 0, len(requests))
	for _, request := range requests {
		selected, err := workspace.canonicalObject(request.Object)
		if err != nil {
			return nil, err
		}
		references, err := workspace.references(request.Object, selected.object)
		if err != nil {
			return nil, err
		}
		definition, err := workspace.definition(request.Object)
		if err != nil {
			return nil, err
		}
		if selected.object.Pkg() == nil || selected.object.Pkg().Name() == "" {
			return nil, fmt.Errorf("resolved object has no package clause: %s", request.Object.Object)
		}
		result = append(result, cutplan.ObjectEvidence{Object: request.Object, Role: request.Role, Package: selected.object.Pkg().Name(), Definition: definition, References: references})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Object.Object < result[j].Object.Object })
	return result, nil
}

func (workspace *Workspace) references(ref cutplan.SymbolRef, object types.Object) ([]cutplan.Position, error) {
	wanted, err := objectKey(workspace.root, workspace.fset, object)
	if err != nil {
		return nil, err
	}
	references := make([]cutplan.Position, 0)
	for _, pkg := range workspace.loaded {
		if pkg.TypesInfo == nil {
			continue
		}
		roles := siteRoles(pkg)
		for identifier, used := range pkg.TypesInfo.Uses {
			if used == nil {
				continue
			}
			key, keyErr := objectKey(workspace.root, workspace.fset, used)
			if keyErr != nil || key != wanted {
				continue
			}
			role := roles[identifier]
			if role == "" {
				role = cutplan.SiteUse
			}
			position, positionErr := tokenSitePosition(workspace.root, workspace.fset, pkg.ID, identifier.Pos(), role)
			if positionErr != nil {
				return nil, fmt.Errorf("resolved use %s: %w", ref.Object, positionErr)
			}
			references = append(references, position)
		}
	}
	return aggregateSites(references), nil
}

func (workspace *Workspace) definition(ref cutplan.SymbolRef) (cutplan.Position, error) {
	values := workspace.objects[ref.Object]
	definitions := make([]cutplan.Position, 0, len(values))
	for _, value := range values {
		definitions = append(definitions, value.definition)
	}
	aggregated := aggregateSites(definitions)
	if len(aggregated) != 1 || aggregated[0].Role != cutplan.SiteDeclaration {
		return cutplan.Position{}, fmt.Errorf("resolved object has ambiguous declaration sites: %s", ref.Object)
	}
	return aggregated[0], nil
}

func siteRoles(pkg *packages.Package) map[*ast.Ident]cutplan.SiteRole {
	result := map[*ast.Ident]cutplan.SiteRole{}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.SelectorExpr:
				if value.Sel != nil {
					result[value.Sel] = cutplan.SiteSelector
				}
			case *ast.ImportSpec:
				if value.Name != nil {
					result[value.Name] = cutplan.SiteImport
				}
			}
			return true
		})
	}
	return result
}

// aggregateSites turns all compiler variants of one physical AST site into
// one exact site with a complete sorted package-variant set. It deliberately
// does not require source and target cuts to have matching variant counts.
func aggregateSites(values []cutplan.Position) []cutplan.Position {
	result := make([]cutplan.Position, 0, len(values))
	indexes := make(map[string]int, len(values))
	for _, value := range values {
		key := physicalSiteKey(value)
		if index, exists := indexes[key]; exists {
			result[index].PackageIDs = append(result[index].PackageIDs, value.PackageIDs...)
			continue
		}
		value.PackageIDs = append([]string(nil), value.PackageIDs...)
		indexes[key] = len(result)
		result = append(result, value)
	}
	for index := range result {
		sort.Strings(result[index].PackageIDs)
		result[index].PackageIDs = uniqueStrings(result[index].PackageIDs)
	}
	return sortedPositions(result)
}

func physicalSiteKey(value cutplan.Position) string {
	return value.Path + "\x00" + strconv.Itoa(value.Offset) + "\x00" + strconv.Itoa(value.Line) + "\x00" + strconv.Itoa(value.Column) + "\x00" + string(value.Role)
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

// FilePath returns a canonical repository path for one loaded source filename.
// It is intentionally unexported from the renderer-facing API; callers use
// File and Files, which refuse ambiguity explicitly.
func (workspace *Workspace) FilePath(filename string) (string, error) {
	_, relative, err := workspacePath(workspace.root, filepath.Clean(filename), true)
	return relative, err
}

// Structure returns compiler-resolved package and file import facts. It is a
// copy suitable for a lock/report; callers cannot mutate the typed workspace.
func (workspace *Workspace) Structure() StructuralSnapshot {
	if workspace == nil {
		return StructuralSnapshot{}
	}
	result := StructuralSnapshot{
		Packages: make([]StructuralPackage, 0, len(workspace.packages)),
		Files:    make([]StructuralFile, 0, len(workspace.files)),
	}
	for _, pkg := range workspace.packages {
		result.Packages = append(result.Packages, StructuralPackage{
			ID: pkg.ID, Path: pkg.Path, Name: pkg.Name, Imports: append([]string(nil), pkg.Imports...),
		})
	}
	for _, file := range workspace.files {
		entry := StructuralFile{Path: file.Path, PackageID: file.PackageID, PackagePath: file.PackagePath}
		imports := workspace.imports[workspaceImportKey(file.Path, file.PackageID)]
		for _, imported := range imports {
			entry.Imports = append(entry.Imports, imported.ref)
		}
		sort.Slice(entry.Imports, func(i, j int) bool {
			if entry.Imports[i].Path != entry.Imports[j].Path {
				return entry.Imports[i].Path < entry.Imports[j].Path
			}
			return entry.Imports[i].Alias < entry.Imports[j].Alias
		})
		result.Files = append(result.Files, entry)
	}
	return canonicalStructure(result)
}

// ObjectResidue returns every compiler-resolved site for ref inside precisely
// the requested source paths. Missing object identity is an empty result;
// package-variant duplicates use the same canonical-object rule as rendering,
// while distinct declarations remain ambiguity.
func (workspace *Workspace) ObjectResidue(ref cutplan.SymbolRef, paths []string) (ObjectResidue, error) {
	if workspace == nil {
		return ObjectResidue{}, fmt.Errorf("nil semantic workspace")
	}
	if ref.Object == "" {
		return ObjectResidue{}, fmt.Errorf("residue object is empty")
	}
	wanted, err := workspace.residuePaths(paths)
	if err != nil {
		return ObjectResidue{}, err
	}
	if len(workspace.objects[ref.Object]) == 0 {
		return ObjectResidue{Object: ref}, nil
	}
	selected, err := workspace.canonicalObject(ref)
	if err != nil {
		return ObjectResidue{}, err
	}
	sites, err := workspace.references(ref, selected.object)
	if err != nil {
		return ObjectResidue{}, err
	}
	definition, err := workspace.definition(ref)
	if err != nil {
		return ObjectResidue{}, err
	}
	sites = append([]cutplan.Position{definition}, sites...)
	result := ObjectResidue{Object: ref}
	for _, site := range sites {
		if wanted[site.Path] {
			result.Sites = append(result.Sites, site)
		}
	}
	result.Sites = sortedPositions(result.Sites)
	return result, nil
}

func (workspace *Workspace) residuePaths(paths []string) (map[string]bool, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("residue paths are empty")
	}
	result := make(map[string]bool, len(paths))
	for _, path := range paths {
		_, relative, err := workspacePath(workspace.root, path, false)
		if err != nil {
			return nil, fmt.Errorf("residue path %q: %w", path, err)
		}
		if result[relative] {
			return nil, fmt.Errorf("duplicate residue path: %s", relative)
		}
		result[relative] = true
	}
	return result, nil
}
