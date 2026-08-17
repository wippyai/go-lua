package wire

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/type/annotation"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func (e *typeEncoder) encodeFunction(f *typ.Function) (*TypeWire, error) {
	out := &TypeWire{Kind: "function"}
	for _, p := range f.TypeParams {
		encoded, err := e.encodeTypeParamWire(p)
		if err != nil {
			return nil, err
		}
		out.TypeParams = append(out.TypeParams, encoded)
	}
	for _, p := range f.Params {
		encoded, err := e.encode(p.Type)
		if err != nil {
			return nil, err
		}
		name := p.Name
		if e.semanticFunctions {
			name = ""
			if p.Receiver {
				name = "self"
			}
		}
		out.Params = append(out.Params, paramWire{Name: name, Type: encoded, Optional: p.Optional})
	}
	var err error
	out.Variadic, err = e.encode(f.Variadic)
	if err != nil {
		return nil, err
	}
	out.Returns, err = e.encodeTypeList(f.Returns)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeFunctionInEnv(w *TypeWire, env *typeDecodeEnv) (typ.Type, error) {
	b := typ.Func()
	var typeParams []*typ.TypeParam
	for _, p := range w.TypeParams {
		decoded, err := decodeTypeParamWireInEnv(p, env)
		if err != nil {
			return nil, err
		}
		typeParams = append(typeParams, decoded)
		b.TypeParamRef(decoded)
	}
	bodyEnv := env.withParams(typeParams)
	for index, p := range w.Params {
		if p.Type == nil {
			return nil, fmt.Errorf("function parameter %d missing type", index)
		}
		t, err := decodeTypeInEnv(p.Type, bodyEnv)
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
		v, err := decodeTypeInEnv(w.Variadic, bodyEnv)
		if err != nil {
			return nil, err
		}
		b.Variadic(v)
	}
	returns, err := decodeTypeListInEnv(w.Returns, bodyEnv)
	if err != nil {
		return nil, err
	}
	if len(returns) > 0 {
		b.Returns(returns...)
	}
	return b.Build(), nil
}

func (e *typeEncoder) encodeInterface(i *typ.Interface) (*TypeWire, error) {
	out := &TypeWire{Kind: "interface", Name: i.Name}
	methods := append([]typ.Method(nil), i.Methods...)
	sort.Slice(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })
	for _, method := range methods {
		encoded, err := e.encode(method.Type)
		if err != nil {
			return nil, err
		}
		out.Methods = append(out.Methods, methodWire{Name: method.Name, Type: encoded})
	}
	return out, nil
}

func decodeInterfaceInEnv(w *TypeWire, env *typeDecodeEnv) (typ.Type, error) {
	methods := make([]typ.Method, 0, len(w.Methods))
	for _, method := range w.Methods {
		t, err := decodeTypeInEnv(method.Type, env)
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

func (e *typeEncoder) encodeGeneric(g *typ.Generic) (*TypeWire, error) {
	out := &TypeWire{Kind: "generic", Name: g.Name}
	for _, p := range g.TypeParams {
		encoded, err := e.encodeTypeParamWire(p)
		if err != nil {
			return nil, err
		}
		out.TypeParams = append(out.TypeParams, encoded)
	}
	var err error
	out.Body, err = e.encode(g.Body)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeGenericInEnv(w *TypeWire, env *typeDecodeEnv) (typ.Type, error) {
	params := make([]*typ.TypeParam, 0, len(w.TypeParams))
	for _, p := range w.TypeParams {
		decoded, err := decodeTypeParamWireInEnv(p, env)
		if err != nil {
			return nil, err
		}
		params = append(params, decoded)
	}
	generic := typ.NewGeneric(w.Name, params, nil)
	body, err := decodeTypeInEnv(w.Body, env.withGeneric(generic).withParams(params))
	if err != nil {
		return nil, err
	}
	generic.SetBody(body)
	return generic, nil
}

func (e *typeEncoder) encodeTypeParam(t *typ.TypeParam) (*TypeWire, error) {
	param, err := e.encodeTypeParamWire(t)
	if err != nil {
		return nil, err
	}
	return &TypeWire{Kind: "typeparam", TypeParams: []typeParamWire{param}}, nil
}

func decodeTypeParamInEnv(w *TypeWire, env *typeDecodeEnv) (typ.Type, error) {
	if len(w.TypeParams) != 1 {
		return nil, fmt.Errorf("typeparam payload has %d params", len(w.TypeParams))
	}
	param := w.TypeParams[0]
	if envParam, ok := env.lookup(param.Name); ok {
		return envParam, nil
	}
	return decodeTypeParamWireInEnv(param, env)
}

func (e *typeEncoder) encodeTypeParamWire(p *typ.TypeParam) (typeParamWire, error) {
	if p == nil {
		return typeParamWire{}, fmt.Errorf("nil type parameter")
	}
	constraint, err := e.encode(p.Constraint)
	if err != nil {
		return typeParamWire{}, err
	}
	return typeParamWire{Name: p.Name, Constraint: constraint}, nil
}

func decodeTypeParamWireInEnv(w typeParamWire, env *typeDecodeEnv) (*typ.TypeParam, error) {
	constraint, err := decodeTypeInEnv(w.Constraint, env)
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
		if ann.Name == "" {
			return nil, fmt.Errorf("annotation missing name")
		}
		encoded := annotationWire{Name: ann.Name}
		if ann.Arg.IsNone() {
			encoded.Kind = "nil"
		} else if v, ok := ann.Arg.AsString(); ok {
			encoded.Kind = "string"
			encoded.String = &v
		} else if v, ok := ann.Arg.AsBool(); ok {
			encoded.Kind = "bool"
			encoded.Bool = &v
		} else if v, ok := ann.Arg.AsInt(); ok {
			encoded.Kind = "int"
			encoded.Int = &v
		} else if v, ok := ann.Arg.AsInt64(); ok {
			encoded.Kind = "int64"
			encoded.Int64 = &v
		} else if v, ok := ann.Arg.AsFloat64(); ok {
			encoded.Kind = "float64"
			encoded.Float64 = &v
		} else {
			return nil, fmt.Errorf("annotation %q has unsupported arg", ann.Name)
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
		if node.Name == "" {
			return nil, fmt.Errorf("annotation missing name")
		}
		var payload annotation.Payload
		switch node.Kind {
		case "nil":
			payload = annotation.Payload{}
		case "":
			return nil, fmt.Errorf("annotation %q missing arg kind", node.Name)
		case "string":
			if node.String == nil {
				return nil, fmt.Errorf("annotation %q missing string arg", node.Name)
			}
			payload = annotation.StringArg(*node.String)
		case "bool":
			if node.Bool == nil {
				return nil, fmt.Errorf("annotation %q missing bool arg", node.Name)
			}
			payload = annotation.BoolArg(*node.Bool)
		case "int":
			if node.Int == nil {
				return nil, fmt.Errorf("annotation %q missing int arg", node.Name)
			}
			payload = annotation.IntArg(*node.Int)
		case "int64":
			if node.Int64 == nil {
				return nil, fmt.Errorf("annotation %q missing int64 arg", node.Name)
			}
			payload = annotation.Int64Arg(*node.Int64)
		case "float64":
			if node.Float64 == nil {
				return nil, fmt.Errorf("annotation %q missing float64 arg", node.Name)
			}
			payload = annotation.Float64Arg(*node.Float64)
		default:
			return nil, fmt.Errorf("annotation %q has unknown arg kind %q", node.Name, node.Kind)
		}
		out = append(out, annotation.Annotation{Name: node.Name, Arg: payload})
	}
	return out, nil
}
