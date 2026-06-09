package flow

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestStaticMembersPointStateAccessStaysCapsuleOwned(t *testing.T) {
	root := findModuleRootForPathAliasGuard(t)
	scanRoots := []string{
		filepath.Join(root, "compiler", "check"),
		filepath.Join(root, "types", "flow"),
	}
	var offenders []string
	for _, scanRoot := range scanRoots {
		err := filepath.WalkDir(scanRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || staticMembersRawAccessAllowed(root, path) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !bytes.Contains(data, []byte(".StaticMembers")) {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, data, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				sel, ok := node.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "StaticMembers" {
					return true
				}
				receiver := staticMembersSelectorReceiver(t, fset, sel.X)
				if !staticMembersReceiverLooksPointState(receiver) {
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
	}
	if len(offenders) != 0 {
		t.Fatalf("raw PointState.StaticMembers access outside owner/root composition: %v", offenders)
	}
}

func staticMembersRawAccessAllowed(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == filepath.Join("types", "flow", "pointstate.go") ||
		rel == filepath.Join("types", "flow", "static_member_facts.go") ||
		rel == filepath.Join("types", "flow", "static_member_effect.go")
}

func staticMembersSelectorReceiver(t *testing.T, fset *token.FileSet, expr ast.Expr) string {
	t.Helper()
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		t.Fatalf("print selector receiver: %v", err)
	}
	return buf.String()
}

func staticMembersReceiverLooksPointState(receiver string) bool {
	switch receiver {
	case "ps", "state", "out", "point", "before", "after", "prev", "next":
		return true
	}
	return strings.HasSuffix(receiver, ".state") ||
		strings.HasSuffix(receiver, ".State") ||
		strings.Contains(receiver, "PointState")
}
