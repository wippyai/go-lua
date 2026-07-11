package symbolic

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
)

// ReturnCase is one possible result for a return slot.  Cases are may facts:
// each case describes a behaviour which the body may produce.
type ReturnCase struct {
	Guard GuardSet
	Expr  ExprID
}

// Requirement is a caller-visible value requirement on a symbolic path.  The
// Value expression normally evaluates to a product constraint.  Requirements
// use the existing summary convention: alternatives form a may set, while
// multiple constraints on one path are combined by the consumer as product
// requirements (and therefore become weaker under summary join).
type Requirement struct {
	Path  Path
	Value ExprID
	Guard GuardSet
}

// HeapDelta is a write performed by the function.  Path is deliberately a
// symbolic root/path, never a caller heap snapshot.
type HeapDelta struct {
	Path  Path
	Value ExprID
	Guard GuardSet
}

// CalleeRef names an unresolved callee by its body identity only.  In
// particular it contains no caller-entry or caller-value digest.
type CalleeRef struct {
	BodyDigest uint64
}

// CallNode records an unresolved call for later SCC composition.
type CallNode struct {
	Callee CalleeRef
	Args   []ExprID
	Guard  GuardSet
}

// WidenReason names a precision collapse.  Events are diagnostic metadata and
// do not take part in semantic equality or the content digest.
type WidenReason string

const (
	WidenCasesPerSlot WidenReason = "cases-per-slot"
	WidenGuardsPerSet WidenReason = "guards-per-set"
	WidenExprs        WidenReason = "expressions"
)

// WidenEvent makes every cap-driven collapse observable. Count is aggregated
// by reason so repeated normalisation remains deterministic.
type WidenEvent struct {
	Reason WidenReason
	Count  uint32
}

// Limits controls the bounded transformer domain. A non-positive limit uses
// the corresponding default.
type Limits struct {
	CapCasesPerSlot int
	CapGuardsPerSet int
	CapExprs        int
}

const (
	CapCasesPerSlot = 32
	CapGuardsPerSet = 8
	CapExprs        = 512
)

func (l Limits) normalized() Limits {
	if l.CapCasesPerSlot <= 0 {
		l.CapCasesPerSlot = CapCasesPerSlot
	}
	if l.CapGuardsPerSet <= 0 {
		l.CapGuardsPerSet = CapGuardsPerSet
	}
	if l.CapExprs <= 0 {
		l.CapExprs = CapExprs
	}
	return l
}

// Transformer is a summary-local symbolic transformer. Expr ids are local to
// Exprs; Join always re-interns ids into a fresh table before combining facts.
type Transformer struct {
	Exprs        *Exprs
	Returns      [][]ReturnCase
	Requirements []Requirement
	HeapDeltas   []HeapDelta
	CallNodes    []CallNode
	WidenEvents  []WidenEvent
}

// NewTransformer creates the empty transformer for reg.
func NewTransformer(reg *axis.Registry) Transformer { return Transformer{Exprs: NewExprs(reg)} }

func transformerRegistry(t Transformer) *axis.Registry {
	if t.Exprs == nil {
		return nil
	}
	return t.Exprs.reg
}

func ensureExprs(t Transformer, reg *axis.Registry) Transformer {
	if t.Exprs == nil {
		t.Exprs = NewExprs(reg)
	}
	return t
}

