package seal

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
)

// Builder collects surfaces before the one Seal transaction. It is not safe
// for concurrent use; a sealed Schema is.
type Builder struct {
	surfaces  []Surface
	installed [schema.SurfaceKindLimit]bool
	phase     schema.SurfaceKind
	rejected  schema.SealFailure
}

func NewBuilder() *Builder { return &Builder{} }

// Register admits one surface. The first rejection is retained and reported
// by Seal, so a caller may register the whole set before inspecting the
// verdict.
func (builder *Builder) Register(surface Surface) bool {
	if builder == nil {
		return false
	}
	if isNil(surface) {
		return builder.reject(schema.SealFailure{Law: LawSurfaceCatalog, Disposition: schema.DispositionMalformed})
	}
	kind := surface.Kind()
	if !kind.Available() {
		return builder.reject(schema.SealFailure{Contributor: kind, Law: LawSurfaceCatalog, Disposition: schema.DispositionMalformed})
	}
	if builder.installed[kind] {
		return builder.reject(schema.SealFailure{Contributor: kind, Law: LawSurfaceUnique, Disposition: schema.DispositionDuplicate})
	}
	// Registration order is the catalog order. Surface.Seal still receives a
	// resolver fenced to lower phases, while references are fully checked after
	// every surface has been published.
	if kind <= builder.phase {
		return builder.reject(schema.SealFailure{Contributor: kind, Law: LawSurfacePhase, Disposition: schema.DispositionMalformed})
	}
	builder.phase = kind
	builder.installed[kind] = true
	builder.surfaces = append(builder.surfaces, surface)
	return true
}

func (builder *Builder) reject(failure schema.SealFailure) bool {
	if !builder.rejected.Available() {
		builder.rejected = failure
	}
	return false
}

// Seal validates every registered surface and returns one immutable table.
// Coverage is total: every declared SurfaceKind must have been registered, so
// a surface added to the catalog but never wired fails loudly here.
func (builder *Builder) Seal() (*Schema, schema.SealFailure) {
	if builder == nil {
		return nil, schema.SealFailure{Law: LawSurfaceCatalog, Disposition: schema.DispositionIncomplete}
	}
	if builder.rejected.Available() {
		return nil, builder.rejected
	}
	if len(builder.surfaces) == 0 {
		return nil, schema.SealFailure{Law: LawSurfaceCatalog, Disposition: schema.DispositionIncomplete}
	}

	table := &Schema{}
	resolver := Resolver{phase: schema.SurfaceKindInvalid + 1}
	var referenceBatches []referenceBatch
	hash := sha256.New()
	var content framing.Writer
	if content.Reset(hash, contentDomain, contentVersion) != nil {
		return nil, schema.SealFailure{Law: LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
	}

	for _, surface := range builder.surfaces {
		kind := surface.Kind()
		entries := append([]schema.Entry(nil), surface.Entries()...)
		references := snapshotReferences(kind, entries, surface)
		view, failure := indexSurface(kind, entries, &content)
		if failure.Available() {
			return nil, failure
		}
		if failure = foldReferences(&content, kind, references); failure.Available() {
			return nil, failure
		}

		if failure = surface.Seal(view, Sealed{resolver: resolver}); failure.Available() {
			failure.Contributor = kind
			return nil, failure
		}
		table.views[kind] = view
		resolver.views[kind] = view
		resolver.phase = kind + 1
		referenceBatches = append(referenceBatches, referenceBatch{kind: kind, values: references})
	}

	for kind := schema.SurfaceKindInvalid + 1; kind < schema.SurfaceKindLimit; kind++ {
		if !resolver.views[kind].Available() {
			return nil, schema.SealFailure{Contributor: kind, Law: LawSurfaceCoverage, Disposition: schema.DispositionIncomplete}
		}
	}

	complete := Resolver{views: resolver.views, phase: schema.SurfaceKindLimit}
	for _, batch := range referenceBatches {
		if failure := validateReferences(complete, batch.kind, batch.values); failure.Available() {
			return nil, failure
		}
	}

	if content.Finish() != nil {
		return nil, schema.SealFailure{Law: LawEntryContent, Disposition: schema.DispositionMalformed}
	}
	copy(table.digest[:], hash.Sum(nil))
	if !table.digest.Available() {
		return nil, schema.SealFailure{Law: LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
	}
	return table, schema.SealFailure{}
}
