// Package exactscalar owns the compiler's finite exact-scalar dataflow. It
// consumes only canonical occurrence and function rows and publishes the
// exact summaries plus the private facts needed by arithmetic derivation.
package exactscalar

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
)

type state struct {
	unknown bool
	values  map[keyspace.LiteralValue]struct{}
}

func (state state) known() bool { return state.unknown || len(state.values) != 0 }

// finite returns the complete finite literal image when this state has no
// opaque remainder. Unknown states deliberately discard their retained image;
// the explicit unbounded remainder makes any partial image unsuitable as a
// complete arithmetic operand.
func (state state) finite() []keyspace.LiteralValue {
	if state.unknown || len(state.values) == 0 {
		return nil
	}
	values := make([]keyspace.LiteralValue, 0, len(state.values))
	for value := range state.values {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool {
		a, b := values[left], values[right]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Integer != b.Integer {
			return a.Integer < b.Integer
		}
		if a.FloatBits != b.FloatBits {
			return a.FloatBits < b.FloatBits
		}
		return a.String < b.String
	})
	return values
}

func (state state) exact() (keyspace.LiteralValue, bool) {
	if state.unknown || len(state.values) != 1 {
		return keyspace.LiteralValue{}, false
	}
	for value := range state.values {
		return value, true
	}
	return keyspace.LiteralValue{}, false
}

// Exact returns the one literal proven for id, if the subject is exact.
func (bundle *Bundle) Exact(id identity.ContentID) (keyspace.LiteralValue, bool) {
	if bundle == nil || !id.Available() {
		return keyspace.LiteralValue{}, false
	}
	return bundle.states[id].exact()
}

// Rows returns the canonical exact-scalar summaries.
func (bundle *Bundle) Rows() []programschema.ExactScalarSummary {
	if bundle == nil {
		return nil
	}
	return append([]programschema.ExactScalarSummary(nil), bundle.summaries...)
}

// ReleaseFacts drops the construction-only fixed-point state after the last
// arithmetic consumer. Canonical summary rows remain owned by the bundle for
// publication.
func (bundle *Bundle) ReleaseFacts() {
	if bundle != nil {
		bundle.states = nil
	}
}

type equationKind uint8

const (
	equationCopy equationKind = iota + 1
	equationUnaryNeg
	equationArithmetic
)

type equation struct {
	kind        equationKind
	output      identity.ContentID
	left, right identity.ContentID
	op          flowkind.BinaryOp
}

// equationSCCs returns the dependency component for every equation subject
// and marks the cyclic components. Dependencies point from an operand to the
// equation output; pure copy cycles remain finite aliases. A later
// equation-specific check decides whether an arithmetic/unary edge actually
// consumes a value from its own cyclic component before widening.
func equationSCCs(equations []equation) (map[identity.ContentID]int, map[identity.ContentID]bool) {
	adjacency := make(map[identity.ContentID][]identity.ContentID)
	nodes := make(map[identity.ContentID]struct{})
	add := func(from, to identity.ContentID) {
		if !from.Available() || !to.Available() {
			return
		}
		nodes[from] = struct{}{}
		nodes[to] = struct{}{}
		adjacency[from] = append(adjacency[from], to)
	}
	for _, item := range equations {
		if !item.output.Available() {
			continue
		}
		nodes[item.output] = struct{}{}
		switch item.kind {
		case equationCopy, equationUnaryNeg:
			add(item.left, item.output)
		case equationArithmetic:
			add(item.left, item.output)
			add(item.right, item.output)
		}
	}

	indices := make(map[identity.ContentID]int, len(nodes))
	lowlinks := make(map[identity.ContentID]int, len(nodes))
	onStack := make(map[identity.ContentID]bool, len(nodes))
	stack := make([]identity.ContentID, 0, len(nodes))
	nextIndex := 0
	components := make(map[identity.ContentID]int, len(nodes))
	cyclic := make(map[identity.ContentID]bool)
	componentID := 0
	var visit func(identity.ContentID)
	visit = func(node identity.ContentID) {
		indices[node] = nextIndex
		lowlinks[node] = nextIndex
		nextIndex++
		stack = append(stack, node)
		onStack[node] = true
		for _, next := range adjacency[node] {
			if _, seen := indices[next]; !seen {
				visit(next)
				if lowlinks[next] < lowlinks[node] {
					lowlinks[node] = lowlinks[next]
				}
			} else if onStack[next] && indices[next] < lowlinks[node] {
				lowlinks[node] = indices[next]
			}
		}
		if lowlinks[node] != indices[node] {
			return
		}

		component := make([]identity.ContentID, 0, 1)
		for {
			last := len(stack) - 1
			member := stack[last]
			stack = stack[:last]
			onStack[member] = false
			component = append(component, member)
			components[member] = componentID
			if member == node {
				break
			}
		}
		if len(component) > 1 {
			for _, member := range component {
				cyclic[member] = true
			}
			componentID++
			return
		}
		member := component[0]
		for _, next := range adjacency[member] {
			if next == member {
				cyclic[member] = true
				break
			}
		}
		componentID++
	}
	for node := range nodes {
		if _, seen := indices[node]; !seen {
			visit(node)
		}
	}
	return components, cyclic
}

