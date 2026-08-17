package wire

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/domain/type/kind"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func encodePrimitive(t typ.Type) (*TypeWire, error) {
	switch t.Kind() {
	case kind.Nil:
		return &TypeWire{Kind: "nil"}, nil
	case kind.Boolean:
		return &TypeWire{Kind: "boolean"}, nil
	case kind.Number:
		return &TypeWire{Kind: "number"}, nil
	case kind.Integer:
		return &TypeWire{Kind: "integer"}, nil
	case kind.String:
		return &TypeWire{Kind: "string"}, nil
	case kind.Any:
		return &TypeWire{Kind: "any"}, nil
	case kind.Unknown:
		return &TypeWire{Kind: "unknown"}, nil
	case kind.Never:
		return &TypeWire{Kind: "never"}, nil
	case kind.Self:
		return &TypeWire{Kind: "self"}, nil
	default:
		return nil, fmt.Errorf("unsupported type node %T", t)
	}
}

func encodeLiteral(l *typ.Literal) (*TypeWire, error) {
	out := &TypeWire{Kind: "literal", Base: l.Base().String()}
	switch l.Base() {
	case kind.Boolean:
		v, ok := l.Value().(bool)
		if !ok {
			return nil, fmt.Errorf("literal boolean has %T", l.Value())
		}
		out.Bool = &v
	case kind.Integer:
		v, ok := l.Value().(int64)
		if !ok {
			return nil, fmt.Errorf("literal integer has %T", l.Value())
		}
		out.Int = &v
	case kind.Number:
		v, ok := l.Value().(float64)
		if !ok {
			return nil, fmt.Errorf("literal number has %T", l.Value())
		}
		out.Number = &v
	case kind.String:
		v, ok := l.Value().(string)
		if !ok {
			return nil, fmt.Errorf("literal string has %T", l.Value())
		}
		out.String = &v
	default:
		return nil, fmt.Errorf("unsupported literal base %s", l.Base())
	}
	return out, nil
}

func decodeLiteral(w *TypeWire) (typ.Type, error) {
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

func (e *typeEncoder) encodeTypeList(types []typ.Type) ([]*TypeWire, error) {
	if len(types) == 0 {
		return nil, nil
	}
	out := make([]*TypeWire, 0, len(types))
	for _, t := range types {
		encoded, err := e.encode(t)
		if err != nil {
			return nil, err
		}
		out = append(out, encoded)
	}
	return out, nil
}

func decodeTypeListInEnv(nodes []*TypeWire, env *typeDecodeEnv) ([]typ.Type, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	out := make([]typ.Type, 0, len(nodes))
	for _, node := range nodes {
		decoded, err := decodeTypeInEnv(node, env)
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

func (e *typeEncoder) encodeRecord(r *typ.Record) (*TypeWire, error) {
	out := &TypeWire{Kind: "record", Open: r.Open}

	fields := append([]typ.Field(nil), r.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for _, f := range fields {
		encoded, err := e.encode(f.Type)
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
		encoded, err := e.encode(member.Type)
		if err != nil {
			return nil, err
		}
		memberWire := staticMemberWire{
			Kind:     encodeStaticMemberKind(member.Kind),
			Type:     encoded,
			Optional: member.Optional,
			Readonly: member.Readonly,
		}
		switch member.Kind {
		case typ.StaticMemberStringIndex:
			memberWire.Name = member.Name
		case typ.StaticMemberIntIndex:
			memberWire.Index = encodeInt64(member.Index)
		}
		out.StaticMembers = append(out.StaticMembers, memberWire)
	}

	var err error
	out.Metatable, err = e.encode(r.Metatable)
	if err != nil {
		return nil, err
	}
	out.MapKey, err = e.encode(r.MapKey)
	if err != nil {
		return nil, err
	}
	out.MapValue, err = e.encode(r.MapValue)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeRecordInEnv(w *TypeWire, env *typeDecodeEnv) (typ.Type, error) {
	b := typetable.NewRecord().SetOpen(w.Open)
	for _, field := range w.Fields {
		t, err := decodeTypeInEnv(field.Type, env)
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
		t, err := decodeTypeInEnv(member.Type, env)
		if err != nil {
			return nil, err
		}
		kind, err := decodeStaticMemberKind(member.Kind)
		if err != nil {
			return nil, err
		}
		var index int64
		switch kind {
		case typ.StaticMemberIntIndex:
			index, err = decodeRequiredInt64(member.Index, "static member int index missing")
			if err != nil {
				return nil, err
			}
		}
		b.AddStaticMember(typ.StaticMember{
			Kind:     kind,
			Name:     member.Name,
			Index:    index,
			Type:     t,
			Optional: member.Optional,
			Readonly: member.Readonly,
		})
	}
	if w.Metatable != nil {
		mt, err := decodeTypeInEnv(w.Metatable, env)
		if err != nil {
			return nil, err
		}
		b.Metatable(mt)
	}
	if w.MapKey != nil || w.MapValue != nil {
		if w.MapKey == nil {
			return nil, fmt.Errorf("record map key missing type")
		}
		if w.MapValue == nil {
			return nil, fmt.Errorf("record map value missing type")
		}
		key, err := decodeTypeInEnv(w.MapKey, env)
		if err != nil {
			return nil, err
		}
		value, err := decodeTypeInEnv(w.MapValue, env)
		if err != nil {
			return nil, err
		}
		b.MapComponent(key, value)
	}
	return b.Build(), nil
}
