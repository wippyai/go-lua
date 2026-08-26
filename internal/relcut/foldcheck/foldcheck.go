// Package foldcheck proves the cut folded functionality instead of renaming
// it. A deleted protocol file's functions must have no near-copy in the
// surviving tree: the semantics live on as declarations and owner judgments,
// never as the same body under a new home.
package foldcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Finding names one transplanted body: a function from a deleted file whose
// normalized body reappears in a surviving file.
type Finding struct {
	DeletedFile  string
	Function     string
	SurvivorFile string
	Survivor     string
	Similarity   float64
}

// Threshold above which two normalized bodies count as one transplanted body.
// A restatement rewrites the statement; a rename preserves it.
const Threshold = 0.90

// minBodyTokens keeps trivial accessors and one-liners out of the comparison;
// a three-token body is idiomatic, not evidence.
const minBodyTokens = 30

// Check reads every deleted file at preCutRev, extracts its function bodies,
// and scans the surviving working tree for near-copies. The survivor scan
// skips tests and the deleted paths themselves.
func Check(repoDir, preCutRev string, deletedPaths []string) ([]Finding, error) {
	deleted := make(map[string]struct{}, len(deletedPaths))
	for _, p := range deletedPaths {
		deleted[p] = struct{}{}
	}
	type body struct {
		file, name string
		tokens     []string
	}
	var cut []body
	for _, p := range deletedPaths {
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			continue
		}
		out, err := exec.Command("git", "-C", repoDir, "show", preCutRev+":"+p).Output()
		if err != nil {
			continue
		}
		for name, toks := range functionBodies(p, out) {
			if len(toks) >= minBodyTokens {
				cut = append(cut, body{file: p, name: name, tokens: toks})
			}
		}
	}
	var findings []Finding
	err := filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(repoDir, path)
		if relErr != nil {
			return nil
		}
		if _, gone := deleted[rel]; gone {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for name, toks := range functionBodies(rel, src) {
			if len(toks) < minBodyTokens {
				continue
			}
			for _, c := range cut {
				if s := similarity(c.tokens, toks); s >= Threshold {
					findings = append(findings, Finding{
						DeletedFile: c.file, Function: c.name,
						SurvivorFile: rel, Survivor: name, Similarity: s,
					})
				}
			}
		}
		return nil
	})
	return findings, err
}

// functionBodies parses one file and returns each function's normalized body
// token stream. Identifiers are kept verbatim: a rename that also renames
// every local would drift under similarity, and a faithful transplant will not.
func functionBodies(name string, src []byte) map[string][]string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	result := make(map[string][]string)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var toks []string
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.Ident:
				toks = append(toks, v.Name)
			case *ast.BasicLit:
				toks = append(toks, v.Value)
			}
			return true
		})
		result[fn.Name.Name] = toks
	}
	return result
}

// similarity is the Jaccard index over token bigrams: order-sensitive enough
// to separate a rewrite from a transplant, cheap enough for a full-tree scan.
func similarity(a, b []string) float64 {
	if len(a) < 2 || len(b) < 2 {
		return 0
	}
	grams := func(t []string) map[string]struct{} {
		m := make(map[string]struct{}, len(t))
		for i := 0; i+1 < len(t); i++ {
			m[t[i]+"\x00"+t[i+1]] = struct{}{}
		}
		return m
	}
	ga, gb := grams(a), grams(b)
	inter := 0
	for g := range ga {
		if _, ok := gb[g]; ok {
			inter++
		}
	}
	union := len(ga) + len(gb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
