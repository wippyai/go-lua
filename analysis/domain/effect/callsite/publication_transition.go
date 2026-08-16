package callsite

import (
	"bytes"
	"crypto/sha256"

	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programquery"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

const publicationObservationDomain = "wippy.analysis.effect.publication-observation.v1\x00"

// PublicationTransitionCandidates is the callsite-owned, solve-independent
// candidate set for one exact selected CallEffect occurrence. It contains no
// raw atom ID, equation point, Program, or Target pointer.
type PublicationTransitionCandidates struct{ set *publicationTransitionSet }

type publicationTransitionSet struct {
	rule        *HotRule
	mount       keyspace.ContentID
	occurrence  keyspace.ContentID
	stage       engine.MountedNativeCallStageReceipt
	observation engine.ReceiptObservation[programquery.EffectObservation]
	rows        []publicationTransitionRow
	sealed      bool
}

type publicationTransitionRow struct {
	id          keyspace.ContentID
	publication effectfactor.PublicationAtomBinding
}

// PublicationTransitionCandidate is one schema-authored potential
// publication. It becomes a proof only by reading its exact, completed
// CallEffect observation through Prove.
type PublicationTransitionCandidate struct {
	set   *publicationTransitionSet
	index uint32
}

// PublicationTransitionProofFailure is the closed reason why a candidate
// cannot become a post-convergence proof. It exposes no observation payload,
// atom identity, or certificate; the Effect owner alone interprets those.
type PublicationTransitionProofFailure uint8

const (
	PublicationTransitionProofFailureNone PublicationTransitionProofFailure = iota
	PublicationTransitionProofFailureInvalidCandidate
	PublicationTransitionProofFailureInvalidSolverState
	PublicationTransitionProofFailureUnreadableObservation
	PublicationTransitionProofFailureObservationInvalid
	PublicationTransitionProofFailureObservationRows
	PublicationTransitionProofFailureObservationAbsent
	PublicationTransitionProofFailureObservationTop
	PublicationTransitionProofFailureAtomNotProven
)

// PublicationTransitionProof is one post-convergence proof. It retains the
// completed Solver/State only to revalidate them through Engine on every
// projection; it carries no raw effect atom or query result.
type PublicationTransitionProof struct {
	candidate PublicationTransitionCandidate
	solver    *engine.Solver
	state     *engine.State
}

// AttachMountedPublicationCandidates joins a selected HotRule's sealed
// occurrence receipt, its graph-owned CallEffect stage, and each explicitly
// authored PublicationAtomBinding. One exact Effect observation is attached
// for the whole occurrence, so multiple candidates never multiply solver
// demand. Opaque and generic effect routes issue no candidate.
func (rule *HotRule) AttachMountedPublicationCandidates(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, effectQuery *engine.ExactQueryImplementation[effectfactor.Value, programquery.EffectObservation], mount, occurrence keyspace.ContentID) (PublicationTransitionCandidates, bool) {
	if rule == nil || rule.opaque || compilation == nil || graph == nil || effectQuery == nil || !mount.Available() || !occurrence.Available() {
		return PublicationTransitionCandidates{}, false
	}
	issuer, issuerOK := rule.ForMount(mount)
	operand, operandOK := issuer.ReceiptForOccurrence(occurrence)
	stage, stageOK := rule.MountedSelectedCallEffectStage(graph, mount, occurrence)
	member, memberOK := stage.RuleMember()
	if !issuerOK || !operandOK || !stageOK || !stage.Available() || stage.Stage() != engine.ArtifactRuleStageCallEffect || stage.MountID() != mount || stage.OccurrenceID() != occurrence || !memberOK {
		return PublicationTransitionCandidates{}, false
	}
	rows, rowsOK := rule.publicationTransitionRows(operand, mount, occurrence)
	if !rowsOK {
		return PublicationTransitionCandidates{}, false
	}
	if len(rows) == 0 {
		return PublicationTransitionCandidates{set: &publicationTransitionSet{rule: rule, mount: mount, occurrence: occurrence, stage: stage, sealed: true}}, true
	}
	observationID, idOK := publicationTransitionID(publicationObservationDomain, mount, occurrence)
	observation, attached := engine.AttachRuleExactObservation(compilation, effectQuery, observationID, member)
	if !idOK || !attached || !observation.Available() {
		return PublicationTransitionCandidates{}, false
	}
	set := &publicationTransitionSet{rule: rule, mount: mount, occurrence: occurrence, stage: stage, observation: observation, rows: rows, sealed: true}
	return PublicationTransitionCandidates{set: set}, set.valid()
}

func (rule *HotRule) publicationTransitionRows(operand hotOperand, mount, occurrence keyspace.ContentID) ([]publicationTransitionRow, bool) {
	if rule == nil || rule.opaque || !rule.accepts(operand) || operand.receipt == nil || operand.receipt.owner != rule || rule.effects == nil || rule.effects.Algebra() == nil {
		return nil, false
	}
	effects := rule.effects.Algebra()
	rows := make([]publicationTransitionRow, 0)
	seen := make(map[keyspace.ContentID]struct{})
	for _, target := range operand.receipt.targets {
		if !target.applicable || !target.valid {
			continue
		}
		for _, publication := range target.publications {
			mounted, mountedOK := publication.MountedCall()
			_, publicationMount, publicationOccurrence, identityOK := effects.MountedCallIdentity(mounted)
			publicationOccurrenceID, occurrenceOK := publication.OccurrenceID()
			if !publication.Valid() || !mountedOK || !identityOK || publicationMount != mount || publicationOccurrence != occurrence || !occurrenceOK || !publicationOccurrenceID.Available() {
				return nil, false
			}
			if _, duplicate := seen[publicationOccurrenceID]; duplicate {
				return nil, false
			}
			candidateID, idOK := publicationTransitionID("wippy.analysis.effect.publication-candidate.v1\x00", mount, occurrence, publicationOccurrenceID)
			if !idOK {
				return nil, false
			}
			seen[publicationOccurrenceID] = struct{}{}
			rows = append(rows, publicationTransitionRow{id: candidateID, publication: publication})
		}
	}
	sortPublicationTransitionRows(rows)
	return rows, true
}

func sortPublicationTransitionRows(rows []publicationTransitionRow) {
	for left := 1; left < len(rows); left++ {
		for right := left; right > 0 && bytes.Compare(rows[right].id[:], rows[right-1].id[:]) < 0; right-- {
			rows[right], rows[right-1] = rows[right-1], rows[right]
		}
	}
}

func publicationTransitionID(domain string, ids ...keyspace.ContentID) (keyspace.ContentID, bool) {
	if domain == "" {
		return keyspace.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	for _, id := range ids {
		if !id.Available() {
			return keyspace.ContentID{}, false
		}
		_, _ = hash.Write(id[:])
	}
	var result keyspace.ContentID
	copy(result[:], hash.Sum(nil))
	return result, result.Available()
}

func (set *publicationTransitionSet) valid() bool {
	if set == nil || !set.sealed || set.rule == nil || set.rule.opaque || set.rule.binding == nil || !set.rule.binding.Sealed() || !set.mount.Available() || !set.occurrence.Available() || !set.stage.Available() || set.stage.Stage() != engine.ArtifactRuleStageCallEffect || set.stage.MountID() != set.mount || set.stage.OccurrenceID() != set.occurrence {
		return false
	}
	if len(set.rows) == 0 {
		return !set.observation.Available()
	}
	if !set.observation.Available() || set.rule.effects == nil || set.rule.effects.Algebra() == nil {
		return false
	}
	if _, memberOK := set.stage.RuleMember(); !memberOK {
		return false
	}
	previous := keyspace.ContentID{}
	seen := make(map[keyspace.ContentID]struct{}, len(set.rows))
	for index := range set.rows {
		row := set.rows[index]
		publicationOccurrence, occurrenceOK := row.publication.OccurrenceID()
		expectedID, idOK := publicationTransitionID("wippy.analysis.effect.publication-candidate.v1\x00", set.mount, set.occurrence, publicationOccurrence)
		if !row.id.Available() || !occurrenceOK || !idOK || row.id != expectedID || index > 0 && bytes.Compare(previous[:], row.id[:]) >= 0 || !publicationTransitionMatches(set, row.publication) {
			return false
		}
		if _, duplicate := seen[publicationOccurrence]; duplicate {
			return false
		}
		seen[publicationOccurrence] = struct{}{}
		previous = row.id
	}
	return true
}

func publicationTransitionMatches(set *publicationTransitionSet, publication effectfactor.PublicationAtomBinding) bool {
	if set == nil || set.rule == nil || set.rule.effects == nil || set.rule.effects.Algebra() == nil || !publication.Valid() {
		return false
	}
	mounted, mountedOK := publication.MountedCall()
	_, mount, occurrence, identityOK := set.rule.effects.Algebra().MountedCallIdentity(mounted)
	return mountedOK && identityOK && mount == set.mount && occurrence == set.occurrence
}

func (candidates PublicationTransitionCandidates) Available() bool {
	return candidates.set != nil && candidates.set.valid()
}

func (candidates PublicationTransitionCandidates) Count() int {
	if !candidates.Available() {
		return 0
	}
	return len(candidates.set.rows)
}

func (candidates PublicationTransitionCandidates) At(index int) (PublicationTransitionCandidate, bool) {
	if !candidates.Available() || index < 0 || index >= len(candidates.set.rows) || uint64(index) > uint64(^uint32(0)) {
		return PublicationTransitionCandidate{}, false
	}
	candidate := PublicationTransitionCandidate{set: candidates.set, index: uint32(index)}
	return candidate, candidate.Available()
}

func (candidate PublicationTransitionCandidate) row() (publicationTransitionRow, bool) {
	if candidate.set == nil || !candidate.set.valid() || uint64(candidate.index) >= uint64(len(candidate.set.rows)) {
		return publicationTransitionRow{}, false
	}
	row := candidate.set.rows[candidate.index]
	return row, row.id.Available() && publicationTransitionMatches(candidate.set, row.publication)
}

func (candidate PublicationTransitionCandidate) Available() bool {
	_, ok := candidate.row()
	return ok
}

func (candidate PublicationTransitionCandidate) ContentID() (keyspace.ContentID, bool) {
	row, ok := candidate.row()
	return row.id, ok
}

// ProveWithFailure reads this candidate's exact, completed Effect
// observation. It returns only a closed failure class; detached observation
// rows, atom IDs, and private membership certificates never leave Callsite.
func (candidate PublicationTransitionCandidate) ProveWithFailure(solver *engine.Solver, state *engine.State) (PublicationTransitionProof, PublicationTransitionProofFailure) {
	proof := PublicationTransitionProof{candidate: candidate, solver: solver, state: state}
	return proof, candidate.proofFailure(solver, state)
}

// Prove is the boolean convenience form of ProveWithFailure.
func (candidate PublicationTransitionCandidate) Prove(solver *engine.Solver, state *engine.State) (PublicationTransitionProof, bool) {
	proof, failure := candidate.ProveWithFailure(solver, state)
	return proof, failure == PublicationTransitionProofFailureNone
}

func (candidate PublicationTransitionCandidate) proofFailure(solver *engine.Solver, state *engine.State) PublicationTransitionProofFailure {
	row, rowOK := candidate.row()
	if !rowOK {
		return PublicationTransitionProofFailureInvalidCandidate
	}
	if solver == nil || state == nil {
		return PublicationTransitionProofFailureInvalidSolverState
	}
	observation, readable := engine.ReceiptObservationResult(candidate.set.observation, solver, state)
	if !readable {
		return PublicationTransitionProofFailureUnreadableObservation
	}
	if !observation.Valid {
		return PublicationTransitionProofFailureObservationInvalid
	}
	if observation.Rows != 1 {
		return PublicationTransitionProofFailureObservationRows
	}
	if !observation.Present {
		return PublicationTransitionProofFailureObservationAbsent
	}
	if observation.Top {
		return PublicationTransitionProofFailureObservationTop
	}
	binding, bindingOK := row.publication.AtomBinding()
	if !bindingOK || !observation.ProvesAtomBinding(binding) {
		return PublicationTransitionProofFailureAtomNotProven
	}
	return PublicationTransitionProofFailureNone
}

func (proof PublicationTransitionProof) valid() bool {
	return proof.candidate.proofFailure(proof.solver, proof.state) == PublicationTransitionProofFailureNone
}

// Valid rechecks the exact completed solver/state pair; a proof cannot outlive
// an activation revision or be replayed through another solver.
func (proof PublicationTransitionProof) Valid() bool { return proof.valid() }

// MatchesCompletion proves that solver and state are the exact completed
// engine pair retained by this authenticated transition proof. Consumers that
// combine a transition with another post-convergence observation must use this
// fence rather than accepting the proof's detached ContentID alone.
func (proof PublicationTransitionProof) MatchesCompletion(solver *engine.Solver, state *engine.State) bool {
	return solver != nil && state != nil && solver == proof.solver && state == proof.state && proof.valid()
}

func (proof PublicationTransitionProof) row() (publicationTransitionRow, bool) {
	row, ok := proof.candidate.row()
	return row, ok && proof.valid()
}

func (proof PublicationTransitionProof) ContentID() (keyspace.ContentID, bool) {
	row, ok := proof.row()
	return row.id, ok
}

func (proof PublicationTransitionProof) DescriptorID() (keyspace.ContentID, bool) {
	row, ok := proof.row()
	if !ok {
		return keyspace.ContentID{}, false
	}
	return row.publication.DescriptorID()
}

func (proof PublicationTransitionProof) OccurrenceID() (keyspace.ContentID, bool) {
	row, ok := proof.row()
	if !ok {
		return keyspace.ContentID{}, false
	}
	return row.publication.OccurrenceID()
}

func (proof PublicationTransitionProof) MountID() keyspace.ContentID {
	if !proof.Valid() || proof.candidate.set == nil {
		return keyspace.ContentID{}
	}
	return proof.candidate.set.mount
}

func (proof PublicationTransitionProof) CallOccurrenceID() keyspace.ContentID {
	if !proof.Valid() || proof.candidate.set == nil {
		return keyspace.ContentID{}
	}
	return proof.candidate.set.occurrence
}

func (proof PublicationTransitionProof) Role() effectfactor.PublicationAtomBindingRole {
	row, ok := proof.row()
	if !ok {
		return effectfactor.PublicationAtomBindingInvalid
	}
	return row.publication.Role()
}

func (proof PublicationTransitionProof) Kind() target.PublicationEffectKind {
	row, ok := proof.row()
	if !ok {
		return target.PublicationEffectInvalid
	}
	return row.publication.Kind()
}

func (proof PublicationTransitionProof) Escape() target.PublicationEscapeDisposition {
	row, ok := proof.row()
	if !ok {
		return target.PublicationEscapeInvalid
	}
	return row.publication.Escape()
}

func (proof PublicationTransitionProof) Mutability() target.PublicationMutabilityDisposition {
	row, ok := proof.row()
	if !ok {
		return target.PublicationMutabilityInvalid
	}
	return row.publication.Mutability()
}

func (proof PublicationTransitionProof) Lifetime() target.PublicationLifetimeDisposition {
	row, ok := proof.row()
	if !ok {
		return target.PublicationLifetimeInvalid
	}
	return row.publication.Lifetime()
}

func (proof PublicationTransitionProof) SubjectSelector() (pack.InputSelector, bool) {
	row, ok := proof.row()
	if !ok {
		return pack.InputSelector{}, false
	}
	return row.publication.SubjectSelector()
}

func (proof PublicationTransitionProof) ContextSelector() (pack.InputSelector, bool) {
	row, ok := proof.row()
	if !ok {
		return pack.InputSelector{}, false
	}
	return row.publication.ContextSelector()
}
