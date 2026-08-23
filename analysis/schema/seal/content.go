package seal

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	contentDomain                 = "wippy.analysis/schema/table"
	contentVersion         uint64 = 1
	contentRecordReference uint64 = 0xfffffff0
)

func foldReferences(content *framing.Writer, kind schema.SurfaceKind, references []referenceSnapshot) schema.SealFailure {
	for _, snapshot := range references {
		if content.Record(contentRecordReference) != nil ||
			content.Uint(uint64(kind)) != nil ||
			content.Bytes(snapshot.owner[:]) != nil ||
			content.Uint(uint64(snapshot.value.Surface)) != nil ||
			content.String(string(snapshot.value.Key)) != nil {
			return schema.SealFailure{
				Contributor: kind,
				Entry:       snapshot.owner,
				Law:         LawEntryContent,
				Disposition: schema.DispositionMalformed,
			}
		}
	}
	return schema.SealFailure{}
}

func isNil(value any) bool {
	return value == nil
}

// indexSurface admits and indexes one surface's rows. It states the identity
// and uniqueness laws every entry of every surface is subject to, and nothing
// about how many rows a surface holds: an inventory of none is indexed like
// any other, and the surface that requires members states that requirement in
// its own Seal.
func indexSurface(kind schema.SurfaceKind, entries []schema.Entry, content *framing.Writer) (View, schema.SealFailure) {
	index := make(map[schema.EntryID]int, len(entries))
	for position, entry := range entries {
		if isNil(entry) {
			return View{}, schema.SealFailure{Contributor: kind, Law: LawEntryPresent, Disposition: schema.DispositionMalformed}
		}
		id := schema.NewEntryID(kind, entry.Key())
		if !id.Available() {
			return View{}, schema.SealFailure{Contributor: kind, Law: LawEntryIdentity, Disposition: schema.DispositionMalformed}
		}
		if !entry.EntryAvailable() {
			return View{}, schema.SealFailure{Contributor: kind, Entry: id, Law: LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		}
		if _, duplicate := index[id]; duplicate {
			return View{}, schema.SealFailure{Contributor: kind, Entry: id, Law: LawEntryUnique, Disposition: schema.DispositionDuplicate}
		}
		index[id] = position
		if content.Record(uint64(kind)) != nil || content.Bytes(id[:]) != nil {
			return View{}, schema.SealFailure{Contributor: kind, Entry: id, Law: LawEntryIdentity, Disposition: schema.DispositionMalformed}
		}
		if entry.EntryContent(content) != nil {
			return View{}, schema.SealFailure{Contributor: kind, Entry: id, Law: LawEntryContent, Disposition: schema.DispositionMalformed}
		}
	}
	return View{kind: kind, entries: append([]schema.Entry(nil), entries...), index: index}, schema.SealFailure{}
}
