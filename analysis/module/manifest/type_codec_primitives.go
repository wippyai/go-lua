package manifest

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
	b := typetable.NewRecord().SetOpen(w.Open)
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
