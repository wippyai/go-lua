package manifest

import (
	"fmt"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

// typeEncoder owns one sealed type projection. Generic and recursive
// declarations can form cycles or shared DAGs in memory while the manifest
// wire remains a tree. Binder-local references preserve those graph edges
// without leaking process-local type IDs into deterministic manifest bytes.
type typeEncoder struct {
	activeGenerics    map[*typ.Generic]bool
	recursiveBinders  map[*typ.Recursive]uint64
	nextBinder        uint64
	semanticFunctions bool
}

func encodeType(t typ.Type) (*typeWire, error) {
	if t != nil {
		if err := typ.ValidateStaticGenericRecurrence(t); err != nil {
			return nil, fmt.Errorf("manifest: invalid static generic recurrence: %w", err)
		}
	}
	return (&typeEncoder{
		activeGenerics:   make(map[*typ.Generic]bool),
		recursiveBinders: make(map[*typ.Recursive]uint64),
	}).encode(t)
}

// encodeSemanticType projects function parameter presentation out of every
// function node while preserving the receiver convention observed by
// typ.TypeEquals. It is for semantic content identity, not manifest display.
func encodeSemanticType(t typ.Type) (*typeWire, error) {
	if t != nil {
		if err := typ.ValidateStaticGenericRecurrence(t); err != nil {
			return nil, fmt.Errorf("manifest: invalid static generic recurrence: %w", err)
		}
	}
	return (&typeEncoder{
		activeGenerics:    make(map[*typ.Generic]bool),
		recursiveBinders:  make(map[*typ.Recursive]uint64),
		semanticFunctions: true,
	}).encode(t)
}

func (e *typeEncoder) encode(t typ.Type) (*typeWire, error) {
	if t == nil {
		return nil, nil
	}
	if e.semanticFunctions {
		if alias, ok := t.(*typ.Alias); ok {
			target := alias.UnaliasedTarget()
			if target == nil || target == t {
				return nil, fmt.Errorf("manifest: semantic alias %q has no acyclic target", alias.Name)
			}
			return e.encode(target)
		}
	}

	switch tt := t.(type) {
	case *typ.Annotated:
		inner, err := e.encode(tt.Inner)
		if err != nil {
			return nil, err
		}
		annotations, err := encodeAnnotations(tt.Annotations)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "annotated", Element: inner, Annotations: annotations}, nil
	case *typ.Optional:
		inner, err := e.encode(tt.Inner)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "optional", Element: inner}, nil
	case *typ.Union:
		members, err := e.encodeTypeList(tt.Members)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "union", Members: members}, nil
	case *typ.Intersection:
		members, err := e.encodeTypeList(tt.Members)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "intersection", Members: members}, nil
	case *typ.Tuple:
		elements, err := e.encodeTypeList(tt.Elements)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "tuple", Elements: elements}, nil
	case *typ.Function:
		return e.encodeFunction(tt)
	case *typ.Array:
		elem, err := e.encode(tt.Element)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "array", Element: elem}, nil
	case *typ.Map:
		key, err := e.encode(tt.Key)
		if err != nil {
			return nil, err
		}
		value, err := e.encode(tt.Value)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "map", Key: key, Value: value}, nil
	case *typ.ReadonlyMap:
		key, err := e.encode(tt.Key)
		if err != nil {
			return nil, err
		}
		value, err := e.encode(tt.Value)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "readonlyMap", Key: key, Value: value}, nil
	case *typ.Record:
		return e.encodeRecord(tt)
	case *typ.Interface:
		return e.encodeInterface(tt)
	case *typ.Alias:
		target, err := e.encode(tt.Target)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "alias", Name: tt.Name, Target: target}, nil
	case *typ.Generic:
		if e.activeGenerics[tt] {
			if tt.Name == "" {
				return nil, fmt.Errorf("recursive anonymous generic requires a stable name")
			}
			return &typeWire{Kind: "genericRef", Name: tt.Name}, nil
		}
		e.activeGenerics[tt] = true
		defer delete(e.activeGenerics, tt)
		return e.encodeGeneric(tt)
	case *typ.Instantiated:
		generic, err := e.encode(tt.Generic)
		if err != nil {
			return nil, err
		}
		args, err := e.encodeTypeList(tt.TypeArgs)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "instantiated", Generic: generic, TypeArgs: args}, nil
	case *typ.Literal:
		return encodeLiteral(tt)
	case *typ.Ref:
		return &typeWire{Kind: "ref", Module: tt.Module, Name: tt.Name}, nil
	case *typ.Meta:
		of, err := e.encode(tt.Of)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "meta", Of: of}, nil
	case *typ.TypeParam:
		return e.encodeTypeParam(tt)
	case *typ.Recursive:
		if binder, ok := e.recursiveBinders[tt]; ok {
			return &typeWire{Kind: "recursiveRef", Binder: binder}, nil
		}
		e.nextBinder++
		binder := e.nextBinder
		e.recursiveBinders[tt] = binder
		body, err := e.encode(tt.Body)
		if err != nil {
			return nil, err
		}
		return &typeWire{Kind: "recursive", Binder: binder, Name: tt.Name, Body: body}, nil
	default:
		return encodePrimitive(t)
	}
}

