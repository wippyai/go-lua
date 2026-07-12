package symboliccall

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
)

// GuardKind is a supported parameter predicate. The deliberately small
// vocabulary makes unknown predicates contextual rather than guessed.
type GuardKind uint8

const (
	GuardTruthy GuardKind = iota + 1
	GuardFalsy
)

type Guard struct {
	Param int
	Kind  GuardKind
}

// GuardedRow preserves correlation between all return slots under one
// conjunction. Rows are may alternatives.
type GuardedRow struct {
	Guards  []Guard
	Returns []Expr
}

// GuardedRequirement is a contravariant caller obligation. Unlike return-row
// guards, widening never makes a conditional requirement unconditional.
type GuardedRequirement struct {
	Guards []Guard
	Param  int
	Value  product.Value
}

type GuardedTransformer struct {
	reg          *axis.Registry
	params       int
	rows         []GuardedRow
	requirements []GuardedRequirement
	valid        bool
	contextual   string
	widened      bool
}

type GuardedLimits struct {
	MaxRows         int
	MaxGuardsPerRow int
}

func NewGuardedTransformer(reg *axis.Registry, params int, rows []GuardedRow, requirements []GuardedRequirement) GuardedTransformer {
	if reg == nil || params < 0 {
		return GuardedTransformer{reg: reg, params: params, valid: true, contextual: "invalid guarded transformer"}
	}
	for _, row := range rows {
		if !validGuards(row.Guards, params) {
			return GuardedTransformer{reg: reg, params: params, valid: true, contextual: "unsupported guard"}
		}
	}
	for _, requirement := range requirements {
		if requirement.Param < 0 || requirement.Param >= params || !validGuards(requirement.Guards, params) {
			return GuardedTransformer{reg: reg, params: params, valid: true, contextual: "unsupported requirement"}
		}
	}
	return normalizeGuarded(GuardedTransformer{reg: reg, params: params, rows: cloneRows(rows), requirements: cloneRequirements(requirements), valid: true})
}

func (t GuardedTransformer) ContextualReason() string { return t.contextual }
func (t GuardedTransformer) Widened() bool            { return t.widened }
func (t GuardedTransformer) RowCount() int            { return len(t.rows) }

