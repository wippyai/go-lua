package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ObjectEntry describes one static value written under an object constructor.
type ObjectEntry struct {
	suffix      path.Path
	source      ValueSource
	expected    product.Value
	hasExpected bool
}

// NewObjectEntry creates a static object-entry descriptor.
func NewObjectEntry(suffix path.Path, source ValueSource) ObjectEntry {
	return ObjectEntry{
		suffix: suffix.Clone(),
		source: source,
	}
}

// Suffix returns the relative static suffix under the constructed object.
func (e ObjectEntry) Suffix() path.Path { return e.suffix.Clone() }

// Source returns the value assigned to the entry.
func (e ObjectEntry) Source() ValueSource { return e.source }

// Expected returns the contextual value contract for this static entry when the
// object literal is checked against a declared table shape.
func (e ObjectEntry) Expected() (product.Value, bool) { return e.expected, e.hasExpected }

// WithExpected returns a copy carrying the contextual value contract for this
// entry.
func (e ObjectEntry) WithExpected(value product.Value) ObjectEntry {
	e = e.copy()
	e.expected = value
	e.hasExpected = true
	return e
}

func (e ObjectEntry) copy() ObjectEntry {
	e.suffix = e.suffix.Clone()
	return e
}

// ObjectLiteral describes static entries associated with an expression.
type ObjectLiteral struct {
	entries     []ObjectEntry
	expected    product.Value
	hasExpected bool
	identity    identity.ID
}

// NewObjectLiteral creates an object literal sidecar from static entries.
func NewObjectLiteral(entries []ObjectEntry) ObjectLiteral {
	return ObjectLiteral{entries: copyObjectEntries(entries)}
}

// Entries returns the static entries for this object literal.
func (l ObjectLiteral) Entries() []ObjectEntry { return copyObjectEntries(l.entries) }

// Expected returns the declared contextual type value the literal is assigned to,
// carried as a type-witness value so the factflow layer stays type-agnostic. The
// boolean reports whether a contextual record target is known for this literal.
func (l ObjectLiteral) Expected() (product.Value, bool) { return l.expected, l.hasExpected }

// Identity returns the stable literal identity when one was attached during
// lowering.
func (l ObjectLiteral) Identity() (identity.ID, bool) {
	if l.identity == (identity.ID{}) {
		return identity.ID{}, false
	}
	return l.identity, true
}

// WithExpected returns a copy carrying the declared contextual type value.
func (l ObjectLiteral) WithExpected(value product.Value) ObjectLiteral {
	out := l.copy()
	out.expected = value
	out.hasExpected = true
	return out
}

// WithIdentity returns a copy carrying a stable literal identity.
func (l ObjectLiteral) WithIdentity(id identity.ID) ObjectLiteral {
	out := l.copy()
	out.identity = id
	return out
}

func (l ObjectLiteral) copy() ObjectLiteral {
	l.entries = copyObjectEntries(l.entries)
	return l
}

func copyObjectEntries(in []ObjectEntry) []ObjectEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]ObjectEntry, len(in))
	for i := range in {
		out[i] = in[i].copy()
	}
	return out
}

func copyObjectLiteralMap(in map[ExprRef]ObjectLiteral) map[ExprRef]ObjectLiteral {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]ObjectLiteral, len(in))
	for expr, fact := range in {
		out[expr] = fact.copy()
	}
	return out
}