// Canonicalize defensively normalizes the fact lanes and deterministically
// garbage-collects/re-interns the reachable expression DAG.
func Canonicalize(t Transformer) Transformer {
	reg := transformerRegistry(t)
	t = ensureExprs(t, reg)
	// Normalize all non-id fields before they participate in traversal order.
	for i := range t.Returns {
		for j := range t.Returns[i] {
			t.Returns[i][j].Guard = NormalizeGuards(t.Returns[i][j].Guard)
		}
	}
	for i := range t.Requirements {
		t.Requirements[i].Guard = NormalizeGuards(t.Requirements[i].Guard)
	}
	for i := range t.HeapDeltas {
		t.HeapDeltas[i].Guard = NormalizeGuards(t.HeapDeltas[i].Guard)
	}
	for i := range t.CallNodes {
		t.CallNodes[i].Guard = NormalizeGuards(t.CallNodes[i].Guard)
	}

	// Sort by semantic spelling, rather than table-local ids, before rebuilding.
	for i := range t.Returns {
		sortReturnCases(t.Exprs, t.Returns[i])
	}
	sortRequirements(t.Exprs, t.Requirements)
	sortHeapDeltas(t.Exprs, t.HeapDeltas)
	sortCallNodes(t.Exprs, t.CallNodes)

	out := NewTransformer(reg)
	memo := make(map[ExprID]ExprID)
	var importExpr func(ExprID) ExprID
	importExpr = func(id ExprID) ExprID {
		if got, ok := memo[id]; ok {
			return got
		}
		x := t.Exprs.At(id)
		if x.Op == OpJoin || x.Op == OpMeet {
			sort.Slice(x.Args, func(i, j int) bool { return exprSpelling(t.Exprs, x.Args[i]) < exprSpelling(t.Exprs, x.Args[j]) })
		}
		for i := range x.Args {
			x.Args[i] = importExpr(x.Args[i])
		}
		got := out.Exprs.Intern(x)
		memo[id] = got
		return got
	}
	out.Returns = make([][]ReturnCase, len(t.Returns))
	for i, cases := range t.Returns {
		for _, c := range cases {
			if c.Expr != 0 {
				out.Returns[i] = append(out.Returns[i], ReturnCase{Guard: cloneGuards(c.Guard), Expr: importExpr(c.Expr)})
			}
		}
		sortReturnCases(out.Exprs, out.Returns[i])
		out.Returns[i] = dedupReturnCases(out.Exprs, out.Returns[i])
	}
	for _, r := range t.Requirements {
		if r.Path.Valid() && r.Value != 0 {
			out.Requirements = append(out.Requirements, Requirement{Path: clonePath(r.Path), Value: importExpr(r.Value), Guard: cloneGuards(r.Guard)})
		}
	}
	for _, d := range t.HeapDeltas {
		if d.Path.Valid() && d.Value != 0 {
			out.HeapDeltas = append(out.HeapDeltas, HeapDelta{Path: clonePath(d.Path), Value: importExpr(d.Value), Guard: cloneGuards(d.Guard)})
		}
	}
	for _, n := range t.CallNodes {
		args := make([]ExprID, len(n.Args))
		for i, a := range n.Args {
			args[i] = importExpr(a)
		}
		out.CallNodes = append(out.CallNodes, CallNode{Callee: n.Callee, Args: args, Guard: cloneGuards(n.Guard)})
	}
	sortRequirements(out.Exprs, out.Requirements)
	out.Requirements = dedupRequirements(out.Exprs, out.Requirements)
	sortHeapDeltas(out.Exprs, out.HeapDeltas)
	out.HeapDeltas = dedupHeapDeltas(out.Exprs, out.HeapDeltas)
	sortCallNodes(out.Exprs, out.CallNodes)
	out.CallNodes = dedupCallNodes(out.Exprs, out.CallNodes)
	out.WidenEvents = normalizeEvents(t.WidenEvents)
	return out
}

// Clone returns an independent canonical copy.
func (t Transformer) Clone() Transformer { return Canonicalize(t) }

