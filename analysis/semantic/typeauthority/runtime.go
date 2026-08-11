package typeauthority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
)

// Form is the exact runtime LType reflection form. FormOther deliberately
// covers known types whose VM KindString is "unknown"; it is not an unknown
// authored type and does not erase structural authority.
type Form uint8

const (
	FormOther Form = iota + 1
	FormNil
	FormBoolean
	FormNumber
	FormInteger
	FormString
	FormAny
	FormUnknown
	FormNever
	FormOptional
	FormUnion
	FormIntersection
	FormTuple
	FormFunction
	FormArray
	FormMap
	FormRecord
	FormGeneric
	FormInstantiated
	FormLiteral
	FormInterface
	FormReadonlyMap
	FormMeta
	FormRecursive
	FormSelf
	FormTypeParameter
)

func (form Form) KindString() string {
	switch form {
	case FormNil:
		return "nil"
	case FormBoolean:
		return "boolean"
	case FormNumber:
		return "number"
	case FormInteger:
		return "integer"
	case FormString:
		return "string"
	case FormAny:
		return "any"
	case FormUnknown:
		return "unknown"
	case FormNever:
		return "never"
	case FormOptional:
		return "optional"
	case FormUnion:
		return "union"
	case FormIntersection:
		return "intersection"
	case FormTuple:
		return "tuple"
	case FormFunction:
		return "function"
	case FormArray:
		return "array"
	case FormMap:
		return "map"
	case FormRecord:
		return "record"
	case FormGeneric:
		return "generic"
	case FormInstantiated:
		return "instantiated"
	case FormInterface:
		return "interface"
	case FormReadonlyMap:
		return "readonlymap"
	case FormMeta:
		return "meta"
	case FormRecursive:
		return "recursive"
	case FormSelf:
		return "self"
	case FormTypeParameter:
		return "typeparam"
	default:
		return "unknown"
	}
}

// RuntimeInner is a dense, Runtime-owner-fenced exact structural type. Its
// fields remain private: a caller cannot turn a portable type byte string into
// a hot handle or combine an inner from another Runtime authority.
type RuntimeInner struct {
	owner *Runtime
	index uint32 // one-based into Runtime.rows
}

// RuntimeInput is a cold, ownership-isolated canonical closed type input. It
// can only be minted by the selector Authority which owns the Link it will be
// sealed against. The bytes are checked before publication and never retained
// by Runtime after sealing.
type RuntimeInput struct {
	authority *Authority
	encoded   []byte
}

type runtimeChild struct {
	inner   RuntimeInner
	present bool
}

type runtimeRange struct{ start, end uint32 }

type runtimeNamedChild struct {
	name      string
	key       runtimeChild
	child     runtimeChild
	effective runtimeChild
	optional  bool
	readonly  bool
}

type runtimeStaticChild struct {
	kind      typ.StaticMemberKind
	stringKey bool
	name      string
	integer   int64
	key       runtimeChild
	child     runtimeChild
	effective runtimeChild
	optional  bool
	readonly  bool
}

type runtimeTupleElement struct {
	key   runtimeChild
	child runtimeChild
}

type runtimeParameter struct {
	name     string
	child    runtimeChild
	optional bool
	receiver bool
}

type runtimeLiteral struct {
	base kind.Kind
	bits uint64
	text string
}

type runtimeRow struct {
	form   Form
	closed bool
	// encoded is the immutable canonical source identity of a closed row. It
	// replaces the former retained typ.Type graph and is used only by cold
	// sealing consumers which need a portable reconstruction.
	encoded []byte
	// atoms is the canonical semantic-union descriptor for this row.  A
	// non-union row normally contributes one complete structural atom; union
	// rows flatten their direct arms and remove subsumed/equivalent atoms at
	// seal time.  Runtime never exposes the construction graph after seal.
	atoms          []uint32
	rank           uint32
	element        runtimeChild
	key            runtimeChild
	value          runtimeChild
	base           runtimeChild
	inner          runtimeChild
	returns        runtimeChild
	variadic       runtimeChild
	body           runtimeChild
	metatable      runtimeChild
	expansion      runtimeChild
	fields         runtimeRange
	staticMembers  runtimeRange
	variants       runtimeRange
	elements       runtimeRange
	parameters     runtimeRange
	results        runtimeRange
	methods        runtimeRange
	typeParameters runtimeRange
	arguments      runtimeRange
	name           string
	open           bool
	tableTop       bool
	metatableAny   bool
	selfRewrite    bool
	literal        runtimeLiteral
}

type runtimeInstantiation struct {
	result RuntimeInner
}

type runtimeInstantiationTrieNode struct {
	result RuntimeInner
	edges  runtimeRange
}

type runtimeInstantiationTrieEdge struct {
	argument uint32
	child    uint32 // one-based into Runtime.instantiationTrieNodes
}

// InstantiationMatch is an immutable cursor in Runtime's fixed-instantiation
// trie. It is owner-fenced and contains no caller slice or typ.Type graph.
type InstantiationMatch struct {
	owner *Runtime
	node  uint32 // one-based into Runtime.instantiationTrieNodes
}

// Runtime is the immutable finite structural authority consumed by runtime
// LType reflection. It is deliberately separate from Static's evaluator:
// Static supplies already-evaluated canonical closed inputs; Runtime seals
// their finite structural closure and has no evaluator or source AST path.
type Runtime struct {
	source *link.Link
	id     keyspace.ContentID

	rows           []runtimeRow
	fields         []runtimeNamedChild
	staticMembers  []runtimeStaticChild
	variants       []runtimeChild
	elements       []runtimeTupleElement
	parameters     []runtimeParameter
	results        []runtimeChild
	methods        []runtimeNamedChild
	typeParameters []runtimeNamedChild
	instantiations []runtimeInstantiation
	arguments      []RuntimeInner

	instantiationRoots     []uint32 // base index minus one -> one-based trie node
	instantiationTrieNodes []runtimeInstantiationTrieNode
	instantiationTrieEdges []runtimeInstantiationTrieEdge

	identities []keyspace.ContentID
	canonical  []uint32

	// Primitive rows are part of the same dense Runtime universe. They are
	// sealed once so structural rules never synthesize typ nodes or use magic
	// non-row identities during recurrent proofs.
	nilRow     uint32
	booleanRow uint32
	numberRow  uint32
	integerRow uint32
	stringRow  uint32
	anyRow     uint32
	unknownRow uint32
	neverRow   uint32
}

// runtimeCanonicalInput records every caller position represented by one
// canonical input identity. Runtime's dense universe must be a function of
// the set of inputs, not the incidental Static result traversal that supplied
// them; positions are restored only after the canonical closure is sealed.
type runtimeCanonicalInput struct {
	input     RuntimeInput
	identity  string
	positions []int
}

// RuntimeInput validates and ownership-isolates one canonical evaluated type
// result. Ordinary canonical bytes are retained as the caller's existing type
// wire language. The scoped validation rejects a free formal that ordinary
// canonical encoding can represent by a presentation name alone; nested
// Generic and Function binders remain valid closed roots.
func (a *Authority) RuntimeInput(encoded []byte) (RuntimeInput, bool) {
	if a == nil || !a.LinkID().Available() || len(encoded) == 0 {
		return RuntimeInput{}, false
	}
	value, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
	if err != nil || value == nil {
		return RuntimeInput{}, false
	}
	canonical, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil || !sameBytes(canonical, encoded) {
		return RuntimeInput{}, false
	}
	if _, err := typ.EncodeCanonicalFormals(context.Background(), value, nil); err != nil {
		return RuntimeInput{}, false
	}
	return RuntimeInput{authority: a, encoded: append([]byte(nil), encoded...)}, true
}

// SealRuntime closes the direct structural children of Static's finite
// canonical input set. Runtime owns structural rows only: occurrence-specific
// TypeValue interpretation belongs to Static, which owns both the evaluated
// result and its Boundary occurrence.
func SealRuntime(types *Authority, inputs []RuntimeInput) (*Runtime, []RuntimeInner, error) {
	if types == nil || types.source == nil || !types.source.ContentID().Available() || types.LinkID() != types.source.ContentID() {
		return nil, nil, errors.New("typeauthority: Runtime Link/selector authority mismatch")
	}
	runtime := &Runtime{
		source: types.source,
		// Relation rows and semantic descriptors are populated by the one
		// seal pass below.  No dense pair cache survives publication.
	}
	builder := runtimeBuilder{runtime: runtime, byIdentity: make(map[string]RuntimeInner)}
	if err := builder.seedPrimitives(); err != nil {
		return nil, nil, err
	}
	inners := make([]RuntimeInner, len(inputs))
	canonicalInputs, err := canonicalRuntimeInputs(types, inputs)
	if err != nil {
		return nil, nil, err
	}
	for _, input := range canonicalInputs {
		value, err := typ.DecodeCanonicalStructural(context.Background(), input.input.encoded)
		if err != nil || value == nil {
			return nil, nil, errors.New("typeauthority: invalid Runtime input")
		}
		inner, err := builder.add(runtimePending{value: value})
		if err != nil {
			return nil, nil, err
		}
		for _, position := range input.positions {
			inners[position] = inner
		}
	}
	if err := builder.close(); err != nil {
		return nil, nil, err
	}
	if err := builder.describe(); err != nil {
		return nil, nil, err
	}
	if err := runtime.sealInstantiationTrie(); err != nil {
		return nil, nil, err
	}
	if err := builder.sealCanonical(); err != nil {
		return nil, nil, err
	}
	if err := builder.sealDescriptors(); err != nil {
		return nil, nil, err
	}
	if err := runtime.sealRanks(); err != nil {
		return nil, nil, err
	}
	if err := runtime.sealIdentity(); err != nil {
		return nil, nil, err
	}
	// The builder is deliberately the only owner of construction graphs and
	// indexes. Runtime retains only dense rows/ranges, canonical source bytes,
	// names, descriptors, and identities.
	builder.pending = nil
	builder.construction = nil
	builder.byIdentity = nil
	return runtime, inners, nil
}

