package transformer

// LuaTypeNameValue retains Lua's canonical type(arg) operation in the value
// DAG. The argument remains symbolic until caller binding; no callee name or
// signature spelling is retained in the transformer.
func (a *Arena) LuaTypeNameValue(arg ValueTerm) ValueTerm {
	if a == nil || arg == 0 || int(arg) >= len(a.values) {
		return 0
	}
	return a.internValue(valueNode{op: valueLuaTypeName, args: []ValueTerm{arg}})
}
