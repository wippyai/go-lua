package semanticsource

import "github.com/wippyai/go-lua/analysis/identity"

// DigestView is one detached owner-fenced interval of typed row identities.
// It stores identities only: the owner's typed rows never cross the boundary,
// so a receipt can be replayed and compared without copying relation payload.
type DigestView struct {
	owner   identity.ContentID
	digests []identity.ContentID
}

// SealDigestView detaches one owner-fenced identity interval. Every identity
// must be derived; an unavailable owner or identity rejects the whole view.
func SealDigestView(owner identity.ContentID, digests []identity.ContentID) (DigestView, bool) {
	view := DigestView{owner: owner, digests: append([]identity.ContentID(nil), digests...)}
	if !view.Valid() {
		return DigestView{}, false
	}
	return view, true
}

// Valid reports whether the view names an available owner and only derived
// row identities.
func (view DigestView) Valid() bool {
	if !view.owner.Available() {
		return false
	}
	for _, digest := range view.digests {
		if !digest.Available() {
			return false
		}
	}
	return true
}

// OwnerID reports the sealed identity that owns this interval.
func (view DigestView) OwnerID() identity.ContentID { return view.owner }

// Count reports the detached typed row count, including zero. A malformed
// view never exposes a partial count.
func (view DigestView) Count() int {
	if !view.Valid() {
		return 0
	}
	return len(view.digests)
}

// DigestAt returns one canonical typed-row identity. Both the validity fence
// and the backing length bound the access, so no phantom row can leak.
func (view DigestView) DigestAt(index int) (identity.ContentID, bool) {
	if !view.Valid() || index < 0 || index >= len(view.digests) {
		return identity.ContentID{}, false
	}
	return view.digests[index], true
}

// Digests returns an owner-independent copy of the canonical identities.
func (view DigestView) Digests() []identity.ContentID {
	if !view.Valid() {
		return nil
	}
	return append([]identity.ContentID(nil), view.digests...)
}

// FencedDigestViews reports whether every view is a complete interval fenced
// to the same owner identity.
func FencedDigestViews(owner identity.ContentID, views ...DigestView) bool {
	if !owner.Available() {
		return false
	}
	for _, view := range views {
		if !view.Valid() || view.owner != owner {
			return false
		}
	}
	return true
}

// DigestCursor walks one exact owner-fenced identity interval. It retains the
// view by value, so traversal cannot observe a later owner state.
type DigestCursor struct {
	view  DigestView
	index int
}

// Cursor creates a detached cursor over the fenced identities.
func (view DigestView) Cursor() DigestCursor { return DigestCursor{view: view} }

// Next returns each identity at most once and then terminates.
func (cursor *DigestCursor) Next() (identity.ContentID, bool) {
	if cursor == nil || !cursor.view.Valid() || cursor.index < 0 || cursor.index >= len(cursor.view.digests) {
		return identity.ContentID{}, false
	}
	digest := cursor.view.digests[cursor.index]
	cursor.index++
	return digest, digest.Available()
}

// OriginPublications seals one publication for every catalog definition owned
// by the given origins, in canonical schema order. The counts hook resolves
// the owner's cardinality for one relation token; an unresolved token, an
// unknown definition, or a rejected claim fails the whole projection.
func OriginPublications(schema ProgramSchema, counts func(Token) (int, bool), origins ...Origin) []Publication {
	if counts == nil || len(origins) == 0 {
		return nil
	}
	definitions, _, err := schemaDefinitions(schema)
	if err != nil {
		return nil
	}
	rows := make([]Publication, 0, len(definitions))
	for _, definition := range definitions {
		token := definition.Token()
		if !ownedOrigin(origins, token.Origin()) {
			continue
		}
		count, resolved := counts(token)
		if !resolved {
			return nil
		}
		publication, err := SealPublication(definition, count)
		if err != nil {
			return nil
		}
		rows = append(rows, publication)
	}
	return rows
}

func ownedOrigin(origins []Origin, origin Origin) bool {
	for _, candidate := range origins {
		if candidate == origin {
			return true
		}
	}
	return false
}
