// Package resultpolicy seals Program's exact-scalar column into the
// occurrence-local arithmetic result policy Value consumes at seal. It owns no
// lattice arithmetic: the package only distinguishes an occurrence whose
// operand images Program proved finite - and whose result image Program
// therefore pre-enumerated - from one whose image Program left open, and it
// retains the exact roster of the former.
//
// The roster is occurrence-scoped on purpose. An exact result atom sealed for
// one occurrence is a fact about that occurrence's operands; a second
// occurrence that computes the same literal has proved nothing, and reading
// the first one's atom would be a global atom table wearing a per-row name.
package resultpolicy

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// Policy is one binary-arithmetic occurrence's immutable result contract.
// A closed occurrence admits exactly the results in its roster, which may be
// empty when every pair of its finite Cartesian product traps. An open
// occurrence has no roster at all: Program did not prove its operand images
// finite, so no exact result of it is a proof.
type Policy struct {
	available bool
	closed    bool
	results   []keyspace.LiteralValue
}

// OpenImage states the policy of an occurrence whose operand images Program
// did not prove finite. It carries no roster: no exact result of such an
// occurrence is a proof about the occurrence.
func OpenImage() Policy { return Policy{available: true} }

// ClosedImage states the policy of an occurrence whose finite result image
// Program pre-enumerated. The roster is copied into canonical order, so a
// sealed policy cannot be mutated through the slice it was built from, and an
// empty roster is a real answer: every pair of the product traps.
func ClosedImage(results ...keyspace.LiteralValue) Policy {
	roster := append([]keyspace.LiteralValue(nil), results...)
	sort.Slice(roster, func(left, right int) bool { return literalLess(roster[left], roster[right]) })
	return Policy{available: true, closed: true, results: roster}
}

// Available reports whether the occurrence was sealed by Seal. A zero Policy
// is not an open policy: it is an unsealed one, and its consumer refuses.
func (policy Policy) Available() bool { return policy.available }

// Closed reports whether Program proved a complete finite operand product.
func (policy Policy) Closed() bool { return policy.available && policy.closed }

// Admits reports whether one exact result belongs to the sealed closed image.
// The roster is held in canonical literal order, so membership is a search
// rather than a scan of a product that is quadratic in the operand widths.
func (policy Policy) Admits(literal keyspace.LiteralValue) bool {
	if !policy.Closed() {
		return false
	}
	index := sort.Search(len(policy.results), func(position int) bool {
		return !literalLess(policy.results[position], literal)
	})
	return index < len(policy.results) && policy.results[index] == literal
}

// Directory is the sealed one-policy-per-arithmetic-occurrence index.
type Directory struct {
	byOccurrence map[identity.ContentID]Policy
}

func (directory Directory) For(occurrence identity.ContentID) (Policy, bool) {
	if !occurrence.Available() || directory.byOccurrence == nil {
		return Policy{}, false
	}
	policy, ok := directory.byOccurrence[occurrence]
	return policy, ok && policy.Available()
}

type exactRoles struct {
	left, right bool
	results     map[keyspace.LiteralValue]struct{}
}

// Seal states one policy for every binary-arithmetic occurrence Program
// published, then folds Program's exact-scalar column onto them. Program emits
// an exact row per retained literal per role, and emits none for a role whose
// image it could not close, so the presence of both operand roles is the
// closure proof and the result rows are the image.
//
// Program's arithmetic summary column is a different statement - the numeric
// representation lattice - and it is published only for occurrences whose
// operands and result all carry a known representation. It is therefore absent
// exactly where an occurrence reads something Program treats as opaque, which
// is one of the shapes an open policy exists for; requiring it here would
// refuse the program instead of describing it.
func Seal(program programschema.Program) (Directory, bool) {
	if !program.Available() {
		return Directory{}, false
	}
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	exactCount, exactPublished := program.ExactScalarSummaryCount()
	if !occurrencesPublished || !exactPublished {
		return Directory{}, false
	}
	occurrences := make([]identity.ContentID, 0, occurrenceCount)
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK {
			return Directory{}, false
		}
		if row.Kind() != programschema.OccurrenceBinaryArithmetic {
			continue
		}
		occurrences = append(occurrences, row.ID())
	}
	exact := make([]programschema.ExactScalarSummary, exactCount)
	for index := 0; index < exactCount; index++ {
		row, rowOK := program.ExactScalarSummaryAt(index)
		if !rowOK {
			return Directory{}, false
		}
		exact[index] = row
	}
	return sealRows(occurrences, exact)
}

func sealRows(occurrences []identity.ContentID, exact []programschema.ExactScalarSummary) (Directory, bool) {
	policies := make(map[identity.ContentID]Policy, len(occurrences))
	roles := make(map[identity.ContentID]*exactRoles, len(occurrences))
	for _, occurrence := range occurrences {
		if !occurrence.Available() {
			return Directory{}, false
		}
		if _, duplicate := policies[occurrence]; duplicate {
			return Directory{}, false
		}
		policies[occurrence] = OpenImage()
		roles[occurrence] = &exactRoles{results: make(map[keyspace.LiteralValue]struct{})}
	}
	for _, row := range exact {
		if !row.Available() {
			return Directory{}, false
		}
		proof := roles[row.OccurrenceID()]
		if proof == nil {
			return Directory{}, false
		}
		switch row.Role() {
		case programschema.ExactScalarSummaryLeft:
			proof.left = true
		case programschema.ExactScalarSummaryRight:
			proof.right = true
		case programschema.ExactScalarSummaryResult:
			if row.SubjectID() != row.OccurrenceID() {
				return Directory{}, false
			}
			cold, literalOK := row.Literal()
			literal := keyspace.LiteralValue{Kind: keyspace.LiteralKind(cold.Kind), Integer: cold.Integer, FloatBits: cold.FloatBits}
			if !literalOK || (literal.Kind != keyspace.LiteralInteger && literal.Kind != keyspace.LiteralFloat) {
				return Directory{}, false
			}
			if _, duplicate := proof.results[literal]; duplicate {
				return Directory{}, false
			}
			proof.results[literal] = struct{}{}
		default:
			return Directory{}, false
		}
	}
	for occurrence := range policies {
		proof := roles[occurrence]
		if !proof.left || !proof.right {
			if len(proof.results) != 0 {
				// A result roster without both complete operands is not a
				// closed Cartesian image and cannot authorize exact execution.
				return Directory{}, false
			}
			continue
		}
		roster := make([]keyspace.LiteralValue, 0, len(proof.results))
		for literal := range proof.results {
			roster = append(roster, literal)
		}
		policies[occurrence] = ClosedImage(roster...)
	}
	return Directory{byOccurrence: policies}, true
}

func literalLess(left, right keyspace.LiteralValue) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Integer != right.Integer {
		return left.Integer < right.Integer
	}
	return left.FloatBits < right.FloatBits
}
