package parsersource

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// NamedConstant is one compiler/ast package-level constant together with the
// judgement a construction proof needs from it: whether its value is the zero
// value of its own type. A discriminant field assigned a constant is in its
// zero state exactly when that constant is zero, so the constant's iota
// position is evidence rather than a naming convention.
type NamedConstant struct {
	Name string
	Zero bool
}

// DiscoverConstants derives every compiler/ast package-level constant whose
// value is decidable from the declaration alone: integer expressions over iota
// and other constants of the same package, string literals, and the boolean
// literals. A constant whose value cannot be decided is omitted rather than
// guessed, so a caller reading a missing name learns that it has no evidence
// instead of receiving a default.
func DiscoverConstants(root string) ([]NamedConstant, error) {
	directory := filepath.Join(root, "compiler", "ast")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("grammar requirements: read AST directory: %w", err)
	}
	type pending struct {
		expressions []goast.Expr
		iota        int
	}
	specifications := make(map[string]pending)
	var order []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return nil, fmt.Errorf("grammar requirements: parse %s: %w", path, parseErr)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*goast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			var inherited []goast.Expr
			for index, specification := range general.Specs {
				valueSpec, ok := specification.(*goast.ValueSpec)
				if !ok {
					continue
				}
				if len(valueSpec.Values) != 0 {
					inherited = valueSpec.Values
				}
				for position, name := range valueSpec.Names {
					if name.Name == "_" {
						continue
					}
					if _, duplicate := specifications[name.Name]; duplicate {
						return nil, fmt.Errorf("grammar requirements: duplicate AST constant %s", name.Name)
					}
					if position >= len(inherited) {
						continue
					}
					specifications[name.Name] = pending{expressions: inherited, iota: index}
					order = append(order, name.Name)
				}
			}
		}
	}
	values := make(map[string]int64, len(specifications))
	texts := make(map[string]string, len(specifications))
	resolving := make(map[string]bool, len(specifications))
	var resolve func(name string) (bool, bool)
	resolve = func(name string) (bool, bool) {
		if zero, decided := values[name]; decided {
			return zero == 0, true
		}
		if text, decided := texts[name]; decided {
			return text == "", true
		}
		specification, known := specifications[name]
		if !known || resolving[name] {
			return false, false
		}
		resolving[name] = true
		defer delete(resolving, name)
		for _, expression := range specification.expressions {
			if text, ok := constantText(expression); ok {
				texts[name] = text
				return text == "", true
			}
			number, ok := constantNumber(expression, specification.iota, func(reference string) (int64, bool) {
				if _, decided := resolve(reference); !decided {
					return 0, false
				}
				number, decided := values[reference]
				return number, decided
			})
			if !ok {
				continue
			}
			values[name] = number
			return number == 0, true
		}
		return false, false
	}
	result := make([]NamedConstant, 0, len(order))
	for _, name := range order {
		zero, decided := resolve(name)
		if !decided {
			continue
		}
		result = append(result, NamedConstant{Name: name, Zero: zero})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Name < result[right].Name })
	return result, nil
}

func constantText(expression goast.Expr) (string, bool) {
	literal, ok := expression.(*goast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return text, true
}

func constantNumber(expression goast.Expr, iota int, lookup func(string) (int64, bool)) (int64, bool) {
	switch value := expression.(type) {
	case *goast.BasicLit:
		if value.Kind != token.INT && value.Kind != token.CHAR {
			return 0, false
		}
		if value.Kind == token.CHAR {
			text, err := strconv.Unquote(value.Value)
			if err != nil || len(text) == 0 {
				return 0, false
			}
			return int64([]rune(text)[0]), true
		}
		number, err := strconv.ParseInt(value.Value, 0, 64)
		if err != nil {
			return 0, false
		}
		return number, true
	case *goast.Ident:
		switch value.Name {
		case "iota":
			return int64(iota), true
		case "true":
			return 1, true
		case "false":
			return 0, true
		}
		return lookup(value.Name)
	case *goast.ParenExpr:
		return constantNumber(value.X, iota, lookup)
	case *goast.UnaryExpr:
		operand, ok := constantNumber(value.X, iota, lookup)
		if !ok {
			return 0, false
		}
		switch value.Op {
		case token.SUB:
			return -operand, true
		case token.ADD:
			return operand, true
		case token.XOR:
			return ^operand, true
		}
		return 0, false
	case *goast.BinaryExpr:
		left, leftOK := constantNumber(value.X, iota, lookup)
		right, rightOK := constantNumber(value.Y, iota, lookup)
		if !leftOK || !rightOK {
			return 0, false
		}
		switch value.Op {
		case token.ADD:
			return left + right, true
		case token.SUB:
			return left - right, true
		case token.MUL:
			return left * right, true
		case token.SHL:
			if right < 0 || right > 62 {
				return 0, false
			}
			return left << uint(right), true
		case token.OR:
			return left | right, true
		case token.AND:
			return left & right, true
		}
		return 0, false
	case *goast.CallExpr:
		if len(value.Args) != 1 {
			return 0, false
		}
		if _, ok := value.Fun.(*goast.Ident); !ok {
			return 0, false
		}
		return constantNumber(value.Args[0], iota, lookup)
	default:
		return 0, false
	}
}