func finiteUnionGrows(current, incoming map[keyspace.LiteralValue]struct{}) bool {
	for value := range incoming {
		if _, exists := current[value]; !exists {
			return true
		}
	}
	return false
}

func unaryFiniteResults(values []keyspace.LiteralValue) map[keyspace.LiteralValue]struct{} {
	results := make(map[keyspace.LiteralValue]struct{})
	for _, literal := range values {
		result, resultOK := scalar.ExactUnaryNegLiteral(literal)
		if !resultOK {
			continue
		}
		if _, duplicate := results[result]; duplicate {
			continue
		}
		results[result] = struct{}{}
	}
	return results
}

func unaryFiniteGrowth(values []keyspace.LiteralValue, current map[keyspace.LiteralValue]struct{}) (grows, reachable bool) {
	for _, literal := range values {
		result, resultOK := scalar.ExactUnaryNegLiteral(literal)
		if !resultOK {
			continue
		}
		reachable = true
		if _, exists := current[result]; !exists {
			return true, true
		}
	}
	return false, reachable
}

func arithmeticFiniteResults(left, right []keyspace.LiteralValue, op flowkind.BinaryOp) map[keyspace.LiteralValue]struct{} {
	results := make(map[keyspace.LiteralValue]struct{})
	for _, leftLiteral := range left {
		for _, rightLiteral := range right {
			result, resultOK := scalar.ExactArithmeticLiteral(leftLiteral, rightLiteral, op)
			// An undefined numeric pair (for example integer division by zero)
			// contributes no reachable result. It must not erase the other
			// finite cells or widen them to an invented unknown value.
			if !resultOK {
				continue
			}
			if _, duplicate := results[result]; duplicate {
				continue
			}
			results[result] = struct{}{}
		}
	}
	return results
}

func arithmeticFiniteGrowth(left, right []keyspace.LiteralValue, op flowkind.BinaryOp, current map[keyspace.LiteralValue]struct{}) (grows, reachable bool) {
	for _, leftLiteral := range left {
		for _, rightLiteral := range right {
			result, resultOK := scalar.ExactArithmeticLiteral(leftLiteral, rightLiteral, op)
			if !resultOK {
				continue
			}
			reachable = true
			if _, exists := current[result]; !exists {
				return true, true
			}
		}
	}
	return false, reachable
}

// Input is the complete canonical row boundary needed by exact-scalar
// derivation. No Program, Flow, compiler state, or domain callback crosses it.
type Input struct {
	Occurrences        []programschema.Occurrence
	OccurrencePoints   []programschema.OccurrencePoint
	OccurrenceInputs   []programschema.OccurrenceInput
	FunctionBoundaries []programschema.FunctionBoundary
	FunctionFormals    []programschema.FunctionFormal
	FunctionVarargs    []programschema.FunctionVararg
	FunctionCaptures   []programschema.FunctionCapture
}

// Bundle owns the private fixed-point states and canonical summaries.
type Bundle struct {
	states    map[identity.ContentID]state
	summaries []programschema.ExactScalarSummary
}

