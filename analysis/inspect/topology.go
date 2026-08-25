package inspect

import (
	"strings"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/domain/composite"
)

// formatRows renders the construct topology: the factor axes the engine
// declares, the rule rows compiled against them, each row's activation
// candidate, and the owner fence every published output is held to.
//
// The rows here are the declared topology of the compilation the Plan was
// bound to. The per-fixture instantiated rows live on engine.CommittedProgram,
// which the Plan does not publish; Gaps names that exactly.
func formatRows(session *Session) string {
	var b strings.Builder
	writef(&b, "session.Fixture=%s", session.fixture)
	writef(&b, "compilation.Available=%t", session.compilation.Available())
	writef(&b, "compilation.Digest=%s", session.compilation.Digest())
	writef(&b, "session.CompileStatus.CompileComplete=%t", session.compileStatus == analysis.CompileComplete)
	writef(&b, "session.SolveStatus.AnalyzeComplete=%t", session.solveStatus == analysis.AnalyzeComplete)

	plans, plansOK := session.compilation.RulePlans()
	writef(&b, "compilation.RulePlans.Available=%t", plansOK && plans.Available())
	if plansOK && plans.Available() {
		writef(&b, "compilation.RulePlans.Digest=%s", plans.Digest())
		writef(&b, "compilation.RulePlans.AxisCount=%d", plans.AxisCount())
		for index := 0; index < plans.AxisCount(); index++ {
			axis, axisOK := plans.AxisAt(index)
			if !axisOK {
				continue
			}
			writef(&b, "compilation.RulePlans.AxisAt(%d).Key=%s", index, axis.Key)
			writef(&b, "compilation.RulePlans.AxisAt(%d).Semantic=%s", index, semanticSpelling(axis.Semantic))
			if storage, storageOK := composite.AxisStorage(session.compilation, axis.Key); storageOK {
				writef(&b, "composite.AxisStorage(%s)=%d", axis.Key, uint8(storage))
			}
			if cardinality, cardinalityOK := composite.AxisCardinality(session.compilation, axis.Key); cardinalityOK {
				writef(&b, "composite.AxisCardinality(%s)=%d", axis.Key, uint8(cardinality))
			}
			if lifetime, lifetimeOK := composite.AxisLifetime(session.compilation, axis.Key); lifetimeOK {
				writef(&b, "composite.AxisLifetime(%s)=%d", axis.Key, uint8(lifetime))
			}
			if mounted, mountedOK := composite.AxisMountDeclared(session.compilation, axis.Key); mountedOK {
				writef(&b, "composite.AxisMountDeclared(%s)=%t", axis.Key, mounted)
			}
		}
		writef(&b, "compilation.RulePlans.Count=%d", plans.Count())
	}

	declared := session.declared
	writef(&b, "composite.Table.RuleCount=%d", declared.RuleCount())
	for position := 0; position < declared.RuleCount(); position++ {
		template, templateOK := declared.RuleAt(position)
		if !templateOK {
			continue
		}
		key := template.Key()
		writef(&b, "rule[%d].Key=%s", position, key)
		writef(&b, "rule[%d].Owner=%s", position, template.Owner())
		writef(&b, "rule[%d].Writes=%s", position, template.Writes())
		writef(&b, "rule[%d].Lane=%d", position, uint8(template.Lane()))
		writef(&b, "rule[%d].Semantic=%s", position, template.Semantic())
		declaration := template.Program()
		writef(&b, "rule[%d].Program.Available=%t", position, declaration.Available())
		if !declaration.Available() {
			continue
		}
		writef(&b, "rule[%d].Program.OperandRole=%s", position, declaration.OperandRole)
		if declaration.Candidate.Issued() {
			writef(&b, "rule[%d].Program.Candidate.IssuedRow=%s", position, declaration.Candidate.IssuedRow)
		} else {
			writef(&b, "rule[%d].Program.Candidate.Axis=%s", position, declaration.Candidate.AxisRelation.Axis.Key)
			writef(&b, "rule[%d].Program.Candidate.Member=%s", position, declaration.Candidate.AxisRelation.Member)
		}
		writef(&b, "rule[%d].Program.JoinCount=%d", position, declaration.JoinCount())
		writef(&b, "rule[%d].Program.Fold.Reducer.Axis=%s", position, declaration.Fold.Reducer.Axis.Key)
		writef(&b, "rule[%d].Program.Fold.Reducer.Member=%s", position, declaration.Fold.Reducer.Member)
		writef(&b, "rule[%d].Program.Fold.InputCount=%d", position, len(declaration.Fold.Inputs))
		writef(&b, "rule[%d].Program.Fold.OutputCount=%d", position, len(declaration.Fold.Outputs))
		for outputIndex, output := range declaration.Fold.Outputs {
			writef(&b, "rule[%d].Program.Fold.Outputs[%d].Column.Axis=%s", position, outputIndex, output.Column.Axis.Key)
			writef(&b, "rule[%d].Program.Fold.Outputs[%d].Column.Key=%s", position, outputIndex, output.Column.Key)
			writef(&b, "rule[%d].Program.Fold.Outputs[%d].Destination.Axis=%s", position, outputIndex, output.Destination.Axis.Key)
			writef(&b, "rule[%d].Program.Fold.Outputs[%d].Destination.Member=%s", position, outputIndex, output.Destination.Member)
			writef(&b, "rule[%d].Program.Fold.Outputs[%d].Mode=%s", position, outputIndex, outputModeSpelling(output.Mode))
			// The owner fence: a rule publishes only into the axis frame its
			// Template declares as owner. Rule.Template.Owner is the fence and
			// OutputDecl.Column.Axis is the write, so the two are printed as one
			// verdict rather than left for a reader to correlate.
			writef(&b, "rule[%d].Program.Fold.Outputs[%d].OwnerFenceHeld=%t", position, outputIndex,
				output.Column.Axis.Key == template.Owner())
		}
		if declaration.Carry != nil {
			writef(&b, "rule[%d].Program.Carry.Mode=%s", position, carryModeSpelling(declaration.Carry.Mode))
			writef(&b, "rule[%d].Program.Carry.Input=%d", position, uint64(declaration.Carry.Input))
		}
		transportCount := 0
		if declaration.Activation != nil {
			transportCount = len(declaration.Activation.Transport)
		}
		writef(&b, "rule[%d].Program.Activation.TransportCount=%d", position, transportCount)
		if declaration.Activation != nil {
			for transportIndex, transport := range declaration.Activation.Transport {
				writef(&b, "rule[%d].Program.Activation.Transport[%d].Axis=%s", position, transportIndex, transport.Axis.EntryReference().Key)
				writef(&b, "rule[%d].Program.Activation.Transport[%d].Exported=%t", position, transportIndex, transport.Exported)
			}
		}
	}

	for _, gap := range Gaps() {
		writef(&b, "unexposed.%s=%s", gap.Layer, gap.Accessor)
	}
	return b.String()
}
