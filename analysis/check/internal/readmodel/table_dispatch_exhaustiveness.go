package readmodel

import (
	"sort"

	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type TableDispatchExhaustiveness = readapi.TableDispatchExhaustiveness

type tableDispatchLookup struct {
	point        cfg.Point
	span         SourceSpan
	table        path.Path
	discriminant path.Path
}

// ForEachTableDispatchExhaustiveness visits dispatch-table lookups whose table
// is missing one or more static keys for the discriminated union being indexed.
func (r Reader) ForEachTableDispatchExhaustiveness(visit func(TableDispatchExhaustiveness) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range cfg.RPOReadOnly(r.result.Graph()) {
		for _, lookup := range r.tableDispatchLookupsAt(point) {
			item, ok := r.tableDispatchExhaustiveness(lookup)
			if !ok {
				continue
			}
			visited = true
			if !visit(item) {
				return true
			}
		}
	}
	return visited
}

func (r Reader) tableDispatchLookupsAt(point cfg.Point) []tableDispatchLookup {
	lookups := r.result.IndexedPathLookupsAt(point)
	out := make([]tableDispatchLookup, 0, len(lookups))
	for _, lookup := range lookups {
		if lookup.Container.Symbol == 0 || lookup.Key.Symbol == 0 || len(lookup.Key.Segments) == 0 {
			continue
		}
		out = append(out, tableDispatchLookup{
			point:        lookup.Point,
			span:         sourceSpanFromBody(lookup.Span),
			table:        lookup.Container,
			discriminant: lookup.Key,
		})
	}
	return out
}

func (r Reader) tableDispatchExhaustiveness(lookup tableDispatchLookup) (TableDispatchExhaustiveness, bool) {
	cases, ok := r.registrationStringDiscriminantCases(lookup.point, lookup.discriminant)
	if !ok {
		return TableDispatchExhaustiveness{}, false
	}
	keys, tableSpan, ok := r.tableDispatchKeysAt(lookup.point, lookup.table)
	if !ok {
		return TableDispatchExhaustiveness{}, false
	}
	var possible []string
	var presentKeys []string
	var missingCases []string
	var missingKeys []string
	for _, c := range cases {
		possible = append(possible, c.name)
		if keys[c.key] {
			presentKeys = append(presentKeys, registrationCaseName(lookup.table.String(), c.key))
			continue
		}
		missingCases = append(missingCases, c.name)
		missingKeys = append(missingKeys, registrationCaseName(lookup.table.String(), c.key))
	}
	if len(missingKeys) == 0 {
		return TableDispatchExhaustiveness{}, false
	}
	sort.Strings(presentKeys)
	return TableDispatchExhaustiveness{
		Point:      lookup.point,
		Table:      lookup.table.String(),
		Target:     lookup.discriminant.String(),
		Possible:   possible,
		Keys:       presentKeys,
		Missing:    missingKeys,
		MissingFor: missingCases,
		TableSpan:  tableSpan,
		LookupSpan: lookup.span,
	}, true
}