type typeDecodeEnv struct {
	parent     *typeDecodeEnv
	params     map[string]*typ.TypeParam
	generics   map[string]*typ.Generic
	recursives map[uint64]*typ.Recursive
}

func (e *typeDecodeEnv) lookupRecursive(binder uint64) (*typ.Recursive, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		recursive, ok := cur.recursives[binder]
		if ok {
			return recursive, true
		}
	}
	return nil, false
}

func (e *typeDecodeEnv) recursiveRegistry() map[uint64]*typ.Recursive {
	for cur := e; cur != nil; cur = cur.parent {
		if cur.recursives != nil {
			return cur.recursives
		}
	}
	return nil
}

func (e *typeDecodeEnv) lookupGeneric(name string) (*typ.Generic, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		generic, ok := cur.generics[name]
		if ok {
			return generic, true
		}
	}
	return nil, false
}

func (e *typeDecodeEnv) lookup(name string) (*typ.TypeParam, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if cur.params == nil {
			continue
		}
		param, ok := cur.params[name]
		if ok {
			return param, true
		}
	}
	return nil, false
}

func (e *typeDecodeEnv) withParams(params []*typ.TypeParam) *typeDecodeEnv {
	if len(params) == 0 {
		return e
	}
	child := &typeDecodeEnv{parent: e, params: make(map[string]*typ.TypeParam, len(params))}
	for _, param := range params {
		if param == nil || param.Name == "" {
			continue
		}
		child.params[param.Name] = param
	}
	return child
}

func (e *typeDecodeEnv) withGeneric(generic *typ.Generic) *typeDecodeEnv {
	if generic == nil || generic.Name == "" {
		return e
	}
	return &typeDecodeEnv{
		parent:   e,
		generics: map[string]*typ.Generic{generic.Name: generic},
	}
}

func decodeType(w *typeWire) (typ.Type, error) {
	decoded, err := decodeTypeInEnv(w, &typeDecodeEnv{recursives: make(map[uint64]*typ.Recursive)})
	if err != nil || decoded == nil {
		return decoded, err
	}
	if err := typ.ValidateStaticGenericRecurrence(decoded); err != nil {
		return nil, fmt.Errorf("invalid static generic recurrence: %w", err)
	}
	return decoded, nil
}

