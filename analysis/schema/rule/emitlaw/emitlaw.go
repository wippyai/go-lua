// Package emitlaw projects one rule's Program declaration into the structural
// half of its law suite.
//
// Every cutover that lands a family hand-writes the same laws: that the
// declaration is well formed, that it agrees with the call shape of the
// reducer it names, that its geometry is the geometry the owner intended, that
// its rule entry hands out its issuance defensively, and that a handful of
// malformed edits are refused. None of that is knowledge about the fold. All
// of it is a function of the declaration, so all of it is emitted here and a
// family keeps only the laws that say what its fold MEANS.
//
// The emitted suite is not a restatement of the declaration compared against
// itself. Its geometry law is stated over a single canonical rendering that is
// checked in and held to the declaration by the freshness law, so a
// declaration that moves without regeneration fails the build and one that
// moves with it is a reviewable diff - the same affordance the authored
// field-by-field law bought, over every field rather than a sample. Its
// mutation law runs Check for real at test time against the verdict this
// emitter observed while deriving, and generation refuses outright when a term
// the declaration cannot do without is admitted after removal.
//
// This emitter consumes the declaration, the rule entry it is sealed under,
// and the axis member roster. It reaches no engine, no composition, and no
// sealed ordinal: what a plan compiler assigns is not knowledge a declaration
// law may hold.
package emitlaw

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"os"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule"
)

// The package aliases the emitted source spells its vocabulary under. They are
// named here because both halves of a mutation row - the Go statement and the
// function that applies it - have to agree on them.
const (
	programPackage = "ruleprogram"
	memberPackage  = "axismember"
	catalogPackage = "axiscatalog"
	emitlawPackage = "emitlaw"

	programPackagePath = "github.com/wippyai/go-lua/analysis/schema/rule/program"
	memberPackagePath  = "github.com/wippyai/go-lua/analysis/schema/axis/member"
	schemaPackagePath  = "github.com/wippyai/go-lua/analysis/schema"
	emitlawPackagePath = "github.com/wippyai/go-lua/analysis/schema/rule/emitlaw"

	// axisCatalogAccessor is the member generator's published accessor for an
	// axis's declaration-only member vocabulary. One name serves every axis
	// because the name is not an owner's choice: the generator writes it into
	// the axis package it generates.
	axisCatalogAccessor = "AxisMemberCatalog"
)

func zeroCandidate() member.CandidateRef { return member.CandidateRef{} }

// Unexpressible is this emitter's one refusal. It names the rule, the clause
// of the declaration that has no emitted law, and why - so a declaration whose
// structural suite cannot be stated is a named gap rather than a family that
// quietly keeps hand-writing it.
type Unexpressible struct {
	Rule   schema.Key
	Clause string
	Detail string
}

func (refusal Unexpressible) Error() string {
	return fmt.Sprintf("rule law emitter: rule %q declares %s: %s",
		string(refusal.Rule), refusal.Clause, refusal.Detail)
}

func unexpressible(ruleKey schema.Key, clause, detail string) error {
	return Unexpressible{Rule: ruleKey, Clause: clause, Detail: detail}
}

// Target names the declaration package one rule's structural law suite is
// emitted into, and the two accessors that package publishes.
//
// The accessor names are declared rather than derived. How a package spells
// the function that hands out its declaration is that package's own statement,
// exactly as the bind arm is in the family emitter, and a generator that
// guessed it would be holding a naming convention as if it were a contract.
type Target struct {
	PackagePath string
	PackageName string
	// Declaration is the exported nullary accessor returning the Program.
	Declaration string
	// Entry is the exported nullary accessor returning the rule.Spec the
	// declaration is sealed under.
	Entry string
	Spec  rule.Spec
}

func (target Target) available() bool {
	return target.PackagePath != "" && token.IsIdentifier(target.PackageName) &&
		token.IsIdentifier(target.Declaration) && token.IsIdentifier(target.Entry) &&
		target.Spec.Key.Available()
}

// Render projects one rule declaration into its structural law suite source.
func Render(target Target, roster definition.Roster) ([]byte, error) {
	if !target.available() {
		return nil, errors.New("rule law emitter: target package, declaration accessor, entry accessor, and rule declaration are required")
	}
	built, err := derive(target, roster)
	if err != nil {
		return nil, err
	}
	source, err := render(built)
	if err != nil {
		return nil, err
	}
	formatted, formatErr := format.Source(source)
	if formatErr != nil {
		return nil, fmt.Errorf("rule law emitter: rule %q: format generated source: %w", string(target.Spec.Key), formatErr)
	}
	return formatted, nil
}

