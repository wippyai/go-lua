package flow

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReferenceRefsPointStateAccessStaysCapsuleOwned(t *testing.T) {
	root := findModuleRootForPathAliasGuard(t)
	scanRoot := filepath.Join(root, "types", "flow")
	var offenders []string
	err := filepath.WalkDir(scanRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || referenceRefsRawAccessAllowed(root, path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte(".FunctionRefs")) && !bytes.Contains(data, []byte(".ClosureRefs")) {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, data, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "FunctionRefs" && sel.Sel.Name != "ClosureRefs") {
				return true
			}
			receiver := staticMembersSelectorReceiver(t, fset, sel.X)
			if !referenceRefsReceiverLooksPointState(receiver) {
				return true
			}
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel+":"+strconv.Itoa(fset.Position(sel.Pos()).Line)+":."+sel.Sel.Name)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", scanRoot, err)
	}
	if len(offenders) != 0 {
		t.Fatalf("raw PointState FunctionRefs/ClosureRefs access outside owner/root/reference-context composition: %v", offenders)
	}
}

func referenceRefsRawAccessAllowed(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == filepath.Join("types", "flow", "pointstate.go") ||
		rel == filepath.Join("types", "flow", "function_refs.go") ||
		rel == filepath.Join("types", "flow", "closure_refs.go") ||
		rel == filepath.Join("types", "flow", "reference_context.go")
}

func referenceRefsReceiverLooksPointState(receiver string) bool {
	switch receiver {
	case "ps", "state", "out", "point", "before", "after", "prev", "next":
		return true
	}
	return strings.HasSuffix(receiver, ".state") ||
		strings.HasSuffix(receiver, ".State") ||
		strings.Contains(receiver, "PointState")
}
