package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

func objectLiteralEvaluator(reg *axis.Registry, typeValues *typevalue.Cache) sourcevalue.ObjectLiteralEvaluator {
	return func(lit factflow.ObjectLiteral, resolve func(factflow.ValueSource) (product.Value, bool)) (product.Value, bool) {
		t, ok := luasourcevalue.ObjectLiteralTypeCached(reg, typeValues, lit, resolve)
		if !ok {
			return product.Value{}, false
		}
		value := typeValues.FromTypeWithWitness(reg, t)
		if id, ok := lit.Identity(); ok {
			value = product.Set(reg, value, identity.Key, identity.Singleton(id))
		}
		return product.Set(reg, value, escape.Key, escape.Fresh()), true
	}
}