// Generate writes one rule's structural law suite, or proves the checked-in
// file is already the one the declaration derives when check is true.
func Generate(target Target, roster definition.Roster, path string, check bool) error {
	if path == "" {
		return errors.New("rule law emitter: output path is required")
	}
	source, err := Render(target, roster)
	if err != nil {
		return err
	}
	if check {
		return fresh(path, source)
	}
	if err := os.WriteFile(path, source, 0o644); err != nil {
		return fmt.Errorf("rule law emitter: write %s: %w", path, err)
	}
	return nil
}

func fresh(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("rule law emitter: read %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("rule law emitter: stale generated law suite: %s", path)
	}
	return nil
}

// suite is one rule's derived law plan.
type suite struct {
	target Target
	// symbol is the subject every emitted law name is spelled with.
	symbol string
	// geometry and identity are the canonical renderings the emitted laws hold
	// the declaration and its rule entry to.
	geometry string
	identity string
	// reducerAxisPackage is the import path of the axis package publishing the
	// member catalog the declared reducer is resolved through.
	reducerAxisPackage string
	reducerKey         schema.Key
	verdicts           []verdict
	mutable            bool
}

func (built suite) refusals() []verdict {
	rows := make([]verdict, 0, len(built.verdicts))
	for _, row := range built.verdicts {
		if row.refused {
			rows = append(rows, row)
		}
	}
	return rows
}

func (built suite) admissions() []verdict {
	rows := make([]verdict, 0, len(built.verdicts))
	for _, row := range built.verdicts {
		if !row.refused {
			rows = append(rows, row)
		}
	}
	return rows
}

// derive resolves one rule declaration against the axis member roster into the
// law plan. Every refusal names the declared clause that has no emitted law.
func derive(target Target, roster definition.Roster) (*suite, error) {
	ruleKey := target.Spec.Key
	declaration := target.Spec.Program
	if !declaration.Available() {
		return nil, unexpressible(ruleKey, "no Program at all",
			"a rule whose execution declaration is still the zero value has no structural law suite to emit")
	}
	if problem, valid := declaration.Check(); !valid {
		return nil, unexpressible(ruleKey, "a malformed Program",
			fmt.Sprintf("Check refuses the declaration this suite would be emitted from: %s at join %d, input %d, output %d",
				problemKindName(problem.Kind), problem.Join, problem.Input, problem.Output))
	}
	// Check has already sealed that the fold names an available reducer, so the
	// axis the call-shape law resolves it through is the one named here.
	reducer := declaration.Fold.Reducer
	axisPackage, err := reducerAxisPackage(ruleKey, roster, reducer.Axis.Key)
	if err != nil {
		return nil, err
	}
	verdicts, err := observe(ruleKey, declaration)
	if err != nil {
		return nil, err
	}
	return &suite{
		target:             target,
		symbol:             target.Declaration,
		geometry:           Canonical(declaration),
		identity:           CanonicalEntry(target.Spec),
		reducerAxisPackage: axisPackage,
		reducerKey:         reducer.Member,
		verdicts:           verdicts,
		mutable:            len(declaration.Joins) != 0 || len(target.Spec.Issues) != 0,
	}, nil
}

// reducerAxisPackage resolves the Go package the reducer's axis publishes its
// member catalog from. It is the axis's own declared import path, which is
// where the member generator writes that catalog. It is not read off a
// carrier: an axis whose fact type lives in a package of its own - because
// that type reaches a dependency the cold catalog's importers must not - would
// hand this a package with no catalog in it.
func reducerAxisPackage(ruleKey schema.Key, roster definition.Roster, axisKey schema.Key) (string, error) {
	for index := 0; index < roster.Count(); index++ {
		source, sourceOK := roster.At(index)
		if !sourceOK {
			return "", unexpressible(ruleKey, "a member roster that does not enumerate", fmt.Sprintf("source %d", index))
		}
		composed, composedOK := source.Compose()
		if !composedOK || composed.Axis != axisKey {
			continue
		}
		cold, coldOK := definition.ColdImportPath(composed)
		if !coldOK {
			return "", unexpressible(ruleKey, "a reducer axis that names no package",
				fmt.Sprintf("axis %q declares no import path, so its member catalog cannot be spelled", string(axisKey)))
		}
		return cold, nil
	}
	return "", unexpressible(ruleKey, "a reducer on an axis the roster does not carry", string(axisKey))
}
