package typeauthority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/subst"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/identity"
)

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
	// value is a construction-only, ownership-isolated graph used to retain
	// authored binder presentation (for example `T` rather than codec-local
	// `scoped-local-1`). SealRuntime drops it with the builder.
	value typ.Type
}

type RuntimeInputFailure uint8

const (
	RuntimeInputFailureNone RuntimeInputFailure = iota
	RuntimeInputFailureAuthority
	RuntimeInputFailureDecode
	RuntimeInputFailureCanonical
	RuntimeInputFailureOpenFormal
)

func (failure RuntimeInputFailure) String() string {
	switch failure {
	case RuntimeInputFailureNone:
		return "none"
	case RuntimeInputFailureAuthority:
		return "authority"
	case RuntimeInputFailureDecode:
		return "decode"
	case RuntimeInputFailureCanonical:
		return "canonical"
	case RuntimeInputFailureOpenFormal:
		return "open-formal"
	default:
		return "invalid"
	}
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
	form   kind.Kind
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
	literal        runtimeLiteral
}

// Runtime is the immutable finite structural authority consumed by runtime
// LType reflection. It is deliberately separate from Static's evaluator:
// Static supplies already-evaluated canonical closed inputs; Runtime seals
// their finite structural closure and has no evaluator or source AST path.
type Runtime struct {
	sourceID identity.ContentID
	id       identity.ContentID

	rows           []runtimeRow
	fields         []runtimeNamedChild
	staticMembers  []runtimeStaticChild
	variants       []runtimeChild
	elements       []runtimeTupleElement
	parameters     []runtimeParameter
	results        []runtimeChild
	methods        []runtimeNamedChild
	typeParameters []runtimeNamedChild
	arguments      []RuntimeInner

	identities []identity.ContentID
	canonical  []uint32

	// The sealed subtype relation of the closed universe. closedPositions maps
	// a one-based dense row onto its universe position (-1 for a bound/open
	// row), closedRows is the inverse, and subtypeBits packs one bitset row of
	// stride words per universe position.
	closedPositions []int32
	closedRows      []uint32
	subtypeStride   int
	subtypeBits     []uint64

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
	input, failure := a.RuntimeInputWithFailure(encoded)
	return input, failure == RuntimeInputFailureNone
}

func (a *Authority) RuntimeInputWithFailure(encoded []byte) (RuntimeInput, RuntimeInputFailure) {
	if a == nil || !a.LinkID().Available() || len(encoded) == 0 {
		return RuntimeInput{}, RuntimeInputFailureAuthority
	}
	value, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
	if err != nil || value == nil {
		return RuntimeInput{}, RuntimeInputFailureDecode
	}
	canonical, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil || !sameBytes(canonical, encoded) {
		return RuntimeInput{}, RuntimeInputFailureCanonical
	}
	input, ok := a.RuntimeInputForType(value)
	if !ok {
		return RuntimeInput{}, RuntimeInputFailureOpenFormal
	}
	return input, RuntimeInputFailureNone
}

// RuntimeInputForType seals the scoped canonical identity while the exact
// evaluated closed graph is still live. Ordinary canonical bytes deliberately
// omit binder ownership and therefore cannot faithfully reconstruct a graph
// containing repeated applications of the same nested Generic declaration.
// RuntimeInput remains opaque and construction-only; Runtime retains no typ
// graph after SealRuntime.
func (a *Authority) RuntimeInputForType(value typ.Type) (RuntimeInput, bool) {
	// A binder-owned formal inside a Generic/Function is structurally closed.
	// ContainsTypeParam cannot distinguish it from a free formal and used to
	// reject valid generic declarations before Runtime could seal them.
	if a == nil || !a.LinkID().Available() || value == nil || !typ.IsGraphClosed(value) || typ.ValidateStaticGenericRecurrence(value) != nil {
		return RuntimeInput{}, false
	}
	encoded, err := typ.EncodeCanonicalFormals(context.Background(), value, nil)
	if err != nil || len(encoded) == 0 {
		return RuntimeInput{}, false
	}
	// Zero external formals is the ownership boundary: the codec accepts
	// binder-local Generic/Function formals, but rejects any parameter that
	// was not introduced by the encoded root.
	if typ.ValidateCanonicalFormals(encoded, 0) != nil {
		return RuntimeInput{}, false
	}
	decoded, err := typ.DecodeCanonicalFormals(context.Background(), encoded, nil)
	if err != nil || decoded == nil {
		return RuntimeInput{}, false
	}
	reencoded, err := typ.EncodeCanonicalFormals(context.Background(), decoded, nil)
	if err != nil || !sameBytes(reencoded, encoded) {
		return RuntimeInput{}, false
	}
	return RuntimeInput{authority: a, encoded: append([]byte(nil), encoded...), value: value}, true
}