func canonicalRuntimeInputs(types *Authority, inputs []RuntimeInput) ([]runtimeCanonicalInput, error) {
	if types == nil {
		return nil, errors.New("typeauthority: nil Runtime selector authority")
	}
	byIdentity := make(map[string]int, len(inputs))
	unique := make([]runtimeCanonicalInput, 0, len(inputs))
	for position, input := range inputs {
		if input.authority != types || len(input.encoded) == 0 {
			return nil, errors.New("typeauthority: foreign Runtime input")
		}
		identity := string(input.encoded)
		if index, duplicate := byIdentity[identity]; duplicate {
			unique[index].positions = append(unique[index].positions, position)
			continue
		}
		byIdentity[identity] = len(unique)
		unique = append(unique, runtimeCanonicalInput{input: input, identity: identity, positions: []int{position}})
	}
	sort.Slice(unique, func(left, right int) bool { return unique[left].identity < unique[right].identity })
	return unique, nil
}

func sameBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func runtimeForm(value typ.Type) Form {
	if value == nil {
		return FormOther
	}
	switch value.Kind() {
	case kind.Nil:
		return FormNil
	case kind.Boolean:
		return FormBoolean
	case kind.Number:
		return FormNumber
	case kind.Integer:
		return FormInteger
	case kind.String:
		return FormString
	case kind.Any:
		return FormAny
	case kind.Unknown:
		return FormUnknown
	case kind.Never:
		return FormNever
	case kind.Optional:
		return FormOptional
	case kind.Union:
		return FormUnion
	case kind.Intersection:
		return FormIntersection
	case kind.Tuple:
		return FormTuple
	case kind.Function:
		return FormFunction
	case kind.Array:
		return FormArray
	case kind.Map:
		return FormMap
	case kind.Record:
		return FormRecord
	case kind.Generic:
		return FormGeneric
	case kind.Instantiated:
		return FormInstantiated
	case kind.Literal:
		return FormLiteral
	case kind.Interface:
		return FormInterface
	case kind.ReadonlyMap:
		return FormReadonlyMap
	case kind.Meta:
		return FormMeta
	case kind.Recursive:
		return FormRecursive
	case kind.Self:
		return FormSelf
	case kind.TypeParam:
		return FormTypeParameter
	default:
		return FormOther
	}
}

func runtimeScope(value typ.Type, external []*typ.TypeParam) keyspace.ContentID {
	locals := make(map[*typ.TypeParam]struct{})
	switch binder := value.(type) {
	case *typ.Function:
		for _, parameter := range binder.TypeParams {
			locals[parameter] = struct{}{}
		}
	case *typ.Generic:
		for _, parameter := range binder.TypeParams {
			locals[parameter] = struct{}{}
		}
	}
	outer := make([]*typ.TypeParam, 0, len(external))
	for _, parameter := range external {
		if _, local := locals[parameter]; !local {
			outer = append(outer, parameter)
		}
	}
	encoded, err := typ.EncodeCanonicalFormals(context.Background(), value, outer)
	if err != nil {
		return keyspace.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.typeauthority.runtime/formal-scope\x00\x01"))
	_, _ = hash.Write(encoded)
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func runtimeCombinedFormals(outer, local []*typ.TypeParam) []*typ.TypeParam {
	if len(outer) == 0 && len(local) == 0 {
		return nil
	}
	combined := make([]*typ.TypeParam, 0, len(outer)+len(local))
	seen := make(map[*typ.TypeParam]struct{}, len(outer)+len(local))
	for _, group := range [][]*typ.TypeParam{outer, local} {
		for _, parameter := range group {
			if parameter == nil {
				continue
			}
			if _, duplicate := seen[parameter]; duplicate {
				continue
			}
			seen[parameter] = struct{}{}
			combined = append(combined, parameter)
		}
	}
	return combined
}

func runtimeDenseOrdinal(length int) (uint32, error) {
	if length < 0 || uint64(length) >= uint64(math.MaxUint32) {
		return 0, errors.New("typeauthority: Runtime dense handle overflow")
	}
	return uint32(length + 1), nil
}

type runtimePending struct {
	value   typ.Type
	formals []*typ.TypeParam
	scope   keyspace.ContentID
	closed  bool
}

// runtimeBuilder is deliberately short-lived. It is the only Runtime-side
// owner of a typ graph, scoped byte key, or TypeParam pointer.
type runtimeBuilder struct {
	runtime      *Runtime
	byIdentity   map[string]RuntimeInner
	pending      []runtimePending
	construction []typ.Type
}

func (b *runtimeBuilder) seedPrimitives() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != 0 {
		return errors.New("typeauthority: invalid Runtime primitive seed")
	}
	seeds := []struct {
		value typ.Type
		dest  *uint32
	}{
		{typ.Nil, &b.runtime.nilRow},
		{typ.Boolean, &b.runtime.booleanRow},
		{typ.Number, &b.runtime.numberRow},
		{typ.Integer, &b.runtime.integerRow},
		{typ.String, &b.runtime.stringRow},
		{typ.Any, &b.runtime.anyRow},
		{typ.Unknown, &b.runtime.unknownRow},
		{typ.Never, &b.runtime.neverRow},
	}
	for _, seed := range seeds {
		inner, err := b.add(runtimePending{value: seed.value})
		if err != nil {
			return err
		}
		*seed.dest = inner.index
	}
	return nil
}

type runtimeInstantiationTrieBuildNode struct {
	result   RuntimeInner
	children map[uint32]*runtimeInstantiationTrieBuildNode
}

func (b *runtimeBuilder) add(input runtimePending) (RuntimeInner, error) {
	if b == nil || b.runtime == nil {
		return RuntimeInner{}, errors.New("typeauthority: nil Runtime builder")
	}
	value := typ.UnwrapStructuralWrappers(input.value)
	if value == nil {
		return RuntimeInner{}, errors.New("typeauthority: nil Runtime type")
	}
	input.value = value
	if !runtimeSupportedNode(value) {
		return RuntimeInner{}, errors.New("typeauthority: unsupported Runtime structural form")
	}
	input.closed = len(input.formals) == 0 || !typ.ContainsTypeParam(value)
	var encoded []byte
	var err error
	keyPrefix := "runtime/closed\x00\x01"
	if input.closed {
		encoded, err = typ.EncodeCanonical(context.Background(), value)
	} else {
		if !input.scope.Available() {
			return RuntimeInner{}, errors.New("typeauthority: scoped Runtime child lacks binder identity")
		}
		encoded, err = typ.EncodeCanonicalFormals(context.Background(), value, input.formals)
		keyPrefix = "runtime/scoped\x00\x01" + string(input.scope[:])
	}
	if err != nil || len(encoded) == 0 {
		return RuntimeInner{}, errors.New("typeauthority: Runtime type lacks canonical identity")
	}
	key := keyPrefix + string(encoded)
	if inner, ok := b.byIdentity[key]; ok {
		return inner, nil
	}
	ordinal, err := runtimeDenseOrdinal(len(b.runtime.rows))
	if err != nil {
		return RuntimeInner{}, err
	}
	inner := RuntimeInner{owner: b.runtime, index: ordinal}
	row := runtimeRow{closed: input.closed}
	if input.closed {
		row.encoded = append([]byte(nil), encoded...)
	}
	b.runtime.rows = append(b.runtime.rows, row)
	b.pending = append(b.pending, input)
	b.construction = append(b.construction, value)
	b.byIdentity[key] = inner
	return inner, nil
}

func runtimeSupportedNode(value typ.Type) bool {
	if value == nil {
		return false
	}
	switch value.(type) {
	case *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Optional,
		*typ.Function, *typ.Record, *typ.Union, *typ.Intersection,
		*typ.Tuple, *typ.Interface, *typ.Generic, *typ.Instantiated,
		*typ.Literal, *typ.Meta, *typ.Recursive, *typ.TypeParam:
		return true
	case *typ.Alias, *typ.Annotated, *typ.Ref:
		return false
	}
	switch value.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Self:
		return true
	default:
		return false
	}
}