// Equal reports semantic equality. Widen event metadata is intentionally not
// part of equality, matching summary digest treatment of non-semantic metadata.
func (t Transformer) Equal(other Transformer) bool {
	a, b := Canonicalize(t), Canonicalize(other)
	if len(a.Returns) != len(b.Returns) || len(a.Requirements) != len(b.Requirements) || len(a.HeapDeltas) != len(b.HeapDeltas) || len(a.CallNodes) != len(b.CallNodes) || a.Exprs.Len() != b.Exprs.Len() {
		return false
	}
	for i := range a.Returns {
		if len(a.Returns[i]) != len(b.Returns[i]) {
			return false
		}
		for j := range a.Returns[i] {
			if !a.Returns[i][j].Guard.Equal(b.Returns[i][j].Guard) || a.Returns[i][j].Expr != b.Returns[i][j].Expr {
				return false
			}
		}
	}
	for i := range a.Requirements {
		if !equalRequirement(a.Exprs, a.Requirements[i], b.Exprs, b.Requirements[i]) {
			return false
		}
	}
	for i := range a.HeapDeltas {
		if !equalDelta(a.Exprs, a.HeapDeltas[i], b.Exprs, b.HeapDeltas[i]) {
			return false
		}
	}
	for i := range a.CallNodes {
		if !equalCall(a.Exprs, a.CallNodes[i], b.Exprs, b.CallNodes[i]) {
			return false
		}
	}
	for i := ExprID(1); int(i) <= a.Exprs.Len(); i++ {
		if exprSpelling(a.Exprs, i) != exprSpelling(b.Exprs, i) {
			return false
		}
	}
	return true
}

// Join is the may-union of every transformer lane. Expression tables are
// re-interned, so an ExprID can never escape its owning transformer.
func Join(a, b Transformer) Transformer { return join(a, b, false) }

func join(a, b Transformer, widening bool) Transformer {
	reg := transformerRegistry(a)
	if reg == nil {
		reg = transformerRegistry(b)
	}
	a, b = ensureExprs(a, reg), ensureExprs(b, reg)
	out := NewTransformer(reg)
	appendImported(&out, a)
	appendImported(&out, b)
	if widening {
		out.WidenEvents = mergeEvents(a.WidenEvents, b.WidenEvents)
	}
	return Canonicalize(out)
}

func appendImported(out *Transformer, in Transformer) {
	memo := map[ExprID]ExprID{}
	var cp func(ExprID) ExprID
	cp = func(id ExprID) ExprID {
		if got, ok := memo[id]; ok {
			return got
		}
		x := in.Exprs.At(id)
		for i := range x.Args {
			x.Args[i] = cp(x.Args[i])
		}
		got := out.Exprs.Intern(x)
		memo[id] = got
		return got
	}
	if len(in.Returns) > len(out.Returns) {
		out.Returns = append(out.Returns, make([][]ReturnCase, len(in.Returns)-len(out.Returns))...)
	}
	for i, cases := range in.Returns {
		for _, c := range cases {
			out.Returns[i] = append(out.Returns[i], ReturnCase{Guard: cloneGuards(c.Guard), Expr: cp(c.Expr)})
		}
	}
	for _, r := range in.Requirements {
		out.Requirements = append(out.Requirements, Requirement{Path: clonePath(r.Path), Value: cp(r.Value), Guard: cloneGuards(r.Guard)})
	}
	for _, d := range in.HeapDeltas {
		out.HeapDeltas = append(out.HeapDeltas, HeapDelta{Path: clonePath(d.Path), Value: cp(d.Value), Guard: cloneGuards(d.Guard)})
	}
	for _, n := range in.CallNodes {
		args := make([]ExprID, len(n.Args))
		for i, v := range n.Args {
			args[i] = cp(v)
		}
		out.CallNodes = append(out.CallNodes, CallNode{Callee: n.Callee, Args: args, Guard: cloneGuards(n.Guard)})
	}
}

// Widen accelerates a growing transformer chain using the default limits.
func Widen(prev, next Transformer) Transformer { return WidenWithLimits(prev, next, Limits{}) }

