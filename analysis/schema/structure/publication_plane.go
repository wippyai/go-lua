package structure

import "github.com/wippyai/go-lua/analysis/schema"

// The publication plane vocabularies. A detached query answer is a table of
// rows, and the class a written row is published at is a member of a closed
// catalog exactly as a published column's value is. The wire byte a payload
// carries is the seal's rank of one of these members, so the catalog is
// declared here rather than beside each family's codec: a family that spells
// its own class list is a second declaration of a vocabulary this surface
// already owns, and two families spelling the same class twice is the
// duplication this table exists to remove.
//
// The keys below are the identities a payload's ranks are folded against, so
// they are the names the wire is pinned to and are deliberately unqualified.

// PublicationClassHeld is the class of a row a producer wrote. A family whose
// rows carry presence alone publishes every written row at this one class; an
// unwritten row is not a class at all but the absence of one, so it never
// occupies a rank here.
const PublicationClassHeld schema.Key = "held"

var publicationRowClasses = [...]nativePublicationMember{
	{PublicationClassHeld, "held"},
}

// PublicationPlaneSpecs returns the canonical structural declarations of the
// publication plane vocabularies. The returned slice is detached so callers
// cannot mutate the inventory owned by this package.
func PublicationPlaneSpecs() []Spec {
	specs := make([]Spec, 0, len(publicationRowClasses))
	for index, member := range publicationRowClasses {
		specs = append(specs, Spec{
			Key:      member.key,
			Category: CategoryPublicationRowClass,
			Ordinal:  uint16(index + 1),
			Spelling: member.spelling,
			Accepted: true,
		})
	}
	return specs
}
