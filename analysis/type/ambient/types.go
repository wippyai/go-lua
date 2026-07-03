// Package ambient owns named runtime types that are always available to
// analysis type annotations.
package ambient

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const Channel = "Channel"
const RuntimeModuleChannel = "channel.Channel"
const Table = typetable.BuiltinTopName

// Lookup returns a fresh ambient type definition for name.
func Lookup(name string) (typ.Type, bool) {
	switch name {
	case Channel:
		return ChannelGeneric(), true
	case Table:
		return typetable.BuiltinTopMarker(), true
	default:
		return nil, false
	}
}

// ChannelGeneric returns the runtime channel marker generic Channel<T>.
func ChannelGeneric() *typ.Generic {
	param := typ.NewTypeParam("T", nil)
	return typ.NewGeneric(Channel, []*typ.TypeParam{param}, typ.NewInterface(Channel, nil))
}

// ChannelPayloadType returns the payload carried by the canonical runtime
// channel instantiations. Source annotations spell the built-in marker as
// Channel<T>; module manifests may publish the same runtime ABI as
// channel.Channel<T>.
func ChannelPayloadType(t typ.Type) (typ.Type, bool) {
	inst, ok := unwrap.Alias(unwrap.Annotations(t)).(*typ.Instantiated)
	if !ok || inst.Generic == nil || !IsRuntimeChannelName(inst.Generic.Name) || len(inst.TypeArgs) != 1 || inst.TypeArgs[0] == nil {
		return nil, false
	}
	return inst.TypeArgs[0], true
}

// IsRuntimeChannelName reports whether name identifies the supported runtime
// channel ABI. This is intentionally exact: arbitrary user-defined
// Something.Channel<T> types are not treated as channel runtime values.
func IsRuntimeChannelName(name string) bool {
	return name == Channel || name == RuntimeModuleChannel
}
