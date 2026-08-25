package memberroster_test

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"

	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule/codegen"
	"github.com/wippyai/go-lua/domain/memberroster"
)

// TestEveryAuthoredDerivationHasTheDerivedCallShape is fence one of the
// authored-Build ruling, and the first thing that holds a derivation to it.
//
// A Build is admitted as authored domain logic BEHIND A SEALED CONTRACT. The
// ledger says the authoring has a deadline and the seam says the relation is
// materialized once; neither says what the call looks like, and a contract
// nobody derives is not a contract. So the shape is derived from the
// declaration alone: Build answers the derivation's State from the schemas of
// its ordered static axes followed by the relation's declared inputs, and
// Count and At consume that State to expose the relation's Subject rows.
//
// Nothing else is a parameter. Which candidate a row hangs off, which
// projection addresses it, and which coordinates it resolves to are the sealed
// knowledge of the family that calls this, exactly as they are for a reducer -
// which is what keeps a derivation from growing the plumbing a reducer cannot.
func TestEveryAuthoredDerivationHasTheDerivedCallShape(t *testing.T) {
	root := moduleRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	var drift []string
	authored := 0
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("member definition source %q does not compose", source.Name)
		}
		for _, relation := range composed.Relations {
			if !relation.Derivation.Build.Available() {
				continue
			}
			authored++
			shape, derivedOK := roster.DerivationSignature(composed.Axis, relation, codegen.DerivationCellType, codegen.ReducerVectorType)
			if !derivedOK {
				drift = append(drift, fmt.Sprintf("%s: declared rows derive no call shape", relation.Key))
				continue
			}
			for _, call := range []struct {
				role    string
				symbol  memberdefinition.GoSymbol
				params  []memberdefinition.DerivedParam
				results []memberdefinition.DerivedParam
			}{
				{role: "Build", symbol: relation.Derivation.Build, params: shape.BuildParams, results: shape.BuildResults},
				{role: "Count", symbol: relation.Derivation.Count, params: shape.CountParams, results: shape.CountResults},
				{role: "At", symbol: relation.Derivation.At, params: shape.AtParams, results: shape.AtResults},
			} {
				decl, file, declOK := findFunction(t, root, call.symbol)
				if !declOK {
					drift = append(drift, fmt.Sprintf("%s %s: %s is not declared", relation.Key, call.role, call.symbol.Name))
					continue
				}
				if problem := compareDerivationCall(file, call.symbol.PackagePath, decl, call.params, call.results); problem != "" {
					drift = append(drift, fmt.Sprintf("%s %s: %s", relation.Key, call.role, problem))
				}
			}
		}
	}
	if authored == 0 {
		t.Fatal("no authored derivation is declared, so this law measures nothing")
	}
	if len(drift) != 0 {
		t.Fatalf("authored derivations whose call is not the one their declaration derives:\n\t%s", strings.Join(drift, "\n\t"))
	}
}

// compareDerivationCall reports the first disagreement between a derived call
// shape and the implementation's actual one, or the empty string.
func compareDerivationCall(file *ast.File, declaring string, decl *ast.FuncDecl, params, results []memberdefinition.DerivedParam) string {
	actualParams := flattenFields(decl.Type.Params)
	if len(actualParams) != len(params) {
		return fmt.Sprintf("takes %d parameters, the declaration derives %d (%s)", len(actualParams), len(params), describeDerivedTypes(params))
	}
	for position, want := range params {
		if problem := compareDerivedType(file, declaring, actualParams[position], want, "parameter", position); problem != "" {
			return problem
		}
	}
	actualResults := flattenFields(decl.Type.Results)
	if len(actualResults) != len(results) {
		return fmt.Sprintf("returns %d results, the declaration derives %d (%s)", len(actualResults), len(results), describeDerivedTypes(results))
	}
	for position, want := range results {
		if problem := compareDerivedType(file, declaring, actualResults[position], want, "result", position); problem != "" {
			return problem
		}
	}
	return ""
}

