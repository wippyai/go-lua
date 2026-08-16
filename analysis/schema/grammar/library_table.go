package grammar

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The declared contract kinds. A kind's authored key is its one spelling in the
// analyzer and is the key its declaration row is identified by. A key names the
// contract algebra a kind is published under and never the world an instance
// carries: which library or which environment is mounted is instance data,
// external and configurable per link, so no flavor of it is spellable here.
const (
	// LibraryContractKind is the kind one library is published as: its own
	// exported values, addressed from the contract root, and nothing outside
	// them.
	LibraryContractKind schema.Key = "library"
	// EnvironmentContractKind is the kind an initial environment is published
	// as. A link mounts one initial environment, so there is one row.
	EnvironmentContractKind schema.Key = "environment"
)

// contractCodecVersion is the version both declared kinds publish their
// instances under. A codec is versioned so a reader has ground to distinguish
// a contract it can decode from one it cannot; these two formats are at their
// first revision.
const contractCodecVersion uint32 = 1

// contractPayloadFormats is the authored payload format role of every declared
// member form. A form's payload format is one format wherever it is declared,
// so the environment kind's base members carry exactly the identities the
// library kind's do: the environment specializes the member-form algebra rather
// than restating it in formats of its own.
//
// The catalog itself is the library surface's, read here through
// library.Class.Required(); this table names the format each form is serialized
// in and nothing else. A form the surface declares and this table does not name
// leaves its member without a payload identity, which the surface's own
// member-form law rejects.
var contractPayloadFormats = map[library.Form]string{
	library.FormCallableSignature:  "callable-signature",
	library.FormIntrinsicMarker:    "intrinsic-marker",
	library.FormEffectLabel:        "effect-label",
	library.FormMetatableEdge:      "metatable-edge",
	library.FormExportValue:        "export-value",
	library.FormResultProvenance:   "result-provenance",
	library.FormResultRefinement:   "result-refinement",
	library.FormSuspension:         "suspension",
	library.FormRuleDelegation:     "rule-delegation",
	library.FormBootRoot:           "boot-root",
	library.FormDeniedEntry:        "denied-entry",
	library.FormEnvironmentSlot:    "environment-slot",
	library.FormPrimitiveMetatable: "primitive-metatable",
	library.FormHostCapability:     "host-capability",
}

// contractIdentity derives one declared identity in the analyzer's global
// semantic domain, so a contract identity is replayable across processes and is
// not minted from local state. An underivable role yields the empty identity,
// which every law that reads it rejects.
func contractIdentity(role string) identity.ContentID {
	key, ok := vocabulary.Key(role)
	if !ok {
		return identity.ContentID{}
	}
	return identity.ContentID(key.Digest())
}

// contractMembers declares one member per form the class owes, in the surface's
// own required order. Completeness is not asserted here: the member-form law
// states it over the sealed row, so a form added to the algebra and left
// unnamed rejects the table rather than passing as a kind that cannot carry it.
func contractMembers(class library.Class) []library.Member {
	forms := class.Required()
	members := make([]library.Member, 0, len(forms))
	for _, form := range forms {
		format, named := contractPayloadFormats[form]
		if !named {
			members = append(members, library.Member{Form: form})
			continue
		}
		members = append(members, library.Member{Form: form, Payload: contractIdentity("contract-payload/" + format)})
	}
	return members
}

// librarySpecs is the authored analyzer contract kind inventory: the kind a
// library is published as, and the kind an initial environment is published as.
// The rows declare the algebra and no world: a mounted library's name, its
// members and its export graph are instance data a link carries, so neither row
// can be read as a claim about which library or which environment exists.
//
// Both kinds address their members by the path of exported values from the
// contract root. That is the whole point of the surface: the contract rides the
// exported value, so an alias of it keeps the contract and a rebound slot does
// not acquire one. The dotted-global-name addressing the analyzer uses today is
// declarable and refused, so the defect is a stated verdict rather than an
// unspellable shape.
//
// Each kind's validation reference is deferred. The law set a kind's instances
// are checked under is owned by a surface that has not landed, so the reference
// carries a form-valid identity and nothing that looks resolved; form-validating
// an identity is not resolving it, and the declaration says which one it did.
func librarySpecs() []library.Spec {
	return []library.Spec{
		{
			Key:        LibraryContractKind,
			Class:      library.ClassLibrary,
			Codec:      library.Codec{Format: contractIdentity("contract-codec/library"), Version: contractCodecVersion},
			Validation: library.LawSet{Resolution: library.ResolutionDeferred, Deferred: contractIdentity("contract-lawset/library")},
			Addressing: library.AddressingExportPath,
			Members:    contractMembers(library.ClassLibrary),
		},
		{
			Key:        EnvironmentContractKind,
			Class:      library.ClassEnvironment,
			Codec:      library.Codec{Format: contractIdentity("contract-codec/environment"), Version: contractCodecVersion},
			Validation: library.LawSet{Resolution: library.ResolutionDeferred, Deferred: contractIdentity("contract-lawset/environment")},
			Addressing: library.AddressingExportPath,
			Members:    contractMembers(library.ClassEnvironment),
		},
	}
}

// libraryKinds admits the authored inventory. A rejected row leaves the table
// unavailable rather than half declared.
func libraryKinds() ([]*library.Entry, bool) {
	specs := librarySpecs()
	entries := make([]*library.Entry, 0, len(specs))
	for _, spec := range specs {
		entry, ok := library.New(spec)
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	return entries, true
}
