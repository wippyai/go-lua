package relations

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/program/semanticsource"
)

var (
	// ErrInvalidRelationDisposition rejects an unset disposition, a disposition
	// that disagrees with its generated owner/form, or a forged definition.
	ErrInvalidRelationDisposition = errors.New("relation disposition ledger: invalid relation disposition")
	// ErrDuplicateRelationDisposition rejects two owners for one source
	// relation. There is no first-wins or last-wins rule.
	ErrDuplicateRelationDisposition = errors.New("relation disposition ledger: duplicate relation disposition")
	// ErrMissingRelationDisposition rejects a generated source relation without
	// one exact disposition.
	ErrMissingRelationDisposition = errors.New("relation disposition ledger: missing relation disposition")
	// ErrInvalidDerivative rejects a declaration with an unset law, unknown
	// observed relation, noncanonical closure, or incompatible kind/replay law.
	ErrInvalidDerivative = errors.New("relation disposition ledger: invalid derivative declaration")
	// ErrDuplicateDerivative rejects two declarations with one identity.
	ErrDuplicateDerivative = errors.New("relation disposition ledger: duplicate derivative declaration")
)

// RelationDispositionKind is the storage disposition already stated by one
// generated canonical relation row. It introduces no second policy: the
// Authored, SealDerived, and VirtualPredicate forms are projected one-to-one.
type RelationDispositionKind uint8

const (
	RelationDispositionInvalid RelationDispositionKind = iota
	RelationDispositionAuthoredComponent
	RelationDispositionSealDerivedComponent
	RelationDispositionVirtualPredicate
)

// RelationDisposition binds one issued relation definition to its one
// generated owner and storage disposition. Callers cannot use this ledger to
// move a relation between owners or reinterpret a virtual relation as storage.
type RelationDisposition struct {
	Definition semanticsource.RelationDef
	Owner      Owner
	Kind       RelationDispositionKind
}

// RelationDispositionLedger is the exact relation-level Wave-B denominator.
// It deliberately does not claim that private index/cold derivative inventory
// is complete; that is a separate sealed ledger below.
type RelationDispositionLedger struct {
	rows  []RelationDisposition
	index map[semanticsource.Token]uint32
}

// GeneratedRelationDispositions projects the complete generated relation
// schema. Every schema form is handled explicitly; an unset or future form
// fails closed instead of acquiring a default disposition.
func GeneratedRelationDispositions() (RelationDispositionLedger, error) {
	schema, err := CanonicalSchema()
	if err != nil {
		return RelationDispositionLedger{}, err
	}
	rows := make([]RelationDisposition, 0, schema.Count())
	for _, row := range schema.Rows() {
		kind, ok := dispositionForForm(row.Form)
		if !ok {
			return RelationDispositionLedger{}, ErrInvalidRelationDisposition
		}
		rows = append(rows, RelationDisposition{
			Definition: row.Definition,
			Owner:      row.Owner,
			Kind:       kind,
		})
	}
	return SealRelationDispositions(schema, rows)
}

// SealRelationDispositions requires exactly one matching row for every
// canonical relation. Missing, duplicate, owner-drifted, and default rows are
// rejected before a ledger becomes observable.
func SealRelationDispositions(schema *Schema, input []RelationDisposition) (RelationDispositionLedger, error) {
	if schema == nil || schema.Count() == 0 {
		return RelationDispositionLedger{}, ErrInvalidRelationDisposition
	}
	expected := make(map[semanticsource.Token]RelationDisposition, schema.Count())
	for _, row := range schema.Rows() {
		kind, ok := dispositionForForm(row.Form)
		if !ok {
			return RelationDispositionLedger{}, ErrInvalidRelationDisposition
		}
		expected[row.Definition.Token()] = RelationDisposition{
			Definition: row.Definition,
			Owner:      row.Owner,
			Kind:       kind,
		}
	}
	if len(input) != len(expected) {
		// Preserve duplicate errors below when cardinality happens to match.
		if len(input) < len(expected) {
			return RelationDispositionLedger{}, ErrMissingRelationDisposition
		}
	}
	seen := make(map[semanticsource.Token]struct{}, len(input))
	rows := make([]RelationDisposition, 0, len(input))
	for _, row := range input {
		token := row.Definition.Token()
		want, known := expected[token]
		if !known || row != want {
			return RelationDispositionLedger{}, ErrInvalidRelationDisposition
		}
		if _, duplicate := seen[token]; duplicate {
			return RelationDispositionLedger{}, ErrDuplicateRelationDisposition
		}
		seen[token] = struct{}{}
		rows = append(rows, row)
	}
	if len(seen) != len(expected) {
		return RelationDispositionLedger{}, ErrMissingRelationDisposition
	}
	sort.Slice(rows, func(left, right int) bool {
		return lessToken(rows[left].Definition.Token(), rows[right].Definition.Token())
	})
	index := make(map[semanticsource.Token]uint32, len(rows))
	for ordinal, row := range rows {
		index[row.Definition.Token()] = uint32(ordinal + 1)
	}
	return RelationDispositionLedger{rows: rows, index: index}, nil
}

