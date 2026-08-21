// Package typecontract is the Lua type domain's adapter for the neutral
// schema/typecontract envelope.
//
// The schema package owns only the portable envelope. This package owns every
// interpretation of its payload: authoring admission, canonical encoding and
// decoding, subtype judgments, callable admission, and fresh-value admission.
// Program and the engine must retain the envelope or a sealed handle; they do
// not inspect these bytes or recreate these judgments.
package typecontract

import (
	"context"
	"errors"
	"fmt"
	"math"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/subtype"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typecall"
)

// Admission is the callable shape required by one domain-owned operation
// row. It is intentionally not a schema vocabulary: schema stores the row's
// opaque declaration and asks this adapter to admit a decoded value.
type Admission uint8

const (
	AdmissionInvalid Admission = iota
	DirectFunction
	OrdinaryCallable
)

// FreshKind is the runtime category required by a nominal fresh-result row.
// The adapter owns the semantic check; the schema only carries the row.
type FreshKind uint8

const (
	FreshInvalid FreshKind = iota
	FreshTable
	FreshFunction
	FreshThread
	FreshUserdata
	FreshError
	FreshReflection
)

// Encode validates a domain type under formals and returns the neutral
// schema envelope. Primitive atoms use the envelope's primitive representation
// so Nil and Any (and the other closed primitive atoms) do not acquire a
// second graph codec. All composite and open types use the domain's existing
// scoped canonical graph codec.
func Encode(ctx context.Context, value typ.Type, formals []*typ.TypeParam) (schematype.Type, error) {
	formalCount, err := portableFormalCount(formals)
	if err != nil {
		return schematype.Type{}, err
	}
	if err := ValidateAuthoring(ctx, value, formals); err != nil {
		return schematype.Type{}, err
	}
	if primitive, ok := primitiveOf(value); ok {
		encoded, available := schematype.NewPrimitive(primitive)
		if !available {
			return schematype.Type{}, errors.New("typecontract: primitive envelope unavailable")
		}
		return encoded, nil
	}
	encoded, err := typ.EncodeCanonicalFormals(ctx, value, formals)
	if err != nil {
		return schematype.Type{}, fmt.Errorf("typecontract: encode: %w", err)
	}
	portable, ok := schematype.NewEncoded(encoded, formalCount)
	if !ok {
		return schematype.Type{}, errors.New("typecontract: encoded envelope unavailable")
	}
	return portable, nil
}

// EncodeStorage returns the same domain-owned canonical graph declaration as
// Encode, but always uses the encoded envelope representation. Persistent
// target rows and Snapshot columns need Type.Bytes for their existing cold
// decoder; primitive atoms therefore retain their canonical domain bytes at
// this storage boundary instead of the compact primitive spelling.
func EncodeStorage(ctx context.Context, value typ.Type, formals []*typ.TypeParam) (schematype.Type, error) {
	formalCount, err := portableFormalCount(formals)
	if err != nil {
		return schematype.Type{}, err
	}
	if err := ValidateAuthoring(ctx, value, formals); err != nil {
		return schematype.Type{}, err
	}
	encoded, err := typ.EncodeCanonicalFormals(ctx, value, formals)
	if err != nil {
		return schematype.Type{}, fmt.Errorf("typecontract: encode storage: %w", err)
	}
	portable, ok := schematype.NewEncoded(encoded, formalCount)
	if !ok {
		return schematype.Type{}, errors.New("typecontract: encoded storage envelope unavailable")
	}
	return portable, nil
}

