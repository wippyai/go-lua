package render

import (
	"fmt"
	"go/ast"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// capturedWitness retains the detached AST identity captured after complete
// preflight. The pointer never crosses a package boundary as semantic truth;
// it is only a causal token that must survive the mechanical rewrite.
type capturedWitness struct {
	from     cutplan.SymbolRef
	to       cutplan.SymbolRef
	source   PhysicalWitnessSite
	ident    *ast.Ident
	expected string
}

func (compiler *compiler) captureWitnesses(intent cutplan.Intent) error {
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			if edit.Kind != cutplan.EditRelocate || edit.Relocate == nil {
				continue
			}
			for _, subject := range edit.Relocate.Subjects {
				captures, err := compiler.captureRelocation(subject)
				if err != nil {
					return fmt.Errorf("operation %s relocation %s -> %s: %w", operation.ID, subject.From.Object, subject.To.Object, err)
				}
				compiler.witnesses = append(compiler.witnesses, captures...)
			}
		}
	}
	return nil
}

func (compiler *compiler) captureRelocation(subject cutplan.Relocation) ([]capturedWitness, error) {
	expectedName, err := targetName(subject.To)
	if err != nil {
		return nil, err
	}
	expected, err := snapshotSourceSites(compiler.snapshot, subject.From)
	if err != nil {
		return nil, err
	}
	captured, err := compiler.captureObjectSites(subject.From, subject.To, expectedName, expected)
	if err != nil {
		return nil, err
	}
	return compiler.compareSourceDenominator(subject.From, captured)
}

func (compiler *compiler) captureObjectSites(from, to cutplan.SymbolRef, expectedName string, expected []cutplan.Position) ([]capturedWitness, error) {
	paths := make(map[string]struct{}, len(expected))
	for _, position := range expected {
		paths[position.Path] = struct{}{}
	}
	orderedPaths := make([]string, 0, len(paths))
	for path := range paths {
		orderedPaths = append(orderedPaths, path)
	}
	sort.Strings(orderedPaths)
	seen := map[string]bool{}
	var result []capturedWitness
	for _, path := range orderedPaths {
		file := compiler.state.files[path]
		if file == nil || file.file == nil || file.info == nil || file.deleted || file.generated != nil || file.origin.AST == nil {
			return nil, fmt.Errorf("semantic source site was not loaded as a typed detached file: %s", path)
		}
		object, err := compiler.state.workspace.ObjectForFile(from, file.origin)
		if err != nil {
			return nil, fmt.Errorf("witness source %s in %s: %w", from.Object, path, err)
		}
		selectorTerminals := selectorTerminals(file.file)
		for ident, resolved := range file.info.Defs {
			if resolved != object {
				continue
			}
			capture, err := witnessFromIdent(file, ident, cutplan.SiteDeclaration, from, to, expectedName)
			if err != nil {
				return nil, err
			}
			key := witnessSourceKey(capture.source)
			if seen[key] {
				return nil, fmt.Errorf("duplicate detached definition site %s", key)
			}
			seen[key] = true
			result = append(result, capture)
		}
		for ident, resolved := range file.info.Uses {
			if resolved != object {
				continue
			}
			role := cutplan.SiteUse
			if selectorTerminals[ident] {
				role = cutplan.SiteSelector
			}
			capture, err := witnessFromIdent(file, ident, role, from, to, expectedName)
			if err != nil {
				return nil, err
			}
			key := witnessSourceKey(capture.source)
			if seen[key] {
				return nil, fmt.Errorf("identifier site has conflicting def/use roles: %s", key)
			}
			seen[key] = true
			result = append(result, capture)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no detached typed identifier sites were loaded")
	}
	sort.Slice(result, func(left, right int) bool {
		return witnessSourceKey(result[left].source) < witnessSourceKey(result[right].source)
	})
	return result, nil
}

func selectorTerminals(file *ast.File) map[*ast.Ident]bool {
	result := map[*ast.Ident]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel != nil {
			result[selector.Sel] = true
		}
		return true
	})
	return result
}

func witnessFromIdent(file *fileState, ident *ast.Ident, role cutplan.SiteRole, from, to cutplan.SymbolRef, expected string) (capturedWitness, error) {
	if file == nil || ident == nil || file.fset == nil {
		return capturedWitness{}, fmt.Errorf("unpositioned detached identifier")
	}
	position := file.fset.PositionFor(ident.Pos(), false)
	if position.Filename == "" {
		return capturedWitness{}, fmt.Errorf("detached identifier has no physical source position")
	}
	tokenFile := file.fset.File(ident.Pos())
	if tokenFile == nil {
		return capturedWitness{}, fmt.Errorf("detached identifier has no token file")
	}
	offset := tokenFile.Offset(ident.Pos())
	if offset < 0 || offset >= len(file.source) {
		return capturedWitness{}, fmt.Errorf("detached identifier offset %d is outside preimage %s", offset, file.path)
	}
	return capturedWitness{
		from: from, to: to, ident: ident, expected: expected,
		source: PhysicalWitnessSite{Path: file.path, Offset: offset, Role: role},
	}, nil
}

