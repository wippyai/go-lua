package memberroster_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/domain/memberroster"
)

// TestEveryAxisSourceComposes is the roster's own admission law: every
// registered axis folds its base with its rules' contributions into one
// complete member definition, and the reducer rows it ends with are exactly
// the rows its contributions declared.
func TestEveryAxisSourceComposes(t *testing.T) {
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	if roster.Count() == 0 {
		t.Fatal("the roster registers no axis")
	}
	for index := 0; index < roster.Count(); index++ {
		source, sourceOK := roster.At(index)
		if !sourceOK {
			t.Fatalf("roster position %d is absent", index)
		}
		if len(source.Base.Reducers) != 0 {
			t.Fatalf("%s declares %d reducers in its base; a reducer belongs to the rule that folds with it", source.Name, len(source.Base.Reducers))
		}
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("%s does not compose", source.Name)
		}
		declared := 0
		rules := make(map[string]struct{}, len(source.Contributions))
		for _, contribution := range source.Contributions {
			if contribution.Axis != source.Base.Axis {
				t.Fatalf("%s: contribution of %q names axis %q", source.Name, contribution.Rule, contribution.Axis)
			}
			if _, duplicate := rules[string(contribution.Rule)]; duplicate {
				t.Fatalf("%s: rule %q contributes twice", source.Name, contribution.Rule)
			}
			rules[string(contribution.Rule)] = struct{}{}
			declared += len(contribution.Reducers)
		}
		if len(composed.Reducers) != declared {
			t.Fatalf("%s composes %d reducers from %d declared", source.Name, len(composed.Reducers), declared)
		}
		for _, reducer := range composed.Reducers {
			if _, known := rules[string(reducer.Rule)]; !known {
				t.Fatalf("%s: reducer %q traces to no registered contribution", source.Name, reducer.Key)
			}
		}
	}
}

// TestEachRuleDeclaresItsReducerInItsOwnPackage is the placement half of the
// composition law: a rule's reducer definition lives in that rule's own
// generator-only package, never in the axis owner's. That is what makes adding
// a rule an edit to the rule's package and one line here, and nothing else.
//
// It is a source law over this file, because where a value is authored is not
// something the value itself can state.
func TestEachRuleDeclaresItsReducerInItsOwnPackage(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "roster.go", nil, 0)
	if err != nil {
		t.Fatalf("parse roster: %v", err)
	}
	imports := make(map[string]string, len(file.Imports))
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		alias := path[strings.LastIndex(path, "/")+1:]
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		imports[alias] = path
	}
	bases := map[string]struct{}{}
	contributions := map[string]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		composite, isComposite := node.(*ast.CompositeLit)
		if !isComposite {
			return true
		}
		selector, isSelector := composite.Type.(*ast.SelectorExpr)
		if !isSelector || selector.Sel.Name != "Source" {
			return true
		}
		for _, element := range composite.Elts {
			field, isField := element.(*ast.KeyValueExpr)
			key, isKey := field.Key.(*ast.Ident)
			if !isField || !isKey {
				continue
			}
			switch key.Name {
			case "Base":
				if path, ok := callPackage(imports, field.Value); ok {
					bases[path] = struct{}{}
				}
			case "Contributions":
				list, isList := field.Value.(*ast.CompositeLit)
				if !isList {
					continue
				}
				for _, entry := range list.Elts {
					if path, ok := callPackage(imports, entry); ok {
						contributions[path] = path
					}
				}
			}
		}
		return true
	})
	if len(contributions) == 0 {
		t.Fatal("the roster registers no reducer contribution")
	}
	for path := range contributions {
		if !strings.HasSuffix(path, "/memberdefinition") {
			t.Fatalf("contribution %q is not a generator-only member definition package", path)
		}
		if _, isBase := bases[path]; isBase {
			t.Fatalf("contribution %q is declared in its axis owner's base package", path)
		}
	}
	// One contribution package declares one rule. The measure is the set of
	// RULE keys the composed roster carries, not the number of contribution
	// values in it: a contribution that declares rows of another axis is
	// folded into that axis's source, so one authored package legitimately
	// appears as several contributions - all of them naming the one rule that
	// authored them. Counting values instead of rules would forbid a rule from
	// declaring a foreign row, which is a different law and not this one.
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	rules := map[schema.Key]struct{}{}
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		for _, contribution := range source.Contributions {
			rules[contribution.Rule] = struct{}{}
		}
	}
	if len(rules) != len(contributions) {
		t.Fatalf("the roster registers %d rules from %d distinct packages; a package declaring two rules is a second central list", len(rules), len(contributions))
	}
}

func callPackage(imports map[string]string, expr ast.Expr) (string, bool) {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return "", false
	}
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return "", false
	}
	qualifier, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return "", false
	}
	path, known := imports[qualifier.Name]
	return path, known
}

// TestEveryComposedAxisNamesThePackageItIsGeneratedInto is the roster-level
// half of the definition's package statement.
//
// A definition is internally consistent without knowing where it will be
// written, so the vocabulary law does not ask. The COMPOSITION does: every
// axis in this roster has a cold catalog on disk, a relation owner beside or
// below it, and a dense Factor coordinate that emitted families type their
// primitives at - and each of those is spelled from this one path. An axis
// that names none would be one whose generated symbols no downstream emitter
// can reach, discovered as an unexplained refusal rather than as this.
func TestEveryComposedAxisNamesThePackageItIsGeneratedInto(t *testing.T) {
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	if roster.Count() == 0 {
		t.Fatal("the roster carries no axis, so this law measures nothing")
	}
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("member definition source %q does not compose", source.Name)
		}
		cold, coldOK := memberdefinition.ColdImportPath(composed)
		if !coldOK {
			t.Fatalf("axis %q names no package, so nothing can spell its cold catalog", composed.Axis)
		}
		if _, denseOK := memberdefinition.DenseCoordinateType(composed, "DenseCoordinate"); !denseOK {
			t.Fatalf("axis %q at %q publishes no dense coordinate an emitted family could type against", composed.Axis, cold)
		}
	}
}
