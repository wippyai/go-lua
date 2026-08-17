package pack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/static"
	"github.com/wippyai/go-lua/internal/canonical"
)

// FormalValue is a Pack-owned view of a seal-time Program formal receipt. The
// endpoint and content-addressed formal ID are retained; no Program proof is.
type FormalValue struct {
	owner       *algebra
	formalID    identity.ContentID
	endpoint    Endpoint
	moduleKey   identity.ContentID
	bodyContext identity.ContentID
	position    uint32
	class       static.Class
	sealed      bool
}

func (value FormalValue) valid() bool {
	return value.sealed && value.owner != nil && value.owner.valid() && value.moduleKey.Available() && value.bodyContext.Available() && value.position != 0 && value.formalID.Available() && value.endpoint.valid() && value.endpoint.owner == value.owner && value.owner.admits(value.class)
}

// Valid reports whether the value is an issued Pack formal.
func (value FormalValue) Valid() bool { return value.valid() }

// ContextID returns the opaque Program formal identity. It is not a Pack
// endpoint coordinate and cannot mint another formal or scalar.
func (value FormalValue) ContextID() identity.ContentID {
	if !value.valid() {
		return identity.ContentID{}
	}
	return value.formalID
}

// Class returns the Static class admitted for this formal.
func (value FormalValue) Class() (static.Class, bool) {
	if !value.valid() {
		return static.Class{}, false
	}
	return value.class, true
}

// Same is the O(1) identity check for formals from one exact Pack authority.
func (value FormalValue) Same(other FormalValue) bool {
	return value.valid() && other.valid() && value.owner == other.owner && value.moduleKey == other.moduleKey && value.bodyContext == other.bodyContext && value.position == other.position && value.formalID == other.formalID && value.endpoint == other.endpoint
}

// FormalValueAt projects one Body boundary formal into the package-owned
// symbolic plane. The returned value has no retained Body or Link handle.
func (body Body) FormalValueAt(index int) (FormalValue, bool) {
	if !body.valid() || index < 0 || index >= len(body.schema.bodies[body.index].formals) {
		return FormalValue{}, false
	}
	row := body.schema.bodies[body.index]
	endpoint := row.formals[index]
	if !endpoint.valid() || endpoint.owner != body.schema.owner || index >= len(row.formalIDs) {
		return FormalValue{}, false
	}
	formalID := row.formalIDs[index]
	if !formalID.Available() {
		return FormalValue{}, false
	}
	value := FormalValue{owner: body.schema.owner, formalID: formalID, endpoint: endpoint, moduleKey: row.moduleKey, bodyContext: row.context, position: uint32(index + 1), class: endpoint.class, sealed: true}
	return value, value.valid()
}

const (
	formalCanonicalDomain  = "wippy.analysis.pack.formal"
	formalCanonicalVersion = 1
	betaCanonicalDomain    = "wippy.analysis.pack.beta"
	betaCanonicalVersion   = 1
)

// EncodeCanonical emits the deterministic replay identity of one formal. The
// Pack algebra ID, opaque Program formal identity, and Static class identity
// are framed; no process address, Link object, or endpoint coordinate enters.
func (value FormalValue) EncodeCanonical(ctx context.Context) ([]byte, error) {
	if !value.valid() {
		return nil, errors.New("pack: invalid formal")
	}
	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, formalCanonicalDomain, formalCanonicalVersion); err != nil {
		return nil, err
	}
	if err := writer.Record(1); err != nil {
		return nil, err
	}
	if err := writer.Bytes(value.owner.id[:]); err != nil {
		return nil, err
	}
	if err := writer.Bytes(value.moduleKey[:]); err != nil {
		return nil, err
	}
	if err := writer.Bytes(value.bodyContext[:]); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(value.position)); err != nil {
		return nil, err
	}
	contextID := value.formalID
	if !contextID.Available() {
		return nil, errors.New("pack: invalid formal context")
	}
	if err := writer.Bytes(contextID[:]); err != nil {
		return nil, err
	}
	if err := writeCanonicalClass(&writer, value.owner, value.class); err != nil {
		return nil, err
	}
	return writer.FinishBytes()
}

// CanonicalID hashes the complete canonical formal stream for compact replay
// comparisons. It is not an inverse constructor and cannot mint a formal.
func (value FormalValue) CanonicalID(ctx context.Context) (identity.ContentID, error) {
	encoded, err := value.EncodeCanonical(ctx)
	if err != nil {
		return identity.ContentID{}, err
	}
	return sha256.Sum256(encoded), nil
}

