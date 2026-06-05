package paramevidence

import (
	"maps"
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/lattice"
	latticeproduct "github.com/wippyai/go-lua/types/lattice/product"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Capability is an operator-level obligation on a parameter value. It is not a
// structural type: it records "this path must support an operation" until the
// contract domain can compose it with other requirements on the same path.
type Capability uint8

const (
	CapabilityLength Capability = iota + 1
	CapabilityStringable
	CapabilityOrderable
)

type capabilitySet uint8

const (
	capLength capabilitySet = 1 << iota
	capStringable
	capOrderable
)

// ParamContract is one parameter's accumulated contract demand.
//
// It is an obligation-domain value, not a forward product fact. Concrete
// structural requirements are carried as a product.AbstractValue only after
// admission, while path-local operator capabilities remain explicit until this
// domain can reduce them with other same-path requirements. That separation keeps
// `#x` from becoming a permanent `string | table` forward fact when another body
// use proves the same path is a table/array/record.
type ParamContract struct {
	value    product.AbstractValue
	caps     capabilitySet
	fields   map[string]ParamContract
	element  *ParamContract
	mapKey   *ParamContract
	mapValue *ParamContract
	top      bool
}

// Contracts is the per-function Contracts component of the canonical
// FunctionState = Points x Contracts.
//
// It is a total function from parameter index to ParamContract: an absent index
// denotes the element Bottom (no obligation on that parameter), matching the
// MapLattice convention that absence is the element least element. The cell at
// index i accumulates every contract demand observed for parameter i.
type Contracts = map[int]ParamContract

// ParamContractDomain is the per-parameter contract lattice. Its join is
// obligation composition: multiple requirements on the same parameter/path are
// conjoined and reduced before any projection to product/typ occurs.
var ParamContractDomain = lattice.Lattice[ParamContract]{
	Bottom:   paramContractBottom,
	Top:      paramContractTop,
	Equal:    equalParamContract,
	LessOrEq: func(a, b ParamContract) bool { return equalParamContract(joinParamContract(a, b), b) },
	Join:     joinParamContract,
	Meet:     nil,
	Widen:    widenParamContract,
}

// ContractDomain is the Contracts component lattice: ParamContractDomain lifted
// pointwise over the parameter index by MapLattice.
//
// The update law is accumulation: applying an observed requirement r to the
// contract for parameter i is Contracts[i] = ContractDomain element-Join of the
// current cell with r. Join is the obligation domain's composition operator, not
// the forward value LUB. Cells on the entry->body->entry cycle use
// ContractDomain.Widen; acyclic cells use exact Join. Bottom is the empty map (no
// parameter constrained); Top is the MapLattice top sentinel (every parameter
// over-demanded).
var ContractDomain = latticeproduct.MapLattice[int](ParamContractDomain)

// DemandFromType admits an observed parameter requirement, expressed as a
// structural type, into the per-parameter contract carrier.
//
// It is the admission boundary for an obligation produced by body usage (a
// required field read, a declared/inferred type, a numeric bound). The result is
// Joined into the cell with ParamContractDomain.Join; a nil requirement carries
// no obligation and admits the element Bottom.
func DemandFromType(t typ.Type) ParamContract {
	if t == nil {
		return paramContractBottom()
	}
	if typ.IsNever(t) {
		return paramContractTop()
	}
	return normalizeParamContract(ParamContract{value: product.FromType(t)})
}

// DemandFromPathType admits a structural obligation at a parameter sub-path.
func DemandFromPathType(segments []constraint.Segment, leaf typ.Type) ParamContract {
	return DemandFromPathContract(segments, DemandFromType(leaf))
}

// DemandFromPathContract wraps a contract obligation under a parameter sub-path
// without projecting the leaf through typ/product.
func DemandFromPathContract(segments []constraint.Segment, leaf ParamContract) ParamContract {
	if len(segments) == 0 {
		return leaf
	}
	field, ok := contractFieldName(segments[0])
	if !ok {
		return paramContractBottom()
	}
	child := DemandFromPathContract(segments[1:], leaf)
	if isContractBottom(child) {
		return paramContractBottom()
	}
	return normalizeParamContract(ParamContract{fields: map[string]ParamContract{field: child}})
}

// DemandFromCapability admits an operator capability obligation at the root.
func DemandFromCapability(cap Capability) ParamContract {
	set := capabilityBit(cap)
	if set == 0 {
		return paramContractBottom()
	}
	return ParamContract{caps: set}
}

// DemandFromPathCapability admits an operator capability obligation at a
// parameter sub-path.
func DemandFromPathCapability(segments []constraint.Segment, cap Capability) ParamContract {
	return DemandFromPathContract(segments, DemandFromCapability(cap))
}

// DemandFromSequenceElement admits a mutable sequence/table obligation whose
// elements must satisfy element. It is the backward-demand form of table.insert
// and ipairs value consumption.
func DemandFromSequenceElement(element ParamContract) ParamContract {
	if isContractBottom(element) {
		element = DemandFromType(typ.Any)
	}
	return normalizeParamContract(ParamContract{element: contractPtr(element)})
}

// IndexedIteratorContract converts ipairs-style iterator variable obligations
// back into an element obligation on the source sequence.
func IndexedIteratorContract(varIndex int, local ParamContract) ParamContract {
	if varIndex != 1 || isContractBottom(local) {
		return paramContractBottom()
	}
	return DemandFromSequenceElement(local)
}

// KeyedIteratorContract converts pairs-style iterator variable obligations back
// into read-only map key/value obligations on the source container.
func KeyedIteratorContract(varIndex int, local ParamContract) ParamContract {
	if isContractBottom(local) || !informativeIteratorLocal(local.ProjectValue()) {
		return paramContractBottom()
	}
	switch varIndex {
	case 0:
		return normalizeParamContract(ParamContract{
			mapKey:   contractPtr(local),
			mapValue: contractPtr(DemandFromType(typ.Any)),
		})
	case 1:
		return normalizeParamContract(ParamContract{
			mapKey:   contractPtr(DemandFromType(typ.Any)),
			mapValue: contractPtr(local),
		})
	default:
		return paramContractBottom()
	}
}

func informativeIteratorLocal(t typ.Type) bool {
	return t != nil && !typ.IsAny(t) && !typ.IsUnknown(t)
}

// ContractTypes projects the solved Contracts carrier to caller-visible concrete
// types keyed by parameter index. It is a projection boundary only: the abstract
// interpreter keeps Contracts as product-domain values, while external bridges
// that need concrete types read them through this function.
func ContractTypes(contracts Contracts) map[int]typ.Type {
	if len(contracts) == 0 {
		return nil
	}
	out := make(map[int]typ.Type, len(contracts))
	for idx, contract := range contracts {
		if idx < 0 || ParamContractDomain.Equal(contract, ParamContractDomain.Bottom()) {
			continue
		}
		out[idx] = contract.ProjectValue()
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ProjectValue projects the obligation to its caller/body structural type. This
// is the only place where capability-only obligations become structural
// approximations such as `string | table`.
func (c ParamContract) ProjectValue() typ.Type {
	if c.top {
		return typ.Never
	}
	t := contractValueType(c.value)
	if ft := fieldsContractType(c.fields); ft != nil {
		t = composeContractTypes(t, ft)
	}
	if et := elementContractType(c.element); et != nil {
		t = composeContractTypes(t, et)
	}
	if mt := mapContractType(c.mapKey, c.mapValue); mt != nil {
		t = composeContractTypes(t, mt)
	}
	t = applyCapabilities(t, c.caps)
	return t
}

// ProductValue admits the obligation into the forward product carrier for body
// entry seeding. Contract-generated records are lower bounds ("these fields are
// required"), so they project as open records at this boundary; otherwise the
// body would treat unmentioned fields as absent and suppress later demands.
func (c ParamContract) ProductValue() product.AbstractValue {
	t := openContractRecords(c.ProjectValue())
	if t == nil {
		return product.AbstractValue{}
	}
	return product.FromType(t)
}

// JoinDemand accumulates one observed requirement into the contract for a
// parameter index, returning the updated Contracts cell map.
//
// This is the locked update law: Contracts[idx] = Join(Contracts[idx], demand).
// The cell only grows up the order. An index whose joined cell equals the element
// Bottom is dropped so absence and an explicit Bottom denote the same total
// function (the MapLattice canonicalization).
func JoinDemand(c Contracts, idx int, demand ParamContract) Contracts {
	if idx < 0 {
		return c
	}
	cur := ParamContractDomain.Bottom()
	if existing, ok := c[idx]; ok {
		cur = existing
	}
	joined := ParamContractDomain.Join(cur, demand)
	if ParamContractDomain.Equal(joined, ParamContractDomain.Bottom()) {
		if _, ok := c[idx]; !ok {
			return c
		}
		out := cloneContracts(c)
		delete(out, idx)
		return out
	}
	out := cloneContracts(c)
	out[idx] = joined
	return out
}

func cloneContracts(c Contracts) Contracts {
	out := make(Contracts, len(c)+1)
	maps.Copy(out, c)
	return out
}

func paramContractBottom() ParamContract { return ParamContract{} }

func paramContractTop() ParamContract { return ParamContract{top: true} }

func joinParamContract(a, b ParamContract) ParamContract {
	if isContractBottom(a) {
		return normalizeParamContract(b)
	}
	if isContractBottom(b) {
		return normalizeParamContract(a)
	}
	if a.top || b.top {
		return paramContractTop()
	}
	out := ParamContract{
		caps:     a.caps | b.caps,
		fields:   joinContractFields(a.fields, b.fields, joinParamContract),
		element:  joinContractPtrs(a.element, b.element),
		mapKey:   joinContractPtrs(a.mapKey, b.mapKey),
		mapValue: joinContractPtrs(a.mapValue, b.mapValue),
	}
	value, impossible := joinContractValues(a.value, b.value, false)
	if impossible {
		return paramContractTop()
	}
	out.value = value
	return normalizeParamContract(out)
}

func widenParamContract(prev, next ParamContract) ParamContract {
	return joinParamContract(prev, next)
}

func joinContractValues(a, b product.AbstractValue, converge bool) (product.AbstractValue, bool) {
	at := contractValueType(a)
	bt := contractValueType(b)
	if at == nil {
		return b, false
	}
	if bt == nil {
		return a, false
	}
	joined := composeObligationTypes(at, bt, converge)
	if joined == nil || typ.IsNever(joined) {
		return product.AbstractValue{}, true
	}
	return product.FromType(joined), false
}

func composeObligationTypes(a, b typ.Type, converge bool) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.TypeEquals(a, b) {
		return a
	}
	if joined, ok := composeArrayObligations(a, b, converge); ok {
		return joined
	}
	if joined, ok := composeRecordObligations(a, b, converge); ok {
		return joined
	}
	if converge {
		return ConvergeContractJoin(a, b)
	}
	return HardContractJoin(a, b)
}

func composeArrayObligations(a, b typ.Type, converge bool) (typ.Type, bool) {
	aa, okA := unwrap.Alias(a).(*typ.Array)
	bb, okB := unwrap.Alias(b).(*typ.Array)
	if !okA || !okB || aa == nil || bb == nil {
		return nil, false
	}
	elem := composeObligationTypes(aa.Element, bb.Element, converge)
	if elem == nil || typ.IsNever(elem) {
		return typ.Never, true
	}
	return typ.NewArray(elem), true
}

func composeRecordObligations(a, b typ.Type, converge bool) (typ.Type, bool) {
	ar, okA := unwrap.Alias(a).(*typ.Record)
	br, okB := unwrap.Alias(b).(*typ.Record)
	if !okA || !okB || ar == nil || br == nil {
		return nil, false
	}
	builder := typ.NewRecord()
	if ar.Open || br.Open {
		builder.SetOpen(true)
	}
	if mt := composeOptionalObligation(ar.Metatable, br.Metatable, converge); mt != nil {
		builder.Metatable(mt)
	}
	if ar.MapKey != nil || br.MapKey != nil || ar.MapValue != nil || br.MapValue != nil {
		key := composeOptionalObligation(ar.MapKey, br.MapKey, converge)
		val := composeOptionalObligation(ar.MapValue, br.MapValue, converge)
		if key != nil && val != nil {
			builder.MapComponent(key, val)
		}
	}

	fields := make(map[string]typ.Field, len(ar.Fields)+len(br.Fields))
	for _, field := range ar.Fields {
		fields[field.Name] = field
	}
	for _, field := range br.Fields {
		if existing, ok := fields[field.Name]; ok {
			field.Type = composeObligationTypes(existing.Type, field.Type, converge)
			field.Optional = existing.Optional && field.Optional
			field.Readonly = existing.Readonly || field.Readonly
		}
		fields[field.Name] = field
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		addObligationRecordField(builder, fields[key])
	}
	return builder.Build(), true
}

func composeOptionalObligation(a, b typ.Type, converge bool) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return composeObligationTypes(a, b, converge)
}

func addObligationRecordField(builder *typ.RecordBuilder, field typ.Field) {
	switch {
	case field.Optional && field.Readonly:
		builder.OptReadonlyField(field.Name, field.Type)
	case field.Optional:
		builder.OptField(field.Name, field.Type)
	case field.Readonly:
		builder.ReadonlyField(field.Name, field.Type)
	default:
		builder.Field(field.Name, field.Type)
	}
}

func openContractRecords(t typ.Type) typ.Type {
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Record:
		builder := typ.NewRecord().SetOpen(true)
		if v.Metatable != nil {
			builder.Metatable(openContractRecords(v.Metatable))
		}
		if v.MapKey != nil && v.MapValue != nil {
			builder.MapComponent(openContractRecords(v.MapKey), openContractRecords(v.MapValue))
		}
		for _, field := range v.Fields {
			field.Type = openContractRecords(field.Type)
			addObligationRecordField(builder, field)
		}
		for _, member := range v.StaticMembers {
			member.Type = openContractRecords(member.Type)
			builder.AddStaticMember(member)
		}
		return builder.Build()
	case *typ.Array:
		return typ.NewArray(openContractRecords(v.Element))
	case *typ.Map:
		return typ.NewMap(openContractRecords(v.Key), openContractRecords(v.Value))
	case *typ.ReadonlyMap:
		return typ.NewReadonlyMap(openContractRecords(v.Key), openContractRecords(v.Value))
	case *typ.Tuple:
		elems := make([]typ.Type, len(v.Elements))
		for i, elem := range v.Elements {
			elems[i] = openContractRecords(elem)
		}
		return typ.NewTuple(elems...)
	case *typ.Optional:
		return typ.NewOptional(openContractRecords(v.Inner))
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		for i, member := range v.Members {
			members[i] = openContractRecords(member)
		}
		return typ.NewUnion(members...)
	case *typ.Intersection:
		members := make([]typ.Type, len(v.Members))
		for i, member := range v.Members {
			members[i] = openContractRecords(member)
		}
		return typ.NewIntersection(members...)
	case *typ.Alias:
		return typ.NewAlias(v.Name, openContractRecords(v.Target))
	default:
		return t
	}
}

