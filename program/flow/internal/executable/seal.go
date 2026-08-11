package executable

import (
	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
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
	staticID keyspace.ContentID,
	moduleID keyspace.ContentID,
) (*Result, error) {
	input, err := validateInputs(sourceView, flow, forest, control, staticID, moduleID)
	if err != nil {
		return nil, err
	}
	seed, err := seedRoots(sourceView, forest, control, input)
	if err != nil {
		return nil, err
	}
	return closeOperands(flow, sourceView, forest, input.counts, seed)
}
