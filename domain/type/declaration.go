// Package typedomain is the root of the analyzer's Lua type domain and the
// domain's declaration statement against the analyzer declaration table.
//
// The domain's subject matter lives in the packages below this one: the type
// graph and its canonical codec, the kind vocabulary the graph dispatches on,
// subtyping, substitution, normalization, table and record construction, member
// access and call resolution, the projection step language, the adapter to the
// schema's neutral type envelope, and the artifact-row authority with its packed
// runtime. None of it is in this file. What is here is the one place the domain
// states what it declares, and the law beside it is stated over every source
// beneath this directory rather than over this file's own imports.
//
// # What the domain owns
//
// Two closed vocabularies, and everything else is a graph algebra over them.
//
//   - The kind vocabulary. Twenty-eight members naming the structural category
//     of a type, returned by every node in the graph, with a name table beside
//     the members. Its ordinals are not a numbering the domain is free with:
//     nine primitive nodes hash as their kind, and the recursive, literal,
//     generic, and formal hashes mix it, so the ordinals are hash seeds in every
//     content identity the analyzer computes over a type. The vocabulary is
//     zero-based, has five holes left by removed members, and carries one member
//     no node returns.
//   - The projection step vocabulary. Four members naming the ways one type is
//     reached from another - a field, a callable's return, a generic argument,
//     an instantiation - dense from one, with a limit sentinel, a membership
//     predicate, and a catalog function that is the one enumeration of them.
//
// # Declaration
//
// The domain declares two rows: assignment and direct-call argument conformance.
// whose value does not conform to the type its target declares. It is declared
// at the foot of this file, from this package alone, and every other surface of
// the analyzer declaration table carries no row of this domain's. The reason
// differs per surface, and where the reason is a missing decision rather than a
// missing subject the decision is named:
//
//   - Axis. An axis is a coordinate space the solver writes during a fixpoint
//     and a Link instantiates a carrier for. The domain writes no coordinate.
//     It does hold the one shape that would become one: the type authority seals
//     the artifact's static type rows into a Link-scoped directory, which is
//     exactly what an axis mount is - it consumes the neutral mounted artifact
//     view, produces a Link authority, and refuses with evidence of its own. It
//     is reached today by direct call from the Program plan, before the mount
//     transaction opens, so it is a caller-sealed input rather than a mounted
//     factor. Declaring an axis for it is the domain's one open axis question,
//     and it is a larger change than a row: the mount phase would have to carry
//     the authority in its input record and recover it at this domain's type,
//     and the domain would come under the composition registry that seals the
//     table. Until the type authority is mounted rather than called, an axis row
//     would declare a carrier nothing writes.
//   - Rule. A rule declares an engine slot at an artifact rule role and attaches
//     at a mount point. Nothing here is bound at a mount or evaluated at a point;
//     every judgment the domain makes is a query over an immutable graph.
//   - Diagnostic. A diagnostic row publishes a code from facts the analyzer
//     already produces, and this domain publishes one: an assignment whose
//     value may carry a runtime family the type its target declares does not
//     admit. The judgment behind it is the conformance package's, which is set
//     containment over the closed runtime vocabulary and nothing else, and the
//     row names every identity it rests on by reference - the publication family
//     the query boundary gates it by, the observation population it is measured
//     over, and the coordinate space whose facts decide it - so the row is data
//     and the declarations it names stay owned where they are declared.
//     The domain's other refusals - an unresolvable member, a non-callable
//     receiver, an interface mismatch, a runtime input that would not decode -
//     are answered to the caller as a status beside the result, and the caller
//     decides what to publish. They publish no code.
//   - Composite. A composite is a relation over declared coordinate spaces, and
//     every axis it names must resolve. The domain names none.
//   - Denominator. A denominator names the surface entry whose universe it
//     quantifies over. The domain owns no entry. The closed worlds it does
//     carry, the sealed selector directory and the finite subtype relation the
//     runtime materializes, are closed by construction inside one sealed
//     authority.
//   - Query. A query family reads declared coordinate spaces and publishes a
//     result codec. The domain reads no coordinate space; its readers hold its
//     graph and its handles directly.
//   - Structure. The structural vocabulary hosts the closed catalogs the analyzer
//     would otherwise spell once per consumer. This domain declares one role:
//     the channel-select case fact family, spelled by channelselect and sealed
//     here. The kind and projection-step vocabularies remain proposals below.
//   - Library. A library contract kind addresses exported values under a member
//     form algebra. The schema's type-contract package is not this surface and is
//     not a surface at all: it is the neutral portable envelope one authored type
//     declaration travels in, plus the semantics interface a domain satisfies to
//     interpret it. This domain's adapter satisfies that interface and encodes
//     and decodes the envelope, which is supplying an implementation value at
//     composition, not declaring a row.
//
// # Proposal: the projection step vocabulary
//
// This one is ready and mechanical. The vocabulary is already dense from one,
// already has a catalog function that supplies its membership without a second
// list, and already has a downstream boundary spelling - the signature wire
// codec's step variants - that is indexed by the catalog and held to it by its
// own law. What the declaration would add is the third party the two are held
// against, and the spelling the owner does not have: the step's rendered name
// today comes from a display switch with a default arm rather than from a
// declared member.
//
// The wiring is four parts. A category on the structural vocabulary for the
// projection step. A StructureSpecs in the owning package built by ranging the
// catalog, taking each member's ordinal from the member itself, in the shape the
// expression-form declaration already uses. One contribution entry in the
// composition that hosts the structural vocabulary. A law stating the sealed
// catalog is the closed step vocabulary member for member, and that a member of
// another category does not answer as a step.
//
// It is left as a proposal here because the composition tables are serialized
// and the row set is not needed by anything today: the boundary spelling that
// would read it is already pinned by a law of its own. The declaration is worth
// landing when a second consumer needs the spelling, and it costs one category
// and one contribution line when it is.
//
// # Proposal: the kind vocabulary, and what blocks it
//
// This one is not wiring. The structural vocabulary numbers a category dense
// from one, and the kind vocabulary is zero-based with five holes; a member's
// kind therefore cannot be its surface ordinal, the way a runtime family's kind
// is. The holes cannot be closed either: the ordinals are hash seeds, so
// renumbering them changes every content identity the analyzer has computed over
// a type.
//
// A declaration must therefore carry a surface ordinal distinct from the kind,
// with an explicit projection in both directions, and it must decide what to do
// about the member no node returns and about the name table's collision - the
// renderer answers "unknown" both for the member spelled unknown and for every
// ordinal it does not cover. Those are decisions about the vocabulary, not about
// the table, and the vocabulary's owner makes them first. Declaring rows over
// the vocabulary as it stands would seal the holes and the collision into the
// analyzer's one catalog rather than resolve them.
//
// # Second spellings
//
// The kind vocabulary is the most restated catalog the analyzer has, and none of
// the restatements is held to it by a law:
//
//   - The canonical codec's wire tags are the graph's serialization commitment
//     and its own vocabulary: it splits the nil member in two, carries no tag for
//     the member no node returns, and is read back by a decode switch that
//     restates the whole list a second time. A member added to the graph and not
//     to the codec is a value that fails to encode at write time rather than a
//     rejected build.
//   - The public type facade renders kinds from a switch covering seventeen of
//     the members and defaulting the rest to the name of a real member.
//   - The signature wire codec spells the kinds as JSON strings, differing from
//     the owner's spelling in one member's capitalization and adding three names
//     that are not kinds at all.
//   - The name table sits beside the const block in the owning package with no
//     completeness statement outside its tests.
//   - The artifact's static node kinds and the Program-side primitive kinds are
//     two more ordinal vocabularies for the same subject matter, crossed by hand
//     switches in the artifact authority, one of which round-trips through the
//     primitive name strings to cross an ordinal boundary.
//   - The adapter's admission and freshness kinds mirror the schema's own member
//     for member, translated by hand switches whose default arms make a member
//     added on one side a runtime error rather than a rejected build.
//
// None of these is a compensation to remove in place: each is a boundary that
// legitimately owns its own spelling. What they are missing is the declaration to
// be pinned against, which is what the proposals above would give the first two
// of them.
//
// # Position
//
// The domain sits at the base of the domain layer. It imports no peer domain,
// and the peer domains that reason about types - the static inventory, the pack
// classification, the heap key algebra - import it. The one domain package it
// reads is the closed runtime family vocabulary, which is scalar vocabulary and
// set algebra with no dependency on any domain at all and therefore sits below
// this one rather than beside it; the law beside this file states that position
// rather than assuming it. The law states both directions, so a declaration
// added here is added by a domain that is still below the composition that
// would seal it.
package typedomain

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	"github.com/wippyai/go-lua/domain/type/conformance"
)

