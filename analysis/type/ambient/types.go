// Package ambient owns named runtime types that are always available to
// analysis type annotations.
package ambient

import "github.com/wippyai/go-lua/analysis/type/typ"

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
