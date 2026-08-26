// Package programsupply records the exact canonical Program relations supplied
// by each closed Program, static, and binder law requirement.
//
//go:generate go run ./cmd/generate -out evidence_gen.go
package programsupply

import (
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/binder"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/programlaw"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/staticlaw"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// ProgramLawRow binds one Program law to its complete positive terminal vector.
type ProgramLawRow struct {
	Requirement programlaw.Requirement
	Terminals   []schema.EntryID
}

// StaticLawRow binds one static constructor family to its complete positive
// terminal vector.
type StaticLawRow struct {
	Family    staticlaw.Family
	Terminals []schema.EntryID
}

// BinderRow binds one binder transition to exactly one polarity. Positive and
// Forbidden are deliberately separate typed vectors, not a generic input union.
type BinderRow struct {
	Requirement binder.Requirement
	Positive    []schema.EntryID
	Forbidden   []schema.EntryID
}

// Evidence is the generated terminal-only supply denominator. Relation owner,
// form, and parent closure remain derived exclusively from the canonical schema.
type Evidence struct {
	Digest      string
	ProgramLaws []ProgramLawRow
	StaticLaws  []StaticLawRow
	BinderLaws  []BinderRow
}

// Output is one schema-derived member of a terminal's transitive parent closure.
type Output struct {
	Relation schema.EntryID
	Owner    denominator.RelationOwner
	Form     denominator.RelationForm
}

// Generated is the checked-in immutable Program-supply evidence value.