// The identities this domain's row is declared against. The code is the
// domain's own; the other three are declared elsewhere and named here by their
// authored key, so the row resolves them at seal rather than restating what
// they mean.
const (
	// Code is the finding this domain publishes: an assignment whose value may
	// carry a runtime family the declared type of its target does not admit.
	Code diagnostic.Code = "type.assignment"
	// CallArgumentCode is the same judgment at a direct-call actual.
	CallArgumentCode diagnostic.Code = "type.call.direct.argument_type"
	// FamilyKey is the publication family the query boundary gates the code by.
	// It is declared beside the diagnostic inventory, and its declared spelling
	// is the first segment of the code.
	FamilyKey schema.Key = "family/type"
	// ObservationKey is the population the row is measured over: the assignments
	// whose value is compared against the type their target declares. It is a
	// member of the canonical observation vocabulary, whose ordinals artifacts
	// carry, so it is declared there rather than here.
	ObservationKey schema.Key = "observation/type-conformance"
	// ConformanceCollectionKey is the observation family that supplies the
	// measured value: the value summary read at the rule occurrence that
	// produces the conformance subject's value. A point-keyed query column is
	// published only at selected points and never at that occurrence, so the
	// row names the observation family the branch pair also reads.
	ConformanceCollectionKey schema.Key = "value-summary/type-conformance"
	// FactKey is the coordinate space whose facts decide the row. The families a
	// value may carry are the value axis's own judgment, and the row reads them
	// rather than producing them.
	FactKey schema.Key = "value"
	// ChannelSelectExhaustivenessCode is the if-chain coverage judgment over
	// published channel-select case facts.
	ChannelSelectExhaustivenessCode diagnostic.Code = "channel.select.exhaustiveness"
	// ChannelSelectFamilyKey is the publication family that code is gated by.
	ChannelSelectFamilyKey schema.Key = "family/channel"
	// ChannelSelectObservationKey is the branch-condition population the
	// exhaustiveness row shares with the polarity pair.
	ChannelSelectObservationKey schema.Key = "observation/branch-condition"
	// ChannelSelectCollectionKey is the observation family that supplies those
	// branch subjects.
	ChannelSelectCollectionKey schema.Key = "value-summary/branch-condition"
	// ChannelSelectFactKey is the coordinate space of accepted select arms.
	ChannelSelectFactKey schema.Key = "channel-select-case"
)