// BetaBinding is one typed beta-reduction binding from a boundary formal to
// an existing scalar image in the same Pack authority.
type BetaBinding struct {
	Variable FormalValue
	Image    Scalar
}

type betaEntry struct {
	variable FormalValue
	image    Scalar
}

// BetaSubstitution is an immutable, owner-fenced finite substitution. It is
// total on the variables it contains and leaves unrelated Pack expressions
// unchanged.
type BetaSubstitution struct {
	owner   *algebra
	entries []betaEntry
}

// NewBetaSubstitution rejects foreign authorities and duplicate variables.
func NewBetaSubstitution(bindings []BetaBinding) (BetaSubstitution, bool) {
	if len(bindings) == 0 {
		return BetaSubstitution{}, true
	}
	entries := make([]betaEntry, len(bindings))
	var owner *algebra
	for index, binding := range bindings {
		if !binding.Variable.valid() || !binding.Image.valid() || binding.Variable.owner != binding.Image.owner {
			return BetaSubstitution{}, false
		}
		if owner == nil {
			owner = binding.Variable.owner
		}
		if owner != binding.Variable.owner {
			return BetaSubstitution{}, false
		}
		entries[index] = betaEntry{variable: binding.Variable, image: binding.Image}
	}
	sort.Slice(entries, func(left, right int) bool {
		if bytes.Compare(entries[left].variable.moduleKey[:], entries[right].variable.moduleKey[:]) != 0 {
			return bytes.Compare(entries[left].variable.moduleKey[:], entries[right].variable.moduleKey[:]) < 0
		}
		if bytes.Compare(entries[left].variable.bodyContext[:], entries[right].variable.bodyContext[:]) != 0 {
			return bytes.Compare(entries[left].variable.bodyContext[:], entries[right].variable.bodyContext[:]) < 0
		}
		if entries[left].variable.position != entries[right].variable.position {
			return entries[left].variable.position < entries[right].variable.position
		}
		leftID := entries[left].variable.formalID
		rightID := entries[right].variable.formalID
		return bytes.Compare(leftID[:], rightID[:]) < 0
	})
	for index := 1; index < len(entries); index++ {
		if entries[index-1].variable.Same(entries[index].variable) {
			return BetaSubstitution{}, false
		}
	}
	return BetaSubstitution{owner: owner, entries: entries}, true
}

// Apply performs beta substitution through scalar endpoint occurrences in a
// Pack term. Free/bound tails and class-only unknowns are preserved exactly.
func (substitution BetaSubstitution) Apply(term Term) (Term, bool) {
	if !term.valid() || substitution.owner != nil && term.owner != substitution.owner {
		return Term{}, false
	}
	if len(substitution.entries) == 0 || term.kind == TermAny {
		return term, true
	}
	replace := func(values []Scalar) ([]Scalar, bool) {
		result := append([]Scalar(nil), values...)
		for index, scalar := range result {
			next, ok := substitution.scalar(scalar)
			if !ok {
				return nil, false
			}
			result[index] = next
		}
		return result, true
	}
	switch term.kind {
	case TermClosed:
		prefix, ok := replace(term.prefix)
		if !ok {
			return Term{}, false
		}
		return closedTerm(substitution.owner, prefix)
	case TermOpen:
		prefix, ok := replace(term.prefix)
		if !ok {
			return Term{}, false
		}
		suffix, ok := replace(term.suffix)
		if !ok {
			return Term{}, false
		}
		return openTerm(substitution.owner, prefix, term.rest, suffix)
	default:
		return Term{}, false
	}
}

// ApplyValue maps every complete equation in one Pack Value. The relation
// target vector is unchanged, so the result remains publishable at the same
// root and the operation commutes with the Pack lattice constructors.
func (substitution BetaSubstitution) ApplyValue(value Value) (Value, bool) {
	if !value.valid() || len(substitution.entries) == 0 {
		if len(substitution.entries) == 0 && value.valid() {
			return value, true
		}
		return Value{}, false
	}
	if substitution.owner == nil || value.owner != substitution.owner {
		return Value{}, false
	}
	if value.bottom || value.top {
		return value, true
	}
	cases := make([]Case, len(value.cases))
	for caseIndex, current := range value.cases {
		if !current.valid() || current.owner != substitution.owner {
			return Value{}, false
		}
		equations := make([]Equation, len(current.equations))
		for equationIndex, equation := range current.equations {
			if equation.kind == EquationScalar {
				scalar, ok := substitution.scalar(equation.scalar)
				if !ok {
					return Value{}, false
				}
				equations[equationIndex], ok = scalarEquation(equation.endpoint, scalar)
				if !ok {
					return Value{}, false
				}
				continue
			}
			term, ok := substitution.Apply(equation.term)
			if !ok {
				return Value{}, false
			}
			equations[equationIndex], ok = packEquation(equation.port, term)
			if !ok {
				return Value{}, false
			}
		}
		cases[caseIndex], _ = exactCase(value.relation, equations)
		if !cases[caseIndex].valid() {
			return Value{}, false
		}
	}
	return valueFromCases(value.relation, cases)
}

