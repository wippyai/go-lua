package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// The runtime types are not a Lua library. They are the named TYPES the host
// runtime publishes and a type annotation resolves: the channel a program spells
// as Channel<T>, and the top of the table lattice a program spells as `table`.
// Neither is a value, so neither is an export a mounted aggregate hands out, and
// this instance publishes no root export value at all - a contract that claimed
// an aggregate here would be claiming a runtime table nobody mounts.
//
// What it publishes is the type itself, addressed by the export key an
// annotation spells it under. That is what makes the name resolvable from
// contract data: today a resolver carries its own table of built-in type names,
// which is a vocabulary its own package invented, and a name it did not think of
// resolves to nothing anywhere.
//
// The types are authored here rather than read out of the ambient package, for
// the reason every signature in this package is authored beside the inventory
// that names it: an instance is the authority for what it says, and a type read
// out of another table at authoring time would make that table the authority and
// this instance its projection. The ambient package still exists, and while it
// does the drift law derives what IT must hold from this instance.
//
// The module spelling of the same runtime ABI - channel.Channel<T>, a named type
// published by a mounted runtime module - is that module's own contract to
// author. This instance publishes the ambient names and does not speak for a
// mount it is not.

// RuntimeRoot is the authored mount selector of the ambient runtime types.
const RuntimeRoot = "runtime"

// runtimeTypeExports is the authored inventory of published type names, in
// canonical order.
var runtimeTypeExports = []string{"Channel", "table"}

// RuntimeTypeExports returns a copy of the authored type export inventory.
func RuntimeTypeExports() []string { return copyNames(runtimeTypeExports) }

// runtimeTypes is the authored type published under each name.
//
// Channel is a generic declaration over one payload parameter whose body is the
// channel marker interface: the runtime carries the payload across the boundary
// and publishes no member on it, so the marker is what the type is rather than a
// shape a program may reach into. The table top is the marker interface with no
// members, which is the top of the table lattice: every table is one, and
// nothing is guaranteed of it.
func runtimeTypes() map[string]typ.Type {
	return map[string]typ.Type{
		"Channel": typ.NewGeneric(
			"Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil)),
		"table": typ.NewInterface("table", nil),
	}
}

// RuntimeContract authors the ambient runtime types as an instance of the
// declared library kind. It publishes one type export per name and nothing else:
// a named type is the whole of what this contract has to say.
func RuntimeContract(kind *library.Entry) (*contract.Instance, bool) {
	if kind == nil || kind.Class() != library.ClassLibrary {
		return nil, false
	}
	published := runtimeTypes()
	members := make([]contract.Member, 0, len(runtimeTypeExports))
	for _, name := range runtimeTypeExports {
		body, err := wire.EncodeExportType(published[name])
		if err != nil {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormExportType, contract.Export(name), body))
	}
	return contract.New(contract.Spec{
		Kind:    kind.Key(),
		Codec:   kind.Codec(),
		Root:    RuntimeRoot,
		Members: members,
	}, kind)
}
