package semantic

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"golang.org/x/tools/go/packages"
)

const packagesAuthority = "golang.org/x/tools/go/packages@v0.35.0"

type packagesLoader struct{}

// These private seams make the load-boundary law testable without turning
// semantic resolution into a second configurable authority.
var loadPackages = packages.Load
var measureSemanticToolchain = semanticToolchain

func (loader packagesLoader) Load(ctx context.Context, request LoadRequest) (LoadResult, error) {
	if request.Root == "" || request.Scratch == "" {
		return LoadResult{}, fmt.Errorf("invalid structured load boundary")
	}
	if len(request.Environment) == 0 || len(request.BuildFlags) == 0 || len(request.Patterns) == 0 {
		return LoadResult{}, fmt.Errorf("incomplete structured load context")
	}
	toolchain, err := measureSemanticToolchain(ctx, request.Root, request.Environment, request.BuildFlags, request.Patterns)
	if err != nil {
		return LoadResult{}, err
	}
	metadataConfig := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedModule | packages.NeedForTest,
		Context: ctx, Dir: request.Root, Env: cloneStrings(request.Environment),
		Tests: true, Overlay: cloneOverlay(request.Overlay), BuildFlags: cloneStrings(request.BuildFlags),
	}
	metadata, err := loadPackages(metadataConfig, "./...")
	if err != nil {
		return LoadResult{}, fmt.Errorf("structured workspace metadata: %w", err)
	}
	if err := validateMetadataPackages(metadata); err != nil {
		return LoadResult{}, err
	}
	patterns, err := typedImpactPatterns(request.Root, metadata, request.Requests, request.scope)
	if err != nil {
		return LoadResult{}, err
	}
	fset := token.NewFileSet()
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedModule | packages.NeedImports,
		Context: ctx,
		Dir:     request.Root,
		Env:     cloneStrings(request.Environment),
		Fset:    fset,
		Tests:   true,
		Overlay: cloneOverlay(request.Overlay),
		// Semantic resolution never produces a build artifact. Disable VCS
		// stamping so an otherwise valid disposable/shadow module cannot fail
		// merely because its parent checkout lacks VCS metadata.
		BuildFlags: cloneStrings(request.BuildFlags),
	}
	loaded, err := loadPackages(config, patterns...)
	if err != nil {
		return LoadResult{}, fmt.Errorf("structured workspace load: %w", err)
	}
	postToolchain, err := measureSemanticToolchain(ctx, request.Root, request.Environment, request.BuildFlags, request.Patterns)
	if err != nil {
		return LoadResult{}, fmt.Errorf("structured loader post-load identity: %w", err)
	}
	if postToolchain != toolchain {
		return LoadResult{}, fmt.Errorf("semantic load context changed during structured workspace load")
	}
	result := LoadResult{Toolchain: toolchain}
	if len(loaded) == 0 {
		result.WorkspaceFailures = []string{"structured workspace load returned no packages"}
		return result, nil
	}
	all := allPackages(loaded)
	for _, pkg := range all {
		for _, packageError := range pkg.Errors {
			if packageError.Kind == packages.ListError {
				result.WorkspaceFailures = append(result.WorkspaceFailures, packageError.Error())
				continue
			}
			diagnostic, diagnosticErr := loaderDiagnostic(request.Root, packageError)
			if diagnosticErr != nil {
				return LoadResult{}, diagnosticErr
			}
			if selectedDiagnostic(diagnostic.Position.Path, request.DiagnosticPaths) {
				result.Diagnostics = append(result.Diagnostics, diagnostic)
			}
		}
	}
	if len(result.WorkspaceFailures) != 0 {
		return result, nil
	}
	var excluded []string
	if pathInsideRoot(request.Root, request.Scratch) {
		excluded = append(excluded, request.Scratch)
	}
	workspace, err := buildWorkspace(request.Root, fset, all, request.Overlay, excluded...)
	if err != nil {
		return LoadResult{}, err
	}
	objects, err := workspace.resolve(request.Requests)
	if err != nil {
		return LoadResult{}, err
	}
	result.Workspace = workspace
	result.Objects = objects
	result.Diagnostics = canonicalDiagnostics(result.Diagnostics)
	return result, nil
}

func validateMetadataPackages(loaded []*packages.Package) error {
	for _, pkg := range allPackages(loaded) {
		for _, packageError := range pkg.Errors {
			if packageError.Kind == packages.ListError {
				return fmt.Errorf("structured workspace metadata failed: %s", packageError.Error())
			}
		}
	}
	return nil
}

// pathInsideRoot distinguishes a session scratch tree placed below a physical
// checkout (which must not become semantic source) from a virtual shadow
// placed below that scratch tree (where excluding the ancestor would erase the whole
// post-state workspace).
func pathInsideRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func packagesEnvironment() []string {
	values := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GOPACKAGESDRIVER=") {
			values = append(values, value)
		}
	}
	return append(values, "GOPACKAGESDRIVER=off")
}

func cloneOverlay(source map[string][]byte) map[string][]byte {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string][]byte, len(source))
	for path, data := range source {
		result[path] = append([]byte(nil), data...)
	}
	return result
}