func joinContractFields(
	a, b map[string]ParamContract,
	join func(ParamContract, ParamContract) ParamContract,
) map[string]ParamContract {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]ParamContract, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if existing, ok := out[k]; ok {
			out[k] = join(existing, v)
		} else {
			out[k] = v
		}
		if isContractBottom(out[k]) {
			delete(out, k)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func contractPtr(c ParamContract) *ParamContract {
	c = normalizeParamContract(c)
	if isContractBottom(c) {
		return nil
	}
	return &c
}

func joinContractPtrs(a, b *ParamContract) *ParamContract {
	switch {
	case a == nil && b == nil:
		return nil
	case a == nil:
		return contractPtr(*b)
	case b == nil:
		return contractPtr(*a)
	default:
		return contractPtr(joinParamContract(*a, *b))
	}
}

func normalizeContractPtr(c *ParamContract) *ParamContract {
	if c == nil {
		return nil
	}
	return contractPtr(*c)
}

func normalizeParamContract(c ParamContract) ParamContract {
	if c.top {
		return paramContractTop()
	}
	for name, child := range c.fields {
		child = normalizeParamContract(child)
		if isContractBottom(child) {
			delete(c.fields, name)
			continue
		}
		c.fields[name] = child
	}
	if len(c.fields) == 0 {
		c.fields = nil
	}
	c.element = normalizeContractPtr(c.element)
	c.mapKey = normalizeContractPtr(c.mapKey)
	c.mapValue = normalizeContractPtr(c.mapValue)
	t := c.ProjectValue()
	if typ.IsNever(t) {
		return paramContractTop()
	}
	return c
}

func equalParamContract(a, b ParamContract) bool {
	a = normalizeParamContractForEqual(a)
	b = normalizeParamContractForEqual(b)
	if a.top || b.top {
		return a.top == b.top
	}
	if a.caps != b.caps || !product.Domain.Equal(contractValueOrBottom(a.value), contractValueOrBottom(b.value)) {
		return false
	}
	if !equalContractPtr(a.element, b.element) ||
		!equalContractPtr(a.mapKey, b.mapKey) ||
		!equalContractPtr(a.mapValue, b.mapValue) {
		return false
	}
	if len(a.fields) != len(b.fields) {
		return false
	}
	for k, av := range a.fields {
		bv, ok := b.fields[k]
		if !ok || !equalParamContract(av, bv) {
			return false
		}
	}
	return true
}

func normalizeParamContractForEqual(c ParamContract) ParamContract {
	if c.top {
		return paramContractTop()
	}
	out := c
	if len(c.fields) > 0 {
		out.fields = make(map[string]ParamContract, len(c.fields))
		for k, v := range c.fields {
			v = normalizeParamContractForEqual(v)
			if !isContractBottom(v) {
				out.fields[k] = v
			}
		}
		if len(out.fields) == 0 {
			out.fields = nil
		}
	}
	out.element = normalizeContractPtr(out.element)
	out.mapKey = normalizeContractPtr(out.mapKey)
	out.mapValue = normalizeContractPtr(out.mapValue)
	return out
}

func isContractBottom(c ParamContract) bool {
	return !c.top && c.caps == 0 && len(c.fields) == 0 && c.element == nil &&
		c.mapKey == nil && c.mapValue == nil && contractValueType(c.value) == nil
}

func contractValueOrBottom(v product.AbstractValue) product.AbstractValue {
	if contractValueType(v) == nil {
		return product.Domain.Bottom()
	}
	return v
}

func contractValueType(v product.AbstractValue) typ.Type {
	if v.IsZero() || product.Domain.Equal(v, product.Domain.Bottom()) {
		return nil
	}
	t := v.ProjectValue()
	if t == nil || typ.IsNever(t) {
		return nil
	}
	return t
}

func fieldsContractType(fields map[string]ParamContract) typ.Type {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	builder := typ.NewRecord()
	added := false
	for _, key := range keys {
		ft := fields[key].ProjectValue()
		if ft == nil {
			continue
		}
		builder.ReadonlyField(key, ft)
		added = true
	}
	if !added {
		return nil
	}
	return builder.Build()
}

func elementContractType(element *ParamContract) typ.Type {
	if element == nil {
		return nil
	}
	elem := element.ProjectValue()
	if elem == nil {
		elem = typ.Any
	}
	return typ.NewArray(elem)
}

func mapContractType(key, val *ParamContract) typ.Type {
	if key == nil && val == nil {
		return nil
	}
	keyType := typ.Any
	if key != nil {
		if projected := key.ProjectValue(); projected != nil {
			keyType = projected
		}
	}
	valType := typ.Any
	if val != nil {
		if projected := val.ProjectValue(); projected != nil {
			valType = projected
		}
	}
	return typ.NewReadonlyMap(keyType, valType)
}

func composeContractTypes(existing, required typ.Type) typ.Type {
	if existing == nil {
		return required
	}
	if required == nil {
		return existing
	}
	joined := HardContractJoin(existing, required)
	if joined == nil {
		return typ.Never
	}
	return joined
}

func equalContractPtr(a, b *ParamContract) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return equalParamContract(*a, *b)
}

