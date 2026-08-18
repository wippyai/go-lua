package census

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// Census is the compact parser/AST construct denominator. Productions are
// derived from parser.go.y semantic actions; the remaining rows are derived
// from compiler/ast declarations. No row is authored by hand and no row is a
// fixture observation.
//
// The shape intentionally reuses parsersource.ActionTemplate and
// parsersource.Schema vocabulary. The future S1 seal join can therefore consume
// these rows without introducing another parser or AST type universe.
type Census struct {
	GrammarSourceDigest string
	ASTDigest           string
	Digest              string
	Productions         []parsersource.ActionTemplate
	Constructors        []parsersource.Constructor
	// Products is the per-action construction grain: one whole-constructor
	// field vector for every construction a reduction or a parser helper
	// performs. A production row states which forms a reduction builds;
	// a product row states what that reduction puts in each of the form's
	// carriers, which is the grain a carrier-state law is stated at.
	Products []parsersource.ActionProduct
	// Mutations is the same grain for the field assignments an action performs
	// on a value it has already constructed. They are held apart from products
	// because a mutation can move a carrier into a state no construction of its
	// form ever names, so folding them into products would lose exactly the
	// distinction a state law depends on.
	Mutations []parsersource.FieldMutation
	// Slots is the typed parent-slot denominator: one row for every carrier of a
	// constructed form that holds another AST value, with the role its parent
	// gives it and the cardinality it holds it at. It is the grain a consumption
	// law is stated at: a carrier row says a form declares a field, a slot row
	// says that field is where a child of an exact type and class lands.
	Slots []parsersource.UseSlot
	// Uses is the per-action half of that grain and the dual of Products: one
	// row for every coordinate of a construction that receives another AST
	// value, naming the slot it fills and where the action obtained the value.
	// A product row states what an action builds; a use row states where each
	// built value goes, so the two are the same relation read from its two ends.
	Uses []parsersource.ActionUse
	// Sequences is the list-building grain: one row for every list-valued
	// result carrier every reduction can leave a value in, stating how that
	// reduction assembles the list. It is a grain of its own because a list is
	// not a constructed AST form - it is the value a sequence carrier later
	// receives - so how long a reduction's list is, and which operand supplies
	// its final member, has no product or use row it could be stated on. A law
	// about a final-open expression list or an assignment target list is stated
	// over these rows.
	Sequences []parsersource.ActionSequence
}

// Canonical returns detached deterministic bytes for all census fields except
// the self-reported Digest. JSON is sufficient here because every map-like
// source is sorted before it reaches the model and the wire value contains no
// maps. The bytes are only a cold identity; they are not a runtime protocol.
func (c Census) Canonical() []byte {
	copy := c
	copy.Digest = ""
	encoded, err := json.Marshal(copy)
	if err != nil {
		return nil
	}
	return append([]byte(nil), encoded...)
}

func digest(c Census) string {
	sum := sha256.Sum256(c.Canonical())
	return hex.EncodeToString(sum[:])
}

// Validate checks a generated census against a freshly-derived source
// snapshot. This is deliberately exact: a stale row, dropped declaration, or
// changed parser action is rejected rather than treated as partial coverage.
func (c Census) Validate(root string) error {
	expected, err := Build(root)
	if err != nil {
		return err
	}
	if c.Digest == "" || c.Digest != digest(c) {
		return fmt.Errorf("parser census: invalid digest")
	}
	if !bytes.Equal(c.Canonical(), expected.Canonical()) || c.Digest != expected.Digest {
		return fmt.Errorf("parser census: generated rows differ from parser.go.y or compiler/ast declarations")
	}
	return nil
}

// Current returns a detached copy of the checked-in census after validating
// it against the current parser and AST source. Callers cannot mutate the
// generated backing slices through this API.
func Current(root string) (Census, error) {
	if err := Generated.Validate(root); err != nil {
		return Census{}, err
	}
	return clone(Generated), nil
}

func clone(c Census) Census {
	result := c
	result.Productions = append([]parsersource.ActionTemplate(nil), c.Productions...)
	for index := range result.Productions {
		result.Productions[index].RHS = append([]string(nil), c.Productions[index].RHS...)
		result.Productions[index].Constructors = append([]string(nil), c.Productions[index].Constructors...)
	}
	result.Constructors = append([]parsersource.Constructor(nil), c.Constructors...)
	for index := range result.Constructors {
		result.Constructors[index].Fields = append([]parsersource.Field(nil), c.Constructors[index].Fields...)
	}
	result.Products = append([]parsersource.ActionProduct(nil), c.Products...)
	for index := range result.Products {
		result.Products[index].Fields = append([]parsersource.ProductField(nil), c.Products[index].Fields...)
		for coordinate := range result.Products[index].Fields {
			result.Products[index].Fields[coordinate].States = append([]parsersource.FieldState(nil), c.Products[index].Fields[coordinate].States...)
		}
	}
	result.Mutations = append([]parsersource.FieldMutation(nil), c.Mutations...)
	for index := range result.Mutations {
		result.Mutations[index].States = append([]parsersource.FieldState(nil), c.Mutations[index].States...)
	}
	result.Slots = append([]parsersource.UseSlot(nil), c.Slots...)
	result.Uses = append([]parsersource.ActionUse(nil), c.Uses...)
	for index := range result.Uses {
		result.Uses[index].Origins = append([]parsersource.UseOrigin(nil), c.Uses[index].Origins...)
		result.Uses[index].Sources = append([]int(nil), c.Uses[index].Sources...)
		result.Uses[index].Symbols = append([]int(nil), c.Uses[index].Symbols...)
	}
	result.Sequences = append([]parsersource.ActionSequence(nil), c.Sequences...)
	for index := range result.Sequences {
		// A construction that states an empty list of operands is not the same
		// row as one that states no operands, so the detached copy keeps an
		// empty slice empty rather than letting it become an absent one.
		if c.Sequences[index].Segments != nil {
			result.Sequences[index].Segments = append(make([]parsersource.SequenceSegment, 0, len(c.Sequences[index].Segments)), c.Sequences[index].Segments...)
		}
		for segment := range result.Sequences[index].Segments {
			source := c.Sequences[index].Segments[segment]
			result.Sequences[index].Segments[segment].Origins = append([]parsersource.UseOrigin(nil), source.Origins...)
			result.Sequences[index].Segments[segment].Sources = append([]int(nil), source.Sources...)
			result.Sequences[index].Segments[segment].Symbols = append([]int(nil), source.Symbols...)
		}
	}
	return result
}
