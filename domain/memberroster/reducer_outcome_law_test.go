package memberroster_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/domain/memberroster"
)

const (
	modulePath      = "github.com/wippyai/go-lua"
	outcomePackage  = "github.com/wippyai/go-lua/analysis/schema/structure"
	outcomeTypeName = "ReductionOutcome"
)

// TestEveryDeclaredFoldReturnsTheSealedOutcome is the vocabulary law for G10,
// stated over source rather than over a declaration: every fold the
// composition declares - a rule's reducer and an owner's source materializer
// alike - answers with the sealed disposition and nothing standing in for it.
//
// It is a source law because the two ways a fold can encode a disposition in
// its value are both invisible to the declaration: returning a bare fact makes
// the fact's own bottom element mean "no candidate", and returning a boolean
// makes a two-valued answer stand in for a five-valued one. Both are refused
// here by name.
//
// A fold answers in one of two shapes, and which one is a property of what its
// rule publishes. A fold that publishes a fact returns exactly (fact,
// structure.ReductionOutcome). A structural fold publishes no fact at all - its
// output is the activation set its branches mount, so its declaration carries
// no output carrier and there is no fact for it to return - and it returns
// exactly structure.ReductionOutcome. The second shape refuses the same two
// encodings as the first: a bare fact has nowhere to go, and a bool is still a
// two-valued answer where five are declared.
func TestEveryDeclaredFoldReturnsTheSealedOutcome(t *testing.T) {
	root := moduleRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	var offenders []string
	for index := 0; index < roster.Count(); index++ {
		source, sourceOK := roster.At(index)
		if !sourceOK {
			t.Fatalf("roster position %d is absent", index)
		}
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("member definition source %q does not compose", source.Name)
		}
		factType, factOK := carrierType(composed, composed.Signature.Fact)
		if !factOK {
			t.Fatalf("%s: fact carrier %q is not declared", source.Name, composed.Signature.Fact)
		}
		for _, reducer := range composed.Reducers {
			if len(reducer.Outputs) == 0 {
				if problem := checkStructuralFoldSignature(t, root, reducer.Implementation); problem != "" {
					offenders = append(offenders, string(reducer.Key)+" (structural rule "+string(reducer.Rule)+"): "+problem)
				}
				continue
			}
			outputType, outputOK := carrierType(composed, reducer.Outputs[0].Carrier)
			if !outputOK {
				t.Fatalf("%s/%s: output carrier %q is not declared", source.Name, reducer.Key, reducer.Outputs[0].Carrier)
			}
			if problem := checkFoldSignature(t, root, reducer.Implementation, outputType); problem != "" {
				offenders = append(offenders, string(reducer.Key)+" (rule "+string(reducer.Rule)+"): "+problem)
			}
		}
		for _, relation := range composed.Relations {
			if !relation.Materialize.Available() {
				continue
			}
			if problem := checkFoldSignature(t, root, relation.Materialize, factType); problem != "" {
				offenders = append(offenders, string(relation.Key)+" (materializer): "+problem)
			}
		}
	}
	if len(offenders) != 0 {
		t.Fatalf("folds outside the sealed outcome vocabulary:\n\t%s", strings.Join(offenders, "\n\t"))
	}
}

// checkStructuralFoldSignature returns the empty string when symbol's Go
// declaration returns exactly structure.ReductionOutcome, and a named problem
// otherwise. It is the shape a fold with no declared output carrier answers in:
// the disposition alone, because the row it settles publishes no fact.
func checkStructuralFoldSignature(t *testing.T, root string, symbol memberdefinition.GoSymbol) string {
	t.Helper()
	decl, file, declOK := findFunction(t, root, symbol)
	if !declOK {
		return "declaration " + symbol.PackagePath + "." + symbol.Name + " was not found"
	}
	results := decl.Type.Results
	if results == nil || len(results.List) != 1 {
		return "returns " + resultSpelling(results) + ", want (" + outcomeTypeName + ")"
	}
	outcomePkg, outcomeName := resolvedType(file, symbol.PackagePath, results.List[0].Type)
	if outcomeName != outcomeTypeName || outcomePkg != outcomePackage {
		return "result is " + outcomePkg + "." + outcomeName + ", want " + outcomePackage + "." + outcomeTypeName
	}
	return ""
}

