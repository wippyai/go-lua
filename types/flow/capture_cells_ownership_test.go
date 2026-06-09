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

func TestCaptureCellsPointStateAccessStaysCapsuleOwned(t *testing.T) {
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
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || captureAxisRawAccessAllowed(root, path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte(".Cells")) && !bytes.Contains(data, []byte(".CellEffects")) {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, data, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Cells" && sel.Sel.Name != "CellEffects") {
				return true
			}
			receiver := staticMembersSelectorReceiver(t, fset, sel.X)
			if !captureAxisReceiverLooksPointState(receiver) {
				return true
			}
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel+":"+strconv.Itoa(fset.Position(sel.Pos()).Line))
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", scanRoot, err)
	}
	if len(offenders) != 0 {
		t.Fatalf("raw PointState.Cells/CellEffects access outside owner/root composition: %v", offenders)
	}
}

func captureAxisRawAccessAllowed(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	switch rel {
	case filepath.Join("types", "flow", "pointstate.go"),
		filepath.Join("types", "flow", "capture_cells.go"),
		filepath.Join("types", "flow", "capture_effects.go"):
		return true
	}
	return false
}

func captureAxisReceiverLooksPointState(receiver string) bool {
	switch receiver {
	case "ps", "state", "out", "point", "before", "after", "prev", "next", "incoming":
		return true
	}
	return strings.HasSuffix(receiver, ".state") ||
		strings.HasSuffix(receiver, ".State") ||
		strings.Contains(receiver, "PointState")
}
