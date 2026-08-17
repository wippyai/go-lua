package flow

import (
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type controlRows struct {
	returns   []programflow.Return
	breaks    []programflow.Break
	labels    []programflow.Label
	gotos     []programflow.Goto
	branches  []programflow.Branch
	loops     []programflow.Loop
	loopCells []keyspace.Term
}

func (r *Rows) AppendReturn(row programflow.Return) {
	if r != nil {
		r.control.returns = append(r.control.returns, row)
	}
}

func (r *Rows) AppendBreak(row programflow.Break) {
	if r != nil {
		r.control.breaks = append(r.control.breaks, row)
	}
}

func (r *Rows) AppendLabel(row programflow.Label) {
	if r != nil {
		r.control.labels = append(r.control.labels, row)
	}
}

func (r *Rows) AppendGoto(row programflow.Goto) {
	if r != nil {
		r.control.gotos = append(r.control.gotos, row)
	}
}

func (r *Rows) AppendBranch(row programflow.Branch) {
	if r != nil {
		r.control.branches = append(r.control.branches, row)
	}
}

func (r *Rows) AppendLoop(row programflow.Loop, cells []keyspace.Term) (programflow.Range, bool) {
	if r == nil {
		return programflow.Range{}, false
	}
	result, ok := rangeFor(len(r.control.loopCells), len(cells))
	if !ok {
		return programflow.Range{}, false
	}
	r.control.loopCells = append(r.control.loopCells, cells...)
	row.Cells = result
	r.control.loops = append(r.control.loops, row)
	return result, true
}
