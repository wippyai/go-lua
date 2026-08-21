package access

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/normalize"
	"github.com/wippyai/go-lua/domain/type/subtype"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

func (q *query) resolveIndex(container, key typ.Type, mode indexMode) fieldResult {
	type frame struct {
		t        typ.Type
		cycle    fieldResult
		phase    uint8
		entered  bool
		optional int
		members  []typ.Type
		next     int
		values   []typ.Type
		nilable  bool
		result   fieldResult
	}
	stack := []frame{{phase: 9}, {t: container}}
	finish := func(result fieldResult) {
		last := len(stack) - 1
		finished := stack[last]
		if finished.entered {
			q.leave(queryKey{op: 3, t: finished.t, key: key, mode: mode})
		}
		stack = stack[:last]
		parent := &stack[len(stack)-1]
		if parent.phase == 9 {
			parent.result = result
			return
		}
		switch parent.phase {
		case 1:
			if !result.ok {
				parent.phase = 2
			} else {
				if result.nilable {
					parent.nilable = true
				}
				if result.t != nil {
					parent.values = append(parent.values, result.t)
				}
			}
		case 2:
			if result.ok {
				parent.nilable = true
				parent.phase = 1
			} else {
				parent.phase = 3
			}
		case 4:
			if result.ok {
				if value, ok := result.materialize(); ok {
					parent.values = append(parent.values, value)
				}
			}
		}
	}
	for len(stack) > 1 {
		top := &stack[len(stack)-1]
		if top.phase == 0 {
			if top.t == nil {
				finish(fieldResult{})
				continue
			}
			if special, ok := SpecialAccessType(top.t); ok {
				finish(fieldResult{t: special, ok: true})
				continue
			}
			visit := queryKey{op: 3, t: top.t, key: key, mode: mode}
			if !q.enter(visit) {
				finish(top.cycle)
				continue
			}
			top.entered = true
			base, optional, ok := accessProjectionBase(top.t)
			if !ok {
				finish(fieldResult{})
				continue
			}
			top.optional = optional
			if special, ok := SpecialAccessType(base); ok {
				finish(optionalizeField(fieldResult{t: special, ok: true}, optional))
				continue
			}
			switch value := unwrap.Annotated(base).(type) {
			case *typ.Union:
				if value == nil || len(value.Members) == 0 {
					finish(fieldResult{})
					continue
				}
				top.members, top.phase = value.Members, 1
			case *typ.Intersection:
				if value == nil {
					finish(fieldResult{})
					continue
				}
				top.members, top.phase = value.Members, 4
			default:
				switch unwrap.Annotated(base).(type) {
				case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
				default:
					finish(fieldResult{})
					continue
				}
				result := iterativeIndexLeaf(base, key, mode)
				if !result.ok && mode == indexRuntime && q.resolveMissing(top.t) {
					result = fieldResult{t: typ.Nil, ok: true}
				}
				finish(optionalizeField(result, optional))
			}
			continue
		}
		switch top.phase {
		case 1:
			if top.next == len(top.members) {
				if len(top.values) == 0 {
					if top.nilable {
						finish(optionalizeField(fieldResult{t: typ.Nil, ok: true}, top.optional))
					} else {
						finish(fieldResult{})
					}
				} else {
					finish(optionalizeField(fieldResult{t: normalize.UnionForEvidence(top.values...), ok: true, nilable: top.nilable}, top.optional))
				}
				continue
			}
			member := top.members[top.next]
			top.next++
			stack = append(stack, frame{t: member, cycle: fieldResult{ok: true}})
		case 2:
			if q.resolveMissing(top.members[top.next-1]) {
				top.nilable = true
				top.phase = 1
			} else {
				top.phase = 3
			}
		case 3:
			finish(fieldResult{})
		case 4:
			if top.next == len(top.members) {
				if len(top.values) == 0 {
					finish(fieldResult{})
				} else if len(top.values) == 1 {
					finish(optionalizeField(fieldResult{t: top.values[0], ok: true}, top.optional))
				} else {
					finish(optionalizeField(fieldResult{t: normalize.IntersectionForMeet(top.values...), ok: true}, top.optional))
				}
				continue
			}
			member := top.members[top.next]
			top.next++
			stack = append(stack, frame{t: member})
		}
	}
	return stack[0].result
}

