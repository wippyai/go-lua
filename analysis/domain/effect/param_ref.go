package effect

import "fmt"

// ParamRef references a call argument by position.
type ParamRef struct {
	Index int
}

func (p ParamRef) String() string {
	if p.Index == -1 {
		return "param[last]"
	}
	return fmt.Sprintf("param[%d]", p.Index)
}

// ResolveParamIndex resolves a ParamRef against a runtime argument count.
func ResolveParamIndex(ref ParamRef, argCount int) (int, bool) {
	if argCount <= 0 {
		return 0, false
	}
	idx := ref.Index
	if idx < 0 {
		idx = argCount + idx
	}
	if idx < 0 || idx >= argCount {
		return 0, false
	}
	return idx, true
}