// Decode validates and materializes a neutral envelope under exactly the
// receiver-owned external formal scope. Primitive atoms lower to the
// canonical typ singleton; graph bytes use the existing scoped decoder.
func Decode(ctx context.Context, value schematype.Type, formals []*typ.TypeParam) (typ.Type, error) {
	formalCount, err := portableFormalCount(formals)
	if err != nil {
		return nil, err
	}
	if !value.Available() {
		return nil, errors.New("typecontract: unavailable type")
	}
	if primitive, ok := value.Primitive(); ok {
		if value.ExternalFormals() != 0 {
			return nil, errors.New("typecontract: primitive has external formals")
		}
		decoded, ok := lowerPrimitive(primitive)
		if !ok {
			return nil, errors.New("typecontract: unsupported primitive")
		}
		return decoded, nil
	}
	// A closed graph has no dependency on the receiving operation's formal
	// scope. It is therefore valid under any outer scope and is decoded with a
	// nil receiver scope. Open graph bytes carry the exact external formal
	// count and must use the matching receiver authority.
	decodeFormals := formals
	if value.ExternalFormals() == 0 {
		decodeFormals = nil
	} else if value.ExternalFormals() != formalCount {
		return nil, fmt.Errorf("typecontract: external formal count %d, want %d", value.ExternalFormals(), len(formals))
	}
	encoded := value.Bytes()
	if err := typ.ValidateCanonicalFormals(encoded, len(decodeFormals)); err != nil {
		return nil, fmt.Errorf("typecontract: invalid encoded type: %w", err)
	}
	decoded, err := typ.DecodeCanonicalFormals(ctx, encoded, decodeFormals)
	if err != nil {
		return nil, fmt.Errorf("typecontract: decode: %w", err)
	}
	return decoded, nil
}

// ValidateAuthoring performs the domain-owned portable authoring admission.
// It rejects unresolved references and runtime annotations, checks all graph
// links iteratively, then delegates canonical graph validity and scope laws to
// typ's one canonical codec. No second type representation is built here.
func ValidateAuthoring(ctx context.Context, value typ.Type, formals []*typ.TypeParam) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := portableFormalCount(formals); err != nil {
		return err
	}
	if err := validateGraph(ctx, value); err != nil {
		return err
	}
	encoded, err := typ.EncodeCanonicalFormals(ctx, value, formals)
	if err != nil {
		return fmt.Errorf("typecontract: canonical authoring: %w", err)
	}
	if err := typ.ValidateCanonicalFormals(encoded, len(formals)); err != nil {
		return fmt.Errorf("typecontract: canonical authoring validation: %w", err)
	}
	return nil
}

func portableFormalCount(formals []*typ.TypeParam) (uint32, error) {
	if uint64(len(formals)) > math.MaxUint32 {
		return 0, errors.New("typecontract: formal scope exceeds portable count")
	}
	return uint32(len(formals)), nil
}

// Assignable is the domain's gradual assignment law. Exact equality and
// Never are handled before the explicit Any boundary; a destination formal is
// not treated as its constraint, because a constraint is not value identity.
func Assignable(source, destination typ.Type) bool {
	if source == nil || destination == nil {
		return false
	}
	if typ.TypeEquals(source, destination) || typ.IsNever(source) {
		return true
	}
	if typ.IsNever(destination) {
		return false
	}
	if typ.IsAny(destination) || typ.IsAny(source) {
		return true
	}
	if typ.ContainsTypeParam(destination) {
		return false
	}
	return subtype.IsSubtype(source, destination)
}

// Admits checks callable admission against a decoded domain type.
func Admits(value typ.Type, admission Admission) bool {
	if value == nil || typ.IsNever(value) {
		return false
	}
	if typ.IsAny(value) {
		return true
	}
	switch admission {
	case DirectFunction:
		return runtimeEvidence(value, directFunctionLeaf, false, false)
	case OrdinaryCallable:
		_, ok := typecall.Callable(value)
		return ok
	default:
		return false
	}
}

// FreshCompatible checks the runtime shape admitted by a fresh-result row.
func FreshCompatible(value typ.Type, fresh FreshKind) bool {
	if value == nil || typ.IsNever(value) {
		return false
	}
	switch fresh {
	case FreshTable:
		return runtimeEvidence(value, directTableLeaf, false, false)
	case FreshFunction:
		return runtimeEvidence(value, directFunctionLeaf, false, false)
	case FreshThread, FreshUserdata, FreshError, FreshReflection:
		return runtimeEvidence(value, func(value typ.Type) bool {
			if _, meta := value.(*typ.Meta); meta {
				return fresh == FreshReflection
			}
			return typ.IsAny(value) || typ.IsUnknown(value) || !knownConcreteRuntimeKind(value)
		}, true, true)
	default:
		return false
	}
}

