package returns

import "github.com/wippyai/go-lua/analysis/domain/effect"

func GetReturn(r effect.Row, retIdx int) *Return {
	for _, l := range r.Labels {
		if ret, ok := effect.NormalizeLabel(l).(Return); ok && ret.ReturnIndex == retIdx {
			return &ret
		}
	}
	return nil
}

func GetErrorReturn(r effect.Row, valueIdx int) *ErrorReturn {
	for _, l := range r.Labels {
		if er, ok := effect.NormalizeLabel(l).(ErrorReturn); ok && er.ValueIndex == valueIdx {
			return &er
		}
	}
	return nil
}

func GetCorrelatedReturn(r effect.Row, idx int) *CorrelatedReturn {
	for _, l := range r.Labels {
		if cr, ok := effect.NormalizeLabel(l).(CorrelatedReturn); ok {
			for _, i := range cr.Indices {
				if i == idx {
					return &cr
				}
			}
		}
	}
	return nil
}

func GetReturnLength(r effect.Row, retIdx int) *ReturnLength {
	for _, l := range r.Labels {
		if ret, ok := effect.NormalizeLabel(l).(ReturnLength); ok && ret.ReturnIndex == retIdx {
			return &ret
		}
	}
	return nil
}

func Returns(retIdx int, derive ReturnType) effect.Row {
	return effect.Row{Labels: []effect.Label{Return{ReturnIndex: retIdx, Transform: derive}}}
}
