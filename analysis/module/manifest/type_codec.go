package manifest

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type typeWire struct {
	Kind string `json:"kind"`

	Base   string   `json:"base,omitempty"`
	Bool   *bool    `json:"bool,omitempty"`
	Int    *int64   `json:"int,omitempty"`
	Number *float64 `json:"number,omitempty"`
	String *string  `json:"string,omitempty"`

	Element  *typeWire   `json:"element,omitempty"`
	Key      *typeWire   `json:"key,omitempty"`
	Value    *typeWire   `json:"value,omitempty"`
	Elements []*typeWire `json:"elements,omitempty"`
	Members  []*typeWire `json:"members,omitempty"`

	Fields        []fieldWire        `json:"fields,omitempty"`
	StaticMembers []staticMemberWire `json:"staticMembers,omitempty"`
	Metatable     *typeWire          `json:"metatable,omitempty"`
	MapKey        *typeWire          `json:"mapKey,omitempty"`
	MapValue      *typeWire          `json:"mapValue,omitempty"`
	Open          bool               `json:"open,omitempty"`

	TypeParams []typeParamWire `json:"typeParams,omitempty"`
	Params     []paramWire     `json:"params,omitempty"`
	Variadic   *typeWire       `json:"variadic,omitempty"`
	Returns    []*typeWire     `json:"returns,omitempty"`

	Module string    `json:"module,omitempty"`
	Name   string    `json:"name,omitempty"`
	Target *typeWire `json:"target,omitempty"`
	Of     *typeWire `json:"of,omitempty"`

	Body     *typeWire   `json:"body,omitempty"`
	Generic  *typeWire   `json:"generic,omitempty"`
	TypeArgs []*typeWire `json:"typeArgs,omitempty"`

	Annotations []annotationWire `json:"annotations,omitempty"`
	Methods     []methodWire     `json:"methods,omitempty"`
}

type fieldWire struct {
	Name     string    `json:"name"`
	Type     *typeWire `json:"type,omitempty"`
	Optional bool      `json:"optional,omitempty"`
	Readonly bool      `json:"readonly,omitempty"`
}

type staticMemberWire struct {
	Kind     string    `json:"kind"`
	Name     string    `json:"name,omitempty"`
	Index    int64     `json:"index,omitempty"`
	Type     *typeWire `json:"type,omitempty"`
	Optional bool      `json:"optional,omitempty"`
	Readonly bool      `json:"readonly,omitempty"`
}

type typeParamWire struct {
	Name       string    `json:"name"`
	Constraint *typeWire `json:"constraint,omitempty"`
}

type paramWire struct {
	Name     string    `json:"name,omitempty"`
	Type     *typeWire `json:"type,omitempty"`
	Optional bool      `json:"optional,omitempty"`
}

type annotationWire struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind,omitempty"`
	String  *string  `json:"string,omitempty"`
	Bool    *bool    `json:"bool,omitempty"`
	Int     *int     `json:"int,omitempty"`
	Int64   *int64   `json:"int64,omitempty"`
	Float64 *float64 `json:"float64,omitempty"`
}

type methodWire struct {
	Name string    `json:"name"`
	Type *typeWire `json:"type,omitempty"`
}

