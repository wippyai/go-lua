package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// RuntimeIndex projects a dynamic-index read from the type witnesses carried
// by tableValue and keyValue.
func RuntimeIndex(reg *axis.Registry, tableValue, keyValue product.Value) (product.Value, bool) {
	return runtimeIndex(reg, tableValue, keyValue, nil)
}

// RuntimeIndex projects a dynamic-index read through this cache's type query
// surface.
func (c *Cache) RuntimeIndex(reg *axis.Registry, tableValue, keyValue product.Value) (product.Value, bool) {
	return runtimeIndex(reg, tableValue, keyValue, c)
}

func runtimeIndex(reg *axis.Registry, tableValue, keyValue product.Value, cache *Cache) (product.Value, bool) {
	tableType, tableOK := cache.TypeOf(reg, tableValue)
	keyType, keyOK := cache.TypeOf(reg, keyValue)
	if !tableOK || tableType == nil {
		return product.Value{}, false
	}
	if !keyOK || keyType == nil {
		keyType = typ.Unknown
	}
	projected, ok := access.RuntimeIndex(tableType, keyType)
	if !ok || projected == nil {
		return product.Value{}, false
	}
	if cache != nil {
		return WithWitness(reg, cache.FromType(reg, projected), projected), true
	}
	return WithWitness(reg, FromType(reg, projected), projected), true
}