// diagnosticRender is the section order the row publishes. The summary names
// the finding, the location and source place it, the evidence states what was
// proven, and the help states the two ways out.
var diagnosticRender = []diagnostic.Section{
	diagnostic.SectionSummary,
	diagnostic.SectionLocation,
	diagnostic.SectionSource,
	diagnostic.SectionEvidence,
	diagnostic.SectionHelp,
}

// ConformanceVerdictStructureSpecs is this domain's second structure row set:
// the assignment conformance verdict vocabulary. Composition hosts the
// catalog; the ordinals and spellings are owned by the conformance package,
// which is the judgment that produces them.
func ConformanceVerdictStructureSpecs() []structure.Spec {
	catalog := conformance.Catalog()
	specs := make([]structure.Spec, 0, len(catalog))
	for _, verdict := range catalog {
		specs = append(specs, structure.Spec{
			Key:      schema.Key(conformance.VerdictKey(verdict)),
			Category: structure.CategoryConformanceVerdict,
			Ordinal:  verdict.Ordinal(),
			Spelling: verdict.Spelling(),
			Accepted: true,
		})
	}
	return specs
}

// ConformanceVerdictFor projects one declared row back to the answer it
// declares. A consumer reading the sealed vocabulary recovers the verdict
// through this rather than converting an ordinal of its own.
func ConformanceVerdictFor(entry *structure.Entry) (conformance.Verdict, bool) {
	if entry == nil || entry.Category() != structure.CategoryConformanceVerdict {
		return conformance.VerdictInvalid, false
	}
	verdict := conformance.Verdict(entry.Ordinal())
	return verdict, verdict.Available()
}

