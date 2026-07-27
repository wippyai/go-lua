package signature

// EscapeVocabVersion pins the signature escape/ownership vocabulary consumed
// across the runtime boundary. It covers placement.Escape's manifest label set and audited
// ownership capability labels currently synced with arena CallArgEscape and
// Ownership. Bump it only with joint cross-repo signoff per fence #1425 when a
// label is added, removed, renamed, or changes boundary meaning.
//
// D2: Export, Opaque, and Freeze changed from claimed-operational to reserved
// because the placement switch has no decision reader for those labels.
const EscapeVocabVersion = 2
