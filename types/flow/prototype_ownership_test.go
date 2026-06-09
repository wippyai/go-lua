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

func TestPrototypePointStateAccessStaysCapsuleOwned(t *testing.T) {
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
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || prototypeRawAccessAllowed(root, path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte(".PrototypeSelf")) && !bytes.Contains(data, []byte(".PrototypeInstances")) {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, data, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "PrototypeSelf" && sel.Sel.Name != "PrototypeInstances") {
				return true
			}
			receiver := staticMembersSelectorReceiver(t, fset, sel.X)
			if !prototypeReceiverLooksPointState(receiver) {
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
		t.Fatalf("raw PointState.PrototypeSelf/PrototypeInstances access outside owner/root composition: %v", offenders)
	}
}

func prototypeRawAccessAllowed(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == filepath.Join("types", "flow", "pointstate.go") ||
		rel == filepath.Join("types", "flow", "prototype_self.go")
}

func prototypeReceiverLooksPointState(receiver string) bool {
	switch receiver {
	case "ps", "state", "out", "point", "before", "after", "prev", "next", "incoming":
		return true
	}
	return strings.HasSuffix(receiver, ".state") ||
		strings.HasSuffix(receiver, ".State") ||
		strings.Contains(receiver, "PointState")
}
