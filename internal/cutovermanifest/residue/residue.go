// Package residue is the legacy-protocol census for one domain package: the
// HotRule/BindHot/SchemaFragment/DeclareSchema/DeclareRule/RegisterRule/
// BindRule occurrences a completed cutover must no longer reference, and the
// files that are structurally nothing but that residue.
//
// The token list and the file:line scan are internal/cutoververify's own
// (see internal/cutoververify/protocolzero.go LegacyProtocolTokens /
// ClassifyProtocolZero) - this package imports that exported classifier
// rather than re-deriving the pattern, so the two tools never drift apart.
package residue

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"

	"github.com/wippyai/go-lua/internal/cutoververify"
)

// Tokens are the legacy protocol identifiers a completed cutover no longer
// references. It is internal/cutoververify.LegacyProtocolTokens, re-exported
// so callers need not import cutoververify themselves.
var Tokens = cutoververify.LegacyProtocolTokens

// Hit is one legacy-token occurrence.
type Hit = cutoververify.ProtocolZeroHit

// Census scans dir (a domain package directory) for every legacy-protocol
// occurrence in its non-test .go files, ordered by file then line.
func Census(dir string) ([]Hit, error) {
	report, err := cutoververify.ClassifyProtocolZero(dir)
	if err != nil {
		return nil, fmt.Errorf("census %s: %w", dir, err)
	}
	return report.Hits, nil
}

// File is one file whose entire exported top-level surface reads as
// protocol residue.
type File struct {
	Path string
	Hits []Hit
	// NoExported reports that the file declares no exported top-level
	// symbol at all: residue is then confined to unexported plumbing, which
	// cannot be referenced outside the package by construction.
	NoExported bool
}

// LegacyFiles returns the files in dir that are structurally pure protocol
// residue: every exported top-level declaration's source range contains at
// least one legacy-token hit. This is a syntactic reading, not a call-graph
// proof - a file with zero exported declarations is included only because
// Go's own visibility rule already makes its symbols unreachable outside the
// package, not because this pass traced every call site.
func LegacyFiles(dir string) ([]File, error) {
	hits, err := Census(dir)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, nil
	}

	byFile := map[string][]Hit{}
	for _, h := range hits {
		byFile[h.File] = append(byFile[h.File], h)
	}
	var paths []string
	for p := range byFile {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var files []File
	fset := token.NewFileSet()
	for _, path := range paths {
		fileHits := byFile[path]
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		astFile, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			// Unparseable: still report the hits, but this pass cannot
			// judge whether the file is pure residue.
			continue
		}
		lines := map[int]bool{}
		for _, h := range fileHits {
			lines[h.Line] = true
		}
		exportedRanges, anyExported := topLevelExportedRanges(fset, astFile)
		if anyExported {
			if !allRangesHit(exportedRanges, lines) {
				continue
			}
		}
		files = append(files, File{Path: path, Hits: fileHits, NoExported: !anyExported})
	}
	return files, nil
}

type lineRange struct{ start, end int }

func topLevelExportedRanges(fset *token.FileSet, file *ast.File) ([]lineRange, bool) {
	var ranges []lineRange
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.IsExported() {
				ranges = append(ranges, spanOf(fset, d))
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						ranges = append(ranges, spanOf(fset, d))
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.IsExported() {
							ranges = append(ranges, spanOf(fset, d))
						}
					}
				}
			}
		}
	}
	return ranges, len(ranges) > 0
}

func spanOf(fset *token.FileSet, n ast.Node) lineRange {
	return lineRange{start: fset.Position(n.Pos()).Line, end: fset.Position(n.End()).Line}
}

func allRangesHit(ranges []lineRange, lines map[int]bool) bool {
	for _, r := range ranges {
		hit := false
		for line := r.start; line <= r.end; line++ {
			if lines[line] {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}
