package memberroster_test

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule/codegen"
	"github.com/wippyai/go-lua/domain/memberroster"
)

// outcomeGoType is the sealed disposition every reducer's last result binds to,
// spelled here through the same constant the outcome law pins.
func outcomeGoType() memberdefinition.GoType {
	return memberdefinition.GoType{PackagePath: outcomePackage, Name: "ReductionOutcome"}
}

// vectorGoType is the one view a many-valued read delivers its cells through.
// It is the execution layer's, named here for the same reason the disposition
// is: both are analyzer vocabulary a declaration derives against rather than
// chooses.
func vectorGoType() memberdefinition.GoType {
	return codegen.SummaryVectorType
}

func cellGoType() memberdefinition.GoType {
	return codegen.SelectionCellType
}

// TestEveryDeclaredFoldHasTheDerivedCallShape is the enforcement half of the
// call-shape contract, and the first consumer that holds a declaration to it.
//
// A reducer's signature is DERIVED from its declared rows - the optional
// candidate carrier, then each input's route and tag carriers when it has
// them, followed by that input's fact carrier or, when the read is
// many-valued, one view over it - answering the declared output carriers and
// one disposition. An implementation whose parameters disagree is not a
// reducer the generated call can reach: the emitter would pass carriers it
// does not accept.
//
// Sealed per-rule data is deliberately absent from the derived vector. A fold
// that needs the owner schema, a derived plan, or a projection takes it from
// the sealed state of the installed Family that calls it, which is why a
// receiver is not counted as a parameter here. That is the whole reason the
// signature cannot grow plumbing.
//
// This law is RED on heap/reducer/closed and is left red rather than narrowed.
// resultClosed takes the Heap schema, the Value schema and a selector
// projection alongside its carriers - three positions of exactly the plumbing
// the call shape exists to forbid - and delivers its many-valued input through
// the engine's own ordered-cell type rather than the view the execution layer
// materializes. Every one of those is the heap-closed cutover's work, and
// narrowing this law would hide the measurement of it.
func TestEveryDeclaredFoldHasTheDerivedCallShape(t *testing.T) {
	root := moduleRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	var drift []string
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("member definition source %q does not compose", source.Name)
		}
		for _, reducer := range composed.Reducers {
			arguments, results, derivedOK := composed.ReducerSignature(reducer, outcomeGoType(), cellGoType(), vectorGoType())
			if !derivedOK {
				drift = append(drift, fmt.Sprintf("%s (rule %s): declared rows name a carrier the axis does not declare", reducer.Key, reducer.Rule))
				continue
			}
			decl, file, declOK := findFunction(t, root, reducer.Implementation)
			if !declOK {
				drift = append(drift, fmt.Sprintf("%s (rule %s): implementation %s is not declared", reducer.Key, reducer.Rule, reducer.Implementation.Name))
				continue
			}
			if problem := compareSignature(file, reducer.Implementation.PackagePath, decl, arguments, results); problem != "" {
				drift = append(drift, fmt.Sprintf("%s (rule %s): %s", reducer.Key, reducer.Rule, problem))
			}
		}
	}
	if len(drift) != 0 {
		t.Fatalf("folds whose signature is not the one their declaration derives:\n\t%s", strings.Join(drift, "\n\t"))
	}
}

// compareSignature reports the first disagreement between a declaration's
// derived call shape and the implementation's actual one, or the empty string.
//
// The comparison itself is compareDerivedType's, which is the one statement of
// what a derived position looks like in source: a slice where the declaration
// says the delivery is one, an instantiated view where it names an element,
// and the named type otherwise. A fold's declared arguments carry exactly
// those three facts, so they are compared as the derived positions they are
// rather than through a second reading that would have to learn each of them
// again - and did not: it could not see a slice at all, so every fold taking
// the cells of a selection read as an unresolvable type.
func compareSignature(file *ast.File, declaring string, decl *ast.FuncDecl, arguments []definition.Argument, results []memberdefinition.GoType) string {
	parameters := flattenFields(decl.Type.Params)
	if len(parameters) != len(arguments) {
		return fmt.Sprintf("takes %d parameters, the declaration derives %d (%s)", len(parameters), len(arguments), describeArguments(arguments))
	}
	for position, argument := range arguments {
		want := memberdefinition.DerivedParam{Type: argument.Type, Element: argument.Element, Slice: argument.Slice}
		if problem := compareDerivedType(file, declaring, parameters[position], want, "parameter", position); problem != "" {
			return problem
		}
	}
	returned := flattenFields(decl.Type.Results)
	if len(returned) != len(results) {
		return fmt.Sprintf("returns %d results, the declaration derives %d", len(returned), len(results))
	}
	for position, want := range results {
		if problem := compareDerivedType(file, declaring, returned[position], memberdefinition.DerivedParam{Type: want}, "result", position); problem != "" {
			return problem
		}
	}
	return ""
}

// flattenFields expands one field list into one expression per declared name,
// so a grouped parameter list is compared position by position.
func flattenFields(fields *ast.FieldList) []ast.Expr {
	if fields == nil {
		return nil
	}
	var expanded []ast.Expr
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for repeat := 0; repeat < count; repeat++ {
			expanded = append(expanded, field.Type)
		}
	}
	return expanded
}

func describeType(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func describeArguments(arguments []definition.Argument) string {
	spelled := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		role := "fact"
		switch argument.Role {
		case definition.ArgumentCandidate:
			role = "candidate"
		case definition.ArgumentRoute:
			role = "route"
		case definition.ArgumentTag:
			role = "tag"
		case definition.ArgumentVector:
			role = "vector"
		}
		spelled = append(spelled, role+" "+describeType(argument.Type.PackagePath, argument.Type.Name))
	}
	return strings.Join(spelled, ", ")
}
