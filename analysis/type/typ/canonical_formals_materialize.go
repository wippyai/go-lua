package typ

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// materializeCanonicalFormalsVariableNode owns the scoped artifact's
// variable-arity reconstruction.  It does not delegate a caller-sized slice
// to the ordinary source constructors: each owned slice is admitted before it
// exists, copied in checkpointed loops, and published only after its hash and
// cached properties have been derived.
func materializeCanonicalFormalsVariableNode(ctx context.Context, admission *canonicalFormalsAdmission, scalar []byte, shape canonicalFormalNodeShape, children []Type, steps *uint64) (Type, error) {
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(scalar), 1); err != nil {
		return nil, err
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(children), canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	switch shape.tag {
	case canonicalTuple:
		if err := canonicalOwnedAggregateShape(scalar, canonicalTuple, len(children)); err != nil {
			return nil, err
		}
		return canonicalOwnedTuple(ctx, admission, steps, children)
	case canonicalUnion:
		if err := canonicalOwnedAggregateShape(scalar, canonicalUnion, len(children)); err != nil {
			return nil, err
		}
		return canonicalScopedUnion(ctx, admission, steps, children)
	case canonicalIntersection:
		if err := canonicalOwnedAggregateShape(scalar, canonicalIntersection, len(children)); err != nil {
			return nil, err
		}
		return canonicalScopedIntersection(ctx, admission, steps, children)
	case canonicalRecord:
		return canonicalOwnedRecord(ctx, admission, steps, scalar, children)
	case canonicalFunction:
		return canonicalOwnedFunction(ctx, admission, steps, scalar, children)
	case canonicalGeneric:
		return canonicalOwnedGenericNode(ctx, admission, steps, scalar, children)
	case canonicalInstantiated:
		return canonicalOwnedInstantiation(ctx, admission, steps, scalar, children)
	case canonicalInterface:
		return canonicalOwnedInterface(ctx, admission, steps, scalar, children)
	default:
		return nil, fmt.Errorf("%w: scoped non-variable node", ErrInvalidCanonicalType)
	}
}

func canonicalOwnedAggregateShape(scalar []byte, tag byte, count int) error {
	reader := canonicalRawReader{raw: scalar}
	got, ok := reader.byte()
	want, countOK := reader.uvarint()
	if !ok || got != tag || !countOK || want != uint64(count) || reader.at != len(reader.raw) {
		return invalidCanonicalFormals("aggregate shape")
	}
	return nil
}

func canonicalOwnedTuple(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, elements []Type) (*Tuple, error) {
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(elements), canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	cleaned := make([]Type, len(elements))
	h := uint64(kind.Tuple)
	var props typeProperties
	for index, element := range elements {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		if element == nil {
			element = Unknown
		}
		cleaned[index] = element
		h = hash.MixHash(h, element.Hash())
		props.include(element)
	}
	return &Tuple{Elements: cleaned, hash: h, typeProperties: props}, nil
}

// canonicalScopedUnion retains the published member sequence exactly.  The
// ordinary source constructor intentionally flattens/deduplicates/sorts; that
// work has already happened before canonical bytes are published, and doing it
// again here could reorder an open recursive graph.
func canonicalScopedUnion(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, members []Type) (Type, error) {
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(members), canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	memberCopy := make([]Type, len(members))
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(members), canonicalFormalsIntBytes); err != nil {
		return nil, err
	}
	hashes := make([]uint64, len(memberCopy))
	h := uint64(kind.Union)
	var props typeProperties
	for index, member := range members {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		memberCopy[index] = member
		hashes[index] = unionMemberHash(member)
		h = hash.MixHash(h, hashes[index])
		props.includeUnionMember(member)
	}
	return &Union{Members: memberCopy, memberHashes: hashes, hash: h, typeProperties: props}, nil
}

func canonicalScopedIntersection(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, members []Type) (Type, error) {
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(members), canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	memberCopy := make([]Type, len(members))
	h := uint64(kind.Intersection)
	var props typeProperties
	for index, member := range members {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		memberCopy[index] = member
		h = hash.MixHash(h, unionMemberHash(member))
		props.include(member)
	}
	return &Intersection{Members: memberCopy, hash: h, typeProperties: props}, nil
}

