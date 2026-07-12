// Package flatinterproc is an isolated proof of one global interprocedural WTO
// over exact function/context CFG cells and summary boundary cells.
package flatinterproc

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

const valueSlot = key.Value(1)

type op uint8

const (
	opFork op = iota + 1
	opConst
	opCall
	opReturn
)

type node struct {
	op         op
	next       []int
	value      product.Value
	callee     ref.FuncRef
	contextual bool
	external   bool
	loopHead   bool
}

type function struct {
	ref   ref.FuncRef
	nodes []node
	entry int
}

type program struct {
	reg       *axis.Registry
	functions map[ref.FuncRef]function
	roots     map[summary.SummaryKey]state.State
}

type snapshot struct {
	summaries map[summary.SummaryKey]summary.Summary
	contexts  map[summary.SummaryKey]state.State
	transfers int
}

func (s snapshot) clone() snapshot {
	out := snapshot{summaries: make(map[summary.SummaryKey]summary.Summary, len(s.summaries)), contexts: make(map[summary.SummaryKey]state.State, len(s.contexts)), transfers: s.transfers}
	for key, value := range s.summaries {
		out.summaries[key] = value.Clone()
	}
	for key, value := range s.contexts {
		out.contexts[key] = value
	}
	return out
}

func contextKey(reg *axis.Registry, callee ref.FuncRef, argument product.Value, contextual bool) summary.SummaryKey {
	key := summary.DefaultSummaryKey(callee)
	if contextual {
		key.Entry.Values = summary.Digest(product.Hash(reg, argument))
		if key.Entry.Values == 0 {
			key.Entry.Values = 1
		}
	}
	return key
}

func stateWithValue(reg *axis.Registry, value product.Value) state.State {
	return state.Domain(reg).Bottom().WriteValue(reg, valueSlot, value)
}

func valueOf(reg *axis.Registry, st state.State) product.Value { return st.ReadValue(reg, valueSlot) }

func returnAt(reg *axis.Registry, sum summary.Summary) product.Value {
	if len(sum.Returns) == 0 {
		return product.Bottom(reg)
	}
	return sum.Returns[0]
}

func sortedContextKeys(contexts map[summary.SummaryKey]state.State) []summary.SummaryKey {
	keys := make([]summary.SummaryKey, 0, len(contexts))
	for key := range contexts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	return keys
}

func (p program) validate() error {
	if p.reg == nil || len(p.functions) == 0 || len(p.roots) == 0 {
		return fmt.Errorf("flatinterproc: incomplete program")
	}
	for id, fn := range p.functions {
		if id != fn.ref || fn.entry < 0 || fn.entry >= len(fn.nodes) {
			return fmt.Errorf("flatinterproc: invalid function %s", id)
		}
		for _, node := range fn.nodes {
			for _, next := range node.next {
				if next < 0 || next >= len(fn.nodes) {
					return fmt.Errorf("flatinterproc: invalid CFG edge")
				}
			}
			if node.op == opCall && !node.external {
				if _, ok := p.functions[node.callee]; !ok {
					return fmt.Errorf("flatinterproc: missing internal callee")
				}
			}
		}
	}
	return nil
}

func diagnostics(reg *axis.Registry, snap snapshot) []string {
	keys := make([]summary.SummaryKey, 0, len(snap.summaries))
	for key := range snap.summaries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	bottom := product.Bottom(reg)
	var out []string
	for _, key := range keys {
		value := returnAt(reg, snap.summaries[key])
		switch {
		case product.Equal(reg, value, bottom):
			out = append(out, key.Ref.String()+":no-return")
		case product.Equal(reg, value, product.Top()):
			out = append(out, key.Ref.String()+":unknown-return")
		}
	}
	return out
}