func iterativeIndexLeaf(container, key typ.Type, mode indexMode) fieldResult {
	return iterativeKeyVariants(key, mode, func(leaf typ.Type) fieldResult {
		switch value := unwrap.Annotated(container).(type) {
		case *typ.Record:
			return iterativeIndexRecord(value, leaf, mode)
		case *typ.Map:
			return iterativeIndexMap(value.Key, value.Value, leaf, mode)
		case *typ.ReadonlyMap:
			return iterativeIndexMap(value.Key, value.Value, leaf, mode)
		case *typ.Array:
			return iterativeIndexArray(value, leaf, mode)
		case *typ.Tuple:
			return iterativeIndexTuple(value, leaf, mode)
		default:
			return fieldResult{}
		}
	})
}

func iterativeKeyVariants(key typ.Type, mode indexMode, project func(typ.Type) fieldResult) fieldResult {
	type item struct {
		value    typ.Type
		optional bool
	}
	work := []item{{value: key}}
	seen := make(map[typ.Type]struct{})
	values := make([]typ.Type, 0, 2)
	nilable := false
	for len(work) != 0 {
		last := len(work) - 1
		current := work[last]
		work = work[:last]
		if _, duplicate := seen[current.value]; duplicate {
			// Re-entering a key-union arm is the must-query identity used by
			// indexByKeyVariants: it contributes no new leaf but cannot make an
			// already productive finite arm fail.
			continue
		}
		seen[current.value] = struct{}{}
		base, optional, ok := accessProjectionBase(current.value)
		if !ok {
			return fieldResult{}
		}
		current.optional = current.optional || optional != 0
		if union, ok := unwrap.Annotated(base).(*typ.Union); ok {
			if union == nil || len(union.Members) == 0 {
				return fieldResult{}
			}
			for index := len(union.Members) - 1; index >= 0; index-- {
				work = append(work, item{value: union.Members[index], optional: current.optional})
			}
			continue
		}
		result := project(base)
		if !result.ok {
			if mode != indexRuntime {
				return fieldResult{}
			}
			result = fieldResult{t: typ.Nil, ok: true}
		}
		if result.nilable || current.optional {
			nilable = true
		}
		if result.t != nil {
			values = append(values, result.t)
		}
	}
	if len(values) == 0 {
		if nilable {
			return fieldResult{t: typ.Nil, ok: true}
		}
		return fieldResult{}
	}
	return fieldResult{t: normalize.UnionForEvidence(values...), ok: true, nilable: nilable}
}

func iterativeIndexRecord(record *typ.Record, key typ.Type, mode indexMode) fieldResult {
	if record == nil {
		return fieldResult{}
	}
	if name, ok := literalStringKey(key); ok {
		return stringKeyInRecord(record, name)
	}
	if index, ok := literalIntKey(key); ok {
		return indexIntInRecord(record, index)
	}
	integer := iterativeKeyMayBeInteger(key)
	if mode != indexRuntime {
		integer = subtype.IsSubtype(key, typ.Integer)
	}
	if integer {
		values := make([]typ.Type, 0, len(record.StaticMembers))
		for _, member := range record.StaticMembers {
			if member.Kind == typ.StaticMemberIntIndex {
				if member.Type == nil {
					values = append(values, typ.Unknown)
				} else {
					values = append(values, member.Type)
				}
			}
		}
		if len(values) != 0 {
			result := fieldResult{t: normalize.UnionForEvidence(values...), ok: true, nilable: true}
			if mapResult := iterativeIndexRecordMap(record, key, mode); mapResult.ok {
				return unionFieldResults(result, mapResult)
			}
			return result
		}
	}
	if result := iterativeIndexRecordMap(record, key, mode); result.ok {
		return result
	}
	if record.Open {
		return fieldResult{t: typ.Unknown, ok: true}
	}
	return fieldResult{}
}

