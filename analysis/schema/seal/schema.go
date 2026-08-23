package seal

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Schema is the immutable catalog produced by a successful seal transaction.
type Schema struct {
	views  [schema.SurfaceKindLimit]View
	digest identity.ContentID
}

func (table *Schema) Available() bool {
	if table == nil || !table.digest.Available() {
		return false
	}
	for kind := schema.SurfaceKindInvalid + 1; kind < schema.SurfaceKindLimit; kind++ {
		if !table.views[kind].Available() {
			return false
		}
	}
	return true
}

func (table *Schema) Digest() identity.ContentID {
	if table == nil {
		return identity.ContentID{}
	}
	return table.digest
}

func (table *Schema) Surface(kind schema.SurfaceKind) (View, bool) {
	if table == nil || !kind.Available() {
		return View{}, false
	}
	view := table.views[kind]
	return view, view.Available()
}

func (table *Schema) Resolver() Resolver {
	if table == nil {
		return Resolver{}
	}
	return Resolver{views: table.views, phase: schema.SurfaceKindLimit}
}

func (table *Schema) Resolve(kind schema.SurfaceKind, key schema.Key) (schema.Entry, schema.Disposition) {
	return table.Resolver().Resolve(kind, key)
}

func (table *Schema) ResolveReference(reference schema.EntryReference) (schema.Entry, schema.Disposition) {
	return table.Resolver().ResolveReference(reference)
}
