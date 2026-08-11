package lower_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program/keyspace"
	programlower "github.com/wippyai/go-lua/program/lower"
)

func TestSourceRejectsEmptyCanonicalName(t *testing.T) {
	_, err := programlower.Lower(programlower.Source{Text: []byte("return 1")})
	if err == nil || !strings.Contains(err.Error(), "source: empty Name") {
		t.Fatalf("Lower(Source{empty Name}) = %v, want source-name rejection", err)
	}
}

func TestSourceTextMutationDoesNotChangeLoweredProgram(t *testing.T) {
	text := []byte("return 1")
	p, err := programlower.Lower(programlower.Source{Name: "logical/mutation.lua", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	text[len(text)-1] = '2'

	sourceView := p.Source()
	flowView := p.Flow().Authored()
	returned, ok := flowView.Control().Returns().At(0)
	if !ok {
		t.Fatal("Program has no authored Return")
	}
	body, _, _, bodyOK := sourceView.Index().Position(returned)
	if !bodyOK {
		t.Fatal("Return has no exact Source position")
	}
	first, ok := sourceView.Order().BodyAt(body, 0)
	if !ok || first != returned {
		t.Fatalf("Return source root = %v/%v, want %v", first, ok, returned)
	}
	_, values, ok := flowView.Control().Returns().Get(returned)
	if !ok {
		t.Fatal("source term is not Return")
	}
	integer, ok := flowView.Values().Member(values, 0)
	if !ok {
		t.Fatal("Return has no first value")
	}
	gotInteger, _, value, ok := sourceView.Literals().Integers().At(int(keyspace.TermOrdinal(integer) - 1))
	if !ok || gotInteger != integer || value != 1 {
		t.Fatalf("lowered integer = %d/%v, want immutable source value 1", value, ok)
	}
}

func TestSourceSpansReconstructOneLogicalName(t *testing.T) {
	const name = "logical/unit.lua"
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte("type Answer = number\nlocal value = 42")})
	if err != nil {
		t.Fatal(err)
	}
	sourceView := p.Source()
	alias, ok := p.Static().Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("Program has no TypeAlias")
	}
	if span, ok := sourceView.Identity().Span(alias); !ok || span.File != name {
		t.Fatalf("TypeAlias Span = %#v/%v, want File %q", span, ok, name)
	}
	_, _, _, coordinate, aliasOK := p.Static().Declarations().Aliases().Get(alias)
	span, ok := sourceView.Identity().Render(coordinate)
	if !aliasOK || !ok || span.File != name {
		t.Fatalf("TypeAliasNameSpan = %#v/%v, want File %q", span, ok, name)
	}
}

func TestSourceParseErrorRetainsExactCallerText(t *testing.T) {
	const source = "local value = @"
	_, err := programlower.Lower(programlower.Source{Name: "logical/invalid.lua", Text: []byte(source)})
	if err == nil {
		t.Fatal("Lower accepted invalid source")
	}
	var parseError *parse.Error
	if !errors.As(err, &parseError) {
		t.Fatalf("Lower error = %T, want wrapped *parse.Error", err)
	}
	if parseError.Source != source {
		t.Fatalf("parse error Source = %q, want exact caller text %q", parseError.Source, source)
	}
	if rendered := parseError.Render(); !strings.Contains(rendered, source) {
		t.Fatalf("rendered parse diagnostic missing source context: %q", rendered)
	}
}
