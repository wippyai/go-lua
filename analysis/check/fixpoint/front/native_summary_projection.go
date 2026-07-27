package front

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// summaryNativeFacts publishes the two properties that decide whether one
// compiled body may be shared by every call site: how exact its result contract
// is, and whether that contract depends on anything but the arguments.
//
// A caller-invariant summary lets one inline cache serve every call site; a
// context-sensitive one does not, and the difference is a published fact rather
// than a caller's guess. A body whose result contract is not declared publishes
// nothing at all: an unstated result has no exactness to report.
func summaryNativeFacts(root Compilation) []NativeProjection {
	var rows []NativeProjection
	var visit func(Compilation)
	visit = func(compilation Compilation) {
		for _, child := range compilation.nested {
			if row, published := summaryBodyFact(child); published {
				rows = append(rows, row)
			}
			visit(child)
		}
	}
	visit(root)
	return rows
}

func summaryBodyFact(compilation Compilation) (NativeProjection, bool) {
	body := compilation.WIR
	if body == nil {
		return NativeProjection{}, false
	}
	returns := body.DeclaredReturnTypes()
	if len(returns) == 0 {
		return NativeProjection{}, false
	}
	generic := false
	for _, parameter := range compilation.Boundary.Parameters {
		generic = generic || typ.ContainsTypeParam(body.Type(parameter.Type))
	}
	exact := true
	for _, result := range returns {
		generic = generic || typ.ContainsTypeParam(result)
		exact = exact && nativeExactResultType(result)
	}
	// A captured declaration that is written somewhere in the bound unit is a
	// second input the caller does not supply, so the summary cannot be shared
	// across call sites and the store through the cell ends it.
	mutableCapture := false
	for _, capture := range compilation.Boundary.Captures {
		mutableCapture = mutableCapture || capture.Mutable
	}
	invariance, revocation := "caller_invariant", ""
	if generic {
		invariance = "context_sensitive"
	}
	if mutableCapture {
		invariance, revocation = "context_sensitive", "write.upvalue"
	}
	exactness := "over_approximation"
	if exact && !generic {
		exactness = "exact"
	}
	key := "interproc_summary/" + fmt.Sprintf("%x", compilation.Body)
	if revocation != "" {
		key += "/contract-revocation/" + revocation
	}
	return NativeProjection{Key: key,
		Value:   "exactness=" + exactness + " invariance=" + invariance,
		Subject: compilation.PrototypeName,
	}, true
}

// nativeExactResultType reports whether a declared result names one concrete
// contract an inline cache can be built against. The exact-capable shapes are
// named positively: a primitive and a literal are single values, and a record,
// array, map, tuple or function names one layout whose own members carry
// whatever further imprecision they have. Every other shape describes a set the
// call site must still discriminate — a union or optional by construction, an
// interface or intersection because any number of layouts satisfy it, a generic
// or instantiated because its arguments decide the layout, a recursive because
// its unfolding does — and an unresolved reference names nothing yet. The type
// vocabulary is open, so an unrecognized shape is not exact either.
func nativeExactResultType(result typ.Type) bool {
	result = unwrap.Alias(result)
	if result == nil || typ.AbsentOrTopLike(result) || typ.ContainsTypeParam(result) {
		return false
	}
	switch result.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal,
		kind.Record, kind.Array, kind.Map, kind.ReadonlyMap, kind.Tuple, kind.Function:
		return true
	}
	return false
}