func primitiveOf(value typ.Type) (schematype.Primitive, bool) {
	if value == nil {
		return schematype.PrimitiveInvalid, false
	}
	switch value.Kind() {
	case kind.Nil:
		return schematype.PrimitiveNil, true
	case kind.Boolean:
		return schematype.PrimitiveBoolean, true
	case kind.Number:
		return schematype.PrimitiveNumber, true
	case kind.Integer:
		return schematype.PrimitiveInteger, true
	case kind.String:
		return schematype.PrimitiveString, true
	case kind.Any:
		return schematype.PrimitiveAny, true
	case kind.Never:
		return schematype.PrimitiveNever, true
	default:
		return schematype.PrimitiveInvalid, false
	}
}

func lowerPrimitive(value schematype.Primitive) (typ.Type, bool) {
	switch value {
	case schematype.PrimitiveNil:
		return typ.Nil, true
	case schematype.PrimitiveBoolean:
		return typ.Boolean, true
	case schematype.PrimitiveNumber:
		return typ.Number, true
	case schematype.PrimitiveInteger:
		return typ.Integer, true
	case schematype.PrimitiveString:
		return typ.String, true
	case schematype.PrimitiveAny:
		return typ.Any, true
	case schematype.PrimitiveNever:
		return typ.Never, true
	default:
		return nil, false
	}
}

func validateGraph(ctx context.Context, root typ.Type) error {
	if root == nil {
		return errors.New("typecontract: nil type")
	}
	stack := []typ.Type{root}
	seen := make(map[typ.Type]struct{})
	for len(stack) != 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := typ.NormalizeNil(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
		if current == nil {
			return errors.New("typecontract: nil type child")
		}
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		switch current.(type) {
		case *typ.Annotated:
			return errors.New("typecontract: annotated type is not portable")
		case *typ.Ref:
			return errors.New("typecontract: unresolved type reference is not portable")
		}
		if current.Kind() == kind.Unknown || current.Kind() == kind.Self || current.Kind() == kind.Refined {
			return fmt.Errorf("typecontract: %s type is not portable", current.Kind())
		}
		if err := validateShape(current, &stack); err != nil {
			return err
		}
	}
	return nil
}

