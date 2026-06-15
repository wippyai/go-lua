// Package ambient owns named runtime types that are always available to
// analysis type annotations.
package ambient

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const Channel = "Channel"

// Lookup returns a fresh ambient type definition for name.
func Lookup(name string) (typ.Type, bool) {
	switch name {
	case Channel:
		return ChannelGeneric(), true
	default:
		return nil, false
	}
}

// ChannelGeneric returns the runtime channel marker generic Channel<T>.
func ChannelGeneric() *typ.Generic {
	param := typ.NewTypeParam("T", nil)
	return typ.NewGeneric(Channel, []*typ.TypeParam{param}, typ.NewInterface(Channel, nil))
}

// ChannelPayloadType returns the payload carried by the ambient Channel<T> type.
func ChannelPayloadType(t typ.Type) (typ.Type, bool) {
	inst, ok := unwrap.Alias(unwrap.Annotations(t)).(*typ.Instantiated)
	if !ok || inst.Generic == nil || inst.Generic.Name != Channel || len(inst.TypeArgs) != 1 || inst.TypeArgs[0] == nil {
		return nil, false
	}
	return inst.TypeArgs[0], true
}