func (substitution BetaSubstitution) scalar(value Scalar) (Scalar, bool) {
	if !value.valid() || value.owner != substitution.owner || value.kind != ScalarEndpoint {
		return value, true
	}
	for _, entry := range substitution.entries {
		if entry.variable.endpoint == value.endpoint {
			return entry.image, true
		}
		// Entries are ordered by opaque formal identity, not by Pack endpoint.
	}
	return value, true
}

// EncodeCanonical emits a deterministic substitution replay identity,
// including each formal coordinate and its complete scalar image.
func (substitution BetaSubstitution) EncodeCanonical(ctx context.Context) ([]byte, error) {
	if substitution.owner == nil && len(substitution.entries) != 0 {
		return nil, errors.New("pack: invalid beta substitution")
	}
	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, betaCanonicalDomain, betaCanonicalVersion); err != nil {
		return nil, err
	}
	if err := writer.Record(1); err != nil {
		return nil, err
	}
	if substitution.owner == nil {
		if err := writer.Bool(false); err != nil {
			return nil, err
		}
	} else {
		if !substitution.owner.valid() {
			return nil, errors.New("pack: invalid beta owner")
		}
		if err := writer.Bool(true); err != nil {
			return nil, err
		}
		if err := writer.Bytes(substitution.owner.id[:]); err != nil {
			return nil, err
		}
	}
	if err := writer.Count(uint64(len(substitution.entries))); err != nil {
		return nil, err
	}
	for _, entry := range substitution.entries {
		if !entry.variable.valid() || entry.variable.owner != substitution.owner || !entry.image.valid() || entry.image.owner != substitution.owner {
			return nil, errors.New("pack: invalid beta entry")
		}
		contextID := entry.variable.formalID
		if !contextID.Available() {
			return nil, errors.New("pack: invalid beta formal context")
		}
		if err := writer.Bytes(entry.variable.moduleKey[:]); err != nil {
			return nil, err
		}
		if err := writer.Bytes(entry.variable.bodyContext[:]); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(entry.variable.position)); err != nil {
			return nil, err
		}
		if err := writer.Bytes(contextID[:]); err != nil {
			return nil, err
		}
		if err := writeCanonicalScalar(&writer, substitution.owner, entry.image); err != nil {
			return nil, err
		}
	}
	return writer.FinishBytes()
}

func writeCanonicalClass(writer *canonical.Writer, owner *algebra, class static.Class) error {
	id, ok := owner.classIdentity(class)
	if !ok {
		return errors.New("pack: foreign class")
	}
	return writer.Bytes(id[:])
}

func writeCanonicalScalar(writer *canonical.Writer, owner *algebra, value Scalar) error {
	if writer == nil || owner == nil || !value.valid() || value.owner != owner {
		return errors.New("pack: invalid scalar")
	}
	if err := writer.Record(uint64(value.kind)); err != nil {
		return err
	}
	switch value.kind {
	case ScalarEndpoint:
		if err := writer.Uint(uint64(value.endpoint.index)); err != nil {
			return err
		}
		return writeCanonicalClass(writer, owner, value.class)
	case ScalarHead:
		if err := writeCanonicalTail(writer, owner, value.tail); err != nil {
			return err
		}
		if err := writer.Uint(uint64(value.offset.index)); err != nil {
			return err
		}
		return writeCanonicalClass(writer, owner, value.class)
	case ScalarAny:
		return writeCanonicalClass(writer, owner, value.class)
	default:
		return errors.New("pack: invalid scalar kind")
	}
}

func writeCanonicalTail(writer *canonical.Writer, owner *algebra, tail TailRef) error {
	if !tail.valid() || tail.owner != owner {
		return errors.New("pack: invalid tail")
	}
	if err := writer.Record(uint64(tail.kind)); err != nil {
		return err
	}
	if err := writer.Uint(uint64(tail.index)); err != nil {
		return err
	}
	return writeCanonicalClass(writer, owner, tail.class)
}
