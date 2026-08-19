package channelselect

import (
	"github.com/wippyai/go-lua/analysis/identity"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// ModuleName is the runtime channel module. It is not Channel<T>.
const ModuleName = "channel"

// ModuleType is the nominal type of the channel module value.
func ModuleType() typ.Type {
	return typ.NewInterface(ModuleName, nil)
}

// IsModuleType reports the nominal channel module, not a user record and not
// a Channel instance.
func IsModuleType(t typ.Type) bool {
	iface, ok := unwrap.Alias(unwrap.Annotations(t)).(*typ.Interface)
	return ok && iface != nil && iface.Name == ModuleName && len(iface.Methods) == 0
}

// SelectFunction is the unspecialized channel.select signature. Precise
// result unions and CaseSet rows come from SpecializeSelect.
func SelectFunction() *typ.Function {
	result, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: 0, Channel: typ.Any, Payload: typ.Any},
	}, true)
	if !ok {
		return nil
	}
	return typ.Func().
		Param("cases", typetable.NewRecord().SetOpen(true).Build()).
		Returns(result).
		Build()
}

// IsSelectFunction reports the unspecialized channel.select signature.
func IsSelectFunction(t typ.Type) bool {
	fn, ok := unwrap.Alias(unwrap.Annotations(t)).(*typ.Function)
	return ok && typ.TypeEquals(fn, SelectFunction())
}

// SpecializeSelect builds the honest select result and accepted CaseSet from
// a parent-issued site and the select argument types. Only nominal cases in
// the first argument table are admitted.
func SpecializeSelect(site identity.ContentID, args []typ.Type) (typ.Type, CaseSet, bool) {
	if !site.Available() || len(args) == 0 || args[0] == nil {
		return nil, CaseSet{}, false
	}
	cases, hasDefault, ok := CasesFromTable(args[0])
	if !ok {
		return nil, CaseSet{}, false
	}
	return SelectFromCases(site, cases, hasDefault)
}
