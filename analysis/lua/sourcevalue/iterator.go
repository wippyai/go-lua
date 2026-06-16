package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func IteratorVariableValue(reg *axis.Registry, typeValues *typevalue.Cache, iter iteration.Iterator, variableIndex int, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (product.Value, bool) {
	switch variableIndex {
	case 0:
		return iteratorKeyValue(reg, typeValues, iter, sourceValue, assertedSourceType, hasAssertedSourceType)
	case 1:
		return iteratorElementValue(reg, typeValues, sourceValue, assertedSourceType, hasAssertedSourceType)
	default:
		return product.Value{}, false
	}
}

func iteratorKeyValue(reg *axis.Registry, typeValues *typevalue.Cache, iter iteration.Iterator, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (product.Value, bool) {
	switch iter.Kind {
	case iteration.IterateIndexed:
		return typeValues.FromTypeWithWitness(reg, typ.Integer), true
	case iteration.IterateKeyed:
		if sourceType, ok := iteratorSourceType(reg, sourceValue, assertedSourceType, hasAssertedSourceType); ok {
			if keyType, ok := projection.KeyOf(sourceType); ok {
				return typeValues.FromTypeWithWitness(reg, keyType), true
			}
		}
		return product.Value{}, false
	default:
		return product.Value{}, false
	}
}

func iteratorElementValue(reg *axis.Registry, typeValues *typevalue.Cache, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (product.Value, bool) {
	sourceType, ok := iteratorSourceType(reg, sourceValue, assertedSourceType, hasAssertedSourceType)
	if !ok {
		return product.Value{}, false
	}
	elem, ok := projection.ElementOf(sourceType)
	if !ok {
		return product.Value{}, false
	}
	return typeValues.FromTypeWithWitness(reg, elem), true
}

func iteratorSourceType(reg *axis.Registry, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (typ.Type, bool) {
	if hasAssertedSourceType {
		return assertedSourceType, true
	}
	return ObjectLiteralEntryType(reg, nil, sourceValue)
}
