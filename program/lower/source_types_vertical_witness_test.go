package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/static"
)

// This vertical witness follows the final static and Flow owners only.  Static
// syntax is not mirrored through a root Program forwarding vocabulary.
func TestSourceTypesVerticalWitnesses(t *testing.T) {
	t.Run("compound type rows", func(t *testing.T) {
		p := parseBindLower(t, "type Item = { readonly name: string, optional count: number }[] | {[string]: number}")
		alias, ok := p.Static().Declarations().Aliases().At(0)
		if !ok {
			t.Fatal("missing Alias")
		}
		_, target, _, _, aliasOK := p.Static().Declarations().Aliases().Get(alias)
		if !aliasOK {
			t.Fatal("missing Alias target")
		}
		if count, ok := p.Static().Types().Unions().MemberCount(target); !ok || count != 2 {
			t.Fatalf("union members = %d/%v, want two", count, ok)
		}
		array, _ := p.Static().Types().Unions().MemberAt(target, 0)
		element, readonly, arrayOK := p.Static().Types().Arrays().Get(array)
		if !arrayOK || readonly || element == 0 {
			t.Fatalf("array member = element %v readonly %v ok %v", element, readonly, arrayOK)
		}
		recordReadonly, fields, recordOK := p.Static().Types().Records().Get(element)
		if !recordOK || !recordReadonly || fields != 2 {
			t.Fatalf("record member = readonly %v fields %d ok %v", recordReadonly, fields, recordOK)
		}
	})

	t.Run("operators and references", func(t *testing.T) {
		p := parseBindLower(t, "type A = number\ntype Result = keyof(A) | A[\"field\"] | (A extends A ? A : A)")
		operators := p.Static().Operators()
		if operators.KeyOfs().Count() != 1 || operators.IndexAccesses().Count() != 1 || operators.Conditionals().Count() != 1 {
			t.Fatalf("static operator counts = %d/%d/%d", operators.KeyOfs().Count(), operators.IndexAccesses().Count(), operators.Conditionals().Count())
		}
		for _, family := range []keyspace.Term{
			func() keyspace.Term { term, _ := operators.KeyOfs().At(0); return term }(),
			func() keyspace.Term { term, _ := operators.IndexAccesses().At(0); return term }(),
			func() keyspace.Term { term, _ := operators.Conditionals().At(0); return term }(),
		} {
			if span, ok := p.Source().Identity().Span(family); !ok || span.StartLine == 0 {
				t.Fatalf("static operator %v has no Source span", family)
			}
		}
	})

	t.Run("runtime type value and typed call", func(t *testing.T) {
		p := parseBindLower(t, "type User = number\nlocal function id<T>(value: T): T return value end\nreturn id::<User>(User)")
		typeValue, typeValueOK := p.Flow().Authored().TypeValues().At(0)
		call, callOK := p.Flow().Authored().Calls().At(0)
		if !typeValueOK || !callOK {
			t.Fatalf("TypeValue/Call = %v/%v %v/%v", typeValue, typeValueOK, call, callOK)
		}
		target, targetOK := p.Static().Operands().TypeValues().Target(typeValue)
		if !targetOK || target == 0 {
			t.Fatalf("TypeValue target = %v/%v", target, targetOK)
		}
		if count, ok := p.Static().Contracts().Calls().TypeArgumentCount(call); !ok || count != 1 {
			t.Fatalf("Call type arguments = %d/%v, want one", count, ok)
		}
		if _, _, _, _, ok := p.Flow().Authored().Calls().Get(call); !ok {
			t.Fatal("Call row is absent")
		}
	})

	t.Run("cast claim has static target", func(t *testing.T) {
		p := parseBindLower(t, "type User = number\nlocal value = 1 as User")
		claim, claimOK := p.Flow().Authored().Claims().At(0)
		if !claimOK {
			t.Fatal("missing ValueClaim")
		}
		_, _, claimKind, rowOK := p.Flow().Authored().Claims().Get(claim)
		if !rowOK || claimKind != kind.ValueClaimTypeAs {
			t.Fatalf("ValueClaim kind = %v/%v", claimKind, rowOK)
		}
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !targetOK || target == 0 {
			t.Fatalf("ValueClaim target = %v/%v", target, targetOK)
		}
		resolution, declaration, root, refOK := p.Static().References().Get(target)
		if !refOK || resolution != static.TypeRefDeclaration || declaration == 0 || root != 0 {
			t.Fatalf("cast target reference = %v/%v/%v/%v", resolution, declaration, root, refOK)
		}
	})
}