// WidenWithLimits is Join plus product widening for matching constant output
// cases and cap-driven, sound over-approximation. It never truncates an output
// behaviour: output caps produce an unguarded joined value, and the Expr cap
// replaces values by Top when that is needed to actually bound the DAG.
func WidenWithLimits(prev, next Transformer, limits Limits) Transformer {
	limits = limits.normalized()
	reg := transformerRegistry(prev)
	if reg == nil {
		reg = transformerRegistry(next)
	}
	prev, next = Canonicalize(ensureExprs(prev, reg)), Canonicalize(ensureExprs(next, reg))
	out := join(prev, next, true)
	// Replace matching constant output cases with product.Widen(prev, next).
	for slot := range out.Returns {
		for i := range out.Returns[slot] {
			c := &out.Returns[slot][i]
			p, pok := returnCase(prev, slot, c.Guard)
			n, nok := returnCase(next, slot, c.Guard)
			if !pok || !nok {
				continue
			}
			px, nx := prev.Exprs.At(p.Expr), next.Exprs.At(n.Expr)
			if px.Op == OpConst && nx.Op == OpConst {
				c.Expr = out.Exprs.Intern(ValueExpr{Op: OpConst, Const: product.Widen(reg, px.Const, nx.Const)})
			}
		}
	}
	out = Canonicalize(out)
	var events []WidenEvent
	if collapseLongGuards(&out, limits.CapGuardsPerSet) {
		events = append(events, WidenEvent{Reason: WidenGuardsPerSet, Count: 1})
	}
	for i := range out.Returns {
		if len(out.Returns[i]) > limits.CapCasesPerSlot {
			args := make([]ExprID, len(out.Returns[i]))
			for j, c := range out.Returns[i] {
				args[j] = c.Expr
			}
			out.Returns[i] = []ReturnCase{{Expr: out.Exprs.Intern(ValueExpr{Op: OpJoin, Args: args})}}
			events = append(events, WidenEvent{Reason: WidenCasesPerSlot, Count: 1})
		}
	}
	out = Canonicalize(out)
	if out.Exprs.Len() > limits.CapExprs {
		top := out.Exprs.Intern(ValueExpr{Op: OpConst, Const: product.Top()})
		for i := range out.Returns {
			if len(out.Returns[i]) != 0 {
				out.Returns[i] = []ReturnCase{{Expr: top}}
			}
		}
		// Requirements keep their path/value fact; Top is the status-quo
		// unconstrained obligation, and guards have already been widened away.
		for i := range out.Requirements {
			out.Requirements[i].Value = top
		}
		for i := range out.HeapDeltas {
			out.HeapDeltas[i].Value = top
		}
		for i := range out.CallNodes {
			for j := range out.CallNodes[i].Args {
				out.CallNodes[i].Args[j] = top
			}
		}
		events = append(events, WidenEvent{Reason: WidenExprs, Count: 1})
	}
	out.WidenEvents = mergeEvents(out.WidenEvents, events)
	return Canonicalize(out)
}

func returnCase(t Transformer, slot int, guard GuardSet) (ReturnCase, bool) {
	if slot >= len(t.Returns) {
		return ReturnCase{}, false
	}
	for _, c := range t.Returns[slot] {
		if c.Guard.Equal(guard) {
			return c, true
		}
	}
	return ReturnCase{}, false
}

func collapseLongGuards(t *Transformer, cap int) bool {
	changed := false
	for i := range t.Returns {
		for j := range t.Returns[i] {
			if len(t.Returns[i][j].Guard) > cap {
				t.Returns[i][j].Guard = nil
				changed = true
			}
		}
	}
	for i := range t.Requirements {
		if len(t.Requirements[i].Guard) > cap {
			t.Requirements[i].Guard = nil
			changed = true
		}
	}
	for i := range t.HeapDeltas {
		if len(t.HeapDeltas[i].Guard) > cap {
			t.HeapDeltas[i].Guard = nil
			changed = true
		}
	}
	for i := range t.CallNodes {
		if len(t.CallNodes[i].Guard) > cap {
			t.CallNodes[i].Guard = nil
			changed = true
		}
	}
	return changed
}

// LessOrEq is the set inclusion order induced by Join for canonical may lanes.
func LessOrEq(a, b Transformer) bool { return Join(a, b).Equal(b) }

