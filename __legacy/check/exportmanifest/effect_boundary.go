package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
)

// analyzedExportEffectRow removes effect labels that are valid imported or
// stdlib vocabulary but not valid publication vocabulary for analyzed exports.
func analyzedExportEffectRow(row effect.Row) effect.Row {
	if row.Pure() {
		return row
	}
	return row.Without(isImportOrStdlibEffectLabel)
}

func isImportOrStdlibEffectLabel(label effect.Label) bool {
	desc, ok := caplabel.DescriptorFor(label)
	return ok && desc.Status == capability.StatusImportOrStdlib
}
