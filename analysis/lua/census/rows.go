package census

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/grammar"
)

// RowKind is the grain one census row is stated at. The three grains are the
// three ways the language can grow: a new parser alternative, a new AST form,
// and a new carrier on an existing form. A denominator that dropped any of them
// would let one of those three changes enter unaccounted.
type RowKind uint8

const (
	RowInvalid RowKind = iota
	RowProduction
	RowForm
	RowCarrier
)

// Row is one census row in the neutral vocabulary a seal-time disposition join
// consumes. It carries the containment edges that join needs and nothing else:
// the parser and AST types this package reads from source stay inside it.
type Row struct {
	// Key is the stable row identity, prefixed by grain.
	Key  string
	Kind RowKind
	// Builds is, for a production row, the form row keys its reduction
	// constructs.
	Builds []string
	// Owner is, for a carrier row, the form row key that declares it.
	Owner string
	// Coordinate marks a carrier that holds a source position rather than a
	// parsed value.
	Coordinate bool
	// Component marks a form the AST itself classifies as structural, so it
	// reaches the analyzer only inside the form declaring it.
	Component bool
}

// ProductionRow, FormRow, and CarrierRow are the row key spellings. They are
// functions rather than a format left to each caller so that one prefix change
// cannot silently split the key space in two.
func ProductionRow(key string) string { return "production:" + key }

func FormRow(name string) string { return "form:" + name }

func CarrierRow(form, field string) string { return "carrier:" + form + "." + field }

// coordinateTypes are the compiler's source-coordinate aliases. A carrier of
// one of these holds a position in the source text, not a parsed value, so no
// rule can own it and the join states that rather than leaving it to a caller's
// name heuristic.
var coordinateTypes = map[string]bool{"Position": true, "[]Position": true}

// Projection is the census as a seal-time join consumes it: the complete row
// denominator plus the census identity that produced it. The identity travels
// with the rows because a row key alone cannot express an action rewritten in
// place, and a join that pins only the row set would close over a parser it no
// longer describes.
type Projection struct {
	Digest string
	Rows   []Row
}

// Project derives the complete projection of a census value.
func Project(value Census) Projection {
	return Projection{Digest: value.Digest, Rows: rows(value)}
}

// rows projects a census into its complete row set, sorted by key so the result
// is a stable denominator rather than a map walk.
func rows(value Census) []Row {
	result := make([]Row, 0, len(value.Productions)+len(value.Constructors))
	for _, production := range value.Productions {
		builds := make([]string, 0, len(production.Constructors))
		for _, name := range production.Constructors {
			builds = append(builds, FormRow(name))
		}
		result = append(result, Row{Key: ProductionRow(production.Key), Kind: RowProduction, Builds: builds})
	}
	for _, constructor := range value.Constructors {
		result = append(result, Row{
			Key:       FormRow(constructor.Name),
			Kind:      RowForm,
			Component: constructor.Class == grammar.ConstructorStructural,
		})
		for _, field := range constructor.Fields {
			result = append(result, Row{
				Key:        CarrierRow(constructor.Name, field.Name),
				Kind:       RowCarrier,
				Owner:      FormRow(constructor.Name),
				Coordinate: coordinateTypes[field.Type],
			})
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	return result
}

// CurrentProjection validates the checked-in census against the current parser
// and AST source and projects it. It is the one call a seal-time join needs: a
// stale census fails here rather than joining against yesterday's language.
func CurrentProjection(root string) (Projection, error) {
	value, err := Current(root)
	if err != nil {
		return Projection{}, err
	}
	projection := Project(value)
	if len(projection.Rows) == 0 || projection.Digest == "" {
		return Projection{}, fmt.Errorf("parser census: projection is empty")
	}
	return projection, nil
}
