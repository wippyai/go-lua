package structure

import "github.com/wippyai/go-lua/analysis/schema"

// The reduction outcome vocabulary. Every fold in the analyzer concludes with
// exactly one of these five dispositions, and this is the one place they are
// declared. A fold's value result carries the fact it computed; the outcome
// carries what the fold concluded, so a fact lattice never has to spell an
// absence, a refusal, or an opaque admission as one of its own elements.
//
// The five are distinct answers, not degrees of one answer:
//
//   - Refuse is the fold declining to answer: its evidence was not
//     authenticated, so it publishes nothing.
//   - NoSelection is an explicitly empty selection: the fold's selected read
//     resolved to no row of a population that exists.
//   - NoCandidate is an absent subject: the row the fold would have folded is
//     not in the population at all.
//   - Concrete is a proved fact.
//   - AuthenticatedOpaque is a proved admission of unknowing: the evidence is
//     owner-authenticated and says the fact is not determinable here, which is
//     a different claim from refusing to look.
//
// The keys are the identities a sealed rank is folded against.
const (
	ReductionOutcomeRefuseKey              schema.Key = "reduction-outcome/refuse"
	ReductionOutcomeNoSelectionKey         schema.Key = "reduction-outcome/no-selection"
	ReductionOutcomeNoCandidateKey         schema.Key = "reduction-outcome/no-candidate"
	ReductionOutcomeConcreteKey            schema.Key = "reduction-outcome/concrete"
	ReductionOutcomeAuthenticatedOpaqueKey schema.Key = "reduction-outcome/authenticated-opaque"
)

// ReductionOutcome is the disposition of one fold. It is the Go spelling of
// CategoryReductionOutcome and the only outcome vocabulary in the analyzer:
// the engine's execution substrate, the activation relation, and the
// Delta/Snapshot path all carry this type rather than a private enum of their
// own.
//
// The zero value is Refuse. A caller cannot manufacture a value-bearing
// disposition by copying a semantic payload into an outcome, and a fold cannot
// encode a disposition in its value result, because the value result and this
// result are separate.
type ReductionOutcome uint8

const (
	Refuse ReductionOutcome = iota
	NoSelection
	NoCandidate
	Concrete
	AuthenticatedOpaque
)

// Available reports whether outcome is one of the five declared dispositions.
func (outcome ReductionOutcome) Available() bool { return outcome <= AuthenticatedOpaque }

// Ordinal is the outcome's declared dense position inside its category. The
// category is numbered from one, so the enum's zero value is ordinal one.
func (outcome ReductionOutcome) Ordinal() uint16 { return uint16(outcome) + 1 }

// Key is the outcome's declared identity. An outcome outside the vocabulary
// resolves to the empty key rather than to a neighbouring member.
func (outcome ReductionOutcome) Key() schema.Key {
	if !outcome.Available() {
		return ""
	}
	return reductionOutcomes[outcome].key
}

var reductionOutcomes = [...]nativePublicationMember{
	{ReductionOutcomeRefuseKey, "refuse"},
	{ReductionOutcomeNoSelectionKey, "no_selection"},
	{ReductionOutcomeNoCandidateKey, "no_candidate"},
	{ReductionOutcomeConcreteKey, "concrete"},
	{ReductionOutcomeAuthenticatedOpaqueKey, "authenticated_opaque"},
}

// ReductionOutcomeSpecs returns the canonical structural declarations of the
// reduction outcome vocabulary. The returned slice is detached so callers
// cannot mutate the inventory owned by this package.
func ReductionOutcomeSpecs() []Spec {
	specs := make([]Spec, 0, len(reductionOutcomes))
	for index, outcome := range reductionOutcomes {
		specs = append(specs, Spec{
			Key:      outcome.key,
			Category: CategoryReductionOutcome,
			Ordinal:  uint16(index + 1),
			Spelling: outcome.spelling,
			Accepted: true,
		})
	}
	return specs
}
