package signature

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
)

const expectedEscapeVocabVersion1Hash = "2074fbf780df680d4b2fdf63c34dafd97570684152743d64379ede084cca43ed"

func TestEscapeVocabVersionPinsCurrentSurface(t *testing.T) {
	got := hashSchemaSurface(escapeVocabularySurface(t))
	want := map[int]string{
		1: expectedEscapeVocabVersion1Hash,
	}[EscapeVocabVersion]
	if want == "" {
		t.Fatalf("no expected escape vocabulary hash for version %d: bump version constant + journal a D-entry", EscapeVocabVersion)
	}
	if got != want {
		t.Fatalf("escape vocabulary surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			EscapeVocabVersion, want, got, strings.Join(escapeVocabularySurface(t), "\n"))
	}
}

func escapeVocabularySurface(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, name := range escapeKindConstNames(t) {
		out = append(out, "escape-kind:"+strings.TrimPrefix(name, "Escape"))
	}
	for _, descriptor := range capability.All() {
		if descriptor.Family != "ownership" {
			continue
		}
		out = append(out, fmt.Sprintf("ownership:%s|symbol:%s|status:%s", descriptor.ID, descriptor.Symbol, descriptor.Status))
	}
	sort.Strings(out)
	return out
}

func escapeKindConstNames(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "operational_effects.go", nil, 0)
	if err != nil {
		t.Fatalf("parse operational_effects.go: %v", err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		inEscapeKindBlock := false
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if ident, ok := value.Type.(*ast.Ident); ok && ident.Name == "EscapeKind" {
				inEscapeKindBlock = true
			}
			if !inEscapeKindBlock {
				continue
			}
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("no EscapeKind constants found")
	}
	return names
}

func hashSchemaSurface(lines []string) string {
	h := sha256.New()
	for _, line := range lines {
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