func encodeType(t typ.Type) (*typeWire, error) {
	if t == nil {
		return nil, nil
	}

	switch tt := t.(type) {
	case *typ.Annotated:
		inner, err := encodeType(tt.Inner)
		if err != nil {
			return nil, err
		}
		annotations, err := encodeAnnotations(tt.Annotations)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "annotated", Element: inner, Annotations: annotations}, nil
	case *typ.Optional:
		inner, err := encodeType(tt.Inner)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "optional", Element: inner}, nil
	case *typ.Union:
		members, err := encodeTypeList(tt.Members)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "union", Members: members}, nil
	case *typ.Intersection:
		members, err := encodeTypeList(tt.Members)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "intersection", Members: members}, nil
	case *typ.Tuple:
		elements, err := encodeTypeList(tt.Elements)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "tuple", Elements: elements}, nil
	case *typ.Function:
		return encodeFunction(tt)
	case *typ.Array:
		elem, err := encodeType(tt.Element)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "array", Element: elem}, nil
	case *typ.Map:
		key, err := encodeType(tt.Key)
		if err != nil {
			return nil, err
		}
		value, err := encodeType(tt.Value)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "map", Key: key, Value: value}, nil
	case *typ.ReadonlyMap:
		key, err := encodeType(tt.Key)
		if err != nil {
			return nil, err
		}
		value, err := encodeType(tt.Value)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "readonlyMap", Key: key, Value: value}, nil
	case *typ.Record:
		return encodeRecord(tt)
	case *typ.Interface:
		return encodeInterface(tt)
	case *typ.Alias:
		target, err := encodeType(tt.Target)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "alias", Name: tt.Name, Target: target}, nil
	case *typ.Generic:
		return encodeGeneric(tt)
	case *typ.Instantiated:
		generic, err := encodeType(tt.Generic)
		if err != nil {
			return nil, err
		}
		args, err := encodeTypeList(tt.TypeArgs)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "instantiated", Generic: generic, TypeArgs: args}, nil
	case *typ.Literal:
		return encodeLiteral(tt)
	case *typ.Ref:
		return &typeWire{Kind: "ref", Module: tt.Module, Name: tt.Name}, nil
	case *typ.Meta:
		of, err := encodeType(tt.Of)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "meta", Of: of}, nil
	case *typ.TypeParam:
		return encodeTypeParam(tt)
	case *typ.Recursive:
		return nil, fmt.Errorf("recursive type %q requires recursive-family manifest encoding", tt.Name)
	default:
		return encodePrimitive(t)
	}
}

func decodeType(w *typeWire) (typ.Type, error) {
	if w == nil {
		return nil, nil
	}

	switch w.Kind {
	case "nil":
		return typ.Nil, nil
	case "boolean":
		return typ.Boolean, nil
	case "number":
		return typ.Number, nil
	case "integer":
		return typ.Integer, nil
	case "string":
		return typ.String, nil
	case "any":
		return typ.Any, nil
	case "unknown":
		return typ.Unknown, nil
	case "never":
		return typ.Never, nil
	case "self":
		return typ.Self, nil
	case "literal":
		return decodeLiteral(w)
	case "annotated":
		inner, err := decodeType(w.Element)
		if err != nil {
			return nil, err
		}
		annotations, err := decodeAnnotations(w.Annotations)
		if err != nil {
			return nil, err
		}
		return typ.NewAnnotated(inner, annotations), nil
	case "optional":
		inner, err := decodeType(w.Element)
		if err != nil {
			return nil, err
		}
		return typ.NewOptional(inner), nil
	case "union":
		members, err := decodeTypeList(w.Members)
		if err != nil {
			return nil, err
		}
		return typ.NewUnion(members...), nil
	case "intersection":
		members, err := decodeTypeList(w.Members)
		if err != nil {
			return nil, err
		}
		return typ.NewIntersection(members...), nil
	case "tuple":
		elements, err := decodeTypeList(w.Elements)
		if err != nil {
			return nil, err
		}
		return typ.NewTuple(elements...), nil
	case "function":
		return decodeFunction(w)
	case "array":
		elem, err := decodeType(w.Element)
		if err != nil {
			return nil, err
		}
		return typ.NewArray(elem), nil
	case "map":
		key, err := decodeType(w.Key)
		if err != nil {
			return nil, err
		}
		value, err := decodeType(w.Value)
		if err != nil {
			return nil, err
		}
		return typ.NewMap(key, value), nil
	case "readonlyMap":
		key, err := decodeType(w.Key)
		if err != nil {
			return nil, err
		}
		value, err := decodeType(w.Value)
		if err != nil {
			return nil, err
		}
		return typ.NewReadonlyMap(key, value), nil
	case "record":
		return decodeRecord(w)
	case "interface":
		return decodeInterface(w)
	case "alias":
		target, err := decodeType(w.Target)
		if err != nil {
			return nil, err
		}
		return typ.NewAlias(w.Name, target), nil
	case "generic":
		return decodeGeneric(w)
	case "instantiated":
		generic, err := decodeType(w.Generic)
		if err != nil {
			return nil, err
		}
		g, ok := generic.(*typ.Generic)
		if !ok {
			return nil, fmt.Errorf("instantiated generic payload is %T", generic)
		}
		args, err := decodeTypeList(w.TypeArgs)
		if err != nil {
			return nil, err
		}
		return typ.Instantiate(g, args...), nil
	case "ref":
		return typ.NewRef(w.Module, w.Name), nil
	case "meta":
		of, err := decodeType(w.Of)
		if err != nil {
			return nil, err
		}
		return typ.NewMeta(of), nil
	case "typeparam":
		return decodeTypeParam(w)
	default:
		return nil, fmt.Errorf("unknown type kind %q", w.Kind)
	}
}

