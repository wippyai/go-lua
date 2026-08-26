// Package emit projects one rule's Program declaration into the Go source of
// its execution family: the installer that seals every primitive through the
// plane, the family that holds the sealed rows, and the worker that performs
// one invocation.
//
// The emitted source is the whole of a rule's execution choreography. What an
// owner still authors beside it is its semantic reducer, the relation
// derivations its joins declare, and the bind arm that hands its axis schemas
// to the constructor emitted here. Nothing about reads, writes, scratch lanes,
// folds, dispositions, or plan-row geometry is authored twice, because all of
// it is a function of the declaration.
//
// The emitter consumes the declaration and the axis member roster and nothing
// else. It never opens a sealed schema: the emitted installer resolves every
// dense address off the plan row it is handed at install time, so generation
// needs the Go types and owner symbols an axis declares, not the ordinals a
// composition seals.
package emit

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"os"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule"
)

// The execution vocabulary the emitted source is typed against. It is named
// here rather than by an axis or a rule for the same reason the reduction
// outcome is: there is one execution layer in the analyzer, and a declaration
// that hand-spelled its own would be a second authority over the primitives
// every family is sealed through.
const (
	executionPackagePath = "github.com/wippyai/go-lua/analysis/engine/execution"
	structurePackagePath = "github.com/wippyai/go-lua/analysis/schema/structure"
	programPackagePath   = "github.com/wippyai/go-lua/analysis/schema/rule/program"
	productPackagePath   = "github.com/wippyai/go-lua/analysis/engine/execution/product"

	// denseCoordinateType is the NAME of an axis's generator-published dense
	// Factor coordinate. One name serves every axis because the name is not an
	// owner's choice; WHICH package publishes it is, and the axis's own
	// definition states that beside where its relation owner is generated.
	denseCoordinateType = "DenseCoordinate"
)

// outcomeType is the sealed disposition every emitted fold concludes with.
func outcomeType() definition.GoType {
	return definition.GoType{PackagePath: structurePackagePath, Name: "ReductionOutcome"}
}

// vectorType and cellType are the two views a many-valued delivery is handed
// through, whether a fold or a relation derivation consumes it. Which of them
// one input takes is definition.ManyValuedView's answer from that input's
// declared read Form; this package only names the types, because they belong
// to the execution vocabulary.
func vectorType() definition.GoType {
	return definition.GoType{PackagePath: executionPackagePath, Name: "SummaryVector"}
}

func cellType() definition.GoType {
	return definition.GoType{PackagePath: executionPackagePath, Name: "SelectedCell"}
}

// Unexpressible is the emitter's one refusal. It names the rule, the clause of
// the declaration that has no emitted form, and why - so a shape the generator
// cannot yet express is a named gap in the execution vocabulary rather than a
// silent omission or a hand-written family that quietly reappears.
type Unexpressible struct {
	Rule   schema.Key
	Clause string
	Detail string
}

func (refusal Unexpressible) Error() string {
	return fmt.Sprintf("rule family emitter: rule %q declares %s, which has no emitted form: %s",
		string(refusal.Rule), refusal.Clause, refusal.Detail)
}

func unexpressible(ruleKey schema.Key, clause, detail string) error {
	return Unexpressible{Rule: ruleKey, Clause: clause, Detail: detail}
}

// Target names the package one rule's family is emitted into. The import path
// is load-bearing and not cosmetic: a symbol the declaration names in this
// same package is spelled unqualified, and one from anywhere else is imported.
type Target struct {
	PackagePath string
	PackageName string
	Spec        rule.Spec
}

func (target Target) available() bool {
	return target.PackagePath != "" && token.IsIdentifier(target.PackageName) && target.Spec.Key.Available()
}

// Render projects one rule declaration into its execution family source.
func Render(target Target, roster definition.Roster) ([]byte, error) {
	if !target.available() {
		return nil, errors.New("rule family emitter: target package and rule declaration are required")
	}
	plan, err := derive(target, roster)
	if err != nil {
		return nil, err
	}
	source, err := render(plan)
	if err != nil {
		return nil, err
	}
	formatted, formatErr := format.Source(source)
	if formatErr != nil {
		return nil, fmt.Errorf("rule family emitter: rule %q: format generated source: %w", string(target.Spec.Key), formatErr)
	}
	return formatted, nil
}

// Generate writes one rule's emitted family, or proves the checked-in file is
// already the one the declaration derives when check is true.
func Generate(target Target, roster definition.Roster, path string, check bool) error {
	if path == "" {
		return errors.New("rule family emitter: output path is required")
	}
	source, err := Render(target, roster)
	if err != nil {
		return err
	}
	if check {
		return fresh(path, source)
	}
	if err := os.WriteFile(path, source, 0o644); err != nil {
		return fmt.Errorf("rule family emitter: write %s: %w", path, err)
	}
	return nil
}

func fresh(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("rule family emitter: read %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("rule family emitter: stale generated family: %s", path)
	}
	return nil
}