func (b *runtimeBuilder) lookupInput(input RuntimeInput) (RuntimeInner, error) {
	if b == nil || input.authority == nil || len(input.encoded) == 0 {
		return RuntimeInner{}, errors.New("typeauthority: invalid Runtime input lookup")
	}
	value, err := typ.DecodeCanonicalStructural(context.Background(), input.encoded)
	if err != nil || value == nil {
		return RuntimeInner{}, errors.New("typeauthority: invalid Runtime input lookup")
	}
	value = typ.UnwrapStructuralWrappers(value)
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil || len(encoded) == 0 {
		return RuntimeInner{}, errors.New("typeauthority: invalid Runtime input identity")
	}
	inner, ok := b.byIdentity["runtime/closed\x00\x01"+string(encoded)]
	if !ok {
		return RuntimeInner{}, errors.New("typeauthority: unsealed Runtime descriptor input")
	}
	return inner, nil
}

func (b *runtimeBuilder) child(value typ.Type, formals []*typ.TypeParam, scope keyspace.ContentID) (RuntimeInner, error) {
	before := len(b.runtime.rows)
	inner, err := b.add(runtimePending{value: value, formals: formals, scope: scope})
	if err != nil {
		return RuntimeInner{}, err
	}
	if len(b.runtime.rows) != before {
		return RuntimeInner{}, errors.New("typeauthority: incomplete Runtime structural closure")
	}
	return inner, nil
}

func (b *runtimeBuilder) close() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.pending) {
		return errors.New("typeauthority: malformed Runtime closure")
	}
	// This finite index walk is the least direct-child closure. add installs a
	// row before it is observed, so a recursive backedge reaches the existing
	// dense handle rather than consuming a Go call stack or minting ancestry.
	for index := 0; index < len(b.pending); index++ {
		current := b.pending[index]
		if err := b.enqueueChildren(current); err != nil {
			return err
		}
	}
	return nil
}

func (b *runtimeBuilder) enqueueChildren(current runtimePending) error {
	child := func(value typ.Type, formals []*typ.TypeParam, scope keyspace.ContentID) error {
		if value == nil {
			return nil
		}
		_, err := b.add(runtimePending{value: value, formals: formals, scope: scope})
		return err
	}
	switch value := current.value.(type) {
	case *typ.Array:
		return child(value.Element, current.formals, current.scope)
	case *typ.Map:
		if err := child(value.Key, current.formals, current.scope); err != nil {
			return err
		}
		return child(value.Value, current.formals, current.scope)
	case *typ.ReadonlyMap:
		if err := child(value.Key, current.formals, current.scope); err != nil {
			return err
		}
		return child(value.Value, current.formals, current.scope)
	case *typ.Optional:
		return child(value.Inner, current.formals, current.scope)
	case *typ.Function:
		formals := runtimeCombinedFormals(current.formals, value.TypeParams)
		scope := runtimeScope(value, current.formals)
		if !scope.Available() {
			return errors.New("typeauthority: unavailable Runtime function binder identity")
		}
		for _, parameter := range value.TypeParams {
			if parameter == nil {
				return errors.New("typeauthority: nil Runtime function type parameter")
			}
			if err := child(parameter.Constraint, formals, scope); err != nil {
				return err
			}
		}
		for _, parameter := range value.Params {
			if err := child(parameter.Type, formals, scope); err != nil {
				return err
			}
		}
		if err := child(value.Variadic, formals, scope); err != nil {
			return err
		}
		for _, result := range value.Returns {
			if err := child(result, formals, scope); err != nil {
				return err
			}
		}
		switch len(value.Returns) {
		case 0:
			return nil
		case 1:
			return child(value.Returns[0], formals, scope)
		default:
			return child(typ.NewTuple(value.Returns...), formals, scope)
		}
	case *typ.Record:
		for _, field := range value.Fields {
			if err := child(typ.LiteralString(field.Name), current.formals, current.scope); err != nil {
				return err
			}
			if err := child(field.Type, current.formals, current.scope); err != nil {
				return err
			}
			if field.Optional {
				if err := child(typeexpr.Optional(field.Type), current.formals, current.scope); err != nil {
					return err
				}
			}
		}
		for _, member := range value.StaticMembers {
			var key typ.Type
			switch member.Kind {
			case typ.StaticMemberStringIndex:
				key = typ.LiteralString(member.Name)
			case typ.StaticMemberIntIndex:
				key = typ.LiteralInt(member.Index)
			default:
				return errors.New("typeauthority: unsupported Runtime static member key")
			}
			if err := child(key, current.formals, current.scope); err != nil {
				return err
			}
			if err := child(member.Type, current.formals, current.scope); err != nil {
				return err
			}
			if member.Optional {
				if err := child(typeexpr.Optional(member.Type), current.formals, current.scope); err != nil {
					return err
				}
			}
		}
		if err := child(value.Metatable, current.formals, current.scope); err != nil {
			return err
		}
		if value.HasMapComponent() {
			if err := child(value.MapKey, current.formals, current.scope); err != nil {
				return err
			}
			if err := child(value.MapValue, current.formals, current.scope); err != nil {
				return err
			}
		}
		return nil
	case *typ.Union:
		for _, member := range value.Members {
			if err := child(member, current.formals, current.scope); err != nil {
				return err
			}
		}
		return nil
	case *typ.Intersection:
		for _, member := range value.Members {
			if err := child(member, current.formals, current.scope); err != nil {
				return err
			}
		}
		return nil
	case *typ.Tuple:
		for index, element := range value.Elements {
			if err := child(typ.LiteralInt(int64(index+1)), current.formals, current.scope); err != nil {
				return err
			}
			if err := child(element, current.formals, current.scope); err != nil {
				return err
			}
		}
		return nil
	case *typ.Interface:
		for _, method := range value.Methods {
			if method.Type == nil {
				return errors.New("typeauthority: nil Runtime interface method")
			}
			if err := child(method.Type, current.formals, current.scope); err != nil {
				return err
			}
		}
		return nil
	case *typ.Meta:
		return child(value.Of, current.formals, current.scope)
	case *typ.Recursive:
		if value.Body == nil || value.Body == value {
			return errors.New("typeauthority: malformed Runtime recursive body")
		}
		return child(value.Body, current.formals, current.scope)
	case *typ.TypeParam:
		return child(value.Constraint, current.formals, current.scope)
	case *typ.Generic:
		formals := runtimeCombinedFormals(current.formals, value.TypeParams)
		scope := runtimeScope(value, current.formals)
		if !scope.Available() {
			return errors.New("typeauthority: unavailable Runtime generic binder identity")
		}
		for _, parameter := range value.TypeParams {
			if parameter == nil {
				return errors.New("typeauthority: nil Runtime generic parameter")
			}
			if err := child(parameter.Constraint, formals, scope); err != nil {
				return err
			}
		}
		if value.Body == nil {
			return errors.New("typeauthority: Runtime generic lacks body")
		}
		return child(value.Body, formals, scope)
	case *typ.Instantiated:
		if value.Generic == nil || value.Generic.Body == nil || len(value.TypeArgs) != len(value.Generic.TypeParams) {
			return errors.New("typeauthority: malformed Runtime instantiation")
		}
		if err := child(value.Generic, current.formals, current.scope); err != nil {
			return err
		}
		for _, argument := range value.TypeArgs {
			if err := child(argument, current.formals, current.scope); err != nil {
				return err
			}
		}
		expanded := subst.ExpandInstantiated(value)
		if expanded == nil || expanded == value {
			return errors.New("typeauthority: Runtime instantiation cannot expand")
		}
		return child(expanded, current.formals, current.scope)
	}
	return nil
}

func (b *runtimeBuilder) describe() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.pending) {
		return errors.New("typeauthority: malformed Runtime description")
	}
	for index := range b.pending {
		if err := b.describeOne(index); err != nil {
			return err
		}
	}
	return nil
}

