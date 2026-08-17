package static

import (
	"errors"

	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// runtimeKindTable is the sealed dense RuntimeKind interpretation. It stays
// separate while Authority's structural-runtime cut is in flight; the final
// Authority splice stores exactly one of these tables, with no map or second
// query route.
type runtimeKindTable struct{ result [256]uint32 }

// RuntimeTypeOf interprets Static's complete closed runtime-kind vocabulary as
// one admitted Static result. It is a dense table lookup: no Value state,
// typ graph, or construction index survives this query.
func (a *Authority) RuntimeTypeOf(kinds runtimekind.Set) (Value, bool) {
	if a == nil {
		return Value{}, false
	}
	return a.runtimeKinds.lookup(a, kinds)
}

// lookup interprets Static's complete closed runtime-kind vocabulary as one
// admitted Static result. It is a dense table lookup: no Value state, typ
// graph, or construction index survives this query.
func (table *runtimeKindTable) lookup(a *Authority, kinds runtimekind.Set) (Value, bool) {
	if a == nil || table == nil || !kinds.Valid() {
		return Value{}, false
	}
	index := table.result[uint8(kinds)]
	if uint64(index) >= uint64(len(a.results)) {
		return Value{}, false
	}
	result := Value{owner: a, index: index}
	return result, a.Owns(result)
}

// sealRuntimeKinds admits the complete 8-bit Static runtime-kind denominator before
// Static seals its order. The vocabulary's opaque partition - the reference
// families the analyzer models no structure for - shares the one closed Unknown
// absorber: OptionalUnknown is structurally distinct from Unknown and would
// break the carrier's strict extensional monotonicity. The partition is named
// by runtimekind, so a family that joins or leaves it moves this seal with it.
func sealRuntimeKinds(a *Authority) (runtimeKindTable, error) {
	var table runtimeKindTable
	if a == nil || a.closedByBytes == nil || len(a.results) < 2 {
		return table, errors.New("static: unavailable runtime-kind construction")
	}
	function, ok := typ.BuiltinPrimitiveType("function")
	if !ok || function == nil {
		return table, errors.New("static: builtin function type unavailable")
	}
	for raw := 0; raw <= int(runtimekind.All); raw++ {
		if raw == 0 {
			table.result[raw] = a.Bottom().index
			continue
		}
		var result Value
		var err error
		if runtimekind.Set(raw)&runtimekind.Opaque != 0 {
			result, err = a.addClosed(typ.Unknown)
		} else {
			var members [6]typ.Type
			count := 0
			if runtimekind.Set(raw).Contains(runtimekind.Nil) {
				members[count] = typ.Nil
				count++
			}
			if runtimekind.Set(raw).Contains(runtimekind.Boolean) {
				members[count] = typ.Boolean
				count++
			}
			if runtimekind.Set(raw).Contains(runtimekind.Number) {
				members[count] = typ.Number
				count++
			}
			if runtimekind.Set(raw).Contains(runtimekind.String) {
				members[count] = typ.String
				count++
			}
			if runtimekind.Set(raw).Contains(runtimekind.Table) {
				members[count] = typ.BuiltinTableTopMarker()
				count++
			}
			if runtimekind.Set(raw).Contains(runtimekind.Function) {
				members[count] = function
				count++
			}
			result, err = a.addClosed(typeexpr.Union(members[:count]...))
		}
		if err != nil || !result.IsClosed() || result.index == 1 {
			return table, errors.New("static: runtime-kind result admission failed")
		}
		table.result[raw] = result.index
	}
	masks := make([]runtimekind.Set, len(a.results))
	values := make([]bool, len(a.results))
	for raw := 1; raw <= int(runtimekind.All); raw++ {
		index := table.result[raw]
		if uint64(index) < uint64(len(masks)) {
			masks[index] |= runtimekind.Set(raw)
			values[index] = true
		}
	}
	a.runtimeKindMask = masks
	a.runtimeKindValue = values
	return table, nil
}
