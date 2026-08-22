package typ

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/domain/type/kind"
)

var (
	ErrInvalidCanonicalType = errors.New("typ: invalid canonical type encoding")
	// ErrCanonicalRecursiveIdentityUnavailable is deliberately distinct from a
	// malformed stream. Structural bytes alone cannot recreate the stricter
	// declaration identity required by a recursive typewitness.
	ErrCanonicalRecursiveIdentityUnavailable = errors.New("typ: canonical recursive identity is unavailable")
)

func canonicalGenericHeader(scalar []byte) (string, int, bool, error) {
	r := canonicalRawReader{raw: scalar}
	tag, ok := r.byte()
	if !ok || tag != canonicalGeneric {
		return "", 0, false, fmt.Errorf("%w: generic scalar", ErrInvalidCanonicalType)
	}
	name, nameOK := r.frame()
	paramCount, paramsOK := r.uvarint()
	hasBody, bodyOK := r.bool()
	if !nameOK || !paramsOK || !bodyOK || paramCount > uint64(maxInt()) || r.at != len(r.raw) {
		return "", 0, false, fmt.Errorf("%w: generic shape", ErrInvalidCanonicalType)
	}
	return string(name), int(paramCount), hasBody, nil
}

// materializeCanonicalUnionNode rebuilds a published union node.
//
// Member order in a union node is fixed by hash at construction time, and the
// hash of a recursive binder is a function of its body. A structural decode
// allocates every binder as an open placeholder and only closes it once the
// whole graph exists, so re-sorting a union that reaches a binder would order
// it by placeholder hashes that no longer hold once the bodies are set. The
// published order is the graph's own order and is retained verbatim in that
// case; without an open binder the member hashes are already final and the
// ordinary canonicalizing constructor still rejects a reordered stream.
func materializeCanonicalUnionNode(children []Type) Type {
	filtered := filterNilTypes(children)
	if !containsOpenRecursiveMember(filtered) {
		return MaterializeUnion(children)
	}
	unique, hashes := deduplicateTypesWithHashes(filtered)
	return newCanonicalUnion(unique, hashes)
}

// materializeCanonicalIntersectionNode is the intersection counterpart of
// materializeCanonicalUnionNode and retains the same published order rule.
func materializeCanonicalIntersectionNode(children []Type) Type {
	filtered := filterNilTypes(children)
	if !containsOpenRecursiveMember(filtered) {
		return MaterializeIntersection(children)
	}
	unique, hashes := deduplicateTypesWithHashes(filtered)
	return newCanonicalIntersection(unique, hashes)
}

func containsOpenRecursiveMember(members []Type) bool {
	for _, member := range members {
		if mayContainOpenRecursive(member) {
			return true
		}
	}
	return false
}

