package seal

import "github.com/wippyai/go-lua/analysis/schema"

// LawID ordinals below SurfaceLawFloor belong to the seal subsystem. Surface
// packages allocate their own laws above that floor so a verdict can never
// confuse a catalog failure with a surface failure.
const (
	LawNone schema.LawID = iota
	LawSurfaceCatalog
	LawSurfaceUnique
	LawEntryPresent
	LawEntryIdentity
	LawEntryAdmissible
	LawEntryUnique
	LawSurfaceCoverage
	LawSurfacePhase
	LawEntryContent
	LawReference

	// SurfaceLawFloor is the first law ordinal a surface may claim.
	SurfaceLawFloor schema.LawID = 1024
)

// SurfaceLawFailure constructs a verdict for a surface-owned law. Claiming a
// root law is itself a malformed surface declaration and is attributed to the
// catalog law instead of silently changing the meaning of the verdict.
func SurfaceLawFailure(kind schema.SurfaceKind, entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	if law < SurfaceLawFloor {
		return schema.SealFailure{
			Contributor: kind,
			Entry:       entry,
			Law:         LawSurfaceCatalog,
			Disposition: schema.DispositionMalformed,
		}
	}
	return schema.SealFailure{Contributor: kind, Entry: entry, Law: law, Disposition: disposition}
}
