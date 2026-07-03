package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func IteratorVariableValue(reg *axis.Registry, typeValues *typevalue.Cache, iter iteration.Iterator, variableIndex int, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (product.Value, bool) {
	switch variableIndex {
	case 0:
		return iteratorKeyValue(reg, typeValues, iter, sourceValue, assertedSourceType, hasAssertedSourceType)
	case 1:
		return iteratorElementValue(reg, typeValues, iter, sourceValue, assertedSourceType, hasAssertedSourceType)
	default:
		return product.Value{}, false
	}
}

func iteratorKeyValue(reg *axis.Registry, typeValues *typevalue.Cache, iter iteration.Iterator, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (product.Value, bool) {
	switch iter.Kind {
	case iteration.IterateIndexed:
		return iteratorPresent(reg, typeValues.FromTypeWithWitness(reg, typ.Integer)), true
	case iteration.IterateKeyed:
		if sourceType, ok := iteratorSourceType(reg, sourceValue, assertedSourceType, hasAssertedSourceType); ok {
			if keyType, ok := iteratorKeyOf(sourceType); ok {
				return iteratorPresent(reg, typeValues.FromTypeWithWitness(reg, keyType)), true
			}
		}
		return product.Value{}, false
	default:
		return product.Value{}, false
	}
}

func iteratorElementValue(reg *axis.Registry, typeValues *typevalue.Cache, iter iteration.Iterator, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (product.Value, bool) {
	sourceType, ok := iteratorSourceType(reg, sourceValue, assertedSourceType, hasAssertedSourceType)
	if !ok {
		if iter.Kind == iteration.IterateIndexed {
			return iteratorDynamicIndexedElementValue(reg, sourceValue)
		}
		return product.Value{}, false
	}
	elem, ok := iteratorElementOf(sourceType)
	if !ok {
		if iter.Kind == iteration.IterateIndexed {
			return iteratorDynamicIndexedElementValue(reg, sourceValue)
		}
		return product.Value{}, false
	}
	return iteratorPresent(reg, typeValues.FromTypeWithWitness(reg, elem)), true
}

func iteratorDynamicIndexedElementValue(reg *axis.Registry, sourceValue product.Value) (product.Value, bool) {
	if reg == nil {
		return product.Value{}, false
	}
	if !iteratorSourceCouldBeTable(reg, sourceValue) {
		return product.Value{}, false
	}
	ev := product.Get(reg, sourceValue, evidence.Key)
	if ev.IsGradualTop() || ev.IsExplicitTop() {
		return iteratorPresent(reg, product.Set(reg, product.Top(), evidence.Key, ev)), true
	}
	return product.Value{}, false
}

func iteratorPresent(reg *axis.Registry, value product.Value) product.Value {
	if reg == nil {
		return value
	}
	return product.WithPresence(reg, value, presence.Present())
}

func iteratorSourceCouldBeTable(reg *axis.Registry, sourceValue product.Value) bool {
	kinds := product.Get(reg, sourceValue, runtimekind.Key)
	if kinds.IsTop() {
		return true
	}
	if kinds.IsBottom() {
		return false
	}
	return !runtimekind.Intersect(kinds, runtimekind.Singleton(runtimekind.Table)).IsBottom()
}

func iteratorKeyOf(source typ.Type) (typ.Type, bool) {
	return iteratorProjectContainer(source, projection.KeyOf)
}

func iteratorElementOf(source typ.Type) (typ.Type, bool) {
	return iteratorProjectContainer(source, projection.ElementOf)
}

func iteratorProjectContainer(source typ.Type, project func(typ.Type) (typ.Type, bool)) (typ.Type, bool) {
	if project == nil {
		return nil, false
	}
	if union, ok := unwrap.Alias(source).(*typ.Union); ok {
		members := make([]typ.Type, 0, len(union.Members))
		for _, member := range union.Members {
			member = unwrap.NormalizeNil(member)
			if member == nil || member.Kind() == kind.Nil || iteratorEmptyRecordWitness(member) {
				continue
			}
			projected, ok := project(member)
			if !ok {
				return nil, false
			}
			members = append(members, projected)
		}
		if len(members) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(members...), true
	}
	if iteratorEmptyRecordWitness(source) {
		return nil, false
	}
	return project(source)
}

func iteratorEmptyRecordWitness(source typ.Type) bool {
	rec, ok := unwrap.Alias(source).(*typ.Record)
	return ok && rec != nil &&
		len(rec.Fields) == 0 &&
		len(rec.StaticMembers) == 0 &&
		rec.MapKey == nil &&
		rec.MapValue == nil &&
		rec.Metatable == nil &&
		!rec.Open
}

func iteratorSourceType(reg *axis.Registry, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (typ.Type, bool) {
	if hasAssertedSourceType {
		return assertedSourceType, true
	}
	sourceType, ok := ObjectLiteralEntryType(reg, nil, sourceValue)
	if !ok {
		return nil, false
	}
	return narrowIteratorSourceTypeByRuntimeKind(reg, sourceValue, sourceType)
}

func narrowIteratorSourceTypeByRuntimeKind(reg *axis.Registry, sourceValue product.Value, sourceType typ.Type) (typ.Type, bool) {
	kinds := product.Get(reg, sourceValue, runtimekind.Key)
	if kinds.IsTop() {
		return sourceType, true
	}
	if kinds.IsBottom() {
		return nil, false
	}
	return narrowTypeByRuntimeKind(sourceType, kinds)
}

func narrowTypeByRuntimeKind(t typ.Type, want runtimekind.Value) (typ.Type, bool) {
	switch tt := t.(type) {
	case *typ.Union:
		members := make([]typ.Type, 0, len(tt.Members))
		for _, member := range tt.Members {
			narrowed, ok := narrowTypeByRuntimeKind(member, want)
			if !ok {
				continue
			}
			members = append(members, narrowed)
		}
		if len(members) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(members...), true
	default:
		got, ok := typevalue.RuntimeKindFromType(t)
		if !ok {
			return t, true
		}
		if runtimekind.Intersect(got, want).IsBottom() {
			return nil, false
		}
		return t, true
	}
}
