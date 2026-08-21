package signature

// EscapeVocabVersion pins the signature escape/ownership vocabulary consumed
// across the runtime boundary. It covers placement.Escape's manifest label set and audited
// ownership capability labels currently synced with arena CallArgEscape and
// Ownership. Bump it only with joint cross-repo signoff per fence #1425 when a
// label is added, removed, renamed, or changes boundary meaning.
//
// D3: Export, Opaque, and Freeze are active formal ownership declarations now
// that Target/Placement consume the declaration row. Export and Opaque require
// shared placement; Freeze is non-escaping and constrains mutability without
// changing placement. All three therefore survive serialized manifests.
const EscapeVocabVersion = 3