// Digest is a deterministic semantic content digest. Like Equal, it excludes
// WidenEvents because they describe how a value was reached, not the value.
func Digest(t Transformer) uint64 {
	t = Canonicalize(t)
	w := internalhash.NewWriter()
	_, _ = w.WriteString("symbolic-transformer-v1")
	write := func(s string) { _, _ = w.WriteString(s); _ = w.WriteByte(0) }
	writeInt := func(n int) { w.WriteIntDecimal(int64(n)); _ = w.WriteByte(0) }
	writeInt(t.Exprs.Len())
	for id := ExprID(1); int(id) <= t.Exprs.Len(); id++ {
		write(exprSpelling(t.Exprs, id))
	}
	writeInt(len(t.Returns))
	for _, cases := range t.Returns {
		writeInt(len(cases))
		for _, c := range cases {
			write(guardSpelling(c.Guard))
			writeInt(int(c.Expr))
		}
	}
	writeInt(len(t.Requirements))
	for _, r := range t.Requirements {
		write(r.Path.String())
		write(guardSpelling(r.Guard))
		writeInt(int(r.Value))
	}
	writeInt(len(t.HeapDeltas))
	for _, d := range t.HeapDeltas {
		write(d.Path.String())
		write(guardSpelling(d.Guard))
		writeInt(int(d.Value))
	}
	writeInt(len(t.CallNodes))
	for _, n := range t.CallNodes {
		w.WriteUintDecimal(n.Callee.BodyDigest)
		_ = w.WriteByte(0)
		write(guardSpelling(n.Guard))
		writeInt(len(n.Args))
		for _, a := range n.Args {
			writeInt(int(a))
		}
	}
	return w.Sum64()
}

func clonePath(p Path) Path {
	return Path{Root: p.Root, Segments: append([]segment.Segment(nil), p.Segments...)}
}

func cloneGuards(in GuardSet) GuardSet { return append(GuardSet(nil), in...) }

func guardSpelling(gs GuardSet) string {
	if len(gs) == 0 {
		return ""
	}
	b := make([]byte, 0, len(gs)*8)
	for _, g := range gs {
		b = append(b, byte(g.Op))
		b = append(b, g.Path.String()...)
		b = append(b, 0)
	}
	return string(b)
}

func exprSpelling(e *Exprs, id ExprID) string {
	seen := map[ExprID]bool{}
	var format func(ExprID) string
	format = func(id ExprID) string {
		if seen[id] {
			return "<cycle>"
		}
		seen[id] = true
		defer delete(seen, id)
		x := e.At(id)
		b := []byte{byte(x.Op), 0}
		switch x.Op {
		case OpConst:
			b = append(b, "const:"...)
			b = appendUint(b, product.Hash(e.reg, x.Const))
		case OpRead:
			b = append(b, "read:"...)
			b = append(b, x.Path.String()...)
		case OpJoin, OpMeet, OpNarrow:
			args := make([]string, len(x.Args))
			for i, arg := range x.Args {
				args[i] = format(arg)
			}
			if x.Op == OpJoin || x.Op == OpMeet {
				sort.Strings(args)
			}
			for _, arg := range args {
				b = append(b, arg...)
				b = append(b, 0)
			}
			if x.Op == OpNarrow {
				b = append(b, "narrow:"...)
				b = appendUint(b, product.Hash(e.reg, x.Const))
			}
		case OpCallResult:
			b = appendUint(b, uint64(x.Call))
			b = append(b, ':')
			b = appendUint(b, uint64(x.Slot))
		case OpAllocation:
			b = appendUint(b, uint64(x.Slot))
		default:
			b = append(b, "invalid"...)
		}
		return string(b)
	}
	return format(id)
}

