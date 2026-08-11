package collector

// This file contains Flow's owner composition and shared construction
// helpers. Semantic operations live in vertical leaf proxy files. The rows
// below are construction-only and are materialized by flow_freeze.go.

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Each child owns one semantic vertical. No child stores a relation belonging
// to another owner; in particular Source order is not shadowed in Flow.
type flowValuesRows struct {
	values     []flow.Value
	valueTerms []keyspace.Term
}

type flowAccessRows struct {
	exactLenses   []flow.ExactLens
	dynamicLenses []flow.DynamicLens
}

type flowStorageRows struct {
	cells        []flow.Cell
	globalCensus bind.GlobalCensus
	reads        []flow.Read
	varargs      []flow.Vararg
	binds        []flow.Bind
	assigns      []flow.Assign
	writes       []flow.Write
}

type flowTablesRows struct {
	tables      []flow.Table
	tableFields []flow.Field
	tableOrder  []keyspace.Term
	tableFilled []bool
}

type flowFunctionsRows struct {
	functions []flow.Function
	captures  []flow.Capture
}

type flowCallsRows struct {
	calls []flow.Call
}

type flowControlRows struct {
	returns   []flow.Return
	breaks    []flow.Break
	labels    []flow.Label
	gotos     []flow.Goto
	branches  []flow.Branch
	loops     []flow.Loop
	loopCells []keyspace.Term
}

type flowOperatorsRows struct {
	unaries  []flow.Unary
	binaries []flow.Binary
	selects  []flow.Select
}

type flowOperandsRows struct {
	claims     []flow.ValueClaim
	typeValues []flow.TypeValue
}

// flowRows is one owner-composed construction state. It contains no
// horizontal operation stream and does not survive materialization.
type flowRows struct {
	values    flowValuesRows
	access    flowAccessRows
	storage   flowStorageRows
	tables    flowTablesRows
	functions flowFunctionsRows
	calls     flowCallsRows
	control   flowControlRows
	operators flowOperatorsRows
	operands  flowOperandsRows
}

// Leaf capabilities are intentionally small. api.go assembles these into the
// FlowWriter root; a caller cannot use one vertical to reach another.
type FlowValuesWriter struct{ collector *Collector }
type FlowAccessWriter struct{ collector *Collector }
type FlowStorageWriter struct{ collector *Collector }
type FlowTablesWriter struct{ collector *Collector }
type FlowFunctionsWriter struct{ collector *Collector }
type FlowCallsWriter struct{ collector *Collector }
type FlowControlWriter struct{ collector *Collector }
type FlowOperatorsWriter struct{ collector *Collector }
type FlowOperandsWriter struct{ collector *Collector }

func familyOK(term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0
}

func rangeFor(poolLen, add int) (flow.Range, bool) {
	if poolLen < 0 || add < 0 {
		return flow.Range{}, false
	}
	start := uint64(poolLen)
	end := start + uint64(add)
	if end > uint64(keyspace.MaxTermOrdinal) {
		return flow.Range{}, false
	}
	return flow.Range{Start: uint32(start), End: uint32(end)}, true
}

func appendTerms(pool *[]keyspace.Term, terms []keyspace.Term) (flow.Range, bool) {
	if pool == nil {
		return flow.Range{}, false
	}
	r, ok := rangeFor(len(*pool), len(terms))
	if !ok {
		return flow.Range{}, false
	}
	*pool = append(*pool, terms...)
	return r, true
}

func cloneTerms(terms []keyspace.Term) []keyspace.Term {
	if len(terms) == 0 {
		return nil
	}
	return append([]keyspace.Term(nil), terms...)
}
