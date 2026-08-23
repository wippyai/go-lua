// Package ambient owns named runtime types that are always available to
// analysis type annotations.
//
// The catalogue below is the single declaration of that namespace. Every
// ambient type is a nominal shape carrying its own name, optionally binding
// ordered type parameters, so one row states the whole declaration: the
// runtime type vocabulary materializes it as a value, and the source lowering
// materializes it as an ordinary Program declaration that annotations resolve
// against like any authored one.
package ambient

import (
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

const Channel = "Channel"
const RuntimeModuleChannel = "channel.Channel"
const Table = typ.BuiltinTableTopName

// Declaration is one ambient named type: the spelling an annotation writes and
// the ordered type parameters it binds. A declaration with no parameters is a
// plain nominal type; a declaration with parameters is the generic over that
// same nominal body.
type Declaration struct {
	Name   string
	Params []string
}

// declarations is the complete ambient namespace, in canonical name order.
var declarations = []Declaration{
	{Name: Channel, Params: []string{"T"}},
	{Name: Table},
}

// Declarations returns the complete ambient namespace in canonical name order.
// The result is a fresh enumeration; the catalogue above stays the only
// authority.
func Declarations() []Declaration {
	out := make([]Declaration, len(declarations))
	copy(out, declarations)
	return out
}

// Lookup returns the ambient declaration named by name.
func Lookup(name string) (Declaration, bool) {
	for _, declaration := range declarations {
		if declaration.Name == name {
			return declaration, true
		}
	}
	return Declaration{}, false
}

// Type returns the runtime type this declaration denotes: its nominal body, or
// the generic binding its parameters over that body.
func (declaration Declaration) Type() typ.Type {
	body := typ.NewInterface(declaration.Name, nil)
	if len(declaration.Params) == 0 {
		return body
	}
	params := make([]*typ.TypeParam, len(declaration.Params))
	for index, name := range declaration.Params {
		params[index] = typ.NewTypeParam(name, nil)
	}
	return typ.NewGeneric(declaration.Name, params, body)
}

// ChannelGeneric returns the runtime channel marker generic Channel<T>.
func ChannelGeneric() *typ.Generic {
	declaration, ok := Lookup(Channel)
	if !ok {
		return nil
	}
	generic, ok := declaration.Type().(*typ.Generic)
	if !ok {
		return nil
	}
	return generic
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
