package assembly

import (
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

func staticCoordinate(w *Collector, span source.Span) (source.Coordinate, bool) {
	if w == nil || !mutationReady(w) {
		return source.Coordinate{}, false
	}
	if !validSpan(w, span) {
		rejectMutationf(w, "program/lower/collector: invalid Static span")
		return source.Coordinate{}, false
	}
	coordinate, ok := source.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	if !ok {
		w.fail(errors.New("program/lower/collector: invalid Static coordinate"))
		return source.Coordinate{}, false
	}
	return coordinate, true
}

func staticEmit(w *Collector, family keyspace.Family, span source.Span, accept func(Term) error) Term {
	if w == nil {
		return 0
	}
	term := w.mint(family, span)
	if term == 0 {
		return 0
	}
	if err := accept(term); err != nil {
		w.fail(err)
		return 0
	}
	return term
}

func staticFill(w *Collector, apply func() error) bool {
	if !mutationReady(w) {
		return false
	}
	if err := apply(); err != nil {
		w.fail(err)
		return false
	}
	return true
}

// staticAdmitExact is the only Static-to-Source coordination point. Static
// rows retain the raw payload, but never retain a Source owner or call an
// admission callback. The concrete Collector operation below appends the
// payload to Source's single exact denominator before the row is minted.
func staticAdmitExact(w *Collector, value keyspace.LiteralValue) bool {
	if w == nil || !mutationReady(w) {
		return false
	}
	if !validRawExactCandidate(value) {
		if w != nil && !w.terminal {
			w.fail(errors.New("program/lower/collector: invalid Static exact payload"))
		}
		return false
	}
	if w.addExact(value) {
		return true
	}
	if w.err == nil && !w.terminal {
		w.fail(errors.New("program/lower/collector: Source exact admission rejected payload"))
	}
	return false
}

func staticAdmitString(w *Collector, value string) bool {
	if !staticStringValid(w, value) {
		return false
	}
	return staticAdmitExact(w, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value})
}

// staticAdmitPaths validates every component of every path first, then
// submits all components through the concrete Source operation. This keeps a
// malformed later component from leaving an earlier path component admitted.
func staticAdmitPaths(w *Collector, paths ...[]string) bool {
	for _, path := range paths {
		if !staticPathValid(w, path) {
			return false
		}
	}
	for _, path := range paths {
		for _, part := range path {
			if !staticAdmitString(w, part) {
				return false
			}
		}
	}
	return true
}

func staticStringValid(w *Collector, value string) bool {
	if value != "" {
		return true
	}
	if w != nil && !w.terminal && w.err == nil {
		w.fail(errors.New("program/lower/collector: empty Static key"))
	}
	return false
}

func staticPathValid(w *Collector, path []string) bool {
	if len(path) == 0 {
		if w != nil && !w.terminal && w.err == nil {
			w.fail(errors.New("program/lower/collector: empty Static type path"))
		}
		return false
	}
	for _, part := range path {
		if !staticStringValid(w, part) {
			return false
		}
	}
	return true
}

func staticFloatBitsValid(w *Collector, bits uint64) bool {
	if !math.IsNaN(math.Float64frombits(bits)) {
		return true
	}
	if w != nil && !w.terminal && w.err == nil {
		w.fail(errors.New("program/lower/collector: NaN static float literal"))
	}
	return false
}

// staticExistingTerm closes the construction-time child edge against the one
// Collector census. A Static relation may point to a predeclared term, but it
// may not smuggle a future ordinal that only becomes valid after a later mint.
func staticExistingTerm(w *Collector, term Term) bool {
	if w == nil || !mutationReady(w) {
		return false
	}
	if !validTermInCounts(w, term) {
		if w.err == nil && !w.terminal {
			w.fail(fmt.Errorf("program/lower/collector: Static child term %d is not already present", term))
		}
		return false
	}
	return true
}

func staticExistingNode(w *Collector, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.Node(w.counts, term) {
		w.fail(fmt.Errorf("program/lower/collector: Static term %d is not a type node", term))
		return false
	}
	return true
}

func staticExistingNodeTerms(w *Collector, terms []Term) bool {
	for _, term := range terms {
		if !staticExistingNode(w, term) {
			return false
		}
	}
	return true
}

func staticExistingOptionalNode(w *Collector, term Term) bool {
	return term == 0 || staticExistingNode(w, term)
}

