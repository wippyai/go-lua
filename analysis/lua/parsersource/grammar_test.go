package parsersource

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDeclaredResultTagsRejectsAmbiguityAndKeepsNamesExact(t *testing.T) {
	write := func(source string) string {
		path := filepath.Join(t.TempDir(), "parser.go.y")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := write("%type<exprlist> varlist exprlist\n%type<stmts> chunk\n%%\n")
	tags, err := DeclaredResultTags(path)
	if err != nil || tags["varlist"] != "exprlist" || tags["exprlist"] != "exprlist" || tags["chunk"] != "stmts" {
		t.Fatalf("tags=%#v err=%v", tags, err)
	}
	if _, err := DeclaredResultTags(write("%type<expr> expr\n%type<expr> expr\n")); err == nil {
		t.Fatal("identical duplicate tag accepted")
	}
	if _, err := DeclaredResultTags(write("%type<expr> expr\n%type<stmt> expr\n")); err == nil {
		t.Fatal("conflicting duplicate tag accepted")
	}
	trailing, err := DeclaredResultTags(write("%type<expr> expr // trailing\n"))
	if err != nil || trailing["expr"] != "expr" || trailing["trailing"] != "" {
		t.Fatalf("trailing comment polluted tags: %#v err=%v", trailing, err)
	}
}

func TestGrammarExtractorHonorsQuotedTerminalsCommentsAndNestedActions(t *testing.T) {
	source := `
expr:
	expr '|' expr {
		text := "{ quoted | braces }"
		raw := ` + "`{\nphantom:\n| }\n`" + `
		// } | phantom:
		if text != "" { $$ = &ast.FooExpr{Flag: true} }
		_ = raw
	} |
	'{' '}' { $$ = &ast.TableExpr{} }
next:
	TOKEN { $$ = &ast.NextExpr{} }
`
	rows, err := parseGrammarAlternatives(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("productions = %d, want 3: %#v", len(rows), rows)
	}
	if got, want := rows[0].RHS, []string{"expr", "'|'", "expr"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("quoted terminal RHS = %#v, want %#v", got, want)
	}
	if rows[0].ActionSignature == rows[1].ActionSignature {
		t.Fatal("distinct parser alternatives have the same action signature")
	}
	if rows[2].Nonterminal != "next" || rows[2].Ordinal != 1 {
		t.Fatalf("next production = %#v", rows[2])
	}
}

func TestGrammarActionSignatureIgnoresLayoutButRetainsLiteralMeaning(t *testing.T) {
	rhs := []string{"TOKEN"}
	first := grammarActionSignature(rhs, `{
		// layout only
		$$ = &ast.ItemExpr{Name: "one two"}
	}`)
	second := grammarActionSignature(rhs, `{ $$=&ast.ItemExpr{Name:"one two"} }`)
	if first != second {
		t.Fatalf("layout-equivalent actions have different signatures: %s != %s", first, second)
	}
	changed := grammarActionSignature(rhs, `{ $$=&ast.ItemExpr{Name:"one  two"} }`)
	if first == changed {
		t.Fatal("action signature ignored a quoted literal change")
	}
}