// checkFoldSignature returns the empty string when symbol's Go declaration
// returns exactly (want, structure.ReductionOutcome), and a named problem
// otherwise.
func checkFoldSignature(t *testing.T, root string, symbol memberdefinition.GoSymbol, want memberdefinition.GoType) string {
	t.Helper()
	decl, file, declOK := findFunction(t, root, symbol)
	if !declOK {
		return "declaration " + symbol.PackagePath + "." + symbol.Name + " was not found"
	}
	results := decl.Type.Results
	if results == nil || len(results.List) != 2 {
		return "returns " + resultSpelling(results) + ", want (" + want.Name + ", " + outcomeTypeName + ")"
	}
	factPackage, factName := resolvedType(file, symbol.PackagePath, results.List[0].Type)
	if factName != want.Name || factPackage != want.PackagePath {
		return "first result is " + factPackage + "." + factName + ", want " + want.PackagePath + "." + want.Name
	}
	outcomePkg, outcomeName := resolvedType(file, symbol.PackagePath, results.List[1].Type)
	if outcomeName != outcomeTypeName || outcomePkg != outcomePackage {
		return "second result is " + outcomePkg + "." + outcomeName + ", want " + outcomePackage + "." + outcomeTypeName
	}
	return ""
}

// resolvedType maps one result expression to the package path and name it
// denotes. An unqualified name belongs to the declaring package; a qualified
// one is resolved through the file's own import set, so a law cannot be
// satisfied by a foreign type that happens to share a spelling.
func resolvedType(file *ast.File, declaring string, expr ast.Expr) (string, string) {
	switch typed := expr.(type) {
	case *ast.Ident:
		// GoType is the declaration authority for the closed built-in spelling
		// set.  An unqualified builtin belongs to no declaring package; treating
		// it as a package-local identifier would make `uint64` resolve as
		// `store.uint64` and reject the exact carrier the declaration derives.
		if (memberdefinition.GoType{Name: typed.Name}).Available() {
			return "", typed.Name
		}
		return declaring, typed.Name
	case *ast.StarExpr:
		path, name := resolvedType(file, declaring, typed.X)
		return path, name
	case *ast.SelectorExpr:
		qualifier, isIdent := typed.X.(*ast.Ident)
		if !isIdent {
			return "", typed.Sel.Name
		}
		return importPath(file, qualifier.Name), typed.Sel.Name
	case *ast.IndexExpr:
		// A generic instantiation resolves to the generic type it names. Its
		// type argument is compared separately, against the element the
		// declaration derives, so the two halves of a vector position are held
		// to the declaration independently.
		return resolvedType(file, declaring, typed.X)
	default:
		return "", "?"
	}
}

// typeArgument resolves the single type argument of a generic instantiation.
// A parameter that is not one has no argument, which is how a vector position
// spelled as a bare type is caught.
func typeArgument(file *ast.File, declaring string, expr ast.Expr) (string, string, bool) {
	indexed, isIndexed := expr.(*ast.IndexExpr)
	if !isIndexed {
		return "", "", false
	}
	path, name := resolvedType(file, declaring, indexed.Index)
	return path, name, true
}

func importPath(file *ast.File, qualifier string) string {
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		if imported.Name != nil {
			if imported.Name.Name == qualifier {
				return path
			}
			continue
		}
		if path == qualifier || strings.HasSuffix(path, "/"+qualifier) {
			return path
		}
	}
	return ""
}

func resultSpelling(results *ast.FieldList) string {
	if results == nil {
		return "()"
	}
	parts := make([]string, 0, len(results.List))
	for _, field := range results.List {
		parts = append(parts, typeSpelling(field.Type))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func typeSpelling(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return "*" + typeSpelling(typed.X)
	case *ast.SelectorExpr:
		return typeSpelling(typed.X) + "." + typed.Sel.Name
	default:
		return "?"
	}
}

func findFunction(t *testing.T, root string, symbol memberdefinition.GoSymbol) (*ast.FuncDecl, *ast.File, bool) {
	t.Helper()
	directory := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(symbol.PackagePath, modulePath+"/")))
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, directory, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", directory, err)
	}
	for _, parsed := range packages {
		for _, file := range parsed.Files {
			for _, declaration := range file.Decls {
				function, isFunction := declaration.(*ast.FuncDecl)
				if !isFunction || function.Name.Name != symbol.Name {
					continue
				}
				if !receiverMatches(function, symbol) {
					continue
				}
				return function, file, true
			}
		}
	}
	return nil, nil, false
}