func encodePrimitive(t typ.Type) (*typeWire, error) {
	switch t.Kind() {
	case kind.Nil:
		return &typeWire{Kind: "nil"}, nil
	case kind.Boolean:
		return &typeWire{Kind: "boolean"}, nil
	case kind.Number:
		return &typeWire{Kind: "number"}, nil
	case kind.Integer:
		return &typeWire{Kind: "integer"}, nil
	case kind.String:
		return &typeWire{Kind: "string"}, nil
	case kind.Any:
		return &typeWire{Kind: "any"}, nil
	case kind.Unknown:
		return &typeWire{Kind: "unknown"}, nil
	case kind.Never:
		return &typeWire{Kind: "never"}, nil
	case kind.Self:
		return &typeWire{Kind: "self"}, nil
	default:
		return nil, fmt.Errorf("unsupported type node %T", t)
	}
}

func encodeLiteral(l *typ.Literal) (*typeWire, error) {
	out := &typeWire{Kind: "literal", Base: l.Base.String()}
	switch l.Base {
	case kind.Boolean:
		v, ok := l.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("literal boolean has %T", l.Value)
		}
		out.Bool = &v
	case kind.Integer:
		v, ok := l.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("literal integer has %T", l.Value)
		}
		out.Int = &v
	case kind.Number:
		v, ok := l.Value.(float64)
		if !ok {
			return nil, fmt.Errorf("literal number has %T", l.Value)
		}
		out.Number = &v
	case kind.String:
		v, ok := l.Value.(string)
		if !ok {
			return nil, fmt.Errorf("literal string has %T", l.Value)
		}
		out.String = &v
	default:
		return nil, fmt.Errorf("unsupported literal base %s", l.Base)
	}
	return out, nil
}

func decodeLiteral(w *typeWire) (typ.Type, error) {
	switch w.Base {
	case "boolean":
		if w.Bool == nil {
			return nil, fmt.Errorf("literal boolean missing value")
		}
		return typ.LiteralBool(*w.Bool), nil
	case "integer":
		if w.Int == nil {
			return nil, fmt.Errorf("literal integer missing value")
		}
		return typ.LiteralInt(*w.Int), nil
	case "number":
		if w.Number == nil {
			return nil, fmt.Errorf("literal number missing value")
		}
		return typ.LiteralNumber(*w.Number), nil
	case "string":
		if w.String == nil {
			return nil, fmt.Errorf("literal string missing value")
		}
		return typ.LiteralString(*w.String), nil
	default:
		return nil, fmt.Errorf("unknown literal base %q", w.Base)
	}
}

func encodeTypeList(types []typ.Type) ([]*typeWire, error) {
	if len(types) == 0 {
		return nil, nil
	}
	out := make([]*typeWire, 0, len(types))
	for _, t := range types {
		encoded, err := encodeType(t)
		if err != nil {
			return nil, err
		}
		out = append(out, encoded)
	}
	return out, nil
}

