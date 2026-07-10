package manifest

import (
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ScopeType gives bare references carried by a manifest its module identity.
//
// A manifest is a module-boundary artifact, so a reference such as Entry in
// its exported type means <manifest path>.Entry, never an arbitrary Entry from
// the consumer's merged manifest set. Keeping this association at the boundary
// prevents unrelated manifests from widening a provider's contract.
func (m *Manifest) ScopeType(t typ.Type) typ.Type {
	if m == nil || m.Path == "" || t == nil {
		return t
	}
	return transform.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		ref, ok := node.(*typ.Ref)
		if !ok || ref.Module != "" || ref.Name == "" || ref.Name == "self" {
			return nil, false
		}
		if local := m.Types[ref.Name]; local != nil {
			return local, true
		}
		return typ.NewRef(m.Path, ref.Name), true
	})
}

// ScopeSignature applies the manifest's module identity to the callable type
// carried by an effect signature. Callers retain ownership of the signature
// value and may clone it according to their normal lookup policy.
func (m *Manifest) ScopeSignature(sig signature.Function) signature.Function {
	if sig.Type == nil {
		return sig
	}
	if scoped, ok := m.ScopeType(sig.Type).(*typ.Function); ok {
		sig.Type = scoped
	}
	return sig
}
