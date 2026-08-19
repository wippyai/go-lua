package executable

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal constructs the canonical pre-Outcome executable closure.  Source
// control admits reachable direct Body roots and Bodies; containment removes
// static terms; authored operands are then closed to a fixed point by an
// iterative typed worklist.
func Seal(
	sourceView source.View,
	flow authored.View,
	forest *containment.Result,
	control *sourcecontrol.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
	paths *semanticpath.Certificate,
) (*Result, error) {
	if paths == nil {
		return nil, errors.New("program/flow/executable: semantic-path certificate unavailable")
	}
	input, err := validateInputs(sourceView, flow, forest, control, staticID, moduleID)
	if err != nil {
		return nil, err
	}
	seed, err := seedRoots(sourceView, forest, control, input)
	if err != nil {
		return nil, err
	}
	result, err := closeOperands(flow, sourceView, forest, input.counts, seed)
	if err != nil {
		return nil, err
	}
	if err := result.installRoots(seed.roots, paths); err != nil {
		return nil, err
	}
	return result, nil
}