func decodeTypeList(nodes []*typeWire) ([]typ.Type, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	out := make([]typ.Type, 0, len(nodes))
	for _, node := range nodes {
		decoded, err := decodeType(node)
		if err != nil {
			return nil, err
		}
		out = append(out, decoded)
	}
	return out, nil
}

func encodeRecord(r *typ.Record) (*typeWire, error) {
	out := &typeWire{Kind: "record", Open: r.Open}

	fields := append([]typ.Field(nil), r.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for _, f := range fields {
		encoded, err := encodeType(f.Type)
		if err != nil {
			return nil, err
		}
		out.Fields = append(out.Fields, fieldWire{
			Name:     f.Name,
			Type:     encoded,
			Optional: f.Optional,
			Readonly: f.Readonly,
		})
	}

	members := append([]typ.StaticMember(nil), r.StaticMembers...)
	sort.Slice(members, func(i, j int) bool {
		return typ.CompareStaticMembers(members[i], members[j]) < 0
	})
	for _, member := range members {
		encoded, err := encodeType(member.Type)
		if err != nil {
			return nil, err
		}
		out.StaticMembers = append(out.StaticMembers, staticMemberWire{
			Kind:     encodeStaticMemberKind(member.Kind),
			Name:     member.Name,
			Index:    member.Index,
			Type:     encoded,
			Optional: member.Optional,
			Readonly: member.Readonly,
		})
	}

	var err error
	out.Metatable, err = encodeType(r.Metatable)
	if err != nil {
		return nil, err
	}
	out.MapKey, err = encodeType(r.MapKey)
	if err != nil {
		return nil, err
	}
	out.MapValue, err = encodeType(r.MapValue)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeRecord(w *typeWire) (typ.Type, error) {
	b := typ.NewRecord().SetOpen(w.Open)
	for _, field := range w.Fields {
		t, err := decodeType(field.Type)
		if err != nil {
			return nil, err
		}
		switch {
		case field.Optional && field.Readonly:
			b.OptReadonlyField(field.Name, t)
		case field.Optional:
			b.OptField(field.Name, t)
		case field.Readonly:
			b.ReadonlyField(field.Name, t)
		default:
			b.Field(field.Name, t)
		}
	}
	for _, member := range w.StaticMembers {
		t, err := decodeType(member.Type)
		if err != nil {
			return nil, err
		}
		kind, err := decodeStaticMemberKind(member.Kind)
		if err != nil {
			return nil, err
		}
		b.AddStaticMember(typ.StaticMember{
			Kind:     kind,
			Name:     member.Name,
			Index:    member.Index,
			Type:     t,
			Optional: member.Optional,
			Readonly: member.Readonly,
		})
	}
	if w.Metatable != nil {
		mt, err := decodeType(w.Metatable)
		if err != nil {
			return nil, err
		}
		b.Metatable(mt)
	}
	if w.MapKey != nil || w.MapValue != nil {
		key, err := decodeType(w.MapKey)
		if err != nil {
			return nil, err
		}
		value, err := decodeType(w.MapValue)
		if err != nil {
			return nil, err
		}
		b.MapComponent(key, value)
	}
	return b.Build(), nil
}

func encodeStaticMemberKind(k typ.StaticMemberKind) string {
	switch k {
	case typ.StaticMemberStringIndex:
		return "string"
	case typ.StaticMemberIntIndex:
		return "int"
	default:
		return "unknown"
	}
}

func decodeStaticMemberKind(s string) (typ.StaticMemberKind, error) {
	switch s {
	case "string":
		return typ.StaticMemberStringIndex, nil
	case "int":
		return typ.StaticMemberIntIndex, nil
	default:
		return 0, fmt.Errorf("unknown static member kind %q", s)
	}
}

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