// SealRuntime closes the direct structural children of Static's finite
// canonical input set. Runtime owns structural rows only: occurrence-specific
// TypeValue interpretation belongs to Static, which owns both the evaluated
// result and its Boundary occurrence.
func SealRuntime(types *Authority, inputs []RuntimeInput) (*Runtime, []RuntimeInner, error) {
	if types == nil || !types.ArtifactBacked() || !types.LinkID().Available() {
		return nil, nil, errors.New("typeauthority: Runtime requires detached artifact authority")
	}
	runtime := &Runtime{
		sourceID: types.LinkID(),
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
		value := input.input.value
		if value == nil {
			var err error
			value, err = typ.DecodeCanonicalFormals(context.Background(), input.input.encoded, nil)
			if err != nil || value == nil {
				return nil, nil, errors.New("typeauthority: invalid Runtime input")
			}
		}
		verified, verifyErr := typ.EncodeCanonicalFormals(context.Background(), value, nil)
		if verifyErr != nil || !sameBytes(verified, input.input.encoded) {
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
	if err := builder.sealCanonical(); err != nil {
		return nil, nil, err
	}
	if err := builder.sealDescriptors(); err != nil {
		return nil, nil, err
	}
	if err := runtime.sealRanks(); err != nil {
		return nil, nil, err
	}
	// The relation is materialized while the construction graphs are alive and
	// is the last consumer of them.
	if err := builder.sealSubtypeRelation(); err != nil {
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

func runtimeScope(value typ.Type, external []*typ.TypeParam) identity.ContentID {
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
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.typeauthority.runtime/formal-scope\x00\x01"))
	_, _ = hash.Write(encoded)
	var id identity.ContentID
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
	scope   identity.ContentID
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
	value, err := typ.DecodeCanonicalFormals(context.Background(), input.encoded, nil)
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

func (b *runtimeBuilder) child(value typ.Type, formals []*typ.TypeParam, scope identity.ContentID) (RuntimeInner, error) {
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
	child := func(value typ.Type, formals []*typ.TypeParam, scope identity.ContentID) error {
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
	row.form = current.value.Kind()
	child := func(value typ.Type, formals []*typ.TypeParam, scope identity.ContentID) (runtimeChild, error) {
		if value == nil {
			return runtimeChild{}, nil
		}
		inner, err := b.child(value, formals, scope)
		return runtimeChild{inner: inner, present: err == nil}, err
	}
	appendUnnamed := func(values *[]runtimeChild, value typ.Type, formals []*typ.TypeParam, scope identity.ContentID) error {
		entry, err := child(value, formals, scope)
		if err != nil {
			return err
		}
		*values = append(*values, entry)
		return nil
	}
	appendNamed := func(values *[]runtimeNamedChild, name string, value typ.Type, optional, readonly bool, formals []*typ.TypeParam, scope identity.ContentID) error {
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
		row.literal.base = value.Base()
		switch value.Base() {
		case kind.Boolean:
			literal, ok := value.Value().(bool)
			if !ok {
				return errors.New("typeauthority: malformed Runtime boolean literal")
			}
			if literal {
				row.literal.bits = 1
			}
		case kind.Integer:
			literal, ok := value.Value().(int64)
			if !ok {
				return errors.New("typeauthority: malformed Runtime integer literal")
			}
			row.literal.bits = uint64(literal)
		case kind.Number:
			literal, ok := value.Value().(float64)
			if !ok {
				return errors.New("typeauthority: malformed Runtime number literal")
			}
			row.literal.bits = math.Float64bits(literal)
		case kind.String:
			literal, ok := value.Value().(string)
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
		expanded := subst.ExpandInstantiated(value)
		if expanded == nil || expanded == value {
			return errors.New("typeauthority: Runtime instantiation cannot expand")
		}
		row.expansion, err = child(expanded, current.formals, current.scope)
		return err
	default:
		// Every remaining supported node is a leaf: it carries no structural
		// child and is described by its kind alone.
		switch row.form {
		case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
			kind.Any, kind.Unknown, kind.Never, kind.Self:
		default:
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
		case kind.Union:
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
		case kind.Optional:
			// Optional is the canonical representation of nil|T. Keep the
			// nil atom explicit so ClassSet's union algebra agrees with the
			// authored nil and T classes.
			for nilIndex, candidate := range runtime.rows {
				if candidate.form != kind.Nil {
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
	if r == nil || !r.sourceID.Available() {
		return errors.New("typeauthority: unavailable Runtime identity source")
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.typeauthority.runtime\x00\x04"))
	_, _ = hash.Write(r.sourceID[:])
	writeRuntimeWord(hash, uint64(len(r.rows)))
	for _, row := range r.rows {
		writeRuntimeWord(hash, uint64(row.form))
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
	writeRuntimeWord(hash, uint64(len(r.arguments)))
	for _, argument := range r.arguments {
		writeRuntimeWord(hash, uint64(argument.index))
	}
	copy(r.id[:], hash.Sum(nil))
	if !r.id.Available() {
		return errors.New("typeauthority: unavailable Runtime content identity")
	}
	r.identities = make([]identity.ContentID, len(r.rows))
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
func (r *Runtime) LinkID() identity.ContentID {
	if r == nil {
		return identity.ContentID{}
	}
	return r.sourceID
}

func (r *Runtime) ContentID() identity.ContentID {
	if r == nil {
		return identity.ContentID{}
	}
	return r.id
}

func (r *Runtime) Count() int {
	if r == nil {
		return 0
	}
	return len(r.rows)
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

func (r *Runtime) Identity(inner RuntimeInner) (identity.ContentID, bool) {
	if !r.owns(inner) || int(inner.index) > len(r.identities) {
		return identity.ContentID{}, false
	}
	id := r.identities[inner.index-1]
	return id, id.Available()
}

// Kind reports the structural category of one dense row. It is the same
// enumeration the authored type graph uses, so a caller never translates
// between a Runtime-local form vocabulary and typ.Type.Kind.
func (r *Runtime) Kind(inner RuntimeInner) (kind.Kind, bool) {
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

// Subtype is the sealed allocation-free owner-local subtype judgment. It is a
// lookup into the relation materialized at seal from the canonical checker; a
// bound/open row has no independent judgment and stays undecided.
func (r *Runtime) Subtype(left, right RuntimeInner) (answer, decided bool) {
	if !r.owns(left) || !r.owns(right) {
		return false, false
	}
	if left.index == right.index {
		return true, true
	}
	if len(r.closedPositions) != len(r.rows) {
		return false, false
	}
	leftPosition, rightPosition := r.closedPositions[left.index-1], r.closedPositions[right.index-1]
	if leftPosition < 0 || rightPosition < 0 {
		return false, false
	}
	word := r.subtypeBits[int(leftPosition)*r.subtypeStride+int(rightPosition)>>6]
	return word&(1<<(uint(rightPosition)&63)) != 0, true
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
