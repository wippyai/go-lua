package memberroster_test

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"

	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
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
			shape, derivedOK := roster.DerivationSignature(composed.Axis, relation)
			if !derivedOK {
				drift = append(drift, fmt.Sprintf("%s: declared rows derive no call shape", relation.Key))
				continue
			}
			for _, call := range []struct {
				role    string
				symbol  memberdefinition.GoSymbol
				params  []memberdefinition.GoType
				results []memberdefinition.GoType
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
func compareDerivationCall(file *ast.File, declaring string, decl *ast.FuncDecl, params, results []memberdefinition.GoType) string {
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

func compareDerivedType(file *ast.File, declaring string, expr ast.Expr, want memberdefinition.GoType, role string, position int) string {
	_, pointer := expr.(*ast.StarExpr)
	path, name := resolvedType(file, declaring, expr)
	if path == want.PackagePath && name == want.Name && pointer == want.Pointer {
		return ""
	}
	return fmt.Sprintf("%s %d is %s, the declaration derives %s", role, position, describeDerivedType(path, name, pointer), describeDerivedType(want.PackagePath, want.Name, want.Pointer))
}

func describeDerivedType(path, name string, pointer bool) string {
	spelled := describeType(path, name)
	if pointer {
		return "*" + spelled
	}
	return spelled
}

func describeDerivedTypes(types []memberdefinition.GoType) string {
	spelled := make([]string, 0, len(types))
	for _, typ := range types {
		spelled = append(spelled, describeDerivedType(typ.PackagePath, typ.Name, typ.Pointer))
	}
	return strings.Join(spelled, ", ")
}