func (b *runtimeBuilder) describeOne(index int) error {
	if index < 0 || index >= len(b.pending) {
		return errors.New("typeauthority: Runtime description index")
	}
	current := b.pending[index]
	row := &b.runtime.rows[index]
	row.form = runtimeForm(current.value)
	row.selfRewrite = subst.Self(current.value, typ.String) != current.value
	child := func(value typ.Type, formals []*typ.TypeParam, scope keyspace.ContentID) (runtimeChild, error) {
		if value == nil {
			return runtimeChild{}, nil
		}
		inner, err := b.child(value, formals, scope)
		return runtimeChild{inner: inner, present: err == nil}, err
	}
	appendUnnamed := func(values *[]runtimeChild, value typ.Type, formals []*typ.TypeParam, scope keyspace.ContentID) error {
		entry, err := child(value, formals, scope)
		if err != nil {
			return err
		}
		*values = append(*values, entry)
		return nil
	}
	appendNamed := func(values *[]runtimeNamedChild, name string, value typ.Type, optional, readonly bool, formals []*typ.TypeParam, scope keyspace.ContentID) error {
		entry, err := child(value, formals, scope)
		if err != nil {
			return err
		}
		*values = append(*values, runtimeNamedChild{name: name, child: entry, optional: optional, readonly: readonly})
		return nil
	}
	switch value := current.value.(type) {
	case *typ.Array:
		var err error
		row.element, err = child(value.Element, current.formals, current.scope)
		return err
	case *typ.Map:
		var err error
		if row.key, err = child(value.Key, current.formals, current.scope); err != nil {
			return err
		}
		row.value, err = child(value.Value, current.formals, current.scope)
		return err
	case *typ.ReadonlyMap:
		var err error
		if row.key, err = child(value.Key, current.formals, current.scope); err != nil {
			return err
		}
		row.value, err = child(value.Value, current.formals, current.scope)
		return err
	case *typ.Optional:
		var err error
		row.inner, err = child(value.Inner, current.formals, current.scope)
		return err
	case *typ.Function:
		formals := runtimeCombinedFormals(current.formals, value.TypeParams)
		scope := runtimeScope(value, current.formals)
		if !scope.Available() {
			return errors.New("typeauthority: unavailable Runtime function description scope")
		}
		if err := runtimeRangeStart(len(b.runtime.typeParameters), &row.typeParameters); err != nil {
			return err
		}
		for _, parameter := range value.TypeParams {
			if parameter == nil {
				return errors.New("typeauthority: nil Runtime function type parameter")
			}
			if err := appendNamed(&b.runtime.typeParameters, parameter.Name, parameter.Constraint, false, false, formals, scope); err != nil {
				return err
			}
		}
		if err := runtimeRangeEnd(len(b.runtime.typeParameters), &row.typeParameters); err != nil {
			return err
		}
		if err := runtimeRangeStart(len(b.runtime.parameters), &row.parameters); err != nil {
			return err
		}
		for _, parameter := range value.Params {
			entry, err := child(parameter.Type, formals, scope)
			if err != nil {
				return err
			}
			b.runtime.parameters = append(b.runtime.parameters, runtimeParameter{
				name: parameter.Name, child: entry, optional: parameter.Optional, receiver: parameter.Receiver,
			})
		}
		if err := runtimeRangeEnd(len(b.runtime.parameters), &row.parameters); err != nil {
			return err
		}
		var err error
		if row.variadic, err = child(value.Variadic, formals, scope); err != nil {
			return err
		}
		if err := runtimeRangeStart(len(b.runtime.results), &row.results); err != nil {
			return err
		}
		for _, result := range value.Returns {
			if err := appendUnnamed(&b.runtime.results, result, formals, scope); err != nil {
				return err
			}
		}
		if err := runtimeRangeEnd(len(b.runtime.results), &row.results); err != nil {
			return err
		}
		var result typ.Type
		switch len(value.Returns) {
		case 1:
			result = value.Returns[0]
		default:
			if len(value.Returns) > 1 {
				result = typ.NewTuple(value.Returns...)
			}
		}
		row.returns, err = child(result, formals, scope)
		return err
	case *typ.Record:
		row.open = value.Open
		row.metatableAny = typetable.IsMetatableUnconstrained(value.Metatable)
		if err := runtimeRangeStart(len(b.runtime.fields), &row.fields); err != nil {
			return err
		}
		for fieldIndex, field := range value.Fields {
			if fieldIndex != 0 && value.Fields[fieldIndex-1].Name >= field.Name {
				return errors.New("typeauthority: Runtime record fields are not strictly sorted")
			}
			key, err := child(typ.LiteralString(field.Name), current.formals, current.scope)
			if err != nil {
				return err
			}
			entry, err := child(field.Type, current.formals, current.scope)
			if err != nil {
				return err
			}
			effective := entry
			if field.Optional {
				effective, err = child(typeexpr.Optional(field.Type), current.formals, current.scope)
				if err != nil {
					return err
				}
			}
			b.runtime.fields = append(b.runtime.fields, runtimeNamedChild{
				name: field.Name, key: key, child: entry, effective: effective,
				optional: field.Optional, readonly: field.Readonly,
			})
		}
		if err := runtimeRangeEnd(len(b.runtime.fields), &row.fields); err != nil {
			return err
		}
		if err := runtimeRangeStart(len(b.runtime.staticMembers), &row.staticMembers); err != nil {
			return err
		}
		for memberIndex, member := range value.StaticMembers {
			if memberIndex != 0 && typ.CompareStaticMembers(value.StaticMembers[memberIndex-1], member) >= 0 {
				return errors.New("typeauthority: Runtime record static members are not strictly sorted")
			}
			var keyType typ.Type
			switch member.Kind {
			case typ.StaticMemberStringIndex:
				keyType = typ.LiteralString(member.Name)
			case typ.StaticMemberIntIndex:
				keyType = typ.LiteralInt(member.Index)
			default:
				return errors.New("typeauthority: unsupported Runtime static member key")
			}
			key, err := child(keyType, current.formals, current.scope)
			if err != nil {
				return err
			}
			entry, err := child(member.Type, current.formals, current.scope)
			if err != nil {
				return err
			}
			effective := entry
			if member.Optional {
				effective, err = child(typeexpr.Optional(member.Type), current.formals, current.scope)
				if err != nil {
					return err
				}
			}
			b.runtime.staticMembers = append(b.runtime.staticMembers, runtimeStaticChild{
				kind: member.Kind, stringKey: member.Kind == typ.StaticMemberStringIndex,
				name: member.Name, integer: member.Index, key: key, child: entry, effective: effective,
				optional: member.Optional, readonly: member.Readonly,
			})
		}
		if err := runtimeRangeEnd(len(b.runtime.staticMembers), &row.staticMembers); err != nil {
			return err
		}
		var err error
		if row.metatable, err = child(value.Metatable, current.formals, current.scope); err != nil {
			return err
		}
		if value.HasMapComponent() {
			if row.key, err = child(value.MapKey, current.formals, current.scope); err != nil {
				return err
			}
			if row.value, err = child(value.MapValue, current.formals, current.scope); err != nil {
				return err
			}
		}
		return nil
	case *typ.Union:
		if err := runtimeRangeStart(len(b.runtime.variants), &row.variants); err != nil {
			return err
		}
		for _, member := range value.Members {
			if err := appendUnnamed(&b.runtime.variants, member, current.formals, current.scope); err != nil {
				return err
			}
		}
		return runtimeRangeEnd(len(b.runtime.variants), &row.variants)
	case *typ.Intersection:
		if err := runtimeRangeStart(len(b.runtime.variants), &row.variants); err != nil {
			return err
		}
		for _, member := range value.Members {
			if err := appendUnnamed(&b.runtime.variants, member, current.formals, current.scope); err != nil {
				return err
			}
		}
		return runtimeRangeEnd(len(b.runtime.variants), &row.variants)
	case *typ.Tuple:
		if err := runtimeRangeStart(len(b.runtime.elements), &row.elements); err != nil {
			return err
		}
		for index, element := range value.Elements {
			key, err := child(typ.LiteralInt(int64(index+1)), current.formals, current.scope)
			if err != nil {
				return err
			}
			entry, err := child(element, current.formals, current.scope)
			if err != nil {
				return err
			}
			b.runtime.elements = append(b.runtime.elements, runtimeTupleElement{key: key, child: entry})
		}
		return runtimeRangeEnd(len(b.runtime.elements), &row.elements)
	case *typ.Interface:
		row.name = value.Name
		row.tableTop = value.Name == typ.BuiltinTableTopName && len(value.Methods) == 0
		if err := runtimeRangeStart(len(b.runtime.methods), &row.methods); err != nil {
			return err
		}
		for _, method := range value.Methods {
			if method.Type == nil {
				return errors.New("typeauthority: nil Runtime interface method")
			}
			if err := appendNamed(&b.runtime.methods, method.Name, method.Type, false, true, current.formals, current.scope); err != nil {
				return err
			}
		}
		return runtimeRangeEnd(len(b.runtime.methods), &row.methods)
	case *typ.Meta:
		var err error
		row.inner, err = child(value.Of, current.formals, current.scope)
		return err
	case *typ.Recursive:
		row.name = value.Name
		if value.Body == nil || value.Body == value {
			return errors.New("typeauthority: malformed Runtime recursive body")
		}
		var err error
		row.body, err = child(value.Body, current.formals, current.scope)
		return err
	case *typ.TypeParam:
		row.name = value.Name
		var err error
		row.inner, err = child(value.Constraint, current.formals, current.scope)
		return err
	case *typ.Literal:
		row.literal.base = value.Base
		switch value.Base {
		case kind.Boolean:
			literal, ok := value.Value.(bool)
			if !ok {
				return errors.New("typeauthority: malformed Runtime boolean literal")
			}
			if literal {
				row.literal.bits = 1
			}
		case kind.Integer:
			literal, ok := value.Value.(int64)
			if !ok {
				return errors.New("typeauthority: malformed Runtime integer literal")
			}
			row.literal.bits = uint64(literal)
		case kind.Number:
			literal, ok := value.Value.(float64)
			if !ok {
				return errors.New("typeauthority: malformed Runtime number literal")
			}
			row.literal.bits = math.Float64bits(literal)
		case kind.String:
			literal, ok := value.Value.(string)
			if !ok {
				return errors.New("typeauthority: malformed Runtime string literal")
			}
			row.literal.text = literal
		default:
			return errors.New("typeauthority: unsupported Runtime literal base")
		}
		return nil
	case *typ.Generic:
		row.name = value.Name
		formals := runtimeCombinedFormals(current.formals, value.TypeParams)
		scope := runtimeScope(value, current.formals)
		if !scope.Available() {
			return errors.New("typeauthority: unavailable Runtime generic description scope")
		}
		if err := runtimeRangeStart(len(b.runtime.typeParameters), &row.typeParameters); err != nil {
			return err
		}
		for _, parameter := range value.TypeParams {
			if parameter == nil {
				return errors.New("typeauthority: nil Runtime generic parameter")
			}
			if err := appendNamed(&b.runtime.typeParameters, parameter.Name, parameter.Constraint, false, false, formals, scope); err != nil {
				return err
			}
		}
		if err := runtimeRangeEnd(len(b.runtime.typeParameters), &row.typeParameters); err != nil {
			return err
		}
		if value.Body == nil {
			return errors.New("typeauthority: Runtime generic lacks body")
		}
		var err error
		row.body, err = child(value.Body, formals, scope)
		return err
	case *typ.Instantiated:
		if value.Generic == nil || value.Generic.Body == nil || len(value.TypeArgs) != len(value.Generic.TypeParams) {
			return errors.New("typeauthority: malformed Runtime instantiation")
		}
		var err error
		row.base, err = child(value.Generic, current.formals, current.scope)
		if err != nil {
			return err
		}
		if err := runtimeRangeStart(len(b.runtime.arguments), &row.arguments); err != nil {
			return err
		}
		for _, argument := range value.TypeArgs {
			inner, err := b.child(argument, current.formals, current.scope)
			if err != nil {
				return err
			}
			b.runtime.arguments = append(b.runtime.arguments, inner)
		}
		if err := runtimeRangeEnd(len(b.runtime.arguments), &row.arguments); err != nil {
			return err
		}
		if uint64(len(b.runtime.instantiations)) >= uint64(math.MaxUint32) {
			return errors.New("typeauthority: Runtime instantiation handle overflow")
		}
		self := RuntimeInner{owner: b.runtime, index: uint32(index + 1)}
		b.runtime.instantiations = append(b.runtime.instantiations, runtimeInstantiation{result: self})
		expanded := subst.ExpandInstantiated(value)
		if expanded == nil || expanded == value {
			return errors.New("typeauthority: Runtime instantiation cannot expand")
		}
		row.expansion, err = child(expanded, current.formals, current.scope)
		return err
	default:
		if row.form == FormOther {
			return errors.New("typeauthority: unsupported Runtime description")
		}
	}
	return nil
}

