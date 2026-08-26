// Package query is Placement's typed read boundary over the canonical
// relation snapshot.
//
// The runtime publishes only opaque binding.ValueTokens. This package is the
// owner-side seam that redeems the Placement Fact column with its sealed
// codec; it never reaches back to an engine root or keeps a second row store.
package query
