package astcodec

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestGeneratedASTCodecObservesSemanticFields proves the generated codec's
// contract through a real parser AST: a literal's authored value is retained
// as a non-zero field state instead of merely proving that the generated file
// exists.
func TestGeneratedASTCodecObservesSemanticFields(t *testing.T) {
	statements, err := parse.ParseString("local value = 7", "codec.lua")
	if err != nil {
		t.Fatal(err)
	}
	occurrences := Observe(statements)
	if err := requireField(occurrences, "LocalAssignStmt", "Names", FieldStateNonEmpty); err != nil {
		t.Fatal(err)
	}
	if err := requireField(occurrences, "NumberExpr", "Value", FieldStateNonEmpty); err != nil {
		t.Fatal(err)
	}
	if _, ok := statements[0].(*ast.LocalAssignStmt); !ok {
		t.Fatalf("parser returned %T, want local assignment", statements[0])
	}
}

func requireField(rows []Occurrence, typ, name string, state FieldState) error {
	for _, row := range rows {
		if row.Type != typ {
			continue
		}
		for _, field := range row.Fields {
			if field.Name == name && field.State == state {
				return nil
			}
		}
	}
	return &missingFieldError{typ: typ, name: name, state: state}
}

type missingFieldError struct {
	typ   string
	name  string
	state FieldState
}

func (e *missingFieldError) Error() string {
	return "generated AST codec did not observe " + e.typ + "." + e.name
}