// ChannelSelectStructureSpecs is this domain's structure row: the
// channel-select case fact family. Composition hosts the catalog; the
// spelling lives in channelselect.
func ChannelSelectStructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(channelselect.Role)
}

// DiagnosticSpec is this domain's declared row. It is pure data: the code it
// publishes under, the family it is gated by, the severity it defaults to, the
// lane its subjects arrive on, the population it is measured over, the
// declaration whose facts decide it, the payload a producer owes it, and the
// presentation it renders from.
//
// The lane is the solver-observed one. The verdict is decided from the families
// a value may carry, which the value axis proves during the solve, so the row
// names that axis as the declaration its facts come from and is collected after
// the fixpoint rather than issued by the compiler.
//
// The payload is the subject the assignment names and the declared type it is
// measured against. Both are read by the presentation and neither is read
// anywhere else, which is the whole of what the surface's requirement law
// holds the row to.
func DiagnosticSpec() diagnostic.Spec {
	return diagnostic.Spec{
		Code:            Code,
		Family:          diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: FamilyKey},
		DefaultSeverity: diagnostic.SeverityError,
		Lane:            diagnostic.LaneBranch,
		Observation:     diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: ObservationKey},
		Collection:      diagnostic.Reference{Surface: schema.SurfaceKindObservation, Key: ConformanceCollectionKey},
		Sites:           []diagnostic.Site{diagnostic.SiteAssignment, diagnostic.SiteMember, diagnostic.SiteMemberAbsent},
		Fact:            diagnostic.Reference{Surface: schema.SurfaceKindAxis, Key: FactKey},
		VerdictCategory: structure.CategoryConformanceVerdict,
		Variants: []diagnostic.Variant{
			{
				Verdict:      conformance.VerdictViolates.Ordinal(),
				Requirements: diagnostic.RequiresSubject | diagnostic.RequiresTarget | diagnostic.RequiresActual,
				Message:      "cannot assign {subject} because it is {actual}, not {target}",
				Help:         "Use a value compatible with the expected type, or change the target type if `{subject}` is valid.",
				Evidence: []diagnostic.Evidence{
					{Anchor: diagnostic.AnchorPrimary, Kind: "abstract fact", Trust: "proven", Reason: "unspecified", Detail: "{subject} is {actual}"},
					{Anchor: diagnostic.AnchorPrimary, Kind: "user assertion", Trust: "claimed", Reason: "unspecified", Detail: "{subject} is declared as {target}"},
					{Anchor: diagnostic.AnchorPrimary, Kind: "missing proof", Trust: "refuted", Reason: "unspecified", Detail: "no proof on this path shows {subject} satisfies {target}"},
				},
				Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "assigned value"}},
			},
			{
				Verdict:      conformance.VerdictMayBeNil.Ordinal(),
				Requirements: diagnostic.RequiresSubject | diagnostic.RequiresTarget,
				Message:      "cannot assign {subject} because it may be nil",
				Help:         "Narrow {subject} to a non-nil value before assigning it, or declare the target as optional.",
				Evidence: []diagnostic.Evidence{
					{Anchor: diagnostic.AnchorPrimary, Kind: "abstract fact", Trust: "proven", Reason: "unspecified", Detail: "{subject} may be nil on this path"},
					{Anchor: diagnostic.AnchorPrimary, Kind: "user assertion", Trust: "claimed", Reason: "unspecified", Detail: "{subject} is declared as {target}, which does not admit nil"},
				},
				Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "assigned value"}},
			},
			{
				Verdict:      conformance.VerdictMemberAbsent.Ordinal(),
				Requirements: diagnostic.RequiresMember | diagnostic.RequiresTarget,
				Message:      "object literal is missing required field \"{member}\"",
				Help:         "Supply {member}, or declare the field optional.",
				Evidence: []diagnostic.Evidence{
					{Anchor: diagnostic.AnchorPrimary, Kind: "missing proof", Trust: "refuted", Reason: "unspecified", Detail: "the object literal establishes no field \"{member}\""},
					{Anchor: diagnostic.AnchorPrimary, Kind: "user assertion", Trust: "claimed", Reason: "unspecified", Detail: "{member} is declared as {target} and is required"},
				},
				Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "object literal"}},
			},
			{
				Verdict:      conformance.VerdictUnproven.Ordinal(),
				Requirements: diagnostic.RequiresSubject | diagnostic.RequiresTarget,
				Message:      "cannot assign {subject} because it comes from any/unknown; no proof shows it satisfies the declared type",
				Help:         "Narrow {subject} with a checked test before assigning it to {target}.",
				Evidence: []diagnostic.Evidence{
					{Anchor: diagnostic.AnchorPrimary, Kind: "unvalidated value", Trust: "unknown", Reason: "unspecified", Detail: "{subject} comes from any or unknown"},
					{Anchor: diagnostic.AnchorPrimary, Kind: "missing proof", Trust: "refuted", Reason: "unspecified", Detail: "no proof on this path shows {subject} satisfies {target}"},
				},
				Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "assigned value"}},
			},
		},
		Render: diagnosticRender,
	}
}

