// Package stdlib owns the identity and mount catalogue of the native Lua
// standard libraries shipped by go-lua.
//
// The catalogue deliberately does not import the Lua runtime. The runtime
// binds these identities to native openers, while each catalogue entry owns its
// declaration provider. This keeps library identity in one provider-owned
// place and lets analysis remain a consumer of the resulting manifests.
package stdlib

import (
	"fmt"
	"sort"
)

// ID is the stable identity of one native standard library. It is distinct
// from Name because the base library is mounted directly into the global
// environment and therefore has an empty module name.
type ID string

const (
	Package   ID = "package"
	Base      ID = "base"
	Table     ID = "table"
	String    ID = "string"
	Math      ID = "math"
	Debug     ID = "debug"
	Coroutine ID = "coroutine"
	UTF8      ID = "utf8"
	Errors    ID = "errors"
)

// Public module names. These remain constants so existing runtime constants
// can alias them without introducing a second spelling authority.
const (
	PackageName   = "package"
	BaseName      = ""
	TableName     = "table"
	StringName    = "string"
	MathName      = "math"
	DebugName     = "debug"
	CoroutineName = "coroutine"
	UTF8Name      = "utf8"
	ErrorsName    = "errors"
)

// Mount describes how a library enters the initial Lua environment.
type Mount uint8

const (
	MountInvalid Mount = iota
	// MountGlobals installs the library's exports directly into _G.
	MountGlobals
	// MountModule installs the library under its non-empty module name.
	MountModule
)

// Library is one immutable catalogue entry.
type Library struct {
	id          ID
	name        string
	mount       Mount
	declaration func() declaration
}

func library(id ID, name string, mount Mount, declaration func() declaration) Library {
	return Library{id: id, name: name, mount: mount, declaration: declaration}
}

// ID returns the stable provider identity.
func (l Library) ID() ID { return l.id }

// Name returns the Lua module name. It is empty only for the base library.
func (l Library) Name() string { return l.name }

// Mount returns how the library enters the initial environment.
func (l Library) Mount() Mount { return l.mount }

// catalogue order is runtime open order. Package must initialize require
// machinery first, and base must initialize _G before named libraries open.
var catalogue = [...]Library{
	library(Package, PackageName, MountModule, packageDeclaration),
	library(Base, BaseName, MountGlobals, baseDeclaration),
	library(Table, TableName, MountModule, tableDeclaration),
	library(String, StringName, MountModule, stringDeclaration),
	library(Math, MathName, MountModule, mathDeclaration),
	library(Debug, DebugName, MountModule, debugDeclaration),
	library(Coroutine, CoroutineName, MountModule, coroutineDeclaration),
	library(UTF8, UTF8Name, MountModule, utf8Declaration),
	library(Errors, ErrorsName, MountModule, errorsDeclaration),
}

// Libraries returns the complete catalogue in runtime open order.
func Libraries() []Library {
	out := make([]Library, len(catalogue))
	copy(out, catalogue[:])
	return out
}

// Lookup returns the catalogue entry with id.
func Lookup(id ID) (Library, bool) {
	for _, entry := range catalogue {
		if entry.id == id {
			return entry, true
		}
	}
	return Library{}, false
}

// Binding associates one catalogue entry with provider-owned data. The same
// exact-coverage operation is used for native openers now and manifest
// providers as declarations migrate out of analysis.
type Binding[T any] struct {
	Library Library
	Value   T
}

// Bind requires exactly one value for every standard-library identity and
// returns bindings in catalogue order. Missing and foreign identities fail
// instead of silently creating a partial runtime or manifest surface.
func Bind[T any](values map[ID]T) ([]Binding[T], error) {
	unknown := make([]string, 0)
	for id := range values {
		if _, ok := Lookup(id); !ok {
			unknown = append(unknown, string(id))
		}
	}
	if len(unknown) != 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("stdlib: unknown library identities %q", unknown)
	}

	bound := make([]Binding[T], 0, len(catalogue))
	for _, entry := range catalogue {
		value, ok := values[entry.id]
		if !ok {
			return nil, fmt.Errorf("stdlib: no provider for %q", entry.id)
		}
		bound = append(bound, Binding[T]{Library: entry, Value: value})
	}
	return bound, nil
}
