package symboliccall

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// GlobalRoot is a stable module-owned binding. Ambient process globals are a
// different capability and remain contextual.
type GlobalRoot struct {
	Module string
	Name   string
}

func (r GlobalRoot) valid() bool {
	return r.Module != "" && r.Name != "" &&
		!strings.ContainsRune(r.Module, '\x00') && !strings.ContainsRune(r.Name, '\x00')
}
func (r GlobalRoot) key() string { return r.Module + "\x00" + r.Name }

type AllocationSite string

type AllocationSpec struct {
	Site       AllocationSite
	Initial    Expr
	Escapes    bool
	CrossActor bool
}

type LocationKind uint8

const (
	LocationCapture LocationKind = iota + 1
	LocationGlobal
	LocationAllocation
)

// SymbolicLocation is relative to one transformer invocation.
type SymbolicLocation struct {
	Kind    LocationKind
	Capture int
	Global  GlobalRoot
	Site    AllocationSite
}

type AllocationIdentity struct {
	Call string
	Site AllocationSite
}

// ConcreteLocation prevents aliasing between closure instances and between
// allocations made by different call-site instantiations.
type ConcreteLocation struct {
	Kind       LocationKind
	Closure    string
	Capture    int
	Global     GlobalRoot
	Allocation AllocationIdentity
}

type EffectWrite struct {
	Target SymbolicLocation
	Value  Expr
}

// EffectRow keeps its return tuple, allocations, writes, and returned
// references correlated under one boundary condition.
type EffectRow struct {
	Boundary    BoundaryRow
	Allocations []AllocationSpec
	Writes      []EffectWrite
	ReturnRefs  []SymbolicLocation
}

type EffectPolicy struct {
	MutableAmbientGlobals bool
	EscapingHeap          bool
	CrossActorHeap        bool
	MailboxOrActorState   bool
	Capabilities          *BoundaryCapabilityRegistry
	StateLanes            []state.LaneID
}

type EffectTransformer struct {
	reg          *axis.Registry
	params       int
	captures     int
	requirements []BoundaryRequirement
	rows         []EffectRow
	valid        bool
	contextual   string
	widened      bool
}

type EffectResult struct {
	Values     []product.Value
	References []ConcreteLocation
	Heap       map[ConcreteLocation]product.Value
}

func NewEffectTransformer(reg *axis.Registry, params, captures int, rows []EffectRow, requirements []BoundaryRequirement, policy EffectPolicy) EffectTransformer {
	t := EffectTransformer{
		reg:          reg,
		params:       params,
		captures:     captures,
		requirements: cloneBoundaryRequirements(requirements),
		rows:         cloneEffectRows(rows),
		valid:        true,
	}
	switch {
	case reg == nil || params < 0 || captures < 0:
		t.contextual = "invalid effect transformer"
	case policy.MailboxOrActorState:
		t.contextual = "mailbox or actor state"
	case policy.CrossActorHeap:
		t.contextual = "cross-actor heap"
	case policy.EscapingHeap:
		t.contextual = "escaping heap"
	case policy.MutableAmbientGlobals:
		t.contextual = "mutable ambient global"
	}
	capabilities := policy.Capabilities
	if capabilities == nil {
		capabilities = DefaultBoundaryCapabilityRegistry()
	}
	if unsupported := capabilities.unsupportedStateLanes(policy.StateLanes); t.contextual == "" && len(unsupported) != 0 {
		t.contextual = unsupportedLaneReason(unsupported)
	}
	return normalizeEffects(t)
}

func (t EffectTransformer) ContextualReason() string { return t.contextual }
func (t EffectTransformer) Widened() bool            { return t.widened }