func materializeCanonicalNode(ctx context.Context, scalar []byte, children []Type, steps *uint64) (Type, error) {
	r := canonicalRawReader{raw: scalar}
	tag, ok := r.byte()
	if !ok {
		return nil, fmt.Errorf("%w: empty scalar", ErrInvalidCanonicalType)
	}
	wantChildren := func(count int) error {
		if len(children) != count {
			return fmt.Errorf("%w: tag %d has %d children, want %d", ErrInvalidCanonicalType, tag, len(children), count)
		}
		return nil
	}
	finish := func() error {
		if r.at != len(r.raw) {
			return fmt.Errorf("%w: trailing node scalar", ErrInvalidCanonicalType)
		}
		return nil
	}
	leaf := func(value Type) (Type, error) {
		if err := wantChildren(0); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return value, nil
	}

	switch tag {
	case canonicalNil:
		return leaf(nil)
	case canonicalPrimitiveNil:
		return leaf(Nil)
	case canonicalBoolean:
		return leaf(Boolean)
	case canonicalNumber:
		return leaf(Number)
	case canonicalInteger:
		return leaf(Integer)
	case canonicalString:
		return leaf(String)
	case canonicalAny:
		return leaf(Any)
	case canonicalUnknown:
		return leaf(Unknown)
	case canonicalNever:
		return leaf(Never)
	case canonicalSelf:
		return leaf(Self)
	case canonicalLiteral:
		if err := wantChildren(0); err != nil {
			return nil, err
		}
		base, ok := r.byte()
		if !ok {
			return nil, fmt.Errorf("%w: literal base", ErrInvalidCanonicalType)
		}
		var value Type
		switch kind.Kind(base) {
		case kind.Boolean:
			v, valid := r.bool()
			if !valid {
				return nil, fmt.Errorf("%w: boolean literal", ErrInvalidCanonicalType)
			}
			value = LiteralBool(v)
		case kind.Integer:
			v, valid := r.varint()
			if !valid {
				return nil, fmt.Errorf("%w: integer literal", ErrInvalidCanonicalType)
			}
			value = LiteralInt(v)
		case kind.Number:
			bits, valid := r.fixed64()
			if !valid {
				return nil, fmt.Errorf("%w: number literal", ErrInvalidCanonicalType)
			}
			value = LiteralNumber(math.Float64frombits(bits))
		case kind.String:
			v, valid := r.frame()
			if !valid {
				return nil, fmt.Errorf("%w: string literal", ErrInvalidCanonicalType)
			}
			value = LiteralString(string(v))
		default:
			return nil, fmt.Errorf("%w: literal base %d", ErrInvalidCanonicalType, base)
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return value, nil
	case canonicalRef:
		if err := wantChildren(0); err != nil {
			return nil, err
		}
		module, okModule := r.frame()
		name, okName := r.frame()
		if !okModule || !okName || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: reference", ErrInvalidCanonicalType)
		}
		return NewRef(string(module), string(name)), nil
	case canonicalOptional:
		if err := wantChildren(1); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return MaterializeOptional(children[0]), nil
	case canonicalUnion, canonicalIntersection, canonicalTuple:
		count, ok := r.uvarint()
		if !ok || count != uint64(len(children)) || count > uint64(maxInt()) || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: aggregate arity", ErrInvalidCanonicalType)
		}
		switch tag {
		case canonicalUnion:
			return materializeCanonicalUnionNode(children), nil
		case canonicalIntersection:
			return materializeCanonicalIntersectionNode(children), nil
		default:
			for range children {
				if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
					return nil, err
				}
			}
			return NewTuple(children...), nil
		}
	case canonicalArray:
		if err := wantChildren(1); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return NewArray(children[0]), nil
	case canonicalMap, canonicalReadonlyMap:
		if err := wantChildren(2); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		if tag == canonicalMap {
			return RebuildMap(children[0], children[1]), nil
		}
		return RebuildReadonlyMap(children[0], children[1]), nil
	case canonicalRecord:
		open, ok := r.bool()
		fieldCount, fieldsOK := r.uvarint()
		// name-frame + optional + readonly is at least three bytes per field.
		if !ok || !fieldsOK || fieldCount > uint64(maxInt()) || fieldCount > uint64((len(r.raw)-r.at)/3) || fieldCount > uint64(len(children)) {
			return nil, fmt.Errorf("%w: record header", ErrInvalidCanonicalType)
		}
		fields := make([]Field, int(fieldCount))
		for index := range fields {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			name, nameOK := r.frame()
			optional, optionalOK := r.bool()
			readonly, readonlyOK := r.bool()
			if !nameOK || !optionalOK || !readonlyOK {
				return nil, fmt.Errorf("%w: record field", ErrInvalidCanonicalType)
			}
			fields[index] = Field{Name: string(name), Optional: optional, Readonly: readonly}
		}
		staticCount, ok := r.uvarint()
		// kind + name-frame + index + optional + readonly is at least five
		// bytes per static member.
		if !ok || staticCount > uint64(maxInt()) || staticCount > uint64((len(r.raw)-r.at)/5) ||
			staticCount > uint64(len(children))-fieldCount {
			return nil, fmt.Errorf("%w: record static count", ErrInvalidCanonicalType)
		}
		members := make([]StaticMember, int(staticCount))
		for index := range members {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			memberKind, kindOK := r.byte()
			name, nameOK := r.frame()
			memberIndex, indexOK := r.varint()
			optional, optionalOK := r.bool()
			readonly, readonlyOK := r.bool()
			if !kindOK || !nameOK || !indexOK || !optionalOK || !readonlyOK ||
				StaticMemberKind(memberKind) < StaticMemberStringIndex || StaticMemberKind(memberKind) > StaticMemberIntIndex {
				return nil, fmt.Errorf("%w: record static member", ErrInvalidCanonicalType)
			}
			members[index] = StaticMember{Kind: StaticMemberKind(memberKind), Name: string(name), Index: memberIndex, Optional: optional, Readonly: readonly}
		}
		hasMap, mapOK := r.bool()
		hasMeta, metaOK := r.bool()
		childCount := int(fieldCount + staticCount)
		if hasMap {
			childCount += 2
		}
		if hasMeta {
			childCount++
		}
		if !mapOK || !metaOK || r.at != len(r.raw) || childCount != len(children) {
			return nil, fmt.Errorf("%w: record child shape", ErrInvalidCanonicalType)
		}
		at := 0
		for index := range fields {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			fields[index].Type = children[at]
			at++
		}
		for index := range members {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			members[index].Type = children[at]
			at++
		}
		parts := RecordParts{Fields: fields, StaticMembers: members, Open: open, AssumeSorted: true}
		if hasMap {
			parts.MapKey, parts.MapValue = children[at], children[at+1]
			at += 2
		}
		if hasMeta {
			parts.Metatable = children[at]
		}
		return RebuildRecord(parts), nil
	case canonicalFunction:
		typeParamCount, ok := r.uvarint()
		paramCount, paramsOK := r.uvarint()
		if !ok || !paramsOK || typeParamCount > uint64(maxInt()) || paramCount > uint64(maxInt()) ||
			typeParamCount > uint64(len(children)) || paramCount > uint64(len(children))-typeParamCount ||
			paramCount > uint64((len(r.raw)-r.at)/2) {
			return nil, fmt.Errorf("%w: function header", ErrInvalidCanonicalType)
		}
		optional := make([]bool, int(paramCount))
		receiver := make([]bool, int(paramCount))
		for index := range optional {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			optional[index], ok = r.bool()
			receiver[index], paramsOK = r.bool()
			if !ok || !paramsOK {
				return nil, fmt.Errorf("%w: function parameter", ErrInvalidCanonicalType)
			}
		}
		hasVariadic, variadicOK := r.bool()
		returnCount, returnsOK := r.uvarint()
		childCount := typeParamCount + paramCount
		if hasVariadic {
			childCount++
		}
		if !variadicOK || !returnsOK || returnCount > uint64(maxInt()) || childCount > uint64(len(children)) ||
			returnCount != uint64(len(children))-childCount || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: function child shape", ErrInvalidCanonicalType)
		}
		at := 0
		typeParams := make([]*TypeParam, int(typeParamCount))
		for index := range typeParams {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			param, valid := children[at].(*TypeParam)
			if !valid || param == nil {
				return nil, fmt.Errorf("%w: function type parameter", ErrInvalidCanonicalType)
			}
			typeParams[index], at = param, at+1
		}
		params := make([]Param, int(paramCount))
		for index := range params {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			name := ""
			if receiver[index] {
				name = "self"
			}
			params[index] = Param{Name: name, Type: children[at], Optional: optional[index], Receiver: receiver[index]}
			at++
		}
		var variadic Type
		if hasVariadic {
			variadic, at = children[at], at+1
		}
		returns := make([]Type, len(children)-at)
		for index := range returns {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			returns[index] = children[at+index]
		}
		return RebuildFunction(FunctionParts{TypeParams: typeParams, Params: params, Variadic: variadic, Returns: returns}), nil
	case canonicalGeneric:
		name, nameOK := r.frame()
		paramCount, paramsOK := r.uvarint()
		hasBody, bodyOK := r.bool()
		childCount := paramCount
		if hasBody {
			childCount++
		}
		if !nameOK || !paramsOK || !bodyOK || paramCount > uint64(maxInt()) || paramCount > uint64(len(children)) || childCount != uint64(len(children)) || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: generic shape", ErrInvalidCanonicalType)
		}
		params := make([]*TypeParam, int(paramCount))
		for index := range params {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			param, valid := children[index].(*TypeParam)
			if !valid || param == nil {
				return nil, fmt.Errorf("%w: generic type parameter", ErrInvalidCanonicalType)
			}
			params[index] = param
		}
		var body Type
		if hasBody {
			body = children[len(children)-1]
		}
		return NewGeneric(string(name), params, body), nil
	case canonicalInstantiated:
		argCount, ok := r.uvarint()
		if !ok || argCount > uint64(maxInt()) || argCount+1 != uint64(len(children)) || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: instantiated shape", ErrInvalidCanonicalType)
		}
		generic, valid := children[0].(*Generic)
		if !valid || generic == nil {
			return nil, fmt.Errorf("%w: instantiated generic", ErrInvalidCanonicalType)
		}
		return Instantiate(generic, children[1:]...), nil
	case canonicalTypeParam:
		name, nameOK := r.frame()
		hasConstraint, constraintOK := r.bool()
		want := 0
		if hasConstraint {
			want = 1
		}
		if !nameOK || !constraintOK || len(children) != want || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: type parameter", ErrInvalidCanonicalType)
		}
		var constraint Type
		if hasConstraint {
			constraint = children[0]
		}
		return NewTypeParam(string(name), constraint), nil
	case canonicalRecursive:
		return nil, ErrCanonicalRecursiveIdentityUnavailable
	case canonicalInterface:
		name, nameOK := r.frame()
		methodCount, methodsOK := r.uvarint()
		if !nameOK || !methodsOK || methodCount > uint64(maxInt()) || methodCount != uint64(len(children)) {
			return nil, fmt.Errorf("%w: interface shape", ErrInvalidCanonicalType)
		}
		methods := make([]Method, int(methodCount))
		for index := range methods {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			methodName, ok := r.frame()
			fn, valid := children[index].(*Function)
			if !ok || !valid || fn == nil {
				return nil, fmt.Errorf("%w: interface method", ErrInvalidCanonicalType)
			}
			methods[index] = Method{Name: string(methodName), Type: fn}
		}
		if r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: interface scalar", ErrInvalidCanonicalType)
		}
		return NewInterface(string(name), methods), nil
	case canonicalMeta:
		if err := wantChildren(1); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return NewMeta(children[0]), nil
	default:
		return nil, fmt.Errorf("%w: unknown scalar tag %d", ErrInvalidCanonicalType, tag)
	}
}

func canonicalDecodeCheckpoint(ctx context.Context, steps *uint64) error {
	*steps++
	if *steps == 1 || *steps&63 == 0 {
		return ctx.Err()
	}
	return nil
}