func compareDerivedType(file *ast.File, declaring string, expr ast.Expr, want memberdefinition.DerivedParam, role string, position int) string {
	if want.Slice {
		slice, isSlice := expr.(*ast.ArrayType)
		if !isSlice || slice.Len != nil {
			return fmt.Sprintf("%s %d is not a slice, the declaration derives %s", role, position, describeDerivedParam(want))
		}
		expr = slice.Elt
	}
	// A many-valued position is an execution view instantiated at the input's
	// own carrier, so the element it delivers is checked against the carrier
	// the declaration named rather than against the view alone.
	if want.Element.Available() {
		elementPath, elementSpelling, elementOK := typeArgument(file, declaring, expr)
		if !elementOK || elementPath != want.Element.PackagePath || elementSpelling != want.Element.Name {
			return fmt.Sprintf("%s %d delivers %s, the declaration derives %s", role, position, describeType(elementPath, elementSpelling), describeType(want.Element.PackagePath, want.Element.Name))
		}
	}
	_, pointer := expr.(*ast.StarExpr)
	path, name := resolvedType(file, declaring, expr)
	if path == want.Type.PackagePath && name == want.Type.Name && pointer == want.Type.Pointer {
		return ""
	}
	return fmt.Sprintf("%s %d is %s, the declaration derives %s", role, position, describeDerivedType(path, name, pointer), describeDerivedParam(want))
}

func describeDerivedParam(want memberdefinition.DerivedParam) string {
	spelled := describeDerivedType(want.Type.PackagePath, want.Type.Name, want.Type.Pointer)
	if want.Element.Available() {
		spelled += "[" + describeType(want.Element.PackagePath, want.Element.Name) + "]"
	}
	if want.Slice {
		return "[]" + spelled
	}
	return spelled
}

func describeDerivedType(path, name string, pointer bool) string {
	spelled := describeType(path, name)
	if pointer {
		return "*" + spelled
	}
	return spelled
}

func describeDerivedTypes(params []memberdefinition.DerivedParam) string {
	spelled := make([]string, 0, len(params))
	for _, param := range params {
		spelled = append(spelled, describeDerivedParam(param))
	}
	return strings.Join(spelled, ", ")
}

// TestEveryDeclaredFoldStateHasTheDerivedCallShape holds a reducer's sealed
// state to the same contract its relations' derivations are held to.
//
// A fold whose judgment rests on its axes' cold schemas cannot take them as
// parameters - that is what keeps a call shape from growing plumbing - so it
// declares the state they are sealed into, and the installed family seals it
// once from exactly the axes the declaration names. The Build that seals it is
// therefore derived from the declaration alone: the schemas of its ordered
// static axes in, the state and its validity out.
//
// The law measures nothing until a fold declares a state, so it says so rather
// than passing vacuously.
func TestEveryDeclaredFoldStateHasTheDerivedCallShape(t *testing.T) {
	root := moduleRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	var drift []string
	declared := 0
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("member definition source %q does not compose", source.Name)
		}
		for _, reducer := range composed.Reducers {
			if !reducer.Derivation.Declared() {
				continue
			}
			declared++
			params, results, derivedOK := roster.ReducerDerivationSignature(reducer.Derivation)
			if !derivedOK {
				drift = append(drift, fmt.Sprintf("%s (rule %s): declared state derives no call shape", reducer.Key, reducer.Rule))
				continue
			}
			decl, file, declOK := findFunction(t, root, reducer.Derivation.Build)
			if !declOK {
				drift = append(drift, fmt.Sprintf("%s (rule %s): state Build %s is not declared", reducer.Key, reducer.Rule, reducer.Derivation.Build.Name))
				continue
			}
			if problem := compareDerivationCall(file, reducer.Derivation.Build.PackagePath, decl, params, results); problem != "" {
				drift = append(drift, fmt.Sprintf("%s (rule %s) state Build: %s", reducer.Key, reducer.Rule, problem))
			}
		}
	}
	if declared == 0 {
		t.Fatal("no fold declares a sealed state, so this law measures nothing")
	}
	if len(drift) != 0 {
		t.Fatalf("fold states whose Build is not the one their declaration derives:\n\t%s", strings.Join(drift, "\n\t"))
	}
}
