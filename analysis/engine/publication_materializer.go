// publication_materializer.go owns the publication driver: the family-erased
// pass that folds a query family's declared inputs over the subjects one epoch
// reconsiders and writes the answers into that family's result column.
//
// The driver is erased at the family boundary and typed everywhere inside it.
// One epoch publishes several families whose keys, inputs, accumulators and
// result contracts are all different types, so the ordered set the epoch walks
// cannot be typed; what a family folds, freezes and writes must be, or the
// checked recovery a column read performs would be the first place a type
// error is noticed. The erasure is therefore exactly one interface wide, and
// nothing below it asserts.
//
// Nothing here decides what a column means. Reachability is read from the
// column the engine's demand pass publishes, the fold and the result contract
// are the owning domain's, and the key universe an answer's absence is proven
// against is the coverage authority's. The driver folds, freezes and writes.

package engine

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Publication errors. They name the law that refused, so an aborted epoch
// reports which obligation was unmet rather than that publication "failed".
var (
	// ErrPublisherUnavailable reports a driver or a publisher that was never
	// completely declared.
	ErrPublisherUnavailable = errors.New("engine: publisher is not completely declared")
	// ErrGateUnreadable reports that the reachability column a family's
	// subjects are admitted by did not answer. A family whose gate cannot be
	// read has no admitted subjects, which is not the same as having none.
	ErrGateUnreadable = errors.New("engine: reachability column does not answer")
	// ErrInputUnreadable reports that a declared input column did not answer.
	// A family reads its declared inputs and nothing else, so an input that
	// answers nothing leaves the fold with no observation to run over.
	ErrInputUnreadable = errors.New("engine: declared input column does not answer")
	// ErrFoldRejected reports that a family's fold refused the observation its
	// declared inputs produced. The fold is the domain's own contract, so a
	// refusal is a violated contract and not an absent answer.
	ErrFoldRejected = errors.New("engine: fold rejected its observation")
	// ErrResultContract reports an answer that does not survive its own result
	// contract: a frozen answer its clone is not equal to, or fingerprints
	// differently from, is not the value the domain declared an answer to be.
	ErrResultContract = errors.New("engine: answer does not survive its result contract")
	// ErrPublishedSlot reports a family already published at a slot other than
	// the one its write capability was minted for. One column has one writer,
	// so a family cannot answer into two.
	ErrPublishedSlot = errors.New("engine: family is published at another slot")
)

// Denominator is the key universe a published column is total over: the
// identity its membership is sealed under, and the members themselves.
//
// It is produced by the coverage authority the sealed table carries and is
// consumed here unchanged. A publisher never derives one: whether a column can
// prove an absence follows from its axis's declared cardinality, which is
// declared once and projected once, so an unavailable Denominator publishes a
// column that reports a miss and an available one publishes a column whose
// silence inside the universe is a fact.
type Denominator[K comparable] struct {
	ID      identity.ContentID
	Members []K
}

// Available reports whether this denominator states a key universe.
func (denominator Denominator[K]) Available() bool { return denominator.ID.Available() }

// erasedPublisher is the family boundary. One epoch holds an ordered set of
// these and knows nothing else about the families in it; each implementation
// carries its own keys, inputs, accumulator, result contract and column.
//
// The interface is unexported, so the set of families a publication can hold is
// the set the engine constructs. A domain declares a family through the query
// surface and is answered through the contributor it declared there; it does
// not hand the driver a publication pass of its own.
type erasedPublisher interface {
	// publishDelta folds every subject this family was asked to reconsider and
	// writes the result into builder. It reads base causally: reachability and
	// the declared inputs are read from the publication the delta derives from,
	// so what a fold observes is a published fact and never a half-written one.
	publishDelta(base *snapshot.Snapshot, builder *snapshot.Builder) error
}

// queryFoldSource is what a family's sealed implementation answers about how
// its answers are produced. Both query lanes satisfy it, and nothing outside
// the engine can: a publisher folds through a contributor's sealed receipt
// rather than through a callback a caller supplied beside it.
type queryFoldSource[V, R any] interface {
	accumulator() (func() R, func(R, OrderedCells[V]) (R, bool), bool)
	frozenResult() (FrozenResult[R], bool)
}