func allPackages(roots []*packages.Package) []*packages.Package {
	seen := map[string]bool{}
	var result []*packages.Package
	packages.Visit(roots, nil, func(pkg *packages.Package) {
		if pkg == nil || seen[pkg.ID] {
			return
		}
		seen[pkg.ID] = true
		result = append(result, pkg)
	})
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func packageFieldOwners(pkg *packages.Package) map[types.Object]string {
	owners := map[types.Object]string{}
	if pkg.TypesInfo == nil {
		return owners
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			specification, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			structure, ok := specification.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					if object := pkg.TypesInfo.Defs[name]; object != nil {
						owners[object] = specification.Name.Name
					}
				}
			}
			return false
		})
	}
	return owners
}

func symbolRef(object types.Object, fieldOwner string) (cutplan.SymbolRef, error) {
	if object.Pkg() == nil {
		return cutplan.SymbolRef{}, fmt.Errorf("object has no package")
	}
	prefix := object.Pkg().Path() + "#"
	switch value := object.(type) {
	case *types.TypeName:
		return cutplan.SymbolRef{Object: prefix + "package:" + value.Name()}, nil
	case *types.Func:
		signature, ok := value.Type().(*types.Signature)
		if !ok {
			return cutplan.SymbolRef{}, fmt.Errorf("function has non-signature type")
		}
		if signature.Recv() == nil {
			return cutplan.SymbolRef{Object: prefix + "package:" + value.Name()}, nil
		}
		receiver, ok := namedType(signature.Recv().Type())
		if !ok {
			return cutplan.SymbolRef{}, fmt.Errorf("method receiver is not a named type")
		}
		return cutplan.SymbolRef{Object: prefix + "type:" + receiver.Obj().Name() + "/method:" + value.Name()}, nil
	case *types.Var:
		if !value.IsField() {
			return cutplan.SymbolRef{Object: prefix + "package:" + value.Name()}, nil
		}
		if fieldOwner == "" {
			return cutplan.SymbolRef{}, fmt.Errorf("struct field has no named owner")
		}
		return cutplan.SymbolRef{Object: prefix + "type:" + fieldOwner + "/field:" + value.Name()}, nil
	case *types.Const:
		return cutplan.SymbolRef{Object: prefix + "package:" + value.Name()}, nil
	default:
		return cutplan.SymbolRef{}, fmt.Errorf("unsupported object kind %T", object)
	}
}

func namedType(value types.Type) (*types.Named, bool) {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	return named, ok
}

func objectKey(root string, fset *token.FileSet, object types.Object) (string, error) {
	position, err := tokenPosition(root, fset, object.Pos())
	if err != nil {
		return "", err
	}
	packagePath := ""
	if object.Pkg() != nil {
		packagePath = object.Pkg().Path()
	}
	return packagePath + "\x00" + fmt.Sprintf("%T", object) + "\x00" + object.Name() + "\x00" + positionKey(position), nil
}

func tokenPosition(root string, fset *token.FileSet, position token.Pos) (cutplan.Position, error) {
	value := fset.PositionFor(position, false)
	if value.Filename == "" || value.Line < 1 || value.Column < 1 {
		return cutplan.Position{}, fmt.Errorf("unpositioned source object")
	}
	_, relative, err := workspacePath(root, value.Filename, true)
	if err != nil {
		return cutplan.Position{}, err
	}
	return cutplan.Position{Path: relative, Offset: value.Offset, Line: value.Line, Column: value.Column}, nil
}

func tokenSitePosition(root string, fset *token.FileSet, packageID string, position token.Pos, role cutplan.SiteRole) (cutplan.Position, error) {
	if packageID == "" {
		return cutplan.Position{}, fmt.Errorf("semantic site has no package variant")
	}
	value, err := tokenPosition(root, fset, position)
	if err != nil {
		return cutplan.Position{}, err
	}
	value.PackageIDs = []string{packageID}
	value.Role = role
	return value, nil
}

var packageErrorPosition = regexp.MustCompile(`^(.+):(\d+):(\d+)$`)

func loaderDiagnostic(root string, packageError packages.Error) (Diagnostic, error) {
	match := packageErrorPosition.FindStringSubmatch(packageError.Pos)
	if match == nil {
		return Diagnostic{}, fmt.Errorf("unpositioned structured package diagnostic %q: %s", packageError.Pos, packageError.Msg)
	}
	line, _ := strconv.Atoi(match[2])
	column, _ := strconv.Atoi(match[3])
	_, relative, err := workspacePath(root, match[1], true)
	if err != nil {
		return Diagnostic{}, fmt.Errorf("structured package diagnostic: %w", err)
	}
	message := strings.TrimSpace(packageError.Msg)
	if message == "" {
		return Diagnostic{}, fmt.Errorf("empty structured package diagnostic")
	}
	return Diagnostic{Position: cutplan.Position{Path: relative, Line: line, Column: column}, Message: message, Kind: fmt.Sprintf("%d", packageError.Kind)}, nil
}

func selectedDiagnostic(path string, selected []string) bool {
	if len(selected) == 0 {
		return true
	}
	for _, candidate := range selected {
		if candidate == path {
			return true
		}
	}
	return false
}

func uniquePositions(values []cutplan.Position) ([]cutplan.Position, error) {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		key := positionKey(value)
		if seen[key] {
			return nil, fmt.Errorf("duplicate resolved reference %s", key)
		}
		seen[key] = true
	}
	return values, nil
}
