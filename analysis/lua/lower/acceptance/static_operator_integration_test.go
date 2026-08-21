package acceptance_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSourceStaticOperatorsKeepExactStaticRows(t *testing.T) {
	p := parseBindLower(t, "type Check = number\ntype Constraint = string\ntype Then = boolean\ntype Otherwise = never\ntype Record = {}\ntype Key = \"field\"\ntype Keys = keyof(Record)\ntype Field = Record[Key]\ntype Choice = Check extends Constraint ? Then : Otherwise")
	aliases := p.Static().Declarations().Aliases()
	if aliases.Count() != 9 {
		t.Fatalf("Static Alias count = %d, want 9", aliases.Count())
	}
	keyAlias, _ := aliases.At(6)
	indexAlias, _ := aliases.At(7)
	conditionalAlias, _ := aliases.At(8)
	_, keyOf, _, _, keyAliasOK := aliases.Get(keyAlias)
	_, indexAccess, _, _, indexAliasOK := aliases.Get(indexAlias)
	_, conditional, _, _, conditionalAliasOK := aliases.Get(conditionalAlias)
	if !keyAliasOK || !indexAliasOK || !conditionalAliasOK {
		t.Fatal("missing Static Alias targets")
	}
	operators := p.Static().Operators()
	if operators.KeyOfs().Count() != 1 || operators.IndexAccesses().Count() != 1 || operators.Conditionals().Count() != 1 {
		t.Fatalf("operator counts = keyof %d indexed %d conditional %d", operators.KeyOfs().Count(), operators.IndexAccesses().Count(), operators.Conditionals().Count())
	}
	inner, keyOK := operators.KeyOfs().Get(keyOf)
	object, index, indexedOK := operators.IndexAccesses().Get(indexAccess)
	check, extends, then, otherwise, conditionalOK := operators.Conditionals().Get(conditional)
	if !keyOK || !indexedOK || !conditionalOK || inner == 0 || object == 0 || index == 0 || check == 0 || extends == 0 || then == 0 || otherwise == 0 {
		t.Fatalf("static operator rows key=%v indexed=%v conditional=%v", keyOK, indexedOK, conditionalOK)
	}
	for _, term := range []keyspace.Term{keyOf, inner, indexAccess, object, index, conditional, check, extends, then, otherwise} {
		if span, ok := p.Source().Identity().Span(term); !ok || span.StartLine == 0 {
			t.Fatalf("static term %v has no Source span", term)
		}
	}
}

func TestSourceStaticOperatorsReachDeclarationParameterAndCallHosts(t *testing.T) {
	for _, sample := range []struct {
		name  string
		input string
		want  func() int
	}{
		{"keyof", "type Subject = keyof(User)\nlocal function f<T: keyof(User)>(value: keyof(User)): keyof(User) return value end\nreturn f::<keyof(User)>(nil)", func() int { return 5 }},
		{"indexed", "type Subject = User[\"field\"]\nlocal function f<T: User[\"field\"]>(value: User[\"field\"]): User[\"field\"] return value end\nreturn f::<User[\"field\"]>(nil)", func() int { return 5 }},
		{"conditional", "type Subject = T extends U ? Then : Else\nlocal function f<T: T extends U ? Then : Else>(value: T extends U ? Then : Else): T extends U ? Then : Else return value end\nreturn f::<T extends U ? Then : Else>(nil)", func() int { return 5 }},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			operators := p.Static().Operators()
			var count int
			switch sample.name {
			case "keyof":
				count = operators.KeyOfs().Count()
			case "indexed":
				count = operators.IndexAccesses().Count()
			case "conditional":
				count = operators.Conditionals().Count()
			}
			if count != sample.want() {
				t.Fatalf("%s static operator count = %d, want %d", sample.name, count, sample.want())
			}
			call, ok := p.Flow().Authored().Calls().At(0)
			if !ok {
				t.Fatal("missing authored generic Call")
			}
			if count, ok := p.Static().Contracts().Calls().TypeArgumentCount(call); !ok || count != 1 {
				t.Fatalf("Call type arguments = %d/%v, want one", count, ok)
			}
		})
	}
}

func TestSourceStaticAnnotationRowsAreOwnedByStaticOperands(t *testing.T) {
	p := parseBindLower(t, "type Item = number\nlocal value: Item @note(1) = 0")
	annotations := p.Static().Operands().Annotations()
	annotation, ok := annotations.At(0)
	if !ok || annotations.Count() != 1 {
		t.Fatalf("Static Annotation = %v/%v count=%d", annotation, ok, annotations.Count())
	}
	row, rowOK := annotations.Get(annotation)
	if !rowOK || row.Target == 0 || row.Values == 0 {
		t.Fatalf("Static Annotation row = %#v/%v", row, rowOK)
	}
}

func TestSourceStaticOperatorContentIDIsDeterministic(t *testing.T) {
	input := "type Result = keyof(User) | User[\"field\"] | (T extends U ? Then : Else)"
	first := parseBindLower(t, input)
	second := parseBindLower(t, input)
	if first.ContentID() != second.ContentID() {
		t.Fatal("same static operators produced different Program ContentID")
	}
}

// These laws enter through authored source, then read token provenance from
// Source and authored static structure from Static.  They intentionally do
// not recreate the retired root forwarding surface.