func runtimeRangeStart(length int, target *runtimeRange) error {
	if target == nil || length < 0 || uint64(length) > uint64(math.MaxUint32) {
		return errors.New("typeauthority: Runtime reflection range overflow")
	}
	target.start = uint32(length)
	return nil
}

func runtimeRangeEnd(length int, target *runtimeRange) error {
	if target == nil || length < 0 || uint64(length) > uint64(math.MaxUint32) {
		return errors.New("typeauthority: Runtime reflection range overflow")
	}
	target.end = uint32(length)
	return nil
}

// sealInstantiationTrie converts the complete fixed-instantiation relation
// into a dense-root, sorted compressed sparse row trie. Construction maps are
// cold and discarded here. Every hot argument step is one binary search over
// the current node's sorted outgoing edges, so exact lookup is
// O(arity*log(max-degree)); Begin and Finish are O(1). No hash collision,
// caller allocation, typ traversal, or semantic work cap participates.
func (r *Runtime) sealInstantiationTrie() error {
	if r == nil || r.instantiationRoots != nil || r.instantiationTrieNodes != nil || r.instantiationTrieEdges != nil {
		return errors.New("typeauthority: invalid Runtime instantiation trie seal")
	}
	buildRoots := make(map[uint32]*runtimeInstantiationTrieBuildNode)
	for _, instantiation := range r.instantiations {
		if !r.owns(instantiation.result) {
			return errors.New("typeauthority: malformed Runtime instantiation row")
		}
		row := r.rows[instantiation.result.index-1]
		if row.form != FormInstantiated || !row.base.present || !r.owns(row.base.inner) ||
			row.arguments.start > row.arguments.end || uint64(row.arguments.end) > uint64(len(r.arguments)) {
			return errors.New("typeauthority: malformed Runtime instantiation row")
		}
		root := buildRoots[row.base.inner.index]
		if root == nil {
			root = &runtimeInstantiationTrieBuildNode{}
			buildRoots[row.base.inner.index] = root
		}
		current := root
		for _, argument := range r.arguments[row.arguments.start:row.arguments.end] {
			if !r.owns(argument) {
				return errors.New("typeauthority: foreign Runtime instantiation argument")
			}
			if current.children == nil {
				current.children = make(map[uint32]*runtimeInstantiationTrieBuildNode)
			}
			next := current.children[argument.index]
			if next == nil {
				next = &runtimeInstantiationTrieBuildNode{}
				current.children[argument.index] = next
			}
			current = next
		}
		if current.result.owner != nil && !r.equal(current.result, instantiation.result) {
			return errors.New("typeauthority: conflicting fixed Runtime instantiation")
		}
		current.result = instantiation.result
	}

	r.instantiationRoots = make([]uint32, len(r.rows))
	queue := make([]*runtimeInstantiationTrieBuildNode, 0, len(buildRoots))
	appendNode := func(node *runtimeInstantiationTrieBuildNode) (uint32, error) {
		if node == nil {
			return 0, errors.New("typeauthority: nil Runtime instantiation trie node")
		}
		ordinal, err := runtimeDenseOrdinal(len(r.instantiationTrieNodes))
		if err != nil {
			return 0, err
		}
		r.instantiationTrieNodes = append(r.instantiationTrieNodes, runtimeInstantiationTrieNode{result: node.result})
		queue = append(queue, node)
		return ordinal, nil
	}
	for index := range r.instantiationRoots {
		base := uint32(index + 1)
		root := buildRoots[base]
		if root == nil {
			continue
		}
		ordinal, err := appendNode(root)
		if err != nil {
			return err
		}
		r.instantiationRoots[index] = ordinal
	}
	for cursor := 0; cursor < len(queue); cursor++ {
		current := queue[cursor]
		var edges runtimeRange
		if err := runtimeRangeStart(len(r.instantiationTrieEdges), &edges); err != nil {
			return err
		}
		arguments := make([]uint32, 0, len(current.children))
		for argument := range current.children {
			arguments = append(arguments, argument)
		}
		sort.Slice(arguments, func(left, right int) bool { return arguments[left] < arguments[right] })
		for _, argument := range arguments {
			if uint64(len(r.instantiationTrieEdges)) >= uint64(math.MaxUint32) {
				return errors.New("typeauthority: Runtime instantiation trie edge overflow")
			}
			child, err := appendNode(current.children[argument])
			if err != nil {
				return err
			}
			r.instantiationTrieEdges = append(r.instantiationTrieEdges, runtimeInstantiationTrieEdge{argument: argument, child: child})
		}
		if err := runtimeRangeEnd(len(r.instantiationTrieEdges), &edges); err != nil {
			return err
		}
		r.instantiationTrieNodes[cursor].edges = edges
	}
	return nil
}

// sealCanonical publishes structural identity only. runtimeBuilder.add has
// already interned equal canonical bytes into one dense row; assignability is
// deliberately not an equality relation and must never rewrite this vector.
func (b *runtimeBuilder) sealCanonical() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) {
		return errors.New("typeauthority: malformed Runtime canonical source")
	}
	runtime := b.runtime
	runtime.canonical = make([]uint32, len(runtime.rows)+1)
	for index := range runtime.rows {
		runtime.canonical[index+1] = uint32(index + 1)
		if !runtime.rows[index].closed {
			continue
		}
		encoded := runtime.rows[index].encoded
		if len(encoded) == 0 {
			return errors.New("typeauthority: closed Runtime row lacks canonical structural identity")
		}
	}
	return nil
}

