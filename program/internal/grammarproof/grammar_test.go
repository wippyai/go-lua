package grammarproof

import (
	"bytes"
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTraceYYVALMutationFormsAreClassified(t *testing.T) {
	tests := []struct {
		name, expression, form, field string
	}{
		{name: "direct", expression: "yyVAL.stmt", form: "direct-field", field: "stmt"},
		{name: "indexed", expression: "yyVAL.stmts[0]", form: "indexed", field: "stmts"},
		{name: "nested", expression: "yyVAL.stmt.Node", form: "nested-selector", field: "stmt"},
		{name: "incdec", expression: "yyVAL.expr", form: "direct-field", field: "expr"},
		{name: "whole-union", expression: "yyVAL", form: "whole-union", field: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expression, err := goparser.ParseExpr(test.expression)
			if err != nil {
				t.Fatal(err)
			}
			field, form, touched := traceYYVALMutationField(expression)
			if !touched || field != test.field || form != test.form {
				t.Fatalf("mutation = %q/%q/%v, want %q/%q/true", field, form, touched, test.field, test.form)
			}
		})
	}
	method, err := goparser.ParseExpr("yyVAL.stmts[0].SetPosFromToken")
	if err != nil {
		t.Fatal(err)
	}
	receiver, ok := traceMethodReceiver(method)
	if !ok {
		t.Fatal("method receiver was not classified")
	}
	field, form, touched := traceYYVALMutationField(receiver)
	if !touched || field != "stmts" || form != "indexed" {
		t.Fatalf("method mutation = %q/%q/%v, want stmts/indexed/true", field, form, touched)
	}
}