// QueryPublisherSpec declares one family's publisher. K is the subject key, G
// the reachability column's fact, V one declared input's value and O the
// family's answer.
type QueryPublisherSpec[K comparable, G, V, O any] struct {
	// Family is the identity this family's answers are published and opened
	// under. It is the sealed table's, and a consumer opens the family the
	// declaration names.
	Family identity.ContentID
	// Write is the capability the engine minted for this family's result
	// column. It is the only way the answers reach storage.
	Write ColumnWrite[K, O]
	// Reach is the execution-reachability column this family's subjects are
	// admitted by. A subject the engine did not reach is never folded.
	Reach snapshot.Axis[K, G]
	// Inputs are the columns this family's fold observes, in the order its
	// ordered observation presents them. They are the family's declared
	// inputs: the fold sees these and nothing else, and each contributes one
	// cell per subject, present when the column holds a row for it.
	Inputs []snapshot.Axis[K, V]
	// Denominator is the key universe the result column is total over, as the
	// coverage authority states it. Left unavailable, the column proves no
	// absence.
	Denominator Denominator[K]
	// Fold is the family's sealed implementation: the accumulator its answers
	// are produced by and the contract they are frozen under.
	Fold queryFoldSource[V, O]
}

// QueryPublisher is one family's typed publisher. It is epoch-local by
// construction and is not safe for concurrent use: it reuses one observation
// record across the subjects of a delta, so a fold's ordered cells are valid
// only while that fold runs.
type QueryPublisher[K comparable, G, V, O any] struct {
	family      identity.ContentID
	write       ColumnWrite[K, O]
	reach       snapshot.Axis[K, G]
	inputs      []snapshot.Axis[K, V]
	denominator Denominator[K]
	begin       func() O
	accumulate  func(O, OrderedCells[V]) (O, bool)
	result      FrozenResult[O]
	// dirty is the subjects this family reconsiders in the next delta.
	dirty []K
	// observation is the ordered cells one fold runs over. It holds one cell
	// per declared input and is refilled per subject, so folding a delta
	// allocates no observation at all.
	observation *orderedCellsRecord[V]
}

// NewQueryPublisher instantiates one family's publisher from its sealed
// implementation. A family with no declared input, no write capability, or an
// implementation whose fold or result contract is not recoverable is refused
// rather than answered by some default.
func NewQueryPublisher[K comparable, G, V, O any](spec QueryPublisherSpec[K, G, V, O]) (*QueryPublisher[K, G, V, O], bool) {
	if !spec.Family.Available() || !spec.Write.Available() || !spec.Reach.Available() || len(spec.Inputs) == 0 || spec.Fold == nil {
		return nil, false
	}
	for _, input := range spec.Inputs {
		if !input.Available() {
			return nil, false
		}
	}
	begin, accumulate, foldOK := spec.Fold.accumulator()
	result, resultOK := spec.Fold.frozenResult()
	if !foldOK || !resultOK || begin == nil || accumulate == nil || !validFrozenResult(result) {
		return nil, false
	}
	if spec.Denominator.Available() && len(spec.Denominator.Members) == 0 {
		return nil, false
	}
	observation := &orderedCellsRecord[V]{cells: make([]summaryCell[V], len(spec.Inputs))}
	observation.live.Store(true)
	return &QueryPublisher[K, G, V, O]{
		family:      spec.Family,
		write:       spec.Write,
		reach:       spec.Reach,
		inputs:      append([]snapshot.Axis[K, V](nil), spec.Inputs...),
		denominator: spec.Denominator,
		begin:       begin,
		accumulate:  accumulate,
		result:      result,
		observation: observation,
	}, true
}

// Reconsider names the subjects the next delta folds. It is the change set,
// and the delta's cost is that set rather than the column: a subject left out
// of it keeps the answer the publication it derives from holds.
func (publisher *QueryPublisher[K, G, V, O]) Reconsider(keys []K) {
	if publisher == nil {
		return
	}
	publisher.dirty = append(publisher.dirty[:0], keys...)
}

// publishDelta folds this family's reconsidered subjects and writes them.
//
// A family whose result column the base publication does not hold is declared
// here, whole, together with the key universe its absences are proven against.
// A family it does hold is edited row by row, which is what makes a generation
// cost its change set.
func (publisher *QueryPublisher[K, G, V, O]) publishDelta(base *snapshot.Snapshot, builder *snapshot.Builder) error {
	if publisher == nil || builder == nil || publisher.begin == nil || publisher.accumulate == nil {
		return ErrPublisherUnavailable
	}
	column, unlocked := publisher.write.column()
	if !unlocked {
		return ErrUnauthorizedColumnWrite
	}
	plan, published := snapshot.OpenQuery[K, O](base, publisher.family)
	if !published {
		return publisher.declare(base, builder)
	}
	if plan.Axis() != column {
		return ErrPublishedSlot
	}
	for _, key := range publisher.dirty {
		answer, emitted, err := publisher.fold(base, key)
		if err != nil {
			return err
		}
		if emitted {
			if err := PublishRow(publisher.write, builder, key, answer); err != nil {
				return err
			}
			continue
		}
		if err := WithdrawRow(publisher.write, builder, key); err != nil {
			return err
		}
	}
	return nil
}

