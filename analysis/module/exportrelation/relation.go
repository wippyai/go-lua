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
	// ReturnTuples is a finite catalog of complete, literal return alternatives
	// published by the exporter. Unlike the ordinary function type, it retains
	// slot correlation without reconstructing a provider body at import sites.
	ReturnTuples []ReturnTuple
	Forwarded    bool
	Store        *OwnershipStore
	NormalEqual  *Equality
	// Borrow lists the formal positions a read-only wrapper never retains: the
	// body neither stores, sends, re-passes, nor returns their graph, so each
	// argument stays frame-local at the caller.
	Borrow []int
	// AllocatedReturn marks a body the producer proved returns a graph that did
	// not exist before the call: evaluating it published a single owned return
	// escape, on an allocation of that evaluation rather than on one of its own
	// parameters. It carries no shape, so a consumer that needs the returned
	// graph still reads the checked return type. A returned capture, module
	// table, or parameter stays unmarked, and a consumer therefore cannot
	// mistake state the producer or the caller already holds for a fresh graph.
	AllocatedReturn bool
}

// ReturnTuple is one complete positional return alternative. Present is an
// optional same-length bitmap for tuple-only non-nil witnesses. It deliberately
// lives outside Value: Value is also the placement-template language, whose
// meaning must not change for a value/error correlation.
type ReturnTuple struct {
	Values  []Value
	Present []bool
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

// OwnershipStore carries the exact formal position of an exported store
// wrapper. Owner names the second formal of a positional ownership.store
// wrapper. When EscapingRoot is set the wrapper instead writes the value into a
// container with no allocation in its own frame (a module-captured local or a
// global), so there is no owner formal and Owner is unused.
type OwnershipStore struct {
	Value, Owner int
	EscapingRoot bool
}

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
	if f.Path == "" || f.Arity < 0 || (!f.Return.Valid(f.Arity) && !f.Conditional.Valid(f.Arity) && !validReturnTuples(f.ReturnTuples, f.Arity) && !f.Store.Valid(f.Arity) && !validBorrow(f.Borrow, f.Arity) && !f.AllocatedReturn) {
		return false
	}
	if (f.Return.Valid(f.Arity) && f.Conditional != nil) ||
		(f.Return.Valid(f.Arity) && len(f.ReturnTuples) != 0) ||
		(f.Conditional != nil && len(f.ReturnTuples) != 0) {
		return false
	}
	if f.NormalEqual != nil && (f.NormalEqual.Left < 0 || f.NormalEqual.Right < 0 || f.NormalEqual.Left >= f.Arity || f.NormalEqual.Right >= f.Arity) {
		return false
	}
	return true
}

func validReturnTuples(tuples []ReturnTuple, arity int) bool {
	if len(tuples) == 0 {
		return false
	}
	for _, tuple := range tuples {
		if len(tuple.Values) == 0 {
			return false
		}
		if len(tuple.Present) != 0 && len(tuple.Present) != len(tuple.Values) {
			return false
		}
		for index, value := range tuple.Values {
			if len(tuple.Present) != 0 && tuple.Present[index] {
				if value.Parameter != nil || value.Scalar != "" || len(value.Table) != 0 {
					return false
				}
				continue
			}
			if !value.Valid(arity) {
				return false
			}
		}
	}
	return true
}

func (c *ConditionalReturn) Valid(arity int) bool {
	return c != nil && c.Parameter >= 0 && c.Parameter < arity && exactScalar(c.Literal) && c.Match.Valid(arity) && c.Otherwise.Valid(arity)
}

func (s *OwnershipStore) Valid(arity int) bool {
	if s == nil || s.Value < 0 || s.Value >= arity {
		return false
	}
	if s.EscapingRoot {
		return true
	}
	return s.Owner >= 0 && s.Owner < arity && s.Value != s.Owner
}

// validBorrow reports whether every borrowed formal position is a distinct,
// in-range parameter. An empty list carries no disposition.
func validBorrow(borrow []int, arity int) bool {
	if len(borrow) == 0 {
		return false
	}
	seen := make(map[int]bool, len(borrow))
	for _, index := range borrow {
		if index < 0 || index >= arity || seen[index] {
			return false
		}
		seen[index] = true
	}
	return true
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