// sealDescriptors derives the canonical finite semantic-union description of
// every sealed row.  Runtime rows remain complete structural atoms; only
// direct Union rows flatten into an antichain.  The construction graph is
// released by SealRuntime, so this is the sole place where typ values are
// interpreted for the descriptor algebra.
func (b *runtimeBuilder) sealDescriptors() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) || len(b.runtime.canonical) != len(b.runtime.rows)+1 {
		return errors.New("typeauthority: malformed Runtime descriptor source")
	}
	runtime := b.runtime
	state := make([]uint8, len(runtime.rows))
	var visit func(int) ([]uint32, error)
	visit = func(index int) ([]uint32, error) {
		if index < 0 || index >= len(runtime.rows) {
			return nil, errors.New("typeauthority: Runtime descriptor index")
		}
		if runtime.rows[index].atoms != nil {
			return runtime.rows[index].atoms, nil
		}
		if state[index] == 1 {
			// A recursive union backedge is itself a complete atom. The
			// subsequent antichain reduction observes the already sealed
			// coinductive relation and removes it when a productive arm covers
			// the cycle.
			return []uint32{runtime.canonical[index+1]}, nil
		}
		state[index] = 1
		atoms := make([]uint32, 0, 1)
		row := runtime.rows[index]
		switch row.form {
		case FormUnion:
			if row.variants.start > row.variants.end || uint64(row.variants.end) > uint64(len(runtime.variants)) {
				return nil, errors.New("typeauthority: malformed Runtime union descriptor range")
			}
			for _, child := range runtime.variants[row.variants.start:row.variants.end] {
				if !child.present || !runtime.owns(child.inner) {
					return nil, errors.New("typeauthority: malformed Runtime union descriptor child")
				}
				childAtoms, err := visit(int(child.inner.index - 1))
				if err != nil {
					return nil, err
				}
				atoms = append(atoms, childAtoms...)
			}
		case FormOptional:
			// Optional is the canonical representation of nil|T. Keep the
			// nil atom explicit so ClassSet's union algebra agrees with the
			// authored nil and T classes.
			for nilIndex, candidate := range runtime.rows {
				if candidate.form != FormNil {
					continue
				}
				atoms = append(atoms, runtime.canonical[uint32(nilIndex+1)])
				break
			}
			if row.inner.present && runtime.owns(row.inner.inner) {
				childAtoms, err := visit(int(row.inner.inner.index - 1))
				if err != nil {
					return nil, err
				}
				atoms = append(atoms, childAtoms...)
			}
		}
		if len(atoms) == 0 {
			atoms = append(atoms, runtime.canonical[index+1])
		}
		for atomIndex := range atoms {
			atom := atoms[atomIndex]
			if atom == 0 || uint64(atom) > uint64(len(runtime.rows)) {
				return nil, errors.New("typeauthority: malformed Runtime semantic atom")
			}
			atoms[atomIndex] = runtime.canonical[atom]
		}
		sort.Slice(atoms, func(left, right int) bool { return atoms[left] < atoms[right] })
		unique := atoms[:0]
		for _, atom := range atoms {
			if len(unique) == 0 || unique[len(unique)-1] != atom {
				unique = append(unique, atom)
			}
		}
		if len(unique) == 0 {
			unique = append(unique, runtime.canonical[index+1])
		}
		runtime.rows[index].atoms = append([]uint32(nil), unique...)
		state[index] = 2
		return runtime.rows[index].atoms, nil
	}
	for index := range runtime.rows {
		if _, err := visit(index); err != nil {
			return err
		}
	}
	return nil
}

// sealRanks assigns the exact-singleton instance of the structural finite-set
// measure. Every closed row is one distinct atom, so each singleton has rank
// 1+|P\\{atom}| = |P|. No assignability relation participates.
func (r *Runtime) sealRanks() error {
	if r == nil || len(r.canonical) != len(r.rows)+1 {
		return errors.New("typeauthority: malformed Runtime rank source")
	}
	closed := uint64(0)
	for _, row := range r.rows {
		if row.closed {
			closed++
		}
	}
	if closed == 0 || closed >= uint64(math.MaxUint32) {
		return errors.New("typeauthority: Runtime rank universe overflow")
	}
	for index := range r.rows {
		if r.rows[index].closed {
			r.rows[index].rank = uint32(closed)
		}
	}
	return nil
}

func (r *Runtime) sealIdentity() error {
	if r == nil || r.source == nil || !r.source.ContentID().Available() {
		return errors.New("typeauthority: unavailable Runtime identity source")
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.typeauthority.runtime\x00\x03"))
	sourceID := r.source.ContentID()
	_, _ = hash.Write(sourceID[:])
	writeRuntimeWord(hash, uint64(len(r.rows)))
	for _, row := range r.rows {
		_, _ = hash.Write([]byte{byte(row.form)})
		if row.closed {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
		writeRuntimeWord(hash, uint64(len(row.encoded)))
		_, _ = hash.Write(row.encoded)
		writeRuntimeWord(hash, uint64(len(row.name)))
		_, _ = hash.Write([]byte(row.name))
		_, _ = hash.Write([]byte{byte(row.literal.base)})
		writeRuntimeWord(hash, row.literal.bits)
		writeRuntimeWord(hash, uint64(len(row.literal.text)))
		_, _ = hash.Write([]byte(row.literal.text))
		for _, flag := range [...]bool{row.open, row.tableTop, row.metatableAny, row.selfRewrite} {
			if flag {
				_, _ = hash.Write([]byte{1})
			} else {
				_, _ = hash.Write([]byte{0})
			}
		}
		for _, child := range [...]runtimeChild{row.element, row.key, row.value, row.base, row.inner, row.returns, row.variadic, row.body, row.metatable, row.expansion} {
			writeRuntimeWord(hash, uint64(child.inner.index))
			if child.present {
				_, _ = hash.Write([]byte{1})
			} else {
				_, _ = hash.Write([]byte{0})
			}
		}
		for _, range_ := range [...]runtimeRange{row.fields, row.staticMembers, row.variants, row.elements, row.parameters, row.results, row.methods, row.typeParameters, row.arguments} {
			writeRuntimeWord(hash, uint64(range_.start))
			writeRuntimeWord(hash, uint64(range_.end))
		}
	}
	writeRuntimeWord(hash, uint64(len(r.canonical)))
	for _, canonical := range r.canonical {
		writeRuntimeWord(hash, uint64(canonical))
	}
	for _, row := range r.rows {
		writeRuntimeWord(hash, uint64(len(row.atoms)))
		for _, atom := range row.atoms {
			writeRuntimeWord(hash, uint64(atom))
		}
		writeRuntimeWord(hash, uint64(row.rank))
	}
	writeRuntimeNamedChildren(hash, r.fields)
	writeRuntimeStaticChildren(hash, r.staticMembers)
	writeRuntimeChildren(hash, r.variants)
	writeRuntimeTupleElements(hash, r.elements)
	writeRuntimeParameters(hash, r.parameters)
	writeRuntimeChildren(hash, r.results)
	writeRuntimeNamedChildren(hash, r.methods)
	writeRuntimeNamedChildren(hash, r.typeParameters)
	writeRuntimeWord(hash, uint64(len(r.instantiations)))
	for _, row := range r.instantiations {
		writeRuntimeWord(hash, uint64(row.result.index))
	}
	writeRuntimeWord(hash, uint64(len(r.arguments)))
	for _, argument := range r.arguments {
		writeRuntimeWord(hash, uint64(argument.index))
	}
	copy(r.id[:], hash.Sum(nil))
	if !r.id.Available() {
		return errors.New("typeauthority: unavailable Runtime content identity")
	}
	r.identities = make([]keyspace.ContentID, len(r.rows))
	for index := range r.rows {
		innerID := sha256.New()
		_, _ = innerID.Write([]byte("wippy.analysis.typeauthority.runtime/inner\x00\x01"))
		_, _ = innerID.Write(r.id[:])
		writeRuntimeWord(innerID, uint64(index+1))
		copy(r.identities[index][:], innerID.Sum(nil))
		if !r.identities[index].Available() {
			return errors.New("typeauthority: unavailable Runtime inner identity")
		}
	}
	return nil
}

func writeRuntimeWord(hash interface{ Write([]byte) (int, error) }, value uint64) {
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], value)
	_, _ = hash.Write(word[:])
}

