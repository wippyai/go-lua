package render

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// row is one printed metric line: a name plus one formatted cell per
// labeled snapshot, with the delta cell (last-first) appended by
// FormatTable when there is more than one snapshot.
type row struct {
	name  string
	cells []string
	delta string
}

// FormatTable renders one plain-text table: a metric column, one column
// per labeled snapshot in the given order, and - when more than one
// snapshot is given - a trailing delta column from the first to the last.
//
// Every "LOC (nt/t)" row reports authored lines only (measure.LOC.NonTest
// and .Test already exclude generated_*.go, rule_members.go, and files
// carrying an emitter's "Code generated" header); the paired "generated
// LOC (nt/t)" row next to the domain total reports what was excluded.
func FormatTable(labels []Labeled) string {
	if len(labels) == 0 {
		return ""
	}

	var rows []row
	for _, name := range unionAreaNames(labels) {
		name := name
		rows = append(rows, locRow("domain/"+name+" LOC (nt/t)", labels, func(r Report) LOC {
			return areaLOC(r, name)
		}))
	}
	rows = append(rows, locRow("domain/ TOTAL LOC (nt/t)", labels, func(r Report) LOC { return r.DomainTotal }))
	rows = append(rows, pairRow("domain/ TOTAL generated LOC (nt/t)", labels,
		func(r Report) int { return r.DomainTotal.GeneratedNonTest },
		func(r Report) int { return r.DomainTotal.GeneratedTest }))
	rows = append(rows, locRow("analysis/engine LOC (nt/t)", labels, func(r Report) LOC { return r.EngineLOC }))
	rows = append(rows, locRow("analysis/schema LOC (nt/t)", labels, func(r Report) LOC { return r.SchemaLOC }))
	rows = append(rows, locRow("analysis/(rest) LOC (nt/t)", labels, func(r Report) LOC { return r.AnalysisRest }))
	rows = append(rows, locRow("internal/ LOC (nt/t)", labels, func(r Report) LOC { return r.InternalLOC }))
	rows = append(rows, locRow("cmd/ LOC (nt/t)", labels, func(r Report) LOC { return r.CmdLOC }))

	rows = append(rows, pairRow("generated files/LOC (domain+analysis)", labels,
		func(r Report) int { return r.GeneratedFiles },
		func(r Report) int { return r.GeneratedLOC }))
	rows = append(rows, pairRow("legacy residue files/occurrences (domain, non-test)", labels,
		func(r Report) int { return r.ResidueFiles },
		func(r Report) int { return r.ResidueOccurrences }))

	rows = append(rows, countRow("family.go count (domain)", labels, func(r Report) int { return r.FamilyFiles }))
	rows = append(rows, countRow("hot_rule.go count (domain)", labels, func(r Report) int { return r.HotRuleFiles }))
	rows = append(rows, countRow("registration.go count (domain)", labels, func(r Report) int { return r.RegistrationFiles }))
	rows = append(rows, countRow("schema_fragment.go count (domain)", labels, func(r Report) int { return r.SchemaFragmentFiles }))
	rows = append(rows, countRow("scheduled_death.go ledger rows", labels, func(r Report) int { return r.ScheduledDeathRows }))

	rows = append(rows, pairRow("rule_templates.go generated/legacy wiring", labels,
		func(r Report) int { return r.RuleTemplatesGenerated },
		func(r Report) int { return r.RuleTemplatesLegacy }))

	rows = append(rows, countRow("emitted (Code generated) files under domain/", labels, func(r Report) int { return r.EmittedDomainFiles }))
	rows = append(rows, countRow("total func Test*", labels, func(r Report) int { return r.TotalTestFuncs }))
	rows = append(rows, countRow("func Test*Law*", labels, func(r Report) int { return r.LawTestFuncs }))
	rows = append(rows, countRow("*_law_test.go files", labels, func(r Report) int { return r.LawTestFiles }))
	rows = append(rows, countRow("exported symbols analysis/engine", labels, func(r Report) int { return r.ExportedEngine }))
	rows = append(rows, countRow("exported symbols analysis/schema", labels, func(r Report) int { return r.ExportedSchema }))

	return renderRows(labels, rows)
}

func areaLOC(r Report, name string) LOC {
	for _, a := range r.DomainAreas {
		if a.Name == name {
			return a.LOC
		}
	}
	return LOC{}
}

func unionAreaNames(labels []Labeled) []string {
	set := map[string]struct{}{}
	for _, l := range labels {
		for _, a := range l.Report.DomainAreas {
			set[a.Name] = struct{}{}
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func locRow(name string, labels []Labeled, get func(Report) LOC) row {
	cells := make([]string, 0, len(labels))
	var first, last LOC
	for i, l := range labels {
		v := get(l.Report)
		if i == 0 {
			first = v
		}
		last = v
		cells = append(cells, fmt.Sprintf("%d/%d", v.NonTest, v.Test))
	}
	delta := ""
	if len(labels) > 1 {
		delta = fmt.Sprintf("%+d/%+d", last.NonTest-first.NonTest, last.Test-first.Test)
	}
	return row{name: name, cells: cells, delta: delta}
}

func countRow(name string, labels []Labeled, get func(Report) int) row {
	cells := make([]string, 0, len(labels))
	var first, last int
	for i, l := range labels {
		v := get(l.Report)
		if i == 0 {
			first = v
		}
		last = v
		cells = append(cells, strconv.Itoa(v))
	}
	delta := ""
	if len(labels) > 1 {
		delta = fmt.Sprintf("%+d", last-first)
	}
	return row{name: name, cells: cells, delta: delta}
}

func pairRow(name string, labels []Labeled, getA, getB func(Report) int) row {
	cells := make([]string, 0, len(labels))
	var firstA, firstB, lastA, lastB int
	for i, l := range labels {
		a, b := getA(l.Report), getB(l.Report)
		if i == 0 {
			firstA, firstB = a, b
		}
		lastA, lastB = a, b
		cells = append(cells, fmt.Sprintf("%d/%d", a, b))
	}
	delta := ""
	if len(labels) > 1 {
		delta = fmt.Sprintf("%+d/%+d", lastA-firstA, lastB-firstB)
	}
	return row{name: name, cells: cells, delta: delta}
}

func renderRows(labels []Labeled, rows []row) string {
	headers := make([]string, 0, len(labels)+2)
	headers = append(headers, "metric")
	for _, l := range labels {
		headers = append(headers, l.Commit)
	}
	hasDelta := len(labels) > 1
	if hasDelta {
		headers = append(headers, fmt.Sprintf("delta %s->%s", labels[0].Commit, labels[len(labels)-1].Commit))
	}

	table := make([][]string, 0, len(rows)+1)
	table = append(table, headers)
	for _, r := range rows {
		line := make([]string, 0, len(headers))
		line = append(line, r.name)
		line = append(line, r.cells...)
		if hasDelta {
			line = append(line, r.delta)
		}
		table = append(table, line)
	}

	widths := make([]int, len(headers))
	for _, line := range table {
		for i, c := range line {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	var b strings.Builder
	for _, line := range table {
		parts := make([]string, len(line))
		for i, c := range line {
			parts[i] = fmt.Sprintf("%-*s", widths[i], c)
		}
		fmt.Fprintln(&b, strings.TrimRight(strings.Join(parts, "  "), " "))
	}
	return b.String()
}
