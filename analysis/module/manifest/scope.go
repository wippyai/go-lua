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
		// A recursive definition is already its own identity boundary. Descending
		// into its body would re-expand its local self references every time a
		// manifest source returns the type, turning a finite recursive graph into
		// an ever-growing tree at successive lookup boundaries.
		if _, ok := node.(*typ.Recursive); ok {
			return node, true
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
