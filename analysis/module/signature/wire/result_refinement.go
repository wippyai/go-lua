package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// The result-refinement payload is the standalone form of one statement a
// library contract makes about a result slot: the slot is refined, and the
// predicate the caller must discharge to consume the refinement is enumerable
// contract data rather than a computation.
//
// It lives here because one arm of the union carries a TYPE, and this is the
// layer that owns the type wire. A refined result type projected beside its
// consumer would be a second answer to what a type is on the wire, and the two
// would diverge the first time the type algebra grew a node.
//
// The union is closed. A refinement whose predicate is not one of the declared
// kinds is not carried as a half-stated one: a member whose selection is driven
// by an arbitrary caller literal is delegated to the rule that owns it, which is
// a different member form entirely.

// ResultRefinementSchema versions the standalone result-refinement payload. It
// is independent of the manifest document schema and of the callable envelope:
// the three are published separately and cannot share a version decision.
const ResultRefinementSchema = "go-lua.result.refinement/v1"

// RefinementKind is the closed catalog of predicates a result refinement may be
// stated over. Each kind names one enumerable relation between caller data and
// one result slot.
type RefinementKind uint8

const (
	RefinementInvalid RefinementKind = iota
	// RefinementLiteralArgument refines a result slot when one argument is a
	// declared string literal. The predicate is the literal itself, so both the
	// predicate and the refined type are contract data.
	RefinementLiteralArgument
	// RefinementSubjectLength refines a result slot when a position argument
	// has been proved to lie within the length of a subject argument. The slot
	// is optional only because the position may fall outside the subject, so a
	// caller that proved otherwise has discharged that optionality and nothing
	// else about the slot changes.
	RefinementSubjectLength
	refinementKindLimit
)

func (kind RefinementKind) Available() bool {
	return kind > RefinementInvalid && kind < refinementKindLimit
}

// refinementKindSpelling is the boundary spelling of each kind. It is the one
// statement of how a kind is written, consulted by both directions, so a kind
// added to the union without a spelling is unwritable and unreadable together
// rather than writable and unreadable.
var refinementKindSpelling = map[RefinementKind]string{
	RefinementLiteralArgument: "refinement.literalArgument",
	RefinementSubjectLength:   "refinement.subjectLength",
}

// ResultRefinement is one member of the closed union. The sealing method is
// unexported, so the catalog of refinements is this file's and a consumer
// cannot introduce an arm the format has no bytes for.
type ResultRefinement interface {
	// Kind is which predicate this refinement is stated over.
	Kind() RefinementKind
	// Available reports whether this refinement states a complete relation: a
	// result slot, the caller data the predicate reads, and - where the kind
	// carries one - the refined type.
	Available() bool
	sealedResultRefinement()
}

// LiteralArgumentRefinement is the refinement of one result slot by a declared
// string literal in one argument position.
type LiteralArgumentRefinement struct {
	// Result is the refined result position.
	Result int
	// Argument is the argument position the literal is read from.
	Argument int
	// Literal is the argument value the refinement is predicated on.
	Literal string
	// Type is what the result slot carries once the predicate holds.
	Type typ.Type
}

func (LiteralArgumentRefinement) Kind() RefinementKind { return RefinementLiteralArgument }

func (refinement LiteralArgumentRefinement) Available() bool {
	return refinement.Result >= 0 && refinement.Argument >= 0 && refinement.Type != nil
}

func (LiteralArgumentRefinement) sealedResultRefinement() {}

// SubjectLengthRefinement is the refinement of one optional result slot by a
// proof that a position argument lies within a subject argument's length.
type SubjectLengthRefinement struct {
	// Result is the refined result position.
	Result int
	// Subject is the argument position whose length bounds the read.
	Subject int
	// Position is the argument position holding the position read.
	Position int
	// Default is the contract's own position when the call omits the position
	// argument. A Lua position is one-based and counts from the end when
	// negative, so zero addresses nothing and states no default.
	Default int64
}

func (SubjectLengthRefinement) Kind() RefinementKind { return RefinementSubjectLength }

func (refinement SubjectLengthRefinement) Available() bool {
	return refinement.Result >= 0 && refinement.Subject >= 0 &&
		refinement.Position >= 0 && refinement.Default != 0
}

func (SubjectLengthRefinement) sealedResultRefinement() {}

// ResultRefinementEquals reports whether two refinements state the same
// relation. Refined types are compared by type identity, so two spellings of
// one type are one refinement.
func ResultRefinementEquals(refinement, other ResultRefinement) bool {
	if refinement == nil || other == nil {
		return refinement == nil && other == nil
	}
	if refinement.Kind() != other.Kind() {
		return false
	}
	switch left := refinement.(type) {
	case LiteralArgumentRefinement:
		right, ok := other.(LiteralArgumentRefinement)
		return ok && left.Result == right.Result && left.Argument == right.Argument &&
			left.Literal == right.Literal && typ.TypeEquals(left.Type, right.Type)
	case SubjectLengthRefinement:
		right, ok := other.(SubjectLengthRefinement)
		return ok && left == right
	default:
		return false
	}
}

// resultRefinementWire is the payload document. The schema is written inside the
// payload so a reader that holds only the bytes rejects a projection it was not
// written for, and each arm is a named object so the kind and the fields it
// carries cannot disagree.
type resultRefinementWire struct {
	Schema          string                   `json:"schema"`
	Kind            string                   `json:"kind"`
	LiteralArgument *literalArgumentWire     `json:"literalArgument,omitempty"`
	SubjectLength   *subjectLengthRefineWire `json:"subjectLength,omitempty"`
}