func applyCapabilities(t typ.Type, caps capabilitySet) typ.Type {
	if caps == 0 {
		return t
	}
	for _, item := range []struct {
		bit capabilitySet
		cap Capability
	}{
		{capLength, CapabilityLength},
		{capStringable, CapabilityStringable},
		{capOrderable, CapabilityOrderable},
	} {
		if caps&item.bit == 0 {
			continue
		}
		t = applyCapability(t, item.cap)
		if typ.IsNever(t) {
			return typ.Never
		}
	}
	return t
}

func applyCapability(t typ.Type, cap Capability) typ.Type {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return capabilityProjection(cap)
	}
	if typ.IsNever(t) {
		return typ.Never
	}
	if capabilityCoversType(t, cap) {
		return t
	}
	if narrowed := narrowTypeToCapability(t, cap); narrowed != nil {
		return narrowed
	}
	return typ.Never
}

func narrowTypeToCapability(t typ.Type, cap Capability) typ.Type {
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Optional:
		return narrowTypeToCapability(v.Inner, cap)
	case *typ.Union:
		var members []typ.Type
		for _, member := range v.Members {
			narrowed := narrowTypeToCapability(member, cap)
			if narrowed == nil || typ.IsNever(narrowed) {
				continue
			}
			members = append(members, narrowed)
		}
		if len(members) == 0 {
			return typ.Never
		}
		return typ.NewUnion(members...)
	case *typ.Alias:
		if narrowed := narrowTypeToCapability(v.Target, cap); narrowed != nil && !typ.IsNever(narrowed) {
			return typ.NewAlias(v.Name, narrowed)
		}
		return typ.Never
	case *typ.TypeParam:
		if v.Constraint == nil {
			return capabilityProjection(cap)
		}
		return narrowTypeToCapability(v.Constraint, cap)
	}
	if capabilityCoversType(t, cap) {
		return t
	}
	if narrowed := subtype.NormalizeIntersection(t, capabilityProjection(cap)); narrowed != nil {
		return narrowed
	}
	return typ.Never
}