func iterativeIndexRecordMap(record *typ.Record, key typ.Type, mode indexMode) fieldResult {
	if record == nil || !record.HasMapComponent() {
		return fieldResult{}
	}
	ok := typetable.MapComponentKeyAdmitsType(record.MapKey, key)
	if !ok && mode == indexRuntime {
		ok = typetable.MapComponentKeyMayOverlapType(record.MapKey, key)
	}
	if !ok {
		return fieldResult{}
	}
	return fieldResult{t: record.MapValue, ok: true, nilable: mode != indexWrite}
}

func iterativeIndexMap(domain, value, key typ.Type, mode indexMode) fieldResult {
	ok := typetable.MapComponentKeyAdmitsType(domain, key)
	if !ok && mode == indexRuntime {
		ok = typetable.MapComponentKeyMayOverlapType(domain, key)
	}
	if !ok {
		return fieldResult{}
	}
	if value == nil {
		value = typ.Unknown
	}
	return fieldResult{t: value, ok: true, nilable: mode != indexWrite}
}

func iterativeIndexArray(array *typ.Array, key typ.Type, mode indexMode) fieldResult {
	if array == nil {
		return fieldResult{}
	}
	if mode == indexRuntime {
		if !iterativeKeyMayBeInteger(key) {
			return fieldResult{}
		}
	} else if !subtype.IsSubtype(key, typ.Integer) {
		return fieldResult{}
	}
	element := array.Element
	if element == nil {
		element = typ.Unknown
	}
	return fieldResult{t: element, ok: true, nilable: mode != indexWrite}
}

func iterativeIndexTuple(tuple *typ.Tuple, key typ.Type, mode indexMode) fieldResult {
	if tuple == nil {
		return fieldResult{}
	}
	if index, ok := literalIntKey(key); ok {
		if index < 1 || index > int64(len(tuple.Elements)) {
			return fieldResult{}
		}
		element := tuple.Elements[index-1]
		if element == nil {
			element = typ.Unknown
		}
		return fieldResult{t: element, ok: true}
	}
	if mode == indexRuntime {
		if !iterativeKeyMayBeInteger(key) {
			return fieldResult{}
		}
	} else if !subtype.IsSubtype(key, typ.Integer) {
		return fieldResult{}
	}
	values := make([]typ.Type, 0, len(tuple.Elements))
	for _, element := range tuple.Elements {
		if element == nil {
			element = typ.Unknown
		}
		values = append(values, element)
	}
	if len(values) == 0 {
		return fieldResult{}
	}
	return fieldResult{t: normalize.UnionForEvidence(values...), ok: true, nilable: mode != indexWrite}
}

func iterativeKeyMayBeInteger(key typ.Type) bool {
	type item struct{ value typ.Type }
	work := []item{{value: key}}
	seen := make(map[typ.Type]struct{})
	for len(work) != 0 {
		last := len(work) - 1
		current := work[last]
		work = work[:last]
		if _, duplicate := seen[current.value]; duplicate {
			continue
		}
		seen[current.value] = struct{}{}
		base, _, ok := accessProjectionBase(current.value)
		if !ok {
			return true
		}
		if typ.IsAny(base) || typ.IsUnknown(base) {
			return true
		}
		if union, ok := unwrap.Annotated(base).(*typ.Union); ok {
			for _, member := range union.Members {
				work = append(work, item{value: member})
			}
			continue
		}
		if literal, ok := base.(*typ.Literal); ok {
			if literal.Base() == kind.Integer {
				return true
			}
			if literal.Base() == kind.Number {
				if number, ok := literal.Value().(float64); ok && number == float64(int64(number)) {
					return true
				}
			}
			continue
		}
		if base.Kind() == kind.Integer || base.Kind() == kind.Number {
			return true
		}
	}
	return false
}
