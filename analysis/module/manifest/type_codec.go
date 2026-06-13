package manifest

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
