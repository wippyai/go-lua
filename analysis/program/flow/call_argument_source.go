package flow

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/provenance"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// CallArgumentSource is the sealed Flow locator for one published call
// argument identity. The argument row carries only its durable identity;
// this owner-issued projection retains the authored term and dense Call
// ordinal needed for source diagnostics. Consumers must query this surface,
// never rebuild a Call→Values→member inverse from sibling rows.
type CallArgumentSource struct {
	id    identity.ContentID
	term  keyspace.Term
	index int
}

// Term is the authored Values member that issued the argument identity.
func (source CallArgumentSource) Term() keyspace.Term {
	if !source.Available() {
		return 0
	}
	return source.term
}

// CallIndex is the dense authored Call ordinal used for construction faults.
func (source CallArgumentSource) CallIndex() int {
	if !source.Available() {
		return -1
	}
	return source.index
}

func (source CallArgumentSource) Available() bool {
	return source.id.Available() && source.term != 0 && source.index >= 0
}

type callArgumentSourceIndex struct {
	owner provenance.Provenance
	rows  []CallArgumentSource
}

func (index *callArgumentSourceIndex) lookup(owner provenance.Provenance, id identity.ContentID) (CallArgumentSource, bool) {
	if index == nil || index.owner != owner || !owner.Available() || !id.Available() {
		return CallArgumentSource{}, false
	}
	position := sort.Search(len(index.rows), func(position int) bool {
		return bytes.Compare(index.rows[position].id[:], id[:]) >= 0
	})
	if position >= len(index.rows) || index.rows[position].id != id {
		return CallArgumentSource{}, false
	}
	row := index.rows[position]
	if position+1 < len(index.rows) && index.rows[position+1].id == id {
		return CallArgumentSource{}, false
	}
	return row, row.Available()
}

func buildCallArgumentSourceIndex(view View) (*callArgumentSourceIndex, bool) {
	if !view.available() {
		return nil, false
	}
	calls := view.Authored().Calls()
	values := view.Authored().Values()
	rows := make([]CallArgumentSource, 0)
	for callIndex := 0; callIndex < calls.Count(); callIndex++ {
		call, callOK := calls.At(callIndex)
		_, _, _, actuals, relationOK := calls.Get(call)
		width, widthOK := values.Len(actuals)
		if !callOK || !relationOK || !widthOK || width < 0 {
			return nil, false
		}
		for argumentIndex := 0; argumentIndex < width; argumentIndex++ {
			member, memberOK := values.Member(actuals, argumentIndex)
			argumentID, argumentOK := view.CallArgumentID(call, argumentIndex)
			if !memberOK || !argumentOK || !argumentID.Available() {
				return nil, false
			}
			rows = append(rows, CallArgumentSource{id: argumentID, term: member, index: callIndex})
		}
	}
	sort.Slice(rows, func(left, right int) bool { return bytes.Compare(rows[left].id[:], rows[right].id[:]) < 0 })
	for index := 1; index < len(rows); index++ {
		if rows[index-1].id == rows[index].id {
			return nil, false
		}
	}
	return &callArgumentSourceIndex{owner: view.Provenance(), rows: rows}, true
}

// CallArgumentSource resolves a published argument identity to its
// owner-issued authored source coordinate. The binary search is over one
// immutable Flow-owned table and performs no allocation.
func (view View) CallArgumentSource(id identity.ContentID) (CallArgumentSource, bool) {
	if !view.projectionFence() {
		return CallArgumentSource{}, false
	}
	return view.component.callArgumentSources.lookup(view.Provenance(), id)
}