func (compiler *compiler) compareSourceDenominator(from cutplan.SymbolRef, captured []capturedWitness) ([]capturedWitness, error) {
	expected, err := snapshotSourceSites(compiler.snapshot, from)
	if err != nil {
		return nil, err
	}
	got := make(map[string]bool, len(captured))
	for _, witness := range captured {
		got[witnessSourceKey(witness.source)] = true
	}
	want := make(map[string]bool, len(expected))
	for _, position := range expected {
		file := compiler.state.files[position.Path]
		if file == nil || file.source == nil {
			return nil, fmt.Errorf("semantic source site %s:%d:%d was not loaded by the declared footprint", position.Path, position.Line, position.Column)
		}
		offset, offsetErr := positionOffset(file.source, position)
		if offsetErr != nil {
			return nil, offsetErr
		}
		if position.Role == "" {
			return nil, fmt.Errorf("semantic source position %s:%d has no site role", position.Path, offset)
		}
		key := witnessSourceKey(PhysicalWitnessSite{Path: position.Path, Offset: offset, Role: position.Role})
		if want[key] {
			continue // Semantic test-package variants may report one physical site repeatedly.
		}
		want[key] = true
	}
	for key := range want {
		if !got[key] {
			return nil, fmt.Errorf("semantic source site has no retained detached identifier witness: %s", key)
		}
	}
	for key := range got {
		if !want[key] {
			return nil, fmt.Errorf("detached identifier witness is absent from semantic source evidence: %s", key)
		}
	}
	return captured, nil
}

func snapshotSourceSites(snapshot semantic.Snapshot, from cutplan.SymbolRef) ([]cutplan.Position, error) {
	var result []cutplan.Position
	for index := range snapshot.Objects {
		candidate := &snapshot.Objects[index]
		if candidate.Object != from || candidate.Role != cutplan.ObjectSource {
			continue
		}
		// Definition is a distinct denominator field. Current collectors may
		// also repeat it in References; collapse that only after preserving its
		// physical role, so package variants cannot create a fake second use.
		result = append(result, candidate.Definition)
		result = append(result, candidate.References...)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("missing semantic source evidence for %s", from.Object)
	}
	return result, nil
}

func positionOffset(source []byte, position cutplan.Position) (int, error) {
	if position.Path == "" || position.Line < 1 || position.Column < 1 {
		return 0, fmt.Errorf("invalid semantic source position")
	}
	line, offset := 1, 0
	for line < position.Line {
		index := offset
		for index < len(source) && source[index] != '\n' {
			index++
		}
		if index == len(source) {
			return 0, fmt.Errorf("semantic source position %s:%d:%d exceeds preimage", position.Path, position.Line, position.Column)
		}
		offset = index + 1
		line++
	}
	offset += position.Column - 1
	if offset < 0 || offset >= len(source) {
		return 0, fmt.Errorf("semantic source position %s:%d:%d exceeds preimage", position.Path, position.Line, position.Column)
	}
	if position.Offset != offset {
		return 0, fmt.Errorf("semantic source position %s:%d:%d has inconsistent byte offset %d, want %d", position.Path, position.Line, position.Column, position.Offset, offset)
	}
	return position.Offset, nil
}

func witnessSourceKey(site PhysicalWitnessSite) string {
	return fmt.Sprintf("%s:%d:%s", site.Path, site.Offset, site.Role)
}

func (compiler *compiler) materializeWitnesses() error {
	if len(compiler.witnesses) == 0 {
		return nil
	}
	anchors := compiler.postAnchors()
	groups := map[string]*RouteWitness{}
	for _, witness := range compiler.witnesses {
		matches := anchors[witness.ident]
		if len(matches) == 0 {
			return fmt.Errorf("causal witness pointer was removed or replaced: %s", witnessSourceKey(witness.source))
		}
		if len(matches) != 1 {
			return fmt.Errorf("causal witness pointer is ambiguous in post AST: %s", witnessSourceKey(witness.source))
		}
		anchor := matches[0]
		if anchor.Name != witness.expected {
			return fmt.Errorf("causal witness %s retained an unexpected target name %q, want %q", witnessSourceKey(witness.source), anchor.Name, witness.expected)
		}
		anchor.Role = witness.source.Role
		key := witness.from.Object + "\x00" + witness.to.Object
		group := groups[key]
		if group == nil {
			group = &RouteWitness{From: witness.from, To: witness.to}
			groups[key] = group
		}
		group.Sites = append(group.Sites, RouteWitnessSite{Source: witness.source, Target: anchor})
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := groups[key]
		sort.Slice(group.Sites, func(left, right int) bool {
			return witnessSourceKey(group.Sites[left].Source) < witnessSourceKey(group.Sites[right].Source)
		})
		compiler.state.witnesses = append(compiler.state.witnesses, *group)
	}
	return nil
}

func (compiler *compiler) postAnchors() map[*ast.Ident][]StructuralAnchor {
	paths := make([]string, 0, len(compiler.state.files))
	for path := range compiler.state.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := map[*ast.Ident][]StructuralAnchor{}
	for _, path := range paths {
		file := compiler.state.files[path]
		if file == nil || file.file == nil || file.deleted || file.generated != nil {
			continue
		}
		index := 0
		ast.Inspect(file.file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			index++
			result[identifier] = append(result[identifier], StructuralAnchor{Path: path, Identifier: index, Name: identifier.Name})
			return true
		})
	}
	return result
}

func canonicalWitnesses(values []RouteWitness) []RouteWitness {
	result := make([]RouteWitness, 0, len(values))
	for _, value := range values {
		copyValue := RouteWitness{From: value.From, To: value.To, Sites: append([]RouteWitnessSite(nil), value.Sites...)}
		sort.Slice(copyValue.Sites, func(left, right int) bool {
			return witnessSourceKey(copyValue.Sites[left].Source) < witnessSourceKey(copyValue.Sites[right].Source)
		})
		result = append(result, copyValue)
	}
	sort.Slice(result, func(left, right int) bool {
		leftKey := result[left].From.Object + "\x00" + result[left].To.Object
		rightKey := result[right].From.Object + "\x00" + result[right].To.Object
		return leftKey < rightKey
	})
	return result
}