// DiagnosticCallArgumentSpec is the call-argument row. It shares the
// type-conformance observation population and the value-axis fact with the
// assignment row; the site discriminator on the observation chooses the code.
func DiagnosticCallArgumentSpec() diagnostic.Spec {
	return diagnostic.Spec{
		Code:            CallArgumentCode,
		Family:          diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: FamilyKey},
		DefaultSeverity: diagnostic.SeverityError,
		Lane:            diagnostic.LaneBranch,
		Observation:     diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: ObservationKey},
		Collection:      diagnostic.Reference{Surface: schema.SurfaceKindObservation, Key: ConformanceCollectionKey},
		Sites:           []diagnostic.Site{diagnostic.SiteCallArgument},
		Fact:            diagnostic.Reference{Surface: schema.SurfaceKindAxis, Key: FactKey},
		VerdictCategory: structure.CategoryConformanceVerdict,
		Variants: []diagnostic.Variant{
			{
				Verdict:      conformance.VerdictViolates.Ordinal(),
				Requirements: diagnostic.RequiresArgument | diagnostic.RequiresSubject | diagnostic.RequiresParameter | diagnostic.RequiresTarget | diagnostic.RequiresActual | diagnostic.RequiresObserved,
				Message:      "{argument} is {actual}, not {target}",
				Help:         "Pass `{subject}` as a value compatible with the parameter type, or change the callee signature if that argument is valid.",
				Evidence: []diagnostic.Evidence{
					{Anchor: diagnostic.AnchorPrimary, Kind: "abstract fact", Trust: "proven", Reason: "unspecified", Detail: "{argument} has {observed}"},
					{Anchor: diagnostic.AnchorPrimary, Kind: "user assertion", Trust: "claimed", Reason: "unspecified", Detail: "{parameter} expects {target}"},
					{Anchor: diagnostic.AnchorPrimary, Kind: "missing proof", Trust: "refuted", Reason: "unspecified", Detail: "no proof on this path shows {subject} satisfies the parameter type"},
				},
				Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "argument value"}},
			},
			{
				Verdict:      conformance.VerdictMayBeNil.Ordinal(),
				Requirements: diagnostic.RequiresSubject | diagnostic.RequiresTarget,
				Message:      "{subject} may be nil, and the parameter type {target} does not admit nil",
				Help:         "Narrow {subject} to a non-nil value before passing it, or declare the parameter as optional.",
				Evidence: []diagnostic.Evidence{
					{Anchor: diagnostic.AnchorPrimary, Kind: "abstract fact", Trust: "proven", Reason: "unspecified", Detail: "{subject} may be nil on this path"},
					{Anchor: diagnostic.AnchorPrimary, Kind: "user assertion", Trust: "claimed", Reason: "unspecified", Detail: "the parameter expects {target}, which does not admit nil"},
				},
				Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "argument value"}},
			},
			{
				Verdict:      conformance.VerdictMemberAbsent.Ordinal(),
				Requirements: diagnostic.RequiresMember | diagnostic.RequiresTarget,
				Message:      "object literal argument is missing required field \"{member}\"",
				Help:         "Supply {member}, or declare the field optional.",
				Evidence: []diagnostic.Evidence{
					{Anchor: diagnostic.AnchorPrimary, Kind: "missing proof", Trust: "refuted", Reason: "unspecified", Detail: "the object literal establishes no field \"{member}\""},
					{Anchor: diagnostic.AnchorPrimary, Kind: "user assertion", Trust: "claimed", Reason: "unspecified", Detail: "{member} is declared as {target} and is required"},
				},
				Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "argument value"}},
			},
			{
				Verdict:      conformance.VerdictUnproven.Ordinal(),
				Requirements: diagnostic.RequiresSubject | diagnostic.RequiresTarget,
				Message:      "{subject} comes from any/unknown; no proof shows it satisfies the parameter type",
				Help:         "Narrow {subject} with a checked test before passing it as {target}.",
				Evidence: []diagnostic.Evidence{
					{Anchor: diagnostic.AnchorPrimary, Kind: "unvalidated value", Trust: "unknown", Reason: "unspecified", Detail: "{subject} comes from any or unknown"},
					{Anchor: diagnostic.AnchorPrimary, Kind: "missing proof", Trust: "refuted", Reason: "unspecified", Detail: "no proof on this path shows {subject} satisfies {target}"},
				},
				Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "argument value"}},
			},
		},
		Render: diagnosticRender,
	}
}

