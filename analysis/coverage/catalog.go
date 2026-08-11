// Package coverage seals the semantic coverage contract between canonical
// source relations and one sealed analysis composition.  It is cold
// validation only: it never carries a Program value, a domain fact, or a
// runtime equation coordinate.
package coverage

import "github.com/wippyai/go-lua/program/semanticsource"

// SourceCatalog is the complete, immutable source-relation denominator.  It
// is derived from issued definitions plus their sealed owner publications, so
// zero-instance relation families remain visible rather than disappearing
// with a particular fixture.
//
// It deliberately records Tokens rather than Go declarations, package paths,
// names, or source locations.  Semantic relation revisions are therefore the
// only source-vocabulary change that can alter this denominator.
type SourceCatalog struct {
	measures []semanticsource.Measure
	index    map[semanticsource.Token]struct{}
	valid    bool
}

// NewSourceCatalog derives the canonical source denominator from complete
// sealed source publications.  A Schema alone is deliberately insufficient:
// Program, Target, and Link owners must publish every token measure, including
// zero-count families, before analysis coverage can begin.
func NewSourceCatalog(publications semanticsource.Publications) (SourceCatalog, bool) {
	canonical := semanticsource.CatalogSchema()
	if canonical.Count() == 0 || publications.Count() != canonical.Count() {
		return SourceCatalog{}, false
	}
	measures := make([]semanticsource.Measure, publications.Count())
	index := make(map[semanticsource.Token]struct{}, publications.Count())
	for at := range measures {
		measure, measured := publications.At(at)
		definition, defined := canonical.DefinitionAt(at)
		if !measured || !defined || measure.Count() < 0 {
			return SourceCatalog{}, false
		}
		token := measure.Token()
		if !availableToken(token) || token != definition.Token() {
			return SourceCatalog{}, false
		}
		if _, duplicate := index[token]; duplicate || at != 0 && compareToken(measures[at-1].Token(), token) >= 0 {
			return SourceCatalog{}, false
		}
		index[token] = struct{}{}
		measures[at] = measure
	}
	return SourceCatalog{measures: measures, index: index, valid: true}, true
}

// Count includes every issued semantic relation family, including families
// whose corresponding sealed artifact has zero rows in a particular project.
func (catalog SourceCatalog) Count() int {
	if !catalog.valid {
		return 0
	}
	return len(catalog.measures)
}

// TokenAt returns one token in canonical numeric identity order.
func (catalog SourceCatalog) TokenAt(index int) (semanticsource.Token, bool) {
	if !catalog.valid || index < 0 || index >= len(catalog.measures) {
		return semanticsource.Token{}, false
	}
	return catalog.measures[index].Token(), true
}

// MeasureAt returns one source-owned cardinality claim in canonical Token
// order.  Zero is a valid and required count.
func (catalog SourceCatalog) MeasureAt(index int) (semanticsource.Measure, bool) {
	if !catalog.valid || index < 0 || index >= len(catalog.measures) {
		return semanticsource.Measure{}, false
	}
	return catalog.measures[index], true
}

func (catalog SourceCatalog) contains(token semanticsource.Token) bool {
	if !catalog.valid || !availableToken(token) {
		return false
	}
	_, exists := catalog.index[token]
	return exists
}

func cloneCatalog(catalog SourceCatalog) SourceCatalog {
	if !catalog.valid {
		return SourceCatalog{}
	}
	cloned := SourceCatalog{measures: append([]semanticsource.Measure(nil), catalog.measures...), index: make(map[semanticsource.Token]struct{}, len(catalog.index)), valid: true}
	for _, measure := range cloned.measures {
		cloned.index[measure.Token()] = struct{}{}
	}
	return cloned
}

func availableToken(token semanticsource.Token) bool {
	return token.Origin() != 0 && token.Revision() != 0 && token.Digest() != 0
}

func compareToken(left, right semanticsource.Token) int {
	if left.Origin() != right.Origin() {
		if left.Origin() < right.Origin() {
			return -1
		}
		return 1
	}
	if left.Facet() != right.Facet() {
		if left.Facet() < right.Facet() {
			return -1
		}
		return 1
	}
	if left.Revision() != right.Revision() {
		if left.Revision() < right.Revision() {
			return -1
		}
		return 1
	}
	if left.Digest() < right.Digest() {
		return -1
	}
	if left.Digest() > right.Digest() {
		return 1
	}
	return 0
}