func receiverMatches(function *ast.FuncDecl, symbol memberdefinition.GoSymbol) bool {
	if symbol.Receiver.Name == "" {
		return function.Recv == nil
	}
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return false
	}
	spelling := typeSpelling(function.Recv.List[0].Type)
	if symbol.ReceiverPointer {
		return spelling == "*"+symbol.Receiver.Name
	}
	return spelling == symbol.Receiver.Name
}

func carrierType(definition memberdefinition.Definition, name string) (memberdefinition.GoType, bool) {
	for _, carrier := range definition.Carriers {
		if carrier.Name == name {
			return carrier.Type, true
		}
	}
	return memberdefinition.GoType{}, false
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("module root was not found")
		}
		directory = parent
	}
}

// TestOutcomePackageIsTheOneDeclaredVocabulary pins the constant this law
// compares against to the package that declares the category, so the law
// cannot drift onto a second spelling.
func TestOutcomePackageIsTheOneDeclaredVocabulary(t *testing.T) {
	if outcomePackage != modulePath+"/analysis/schema/structure" {
		t.Fatalf("outcome package = %q", outcomePackage)
	}
}

// TestEveryOutcomeHasADeclaredProducer is the population half of G10: the
// vocabulary is five-valued because five distinct things happen, so each
// member must be the conclusion of some declared fold. A vocabulary whose
// members no fold ever returns is a vocabulary the analyzer cannot observe.
//
// The named specimens the ruling fixes are arithmetic for NoCandidate (its
// bottom result today means "no candidate", which is exactly the encoding this
// outcome removes), the activation relation for NoSelection, call dispatch for
// Refuse, and an authenticated opaque read for AuthenticatedOpaque. This law
// is RED until those rules are declarations in the roster: the two folds
// migrated so far conclude only Concrete and Refuse, so three of the five
// outcomes have no producer and cannot be observed on the Delta path at all.
// It is left red rather than narrowed, because narrowing it would hide exactly
// the gap it exists to measure.
func TestEveryOutcomeHasADeclaredProducer(t *testing.T) {
	root := moduleRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	produced := map[string]string{}
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("member definition source %q does not compose", source.Name)
		}
		for _, reducer := range composed.Reducers {
			for _, outcome := range concludedOutcomes(t, root, reducer.Implementation) {
				if _, seen := produced[outcome]; !seen {
					produced[outcome] = string(reducer.Rule)
				}
			}
		}
	}
	specimens := []struct{ outcome, specimen string }{
		{"Refuse", "call dispatch"},
		{"NoSelection", "the activation relation"},
		{"NoCandidate", "value arithmetic"},
		{"Concrete", "value transfer"},
		{"AuthenticatedOpaque", "an authenticated opaque read"},
	}
	var missing []string
	for _, row := range specimens {
		if _, ok := produced[row.outcome]; !ok {
			missing = append(missing, row.outcome+" (specimen: "+row.specimen+")")
		}
	}
	if len(missing) != 0 {
		t.Fatalf("outcomes no declared fold concludes, so no Delta row can carry them:\n\t%s", strings.Join(missing, "\n\t"))
	}
}

// concludedOutcomes reports the sealed outcome members one fold returns, read
// from the return statements of its declaration.
func concludedOutcomes(t *testing.T, root string, symbol memberdefinition.GoSymbol) []string {
	t.Helper()
	decl, file, declOK := findFunction(t, root, symbol)
	if !declOK {
		return nil
	}
	var outcomes []string
	ast.Inspect(decl, func(node ast.Node) bool {
		statement, isReturn := node.(*ast.ReturnStmt)
		if !isReturn || len(statement.Results) != 2 {
			return true
		}
		path, name := resolvedType(file, symbol.PackagePath, statement.Results[1])
		if path == outcomePackage {
			outcomes = append(outcomes, name)
		}
		return true
	})
	return outcomes
}