func formalAt(input Input, boundary programschema.FunctionBoundary, index int) (programschema.FunctionFormal, bool) {
	if index < 0 || index >= boundary.FormalCount() {
		return programschema.FunctionFormal{}, false
	}
	offset, _, ok := boundary.FormalSpan()
	if !ok || uint64(offset)+uint64(index) >= uint64(len(input.FunctionFormals)) {
		return programschema.FunctionFormal{}, false
	}
	formal := input.FunctionFormals[int(offset)+index]
	return formal, formal.Available()
}

func vararg(input Input, boundary programschema.FunctionBoundary) (programschema.FunctionVararg, bool) {
	if !boundary.HasVararg() {
		return programschema.FunctionVararg{}, false
	}
	offset, count, ok := boundary.VarargSpan()
	if !ok || count != 1 || uint64(offset) >= uint64(len(input.FunctionVarargs)) {
		return programschema.FunctionVararg{}, false
	}
	row := input.FunctionVarargs[offset]
	return row, row.Available()
}

func captureAt(input Input, boundary programschema.FunctionBoundary, index int) (programschema.FunctionCapture, bool) {
	if index < 0 || index >= boundary.CaptureCount() {
		return programschema.FunctionCapture{}, false
	}
	offset, _, ok := boundary.CaptureSpan()
	if !ok || uint64(offset)+uint64(index) >= uint64(len(input.FunctionCaptures)) {
		return programschema.FunctionCapture{}, false
	}
	row := input.FunctionCaptures[int(offset)+index]
	return row, row.Available() && row.InnerBodyID() == boundary.BodyID()
}