func dispositionForForm(form Form) (RelationDispositionKind, bool) {
	switch form {
	case FormAuthored:
		return RelationDispositionAuthoredComponent, true
	case FormSealDerived:
		return RelationDispositionSealDerivedComponent, true
	case FormVirtualPredicate:
		return RelationDispositionVirtualPredicate, true
	default:
		return RelationDispositionInvalid, false
	}
}

// Count reports the complete generated relation denominator.
func (ledger RelationDispositionLedger) Count() int { return len(ledger.rows) }

// At returns one canonical token-ordered relation disposition.
func (ledger RelationDispositionLedger) At(index int) (RelationDisposition, bool) {
	if index < 0 || index >= len(ledger.rows) {
		return RelationDisposition{}, false
	}
	return ledger.rows[index], true
}

// Disposition resolves one exact generated relation token.
func (ledger RelationDispositionLedger) Disposition(token semanticsource.Token) (RelationDisposition, bool) {
	ordinal := ledger.index[token]
	if ordinal == 0 || int(ordinal) > len(ledger.rows) {
		return RelationDisposition{}, false
	}
	return ledger.rows[ordinal-1], true
}

// Derivative is a generator-issued ledger-local identity. It is not a Program,
// Link, engine, persistence, or semantic relation identity.
type Derivative uint32

// DerivativeKind separates solver-hot private indexes from post-root cold
// artifact/rebind derivatives.
type DerivativeKind uint8

const (
	DerivativeInvalid DerivativeKind = iota
	DerivativePrivateIndex
	DerivativePostRootCold
)

// QueryLaw states the sole query shape justified by a derivative. It is a
// deliberately small complexity vocabulary, not language/domain semantics.
type QueryLaw uint8

const (
	QueryInvalid QueryLaw = iota
	QueryExactLookup
	QueryMembership
	QueryProjection
	QueryEnumeration
	QueryRebind
)

// AsymptoticLaw is the declared per-query upper bound.
type AsymptoticLaw uint8

const (
	AsymptoticInvalid AsymptoticLaw = iota
	AsymptoticConstant
	AsymptoticLogarithmic
	AsymptoticOutputLinear
)

// AllocationLaw states who, if anyone, owns query-time allocation.
type AllocationLaw uint8

const (
	AllocationInvalid AllocationLaw = iota
	AllocationZero
	AllocationCallerOwned
	AllocationColdOnly
)

// ReplayLaw states whether the derivative is rebuilt or persisted only under
// the exact observed relation closure.
type ReplayLaw uint8

const (
	ReplayInvalid ReplayLaw = iota
	ReplayRebuild
	ReplayPersistObservedClosure
)

// CardinalityTerm contributes Coefficient times the product of its Factors to
// a retained cardinality bound. Factors are canonical relation-token order.
// This small polynomial form can state real structural bounds without adding
// a general expression language; validation rejects the specific retained
// Application×Operation product forbidden by the Program/Link design.
type CardinalityTerm struct {
	Factors     []semanticsource.Token
	Coefficient uint32
}

// CardinalityLaw is the upper bound Constant + sum(Terms). Terms are
// canonical monomial order and duplicate-free.
type CardinalityLaw struct {
	Constant uint32
	Terms    []CardinalityTerm
}

// DerivativeDeclaration records every fact needed to justify retaining one
// private index or post-root cold derivative. Observed is the exact relation
// digest closure; it must include every relation used by Cardinality.
type DerivativeDeclaration struct {
	Derivative Derivative
	// Owner is the one Program/Target/Link child component that retains this
	// private derivative.  Observed relations may cross component boundaries;
	// that does not transfer ownership of the derivative itself.
	Owner       Owner
	Kind        DerivativeKind
	Observed    []semanticsource.Token
	Query       QueryLaw
	Cardinality CardinalityLaw
	Asymptotic  AsymptoticLaw
	Allocation  AllocationLaw
	Replay      ReplayLaw
}

// DerivativeLedger is a validated, detached declaration set. No production
// canonical instance is published until the inventory is exhaustive.
type DerivativeLedger struct {
	rows []DerivativeDeclaration
}

// SealDerivativeLedger validates declarations against the exact generated
// relation denominator. It does not discover indexes from Go syntax.
func SealDerivativeLedger(relations RelationDispositionLedger, input []DerivativeDeclaration) (DerivativeLedger, error) {
	if relations.Count() == 0 {
		return DerivativeLedger{}, ErrInvalidDerivative
	}
	rows := make([]DerivativeDeclaration, 0, len(input))
	seen := make(map[Derivative]struct{}, len(input))
	for _, declaration := range input {
		if _, duplicate := seen[declaration.Derivative]; duplicate {
			return DerivativeLedger{}, ErrDuplicateDerivative
		}
		row, ok := validateDerivative(relations, declaration)
		if !ok {
			return DerivativeLedger{}, ErrInvalidDerivative
		}
		seen[row.Derivative] = struct{}{}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(left, right int) bool { return rows[left].Derivative < rows[right].Derivative })
	return DerivativeLedger{rows: rows}, nil
}

