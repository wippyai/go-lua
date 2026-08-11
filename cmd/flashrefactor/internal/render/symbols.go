package render

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

type symbolShape struct {
	packagePath string
	owner       string
	member      string
	kind        symbolKind
}

type symbolKind uint8

const (
	symbolInvalid symbolKind = iota
	symbolPackage
	symbolField
	symbolMethod
)

func parseSymbol(ref cutplan.SymbolRef) (symbolShape, error) {
	parts := strings.Split(ref.Object, "#")
	if len(parts) != 2 || parts[0] == "" {
		return symbolShape{}, fmt.Errorf("invalid exact symbol %q", ref.Object)
	}
	shape := symbolShape{packagePath: parts[0]}
	if strings.HasPrefix(parts[1], "package:") {
		shape.kind = symbolPackage
		shape.member = strings.TrimPrefix(parts[1], "package:")
	} else {
		member := strings.Split(parts[1], "/")
		if len(member) != 2 || !strings.HasPrefix(member[0], "type:") {
			return symbolShape{}, fmt.Errorf("invalid exact symbol %q", ref.Object)
		}
		shape.owner = strings.TrimPrefix(member[0], "type:")
		switch {
		case strings.HasPrefix(member[1], "field:"):
			shape.kind, shape.member = symbolField, strings.TrimPrefix(member[1], "field:")
		case strings.HasPrefix(member[1], "method:"):
			shape.kind, shape.member = symbolMethod, strings.TrimPrefix(member[1], "method:")
		default:
			return symbolShape{}, fmt.Errorf("invalid exact symbol %q", ref.Object)
		}
	}
	if !token.IsIdentifier(shape.member) || shape.member == "_" || (shape.owner != "" && !token.IsIdentifier(shape.owner)) {
		return symbolShape{}, fmt.Errorf("invalid exact symbol %q", ref.Object)
	}
	return shape, nil
}

func targetName(ref cutplan.SymbolRef) (string, error) {
	shape, err := parseSymbol(ref)
	if err != nil {
		return "", err
	}
	return shape.member, nil
}

func (state *renderState) sourceObject(ref cutplan.SymbolRef) (types.Object, error) {
	object, err := state.workspace.Object(ref)
	if err != nil {
		return nil, fmt.Errorf("source object %s: %w", ref.Object, err)
	}
	return object, nil
}

func sameObjectKind(object types.Object, shape symbolShape) bool {
	switch shape.kind {
	case symbolPackage:
		switch object.(type) {
		case *types.TypeName, *types.Func, *types.Var, *types.Const:
			return true
		}
	case symbolField:
		value, ok := object.(*types.Var)
		return ok && value.IsField()
	case symbolMethod:
		value, ok := object.(*types.Func)
		if !ok {
			return false
		}
		signature, ok := value.Type().(*types.Signature)
		return ok && signature.Recv() != nil
	}
	return false
}

func objectPackagePath(object types.Object) string {
	if object == nil || object.Pkg() == nil {
		return ""
	}
	return object.Pkg().Path()
}