func canonicalOwnedRecord(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, scalar []byte, children []Type) (*Record, error) {
	reader := canonicalRawReader{raw: scalar}
	tag, tagOK := reader.byte()
	open, openOK := reader.bool()
	fieldCount, fieldsOK := reader.uvarint()
	if !tagOK || tag != canonicalRecord || !openOK || !fieldsOK || fieldCount > uint64(maxInt()) || fieldCount > uint64(len(children)) {
		return nil, invalidCanonicalFormals("record header")
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, int(fieldCount), canonicalFormalsFieldBytes); err != nil {
		return nil, err
	}
	fields := make([]Field, int(fieldCount))
	for index := range fields {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		name, nameOK := reader.frame()
		optional, optionalOK := reader.bool()
		readonly, readonlyOK := reader.bool()
		if !nameOK || !optionalOK || !readonlyOK {
			return nil, invalidCanonicalFormals("record field")
		}
		fields[index] = Field{Name: string(name), Optional: optional, Readonly: readonly}
	}
	staticCount, staticsOK := reader.uvarint()
	if !staticsOK || staticCount > uint64(maxInt()) || staticCount > uint64(len(children))-fieldCount {
		return nil, invalidCanonicalFormals("record static count")
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, int(staticCount), canonicalFormalsStaticMemBytes); err != nil {
		return nil, err
	}
	members := make([]StaticMember, int(staticCount))
	for index := range members {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		memberKind, kindOK := reader.byte()
		name, nameOK := reader.frame()
		memberIndex, indexOK := reader.varint()
		optional, optionalOK := reader.bool()
		readonly, readonlyOK := reader.bool()
		if !kindOK || !nameOK || !indexOK || !optionalOK || !readonlyOK || StaticMemberKind(memberKind) < StaticMemberStringIndex || StaticMemberKind(memberKind) > StaticMemberIntIndex {
			return nil, invalidCanonicalFormals("record static member")
		}
		members[index] = StaticMember{Kind: StaticMemberKind(memberKind), Name: string(name), Index: memberIndex, Optional: optional, Readonly: readonly}
	}
	hasMap, mapOK := reader.bool()
	hasMeta, metaOK := reader.bool()
	childCount := int(fieldCount + staticCount)
	if hasMap {
		childCount += 2
	}
	if hasMeta {
		childCount++
	}
	if !mapOK || !metaOK || reader.at != len(reader.raw) || childCount != len(children) {
		return nil, invalidCanonicalFormals("record child shape")
	}
	at := 0
	for index := range fields {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		fields[index].Type = children[at]
		if fields[index].Type == nil {
			fields[index].Type = Unknown
		}
		at++
	}
	for index := range members {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		members[index].Type = children[at]
		if members[index].Type == nil {
			members[index].Type = Unknown
		}
		at++
	}
	var mapKey, mapValue, metatable Type
	if hasMap {
		mapKey, mapValue = children[at], children[at+1]
		at += 2
		if mapKey == nil {
			mapKey = Unknown
		}
		if mapValue == nil {
			mapValue = Unknown
		}
	}
	if hasMeta {
		metatable = children[at]
	}
	if !fieldsSortedByName(fields) || !staticMembersSorted(members) {
		return nil, invalidCanonicalFormals("record member order")
	}
	h := uint64(kind.Record)
	var props typeProperties
	for _, field := range fields {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		h = hash.MixHash(h, hash.FnvString(field.Name))
		h = hash.MixHash(h, field.Type.Hash())
		if field.Optional {
			h = hash.MixHash(h, 1)
		}
		if field.Readonly {
			h = hash.MixHash(h, 2)
		}
		props.include(field.Type)
	}
	for _, member := range members {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		h = hash.MixHash(h, recordStaticHash)
		h = hash.MixHash(h, uint64(member.Kind))
		if member.Kind == StaticMemberStringIndex {
			h = hash.MixHash(h, hash.FnvString(member.Name))
		} else {
			h = hash.MixHash(h, uint64(member.Index))
		}
		h = hash.MixHash(h, member.Type.Hash())
		if member.Optional {
			h = hash.MixHash(h, 1)
		}
		if member.Readonly {
			h = hash.MixHash(h, 2)
		}
		props.include(member.Type)
	}
	if metatable != nil {
		h = hash.MixHash(h, metatable.Hash())
	}
	if open {
		h = hash.MixHash(h, 3)
	}
	if mapKey != nil {
		h = hash.MixHash(h, recordMapKeyHash)
		h = hash.MixHash(h, mapKey.Hash())
	}
	if mapValue != nil {
		h = hash.MixHash(h, recordMapValueHash)
		h = hash.MixHash(h, mapValue.Hash())
	}
	props.includeTypes(metatable, mapKey, mapValue)
	return &Record{Fields: fields, StaticMembers: members, Metatable: metatable, MapKey: mapKey, MapValue: mapValue, Open: open, sorted: true, hash: h, equalityHashCache: &equalityHashCache{}, typeProperties: props}, nil
}