func writeRuntimeChild(hash interface{ Write([]byte) (int, error) }, child runtimeChild) {
	writeRuntimeWord(hash, uint64(child.inner.index))
	if child.present {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
}

func writeRuntimeChildren(hash interface{ Write([]byte) (int, error) }, values []runtimeChild) {
	writeRuntimeWord(hash, uint64(len(values)))
	for _, value := range values {
		writeRuntimeChild(hash, value)
	}
}

func writeRuntimeNamedChildren(hash interface{ Write([]byte) (int, error) }, values []runtimeNamedChild) {
	writeRuntimeWord(hash, uint64(len(values)))
	for _, value := range values {
		writeRuntimeWord(hash, uint64(len(value.name)))
		_, _ = hash.Write([]byte(value.name))
		writeRuntimeChild(hash, value.key)
		writeRuntimeChild(hash, value.child)
		writeRuntimeChild(hash, value.effective)
		if value.optional {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
		if value.readonly {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
	}
}

func writeRuntimeStaticChildren(hash interface{ Write([]byte) (int, error) }, values []runtimeStaticChild) {
	writeRuntimeWord(hash, uint64(len(values)))
	for _, value := range values {
		_, _ = hash.Write([]byte{byte(value.kind)})
		writeRuntimeWord(hash, uint64(len(value.name)))
		_, _ = hash.Write([]byte(value.name))
		writeRuntimeWord(hash, uint64(value.integer))
		writeRuntimeChild(hash, value.key)
		writeRuntimeChild(hash, value.child)
		writeRuntimeChild(hash, value.effective)
		if value.optional {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
		if value.readonly {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
	}
}

func writeRuntimeTupleElements(hash interface{ Write([]byte) (int, error) }, values []runtimeTupleElement) {
	writeRuntimeWord(hash, uint64(len(values)))
	for _, value := range values {
		writeRuntimeChild(hash, value.key)
		writeRuntimeChild(hash, value.child)
	}
}

func writeRuntimeParameters(hash interface{ Write([]byte) (int, error) }, values []runtimeParameter) {
	writeRuntimeWord(hash, uint64(len(values)))
	for _, value := range values {
		writeRuntimeWord(hash, uint64(len(value.name)))
		_, _ = hash.Write([]byte(value.name))
		writeRuntimeChild(hash, value.child)
		if value.optional {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
		if value.receiver {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
	}
}

// LinkID identifies the exact sealed Link Runtime is fenced to. It is a
// composition check, not a portable inner identity; Inner identity is always
// derived from Runtime.ContentID plus the immutable dense row.
func (r *Runtime) LinkID() keyspace.ContentID {
	if r == nil || r.source == nil {
		return keyspace.ContentID{}
	}
	return r.source.ContentID()
}

// Link returns Runtime's exact Link owner. Boundary Values are pointer-fenced
// hot capabilities; equal content identities do not permit cross-Link reuse.
func (r *Runtime) Link() *link.Link {
	if r == nil {
		return nil
	}
	return r.source
}

func (r *Runtime) ContentID() keyspace.ContentID {
	if r == nil {
		return keyspace.ContentID{}
	}
	return r.id
}

func (r *Runtime) Count() int {
	if r == nil {
		return 0
	}
	return len(r.rows)
}

func (r *Runtime) At(index int) (RuntimeInner, bool) {
	if r == nil || index < 0 || index >= len(r.rows) {
		return RuntimeInner{}, false
	}
	return RuntimeInner{owner: r, index: uint32(index + 1)}, true
}

// InnerAtIndex authenticates a Runtime-local one-based atom index. It is a
// cold projection used by Static's descriptor builder; callers cannot mint a
// handle because the Runtime owner remains private inside RuntimeInner.
func (r *Runtime) InnerAtIndex(index uint32) (RuntimeInner, bool) {
	if r == nil || index == 0 || uint64(index) > uint64(len(r.rows)) {
		return RuntimeInner{}, false
	}
	return RuntimeInner{owner: r, index: index}, true
}

func (r *Runtime) Index(inner RuntimeInner) (uint32, bool) {
	if !r.owns(inner) {
		return 0, false
	}
	return inner.index, true
}

func (r *Runtime) owns(inner RuntimeInner) bool {
	return r != nil && inner.owner == r && inner.index != 0 && uint64(inner.index) <= uint64(len(r.rows))
}

func (r *Runtime) equal(left, right RuntimeInner) bool {
	return r.owns(left) && r.owns(right) && left.index == right.index
}

func (r *Runtime) Equal(left, right RuntimeInner) bool { return r.equal(left, right) }

// Fingerprint is the allocation-free Runtime-local identity of an admitted
// inner. Its canonical global counterpart is Identity.
func (r *Runtime) Fingerprint(inner RuntimeInner) uint64 {
	if !r.owns(inner) {
		return 0
	}
	return uint64(inner.index)
}

func (r *Runtime) Identity(inner RuntimeInner) (keyspace.ContentID, bool) {
	if !r.owns(inner) || int(inner.index) > len(r.identities) {
		return keyspace.ContentID{}, false
	}
	id := r.identities[inner.index-1]
	return id, id.Available()
}

// Identity exposes an inner's canonical Runtime-scoped identity without
// granting access to its private dense coordinate.
func (inner RuntimeInner) Identity() (keyspace.ContentID, bool) {
	if inner.owner == nil {
		return keyspace.ContentID{}, false
	}
	return inner.owner.Identity(inner)
}

func (r *Runtime) Form(inner RuntimeInner) (Form, bool) {
	if !r.owns(inner) {
		return 0, false
	}
	return r.rows[inner.index-1].form, true
}

// CanonicalEncoding returns an ownership-isolated portable reconstruction of
// one closed row. Runtime never exposes or retains a typ.Type graph.
func (r *Runtime) CanonicalEncoding(inner RuntimeInner) ([]byte, bool) {
	if !r.owns(inner) || !r.rows[inner.index-1].closed || len(r.rows[inner.index-1].encoded) == 0 {
		return nil, false
	}
	return append([]byte(nil), r.rows[inner.index-1].encoded...), true
}

// StructuralEqual is a fully sealed three-valued structural judgment. A
// scoped formal child is exact only against its own identity; a comparison to
// another scoped child remains intentionally undecided rather than treating
// two unrelated lexical binders as presentation-equal types.
func (r *Runtime) StructuralEqual(left, right RuntimeInner) (answer, decided bool) {
	if !r.owns(left) || !r.owns(right) {
		return false, false
	}
	if left.index == right.index {
		return true, true
	}
	return false, r.rows[left.index-1].closed && r.rows[right.index-1].closed
}

// Subtype is the sealed allocation-free owner-local subtype judgment.
func (r *Runtime) Subtype(left, right RuntimeInner) (answer, decided bool) {
	if !r.owns(left) || !r.owns(right) {
		return false, false
	}
	if left.index == right.index {
		return true, true
	}
	if !r.rows[left.index-1].closed || !r.rows[right.index-1].closed {
		return false, false
	}
	return r.runtimeRowSubtype(left.index, right.index)
}

// Canonical returns the semantic-equivalence representative used by Runtime
// descriptors. Exact Runtime identities remain distinct: this projection is
// only the quotient consumed by union algebra (not StructuralEqual).
func (r *Runtime) Canonical(inner RuntimeInner) (RuntimeInner, bool) {
	if !r.owns(inner) || len(r.canonical) != len(r.rows)+1 || r.canonical[inner.index] == 0 {
		return RuntimeInner{}, false
	}
	return RuntimeInner{owner: r, index: r.canonical[inner.index]}, true
}

// DescriptorCount and DescriptorAt expose Runtime's immutable semantic-union
// descriptor without exposing dense row identities or construction graphs.
// The descriptor is a cold seal artifact; callers should retain only the
// returned atom handles they need for their own owner-fenced algebra.
func (r *Runtime) DescriptorCount(inner RuntimeInner) int {
	if !r.owns(inner) {
		return 0
	}
	return len(r.rows[inner.index-1].atoms)
}

func (r *Runtime) DescriptorAt(inner RuntimeInner, index int) (RuntimeInner, bool) {
	if !r.owns(inner) || index < 0 || index >= len(r.rows[inner.index-1].atoms) {
		return RuntimeInner{}, false
	}
	atom := r.rows[inner.index-1].atoms[index]
	if atom == 0 || uint64(atom) > uint64(len(r.rows)) {
		return RuntimeInner{}, false
	}
	return RuntimeInner{owner: r, index: atom}, true
}

func (r *Runtime) Rank(inner RuntimeInner) (uint64, bool) {
	if !r.owns(inner) || !r.rows[inner.index-1].closed || r.rows[inner.index-1].rank == 0 {
		return 0, false
	}
	return uint64(r.rows[inner.index-1].rank), true
}

func (r *Runtime) Element(inner RuntimeInner) (RuntimeInner, bool) {
	return r.directChild(inner, 0)
}

func (r *Runtime) Inner(inner RuntimeInner) (RuntimeInner, bool) {
	return r.directChild(inner, 1)
}

func (r *Runtime) Return(inner RuntimeInner) (RuntimeInner, bool) {
	return r.directChild(inner, 2)
}

func (r *Runtime) directChild(inner RuntimeInner, selector uint8) (RuntimeInner, bool) {
	if !r.owns(inner) {
		return RuntimeInner{}, false
	}
	row := r.rows[inner.index-1]
	var child runtimeChild
	switch selector {
	case 0:
		child = row.element
	case 1:
		child = row.inner
	case 2:
		child = row.returns
	default:
		return RuntimeInner{}, false
	}
	return child.inner, child.present
}

func (r *Runtime) Mapping(inner RuntimeInner) (RuntimeInner, RuntimeInner, bool) {
	if !r.owns(inner) {
		return RuntimeInner{}, RuntimeInner{}, false
	}
	row := r.rows[inner.index-1]
	return row.key.inner, row.value.inner, row.key.present && row.value.present
}

func (r *Runtime) FieldCount(inner RuntimeInner) int {
	if !r.owns(inner) {
		return 0
	}
	range_ := r.rows[inner.index-1].fields
	return int(range_.end - range_.start)
}

func (r *Runtime) VariantCount(inner RuntimeInner) int {
	if !r.owns(inner) {
		return 0
	}
	range_ := r.rows[inner.index-1].variants
	return int(range_.end - range_.start)
}

func (r *Runtime) ParameterCount(inner RuntimeInner) int {
	if !r.owns(inner) {
		return 0
	}
	range_ := r.rows[inner.index-1].parameters
	return int(range_.end - range_.start)
}

func (r *Runtime) TypeParameterCount(inner RuntimeInner) int {
	if !r.owns(inner) {
		return 0
	}
	range_ := r.rows[inner.index-1].typeParameters
	return int(range_.end - range_.start)
}

func (r *Runtime) FieldAt(inner RuntimeInner, index int) (string, RuntimeInner, bool, bool) {
	entry, ok := r.namedAt(inner, index, 0)
	return entry.name, entry.child.inner, entry.child.present, ok
}

// Field resolves one already-sealed record field by the canonical field order
// emitted by typ.Record. It is a tight binary search over dense data and does
// not decode or inspect a type graph.
func (r *Runtime) Field(inner RuntimeInner, name string) (RuntimeInner, bool, bool) {
	if !r.owns(inner) {
		return RuntimeInner{}, false, false
	}
	range_ := r.rows[inner.index-1].fields
	low, high := int(range_.start), int(range_.end)
	for low < high {
		middle := low + (high-low)/2
		if r.fields[middle].name < name {
			low = middle + 1
		} else {
			high = middle
		}
	}
	if low == int(range_.end) || r.fields[low].name != name {
		return RuntimeInner{}, false, false
	}
	child := r.fields[low].child
	return child.inner, child.present, true
}

func (r *Runtime) VariantAt(inner RuntimeInner, index int) (RuntimeInner, bool, bool) {
	return r.unnamedAt(inner, index, 0)
}

func (r *Runtime) ParameterAt(inner RuntimeInner, index int) (RuntimeInner, bool, bool) {
	if !r.owns(inner) || index < 0 {
		return RuntimeInner{}, false, false
	}
	range_ := r.rows[inner.index-1].parameters
	if uint64(index) >= uint64(range_.end-range_.start) || uint64(range_.end) > uint64(len(r.parameters)) {
		return RuntimeInner{}, false, false
	}
	child := r.parameters[range_.start+uint32(index)].child
	return child.inner, child.present, true
}

func (r *Runtime) TypeParameterAt(inner RuntimeInner, index int) (string, RuntimeInner, bool, bool) {
	entry, ok := r.namedAt(inner, index, 1)
	return entry.name, entry.child.inner, entry.child.present, ok
}

func (r *Runtime) namedAt(inner RuntimeInner, index int, source uint8) (runtimeNamedChild, bool) {
	if !r.owns(inner) || index < 0 {
		return runtimeNamedChild{}, false
	}
	row := r.rows[inner.index-1]
	range_ := row.fields
	values := r.fields
	if source == 1 {
		range_, values = row.typeParameters, r.typeParameters
	}
	if uint64(index) >= uint64(range_.end-range_.start) {
		return runtimeNamedChild{}, false
	}
	return values[range_.start+uint32(index)], true
}

func (r *Runtime) unnamedAt(inner RuntimeInner, index int, source uint8) (RuntimeInner, bool, bool) {
	if !r.owns(inner) || index < 0 {
		return RuntimeInner{}, false, false
	}
	row := r.rows[inner.index-1]
	range_ := row.variants
	values := r.variants
	if source != 0 {
		return RuntimeInner{}, false, false
	}
	if uint64(index) >= uint64(range_.end-range_.start) {
		return RuntimeInner{}, false, false
	}
	child := values[range_.start+uint32(index)]
	return child.inner, child.present, true
}

func (r *Runtime) InstantiationCount() int {
	if r == nil {
		return 0
	}
	return len(r.instantiations)
}

// InstantiationAt reports a fixed sealed instantiation row without allocating
// a transient argument slice. InstantiationArgumentAt exposes each argument
// by dense offset for hot TypeValue closure construction.
func (r *Runtime) InstantiationAt(index int) (RuntimeInner, RuntimeInner, int, bool) {
	if r == nil || index < 0 || index >= len(r.instantiations) {
		return RuntimeInner{}, RuntimeInner{}, 0, false
	}
	instantiation := r.instantiations[index]
	if !r.owns(instantiation.result) {
		return RuntimeInner{}, RuntimeInner{}, 0, false
	}
	row := r.rows[instantiation.result.index-1]
	if row.form != FormInstantiated || !row.base.present || !r.owns(row.base.inner) || row.arguments.start > row.arguments.end || uint64(row.arguments.end) > uint64(len(r.arguments)) {
		return RuntimeInner{}, RuntimeInner{}, 0, false
	}
	return instantiation.result, row.base.inner, int(row.arguments.end - row.arguments.start), true
}

func (r *Runtime) InstantiationArgumentAt(index, argument int) (RuntimeInner, bool) {
	if r == nil || index < 0 || argument < 0 || index >= len(r.instantiations) {
		return RuntimeInner{}, false
	}
	result := r.instantiations[index].result
	if !r.owns(result) {
		return RuntimeInner{}, false
	}
	row := r.rows[result.index-1]
	if row.form != FormInstantiated || row.arguments.start > row.arguments.end || uint64(row.arguments.end) > uint64(len(r.arguments)) {
		return RuntimeInner{}, false
	}
	range_ := row.arguments
	if uint64(argument) >= uint64(range_.end-range_.start) {
		return RuntimeInner{}, false
	}
	return r.arguments[range_.start+uint32(argument)], true
}

// BeginInstantiation starts one allocation-free exact lookup at base. Dense
// base dispatch is O(1). A false result means this Runtime has no fixed
// instantiation whose base is the supplied inner.
func (r *Runtime) BeginInstantiation(base RuntimeInner) (InstantiationMatch, bool) {
	if !r.owns(base) || uint64(base.index) > uint64(len(r.instantiationRoots)) {
		return InstantiationMatch{}, false
	}
	node := r.instantiationRoots[base.index-1]
	if node == 0 || uint64(node) > uint64(len(r.instantiationTrieNodes)) {
		return InstantiationMatch{}, false
	}
	return InstantiationMatch{owner: r, node: node}, true
}

// MatchInstantiationArgument advances one trie edge. Edges are sorted by
// RuntimeInner index and stored in CSR form, so each step is O(log degree)
// with no hash collision, allocation, or typ traversal.
func (r *Runtime) MatchInstantiationArgument(match InstantiationMatch, argument RuntimeInner) (InstantiationMatch, bool) {
	if !r.ownsInstantiationMatch(match) || !r.owns(argument) {
		return InstantiationMatch{}, false
	}
	range_ := r.instantiationTrieNodes[match.node-1].edges
	low, high := int(range_.start), int(range_.end)
	for low < high {
		middle := low + (high-low)/2
		if r.instantiationTrieEdges[middle].argument < argument.index {
			low = middle + 1
		} else {
			high = middle
		}
	}
	if low == int(range_.end) || r.instantiationTrieEdges[low].argument != argument.index {
		return InstantiationMatch{}, false
	}
	child := r.instantiationTrieEdges[low].child
	if child == 0 || uint64(child) > uint64(len(r.instantiationTrieNodes)) {
		return InstantiationMatch{}, false
	}
	return InstantiationMatch{owner: r, node: child}, true
}

// FinishInstantiation accepts only an exact terminal. Reaching a shared
// prefix is not success unless that prefix is itself one of Runtime's fixed
// instantiation rows.
func (r *Runtime) FinishInstantiation(match InstantiationMatch) (RuntimeInner, bool) {
	if !r.ownsInstantiationMatch(match) {
		return RuntimeInner{}, false
	}
	result := r.instantiationTrieNodes[match.node-1].result
	return result, r.owns(result)
}

func (r *Runtime) ownsInstantiationMatch(match InstantiationMatch) bool {
	return r != nil && match.owner == r && match.node != 0 && uint64(match.node) <= uint64(len(r.instantiationTrieNodes))
}

// Instantiate is the slice convenience form of the same authoritative trie
// lookup. It performs no allocation itself and has
// O(arity*log(max-degree)) worst-case time.
func (r *Runtime) Instantiate(base RuntimeInner, arguments []RuntimeInner) (RuntimeInner, bool) {
	match, ok := r.BeginInstantiation(base)
	if !ok {
		return RuntimeInner{}, false
	}
	for _, argument := range arguments {
		match, ok = r.MatchInstantiationArgument(match, argument)
		if !ok {
			return RuntimeInner{}, false
		}
	}
	return r.FinishInstantiation(match)
}
