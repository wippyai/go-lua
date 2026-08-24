package measure

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// scheduledDeathRows finds every file named scheduled_death.go under root
// and counts the composite-literal elements of its package-level slice
// variables - the scheduled-death migration ledger. Parsing the file
// rather than grepping it survives reformatting and comment changes that
// a line-count heuristic would miscount.
func scheduledDeathRows(root string) (int, error) {
	total := 0
	err := walkGoFiles(root, func(path, name string) error {
		if name != "scheduled_death.go" {
			return nil
		}
		n, err := countSliceLiteralElements(path)
		if err != nil {
			return err
		}
		total += n
		return nil
	})
	return total, err
}

// countSliceLiteralElements parses the Go source file at path and sums the
// element counts of every package-level var whose initializer is a slice
// composite literal.
func countSliceLiteralElements(path string) (int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return 0, err
	}
	rows := 0
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, value := range vs.Values {
				cl, ok := value.(*ast.CompositeLit)
				if !ok {
					continue
				}
				if _, isSlice := cl.Type.(*ast.ArrayType); !isSlice {
					continue
				}
				rows += len(cl.Elts)
			}
		}
	}
	return rows, nil
}
