package engine

// projectExactLocal stores one read slot's typed projector on that exact-read
// cell. The callback belongs to that slot; there is no ordinal dispatch table
// or shared rule-level projector.
func projectExactLocal[O any](project func(O) (uint64, bool)) func(any) (uint64, bool) {
	if project == nil {
		return nil
	}
	return func(operand any) (uint64, bool) {
		typed, ok := operand.(O)
		if !ok {
			return 0, false
		}
		return project(typed)
	}
}