func appendUint(dst []byte, n uint64) []byte {
	if n == 0 {
		return append(dst, '0')
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return append(dst, buf[i:]...)
}

func sortReturnCases(e *Exprs, in []ReturnCase) {
	sort.Slice(in, func(i, j int) bool {
		if !in[i].Guard.Equal(in[j].Guard) {
			return in[i].Guard.Less(in[j].Guard)
		}
		return exprSpelling(e, in[i].Expr) < exprSpelling(e, in[j].Expr)
	})
}
func dedupReturnCases(e *Exprs, in []ReturnCase) []ReturnCase {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for _, x := range in {
		if len(out) == 0 || !out[len(out)-1].Guard.Equal(x.Guard) || exprSpelling(e, out[len(out)-1].Expr) != exprSpelling(e, x.Expr) {
			out = append(out, x)
		}
	}
	return out
}

func sortRequirements(e *Exprs, in []Requirement) {
	sort.Slice(in, func(i, j int) bool {
		if !in[i].Path.Equal(in[j].Path) {
			return in[i].Path.Less(in[j].Path)
		}
		if !in[i].Guard.Equal(in[j].Guard) {
			return in[i].Guard.Less(in[j].Guard)
		}
		return exprSpelling(e, in[i].Value) < exprSpelling(e, in[j].Value)
	})
}
func dedupRequirements(e *Exprs, in []Requirement) []Requirement {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for _, x := range in {
		if len(out) == 0 || !equalRequirement(e, out[len(out)-1], e, x) {
			out = append(out, x)
		}
	}
	return out
}
func equalRequirement(ae *Exprs, a Requirement, be *Exprs, b Requirement) bool {
	return a.Path.Equal(b.Path) && a.Guard.Equal(b.Guard) && exprSpelling(ae, a.Value) == exprSpelling(be, b.Value)
}

func sortHeapDeltas(e *Exprs, in []HeapDelta) {
	sort.Slice(in, func(i, j int) bool {
		if !in[i].Path.Equal(in[j].Path) {
			return in[i].Path.Less(in[j].Path)
		}
		if !in[i].Guard.Equal(in[j].Guard) {
			return in[i].Guard.Less(in[j].Guard)
		}
		return exprSpelling(e, in[i].Value) < exprSpelling(e, in[j].Value)
	})
}
func dedupHeapDeltas(e *Exprs, in []HeapDelta) []HeapDelta {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for _, x := range in {
		if len(out) == 0 || !equalDelta(e, out[len(out)-1], e, x) {
			out = append(out, x)
		}
	}
	return out
}
func equalDelta(ae *Exprs, a HeapDelta, be *Exprs, b HeapDelta) bool {
	return a.Path.Equal(b.Path) && a.Guard.Equal(b.Guard) && exprSpelling(ae, a.Value) == exprSpelling(be, b.Value)
}

func sortCallNodes(e *Exprs, in []CallNode) {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Callee.BodyDigest != in[j].Callee.BodyDigest {
			return in[i].Callee.BodyDigest < in[j].Callee.BodyDigest
		}
		if !in[i].Guard.Equal(in[j].Guard) {
			return in[i].Guard.Less(in[j].Guard)
		}
		n := len(in[i].Args)
		if len(in[j].Args) < n {
			n = len(in[j].Args)
		}
		for k := 0; k < n; k++ {
			ai, aj := exprSpelling(e, in[i].Args[k]), exprSpelling(e, in[j].Args[k])
			if ai != aj {
				return ai < aj
			}
		}
		return len(in[i].Args) < len(in[j].Args)
	})
}
func dedupCallNodes(e *Exprs, in []CallNode) []CallNode {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for _, x := range in {
		if len(out) == 0 || !equalCall(e, out[len(out)-1], e, x) {
			out = append(out, x)
		}
	}
	return out
}
func equalCall(ae *Exprs, a CallNode, be *Exprs, b CallNode) bool {
	if a.Callee != b.Callee || !a.Guard.Equal(b.Guard) || len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if exprSpelling(ae, a.Args[i]) != exprSpelling(be, b.Args[i]) {
			return false
		}
	}
	return true
}

func normalizeEvents(in []WidenEvent) []WidenEvent { return mergeEvents(in) }
func mergeEvents(sets ...[]WidenEvent) []WidenEvent {
	counts := map[WidenReason]uint32{}
	for _, set := range sets {
		for _, e := range set {
			counts[e.Reason] += e.Count
		}
	}
	out := make([]WidenEvent, 0, len(counts))
	for r, c := range counts {
		if r != "" && c != 0 {
			out = append(out, WidenEvent{Reason: r, Count: c})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reason < out[j].Reason })
	return out
}