func capabilityCoversType(t typ.Type, cap Capability) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) && cap == CapabilityLength {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Optional:
		return false
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !capabilityCoversType(member, cap) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if capabilityCoversType(member, cap) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return capabilityCoversType(v.Target, cap)
	case *typ.TypeParam:
		return v.Constraint != nil && capabilityCoversType(v.Constraint, cap)
	case *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Record, *typ.Tuple:
		return cap == CapabilityLength
	case *typ.Literal:
		switch v.Base {
		case kind.String:
			return cap == CapabilityLength || cap == CapabilityStringable || cap == CapabilityOrderable
		case kind.Integer, kind.Number:
			return cap == CapabilityStringable || cap == CapabilityOrderable
		default:
			return false
		}
	}
	switch t.Kind() {
	case kind.String:
		return cap == CapabilityLength || cap == CapabilityStringable || cap == CapabilityOrderable
	case kind.Integer, kind.Number:
		return cap == CapabilityStringable || cap == CapabilityOrderable
	default:
		return false
	}
}

func capabilityProjection(cap Capability) typ.Type {
	switch cap {
	case CapabilityLength:
		return typ.NewUnion(
			typ.String,
			typ.NewArray(typ.Any),
			typ.NewMap(typ.Any, typ.Any),
			typ.NewReadonlyMap(typ.Any, typ.Any),
			typ.NewRecord().SetOpen(true).Build(),
		)
	case CapabilityStringable, CapabilityOrderable:
		return typ.NewUnion(typ.String, typ.Number)
	default:
		return nil
	}
}

func capabilityBit(cap Capability) capabilitySet {
	switch cap {
	case CapabilityLength:
		return capLength
	case CapabilityStringable:
		return capStringable
	case CapabilityOrderable:
		return capOrderable
	default:
		return 0
	}
}

func contractFieldName(seg constraint.Segment) (string, bool) {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}