func canonicalOwnedFunction(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, scalar []byte, children []Type) (*Function, error) {
	reader := canonicalRawReader{raw: scalar}
	tag, tagOK := reader.byte()
	typeParamCount, typeParamsOK := reader.uvarint()
	paramCount, paramsOK := reader.uvarint()
	if !tagOK || tag != canonicalFunction || !typeParamsOK || !paramsOK || typeParamCount > uint64(maxInt()) || paramCount > uint64(maxInt()) || typeParamCount > uint64(len(children)) || paramCount > uint64(len(children))-typeParamCount {
		return nil, invalidCanonicalFormals("function header")
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, int(paramCount), canonicalFormalsBoolBytes*2); err != nil {
		return nil, err
	}
	optional := make([]bool, int(paramCount))
	receiver := make([]bool, int(paramCount))
	for index := range optional {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		var ok bool
		optional[index], ok = reader.bool()
		receiver[index], paramsOK = reader.bool()
		if !ok || !paramsOK {
			return nil, invalidCanonicalFormals("function parameter")
		}
	}
	hasVariadic, variadicOK := reader.bool()
	returnCount, returnsOK := reader.uvarint()
	childCount := typeParamCount + paramCount
	if hasVariadic {
		childCount++
	}
	if !variadicOK || !returnsOK || returnCount > uint64(maxInt()) || childCount > uint64(len(children)) || returnCount != uint64(len(children))-childCount || reader.at != len(reader.raw) {
		return nil, invalidCanonicalFormals("function child shape")
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, int(typeParamCount), canonicalFormalsPointerBytes); err != nil {
		return nil, err
	}
	typeParams := make([]*TypeParam, int(typeParamCount))
	at := 0
	for index := range typeParams {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		param, valid := children[at].(*TypeParam)
		if !valid || param == nil {
			return nil, invalidCanonicalFormals("function type parameter")
		}
		typeParams[index], at = param, at+1
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, int(paramCount), canonicalFormalsParamBytes); err != nil {
		return nil, err
	}
	params := make([]Param, int(paramCount))
	for index := range params {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		value := NormalizeNil(children[at])
		if value == nil {
			return nil, invalidCanonicalFormals("function parameter type")
		}
		name := ""
		if receiver[index] {
			name = "self"
		}
		params[index] = Param{Name: name, Type: value, Optional: optional[index], Receiver: receiver[index]}
		at++
	}
	var variadic Type
	if hasVariadic {
		variadic = NormalizeNil(children[at])
		if variadic == nil {
			return nil, invalidCanonicalFormals("function variadic type")
		}
		at++
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(children)-at, canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	returns := make([]Type, len(children)-at)
	for index := range returns {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		returns[index] = NormalizeNil(children[at+index])
		if returns[index] == nil {
			return nil, invalidCanonicalFormals("function return type")
		}
	}
	returns, err := canonicalOwnedNormalizeFunctionReturns(ctx, admission, steps, returns)
	if err != nil {
		return nil, err
	}
	return canonicalOwnedFunctionParts(ctx, admission, steps, typeParams, params, variadic, returns)
}

// canonicalOwnedNormalizeFunctionReturns is the cancellable equivalent of
// normalizeFunctionReturns.  Canonical bytes must represent the same Function
// semantic shape as source construction, including one-level tuple return
// flattening; the mandatory byte round-trip then rejects a noncanonical wire
// shape instead of admitting a second function vocabulary.
func canonicalOwnedNormalizeFunctionReturns(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, returns []Type) ([]Type, error) {
	total := len(returns)
	hasTuple := false
	for _, result := range returns {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		if tuple, ok := result.(*Tuple); ok {
			hasTuple = true
			if len(tuple.Elements) > maxInt()-total {
				return nil, invalidCanonicalFormals("function result admission")
			}
			total += len(tuple.Elements) - 1
		}
	}
	if !hasTuple {
		return returns, nil
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, total, canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	normalized := make([]Type, 0, total)
	for _, result := range returns {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		if tuple, ok := result.(*Tuple); ok {
			for _, element := range tuple.Elements {
				if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
					return nil, err
				}
				normalized = append(normalized, element)
			}
			continue
		}
		normalized = append(normalized, result)
	}
	return normalized, nil
}

func canonicalOwnedFunctionParts(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, typeParams []*TypeParam, params []Param, variadic Type, returns []Type) (*Function, error) {
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(typeParams), canonicalFormalsPointerBytes); err != nil {
		return nil, err
	}
	paramCopy := make([]*TypeParam, len(typeParams))
	h := uint64(kind.Function)
	var props typeProperties
	for index, param := range typeParams {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		if param == nil {
			return nil, invalidCanonicalFormals("function type parameter")
		}
		paramCopy[index] = param
		h = hash.MixHash(h, param.Hash())
		props.include(param)
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(params), canonicalFormalsParamBytes); err != nil {
		return nil, err
	}
	paramsCopy := make([]Param, len(params))
	semantic := true
	for index, param := range params {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		param.Type = NormalizeNil(param.Type)
		if param.Type == nil {
			return nil, invalidCanonicalFormals("function parameter type")
		}
		param.Receiver = param.Receiver || param.Name == "self"
		if param.Name != "" && (!param.Receiver || param.Name != "self") {
			semantic = false
		}
		paramsCopy[index] = param
		h = hash.MixHash(h, param.Type.Hash())
		if param.Receiver {
			h = hash.MixHash(h, 2)
		}
		if param.Optional {
			h = hash.MixHash(h, 1)
		}
		props.include(param.Type)
	}
	variadic = NormalizeNil(variadic)
	if variadic != nil {
		h = hash.MixHash(h, variadic.Hash())
		props.include(variadic)
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(returns), canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	returnsCopy := make([]Type, len(returns))
	for index, result := range returns {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		result = NormalizeNil(result)
		if result == nil {
			return nil, invalidCanonicalFormals("function return type")
		}
		returnsCopy[index] = result
		h = hash.MixHash(h, result.Hash())
		props.include(result)
	}
	fn := &Function{TypeParams: paramCopy, Params: paramsCopy, Variadic: variadic, Returns: returnsCopy, hash: h, equalityHashCache: &equalityHashCache{}, typeProperties: props}
	if semantic {
		fn.semantic.Store(fn)
	}
	return fn, nil
}

func canonicalOwnedGenericNode(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, scalar []byte, children []Type) (*Generic, error) {
	name, paramCount, hasBody, err := canonicalGenericHeader(scalar)
	if err != nil {
		return nil, err
	}
	if len(children) != paramCount+boolChildCount(hasBody) {
		return nil, invalidCanonicalFormals("generic child shape")
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, paramCount, canonicalFormalsPointerBytes); err != nil {
		return nil, err
	}
	params := make([]*TypeParam, paramCount)
	for index := range params {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		param, valid := children[index].(*TypeParam)
		if !valid || param == nil {
			return nil, invalidCanonicalFormals("generic type parameter")
		}
		params[index] = param
	}
	var body Type
	if hasBody {
		body = children[len(children)-1]
	}
	return canonicalOwnedGeneric(ctx, admission, steps, name, params, body)
}

func canonicalOwnedGeneric(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, name string, params []*TypeParam, body Type) (*Generic, error) {
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(params), canonicalFormalsPointerBytes); err != nil {
		return nil, err
	}
	copyParams := make([]*TypeParam, len(params))
	h := hash.MixHash(uint64(kind.Generic), hash.FnvString(name))
	var props typeProperties
	for index, param := range params {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		if param == nil {
			return nil, invalidCanonicalFormals("generic type parameter")
		}
		copyParams[index] = param
		h = hash.MixHash(h, param.Hash())
		props.include(param)
	}
	if body != nil {
		h = hash.MixHash(h, body.Hash())
		props.include(body)
	}
	return &Generic{Name: name, TypeParams: copyParams, Body: body, hash: h, typeProperties: props}, nil
}

