package body

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
)

// ErrConflictingSummaryInput rejects two different observations for the same
// exact summary key. Publishing either one would make lineage order-dependent.
var (
	ErrConflictingSummaryInput          = errors.New("body: conflicting summary input records")
	ErrConflictingSummaryInputProviders = errors.New("body: both typed and anonymous summary input providers configured")
)

// ResultVersionDigest is the full-width identity of the exact semantic inputs
// consumed by one body solve. It is produced by the same byte stream as the
// legacy 64-bit ResultVersion; it is authority, while ResultVersion remains a
// compact compatibility and indexing value.
type ResultVersionDigest [sha256.Size]byte

func (d ResultVersionDigest) String() string { return hex.EncodeToString(d[:]) }

// SummaryInputKey is the body-layer spelling of an interprocedural summary
// key. RefKind and RefID identify the function namespace; the remaining fields
// identify its exact abstract entry dimensions. The body package deliberately
// owns this neutral record so its digest encoder does not depend on a fixpoint
// implementation package.
type SummaryInputKey struct {
	RefKind    uint8
	RefID      uint64
	Values     uint64
	Facts      uint64
	References uint64
}

func (k SummaryInputKey) less(other SummaryInputKey) bool {
	if k.RefKind != other.RefKind {
		return k.RefKind < other.RefKind
	}
	if k.RefID != other.RefID {
		return k.RefID < other.RefID
	}
	if k.Values != other.Values {
		return k.Values < other.Values
	}
	if k.Facts != other.Facts {
		return k.Facts < other.Facts
	}
	return k.References < other.References
}

// SummaryInput records one exact dynamic summary read. Missing reads are part
// of lineage because a later appearance can change the solve. PayloadDigest is
// the normalized semantic payload digest and must be zero when Present is
// false.
type SummaryInput struct {
	Key           SummaryInputKey
	Present       bool
	PayloadDigest uint64
}

// ResultVersionLineage is an immutable publication record for a solved body.
// Its full digest covers the static body/environment, schedule, lanes,
// widening decisions, entry and initial states, closed dynamic invariants, and
// the canonical SummaryInputs vector. Accessors return values or clones so a
// published Result cannot be mutated through lineage.
type ResultVersionLineage struct {
	legacy   uint64
	digest   ResultVersionDigest
	inputs   []SummaryInput
	complete bool
}

// ResultVersion returns the compact compatibility digest.
func (l ResultVersionLineage) ResultVersion() uint64 { return l.legacy }

// Digest returns the authoritative full-width semantic-input identity. It is
// zero when Complete is false so partial or anonymous lineage cannot be used as
// cache or publication authority.
func (l ResultVersionLineage) Digest() ResultVersionDigest {
	if !l.complete {
		return ResultVersionDigest{}
	}
	return l.digest
}

// Complete reports whether every semantic input was captured through
// collision-resistant canonical authority. It remains false until the summary,
// product, and state layers expose that authority; widening their uint64 hashes
// into SHA-256 would not make the lineage collision-safe.
func (l ResultVersionLineage) Complete() bool { return l.complete }

// SummaryInputs returns a defensive copy of the canonical dependency vector.
func (l ResultVersionLineage) SummaryInputs() []SummaryInput {
	return append([]SummaryInput(nil), l.inputs...)
}

func canonicalSummaryInputs(ctx context.Context, provider func() []SummaryInput) ([]SummaryInput, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if provider == nil {
		return nil, nil
	}
	inputs := append([]SummaryInput(nil), provider()...)
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	for i := range inputs {
		if i%64 == 0 && ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if !inputs[i].Present {
			inputs[i].PayloadDigest = 0
		}
	}
	var comparisons uint64
	var canceled error
	sort.Slice(inputs, func(i, j int) bool {
		comparisons++
		if comparisons%64 == 0 && ctx != nil {
			if err := ctx.Err(); err != nil {
				canceled = err
				return false
			}
		}
		left, right := inputs[i], inputs[j]
		if left.Key != right.Key {
			return left.Key.less(right.Key)
		}
		if left.Present != right.Present {
			return !left.Present
		}
		return left.PayloadDigest < right.PayloadDigest
	})
	if canceled != nil {
		return nil, canceled
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if len(inputs) < 2 {
		return inputs, nil
	}
	out := inputs[:1]
	for i, input := range inputs[1:] {
		if i%64 == 0 && ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		last := out[len(out)-1]
		if input.Key == last.Key && (input.Present != last.Present || input.PayloadDigest != last.PayloadDigest) {
			return nil, ErrConflictingSummaryInput
		}
		if input != last {
			out = append(out, input)
		}
	}
	return out, nil
}

// ResultVersionLineage returns the immutable lineage published with this
// result. A nil result returns the zero record.
func (r *Result) ResultVersionLineage() ResultVersionLineage {
	if r == nil {
		return ResultVersionLineage{}
	}
	lineage := r.resultLineage
	lineage.inputs = append([]SummaryInput(nil), lineage.inputs...)
	return lineage
}

// ResultVersionDigest returns the full-width semantic-input identity.
func (r *Result) ResultVersionDigest() ResultVersionDigest {
	return r.ResultVersionLineage().Digest()
}