// validateShape checks the exported construction fields whose nilness is
// meaningful. typ's canonical encoder remains the authority for all scalar
// and graph canonicality rules; this pass only makes malformed authoring
// links fail with a domain error before encoding.
func validateShape(value typ.Type, stack *[]typ.Type) error {
	push := func(child typ.Type, label string) error {
		if typ.NormalizeNil(child) == nil {
			return fmt.Errorf("typecontract: nil %s", label)
		}
		*stack = append(*stack, child)
		return nil
	}
	switch current := value.(type) {
	case *typ.Optional:
		return push(current.Inner, "optional child")
	case *typ.Union:
		for index, child := range current.Members {
			if err := push(child, fmt.Sprintf("union member %d", index)); err != nil {
				return err
			}
		}
	case *typ.Intersection:
		for index, child := range current.Members {
			if err := push(child, fmt.Sprintf("intersection member %d", index)); err != nil {
				return err
			}
		}
	case *typ.Tuple:
		for index, child := range current.Elements {
			if err := push(child, fmt.Sprintf("tuple element %d", index)); err != nil {
				return err
			}
		}
	case *typ.Array:
		return push(current.Element, "array element")
	case *typ.Map:
		if err := push(current.Key, "map key"); err != nil {
			return err
		}
		return push(current.Value, "map value")
	case *typ.ReadonlyMap:
		if err := push(current.Key, "readonly-map key"); err != nil {
			return err
		}
		return push(current.Value, "readonly-map value")
	case *typ.Record:
		if (current.MapKey == nil) != (current.MapValue == nil) {
			return errors.New("typecontract: partial record map component")
		}
		for index, field := range current.Fields {
			if err := push(field.Type, fmt.Sprintf("record field %d", index)); err != nil {
				return err
			}
		}
		for index, member := range current.StaticMembers {
			if err := push(member.Type, fmt.Sprintf("record static member %d", index)); err != nil {
				return err
			}
		}
		if current.Metatable != nil {
			if err := push(current.Metatable, "record metatable"); err != nil {
				return err
			}
		}
		if current.MapKey != nil {
			if err := push(current.MapKey, "record map key"); err != nil {
				return err
			}
			if err := push(current.MapValue, "record map value"); err != nil {
				return err
			}
		}
	case *typ.Function:
		for index, formal := range current.TypeParams {
			if formal == nil {
				return fmt.Errorf("typecontract: nil function formal %d", index)
			}
			if formal.Constraint != nil {
				if err := push(formal.Constraint, fmt.Sprintf("function formal %d constraint", index)); err != nil {
					return err
				}
			}
		}
		for index, parameter := range current.Params {
			if err := push(parameter.Type, fmt.Sprintf("function parameter %d", index)); err != nil {
				return err
			}
		}
		if current.Variadic != nil {
			if err := push(current.Variadic, "function variadic"); err != nil {
				return err
			}
		}
		for index, result := range current.Returns {
			if err := push(result, fmt.Sprintf("function result %d", index)); err != nil {
				return err
			}
		}
	case *typ.Generic:
		if current.Body == nil {
			return errors.New("typecontract: incomplete generic type")
		}
		for index, formal := range current.TypeParams {
			if formal == nil {
				return fmt.Errorf("typecontract: nil generic formal %d", index)
			}
			if formal.Constraint != nil {
				if err := push(formal.Constraint, fmt.Sprintf("generic formal %d constraint", index)); err != nil {
					return err
				}
			}
		}
		return push(current.Body, "generic body")
	case *typ.Instantiated:
		if current.Generic == nil {
			return errors.New("typecontract: instantiated type has nil generic")
		}
		if err := push(current.Generic, "instantiated generic"); err != nil {
			return err
		}
		for index, argument := range current.TypeArgs {
			if err := push(argument, fmt.Sprintf("instantiated argument %d", index)); err != nil {
				return err
			}
		}
	case *typ.TypeParam:
		if current.Constraint != nil {
			return push(current.Constraint, "type formal constraint")
		}
	case *typ.Recursive:
		if current.Body == nil {
			return errors.New("typecontract: incomplete recursive type")
		}
		return push(current.Body, "recursive body")
	case *typ.Interface:
		for index, method := range current.Methods {
			if err := push(method.Type, fmt.Sprintf("interface method %d", index)); err != nil {
				return err
			}
		}
	case *typ.Meta:
		return push(current.Of, "meta type")
	case *typ.Alias:
		return push(current.Target, "alias target")
	}
	return nil
}

type evidenceState uint8

const (
	evidenceEnter evidenceState = iota
	evidenceUnary
	evidenceUnion
	evidenceIntersection
)

type evidenceFrame struct {
	value         typ.Type
	state         evidenceState
	members       []typ.Type
	next          int
	activeType    typ.Type
	activeGeneric *typ.Generic
}

