package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/internal/staticmemberwitness"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type functionValueTypeReader interface {
	FunctionValueTypeForValue(product.Value) (*typ.Function, bool)
}

func projectReturnSlots(
	reg *axis.Registry,
	result ResultReader,
	exit state.State,
	arity int,
	declared []product.Value,
) []product.Value {
	fallback := projectReturnSlotsFromExit(reg, result, exit, arity, declared)
	if returns, ok := projectReturnSlotsFromSources(reg, result, exit, arity); ok && returnSlotsAtLeastAsPrecise(reg, returns, fallback) {
		return returns
	}
	return fallback
}

func projectReturnSlotsFromExit(
	reg *axis.Registry,
	result ResultReader,
	exit state.State,
	arity int,
	declared []product.Value,
) []product.Value {
	returns := make([]product.Value, arity)
	for i := range returns {
		returns[i] = exit.ReadValue(reg, key.ReturnSlot(i))
		returns[i] = enrichReturnSlotFromHeapIdentity(reg, result, exit, returns[i])
		if i < len(declared) {
			returns[i] = joinDeclaredReturnValueIfUseful(reg, returns[i], declared[i])
		} else if returnSlotOmittedOnReachableReturn(reg, result, i) {
			returns[i] = joinOmittedReturnValue(reg, returns[i])
		}
	}
	return returns
}

func returnSlotsAtLeastAsPrecise(reg *axis.Registry, candidate, fallback []product.Value) bool {
	if len(candidate) != len(fallback) {
		return false
	}
	for i := range candidate {
		if !product.LessOrEq(reg, candidate[i], fallback[i]) {
			return false
		}
	}
	return true
}

func projectReturnSlotsFromSources(
	reg *axis.Registry,
	result ResultReader,
	exit state.State,
	arity int,
) ([]product.Value, bool) {
	slots := newReturnSlotProjection(reg, result)
	if !slots.OK() || arity <= 0 || len(slots.reachable) == 0 {
		return nil, false
	}
	returns := make([]product.Value, arity)
	for i := range returns {
		returns[i] = product.Bottom(reg)
	}
	for _, point := range slots.reachable {
		sources, ok := slots.Sources(point)
		if !ok {
			return nil, false
		}
		for i := range returns {
			value, ok := slots.Value(point, sources, i)
			if !ok {
				return nil, false
			}
			value = enrichReturnSlotFromHeapIdentity(reg, result, exit, value)
			if i < len(slots.declared) {
				value = mergeDeclaredReturnSourceValue(reg, slots, value, i)
			}
			returns[i] = product.Join(reg, returns[i], value)
		}
	}
	return returns, true
}

func enrichReturnSlotFromHeapIdentity(reg *axis.Registry, result ResultReader, exit state.State, value product.Value) product.Value {
	var ks *keyspace.KeySpace
	if result != nil {
		ks = result.KeySpace()
	}
	if reg == nil || ks == nil {
		return value
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return value
	}
	object := exit.ReadHeapTableObject(reg, id)
	if heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
		return value
	}
	builder := staticmemberwitness.NewBuilder()
	for memberKey, memberValue := range object.StaticMembers() {
		if product.Equal(reg, memberValue, product.Bottom(reg)) {
			continue
		}
		segments, ok := ks.SuffixSegmentsView(memberKey)
		if !ok {
			continue
		}
		memberType, ok := heapStaticMemberType(reg, result, memberValue)
		if !ok {
			continue
		}
		builder.Add(segments, memberType)
	}
	witness, ok := builder.Build()
	if !ok {
		return value
	}
	if existing, ok := typevalue.TypeOf(reg, value); ok && existing != nil {
		merged, mergedOK := typetable.OverlayRecordMembers(existing, witness)
		if !mergedOK {
			return value
		}
		witness = merged
	}
	return typevalue.WithWitness(reg, value, witness)
}

func heapStaticMemberType(reg *axis.Registry, result ResultReader, value product.Value) (typ.Type, bool) {
	if reader, ok := result.(functionValueTypeReader); ok {
		if fn, ok := reader.FunctionValueTypeForValue(value); ok && fn != nil {
			return fn, true
		}
	}
	t, ok := typevalue.TypeOf(reg, value)
	return t, ok && t != nil
}

func returnSlotOmittedOnReachableReturn(reg *axis.Registry, result ResultReader, index int) bool {
	sourceReader, ok := result.(returnValueSourceReader)
	if reg == nil || !ok || index < 0 {
		return false
	}
	for _, point := range result.ReturnPoints() {
		if projectedReturnPointUnreachable(reg, result, point) {
			continue
		}
		sources, ok := sourceReader.ReturnValueSources(point)
		if !ok {
			continue
		}
		if index >= len(sources) {
			return true
		}
	}
	return false
}

func joinOmittedReturnValue(reg *axis.Registry, value product.Value) product.Value {
	if product.Equal(reg, value, product.Bottom(reg)) {
		return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
	}
	return product.WithPresence(reg, value, presence.Join(product.PresenceOf(value), presence.Absent()))
}

func joinDeclaredReturnValue(reg *axis.Registry, value product.Value, declared product.Value) product.Value {
	return product.WithPresence(reg, refinement.MergeDeclaredContract(reg, value, declared), product.PresenceOf(declared))
}

func mergeDeclaredReturnSourceValue(reg *axis.Registry, slots returnSlotProjection, value product.Value, index int) product.Value {
	if index < 0 || index >= len(slots.declared) || !declaredReturnValueUseful(reg, slots.declared[index]) {
		return value
	}
	if product.Equal(reg, value, product.Bottom(reg)) || product.Equal(reg, value, product.Top()) || !typevalue.HasConcreteType(reg, value) {
		return joinDeclaredReturnValue(reg, value, slots.declared[index])
	}
	return slots.ValueWithDeclaredContract(value, index)
}

func joinDeclaredReturnValueIfUseful(reg *axis.Registry, value product.Value, declared product.Value) product.Value {
	if product.Equal(reg, value, product.Bottom(reg)) || product.Equal(reg, value, product.Top()) {
		return joinDeclaredReturnValue(reg, value, declared)
	}
	if !declaredReturnValueUseful(reg, declared) {
		return value
	}
	return joinDeclaredReturnValue(reg, value, declared)
}

func declaredReturnValueUseful(reg *axis.Registry, declared product.Value) bool {
	t, ok := typevalue.TypeOf(reg, declared)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	return true
}
