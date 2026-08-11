// Package programsupply records the exact canonical Program relations supplied
// by each closed Program, static, and binder law requirement.
package programsupply

import (
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/binder"
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/programlaw"
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/staticlaw"
	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// Reference is one exact issued relation token without a string catalog name.
type Reference struct {
	Origin   semanticsource.Origin
	Facet    semanticsource.Facet
	Revision semanticsource.Revision
}

// ProgramLawRow binds one Program law to its complete positive terminal vector.
type ProgramLawRow struct {
	Requirement programlaw.Requirement
	Terminals   []Reference
}

// StaticLawRow binds one static constructor family to its complete positive
// terminal vector.
type StaticLawRow struct {
	Family    staticlaw.Family
	Terminals []Reference
}

// BinderRow binds one binder transition to exactly one polarity. Positive and
// Forbidden are deliberately separate typed vectors, not a generic input union.
type BinderRow struct {
	Requirement binder.Requirement
	Positive    []Reference
	Forbidden   []Reference
}

// Evidence is the generated terminal-only supply denominator. Relation owner,
// form, and parent closure remain derived exclusively from the canonical schema.
type Evidence struct {
	SchemaDigest string
	Digest       string
	ProgramLaws  []ProgramLawRow
	StaticLaws   []StaticLawRow
	BinderLaws   []BinderRow
}

// Output is one schema-derived member of a terminal's transitive parent closure.
type Output struct {
	Relation Reference
	Owner    relations.Owner
	Form     relations.Form
}

// Generated is assigned by the checked-in generated evidence source.
var Generated Evidence
