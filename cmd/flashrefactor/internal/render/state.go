package render

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"iter"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/generate"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// fileState is a private, detached AST. ast.File and types.Info in a semantic
// Workspace are borrowed immutable evidence; mutating either would silently
// corrupt later collection. We parse a private AST and re-key the small slice
// of type information used by gorewrite onto its nodes.
type fileState struct {
	path        string
	packageName string
	origin      semantic.WorkspaceFile // exact immutable package variant cloned into file.
	source      []byte                 // exact preimage; nil means the path was absent.
	fset        *token.FileSet
	file        *ast.File
	info        *types.Info
	generated   []byte // non-Go provider output; mutually exclusive with file.
	deleted     bool
}

func (state *fileState) bytes() ([]byte, error) {
	if state.deleted {
		return nil, nil
	}
	if state.generated != nil {
		return append([]byte(nil), state.generated...), nil
	}
	var output bytes.Buffer
	if err := format.Node(&output, state.fset, state.file); err != nil {
		return nil, fmt.Errorf("format %s: %w", state.path, err)
	}
	return output.Bytes(), nil
}

func (state *fileState) ensureGo() error {
	if state.deleted {
		return fmt.Errorf("%s is retired", state.path)
	}
	if state.generated != nil {
		return fmt.Errorf("%s is provider-generated, not Go source", state.path)
	}
	if state.file == nil || state.fset == nil {
		return fmt.Errorf("%s has no Go syntax", state.path)
	}
	return nil
}

type renderState struct {
	workspace  *semantic.Workspace
	files      map[string]*fileState
	writes     map[string]struct{}
	hazards    []cutplanHazard
	providers  map[string]providerEvidence
	registry   generate.Registry
	provenance []Provenance
	witnesses  []RouteWitness
}

// These private aliases keep state.go independent from operation mechanics
// while preventing a sprawling renderer context struct.
type cutplanHazard struct {
	code, severity, detail string
	path                   string
}

type providerEvidence struct {
	name, identity string
}

func newRenderState(workspace *semantic.Workspace, writes []string, registry generate.Registry) (*renderState, error) {
	if workspace == nil {
		return nil, fmt.Errorf("render requires a pre-cut semantic workspace")
	}
	state := &renderState{
		workspace:  workspace,
		files:      make(map[string]*fileState),
		writes:     make(map[string]struct{}, len(writes)),
		providers:  make(map[string]providerEvidence),
		registry:   registry,
		provenance: nil,
		witnesses:  nil,
	}
	for _, path := range writes {
		if _, duplicate := state.writes[path]; duplicate {
			continue
		}
		state.writes[path] = struct{}{}
	}
	return state, nil
}

func (state *renderState) writeAllowed(path string) error {
	if _, ok := state.writes[path]; !ok {
		return fmt.Errorf("renderer attempted undeclared output %s", path)
	}
	return nil
}

func (state *renderState) file(path, packageName string) (*fileState, error) {
	if value := state.files[path]; value != nil {
		if packageName != "" && value.packageName != packageName {
			return nil, fmt.Errorf("%s package %q conflicts with destination package %q", path, value.packageName, packageName)
		}
		return value, nil
	}
	original, err := state.workspace.File(path)
	if err == nil {
		pkg, packageErr := state.workspace.PackageForFile(original)
		if packageErr != nil {
			return nil, packageErr
		}
		value, cloneErr := cloneWithInfo(original, state.workspace.FileSet(), pkg.Info)
		if cloneErr != nil {
			return nil, cloneErr
		}
		if packageName != "" && value.packageName != packageName {
			return nil, fmt.Errorf("%s declares package %q, not destination package %q", path, value.packageName, packageName)
		}
		state.files[path] = value
		return value, nil
	}
	if packageName == "" {
		return nil, fmt.Errorf("new destination %s has no declared package", path)
	}
	value, parseErr := newGoFile(path, packageName)
	if parseErr != nil {
		return nil, parseErr
	}
	state.files[path] = value
	return value, nil
}

