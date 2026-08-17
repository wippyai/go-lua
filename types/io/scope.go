package io

import (
	"github.com/wippyai/go-lua/domain/type/transform"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/types/signature"
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
	return manifestTypeScoper{
		manifest:  m,
		resolving: make(map[string]bool),
		scoped:    make(map[string]typ.Type),
	}.scope(t)
}

// manifestTypeScoper expands local definitions while retaining a module path
// at recursive reference edges. A manifest definition may itself contain a
// local reference (Entry -> EntryID), so replacing only the outermost
// reference would still leak an unqualified name into the consumer namespace.
type manifestTypeScoper struct {
	manifest  *Manifest
	resolving map[string]bool
	scoped    map[string]typ.Type
}

func (s manifestTypeScoper) scope(t typ.Type) typ.Type {
	return transform.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		// A structural function value can cross the equation boundary with a
		// freshly decoded recursive or generic graph. Reattach such a graph only
		// when the defining manifest publishes an exact matching declaration.
		// This preserves the provider's identity rather than treating a consumer
		// reconstruction as a new recursive family.
		if provider, ok := s.providerType(node); ok {
			return provider, true
		}
		ref, ok := node.(*typ.Ref)
		if !ok || ref.Module != "" || ref.Name == "" || ref.Name == "self" {
			return nil, false
		}
		if local := s.manifest.Types[ref.Name]; local != nil {
			if scoped, ok := s.scoped[ref.Name]; ok {
				return scoped, true
			}
			if s.resolving[ref.Name] {
				return typ.NewRef(s.manifest.Path, ref.Name), true
			}
			s.resolving[ref.Name] = true
			scoped := s.scope(local)
			delete(s.resolving, ref.Name)
			s.scoped[ref.Name] = scoped
			return scoped, true
		}
		return typ.NewRef(s.manifest.Path, ref.Name), true
	})
}

func (s manifestTypeScoper) providerType(node typ.Type) (typ.Type, bool) {
	if s.manifest == nil || len(s.manifest.Types) == 0 {
		return nil, false
	}
	var name string
	switch value := node.(type) {
	case *typ.Recursive:
		name = value.Name
	case *typ.Generic:
		name = value.Name
	default:
		return nil, false
	}
	if name == "" {
		return nil, false
	}
	provider := s.manifest.Types[name]
	if provider == node {
		// The provider graph is already authoritative. Do not descend into its
		// recursive body, which would rebuild a local-only graph.
		return node, true
	}
	if provider == nil || !typ.TypeEquals(provider, node) {
		return nil, false
	}
	return provider, true
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
