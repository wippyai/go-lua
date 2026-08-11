package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// validateAuthoringType performs the target-owned preflight before the shared
// scoped encoder owns canonicalization. It is deliberately iterative: target
// declarations must remain safe for arbitrarily deep or recursive type graphs.
//
// Alias is permitted as transparent authoring sugar. Annotated is rejected
// rather than silently erased: annotations are source/runtime validation
// semantics and cannot become accidental portable ABI metadata.
func validateAuthoringType(root typ.Type) error {
	if root == nil {
		return errors.New("nil type")
	}
	stack := []typ.Type{root}
	seen := make(map[typ.Type]struct{})
	mark := func(value typ.Type) bool {
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
		return true
	}
	push := func(value typ.Type) error {
		if value == nil {
			return errors.New("nil type child")
		}
		stack = append(stack, value)
		return nil
	}
	for len(stack) > 0 {
		last := len(stack) - 1
		value := stack[last]
		stack = stack[:last]
		if value == nil {
			return errors.New("nil type")
		}
		switch current := value.(type) {
		case *typ.Annotated:
			return errors.New("annotated type is not portable target ABI")
		case *typ.Ref:
			return errors.New("unresolved type reference is not portable target ABI")
		case *typ.Optional:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil optional type")
				}
				continue
			}
			if err := push(current.Inner); err != nil {
				return err
			}
		case *typ.Union:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil union type")
				}
				continue
			}
			for _, child := range current.Members {
				if err := push(child); err != nil {
					return err
				}
			}
		case *typ.Intersection:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil intersection type")
				}
				continue
			}
			for _, child := range current.Members {
				if err := push(child); err != nil {
					return err
				}
			}
		case *typ.Tuple:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil tuple type")
				}
				continue
			}
			for _, child := range current.Elements {
				if err := push(child); err != nil {
					return err
				}
			}
		case *typ.Array:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil array type")
				}
				continue
			}
			if err := push(current.Element); err != nil {
				return err
			}
		case *typ.Map:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil map type")
				}
				continue
			}
			if err := push(current.Key); err != nil {
				return err
			}
			if err := push(current.Value); err != nil {
				return err
			}
		case *typ.ReadonlyMap:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil readonly map type")
				}
				continue
			}
			if err := push(current.Key); err != nil {
				return err
			}
			if err := push(current.Value); err != nil {
				return err
			}
		case *typ.Record:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil record type")
				}
				continue
			}
			if (current.MapKey == nil) != (current.MapValue == nil) {
				return errors.New("partial record map component")
			}
			for _, field := range current.Fields {
				if err := push(field.Type); err != nil {
					return err
				}
			}
			for _, member := range current.StaticMembers {
				if err := push(member.Type); err != nil {
					return err
				}
			}
			if current.Metatable != nil {
				if err := push(current.Metatable); err != nil {
					return err
				}
			}
			if current.MapKey != nil {
				if err := push(current.MapKey); err != nil {
					return err
				}
				if err := push(current.MapValue); err != nil {
					return err
				}
			}
		case *typ.Function:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil function type")
				}
				continue
			}
			for _, formal := range current.TypeParams {
				if err := pushTypeParam(formal, push); err != nil {
					return err
				}
			}
			for _, parameter := range current.Params {
				if err := push(parameter.Type); err != nil {
					return err
				}
			}
			if current.Variadic != nil {
				if err := push(current.Variadic); err != nil {
					return err
				}
			}
			for _, result := range current.Returns {
				if err := push(result); err != nil {
					return err
				}
			}
		case *typ.Generic:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil generic type")
				}
				continue
			}
			if current.Body == nil {
				return errors.New("incomplete generic type")
			}
			for _, formal := range current.TypeParams {
				if err := pushTypeParam(formal, push); err != nil {
					return err
				}
			}
			if err := push(current.Body); err != nil {
				return err
			}
		case *typ.Instantiated:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil instantiated type")
				}
				continue
			}
			if current.Generic == nil {
				return errors.New("instantiated type has nil generic")
			}
			if err := push(current.Generic); err != nil {
				return err
			}
			for _, argument := range current.TypeArgs {
				if err := push(argument); err != nil {
					return err
				}
			}
		case *typ.TypeParam:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil type formal")
				}
				continue
			}
			if current.Constraint != nil {
				if err := push(current.Constraint); err != nil {
					return err
				}
			}
		case *typ.Recursive:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil recursive type")
				}
				continue
			}
			if current.Body == nil {
				return errors.New("incomplete recursive type")
			}
			if err := push(current.Body); err != nil {
				return err
			}
		case *typ.Interface:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil interface type")
				}
				continue
			}
			for _, method := range current.Methods {
				if method.Type == nil {
					return errors.New("interface has nil method")
				}
				if err := push(method.Type); err != nil {
					return err
				}
			}
		case *typ.Meta:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil meta type")
				}
				continue
			}
			if err := push(current.Of); err != nil {
				return err
			}
		case *typ.Alias:
			if current == nil || !mark(value) {
				if current == nil {
					return errors.New("nil alias type")
				}
				continue
			}
			if err := push(current.Target); err != nil {
				return err
			}
		case *typ.Literal:
			if current == nil {
				return errors.New("nil literal type")
			}
		default:
			switch value.Kind() {
			case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Any, kind.Never:
				// The shared scoped codec makes the final concrete-type check.
			case kind.Unknown:
				return errors.New("unknown type is not portable target ABI")
			case kind.Self:
				return errors.New("self type is not portable target ABI")
			default:
				return errors.New("unsupported portable target type")
			}
		}
	}
	return nil
}

func pushTypeParam(param *typ.TypeParam, push func(typ.Type) error) error {
	if param == nil {
		return errors.New("nil nested type formal")
	}
	return push(param)
}