func (state *renderState) existingFile(path string) (*fileState, semantic.WorkspaceFile, semantic.WorkspacePackage, error) {
	original, err := state.workspace.File(path)
	if err != nil {
		return nil, semantic.WorkspaceFile{}, semantic.WorkspacePackage{}, err
	}
	pkg, err := state.workspace.PackageForFile(original)
	if err != nil {
		return nil, semantic.WorkspaceFile{}, semantic.WorkspacePackage{}, err
	}
	value := state.files[path]
	if value == nil {
		value, err = cloneWithInfo(original, state.workspace.FileSet(), pkg.Info)
		if err != nil {
			return nil, semantic.WorkspaceFile{}, semantic.WorkspacePackage{}, err
		}
		state.files[path] = value
	}
	return value, original, pkg, nil
}

func cloneWithInfo(source semantic.WorkspaceFile, sourceSet *token.FileSet, info *types.Info) (*fileState, error) {
	if source.AST == nil || sourceSet == nil || info == nil {
		return nil, fmt.Errorf("%s has incomplete typed syntax", source.Path)
	}
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, source.Path, source.Source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse detached %s: %w", source.Path, err)
	}
	clonedInfo, err := cloneFileInfo(source.AST, file, sourceSet, set, info)
	if err != nil {
		return nil, err
	}
	return &fileState{
		path: source.Path, packageName: file.Name.Name, origin: source, source: append([]byte(nil), source.Source...),
		fset: set, file: file, info: clonedInfo,
	}, nil
}

func newGoFile(path, packageName string) (*fileState, error) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, "package "+packageName+"\n", parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("create destination %s: %w", path, err)
	}
	return &fileState{path: path, packageName: packageName, fset: set, file: file, info: &types.Info{}}, nil
}