func decodeTypeInEnv(w *typeWire, env *typeDecodeEnv) (typ.Type, error) {
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
		inner, err := decodeTypeInEnv(w.Element, env)
		if err != nil {
			return nil, err
		}
		annotations, err := decodeAnnotations(w.Annotations)
		if err != nil {
			return nil, err
		}
		return typ.NewAnnotated(inner, annotations), nil
	case "optional":
		inner, err := decodeTypeInEnv(w.Element, env)
		if err != nil {
			return nil, err
		}
		return typeexpr.Optional(inner), nil
	case "union":
		members, err := decodeTypeListInEnv(w.Members, env)
		if err != nil {
			return nil, err
		}
		return typeexpr.Union(members...), nil
	case "intersection":
		members, err := decodeTypeListInEnv(w.Members, env)
		if err != nil {
			return nil, err
		}
		return typeexpr.Intersection(members...), nil
	case "tuple":
		elements, err := decodeTypeListInEnv(w.Elements, env)
		if err != nil {
			return nil, err
		}
		return typ.NewTuple(elements...), nil
	case "function":
		return decodeFunctionInEnv(w, env)
	case "array":
		elem, err := decodeTypeInEnv(w.Element, env)
		if err != nil {
			return nil, err
		}
		return typ.NewArray(elem), nil
	case "map":
		if w.Key == nil {
			return nil, fmt.Errorf("map key missing type")
		}
		if w.Value == nil {
			return nil, fmt.Errorf("map value missing type")
		}
		key, err := decodeTypeInEnv(w.Key, env)
		if err != nil {
			return nil, err
		}
		value, err := decodeTypeInEnv(w.Value, env)
		if err != nil {
			return nil, err
		}
		return typetable.NewMap(key, value), nil
	case "readonlyMap":
		if w.Key == nil {
			return nil, fmt.Errorf("readonly map key missing type")
		}
		if w.Value == nil {
			return nil, fmt.Errorf("readonly map value missing type")
		}
		key, err := decodeTypeInEnv(w.Key, env)
		if err != nil {
			return nil, err
		}
		value, err := decodeTypeInEnv(w.Value, env)
		if err != nil {
			return nil, err
		}
		return typetable.NewReadonlyMap(key, value), nil
	case "record":
		return decodeRecordInEnv(w, env)
	case "interface":
		return decodeInterfaceInEnv(w, env)
	case "alias":
		target, err := decodeTypeInEnv(w.Target, env)
		if err != nil {
			return nil, err
		}
		return typ.NewAlias(w.Name, target), nil
	case "generic":
		return decodeGenericInEnv(w, env)
	case "genericRef":
		generic, ok := env.lookupGeneric(w.Name)
		if !ok {
			return nil, fmt.Errorf("generic reference %q is out of scope", w.Name)
		}
		return generic, nil
	case "instantiated":
		generic, err := decodeTypeInEnv(w.Generic, env)
		if err != nil {
			return nil, err
		}
		g, ok := generic.(*typ.Generic)
		if !ok {
			return nil, fmt.Errorf("instantiated generic payload is %T", generic)
		}
		args, err := decodeTypeListInEnv(w.TypeArgs, env)
		if err != nil {
			return nil, err
		}
		return typ.Instantiate(g, args...), nil
	case "ref":
		return typ.NewRef(w.Module, w.Name), nil
	case "meta":
		of, err := decodeTypeInEnv(w.Of, env)
		if err != nil {
			return nil, err
		}
		return typ.NewMeta(of), nil
	case "typeparam":
		return decodeTypeParamInEnv(w, env)
	case "recursive":
		if w.Binder == 0 {
			return nil, fmt.Errorf("recursive type %q is missing a binder", w.Name)
		}
		registry := env.recursiveRegistry()
		if registry == nil {
			return nil, fmt.Errorf("recursive type %q has no decode registry", w.Name)
		}
		if _, exists := registry[w.Binder]; exists {
			return nil, fmt.Errorf("recursive binder %d is already in scope", w.Binder)
		}
		recursive := typ.NewRecursivePlaceholder(w.Name)
		registry[w.Binder] = recursive
		body, err := decodeTypeInEnv(w.Body, env)
		if err != nil {
			delete(registry, w.Binder)
			return nil, err
		}
		if body == nil {
			delete(registry, w.Binder)
			return nil, fmt.Errorf("recursive type %q is missing a body", w.Name)
		}
		recursive.SetBody(body)
		return recursive, nil
	case "recursiveRef":
		if w.Binder == 0 {
			return nil, fmt.Errorf("recursive reference is missing a binder")
		}
		recursive, ok := env.lookupRecursive(w.Binder)
		if !ok {
			return nil, fmt.Errorf("recursive reference %d is out of scope", w.Binder)
		}
		return recursive, nil
	default:
		return nil, fmt.Errorf("unknown type kind %q", w.Kind)
	}
}
