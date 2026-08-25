package structure

import "github.com/wippyai/go-lua/analysis/schema"

// The publication descriptor vocabularies: what one authored effect occurrence
// does, and what it declares about escape, mutability and lifetime.
//
// These are Target's semantic authority, and they are declared here so a
// published column can rank them. The ordinals below are each enum's own
// numbering in analysis/program/target/vocabulary, so a projection maps a
// descriptor to a member by its value and no translation table stands between
// the authored disposition and the byte a consumer reads. The Invalid zero of
// each enum is absence, not a member, so it is absent from these tables.
//
// The keys are qualified because they are declaration identities in one shared
// surface; the spellings are not, because they are read inside one category.

// The authored publication effect kinds.
const (
	PublicationKindSendTransfer   schema.Key = "publication/kind/send-transfer"
	PublicationKindReturnEscape   schema.Key = "publication/kind/return-escape"
	PublicationKindCallbackEscape schema.Key = "publication/kind/callback-escape"
	PublicationKindFreezeSeal     schema.Key = "publication/kind/freeze-seal"
	PublicationKindWriteMutation  schema.Key = "publication/kind/write-mutation"
	PublicationKindCloseRelease   schema.Key = "publication/kind/close-release"
)

// The authored escape dispositions.
const (
	PublicationEscapeKeyNone         schema.Key = "publication/escape/none"
	PublicationEscapeKeySendTransfer schema.Key = "publication/escape/send-transfer"
	PublicationEscapeKeyReturn       schema.Key = "publication/escape/return"
	PublicationEscapeKeyCallback     schema.Key = "publication/escape/callback"
)

// The authored mutability dispositions.
const (
	PublicationMutabilityKeyPreserve    schema.Key = "publication/mutability/preserve"
	PublicationMutabilityKeySeal        schema.Key = "publication/mutability/seal"
	PublicationMutabilityKeyWrite       schema.Key = "publication/mutability/write"
	PublicationMutabilityKeyCopyOnWrite schema.Key = "publication/mutability/copy-on-write"
)

// The authored lifetime dispositions.
const (
	PublicationLifetimeKeyPreserve schema.Key = "publication/lifetime/preserve"
	PublicationLifetimeKeyRelease  schema.Key = "publication/lifetime/release"
)

var publicationEffectKinds = [...]nativePublicationMember{
	{PublicationKindSendTransfer, "send-transfer"},
	{PublicationKindReturnEscape, "return-escape"},
	{PublicationKindCallbackEscape, "callback-escape"},
	{PublicationKindFreezeSeal, "freeze-seal"},
	{PublicationKindWriteMutation, "write-mutation"},
	{PublicationKindCloseRelease, "close-release"},
}

var publicationEscapes = [...]nativePublicationMember{
	{PublicationEscapeKeyNone, "none"},
	{PublicationEscapeKeySendTransfer, "send-transfer"},
	{PublicationEscapeKeyReturn, "return"},
	{PublicationEscapeKeyCallback, "callback"},
}

var publicationMutabilities = [...]nativePublicationMember{
	{PublicationMutabilityKeyPreserve, "preserve"},
	{PublicationMutabilityKeySeal, "seal"},
	{PublicationMutabilityKeyWrite, "write"},
	{PublicationMutabilityKeyCopyOnWrite, "copy-on-write"},
}

var publicationLifetimes = [...]nativePublicationMember{
	{PublicationLifetimeKeyPreserve, "preserve"},
	{PublicationLifetimeKeyRelease, "release"},
}

// PublicationEffectSpecs returns the canonical structural declarations of the
// four publication descriptor vocabularies. The returned slice is detached so
// callers cannot mutate the inventory owned by this package.
func PublicationEffectSpecs() []Spec {
	total := len(publicationEffectKinds) + len(publicationEscapes) + len(publicationMutabilities) + len(publicationLifetimes)
	specs := make([]Spec, 0, total)
	appendCategory := func(category Category, members []nativePublicationMember) {
		for index, member := range members {
			specs = append(specs, Spec{
				Key:      member.key,
				Category: category,
				Ordinal:  uint16(index + 1),
				Spelling: member.spelling,
				Accepted: true,
			})
		}
	}
	appendCategory(CategoryPublicationEffectKind, publicationEffectKinds[:])
	appendCategory(CategoryPublicationEscape, publicationEscapes[:])
	appendCategory(CategoryPublicationMutability, publicationMutabilities[:])
	appendCategory(CategoryPublicationLifetime, publicationLifetimes[:])
	return specs
}
