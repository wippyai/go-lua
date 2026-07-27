package front

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
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
func frozenBodyNativeFacts(root Compilation) []NativeProjection {
	var rows []NativeProjection
	var visit func(compilation Compilation, evaluated bool)
	visit = func(compilation Compilation, evaluated bool) {
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

func frozenBodyOccurrenceFacts(compilation Compilation) []NativeProjection {
	var out []NativeProjection
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

func frozenBodyRow(compilation Compilation, family, occurrence, content string) NativeProjection {
	return NativeProjection{
		Key:        family + "/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence,
		Value:      content,
		Occurrence: occurrence,
	}
}

const (
	claimAssertKind               = "claim-kind/2"
	claimAssertThrowTemplateValue = "allocates=false;false_arm=passes;kind=claim_assert;nil_arm=throws;preserves_word_on_success=true"
)

func projectedEvalNodeOperation(name string) bool {
	switch name {
	case "closure", "length":
		return true
	default:
		return false
	}
}

func artifactOperandsByRole(operands []equation.Operand, roles ...string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(roles))
	for _, role := range roles {
		for _, operand := range operands {
			if operand.Role == role && !operand.Term.Entry {
				out[role] = operand.Term.Encoding
				break
			}
		}
		if out[role] == nil {
			return nil, fmt.Errorf("front: missing closed artifact operand %q", role)
		}
	}
	return out, nil
}
