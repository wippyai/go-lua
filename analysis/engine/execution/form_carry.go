// form_carry.go owns the WT form: one exact read folded onto one exact write
// whose carried prior fact passes through an owner-issued transform.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/generated"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// classifyCarryForm claims a descriptor whose sealed carry names a transform.
// An identity carry hands the prior output fact on unchanged and is the exact
// form's; a transformed carry applies one owner-issued candidate-indexed
// transition to it, which no identity fold can do.
func classifyCarryForm(rule generated.CompiledRule) (FormRow, bool) {
	mode, modeOK := rule.OutputMode()
	if !modeOK || mode != ruleprogram.ModeExact || rule.ReadCount() != 1 {
		return FormRow{}, false
	}
	if form, ok := rule.ReadFormAt(0); !ok || form != ruleprogram.Exact {
		return FormRow{}, false
	}
	if carry, ok := rule.CarryMode(); !ok || carry != ruleprogram.CarryTransform {
		return FormRow{}, false
	}
	if _, present := rule.CarryTransform(); !present {
		return FormRow{}, false
	}
	input := rule.ReadInput()
	if input < 0 || input >= rule.InputCount() || input > int(^uint16(0)) || rule.InputCount() <= 0 {
		return FormRow{}, false
	}
	return FormRow{Form: FormCarry, Input: uint16(input)}, true
}
