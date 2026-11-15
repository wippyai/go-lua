package parse

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func scanStringToken(input string) (ast.Token, error) {
	lexer := &Lexer{NewScanner(strings.NewReader(input), "test"), nil, false, ast.Token{}, TNil, nil}
	return lexer.scanner.Scan(lexer)
}

// Expected bytes come from the Lua 5.4 reference interpreter.
func TestScanLua54Escapes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"hex", `"\x41\x4a\x4A\xff"`, "AJJ\xff"},
		{"hex zero", `"\x00"`, "\x00"},
		{"utf8 ascii", `"\u{41}"`, "A"},
		{"utf8 two bytes", `"\u{E4}"`, "\xc3\xa4"},
		{"utf8 three bytes", `"\u{20AC}"`, "\xe2\x82\xac"},
		{"utf8 four bytes", `"\u{1F600}"`, "\xf0\x9f\x98\x80"},
		{"utf8 zero", `"\u{0}"`, "\x00"},
		{"utf8 surrogate", `"\u{D800}"`, "\xed\xa0\x80"},
		{"utf8 past unicode", `"\u{110000}"`, "\xf4\x90\x80\x80"},
		{"utf8 max", `"\u{7FFFFFFF}"`, "\xfd\xbf\xbf\xbf\xbf\xbf"},
		{"utf8 leading zeros", `"\u{000000041}"`, "A"},
		{"skip whitespace", "\"a\\z   \n\t  b\"", "ab"},
		{"skip crlf", "\"a\\z\r\n  b\"", "ab"},
		{"skip nothing", `"a\zb"`, "ab"},
		{"skip at end", `"a\z"`, "a"},
		{"escaped newline", "\"a\\\nb\"", "a\nb"},
		{"escaped crlf", "\"a\\\r\nb\"", "a\nb"},
		{"decimal", `"\65\066\0491"`, "AB11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := scanStringToken(tt.input)
			if err != nil {
				t.Fatalf("Scan(%q) error: %v", tt.input, err)
			}
			if tok.Type != TString {
				t.Fatalf("Scan(%q) type = %d, want TString", tt.input, tok.Type)
			}
			if tok.Str != tt.expected {
				t.Errorf("Scan(%q) = %q, want %q", tt.input, tok.Str, tt.expected)
			}
		})
	}
}

func TestScanSkipEscapeCountsLines(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader("\"a\\z\n\n  b\" x"), "test"), nil, false, ast.Token{}, TNil, nil}
	if _, err := lexer.scanner.Scan(lexer); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if tok.Pos.Line != 3 {
		t.Errorf("token after skipped lines on line %d, want 3", tok.Pos.Line)
	}
}

// Messages follow llex.c.
func TestScanInvalidEscapes(t *testing.T) {
	tests := []struct {
		input   string
		message string
	}{
		{`"\q"`, "invalid escape sequence"},
		{`"\."`, "invalid escape sequence"},
		{`"\%"`, "invalid escape sequence"},
		{`"\d"`, "invalid escape sequence"},
		{`"\x4"`, "hexadecimal digit expected"},
		{`"\xg1"`, "hexadecimal digit expected"},
		{`"\x`, "hexadecimal digit expected"},
		{`"\u41"`, "missing '{'"},
		{`"\u{}"`, "hexadecimal digit expected"},
		{`"\u{41"`, "missing '}'"},
		{`"\u{80000000}"`, "UTF-8 value too large"},
		{`"\u{100000000}"`, "UTF-8 value too large"},
		{`"\256"`, "decimal escape too large"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := scanStringToken(tt.input)
			if err == nil {
				t.Fatalf("Scan(%q) succeeded, want error %q", tt.input, tt.message)
			}
			perr, ok := err.(*Error)
			if !ok {
				t.Fatalf("Scan(%q) error type %T, want *Error", tt.input, err)
			}
			if perr.Message != tt.message {
				t.Errorf("Scan(%q) message = %q, want %q", tt.input, perr.Message, tt.message)
			}
		})
	}
}
