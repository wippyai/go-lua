package signaturelookup

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/derive"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// deriveManifestSignature recovers a signature for a module member that carries
// no explicit entry, by resolving the member's type from the module's own
// exported and defined types and running the effect-derivation rules over it.
//
// This is how typed manifest members participate in call-result materialization
// even when they carry no explicit effect row. If a canonical error type is
// configured, the member's type can additionally encode (value,
// Optional(ErrorType)) and the derivation recovers that presence correlation.
// Resolution is the lookup's own job - it maps a member name to the type the
// manifest already declares - so nothing is hardcoded and no effect is
// registered by hand.
func deriveManifestSignature(m *manifest.Manifest, name string) (signature.Function, bool) {
	if m == nil {
		return signature.Function{}, false
	}
	local, ok := manifestLocalSignatureName(m.Path, name)
	if !ok {
		return signature.Function{}, false
	}
	fn, ok := resolveMemberFunction(m, local)
	if !ok || fn == nil {
		return signature.Function{}, false
	}
	row := derive.ApplyDefault(fn, effect.Empty, derive.Context{ErrorType: m.ErrorType})
	return signature.Function{Type: fn, Effect: row}, true
}

// resolveMemberFunction maps a module-local member name to its function type. A
// dotted name (Type.method) selects a method of a defined interface; a bare name
// selects a member of the module export.
func resolveMemberFunction(m *manifest.Manifest, local string) (*typ.Function, bool) {
	if i := strings.LastIndex(local, "."); i >= 0 {
		typeName, method := local[:i], local[i+1:]
		t, ok := m.Types[typeName]
		if !ok {
			return nil, false
		}
		return interfaceMethodType(t, method)
	}
	return memberFunctionType(m.Export, local)
}

// interfaceMethodType returns the function type of method on t when t is (or
// wraps) an interface declaring it.
func interfaceMethodType(t typ.Type, method string) (*typ.Function, bool) {
	iface, ok := typ.UnwrapTransparentWrappers(t).(*typ.Interface)
	if !ok || iface == nil {
		return nil, false
	}
	for _, m := range iface.Methods {
		if m.Name == method {
			return m.Type, m.Type != nil
		}
	}
	return nil, false
}

// memberFunctionType returns the function type of the named member of an export
// type, treating interfaces, records, and their intersections uniformly.
func memberFunctionType(t typ.Type, name string) (*typ.Function, bool) {
	switch n := typ.UnwrapTransparentWrappers(t).(type) {
	case *typ.Interface:
		return interfaceMethodType(n, name)
	case *typ.Record:
		for _, field := range n.Fields {
			if field.Name == name {
				fn, ok := typ.UnwrapTransparentWrappers(field.Type).(*typ.Function)
				return fn, ok && fn != nil
			}
		}
	case *typ.Intersection:
		for _, member := range n.Members {
			if fn, ok := memberFunctionType(member, name); ok {
				return fn, true
			}
		}
	}
	return nil, false
}