// DiagnosticChannelSelectExhaustivenessSpec is the if-chain coverage row.
// Facts are the published channel-select case column; the Program if-chain
// names the handled ordinals. A lookalike table member is never a fact.
func DiagnosticChannelSelectExhaustivenessSpec() diagnostic.Spec {
	return diagnostic.Spec{
		Code:            ChannelSelectExhaustivenessCode,
		Family:          diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: ChannelSelectFamilyKey},
		DefaultSeverity: diagnostic.SeverityWarning,
		Lane:            diagnostic.LaneBranch,
		Observation:     diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: ChannelSelectObservationKey},
		Collection:      diagnostic.Reference{Surface: schema.SurfaceKindObservation, Key: ChannelSelectCollectionKey},
		Fact:            diagnostic.Reference{Surface: schema.SurfaceKindAxis, Key: ChannelSelectFactKey},
		Requirements:    diagnostic.RequiresSubject | diagnostic.RequiresHandled | diagnostic.RequiresMissing,
		Message:         "channel select is not exhaustive; missing case: {missing}",
		Help:            "Add an elseif branch for each missing case",
		Evidence: []diagnostic.Evidence{
			{
				Anchor: diagnostic.AnchorPrimary,
				Kind:   "abstract fact",
				Trust:  "proven",
				Reason: "unspecified",
				Detail: "branch chain checks channel `{subject}.channel`",
			},
			{
				Anchor: diagnostic.AnchorPrimary,
				Kind:   "abstract fact",
				Trust:  "proven",
				Reason: "unspecified",
				Detail: "handled cases: {handled}",
			},
			{
				Anchor: diagnostic.AnchorPrimary,
				Kind:   "missing proof",
				Trust:  "unknown",
				Reason: "unspecified",
				Detail: "missing cases: {missing}",
			},
			{
				Anchor: diagnostic.AnchorPrimary,
				Kind:   "missing proof",
				Trust:  "unknown",
				Reason: "unspecified",
				Detail: "no default case handles the remaining channel cases",
			},
		},
		Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "channel case check"}},
		Render: diagnosticRender,
	}
}
