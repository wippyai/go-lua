package seal

import "github.com/wippyai/go-lua/analysis/schema"

type referenceSnapshot struct {
	owner schema.EntryID
	value schema.EntryReference
}

type referenceBatch struct {
	kind   schema.SurfaceKind
	values []referenceSnapshot
}

// snapshotReferences collects every common reference provider once. The
// resulting values are copied, so a surface that mutates its source reference
// slice while running Seal cannot change what the post-surface pass validates.
func snapshotReferences(kind schema.SurfaceKind, entries []schema.Entry, surface Surface) []referenceSnapshot {
	var snapshots []referenceSnapshot
	appendProvider := func(owner schema.EntryID, provider any) {
		values, ok := referenceValues(provider)
		if !ok {
			return
		}
		for _, value := range values {
			snapshots = append(snapshots, referenceSnapshot{owner: owner, value: value})
		}
	}

	appendProvider(schema.EntryID{}, surface)
	for _, entry := range entries {
		if isNil(entry) {
			continue
		}
		appendProvider(schema.NewEntryID(kind, entry.Key()), entry)
	}
	return snapshots
}

// referenceValues accepts only the explicit root-owned provider interfaces. A
// declaration that wants the common post-surface reference law must opt into
// one of those contracts; the seal does not discover hidden adapters through
// reflection.
func referenceValues(provider any) (schema.EntryReferences, bool) {
	if typed, ok := provider.(schema.ReferenceProvider); ok {
		return typed.References().Clone(), true
	}
	if typed, ok := provider.(schema.EntryReferenceProvider); ok {
		return typed.EntryReferences().Clone(), true
	}
	return nil, false
}

func validateReferences(resolver Resolver, kind schema.SurfaceKind, references []referenceSnapshot) schema.SealFailure {
	for _, snapshot := range references {
		reference := snapshot.value
		if !reference.Available() {
			return schema.SealFailure{
				Contributor: kind,
				Entry:       snapshot.owner,
				Law:         LawReference,
				Disposition: schema.DispositionMalformed,
			}
		}
		if _, disposition := resolver.ResolveReference(reference); disposition != schema.DispositionAccepted {
			return schema.SealFailure{
				Contributor: kind,
				Entry:       snapshot.owner,
				Law:         LawReference,
				Disposition: disposition,
			}
		}
	}
	return schema.SealFailure{}
}