func canonicalOwnedSetGenericBody(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, generic *Generic, body Type) error {
	if generic == nil || body == nil || generic.Body != nil {
		return invalidCanonicalFormals("generic body")
	}
	if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
		return err
	}
	h := hash.MixHash(uint64(kind.Generic), hash.FnvString(generic.Name))
	for _, param := range generic.TypeParams {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return err
		}
		h = hash.MixHash(h, param.Hash())
	}
	generic.Body = body
	generic.rev++
	generic.hash = hash.MixHash(h, body.Hash())
	generic.typeProperties.invalidateOpenRecursiveCache()
	generic.typeProperties.include(body)
	generic.strCache = stringCache{}
	return nil
}

func canonicalOwnedInstantiation(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, scalar []byte, children []Type) (*Instantiated, error) {
	if err := canonicalOwnedAggregateShape(scalar, canonicalInstantiated, len(children)-1); err != nil {
		return nil, err
	}
	if len(children) == 0 {
		return nil, invalidCanonicalFormals("instantiated shape")
	}
	generic, valid := children[0].(*Generic)
	if !valid || generic == nil || len(children)-1 != len(generic.TypeParams) {
		return nil, invalidCanonicalFormals("instantiated generic")
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(children)-1, canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	arguments := make([]Type, len(children)-1)
	h := hash.MixHash(uint64(kind.Instantiated), generic.Hash())
	props := typePropertiesOf(generic)
	props.containsInstantiated = true
	for index, argument := range children[1:] {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		if argument == nil {
			return nil, invalidCanonicalFormals("instantiated argument")
		}
		arguments[index] = argument
		h = hash.MixHash(h, argument.Hash())
		props.include(argument)
	}
	return &Instantiated{Generic: generic, TypeArgs: arguments, hash: h, equalityHashCache: &equalityHashCache{}, typeProperties: props}, nil
}

func canonicalOwnedInterface(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, scalar []byte, children []Type) (*Interface, error) {
	reader := canonicalRawReader{raw: scalar}
	tag, tagOK := reader.byte()
	name, nameOK := reader.frame()
	methodCount, methodsOK := reader.uvarint()
	if !tagOK || tag != canonicalInterface || !nameOK || !methodsOK || methodCount > uint64(maxInt()) || int(methodCount) != len(children) {
		return nil, invalidCanonicalFormals("interface shape")
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(children), canonicalFormalsMethodBytes); err != nil {
		return nil, err
	}
	methods := make([]Method, len(children))
	h := hash.MixHash(uint64(kind.Interface), hash.FnvString(string(name)))
	var props typeProperties
	for index := range methods {
		if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		methodName, methodOK := reader.frame()
		function, valid := children[index].(*Function)
		if !methodOK || !valid || function == nil {
			return nil, invalidCanonicalFormals("interface method")
		}
		methods[index] = Method{Name: string(methodName), Type: function}
		h = hash.MixHash(h, hash.FnvString(methods[index].Name))
		h = hash.MixHash(h, function.Hash())
		props.include(function)
	}
	if reader.at != len(reader.raw) {
		return nil, invalidCanonicalFormals("interface scalar")
	}
	return &Interface{Name: string(name), Methods: methods, hash: h, typeProperties: props}, nil
}