// InstantiateRows returns feasible concrete rows without joining their slots,
// preserving correlations for the caller. Uncertain guards retain the row.
func (t GuardedTransformer) InstantiateRows(params []product.Value) ([][]product.Value, error) {
	if !t.valid || t.contextual != "" {
		return nil, fmt.Errorf("symboliccall: contextual guarded transformer: %s", t.contextual)
	}
	if len(params) != t.params {
		return nil, fmt.Errorf("symboliccall: got %d parameters, want %d", len(params), t.params)
	}
	var out [][]product.Value
	for _, row := range t.rows {
		if !guardsMayHold(t.reg, row.Guards, params) {
			continue
		}
		width := guardedReturnArity(t.rows)
		values := make([]product.Value, width)
		for i := range values {
			values[i] = product.Absent(t.reg)
		}
		for i, expr := range row.Returns {
			value, err := eval(t.reg, expr, params)
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		out = append(out, values)
	}
	return out, nil
}

// Instantiate joins feasible rows slotwise for consumers that do not use the
// correlation surface.
func (t GuardedTransformer) Instantiate(params []product.Value) ([]product.Value, error) {
	rows, err := t.InstantiateRows(params)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	out := make([]product.Value, width)
	for i := range out {
		out[i] = product.Bottom(t.reg)
	}
	for _, row := range rows {
		for i, value := range row {
			out[i] = product.Join(t.reg, out[i], value)
		}
	}
	return out, nil
}

func guardsMayHold(reg *axis.Registry, guards []Guard, params []product.Value) bool {
	for _, guard := range guards {
		if guard.Param < 0 || guard.Param >= len(params) {
			return false
		}
		switch guard.Kind {
		case GuardTruthy:
			if !valueref.CanBeTruthy(reg, params[guard.Param]) {
				return false
			}
		case GuardFalsy:
			if !valueref.CanBeFalsy(reg, params[guard.Param]) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func JoinGuarded(a, b GuardedTransformer) GuardedTransformer {
	if !a.valid {
		return b
	}
	if !b.valid {
		return a
	}
	if a.contextual != "" {
		if b.contextual != "" && b.contextual < a.contextual {
			return b
		}
		return a
	}
	if b.contextual != "" {
		return b
	}
	if a.reg != b.reg || a.params != b.params {
		return GuardedTransformer{reg: a.reg, params: a.params, valid: true, contextual: "incompatible guarded transformer"}
	}
	out := GuardedTransformer{reg: a.reg, params: a.params, valid: true, widened: a.widened || b.widened}
	out.rows = append(cloneRows(a.rows), cloneRows(b.rows)...)
	var ok bool
	out.requirements, ok = joinRequirements(a.reg, a.requirements, b.requirements)
	if !ok {
		return GuardedTransformer{reg: a.reg, params: a.params, valid: true, contextual: "contradictory requirement"}
	}
	return normalizeGuarded(out)
}

func WidenGuarded(prev, next GuardedTransformer, limits GuardedLimits) GuardedTransformer {
	out := JoinGuarded(prev, next)
	if !out.valid || out.contextual != "" {
		return out
	}
	if limits.MaxGuardsPerRow > 0 {
		for i := range out.rows {
			if len(out.rows[i].Guards) > limits.MaxGuardsPerRow {
				out.rows[i].Guards = nil // may-return guard weakening is sound
				out.widened = true
			}
		}
		for _, requirement := range out.requirements {
			if len(requirement.Guards) > limits.MaxGuardsPerRow {
				// Requirements are contravariant checker obligations. Neither
				// dropping their value nor erasing their guard is an approved
				// precision loss, so overflow falls back atomically.
				return GuardedTransformer{reg: out.reg, params: out.params, valid: true, contextual: "requirement guard budget"}
			}
		}
	}
	out = normalizeGuarded(out)
	if limits.MaxRows > 0 && len(out.rows) > limits.MaxRows {
		width := 0
		for _, row := range out.rows {
			if len(row.Returns) > width {
				width = len(row.Returns)
			}
		}
		returns := make([]Expr, width)
		for i := range width {
			var alternatives []Expr
			for _, row := range out.rows {
				if i < len(row.Returns) {
					alternatives = append(alternatives, row.Returns[i])
				}
			}
			returns[i] = Join(alternatives...)
		}
		out.rows = []GuardedRow{{Returns: returns}}
		out.widened = true
	}
	return normalizeGuarded(out)
}

func LessOrEqGuarded(a, b GuardedTransformer) bool { return EqualGuarded(JoinGuarded(a, b), b) }

func EqualGuarded(a, b GuardedTransformer) bool {
	a, b = normalizeGuarded(a), normalizeGuarded(b)
	if a.valid != b.valid {
		return false
	}
	if a.contextual != "" || b.contextual != "" {
		return a.contextual != "" && b.contextual != ""
	}
	if a.reg != b.reg || a.params != b.params || len(a.rows) != len(b.rows) || len(a.requirements) != len(b.requirements) {
		return false
	}
	usedRows := make([]bool, len(b.rows))
	for _, left := range a.rows {
		found := false
		for j, right := range b.rows {
			if !usedRows[j] && rowEqual(left, right) {
				usedRows[j], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	usedRequirements := make([]bool, len(b.requirements))
	for _, left := range a.requirements {
		found := false
		for j, right := range b.requirements {
			if !usedRequirements[j] && requirementEqual(a.reg, left, right) {
				usedRequirements[j], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func validGuards(guards []Guard, params int) bool {
	for _, guard := range guards {
		if guard.Param < 0 || guard.Param >= params || guard.Kind != GuardTruthy && guard.Kind != GuardFalsy {
			return false
		}
	}
	return true
}

func normalizeGuarded(t GuardedTransformer) GuardedTransformer {
	rows := t.rows[:0]
	for _, row := range t.rows {
		var satisfiable bool
		row.Guards, satisfiable = normalizeGuardSet(row.Guards)
		if satisfiable {
			rows = append(rows, row)
		}
	}
	t.rows = rows
	requirements := t.requirements[:0]
	for _, requirement := range t.requirements {
		if product.Equal(t.reg, requirement.Value, product.Bottom(t.reg)) {
			t.requirements = nil
			t.contextual = "contradictory requirement"
			return t
		}
		if product.Equal(t.reg, requirement.Value, product.Top()) {
			continue
		}
		var satisfiable bool
		requirement.Guards, satisfiable = normalizeGuardSet(requirement.Guards)
		if satisfiable {
			requirements = append(requirements, requirement)
		}
	}
	t.requirements = requirements
	sort.Slice(t.rows, func(i, j int) bool { return rowLess(t.reg, t.rows[i], t.rows[j]) })
	dedupRows := t.rows[:0]
	for _, row := range t.rows {
		if len(dedupRows) == 0 || !rowEqual(dedupRows[len(dedupRows)-1], row) {
			dedupRows = append(dedupRows, row)
		}
	}
	keptRows := dedupRows[:0]
	for i, candidate := range dedupRows {
		subsumed := false
		for j, other := range dedupRows {
			if i != j && rowSubsumes(other, candidate) {
				subsumed = true
				break
			}
		}
		if !subsumed {
			keptRows = append(keptRows, candidate)
		}
	}
	t.rows = keptRows
	sort.Slice(t.requirements, func(i, j int) bool { return requirementLess(t.reg, t.requirements[i], t.requirements[j]) })
	return t
}

func normalizeGuardSet(in []Guard) ([]Guard, bool) {
	out := append([]Guard(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Param != out[j].Param {
			return out[i].Param < out[j].Param
		}
		return out[i].Kind < out[j].Kind
	})
	dedup := out[:0]
	for _, guard := range out {
		if len(dedup) != 0 && dedup[len(dedup)-1].Param == guard.Param && dedup[len(dedup)-1].Kind != guard.Kind {
			return nil, false
		}
		if len(dedup) == 0 || dedup[len(dedup)-1] != guard {
			dedup = append(dedup, guard)
		}
	}
	return dedup, true
}

func joinRequirements(reg *axis.Registry, a, b []GuardedRequirement) ([]GuardedRequirement, bool) {
	out := cloneRequirements(a)
	for _, incoming := range b {
		found := false
		for i := range out {
			if out[i].Param == incoming.Param && guardsEqual(out[i].Guards, incoming.Guards) {
				out[i].Value = product.Meet(reg, out[i].Value, incoming.Value)
				if product.Equal(reg, out[i].Value, product.Bottom(reg)) {
					return nil, false
				}
				found = true
				break
			}
		}
		if !found {
			out = append(out, incoming)
		}
	}
	return out, true
}

func guardedReturnArity(rows []GuardedRow) int {
	width := 0
	for _, row := range rows {
		if len(row.Returns) > width {
			width = len(row.Returns)
		}
	}
	return width
}

func cloneRows(in []GuardedRow) []GuardedRow {
	out := make([]GuardedRow, len(in))
	for i, row := range in {
		out[i] = GuardedRow{Guards: append([]Guard(nil), row.Guards...), Returns: cloneExprs(row.Returns)}
	}
	return out
}

func cloneRequirements(in []GuardedRequirement) []GuardedRequirement {
	out := make([]GuardedRequirement, len(in))
	for i, requirement := range in {
		out[i] = requirement
		out[i].Guards = append([]Guard(nil), requirement.Guards...)
	}
	return out
}

func guardsEqual(a, b []Guard) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func rowEqual(a, b GuardedRow) bool {
	return guardsEqual(a.Guards, b.Guards) && exprSliceEqual(a.Returns, b.Returns)
}
func rowSubsumes(superset, candidate GuardedRow) bool {
	if !guardsSubset(superset.Guards, candidate.Guards) || len(superset.Returns) != len(candidate.Returns) {
		return false
	}
	for i := range candidate.Returns {
		if !exprEqual(Join(candidate.Returns[i], superset.Returns[i]), superset.Returns[i]) {
			return false
		}
	}
	return true
}
func guardsSubset(a, b []Guard) bool {
	for _, want := range a {
		found := false
		for _, got := range b {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func rowLess(reg *axis.Registry, a, b GuardedRow) bool {
	if c := compareGuards(a.Guards, b.Guards); c != 0 {
		return c < 0
	}
	return rowHash(reg, a) < rowHash(reg, b)
}
func rowHash(reg *axis.Registry, row GuardedRow) uint64 {
	var h uint64
	for _, expr := range row.Returns {
		if expr.n != nil && expr.n.op == opConst {
			h ^= product.Hash(reg, expr.n.value) + 0x9e3779b97f4a7c15 + (h << 6) + (h >> 2)
		}
	}
	return h
}
func compareGuards(a, b []Guard) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i].Param != b[i].Param {
			if a[i].Param < b[i].Param {
				return -1
			}
			return 1
		}
		if a[i].Kind != b[i].Kind {
			if a[i].Kind < b[i].Kind {
				return -1
			}
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}
func requirementEqual(reg *axis.Registry, a, b GuardedRequirement) bool {
	return a.Param == b.Param && guardsEqual(a.Guards, b.Guards) && product.Equal(reg, a.Value, b.Value)
}
func requirementLess(reg *axis.Registry, a, b GuardedRequirement) bool {
	if c := compareGuards(a.Guards, b.Guards); c != 0 {
		return c < 0
	}
	if a.Param != b.Param {
		return a.Param < b.Param
	}
	return product.Hash(reg, a.Value) < product.Hash(reg, b.Value)
}
