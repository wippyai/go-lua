package engine

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// frozenBodyNativeFacts reads the structural rows the equation kernels publish
// for an evaluated body out of the frozen artifact of a body that was not
// evaluated. Evaluation is rooted at the root artifact, so only the root body's
// equations reach the value closure; a nested lexical body is an independently
// admitted publication whose eval-node and claim-assert occurrences would
// otherwise be silent.
//
// It derives nothing the kernels do not: the same occurrence kinds, the same
// operand roles and the same published values. The root body is never projected
// here, so one occurrence never carries two rows.
func frozenBodyNativeFacts(root front.Compilation) []NativeFact {
	var rows []NativeFact
	var visit func(compilation front.Compilation, evaluated bool)
	visit = func(compilation front.Compilation, evaluated bool) {
		if !evaluated {
			rows = append(rows, frozenBodyOccurrenceFacts(compilation)...)
		}
		for _, child := range compilation.Nested {
			visit(child, false)
		}
	}
	visit(root, true)
	return rows
}

func frozenBodyOccurrenceFacts(compilation front.Compilation) []NativeFact {
	var out []NativeFact
	for _, operation := range compilation.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "eval-node":
			operands, err := artifactOperandsByRole(operation.Operands, "operation")
			if err != nil {
				continue
			}
			name := string(operands["operation"])
			if !projectedEvalNodeOperation(name) {
				continue
			}
			out = append(out, frozenBodyRow(compilation, "eval_node", operation.Target.Name, "operation="+name))
		case "claim":
			operands, err := artifactOperandsByRole(operation.Operands, "kind")
			if err != nil || string(operands["kind"]) != claimAssertKind {
				continue
			}
			out = append(out, frozenBodyRow(compilation, "throw_template", operation.Target.Name, claimAssertThrowTemplateValue))
		}
	}
	return out
}

func frozenBodyRow(compilation front.Compilation, family, occurrence, content string) NativeFact {
	return NativeFact{
		Lane: NativeLaneValues, Family: family,
		Key:        family + "/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence,
		Value:      content,
		Occurrence: occurrence, Trust: NativeTrustProven,
	}
}