// cloneFileInfo remaps only AST-node keys. The referenced type objects and
// selections are immutable compiler facts and deliberately stay shared. Node
// correspondence streams checked preorder occurrences, never source position:
// nested selectors such as pkg.Value.Field legitimately share one Pos().
//
// InitOrder intentionally remains nil. Its Rhs nodes may point into other
// files in the package and a file-local clone cannot preserve that authority.
// The renderer never consumes initialization order; a future consumer must
// own a whole-package clone rather than silently borrowing original AST nodes.
func cloneFileInfo(original, copied *ast.File, originalSet, copiedSet *token.FileSet, info *types.Info) (*types.Info, error) {
	result := &types.Info{
		Types:        map[ast.Expr]types.TypeAndValue{},
		Defs:         map[*ast.Ident]types.Object{},
		Uses:         map[*ast.Ident]types.Object{},
		Implicits:    map[ast.Node]types.Object{},
		Selections:   map[*ast.SelectorExpr]*types.Selection{},
		Scopes:       map[ast.Node]*types.Scope{},
		Instances:    map[*ast.Ident]types.Instance{},
		FileVersions: map[*ast.File]string{},
		InitOrder:    nil,
	}
	if info == nil {
		return result, nil
	}
	if err := zipTypedNodes(original, copied, originalSet, copiedSet, func(source, target ast.Node) {
		transferInfoEntry(result, info, source, target)
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func zipTypedNodes(original, copied ast.Node, originalSet, copiedSet *token.FileSet, receive func(ast.Node, ast.Node)) error {
	nextSource, stopSource := iter.Pull(ast.Preorder(original))
	defer stopSource()
	nextTarget, stopTarget := iter.Pull(ast.Preorder(copied))
	defer stopTarget()
	for occurrence := 0; ; occurrence++ {
		source, sourceOK := nextNonComment(nextSource)
		target, targetOK := nextNonComment(nextTarget)
		if sourceOK != targetOK {
			return fmt.Errorf("detached AST shape differs at typed occurrence %d", occurrence)
		}
		if !sourceOK {
			return nil
		}
		if reflect.TypeOf(source) != reflect.TypeOf(target) {
			return fmt.Errorf("detached AST shape differs at typed occurrence %d: %T source, %T clone", occurrence, source, target)
		}
		sourceStart, sourceEnd, err := nodeOffsets(originalSet, source)
		if err != nil {
			return fmt.Errorf("source AST occurrence %d: %w", occurrence, err)
		}
		targetStart, targetEnd, err := nodeOffsets(copiedSet, target)
		if err != nil {
			return fmt.Errorf("clone AST occurrence %d: %w", occurrence, err)
		}
		if sourceStart != targetStart || sourceEnd != targetEnd {
			return fmt.Errorf("detached AST span differs at typed occurrence %d: source [%d,%d), clone [%d,%d)", occurrence, sourceStart, sourceEnd, targetStart, targetEnd)
		}
		receive(source, target)
	}
}

func nextNonComment(next func() (ast.Node, bool)) (ast.Node, bool) {
	for {
		node, ok := next()
		if !ok || !isCommentNode(node) {
			return node, ok
		}
	}
}

func isCommentNode(node ast.Node) bool {
	switch node.(type) {
	case *ast.Comment, *ast.CommentGroup:
		return true
	default:
		return false
	}
}

func nodeOffsets(set *token.FileSet, node ast.Node) (int, int, error) {
	if set == nil || node == nil {
		return 0, 0, fmt.Errorf("node or file set is nil")
	}
	startFile, endFile := set.File(node.Pos()), set.File(node.End())
	if startFile == nil || endFile == nil || startFile != endFile {
		return 0, 0, fmt.Errorf("node has no single local source span")
	}
	return startFile.Offset(node.Pos()), endFile.Offset(node.End()), nil
}

func transferInfoEntry(target, source *types.Info, original, copied ast.Node) {
	if expression, ok := original.(ast.Expr); ok {
		if value, exists := source.Types[expression]; exists {
			target.Types[copied.(ast.Expr)] = value
		}
	}
	if identifier, ok := original.(*ast.Ident); ok {
		copiedIdentifier := copied.(*ast.Ident)
		if value, exists := source.Defs[identifier]; exists {
			target.Defs[copiedIdentifier] = value
		}
		if value, exists := source.Uses[identifier]; exists {
			target.Uses[copiedIdentifier] = value
		}
		if value, exists := source.Instances[identifier]; exists {
			target.Instances[copiedIdentifier] = value
		}
	}
	if value, exists := source.Implicits[original]; exists {
		target.Implicits[copied] = value
	}
	if selector, ok := original.(*ast.SelectorExpr); ok {
		if value, exists := source.Selections[selector]; exists {
			target.Selections[copied.(*ast.SelectorExpr)] = value
		}
	}
	if value, exists := source.Scopes[original]; exists {
		target.Scopes[copied] = value
	}
	if file, ok := original.(*ast.File); ok {
		if value, exists := source.FileVersions[file]; exists {
			target.FileVersions[copied.(*ast.File)] = value
		}
	}
}

func (state *renderState) output() (Output, error) {
	paths := make([]string, 0, len(state.writes))
	for path := range state.writes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	output := Output{}
	for _, path := range paths {
		file := state.files[path]
		if file == nil {
			return Output{}, fmt.Errorf("declared write %s has no final state", path)
		}
		if file.deleted {
			if file.source == nil {
				return Output{}, fmt.Errorf("declared write %s deletes an absent file", path)
			}
			output.Files = append(output.Files, semantic.VirtualFile{Path: path, Delete: true})
			output.Diffs = append(output.Diffs, DiffInput{Path: path, Before: append([]byte(nil), file.source...), Delete: true})
			continue
		}
		content, err := file.bytes()
		if err != nil {
			return Output{}, err
		}
		if file.source != nil && bytes.Equal(content, file.source) {
			return Output{}, fmt.Errorf("declared write %s is unchanged", path)
		}
		output.Files = append(output.Files, semantic.VirtualFile{Path: path, Content: content})
		output.Diffs = append(output.Diffs, DiffInput{Path: path, Before: append([]byte(nil), file.source...), After: append([]byte(nil), content...)})
	}
	for _, value := range canonicalHazards(state.hazards) {
		output.Hazards = append(output.Hazards, value)
	}
	for _, value := range canonicalProviders(state.providers) {
		output.Providers = append(output.Providers, value)
	}
	output.Provenance = canonicalProvenance(state.provenance)
	output.Witnesses = canonicalWitnesses(state.witnesses)
	return output, nil
}

func sourcePathForObject(workspace *semantic.Workspace, object types.Object) (string, error) {
	if workspace == nil || workspace.FileSet() == nil || object == nil {
		return "", fmt.Errorf("object has no semantic source")
	}
	position := workspace.FileSet().PositionFor(object.Pos(), false)
	if position.Filename == "" {
		return "", fmt.Errorf("object %s has no source position", object.Name())
	}
	path, err := workspace.FilePath(filepath.Clean(position.Filename))
	if err != nil {
		return "", err
	}
	return path, nil
}
