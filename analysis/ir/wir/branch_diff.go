package wir

import "github.com/wippyai/go-lua/analysis/domain/path"

// BranchDiffConstraint is neutral branch metadata for a difference-logic proof:
//
//	coHi*hi + coHi2*hi2 - lo <= c
//
// It is derived from syntax during lowering and carried by OpBranch. Transfer
// decides how to project the descriptor into fact lanes.
type BranchDiffConstraint struct {
	CoHi     int64
	HiPath   path.Path
	HiIsLen  bool
	CoHi2    int64
	Hi2Path  path.Path
	Hi2IsLen bool
	HasHi2   bool
	LoPath   path.Path
	LoIsLen  bool
	C        int64
	Edge     bool
}