// InstantiateEffects evaluates one transformer invocation transactionally.
// callID and closureID are ownership identities, not cache keys: rebasing a
// fresh site by callID is what prevents allocations from distinct callers from
// aliasing. The input heap is never mutated.
func (t EffectTransformer) InstantiateEffects(
	callID string,
	closureID string,
	params []product.Value,
	captures []product.Value,
	varargs []product.Value,
	globals map[GlobalRoot]product.Value,
	heap map[ConcreteLocation]product.Value,
) ([]EffectResult, error) {
	if !t.valid || t.contextual != "" {
		return nil, fmt.Errorf("symboliccall: contextual effect transformer: %s", t.contextual)
	}
	if len(params) != t.params || len(captures) != t.captures {
		return nil, fmt.Errorf("symboliccall: effect boundary shape mismatch")
	}
	if err := checkBoundaryRequirements(t.reg, t.requirements, params, captures, varargs); err != nil {
		return nil, err
	}

	var results []EffectResult
	for _, row := range t.rows {
		if !boundaryConditionMayHold(t.reg, row.Boundary.Guards, row.Boundary.VarargLength, params, captures, varargs) {
			continue
		}
		if len(row.Allocations) != 0 && callID == "" {
			return nil, fmt.Errorf("symboliccall: allocation requires call identity")
		}
		result := EffectResult{
			Values:     make([]product.Value, len(row.Boundary.Returns)),
			References: make([]ConcreteLocation, len(row.ReturnRefs)),
			Heap:       cloneHeap(heap),
		}
		for i, expr := range row.Boundary.Returns {
			value, err := evalEnvironment(t.reg, expr, params, captures, varargs, globals)
			if err != nil {
				return nil, err
			}
			result.Values[i] = value
		}
		for _, allocation := range row.Allocations {
			location := concreteAllocation(callID, allocation.Site)
			value, err := evalEnvironment(t.reg, allocation.Initial, params, captures, varargs, globals)
			if err != nil {
				return nil, err
			}
			result.Heap[location] = value
		}
		for _, write := range row.Writes {
			location, strong, err := resolveLocation(callID, closureID, write.Target)
			if err != nil {
				return nil, err
			}
			value, err := evalEnvironment(t.reg, write.Value, params, captures, varargs, globals)
			if err != nil {
				return nil, err
			}
			if strong {
				// Only a declared fresh, non-escaped allocation reaches this arm.
				result.Heap[location] = value
			} else {
				prior := initialLocationValue(t.reg, location, captures, globals, result.Heap)
				result.Heap[location] = product.Join(t.reg, prior, value)
			}
		}
		for i, ref := range row.ReturnRefs {
			location, _, err := resolveLocation(callID, closureID, ref)
			if err != nil {
				return nil, err
			}
			result.References[i] = location
		}
		results = append(results, result)
	}
	return results, nil
}

func JoinEffects(a, b EffectTransformer) EffectTransformer {
	if !a.valid {
		return b
	}
	if !b.valid {
		return a
	}
	if a.contextual != "" || b.contextual != "" {
		reason := a.contextual
		if reason == "" || b.contextual != "" && b.contextual < reason {
			reason = b.contextual
		}
		return EffectTransformer{reg: a.reg, params: a.params, captures: a.captures, valid: true, contextual: reason}
	}
	if a.reg != b.reg || a.params != b.params || a.captures != b.captures {
		return EffectTransformer{reg: a.reg, params: a.params, captures: a.captures, valid: true, contextual: "incompatible effect shape"}
	}
	requirementJoin := JoinBoundary(
		NewBoundaryTransformer(a.reg, a.params, a.captures, nil, a.requirements, BoundaryPolicy{}),
		NewBoundaryTransformer(b.reg, b.params, b.captures, nil, b.requirements, BoundaryPolicy{}),
	)
	if requirementJoin.contextual != "" {
		return EffectTransformer{reg: a.reg, params: a.params, captures: a.captures, valid: true, contextual: requirementJoin.contextual}
	}
	out := EffectTransformer{
		reg:          a.reg,
		params:       a.params,
		captures:     a.captures,
		requirements: cloneBoundaryRequirements(requirementJoin.requirements),
		rows:         append(cloneEffectRows(a.rows), cloneEffectRows(b.rows)...),
		valid:        true,
		widened:      a.widened || b.widened,
	}
	return normalizeEffects(out)
}

func WidenEffects(prev, next EffectTransformer, maxRows int) EffectTransformer {
	out := JoinEffects(prev, next)
	if out.contextual == "" && maxRows > 0 && len(out.rows) > maxRows {
		// Correlated writes cannot be safely collapsed slotwise. Budget overflow
		// therefore falls back atomically instead of inventing effect pairs.
		out.rows = nil
		out.contextual = "effect row budget"
		out.widened = true
	}
	return out
}

