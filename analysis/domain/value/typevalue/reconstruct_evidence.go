package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func runtimeKindType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	members := make([]typ.Type, 0, len(kinds.Tags()))
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil:
			members = append(members, typ.Nil)
		case runtimekind.Boolean:
			members = append(members, typ.Boolean)
		case runtimekind.Number:
			members = append(members, typ.Number)
		case runtimekind.String:
			members = append(members, typ.String)
		case runtimekind.Table:
			members = append(members, typ.BuiltinTableTopMarker())
		case runtimekind.Function:
			members = append(members, typ.Func().Variadic(typ.Any).Returns(typ.Any).Build())
		default:
			return nil, false
		}
	}
	t, ok := typenormalize.UnionType(members)
	if !ok {
		return nil, false
	}
	return typeWithPresence(t, product.PresenceOf(value)), true
}
