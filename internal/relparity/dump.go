package relparity

import (
	"strconv"
	"strings"
)

// Row is one observed fact: an accessor path and the value the side published
// under it. The accessor path is the stable key the comparison is addressed
// by, and Occurrence orders the rows that repeat one accessor within a single
// (fixture, verb) dump.
//
// Keying by accessor rather than by line position is what makes a divergence
// report readable: an inserted or removed row names itself, instead of
// shifting every following line into a false difference.
type Row struct {
	Fixture    string    `json:"fixture"`
	Verb       string    `json:"verb"`
	Key        string    `json:"key"`
	Occurrence int       `json:"occurrence"`
	Value      string    `json:"value"`
	Dimension  Dimension `json:"dimension"`
	Index      int       `json:"index"`
}

// Dimension labels which parity face a row speaks to. It is a report facet
// only: Compare examines every row whatever its dimension, including
// DimensionUnclassified, so a vocabulary this table has never seen still
// diverges loudly rather than silently.
type Dimension string

// The closed facet vocabulary of the Wave 4C comparison.
const (
	DimensionKey          Dimension = "key"
	DimensionScope        Dimension = "scope"
	DimensionValue        Dimension = "value"
	DimensionOutcome      Dimension = "outcome"
	DimensionDiagnostic   Dimension = "diagnostic"
	DimensionLineage      Dimension = "lineage"
	DimensionProcess      Dimension = "process"
	DimensionUnclassified Dimension = "unclassified"
)

// DimensionRule assigns one dimension to accessor paths it recognises.
// Contains and Prefix match the whole path; Terminal and Suffix match the
// final accessor segment, exactly and by ending.
type DimensionRule struct {
	Dimension Dimension
	Contains  []string
	Prefix    []string
	Terminal  []string
	Suffix    []string
}

// DimensionRules is the declared facet table, in resolution order, measured
// against the baseline dump vocabulary. It is exported so a later wave extends
// it with the replacement's own accessors rather than reimplementing
// classification.
var DimensionRules = []DimensionRule{
	{
		Dimension: DimensionLineage,
		Contains:  []string{"Provenance", "Support", "Derivation", ".Program.", ".Fold.", ".Candidate."},
		Prefix:    []string{"why"},
	},
	{
		Dimension: DimensionDiagnostic,
		Contains:  []string{"Finding", "Diagnostics"},
		Prefix:    []string{"report."},
		Terminal:  []string{"Code", "Message", "Render"},
	},
	{
		Dimension: DimensionScope,
		Contains:  []string{"transition", "Transition", "Transport", "Region", "Scope"},
		Terminal: []string{"Lane", "Owner", "Class", "Axis", "Exported", "Population",
			"AxisLifetime", "AxisStorage", "AxisMountDeclared", "AxisCardinality"},
	},
	{
		Dimension: DimensionOutcome,
		Contains:  []string{"Refusal", "Admit", "Failure"},
		Prefix:    []string{"unexposed."},
		Terminal: []string{"Written", "Present", "Status", "Available", "Exact", "Trust", "Kind", "State", "Phase", "Reason",
			"CompileComplete", "AnalyzeComplete", "StateFinal"},
	},
	{
		Dimension: DimensionKey,
		Terminal: []string{"ID", "SiteID", "ContentID", "ContractID", "SourceID", "Key", "Digest", "Member", "Family", "Rule", "Writes",
			"Fixture", "Name", "Anchor", "OperationAt", "OperationContentID", "ProtocolAt", "StateName"},
	},
	{
		Dimension: DimensionValue,
		Contains:  []string{"Column", "Layout", "Cell", "View"},
		Terminal: []string{"Flag", "Carrier", "Codec", "Projection", "Value", "Payload", "Width",
			"Semantic", "Declaration", "ExternalFormals", "WordAt"},
		// A cardinality accessor is a published value whatever it counts.
		Suffix: []string{"Count"},
	},
}

// Classify labels one accessor path.
func Classify(key string) Dimension {
	terminal := terminalSegment(key)
	for _, rule := range DimensionRules {
		for _, needle := range rule.Contains {
			if strings.Contains(key, needle) {
				return rule.Dimension
			}
		}
		for _, prefix := range rule.Prefix {
			if strings.HasPrefix(key, prefix) {
				return rule.Dimension
			}
		}
		for _, name := range rule.Terminal {
			if terminal == name {
				return rule.Dimension
			}
		}
		for _, ending := range rule.Suffix {
			if strings.HasSuffix(terminal, ending) {
				return rule.Dimension
			}
		}
	}
	return DimensionUnclassified
}

// terminalSegment is the last dotted accessor segment with its call and index
// syntax removed, so FamilyAt(3).QueryAt(0).Cell.Present reduces to Present
// and result.FamilyAt(3).RootAt(1) reduces to RootAt.
func terminalSegment(key string) string {
	segment := key
	if index := strings.LastIndex(segment, "."); index >= 0 {
		segment = segment[index+1:]
	}
	if index := strings.IndexAny(segment, "(["); index >= 0 {
		segment = segment[:index]
	}
	return segment
}

// ParseDump turns one side's dump of a (fixture, verb) into rows. Each line is
// an accessor path, an equals sign, and the published value; a line with no
// equals sign is a bare accessor and carries an empty value. Blank lines carry
// nothing and are dropped.
func ParseDump(fixture, verb, dump string) []Row {
	occurrences := make(map[string]int)
	var rows []Row
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value := line, ""
		if index := strings.Index(line, "="); index >= 0 {
			key, value = line[:index], line[index+1:]
		}
		row := Row{
			Fixture:    fixture,
			Verb:       verb,
			Key:        key,
			Occurrence: occurrences[key],
			Value:      value,
			Dimension:  Classify(key),
			Index:      len(rows),
		}
		occurrences[key]++
		rows = append(rows, row)
	}
	return rows
}

// Address is the comparison key of a row: the accessor path plus the ordinal
// of this repetition of it inside the dump.
func (row Row) Address() string {
	return row.Key + "\x00" + strconv.Itoa(row.Occurrence)
}
