package seal

import "github.com/wippyai/go-lua/analysis/schema"

// Resolver is the phase-fenced lookup capability made available while a
// surface is sealing. Its phase admits only views already published by the
// catalog transaction.
type Resolver struct {
	views [schema.SurfaceKindLimit]View
	phase schema.SurfaceKind
}

func (resolver Resolver) Complete() bool {
	return resolver.phase == schema.SurfaceKindLimit
}

func (resolver Resolver) Registered(kind schema.SurfaceKind) bool {
	return kind.Available() && kind < resolver.phase && resolver.views[kind].Available()
}

func (resolver Resolver) Surface(kind schema.SurfaceKind) (View, bool) {
	if !resolver.Registered(kind) {
		return View{}, false
	}
	return resolver.views[kind], true
}

func (resolver Resolver) Resolve(kind schema.SurfaceKind, key schema.Key) (schema.Entry, schema.Disposition) {
	if !kind.Available() || !key.Available() {
		return nil, schema.DispositionMalformed
	}
	if !resolver.Registered(kind) {
		return nil, schema.DispositionMalformed
	}
	entry, ok := resolver.views[kind].ByID(schema.NewEntryID(kind, key))
	if !ok {
		return nil, schema.DispositionIncomplete
	}
	return entry, schema.DispositionAccepted
}

func (resolver Resolver) ResolveReference(reference schema.EntryReference) (schema.Entry, schema.Disposition) {
	return resolver.Resolve(reference.Surface, reference.Key)
}

// Sealed is the lower-phase capability passed to Surface.Seal.
type Sealed struct {
	resolver Resolver
}

func (sealed Sealed) Registered(kind schema.SurfaceKind) bool {
	return sealed.resolver.Registered(kind)
}

func (sealed Sealed) Surface(kind schema.SurfaceKind) (View, bool) {
	return sealed.resolver.Surface(kind)
}

func (sealed Sealed) Resolve(kind schema.SurfaceKind, key schema.Key) (schema.Entry, schema.Disposition) {
	return sealed.resolver.Resolve(kind, key)
}

func (sealed Sealed) ResolveReference(reference schema.EntryReference) (schema.Entry, schema.Disposition) {
	return sealed.resolver.ResolveReference(reference)
}

func (sealed Sealed) Resolver() Resolver {
	return sealed.resolver
}
