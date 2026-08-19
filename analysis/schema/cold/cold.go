// Package cold declares the catalog of Program facts a compiled artifact
// publishes once and every Link that mounts it shares.
//
// # Two catalogs, one declaration source
//
// A Link's runtime publication and a compiled program's cold publication are
// two stores with two dense slot ranges. They are addressed by axes, and an
// axis names the schema that sealed the column it addresses; so the two
// ranges have to be sealed under two identities, or a cold axis would be a
// structurally valid address into a runtime snapshot's slot of the same
// number and the checked recovery would be the only thing standing between a
// consumer and a column nobody published for it.
//
// The cold catalog identity is therefore derived from the runtime schema
// identity rather than being authored beside it. One declaration source
// produces both, a cold column is bound to the exact declaration catalog it
// was compiled under, and no axis of either catalog can address the other.
package cold

import "github.com/wippyai/go-lua/analysis/identity"

// catalogDomain separates this derivation from every other digest derived in
// the tree, and its version travels in the identity: a change to what the
// cold catalog contains is a new catalog, not the same catalog with different
// contents.
const catalogDomain = "analysis/cold-catalog/v1"

// CatalogID is the identity a compiled program's cold publication is sealed
// under, derived from the runtime schema identity the program was compiled
// against. An unavailable runtime schema derives no catalog.
func CatalogID(runtimeSchema identity.ContentID) (identity.ContentID, bool) {
	if !runtimeSchema.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(catalogDomain, runtimeSchema[:])
}