// declare seals this family's result column for the first time. The answers
// are the folds of the reconsidered subjects, and the column is sealed with the
// key universe its coverage authority states, so a subject inside that universe
// without an answer is absent as a published fact from the first generation on.
func (publisher *QueryPublisher[K, G, V, O]) declare(base *snapshot.Snapshot, builder *snapshot.Builder) error {
	rows := make(map[K]O, len(publisher.dirty))
	for _, key := range publisher.dirty {
		answer, emitted, err := publisher.fold(base, key)
		if err != nil {
			return err
		}
		if emitted {
			rows[key] = answer
		}
	}
	content := snapshot.Content[K, O]{Rows: rows}
	if publisher.denominator.Available() {
		content.Denominator = publisher.denominator.ID
		content.Members = publisher.denominator.Members
	}
	_, err := PublishQueryColumn(publisher.write, builder, publisher.family, content)
	return err
}

// fold produces one subject's answer and whether the family emits it.
//
// The emission law is the contributor's own Begin and Accumulate pair, because
// that pair is the whole of what the query surface declares about a fold; there
// is no Finish hook to consult and none is invented here. What the pair says is
// how observations compose, so a subject with an observation is folded and
// emitted, and a subject with none is not: a subject outside the reached set is
// never folded, and a reached subject whose declared inputs hold no row was
// derived nothing, so publishing Begin's empty accumulator for it would state a
// fact the solver never produced. Both silences are absences, and the column's
// key universe is what makes them provable.
func (publisher *QueryPublisher[K, G, V, O]) fold(base *snapshot.Snapshot, key K) (O, bool, error) {
	var zero O
	if _, status := snapshot.Read(base, publisher.reach, key); status != snapshot.ReadHit {
		if !status.Outcome() {
			return zero, false, ErrGateUnreadable
		}
		return zero, false, nil
	}
	observed := false
	for index, input := range publisher.inputs {
		value, status := snapshot.Read(base, input, key)
		if !status.Outcome() {
			return zero, false, ErrInputUnreadable
		}
		held := status == snapshot.ReadHit
		publisher.observation.cells[index] = summaryCell[V]{value: value, present: held}
		observed = observed || held
	}
	if !observed {
		return zero, false, nil
	}
	answer, accepted := publisher.accumulate(publisher.begin(), OrderedCells[V]{record: publisher.observation})
	if !accepted {
		return zero, false, ErrFoldRejected
	}
	frozen := publisher.result.Freeze(answer)
	clone := publisher.result.Clone(frozen)
	if !publisher.result.Equal(frozen, clone) || publisher.result.Fingerprint(frozen) != publisher.result.Fingerprint(clone) {
		return zero, false, ErrResultContract
	}
	return frozen, true, nil
}

// Publication is one epoch's publication driver: the ordered set of family
// publishers a generation is written by.
type Publication struct{ publishers []erasedPublisher }

// NewPublication opens an empty driver.
func NewPublication() *Publication { return &Publication{} }

// AddQueryPublisher admits one family's publisher to the driver. The order
// families are added in is the order their columns are written in.
func AddQueryPublisher[K comparable, G, V, O any](publication *Publication, publisher *QueryPublisher[K, G, V, O]) bool {
	if publication == nil || publisher == nil || !publisher.write.Available() {
		return false
	}
	publication.publishers = append(publication.publishers, publisher)
	return true
}

// PublishDelta writes the generation that follows base. Every family folds its
// reconsidered subjects and writes its column, and the whole set is sealed
// together.
//
// The epoch is all or nothing. A refused fold, an unreadable input or an
// unauthorized write abandons the delta before it seals, so no column is left
// holding some of a generation's answers: the publication base holds is
// untouched, because a delta shares base's structure and never writes into it.
func (publication *Publication) PublishDelta(base snapshot.Snapshot, generation identity.Generation) (snapshot.Snapshot, error) {
	if publication == nil || len(publication.publishers) == 0 {
		return snapshot.Snapshot{}, ErrPublisherUnavailable
	}
	builder := snapshot.NewDelta(base, generation)
	for _, publisher := range publication.publishers {
		if err := publisher.publishDelta(&base, &builder); err != nil {
			return snapshot.Snapshot{}, err
		}
	}
	return builder.Seal()
}