type literalArgumentWire struct {
	Result   int       `json:"result"`
	Argument int       `json:"argument"`
	Literal  string    `json:"literal"`
	Type     *TypeWire `json:"type,omitempty"`
}

type subjectLengthRefineWire struct {
	Result   int   `json:"result"`
	Subject  int   `json:"subject"`
	Position int   `json:"position"`
	Default  int64 `json:"default"`
}

// EncodeResultRefinement writes one result refinement as a member payload body.
// A refinement that states no complete relation is refused rather than published
// as one that does: a member carrying it would claim a discharge condition no
// caller could satisfy.
func EncodeResultRefinement(refinement ResultRefinement) ([]byte, error) {
	if refinement == nil {
		return nil, errors.New("signature/wire: encode absent result refinement")
	}
	spelling, named := refinementKindSpelling[refinement.Kind()]
	if !named {
		return nil, fmt.Errorf("signature/wire: unsupported result refinement kind %d", refinement.Kind())
	}
	if !refinement.Available() {
		return nil, fmt.Errorf("signature/wire: incomplete result refinement %s", spelling)
	}
	document := resultRefinementWire{Schema: ResultRefinementSchema, Kind: spelling}
	switch arm := refinement.(type) {
	case LiteralArgumentRefinement:
		encoded, err := EncodeType(arm.Type)
		if err != nil {
			return nil, fmt.Errorf("signature/wire: encode result refinement: %w", err)
		}
		if encoded == nil {
			return nil, errors.New("signature/wire: encode result refinement: refined type wrote nothing")
		}
		document.LiteralArgument = &literalArgumentWire{
			Result: arm.Result, Argument: arm.Argument, Literal: arm.Literal, Type: encoded,
		}
	case SubjectLengthRefinement:
		document.SubjectLength = &subjectLengthRefineWire{
			Result: arm.Result, Subject: arm.Subject, Position: arm.Position, Default: arm.Default,
		}
	default:
		return nil, fmt.Errorf("signature/wire: unsupported result refinement %T", refinement)
	}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("signature/wire: encode result refinement: %w", err)
	}
	return data, nil
}

// DecodeResultRefinement reads one result-refinement payload. The payload is an
// external boundary in the same sense a manifest is, so a malformed body is
// always an error and a type-codec builder never leaks a panic to the reader.
func DecodeResultRefinement(data []byte) (refinement ResultRefinement, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			refinement = nil
			err = fmt.Errorf("signature/wire: invalid result refinement: %v", recovered)
		}
	}()
	document, err := readResultRefinementDocument(data)
	if err != nil {
		return nil, err
	}
	kind, known := resultRefinementKind(document.Kind)
	if !known {
		return nil, fmt.Errorf("signature/wire: unknown result refinement kind %q", document.Kind)
	}
	decoded, err := decodeResultRefinementArm(kind, document)
	if err != nil {
		return nil, err
	}
	if !decoded.Available() {
		return nil, fmt.Errorf("signature/wire: incomplete result refinement %q", document.Kind)
	}
	return decoded, nil
}

// readResultRefinementDocument reads the payload framing: one document, in this
// format, with no field this format does not declare.
func readResultRefinementDocument(data []byte) (resultRefinementWire, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return resultRefinementWire{}, errors.New("signature/wire: decode empty result refinement")
	}
	var document resultRefinementWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return resultRefinementWire{}, fmt.Errorf("signature/wire: decode result refinement: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return resultRefinementWire{}, errors.New("signature/wire: decode multiple result refinements")
		}
		return resultRefinementWire{}, fmt.Errorf("signature/wire: decode result refinement: %w", err)
	}
	if document.Schema != ResultRefinementSchema {
		return resultRefinementWire{}, fmt.Errorf(
			"signature/wire: result refinement schema %q, want %q", document.Schema, ResultRefinementSchema)
	}
	return document, nil
}

// decodeResultRefinementArm reads exactly the arm the kind names. An arm the
// kind does not name is refused wherever it appears, so a document cannot carry
// one relation and be read as another.
func decodeResultRefinementArm(kind RefinementKind, document resultRefinementWire) (ResultRefinement, error) {
	switch kind {
	case RefinementLiteralArgument:
		if document.LiteralArgument == nil || document.SubjectLength != nil {
			return nil, fmt.Errorf("signature/wire: result refinement %q carries the wrong arm", document.Kind)
		}
		arm := document.LiteralArgument
		if arm.Type == nil {
			return nil, fmt.Errorf("signature/wire: result refinement %q refines no type", document.Kind)
		}
		refined, err := DecodeType(arm.Type)
		if err != nil {
			return nil, fmt.Errorf("signature/wire: decode result refinement: %w", err)
		}
		return LiteralArgumentRefinement{
			Result: arm.Result, Argument: arm.Argument, Literal: arm.Literal, Type: refined,
		}, nil
	case RefinementSubjectLength:
		if document.SubjectLength == nil || document.LiteralArgument != nil {
			return nil, fmt.Errorf("signature/wire: result refinement %q carries the wrong arm", document.Kind)
		}
		arm := document.SubjectLength
		return SubjectLengthRefinement{
			Result: arm.Result, Subject: arm.Subject, Position: arm.Position, Default: arm.Default,
		}, nil
	default:
		return nil, fmt.Errorf("signature/wire: unsupported result refinement kind %d", kind)
	}
}

// resultRefinementKind resolves one boundary spelling. The spelling table is the
// single statement both directions read, so resolution is a table read and never
// a second switch.
func resultRefinementKind(spelling string) (RefinementKind, bool) {
	for kind, named := range refinementKindSpelling {
		if named == spelling {
			return kind, true
		}
	}
	return RefinementInvalid, false
}