func EqualEffects(a, b EffectTransformer) bool {
	a, b = normalizeEffects(a), normalizeEffects(b)
	if a.valid != b.valid {
		return false
	}
	if a.contextual != "" || b.contextual != "" {
		return a.contextual != "" && b.contextual != ""
	}
	if a.reg != b.reg || a.params != b.params || a.captures != b.captures || len(a.rows) != len(b.rows) || len(a.requirements) != len(b.requirements) {
		return false
	}
	for i := range a.requirements {
		if boundaryRequirementKey(a.requirements[i]) != boundaryRequirementKey(b.requirements[i]) || !product.Equal(a.reg, a.requirements[i].Allowed, b.requirements[i].Allowed) {
			return false
		}
	}
	for i := range a.rows {
		if effectRowKey(a.reg, a.rows[i]) != effectRowKey(b.reg, b.rows[i]) {
			return false
		}
	}
	return true
}

func LessOrEqEffects(a, b EffectTransformer) bool { return EqualEffects(JoinEffects(a, b), b) }

func normalizeEffects(t EffectTransformer) EffectTransformer {
	if !t.valid || t.contextual != "" || t.reg == nil {
		return t
	}
	requirements := NewBoundaryTransformer(t.reg, t.params, t.captures, nil, t.requirements, BoundaryPolicy{})
	if requirements.contextual != "" {
		t.contextual = requirements.contextual
		return t
	}
	t.requirements = cloneBoundaryRequirements(requirements.requirements)
	t.rows = cloneEffectRows(t.rows)
	for i := range t.rows {
		row := &t.rows[i]
		boundary := NewBoundaryTransformer(t.reg, t.params, t.captures, []BoundaryRow{row.Boundary}, nil, BoundaryPolicy{})
		if boundary.contextual != "" || len(boundary.rows) != 1 {
			t.contextual = "invalid effect boundary"
			return t
		}
		row.Boundary = boundary.rows[0]
		if err := normalizeEffectRow(t.reg, t.params, t.captures, row); err != nil {
			t.contextual = err.Error()
			return t
		}
	}
	sort.Slice(t.rows, func(i, j int) bool { return effectRowKey(t.reg, t.rows[i]) < effectRowKey(t.reg, t.rows[j]) })
	rows := t.rows[:0]
	for _, row := range t.rows {
		if len(rows) == 0 || effectRowKey(t.reg, rows[len(rows)-1]) != effectRowKey(t.reg, row) {
			rows = append(rows, row)
		}
	}
	t.rows = rows
	return t
}

func normalizeEffectRow(reg *axis.Registry, params, captures int, row *EffectRow) error {
	sites := make(map[AllocationSite]struct{}, len(row.Allocations))
	expressions := cloneExprs(row.Boundary.Returns)
	for i := range row.Allocations {
		allocation := &row.Allocations[i]
		if allocation.Site == "" {
			return fmt.Errorf("invalid allocation site")
		}
		if allocation.Escapes {
			return fmt.Errorf("escaping heap")
		}
		if allocation.CrossActor {
			return fmt.Errorf("cross-actor heap")
		}
		if _, duplicate := sites[allocation.Site]; duplicate {
			return fmt.Errorf("duplicate allocation site")
		}
		sites[allocation.Site] = struct{}{}
		allocation.Initial = canonicalizeBoundaryExpr(reg, allocation.Initial)
		expressions = append(expressions, allocation.Initial)
	}
	sort.Slice(row.Allocations, func(i, j int) bool { return row.Allocations[i].Site < row.Allocations[j].Site })
	for i := range row.Writes {
		write := &row.Writes[i]
		if !validEffectLocation(write.Target, params, captures, sites) {
			return fmt.Errorf("invalid effect write target")
		}
		write.Value = canonicalizeBoundaryExpr(reg, write.Value)
		expressions = append(expressions, write.Value)
	}
	for _, ref := range row.ReturnRefs {
		if !validEffectLocation(ref, params, captures, sites) {
			return fmt.Errorf("invalid returned reference")
		}
	}
	if exprConstantCollision(reg, expressions) {
		return fmt.Errorf("constant canonical hash collision")
	}
	return nil
}

func validEffectLocation(location SymbolicLocation, _, captures int, sites map[AllocationSite]struct{}) bool {
	switch location.Kind {
	case LocationCapture:
		return location.Capture >= 0 && location.Capture < captures
	case LocationGlobal:
		return location.Global.valid()
	case LocationAllocation:
		_, ok := sites[location.Site]
		return ok
	default:
		return false
	}
}