func validateDerivative(relations RelationDispositionLedger, input DerivativeDeclaration) (DerivativeDeclaration, bool) {
	if input.Derivative == 0 || !input.Owner.valid() || input.Kind < DerivativePrivateIndex || input.Kind > DerivativePostRootCold ||
		input.Query < QueryExactLookup || input.Query > QueryRebind ||
		input.Asymptotic < AsymptoticConstant || input.Asymptotic > AsymptoticOutputLinear ||
		input.Allocation < AllocationZero || input.Allocation > AllocationColdOnly ||
		input.Replay < ReplayRebuild || input.Replay > ReplayPersistObservedClosure || len(input.Observed) == 0 {
		return DerivativeDeclaration{}, false
	}
	if input.Kind == DerivativePrivateIndex && input.Replay != ReplayRebuild {
		return DerivativeDeclaration{}, false
	}
	if input.Kind == DerivativePostRootCold && input.Allocation != AllocationColdOnly {
		return DerivativeDeclaration{}, false
	}
	observed := append([]semanticsource.Token(nil), input.Observed...)
	if !canonicalTokens(relations, observed) {
		return DerivativeDeclaration{}, false
	}
	terms := append([]CardinalityTerm(nil), input.Cardinality.Terms...)
	if input.Cardinality.Constant == 0 && len(terms) == 0 {
		return DerivativeDeclaration{}, false
	}
	for index, term := range terms {
		if term.Coefficient == 0 || len(term.Factors) == 0 || !canonicalTokens(relations, term.Factors) || forbiddenRetainedProduct(term.Factors) {
			return DerivativeDeclaration{}, false
		}
		if index > 0 && !lessCardinalityTerm(terms[index-1], term) {
			return DerivativeDeclaration{}, false
		}
		for _, factor := range term.Factors {
			at := sort.Search(len(observed), func(index int) bool { return !lessToken(observed[index], factor) })
			if at >= len(observed) || observed[at] != factor {
				return DerivativeDeclaration{}, false
			}
		}
		terms[index].Factors = append([]semanticsource.Token(nil), term.Factors...)
	}
	input.Observed = observed
	input.Cardinality.Terms = terms
	return input, true
}

func lessCardinalityTerm(left, right CardinalityTerm) bool {
	limit := len(left.Factors)
	if len(right.Factors) < limit {
		limit = len(right.Factors)
	}
	for index := 0; index < limit; index++ {
		if left.Factors[index] == right.Factors[index] {
			continue
		}
		return lessToken(left.Factors[index], right.Factors[index])
	}
	if len(left.Factors) != len(right.Factors) {
		return len(left.Factors) < len(right.Factors)
	}
	return false
}

func forbiddenRetainedProduct(factors []semanticsource.Token) bool {
	application, applicationOK := CatalogToken("LinkProjectBaseApplication@-")
	operation, operationOK := CatalogToken("TargetOperation@-")
	boundary, boundaryOK := CatalogToken("LinkBoundary@-")
	if !applicationOK || !operationOK || !boundaryOK {
		return true
	}
	hasApplication := false
	hasOperation := false
	for _, factor := range factors {
		if factor == boundary {
			return true
		}
		hasApplication = hasApplication || factor == application
		hasOperation = hasOperation || factor.Origin() == operation.Origin()
	}
	return hasApplication && hasOperation
}

func canonicalTokens(relations RelationDispositionLedger, tokens []semanticsource.Token) bool {
	for index, token := range tokens {
		if _, known := relations.Disposition(token); !known || index > 0 && !lessToken(tokens[index-1], token) {
			return false
		}
	}
	return true
}

// Count reports the number of declared retained derivatives.
func (ledger DerivativeLedger) Count() int { return len(ledger.rows) }

// At returns a detached declaration.
func (ledger DerivativeLedger) At(index int) (DerivativeDeclaration, bool) {
	if index < 0 || index >= len(ledger.rows) {
		return DerivativeDeclaration{}, false
	}
	row := ledger.rows[index]
	row.Observed = append([]semanticsource.Token(nil), row.Observed...)
	row.Cardinality.Terms = append([]CardinalityTerm(nil), row.Cardinality.Terms...)
	for index := range row.Cardinality.Terms {
		row.Cardinality.Terms[index].Factors = append([]semanticsource.Token(nil), row.Cardinality.Terms[index].Factors...)
	}
	return row, true
}
