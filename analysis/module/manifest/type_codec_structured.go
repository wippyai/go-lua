package manifest

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func encodeFunction(f *typ.Function) (*typeWire, error) {
	out := &typeWire{Kind: "function"}
	for _, p := range f.TypeParams {
		encoded, err := encodeTypeParamWire(p)
		if err != nil {
			return nil, err
		}
		out.TypeParams = append(out.TypeParams, encoded)
	}
	for _, p := range f.Params {
		encoded, err := encodeType(p.Type)
		if err != nil {
			return nil, err
		}
		out.Params = append(out.Params, paramWire{Name: p.Name, Type: encoded, Optional: p.Optional})
	}
	var err error
	out.Variadic, err = encodeType(f.Variadic)
	if err != nil {
		return nil, err
	}
	out.Returns, err = encodeTypeList(f.Returns)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeFunction(w *typeWire) (typ.Type, error) {
	b := typ.Func()
	for _, p := range w.TypeParams {
		decoded, err := decodeTypeParamWire(p)
		if err != nil {
			return nil, err
		}
		b.TypeParamRef(decoded)
	}
	for _, p := range w.Params {
		t, err := decodeType(p.Type)
		if err != nil {
			return nil, err
		}
		if p.Optional {
			b.OptParam(p.Name, t)
		} else {
			b.Param(p.Name, t)
		}
	}
	if w.Variadic != nil {
		v, err := decodeType(w.Variadic)
		if err != nil {
			return nil, err
		}
		b.Variadic(v)
	}
	returns, err := decodeTypeList(w.Returns)
	if err != nil {
		return nil, err
	}
	if len(returns) > 0 {
		b.Returns(returns...)
	}
	return b.Build(), nil
}

func encodeInterface(i *typ.Interface) (*typeWire, error) {
	out := &typeWire{Kind: "interface", Name: i.Name}
	methods := append([]typ.Method(nil), i.Methods...)
	sort.Slice(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })
	for _, method := range methods {
		encoded, err := encodeType(method.Type)
		if err != nil {
			return nil, err
		}
		out.Methods = append(out.Methods, methodWire{Name: method.Name, Type: encoded})
	}
	return out, nil
}

func decodeInterface(w *typeWire) (typ.Type, error) {
	methods := make([]typ.Method, 0, len(w.Methods))
	for _, method := range w.Methods {
		t, err := decodeType(method.Type)
		if err != nil {
			return nil, err
		}
		fn, ok := t.(*typ.Function)
		if !ok {
			return nil, fmt.Errorf("interface method %q type is %T", method.Name, t)
		}
		methods = append(methods, typ.Method{Name: method.Name, Type: fn})
	}
	return typ.NewInterface(w.Name, methods), nil
}

func encodeGeneric(g *typ.Generic) (*typeWire, error) {
	out := &typeWire{Kind: "generic", Name: g.Name}
	for _, p := range g.TypeParams {
		encoded, err := encodeTypeParamWire(p)
		if err != nil {
			return nil, err
		}
		out.TypeParams = append(out.TypeParams, encoded)
	}
	var err error
	out.Body, err = encodeType(g.Body)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeGeneric(w *typeWire) (typ.Type, error) {
	params := make([]*typ.TypeParam, 0, len(w.TypeParams))
	for _, p := range w.TypeParams {
		decoded, err := decodeTypeParamWire(p)
		if err != nil {
			return nil, err
		}
		params = append(params, decoded)
	}
	body, err := decodeType(w.Body)
	if err != nil {
		return nil, err
	}
	return typ.NewGeneric(w.Name, params, body), nil
}

func encodeTypeParam(t *typ.TypeParam) (*typeWire, error) {
	param, err := encodeTypeParamWire(t)
	if err != nil {
		return nil, err
	}
	return &typeWire{Kind: "typeparam", TypeParams: []typeParamWire{param}}, nil
}

func decodeTypeParam(w *typeWire) (typ.Type, error) {
	if len(w.TypeParams) != 1 {
		return nil, fmt.Errorf("typeparam payload has %d params", len(w.TypeParams))
	}
	return decodeTypeParamWire(w.TypeParams[0])
}

func encodeTypeParamWire(p *typ.TypeParam) (typeParamWire, error) {
	if p == nil {
		return typeParamWire{}, fmt.Errorf("nil type parameter")
	}
	constraint, err := encodeType(p.Constraint)
	if err != nil {
		return typeParamWire{}, err
	}
	return typeParamWire{Name: p.Name, Constraint: constraint}, nil
}

func decodeTypeParamWire(w typeParamWire) (*typ.TypeParam, error) {
	constraint, err := decodeType(w.Constraint)
	if err != nil {
		return nil, err
	}
	return typ.NewTypeParam(w.Name, constraint), nil
}

func encodeAnnotations(annotations []annotation.Annotation) ([]annotationWire, error) {
	if len(annotations) == 0 {
		return nil, nil
	}
	out := make([]annotationWire, 0, len(annotations))
	for _, ann := range annotations {
		encoded := annotationWire{Name: ann.Name}
		switch v := ann.Arg.(type) {
		case nil:
			encoded.Kind = "nil"
		case string:
			encoded.Kind = "string"
			encoded.String = &v
		case bool:
			encoded.Kind = "bool"
			encoded.Bool = &v
		case int:
			encoded.Kind = "int"
			encoded.Int = &v
		case int64:
			encoded.Kind = "int64"
			encoded.Int64 = &v
		case float64:
			encoded.Kind = "float64"
			encoded.Float64 = &v
		default:
			return nil, fmt.Errorf("annotation %q has unsupported arg %T", ann.Name, ann.Arg)
		}
		out = append(out, encoded)
	}
	return out, nil
}

func decodeAnnotations(nodes []annotationWire) ([]annotation.Annotation, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	out := make([]annotation.Annotation, 0, len(nodes))
	for _, node := range nodes {
		var arg any
		switch node.Kind {
		case "", "nil":
			arg = nil
		case "string":
			if node.String == nil {
				return nil, fmt.Errorf("annotation %q missing string arg", node.Name)
			}
			arg = *node.String
		case "bool":
			if node.Bool == nil {
				return nil, fmt.Errorf("annotation %q missing bool arg", node.Name)
			}
			arg = *node.Bool
		case "int":
			if node.Int == nil {
				return nil, fmt.Errorf("annotation %q missing int arg", node.Name)
			}
			arg = *node.Int
		case "int64":
			if node.Int64 == nil {
				return nil, fmt.Errorf("annotation %q missing int64 arg", node.Name)
			}
			arg = *node.Int64
		case "float64":
			if node.Float64 == nil {
				return nil, fmt.Errorf("annotation %q missing float64 arg", node.Name)
			}
			arg = *node.Float64
		default:
			return nil, fmt.Errorf("annotation %q has unknown arg kind %q", node.Name, node.Kind)
		}
		out = append(out, annotation.Annotation{Name: node.Name, Arg: arg})
	}
	return out, nil
}
