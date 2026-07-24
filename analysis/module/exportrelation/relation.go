// Package exportrelation owns closed value relations which accompany a module
// export.  The type surface remains the authority for callable compatibility;
// these rows carry only producer-evaluated, finite result templates.
package exportrelation

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Summary is the single module-boundary publication consumed by an importer.
// A nil Type or malformed relation is intentionally unusable by consumers.
type Summary struct {
	Type      typ.Type
	Functions []Function
}

// Function identifies one static member of a module export. Path is a
// dot-separated static member path relative to the require result.
type Function struct {
	Path        string
	Arity       int
	Return      Value
	Conditional *ConditionalReturn
	Forwarded   bool
	Store       *OwnershipStore
	NormalEqual *Equality
}

// ConditionalReturn is a closed, producer-evaluated two-way return relation.
// It records the one literal test that selects Match; Otherwise is selected
// only when the exact caller argument is a different closed scalar.
type ConditionalReturn struct {
	Parameter int
	Literal   string
	Match     Value
	Otherwise Value
}

// OwnershipStore carries the two exact formal positions of an exported
// wrapper around the already-published ownership.store contract.
type OwnershipStore struct{ Value, Owner int }

// Equality is a normal-return-only equality between two formal parameters.
type Equality struct{ Left, Right int }

// Value is a finite, serializable return template. A Parameter is an ordinal
// reference in the exported callable's already-validated signature; it is
// materialized only from that call site's exact, published argument fact.
// An unknown argument therefore cannot become an imported result witness.
type Value struct {
	Parameter *int
	Scalar    string
	Table     []Member
}

type Member struct {
	Suffix string
	Value  Value
}

func (s Summary) Function(path string, arity int) (Function, bool) {
	if path == "" || arity < 0 {
		return Function{}, false
	}
	for _, f := range s.Functions {
		if f.Path == path && f.Arity == arity {
			return f, f.Valid()
		}
	}
	return Function{}, false
}

func (f Function) Valid() bool {
	if f.Path == "" || f.Arity < 0 || (!f.Return.Valid(f.Arity) && !f.Conditional.Valid(f.Arity) && !f.Store.Valid(f.Arity)) {
		return false
	}
	if f.Return.Valid(f.Arity) && f.Conditional != nil {
		return false
	}
	if f.NormalEqual != nil && (f.NormalEqual.Left < 0 || f.NormalEqual.Right < 0 || f.NormalEqual.Left >= f.Arity || f.NormalEqual.Right >= f.Arity) {
		return false
	}
	return true
}

func (c *ConditionalReturn) Valid(arity int) bool {
	return c != nil && c.Parameter >= 0 && c.Parameter < arity && exactScalar(c.Literal) && c.Match.Valid(arity) && c.Otherwise.Valid(arity)
}

func (s *OwnershipStore) Valid(arity int) bool {
	return s != nil && s.Value >= 0 && s.Owner >= 0 && s.Value != s.Owner && s.Value < arity && s.Owner < arity
}

// Closed reports whether v is an exact immutable literal tree without a
// call-site substitution. Parameterized templates are valid export relations,
// but are not literal trees on their own.
func (v Value) Closed() bool {
	if v.Parameter != nil {
		return false
	}
	if v.Scalar != "" {
		return exactScalar(v.Scalar)
	}
	if len(v.Table) == 0 {
		return false
	}
	for _, member := range v.Table {
		if !member.Value.Closed() {
			return false
		}
	}
	return true
}

func exactScalar(value string) bool {
	switch value {
	case "scalar/bool/true", "scalar/bool/false", "scalar/nil":
		return true
	}
	if stringValue, found := strings.CutPrefix(value, "scalar/string/"); found {
		_, err := strconv.Unquote(stringValue)
		return err == nil
	}
	if number, found := strings.CutPrefix(value, "scalar/number/"); found {
		_, err := strconv.ParseFloat(number, 64)
		return err == nil
	}
	return false
}

func (v Value) Valid(arity int) bool {
	if v.Parameter != nil {
		return *v.Parameter >= 0 && *v.Parameter < arity && v.Scalar == "" && len(v.Table) == 0
	}
	if v.Scalar != "" {
		return len(v.Table) == 0
	}
	if len(v.Table) == 0 {
		return false
	}
	seen := make(map[string]bool, len(v.Table))
	for _, m := range v.Table {
		if m.Suffix == "" || seen[m.Suffix] || !m.Value.Valid(arity) {
			return false
		}
		seen[m.Suffix] = true
	}
	return true
}
