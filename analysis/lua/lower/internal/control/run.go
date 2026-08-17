package control

import "fmt"

// Run advances exactly one private control continuation. Cross-vertical work
// is requested through its typed inbox; its result returns through continuation.Result.
func (w *Writer) Run() error {
	if err := w.ready(); err != nil {
		return err
	}
	if len(w.steps) == 0 {
		return fmt.Errorf("lualower: missing control continuation")
	}
	last := len(w.steps) - 1
	current := w.steps[last]
	w.steps = w.steps[:last]

	switch current.kind {
	case finishReturnStep:
		return w.finishReturn(current)
	case finishIfConditionStep:
		return w.finishIfCondition(current)
	case finishIfThenStep:
		return w.finishIfThen(current)
	case finishIfElseStep:
		return w.finishIfElse(current)
	case finishWhileConditionStep:
		return w.finishWhileCondition(current)
	case finishRepeatConditionStep:
		return w.finishRepeatCondition(current)
	case finishRepeatControlStep:
		return w.finishRepeatControl(current)
	case finishLoopStep:
		return w.finishLoop(current)
	case numberControlStep:
		return w.runNumberControls(current)
	case appendNumberControlStep:
		return w.appendNumberControl(current)
	case finishGenericControlsStep:
		return w.finishGenericControls(current)
	default:
		return fmt.Errorf("lualower: unknown control continuation %d", current.kind)
	}
}