func staticExistingScope(w *Collector, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.ScopeHandle(w.counts, term) {
		w.fail(fmt.Errorf("program/lower/collector: Static term %d is not a scope handle", term))
		return false
	}
	return true
}

func staticExistingReferenceTarget(w *Collector, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.TypeReferenceTarget(w.counts, term) {
		w.fail(fmt.Errorf("program/lower/collector: Static term %d is not a TypeRef target", term))
		return false
	}
	return true
}

func staticExistingTypeParameterOwner(w *Collector, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.TypeParameterOwner(w.counts, term) {
		w.fail(fmt.Errorf("program/lower/collector: Static term %d is not a TypeParam owner", term))
		return false
	}
	return true
}

func staticExistingAnnotationTarget(w *Collector, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.AnnotationTarget(w.counts, term) {
		w.fail(fmt.Errorf("program/lower/collector: Static term %d is not an Annotation target", term))
		return false
	}
	return true
}

func staticExistingFamily(w *Collector, term Term, family keyspace.Family) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if keyspace.TermFamily(term) != family {
		rejectMutationf(w, "program/lower/collector: Static term %d has wrong family", term)
		return false
	}
	return true
}

func staticExistingOptional(w *Collector, term Term) bool {
	return term == 0 || staticExistingTerm(w, term)
}

func staticExistingFamilyTerms(w *Collector, terms []Term, family keyspace.Family) bool {
	for _, term := range terms {
		if !staticExistingFamily(w, term, family) {
			return false
		}
	}
	return true
}

func staticTypeFunctionForScope(w *Collector, signature, scope Term) bool {
	if !staticExistingFamily(w, signature, keyspace.FamilyTypeFunction) {
		return false
	}
	rowScope, ok := w.static.TypeFunctionScope(signature)
	if !ok {
		w.fail(fmt.Errorf("program/lower/collector: TypeFunction %d row is absent", signature))
		return false
	}
	if rowScope != scope {
		w.fail(fmt.Errorf("program/lower/collector: TypeFunction %d has foreign interface scope", signature))
		return false
	}
	return true
}

func staticPublicationAdmission(w *Collector, assign Term, pair uint32, target Term) bool {
	if !staticExistingFamily(w, assign, keyspace.FamilyAssign) || !staticExistingFamily(w, target, keyspace.FamilyTypeRef) {
		return false
	}
	resolution, ok := w.static.ReferenceResolution(target)
	if !ok {
		w.fail(fmt.Errorf("program/lower/collector: publication TypeRef %d row is absent", target))
		return false
	}
	if resolution != programstatic.TypeRefDeclaration && resolution != programstatic.TypeRefCanonicalPath {
		w.fail(fmt.Errorf("program/lower/collector: publication target %d is unresolved", target))
		return false
	}
	if w.static.PublicationExists(assign, pair) {
		w.fail(fmt.Errorf("program/lower/collector: duplicate publication Assign %d pair %d", assign, pair))
		return false
	}
	return true
}

// staticTypeParamsForOwner closes a generic-child edge against the authored
// TypeParam declaration row. A family/count check alone permits a parameter
// belonging to another alias/signature/function and permits duplicate claims
// in one generic list; both would otherwise survive until Static freeze.
func staticTypeParamsForOwner(w *Collector, owner Term, params []Term) bool {
	if w == nil {
		return false
	}
	seen := make(map[Term]struct{}, len(params))
	for _, param := range params {
		if !staticExistingFamily(w, param, keyspace.FamilyTypeParam) {
			return false
		}
		paramOwner, ok := w.static.TypeParameterOwner(param)
		if !ok {
			w.fail(fmt.Errorf("program/lower/collector: TypeParam %d row is absent", param))
			return false
		}
		if paramOwner != owner {
			w.fail(fmt.Errorf("program/lower/collector: TypeParam %d belongs to another owner", param))
			return false
		}
		if _, duplicate := seen[param]; duplicate {
			w.fail(fmt.Errorf("program/lower/collector: TypeParam %d claimed more than once", param))
			return false
		}
		seen[param] = struct{}{}
	}
	return true
}

// Keep the conversion local to this construction API. The public method
// accepts float64, while Static's authored row retains exact IEEE bits.
func mathFloatBits(value float64) uint64 {
	return math.Float64bits(value)
}
