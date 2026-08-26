// Package expand evaluates one sealed dependent C-to-R expansion.
//
// Mount owns the physical Evidence: the publisher resolves its ordered
// semantic identities once and issues the reader-key ValueTokens under the
// runtime fence. This package only redeems that immutable evidence. It does
// not consult an owner, derive an ordinal, construct a RowID, scan the reader,
// or invent a missing/default key.
package expand
