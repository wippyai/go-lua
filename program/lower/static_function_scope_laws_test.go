package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
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

	outerBind, ok := p.BindAt(0)
	if !ok {
		t.Fatal("missing outer Bind")
	}
	outer := boundCell(t, p, outerBind, 0)
	outerFunction, ok := p.FunctionAt(0)
	if !ok {
		t.Fatal("missing outer Function")
	}
	nestedFunction, ok := p.FunctionAt(1)
	if !ok {
		t.Fatal("missing nested Function")
	}

	assertFunctionStaticScope := func(function, preFormalsSource program.Term) {
		t.Helper()
		formal, ok := p.FormalAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing formal", function)
		}
		generic, ok := p.FunctionTypeParamAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing generic", function)
		}
		_, _, constraint, ok := p.TypeParam(generic)
		if !ok {
			t.Fatalf("Function(%v) generic TypeParam absent", function)
		}
		constraintScope, constraintOperand, ok := p.TypeOf(constraint)
		if !ok || constraintScope != generic {
			t.Fatalf(
				"Function(%v) generic TypeOf = scope %v operand %v ok %v; want TypeParam scope",
				function, constraintScope, constraintOperand, ok,
			)
		}
		if _, source, ok := p.Read(constraintOperand); !ok || source != preFormalsSource {
			t.Fatalf(
				"Function(%v) generic TypeOf source = %v/%v; want pre-formals source %v",
				function, source, ok, preFormalsSource,
			)
		}

		returnType, ok := p.FunctionReturnAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing return type", function)
		}
		returnTypeOf, annotationTarget := program.Term(0), program.Term(0)
		for index := 0; index < 2; index++ {
			member, ok := p.UnionMember(returnType, index)
			if !ok {
				t.Fatalf("Function(%v) return type is not a two-member Union", function)
			}
			if _, _, ok := p.TypeOf(member); ok {
				returnTypeOf = member
			}
			if count, ok := p.TargetAnnotationCount(member); ok && count == 1 {
				annotationTarget = member
			}
		}
		if returnTypeOf == 0 || annotationTarget == 0 {
			t.Fatalf("Function(%v) return type omitted typeof or annotation target", function)
		}
		returnScope, returnOperand, ok := p.TypeOf(returnTypeOf)
		if !ok || returnScope != function {
			t.Fatalf(
				"Function(%v) return TypeOf = scope %v operand %v ok %v; want Function scope",
				function, returnScope, returnOperand, ok,
			)
		}
		if _, source, ok := p.Read(returnOperand); !ok || source != formal {
			t.Fatalf(
				"Function(%v) return TypeOf source = %v/%v; want formal %v",
				function, source, ok, formal,
			)
		}
		if !p.Static(returnTypeOf) || !p.Static(returnOperand) {
			t.Fatalf("Function(%v) return TypeOf did not retain static containment", function)
		}

		annotation, ok := p.TargetAnnotationAt(annotationTarget, 0)
		if !ok {
			t.Fatalf("Function(%v) missing return Annotation", function)
		}
		annotationScope, _, _, values, ok := p.Annotation(annotation)
		if !ok || annotationScope != function {
			t.Fatalf(
				"Function(%v) Annotation = scope %v values %v ok %v; want Function scope",
				function, annotationScope, values, ok,
			)
		}
		annotationValue := valueAt(t, p, values, 0)
		if _, source, ok := p.Read(annotationValue); !ok || source != formal {
			t.Fatalf(
				"Function(%v) Annotation source = %v/%v; want formal %v",
				function, source, ok, formal,
			)
		}
		if !p.Static(annotation) || !p.Static(annotationValue) {
			t.Fatalf("Function(%v) Annotation did not retain static containment", function)
		}
	}

	assertFunctionStaticScope(outerFunction, outer)
	outerFormal, ok := p.FormalAt(outerFunction, 0)
	if !ok {
		t.Fatal("missing outer Function formal")
	}
	assertFunctionStaticScope(nestedFunction, outerFormal)
}
