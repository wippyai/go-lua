package assign

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func expandedAssignValues(synthAPI api.SynthAPI, info *cfg.AssignInfo, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	if synthAPI == nil || info == nil || len(info.Targets) == 0 || len(info.Sources) == 0 {
		return nil
	}
	return synthAPI.ExpandValuesWithSpecTypes(info.Sources, len(info.Targets), p, specTypes)
}

func assignValueAt(values []typ.Type, i int) typ.Type {
	if i < 0 || i >= len(values) {
		return nil
	}
	return values[i]
}
