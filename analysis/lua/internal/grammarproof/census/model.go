package census

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/grammar"
)

// Census is the compact parser/AST construct denominator. Productions are
// derived from parser.go.y semantic actions; the remaining rows are derived
// from compiler/ast declarations. No row is authored by hand and no row is a
// fixture observation.
//
// The shape intentionally reuses grammarproof.ActionTemplate and
// grammar.Schema vocabulary. The future S1 seal join can therefore consume
// these rows without introducing another parser or AST type universe.
type Census struct {
	GrammarSourceDigest string
	ASTDigest           string
	Digest              string
	Productions         []grammarproof.ActionTemplate
	Constructors        []grammar.Constructor
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
	result.Productions = append([]grammarproof.ActionTemplate(nil), c.Productions...)
	for index := range result.Productions {
		result.Productions[index].RHS = append([]string(nil), c.Productions[index].RHS...)
		result.Productions[index].Constructors = append([]string(nil), c.Productions[index].Constructors...)
	}
	result.Constructors = append([]grammar.Constructor(nil), c.Constructors...)
	for index := range result.Constructors {
		result.Constructors[index].Fields = append([]grammar.Field(nil), c.Constructors[index].Fields...)
	}
	return result
}

// Equal reports exact equality of two detached census values. It is kept for
// generator tests and future seal-join callers; it does not compare a
// self-reported digest in place of the rows.
func Equal(left, right Census) bool {
	return reflect.DeepEqual(left, right)
}