// runtimeEvidence is a finite graph machine shared by callable and fresh
// admission. Union is existential, intersection universal, and cycles use
// the caller's polarity. The explicit continuation keeps deep authored types
// off the host stack.
func runtimeEvidence(value typ.Type, leaf func(typ.Type) bool, cycle, followBounds bool) bool {
	active := make(map[typ.Type]bool)
	activeGenerics := make(map[*typ.Generic]bool)
	stack := []evidenceFrame{{value: value}}
	result := false
	finish := func(value bool) {
		index := len(stack) - 1
		frame := stack[index]
		if frame.activeType != nil {
			delete(active, frame.activeType)
		}
		if frame.activeGeneric != nil {
			delete(activeGenerics, frame.activeGeneric)
		}
		stack = stack[:index]
		result = value
	}
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		if top.state != evidenceEnter {
			switch top.state {
			case evidenceUnary:
				finish(result)
			case evidenceUnion:
				if result {
					finish(true)
				} else if top.next < len(top.members) {
					child := top.members[top.next]
					top.next++
					stack = append(stack, evidenceFrame{value: child})
				} else {
					finish(false)
				}
			case evidenceIntersection:
				if !result {
					finish(false)
				} else if top.next < len(top.members) {
					child := top.members[top.next]
					top.next++
					stack = append(stack, evidenceFrame{value: child})
				} else {
					finish(true)
				}
			}
			continue
		}
		current := typ.NormalizeNil(top.value)
		if current == nil || typ.IsNever(current) {
			finish(false)
			continue
		}
		if typ.IsAny(current) {
			finish(true)
			continue
		}
		if active[current] {
			finish(cycle)
			continue
		}
		active[current] = true
		top.activeType = current
		switch current := current.(type) {
		case *typ.Annotated:
			top.state = evidenceUnary
			stack = append(stack, evidenceFrame{value: current.Inner})
		case *typ.Alias:
			top.state = evidenceUnary
			stack = append(stack, evidenceFrame{value: current.UnaliasedTarget()})
		case *typ.Optional:
			top.state = evidenceUnary
			stack = append(stack, evidenceFrame{value: current.Inner})
		case *typ.Recursive:
			top.state = evidenceUnary
			stack = append(stack, evidenceFrame{value: current.Body})
		case *typ.TypeParam:
			if followBounds && current.Constraint != nil {
				top.state = evidenceUnary
				stack = append(stack, evidenceFrame{value: current.Constraint})
			} else {
				finish(leaf(current))
			}
		case *typ.Instantiated:
			if current.Generic == nil || len(current.TypeArgs) != len(current.Generic.TypeParams) || current.Generic.Body == nil {
				finish(leaf(current))
				continue
			}
			if activeGenerics[current.Generic] {
				finish(cycle)
				continue
			}
			activeGenerics[current.Generic] = true
			top.activeGeneric = current.Generic
			expanded := subst.ExpandInstantiatedRoot(current)
			if expanded == current || expanded == nil {
				finish(leaf(current))
				continue
			}
			top.state = evidenceUnary
			stack = append(stack, evidenceFrame{value: expanded})
		case *typ.Union:
			if len(current.Members) == 0 {
				finish(false)
				continue
			}
			top.state, top.members, top.next = evidenceUnion, current.Members, 1
			stack = append(stack, evidenceFrame{value: current.Members[0]})
		case *typ.Intersection:
			if len(current.Members) == 0 {
				finish(false)
				continue
			}
			top.state, top.members, top.next = evidenceIntersection, current.Members, 1
			stack = append(stack, evidenceFrame{value: current.Members[0]})
		default:
			finish(leaf(current))
		}
	}
	return result
}

func directFunctionLeaf(value typ.Type) bool {
	return value != nil && value.Kind() == kind.Function
}

func directTableLeaf(value typ.Type) bool {
	if value == nil {
		return false
	}
	if typ.IsBuiltinTableTopMarker(value) {
		return true
	}
	switch value.Kind() {
	case kind.Record, kind.Array, kind.Tuple, kind.Map, kind.ReadonlyMap:
		return true
	default:
		return false
	}
}

func knownConcreteRuntimeKind(value typ.Type) bool {
	if value == nil {
		return false
	}
	if typ.IsBuiltinTableTopMarker(value) {
		return true
	}
	if literal, ok := value.(*typ.Literal); ok {
		switch literal.Base() {
		case kind.Boolean, kind.Integer, kind.Number, kind.String:
			return true
		}
	}
	switch value.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Function,
		kind.Record, kind.Array, kind.Tuple, kind.Map, kind.ReadonlyMap:
		return true
	default:
		return false
	}
}