func Compile(input Input) (*Bundle, programconstruction.Fault) {
	states := make(map[identity.ContentID]state)
	var equations []equation
	recurrent := make([]bool, 0)
	join := func(id identity.ContentID, incoming state, recurrent bool) bool {
		if !id.Available() || !incoming.known() {
			return false
		}
		current := states[id]
		if current.unknown {
			return false
		}
		if incoming.unknown {
			current.unknown = true
			current.values = nil
			states[id] = current
			return true
		}
		if recurrent && finiteUnionGrows(current.values, incoming.values) {
			// A recurrence-derived result strictly grew its widening point.
			// Preserve no truncated prefix: unknown is the explicit owner
			// remainder consumed by finite arithmetic as Top.
			current.unknown = true
			current.values = nil
			states[id] = current
			return true
		}
		changed := false
		if len(incoming.values) != 0 {
			if current.values == nil {
				current.values = make(map[keyspace.LiteralValue]struct{}, len(incoming.values))
			}
			for value := range incoming.values {
				if _, exists := current.values[value]; !exists {
					current.values[value] = struct{}{}
					changed = true
				}
			}
		}
		states[id] = current
		return changed
	}
	unknown := state{unknown: true}
	exact := func(literal keyspace.LiteralValue) state {
		return state{values: map[keyspace.LiteralValue]struct{}{literal: {}}}
	}

	for _, boundary := range input.FunctionBoundaries {
		for index := 0; index < boundary.FormalCount(); index++ {
			formal, ok := formalAt(input, boundary, index)
			if !ok {
				return nil, programconstruction.New(programcatalog.FunctionFormal(), programconstruction.IssueExactScalarUnavailable, -1, index)
			}
			join(formal.CellID(), unknown, false)
		}
		if boundary.HasVararg() {
			row, ok := vararg(input, boundary)
			if !ok {
				return nil, programconstruction.New(programcatalog.FunctionVararg(), programconstruction.IssueExactScalarUnavailable, -1, -1)
			}
			join(row.CellID(), unknown, false)
		}
		for index := 0; index < boundary.CaptureCount(); index++ {
			capture, ok := captureAt(input, boundary, index)
			if !ok {
				return nil, programconstruction.New(programcatalog.FunctionCapture(), programconstruction.IssueExactScalarUnavailable, -1, index)
			}
			join(capture.InnerCellID(), unknown, false)
			join(capture.OuterCellID(), unknown, false)
		}
	}

	for index, row := range input.Occurrences {
		if !programschema.OccurrenceDenseAvailable(row, input.OccurrencePoints, input.OccurrenceInputs) {
			return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarUnavailable, index, -1)
		}
		switch row.Kind() {
		case programschema.OccurrenceValueSource:
			span, spanOK := programschema.OccurrenceValueSourceSpanID(row, input.OccurrenceInputs)
			family, literal, literalOK := row.Literal()
			if !spanOK {
				return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarValueSourceAppend, index, -1)
			}
			if literalOK && (family == keyspace.FamilyInteger || family == keyspace.FamilyFloat) {
				join(row.ID(), exact(literal), false)
				join(span, exact(literal), false)
			} else {
				join(row.ID(), unknown, false)
				join(span, unknown, false)
			}
		case programschema.OccurrenceValuesMember:
			count, ok := programschema.OccurrenceInputCount(row, input.OccurrenceInputs)
			if !ok || count != 2 {
				return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarValues, index, -1)
			}
			span, ok := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 1)
			if !ok {
				return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarValues, index, -1)
			}
			equations = append(equations, equation{kind: equationCopy, output: row.ID(), left: span})
		case programschema.OccurrenceStorageRead:
			cell, span, ok := programschema.OccurrenceStorageReadOperands(row, input.OccurrenceInputs)
			if !ok {
				return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarStorageRead, index, -1)
			}
			equations = append(equations, equation{kind: equationCopy, output: span, left: cell}, equation{kind: equationCopy, output: row.ID(), left: cell})
		case programschema.OccurrenceStorageBindTransfer, programschema.OccurrenceStorageWrite:
			count, ok := programschema.OccurrenceInputCount(row, input.OccurrenceInputs)
			if !ok || count < 3 {
				return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarStorageBind, index, -1)
			}
			from, fromOK := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 1)
			to, toOK := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 2)
			if !fromOK || !toOK {
				return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarStorageBind, index, -1)
			}
			equations = append(equations, equation{kind: equationCopy, output: to, left: from})
		case programschema.OccurrenceBinaryArithmetic:
			left, right, op, ok := programschema.OccurrenceBinaryArithmeticOperands(row, input.OccurrenceInputs)
			if !ok {
				return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarUnavailable, index, -1)
			}
			equations = append(equations, equation{kind: equationArithmetic, output: row.ID(), left: left, right: right, op: op})
		case programschema.OccurrenceUnary:
			operand, ok := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 0)
			if !ok {
				return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarUnavailable, index, -1)
			}
			if flowkind.UnaryOp(row.Code()) == flowkind.UnaryNeg {
				equations = append(equations, equation{kind: equationUnaryNeg, output: row.ID(), left: operand})
			} else {
				join(row.ID(), unknown, false)
			}
		case programschema.OccurrenceSelect, programschema.OccurrenceValueClaim, programschema.OccurrenceBinaryEquality, programschema.OccurrenceBinaryOrder:
			join(row.ID(), unknown, false)
		case programschema.OccurrenceIndexRead:
			if count, ok := programschema.OccurrenceInputCount(row, input.OccurrenceInputs); ok && count >= 3 {
				if result, ok := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 2); ok {
					join(result, unknown, false)
				}
			}
		case programschema.OccurrenceAllocation, programschema.OccurrenceCall:
			join(row.ID(), unknown, false)
		}
	}

	components, cyclic := equationSCCs(equations)
	recurrent = make([]bool, len(equations))
	sameComponent := func(left, right identity.ContentID) bool {
		leftComponent, leftOK := components[left]
		rightComponent, rightOK := components[right]
		return leftOK && rightOK && leftComponent == rightComponent
	}
	for index, item := range equations {
		if !cyclic[item.output] {
			continue
		}
		switch item.kind {
		case equationUnaryNeg:
			recurrent[index] = sameComponent(item.output, item.left)
		case equationArithmetic:
			recurrent[index] = sameComponent(item.output, item.left) ||
				sameComponent(item.output, item.right)
		}
	}

	changed := true
	for changed {
		changed = false
		for equationIndex, item := range equations {
			isRecurrent := recurrent[equationIndex]
			switch item.kind {
			case equationCopy:
				if join(item.output, states[item.left], false) {
					changed = true
				}
			case equationUnaryNeg:
				operand := states[item.left]
				if !operand.known() {
					continue
				}
				values := operand.finite()
				if len(values) == 0 {
					if join(item.output, unknown, false) {
						changed = true
					}
					continue
				}
				if isRecurrent {
					current := states[item.output]
					if !current.unknown {
						grows, reachable := unaryFiniteGrowth(values, current.values)
						if !reachable || grows {
							if join(item.output, unknown, false) {
								changed = true
							}
						}
					}
					continue
				}
				results := unaryFiniteResults(values)
				if len(results) != 0 && join(item.output, state{values: results}, isRecurrent) {
					changed = true
				}
				if operand.unknown || len(results) == 0 {
					if join(item.output, unknown, false) {
						changed = true
					}
				}
			case equationArithmetic:
				left, right := states[item.left], states[item.right]
				if !left.known() || !right.known() {
					continue
				}
				leftValues, rightValues := left.finite(), right.finite()
				if len(leftValues) != 0 && len(rightValues) != 0 {
					if isRecurrent {
						current := states[item.output]
						if !current.unknown {
							grows, reachable := arithmeticFiniteGrowth(leftValues, rightValues, item.op, current.values)
							if !reachable || grows {
								if join(item.output, unknown, false) {
									changed = true
								}
							}
						}
						continue
					}
					results := arithmeticFiniteResults(leftValues, rightValues, item.op)
					if len(results) != 0 && join(item.output, state{values: results}, isRecurrent) {
						changed = true
					}
				}
				if left.unknown || right.unknown || len(leftValues) == 0 || len(rightValues) == 0 {
					if join(item.output, unknown, false) {
						changed = true
					}
				}
			}
		}
	}

	summaries := make([]programschema.ExactScalarSummary, 0, len(input.Occurrences)*3)
	seen := make(map[identity.ContentID]struct{}, len(input.Occurrences)*3)
	for index, row := range input.Occurrences {
		if row.Kind() != programschema.OccurrenceBinaryArithmetic {
			continue
		}
		left, right, _, ok := programschema.OccurrenceBinaryArithmeticOperands(row, input.OccurrenceInputs)
		if !ok {
			return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarUnavailable, index, -1)
		}
		uses := [...]struct {
			role    programschema.ExactScalarSummaryRole
			subject identity.ContentID
		}{
			{role: programschema.ExactScalarSummaryLeft, subject: left},
			{role: programschema.ExactScalarSummaryRight, subject: right},
			{role: programschema.ExactScalarSummaryResult, subject: row.ID()},
		}
		for _, use := range uses {
			for _, literal := range states[use.subject].finite() {
				// A finite state is a sealed complete image. Emit one
				// immutable summary per retained atom, including all
				// alternatives of a guarded operand and every result of
				// its finite Cartesian product.
				if literal.Kind != keyspace.LiteralInteger && literal.Kind != keyspace.LiteralFloat {
					continue
				}
				body, ok := row.BodyID()
				if !ok {
					return nil, programconstruction.New(programcatalog.Occurrence(), programconstruction.IssueExactScalarUnavailable, index, -1)
				}
				summary, ok := programschema.NewExactScalarSummary(row.ID(), use.subject, body, use.role, programschema.SummaryLiteral{Kind: uint8(literal.Kind), Integer: literal.Integer, FloatBits: literal.FloatBits})
				if !ok {
					return nil, programconstruction.New(programcatalog.ExactScalarSummary(), programconstruction.IssueExactScalarUnavailable, index, -1)
				}
				if _, duplicate := seen[summary.ID()]; duplicate {
					return nil, programconstruction.New(programcatalog.ExactScalarSummary(), programconstruction.IssueExactScalarUnavailable, index, -1)
				}
				seen[summary.ID()] = struct{}{}
				summaries = append(summaries, summary)
			}
		}
	}
	identity.SortByContentID(summaries, func(row programschema.ExactScalarSummary) identity.ContentID { return row.ID() })
	return &Bundle{states: states, summaries: summaries}, programconstruction.Fault{}
}