func TestTraceYYVALAliasFormsAreClassifiedOrRejected(t *testing.T) {
	tests := []struct {
		name, expression, form, field string
	}{
		{name: "address", expression: "&yyVAL.stmt", form: "direct-field", field: "stmt"},
		{name: "nested-address", expression: "&yyVAL.stmts[0]", form: "indexed", field: "stmts"},
		{name: "call-argument", expression: "consume(yyVAL.stmt.Node)", form: "nested-selector", field: "stmt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expression, err := goparser.ParseExpr(test.expression)
			if err != nil {
				t.Fatal(err)
			}
			var argument goast.Expr
			if call, ok := expression.(*goast.CallExpr); ok {
				argument = call.Args[0]
			} else if unary, ok := expression.(*goast.UnaryExpr); ok {
				argument = unary.X
			} else {
				t.Fatal("test expression has no alias operand")
			}
			field, form, touched := traceYYVALReference(argument)
			if !touched || field != test.field || form != test.form {
				t.Fatalf("alias = %q/%q/%v, want %q/%q/true", field, form, touched, test.field, test.form)
			}
		})
	}

	file, err := goparser.ParseFile(token.NewFileSet(), "parser.go", `package parse
func probe() {
	switch yynt {
	case 1:
		yyVAL.stmts = append(yyVAL.stmts, yyVAL.stmt)
	}
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	target, err := uniqueYYNTSwitch(file)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := traceSemanticTargets(target)
	if err != nil || !reflect.DeepEqual(fields[1], []string{"stmt", "stmts"}) {
		t.Fatalf("specific aliases were not observed: fields=%#v err=%v", fields, err)
	}

	for name, action := range map[string]string{
		"whole-union-call":      "sink(yyVAL)",
		"indeterminate-call":    "sink(wrapper(yyVAL.stmt))",
		"indeterminate-address": "sink(&(wrapper(yyVAL.stmt)))",
	} {
		t.Run(name, func(t *testing.T) {
			file, err := goparser.ParseFile(token.NewFileSet(), "parser.go", "package parse; func probe() { switch yynt { case 1: "+action+" } }", 0)
			if err != nil {
				t.Fatal(err)
			}
			target, err := uniqueYYNTSwitch(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := traceSemanticTargets(target); err == nil || (!strings.Contains(err.Error(), "unsupported") && !strings.Contains(err.Error(), "indeterminate")) {
				t.Fatalf("unsafe alias accepted: %v", err)
			}
		})
	}
}

func TestTraceYYNTSwitchCountFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{name: "one", body: "switch yynt { case 1: }", want: 1},
		{name: "three", body: "switch yynt { case 1: }; switch yynt { case 2: }; switch yynt { case 3: }", want: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := goparser.ParseFile(token.NewFileSet(), "parser.go", "package parse; func probe() {"+test.body+"}", 0)
			if err != nil {
				t.Fatal(err)
			}
			target, err := uniqueYYNTSwitch(file)
			if test.want == 1 {
				if err != nil || target == nil {
					t.Fatalf("unique switch = %v, want success", err)
				}
				return
			}
			if err == nil || target != nil || !strings.Contains(err.Error(), "3 semantic switches") {
				t.Fatalf("three switches accepted: target=%v err=%v", target != nil, err)
			}
		})
	}
}

func TestTraceManifestPinsResolvedToolsAndGoSelection(t *testing.T) {
	root := moduleRoot(t)
	sources, err := corpus(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := traceInputManifest(root, sources)
	if err != nil {
		t.Fatal(err)
	}
	for name, executable := range map[string]traceExecutable{"go": manifest.goExecutable, "goyacc": manifest.goyacc} {
		if !filepath.IsAbs(executable.lookupPath) || !filepath.IsAbs(executable.resolvedPath) {
			t.Fatalf("%s paths are not absolute: %#v", name, executable)
		}
		contents, err := os.ReadFile(executable.resolvedPath)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(contents, executable.contents) {
			t.Fatalf("%s executable bytes changed after resolution", name)
		}
	}
	for _, key := range []string{"GOWORK", "GOTOOLCHAIN", "GOENV"} {
		if got := traceEnvironmentValue(manifest.commandEnv, key); got == "" {
			t.Fatalf("forced %s selection missing", key)
		}
	}
	if got := traceEnvironmentValue(manifest.commandEnv, "GOWORK"); got != "off" {
		t.Fatalf("GOWORK=%q, want off", got)
	}
	if got := traceEnvironmentValue(manifest.commandEnv, "GOTOOLCHAIN"); got != "local" {
		t.Fatalf("GOTOOLCHAIN=%q, want local", got)
	}
	if got := traceEnvironmentValue(manifest.commandEnv, "GOENV"); got != "off" {
		t.Fatalf("GOENV=%q, want off", got)
	}
	entries := make(map[string][]byte, len(manifest.entries))
	for _, entry := range manifest.entries {
		entries[entry.name] = entry.contents
	}
	for name, executable := range map[string]traceExecutable{"toolchain:go": manifest.goExecutable, "toolchain:goyacc": manifest.goyacc} {
		if !bytes.Contains(entries[name], []byte(filepath.ToSlash(executable.resolvedPath))) {
			t.Fatalf("manifest %s does not commit resolved path", name)
		}
		if !bytes.HasSuffix(entries[name], executable.contents) {
			t.Fatalf("manifest %s does not commit exact executable bytes", name)
		}
	}
}

func TestDeclaredResultTagsRejectsAmbiguityAndKeepsNamesExact(t *testing.T) {
	write := func(source string) string {
		path := filepath.Join(t.TempDir(), "parser.go.y")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := write("%type<exprlist> varlist exprlist\n%type<stmts> chunk\n%%\n")
	tags, err := declaredResultTags(path)
	if err != nil || tags["varlist"] != "exprlist" || tags["exprlist"] != "exprlist" || tags["chunk"] != "stmts" {
		t.Fatalf("tags=%#v err=%v", tags, err)
	}
	if _, err := declaredResultTags(write("%type<expr> expr\n%type<expr> expr\n")); err == nil {
		t.Fatal("identical duplicate tag accepted")
	}
	if _, err := declaredResultTags(write("%type<expr> expr\n%type<stmt> expr\n")); err == nil {
		t.Fatal("conflicting duplicate tag accepted")
	}
	trailing, err := declaredResultTags(write("%type<expr> expr // trailing\n"))
	if err != nil || trailing["expr"] != "expr" || trailing["trailing"] != "" {
		t.Fatalf("trailing comment polluted tags: %#v err=%v", trailing, err)
	}
}

func TestSequenceCarriersDeriveUnionRootsAndPrivateWrapperFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "compiler", "parse")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `%{
package parse
type wrapper struct { Values []int; Flag bool }
%}
%type<roots> root
%type<wrapped> wrapped
%type<scalar> scalar
%union {
 roots []int
 wrapped wrapper
 scalar int
}
%%
`
	if err := os.WriteFile(filepath.Join(path, "parser.go.y"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := SequenceCarriers(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []SequenceCarrier{{Tag: "roots"}, {Tag: "wrapped", Field: "Values"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("carriers=%#v want %#v", rows, want)
	}
	broken := strings.Replace(source, "scalar int", "", 1)
	if err := os.WriteFile(filepath.Join(path, "parser.go.y"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SequenceCarriers(root); err == nil {
		t.Fatal("missing union disposition accepted")
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
	if got, want := rows[0].rhs, []string{"expr", "'|'", "expr"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("quoted terminal RHS = %#v, want %#v", got, want)
	}
	if rows[0].actionSignature == rows[1].actionSignature {
		t.Fatal("distinct parser alternatives have the same action signature")
	}
	if rows[2].nonterminal != "next" || rows[2].ordinal != 1 {
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
