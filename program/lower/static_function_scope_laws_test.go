package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

// TestFunctionStaticScopesSealAtTheirDistinctHeaderPhases keeps the two
// Function-hosted static frontiers separate. A Function TypeParam is authored
// before that function's formals and retains the enclosing body; a return
// type and its annotation are authored after the formals and use the
// function's own body. Nesting makes both ownership identities observable.
func TestFunctionStaticScopesSealAtTheirDistinctHeaderPhases(t *testing.T) {
	p := parseBindLower(t, `
local outer = 1
local f = function<T: typeof(outer)>(value: T): typeof(value) | number @returns(value)
  local nested = function<U: typeof(value)>(inner: U): typeof(inner) | number @returns(inner)
    return inner
  end
  return value
end
return f
`)

	outerBind, ok := p.Flow().Authored().Storage().Binds().At(0)
	if !ok {
		t.Fatal("missing outer Bind")
	}
	outer := boundCell(t, p, outerBind, 0)
	outerFunction, ok := p.Flow().Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing outer Function")
	}
	nestedFunction, ok := p.Flow().Authored().Functions().At(1)
	if !ok {
		t.Fatal("missing nested Function")
	}

	assertFunctionStaticScope := func(function, preFormalsSource keyspace.Term) {
		t.Helper()
		formal, ok := p.Source().Formals().At(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing formal", function)
		}
		generic, ok := p.Static().Contracts().Functions().TypeParamAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing generic", function)
		}
		_, _, constraint, ok := p.Static().Declarations().TypeParams().Get(generic)
		if !ok {
			t.Fatalf("Function(%v) generic TypeParam absent", function)
		}
		constraintScope, constraintOperand, ok := p.Static().Operators().TypeOfs().Get(constraint)
		if !ok || constraintScope != generic {
			t.Fatalf(
				"Function(%v) generic TypeOf = scope %v operand %v ok %v; want TypeParam scope",
				function, constraintScope, constraintOperand, ok,
			)
		}
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(constraintOperand); !ok || source != preFormalsSource {
			t.Fatalf(
				"Function(%v) generic TypeOf source = %v/%v; want pre-formals source %v",
				function, source, ok, preFormalsSource,
			)
		}

		returnType, ok := p.Static().Contracts().Functions().ReturnAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing return type", function)
		}
		returnTypeOf, annotationTarget := keyspace.Term(0), keyspace.Term(0)
		for index := 0; index < 2; index++ {
			member, ok := p.Static().Types().Unions().MemberAt(returnType, index)
			if !ok {
				t.Fatalf("Function(%v) return type is not a two-member Union", function)
			}
			if _, _, ok := p.Static().Operators().TypeOfs().Get(member); ok {
				returnTypeOf = member
			}
			if count, ok := p.Static().Operands().Annotations().ForCount(member); ok && count == 1 {
				annotationTarget = member
			}
		}
		if returnTypeOf == 0 || annotationTarget == 0 {
			t.Fatalf("Function(%v) return type omitted typeof or annotation target", function)
		}
		returnScope, returnOperand, ok := p.Static().Operators().TypeOfs().Get(returnTypeOf)
		if !ok || returnScope != function {
			t.Fatalf(
				"Function(%v) return TypeOf = scope %v operand %v ok %v; want Function scope",
				function, returnScope, returnOperand, ok,
			)
		}
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(returnOperand); !ok || source != formal {
			t.Fatalf(
				"Function(%v) return TypeOf source = %v/%v; want formal %v",
				function, source, ok, formal,
			)
		}
		if !p.Flow().Containment().Static(returnTypeOf) || !p.Flow().Containment().Static(returnOperand) {
			t.Fatalf("Function(%v) return TypeOf did not retain static containment", function)
		}

		annotation, ok := p.Static().Operands().Annotations().ForAt(annotationTarget, 0)
		if !ok {
			t.Fatalf("Function(%v) missing return Annotation", function)
		}
		annotationRow, ok := p.Static().Operands().Annotations().Get(annotation)
		if !ok || annotationRow.Scope != function {
			t.Fatalf(
				"Function(%v) Annotation = scope %v values %v ok %v; want Function scope",
				function, annotationRow.Scope, annotationRow.Values, ok,
			)
		}
		annotationValue := valueAt(t, p, annotationRow.Values, 0)
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(annotationValue); !ok || source != formal {
			t.Fatalf(
				"Function(%v) Annotation source = %v/%v; want formal %v",
				function, source, ok, formal,
			)
		}
		if !p.Flow().Containment().Static(annotation) || !p.Flow().Containment().Static(annotationValue) {
			t.Fatalf("Function(%v) Annotation did not retain static containment", function)
		}
	}

	assertFunctionStaticScope(outerFunction, outer)
	outerFormal, ok := p.Source().Formals().At(outerFunction, 0)
	if !ok {
		t.Fatal("missing outer Function formal")
	}
	assertFunctionStaticScope(nestedFunction, outerFormal)
}
