// Package values owns Lua expression-list type assembly over canonical Link
// Values. Equations are immutable domain data for the one Type Factor law.
package values

import (
	"sort"

	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/carrier"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/semantic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// Equation is one exact Program Values law:
//
//	result = Scalar(fixed[0]) · ... · Scalar(fixed[n]) · tail
//
// tail is absent exactly when hasTail is false. Link Values are the only
// operands, so raw Program Terms never cross a shard boundary.
type Equation struct {
	result      link.Value
	groups      []link.Value
	fixedGroups []uint32
	tailGroup   uint32
	hasTail     bool
}

// Equations is the closed finite Values equation set for one Link.  It owns
// no fact storage, solver identity, or type admission authority.
type Equations struct{ entries []Equation }

// Build extracts all Values equations from a sealed Link.  It performs only
// cold structural validation; Pack alternatives remain a hot fact concern.
func Build(source *link.Link) (*Equations, bool) {
	if source == nil || !source.ContentID().Available() {
		return nil, false
	}
	result := &Equations{}
	for shardIndex := 0; shardIndex < source.ShardCount(); shardIndex++ {
		shard, ok := source.ShardAt(shardIndex)
		if !ok {
			return nil, false
		}
		p, ok := source.Program(shard)
		if !ok || p == nil || !result.appendProgram(source, shard, p) {
			return nil, false
		}
	}
	return result, true
}

func (set *Equations) Count() int {
	if set == nil {
		return 0
	}
	return len(set.entries)
}

func (set *Equations) At(index int) (Equation, bool) {
	if set == nil || index < 0 || index >= len(set.entries) {
		return Equation{}, false
	}
	return set.entries[index], true
}

func (equation Equation) Result() link.Value { return equation.result }

func (equation Equation) FixedCount() int { return len(equation.fixedGroups) }

func (equation Equation) FixedAt(index int) (link.Value, bool) {
	if index < 0 || index >= len(equation.fixedGroups) {
		return 0, false
	}
	group := int(equation.fixedGroups[index])
	if group < 0 || group >= len(equation.groups) || uint32(group) != equation.fixedGroups[index] {
		return 0, false
	}
	return equation.groups[group], true
}

func (equation Equation) Tail() (link.Value, bool) {
	if !equation.hasTail {
		return 0, false
	}
	group := int(equation.tailGroup)
	if group < 0 || group >= len(equation.groups) || uint32(group) != equation.tailGroup {
		return 0, false
	}
	return equation.groups[group], true
}

// Evaluate is the sole Values transfer law. It projects finite carrier Data
// for Pack assembly and constructs a Data-only result. Origins are deliberately
// not propagated through expression evaluation: the next Cell/Read slice owns
// source evidence. This keeps the current literal/Values cut precise without
// pretending a flat Origin set can express grouped result-position selection.
func (equation Equation) Evaluate(table *typedomain.Table, universe *origin.Universe, empty typedomain.Pack, read func(int, link.Value) (carrier.Value, bool), scratch []typedomain.Pack) (carrier.Value, bool) {
	if table == nil || !table.Sealed() || universe == nil || read == nil || len(scratch) < len(equation.groups) {
		return carrier.Value{}, false
	}
	top := false
	readData := func(index int, key link.Value) (typedomain.Pack, bool) {
		value, ok := read(index, key)
		if !ok {
			return typedomain.Pack{}, false
		}
		if value.IsTop() {
			top = true
			return empty, true
		}
		data, ok := value.Data()
		if !ok {
			return typedomain.Pack{}, false
		}
		return data, true
	}
	for index, key := range equation.groups {
		value, ok := readData(index, key)
		if !ok {
			return carrier.Value{}, false
		}
		scratch[index] = value
	}
	if top {
		return carrier.Top(table, universe)
	}
	data, ok := typedomain.AssembleGrouped(scratch[:len(equation.groups)], equation.fixedGroups, equation.tailGroup, equation.hasTail, empty)
	if !ok {
		return carrier.Value{}, false
	}
	return carrier.New(table, universe, data, origin.Empty())
}

func (set *Equations) appendProgram(source *link.Link, shard link.Shard, p *program.Program) bool {
	for index := 0; index < p.ValuesCount(); index++ {
		term, ok := p.ValuesAt(index)
		if !ok {
			return false
		}
		equation, ok := build(source, shard, p, term)
		if !ok {
			return false
		}
		set.entries = append(set.entries, equation)
	}
	return true
}

func build(source *link.Link, shard link.Shard, p *program.Program, term program.Term) (Equation, bool) {
	result, ok := source.ValueOf(shard, term)
	if !ok {
		return Equation{}, false
	}
	fixedLen, ok := p.ValuesLen(term)
	if !ok {
		return Equation{}, false
	}
	fixed := make([]link.Value, fixedLen)
	for index := range fixed {
		value, ok := p.Value(term, index)
		if !ok {
			return Equation{}, false
		}
		fixed[index], ok = source.ValueOf(shard, value)
		if !ok {
			return Equation{}, false
		}
	}
	_, tail, ok := p.Values(term)
	if !ok {
		return Equation{}, false
	}
	operands := append([]link.Value(nil), fixed...)
	final := link.Value(0)
	hasTail := tail != 0
	if hasTail {
		final, ok = source.ValueOf(shard, tail)
		if !ok {
			return Equation{}, false
		}
		operands = append(operands, final)
	}
	groups := canonicalGroups(operands)
	fixedGroups := make([]uint32, len(fixed))
	for index, value := range fixed {
		group, ok := groupOf(groups, value)
		if !ok {
			return Equation{}, false
		}
		fixedGroups[index] = group
	}
	equation := Equation{result: result, groups: groups, fixedGroups: fixedGroups, hasTail: hasTail}
	if hasTail {
		group, ok := groupOf(groups, final)
		if !ok {
			return Equation{}, false
		}
		equation.tailGroup = group
	}
	return equation, true
}

func canonicalGroups(operands []link.Value) []link.Value {
	if len(operands) == 0 {
		return nil
	}
	groups := append([]link.Value(nil), operands...)
	sort.Slice(groups, func(left, right int) bool { return groups[left] < groups[right] })
	end := 0
	for _, value := range groups {
		if end == 0 || groups[end-1] != value {
			groups[end] = value
			end++
		}
	}
	return groups[:end]
}

func groupOf(groups []link.Value, value link.Value) (uint32, bool) {
	index := sort.Search(len(groups), func(index int) bool { return groups[index] >= value })
	if index < 0 || index >= len(groups) || groups[index] != value || uint64(index) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(index), true
}

// Declare binds the one domain equation set to the current public Factor.
// The execution closure is deliberately a thin Access reader/writer around
// Equation.Evaluate; it does not restate Lua Values semantics.
func Declare(solver *engine.Solver, source *link.Link, table *typedomain.Table, universe *origin.Universe, factor *engine.Factor[link.Value, carrier.Value]) bool {
	if solver == nil || source == nil || table == nil || !table.Sealed() || universe == nil || factor == nil {
		return false
	}
	empty, ok := table.Closed()
	if !ok {
		return false
	}
	equations, ok := Build(source)
	if !ok {
		return false
	}
	for index := 0; index < equations.Count(); index++ {
		equation, ok := equations.At(index)
		if !ok || !declare(solver, source, table, universe, factor, empty, equation) {
			return false
		}
	}
	return true
}

func declare(solver *engine.Solver, source *link.Link, table *typedomain.Table, universe *origin.Universe, factor *engine.Factor[link.Value, carrier.Value], empty typedomain.Pack, equation Equation) bool {
	shard, term, ok := source.ValueOrigin(equation.Result())
	if !ok {
		return false
	}
	// The serial current solver never reenters a Rule callback. This scratch is
	// declaration-owned and cannot escape Equation.Evaluate.
	scratch := make([]typedomain.Pack, len(equation.groups))
	reads := make([]engine.ReadRef[link.Value, carrier.Value], len(equation.groups))
	rule, ok := engine.DeclareRule(solver, factor, semantic.Values(source, equation.Result()), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, term)
	}, func(access engine.Access[link.Value, carrier.Value]) bool {
		assembled, valid := equation.Evaluate(table, universe, empty, func(index int, key link.Value) (carrier.Value, bool) {
			if index < 0 || index >= len(reads) {
				return carrier.Value{}, false
			}
			value, _, valid := engine.ReadAt(access, reads[index], key)
			return value, valid
		}, scratch)
		return valid && access.Set(equation.Result(), assembled)
	})
	if !ok {
		return false
	}
	if !engine.WriteExact(rule, equation.Result()) {
		return false
	}
	for index, key := range equation.groups {
		ref, ok := engine.ReadExact(rule, 0, factor, key)
		if !ok {
			return false
		}
		reads[index] = ref
	}
	return true
}