func checkBoundaryRequirements(reg *axis.Registry, requirements []BoundaryRequirement, params, captures, varargs []product.Value) error {
	for _, requirement := range requirements {
		if !boundaryConditionMayHold(reg, requirement.Guards, requirement.VarargLength, params, captures, varargs) {
			continue
		}
		actual, ok := readRoot(reg, requirement.Root, params, captures, varargs)
		if !ok || !product.LessOrEq(reg, actual, requirement.Allowed) {
			return fmt.Errorf("symboliccall: entry requirement rejected %s", rootKey(requirement.Root))
		}
	}
	return nil
}

func resolveLocation(callID, closureID string, location SymbolicLocation) (ConcreteLocation, bool, error) {
	switch location.Kind {
	case LocationCapture:
		if closureID == "" {
			return ConcreteLocation{}, false, fmt.Errorf("symboliccall: capture write requires closure identity")
		}
		return ConcreteLocation{Kind: LocationCapture, Closure: closureID, Capture: location.Capture}, false, nil
	case LocationGlobal:
		return ConcreteLocation{Kind: LocationGlobal, Global: location.Global}, false, nil
	case LocationAllocation:
		if callID == "" {
			return ConcreteLocation{}, false, fmt.Errorf("symboliccall: allocation write requires call identity")
		}
		return concreteAllocation(callID, location.Site), true, nil
	default:
		return ConcreteLocation{}, false, fmt.Errorf("symboliccall: invalid effect location")
	}
}

func concreteAllocation(callID string, site AllocationSite) ConcreteLocation {
	return ConcreteLocation{Kind: LocationAllocation, Allocation: AllocationIdentity{Call: callID, Site: site}}
}

func initialLocationValue(reg *axis.Registry, location ConcreteLocation, captures []product.Value, globals map[GlobalRoot]product.Value, heap map[ConcreteLocation]product.Value) product.Value {
	if value, ok := heap[location]; ok {
		return value
	}
	switch location.Kind {
	case LocationCapture:
		if location.Capture >= 0 && location.Capture < len(captures) {
			return captures[location.Capture]
		}
	case LocationGlobal:
		if value, ok := globals[location.Global]; ok {
			return value
		}
	}
	return product.Bottom(reg)
}

func cloneHeap(in map[ConcreteLocation]product.Value) map[ConcreteLocation]product.Value {
	out := make(map[ConcreteLocation]product.Value, len(in))
	for location, value := range in {
		out[location] = value
	}
	return out
}

func cloneEffectRows(in []EffectRow) []EffectRow {
	out := make([]EffectRow, len(in))
	for i, row := range in {
		out[i] = row
		out[i].Boundary = cloneBoundaryRows([]BoundaryRow{row.Boundary})[0]
		out[i].Allocations = append([]AllocationSpec(nil), row.Allocations...)
		out[i].Writes = append([]EffectWrite(nil), row.Writes...)
		out[i].ReturnRefs = append([]SymbolicLocation(nil), row.ReturnRefs...)
	}
	return out
}

func effectRowKey(reg *axis.Registry, row EffectRow) string {
	var b strings.Builder
	b.WriteString(boundaryRowKey(reg, row.Boundary))
	for _, allocation := range row.Allocations {
		b.WriteString("|a:")
		b.WriteString(string(allocation.Site))
		b.WriteByte(':')
		b.WriteString(exprCanonicalKey(reg, allocation.Initial))
	}
	for _, write := range row.Writes {
		b.WriteString("|w:")
		b.WriteString(symbolicLocationKey(write.Target))
		b.WriteByte(':')
		b.WriteString(exprCanonicalKey(reg, write.Value))
	}
	for _, ref := range row.ReturnRefs {
		b.WriteString("|r:")
		b.WriteString(symbolicLocationKey(ref))
	}
	return b.String()
}

func symbolicLocationKey(location SymbolicLocation) string {
	switch location.Kind {
	case LocationCapture:
		return "c:" + strconv.Itoa(location.Capture)
	case LocationGlobal:
		return "g:" + location.Global.key()
	case LocationAllocation:
		return "a:" + string(location.Site)
	default:
		return "invalid"
	}
}
