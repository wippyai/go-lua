package parse

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func scanOne(t *testing.T, src string) (ast.Token, error) {
	t.Helper()
	lexer := &Lexer{NewScanner(strings.NewReader(src), "test"), nil, false, ast.Token{}, TNil, nil}
	return lexer.scanner.Scan(lexer)
}

func TestScanNumerals(t *testing.T) {
	for _, src := range []string{
		"10", "010", "3.", ".5", "1e5", "1E+5", "2.5e-3",
		"0x10", "0XfF", "0xffffffffffffffffff", "0x1p4", "0x1P-1", "0xA.8p0", "0x.8", "0x8.",
		"9223372036854775808",
	} {
		tok, err := scanOne(t, src)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", src, err)
		}
		if tok.Type != TNumber || tok.Str != src {
			t.Fatalf("%s: got token %d %q", src, tok.Type, tok.Str)
		}
	}
}

func TestScanNumeralStopsAtOperator(t *testing.T) {
	tok, err := scanOne(t, "0x1p4+1")
	if err != nil {
		t.Fatal(err)
	}
	if tok.Str != "0x1p4" {
		t.Fatalf("got %q, want 0x1p4", tok.Str)
	}
	tok, err = scanOne(t, "1e-2-3")
	if err != nil {
		t.Fatal(err)
	}
	if tok.Str != "1e-2" {
		t.Fatalf("got %q, want 1e-2", tok.Str)
	}
}

func TestScanMalformedNumerals(t *testing.T) {
	for _, src := range []string{"3x", "0x", "0xg", "1e", "1e+", "1.2.3", "3..2", "0x1p", "1p4", "12abc", "1_000"} {
		if _, err := scanOne(t, src); err == nil || !strings.Contains(err.Error(), "malformed number") {
			t.Fatalf("%s: expected malformed number error, got %v", src, err)
		}
	}
}
