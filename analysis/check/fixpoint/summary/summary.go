// Package summary defines fixed-point function summaries for analysis checks.
package summary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Digest is an explicit caller-provided digest for future entry dimensions.
type Digest uint64

// EntryKey identifies the abstract call-entry dimensions for a summary key.
type EntryKey struct {
	Values     Digest
	Facts      Digest
	References Digest
}

// SummaryKey identifies one exact summary entry.
type SummaryKey struct {
	Ref   ref.FuncRef
	Entry EntryKey
}

// DefaultSummaryKey returns the default summary key for r.
func DefaultSummaryKey(r ref.FuncRef) SummaryKey {
	return SummaryKey{Ref: r}
}

// Less reports whether k sorts before other.
func (k SummaryKey) Less(other SummaryKey) bool {
	if k.Ref != other.Ref {
		return k.Ref.Less(other.Ref)
	}
	if k.Entry.Values != other.Entry.Values {
		return k.Entry.Values < other.Entry.Values
	}
	if k.Entry.Facts != other.Entry.Facts {
		return k.Entry.Facts < other.Entry.Facts
	}
	return k.Entry.References < other.Entry.References
}

// Summary is the fixed-point analysis summary payload for one function entry.
type Summary struct {
	Returns                         []product.Value
	NormalReturnParams              []product.Value
	NormalReturnParamConditions     []ParamCondition
	NormalReturnParamEqualities     []ParamEquality
	NormalReturnFacts               NormalReturnFacts
	ReturnConditionParamRefinements []ReturnConditionParamRefinement
	ReturnPresenceRelations         []ReturnPresenceRelation
}

// ParamCondition is a summary-local truthiness condition for one parameter on
// normal return.
type ParamCondition uint8

const (
	ParamConditionBottom ParamCondition = iota
	ParamConditionTruthy
	ParamConditionFalsy
	ParamConditionTop
)

// IsUseful reports whether c carries a caller-applicable condition.
func (c ParamCondition) IsUseful() bool {
	return c == ParamConditionTruthy || c == ParamConditionFalsy
}

// ParamEquality records a normal-return equality relation between two
// parameter roots.
type ParamEquality struct {
	Left  int
	Right int
}

// Normalize returns s with trailing bottom slots removed.
func Normalize(reg *axis.Registry, s Summary) Summary {
	out := s.Clone()
	bottom := product.Bottom(reg)
	for len(out.Returns) > 0 && product.Equal(reg, out.Returns[len(out.Returns)-1], bottom) {
		out.Returns = out.Returns[:len(out.Returns)-1]
	}
	for len(out.NormalReturnParams) > 0 &&
		product.Equal(reg, out.NormalReturnParams[len(out.NormalReturnParams)-1], bottom) {
		out.NormalReturnParams = out.NormalReturnParams[:len(out.NormalReturnParams)-1]
	}
	for len(out.NormalReturnParamConditions) > 0 &&
		!out.NormalReturnParamConditions[len(out.NormalReturnParamConditions)-1].IsUseful() {
		out.NormalReturnParamConditions = out.NormalReturnParamConditions[:len(out.NormalReturnParamConditions)-1]
	}
	out.NormalReturnParamEqualities = normalizeParamEqualities(out.NormalReturnParamEqualities)
	out.NormalReturnFacts = normalizeNormalReturnFacts(reg, out.NormalReturnFacts)
	out.ReturnConditionParamRefinements = normalizeReturnConditionParamRefinements(
		reg,
		out.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = normalizeReturnPresenceRelations(out.ReturnPresenceRelations)
	if len(out.Returns) == 0 &&
		len(out.NormalReturnParams) == 0 &&
		len(out.NormalReturnParamConditions) == 0 &&
		len(out.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(out.NormalReturnFacts) &&
		len(out.ReturnConditionParamRefinements) == 0 &&
		len(out.ReturnPresenceRelations) == 0 {
		return Summary{}
	}
	return out
}

// Equal reports whether a and b have equal summary lanes. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Equal(reg *axis.Registry, a, b Summary) bool {
	n := max(len(a.Returns), len(b.Returns))
	for i := range n {
		if !product.Equal(reg, returnAt(reg, a, i), returnAt(reg, b, i)) {
			return false
		}
	}
	n = max(len(a.NormalReturnParams), len(b.NormalReturnParams))
	for i := range n {
		if !product.Equal(reg, normalReturnParamAt(reg, a, i), normalReturnParamAt(reg, b, i)) {
			return false
		}
	}
	n = max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	for i := range n {
		if normalReturnParamConditionAt(reg, a, i) != normalReturnParamConditionAt(reg, b, i) {
			return false
		}
	}
	return paramEqualitiesSummaryEqual(reg, a, b) &&
		normalReturnFactsEqual(reg, a.NormalReturnFacts, b.NormalReturnFacts) &&
		returnConditionParamRefinementsEqual(reg, a.ReturnConditionParamRefinements, b.ReturnConditionParamRefinements) &&
		returnPresenceRelationsEqual(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
}

// LessOrEq reports whether a is less than or equal to b componentwise. Missing
// return and value-constraint slots are bottom. Missing condition slots within
// the known normal-return parameter arity are top/no-constraint.
func LessOrEq(reg *axis.Registry, a, b Summary) bool {
	if summaryBottom(a) {
		return true
	}
	if summaryBottom(b) {
		return summaryBottom(a)
	}
	n := max(len(a.Returns), len(b.Returns))
	for i := range n {
		if !product.LessOrEq(reg, returnAt(reg, a, i), returnAt(reg, b, i)) {
			return false
		}
	}
	n = max(len(a.NormalReturnParams), len(b.NormalReturnParams))
	for i := range n {
		if !product.LessOrEq(reg, normalReturnParamAt(reg, a, i), normalReturnParamAt(reg, b, i)) {
			return false
		}
	}
	n = max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	for i := range n {
		if !paramConditionLessOrEq(normalReturnParamConditionAt(reg, a, i), normalReturnParamConditionAt(reg, b, i)) {
			return false
		}
	}
	return paramEqualitiesSummaryLessOrEq(reg, a, b) &&
		normalReturnFactsLessOrEq(reg, a.NormalReturnFacts, b.NormalReturnFacts) &&
		returnConditionParamRefinementsLessOrEq(reg, a.ReturnConditionParamRefinements, b.ReturnConditionParamRefinements) &&
		returnPresenceRelationsLessOrEq(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
}

// Join returns the componentwise join of a and b. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Join(reg *axis.Registry, a, b Summary) Summary {
	returns := max(len(a.Returns), len(b.Returns))
	params := max(len(a.NormalReturnParams), len(b.NormalReturnParams))
	conditions := max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	if summaryBottom(a) {
		return Normalize(reg, b)
	}
	if summaryBottom(b) {
		return Normalize(reg, a)
	}
	if returns == 0 && params == 0 && conditions == 0 &&
		len(a.NormalReturnParamEqualities) == 0 && len(b.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(a.NormalReturnFacts) && normalReturnFactsEmpty(b.NormalReturnFacts) &&
		len(a.ReturnConditionParamRefinements) == 0 && len(b.ReturnConditionParamRefinements) == 0 &&
		len(a.ReturnPresenceRelations) == 0 && len(b.ReturnPresenceRelations) == 0 {
		return Summary{}
	}
	out := Summary{}
	if returns > 0 {
		out.Returns = make([]product.Value, returns)
	}
	for i := range returns {
		out.Returns[i] = product.Join(reg, returnAt(reg, a, i), returnAt(reg, b, i))
	}
	if params > 0 {
		out.NormalReturnParams = make([]product.Value, params)
	}
	for i := range params {
		out.NormalReturnParams[i] = product.Join(reg, normalReturnParamAt(reg, a, i), normalReturnParamAt(reg, b, i))
	}
	if conditions > 0 {
		out.NormalReturnParamConditions = make([]ParamCondition, conditions)
	}
	for i := range conditions {
		out.NormalReturnParamConditions[i] = joinParamCondition(
			normalReturnParamConditionAt(reg, a, i),
			normalReturnParamConditionAt(reg, b, i),
		)
	}
	out.NormalReturnParamEqualities = joinParamEqualities(reg, a, b)
	out.NormalReturnFacts = joinNormalReturnFacts(reg, a.NormalReturnFacts, b.NormalReturnFacts)
	out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
		reg,
		a.ReturnConditionParamRefinements,
		b.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = joinReturnPresenceRelations(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
	return Normalize(reg, out)
}

// Widen returns the componentwise widening from prev to next. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Widen(reg *axis.Registry, prev, next Summary) Summary {
	returns := max(len(prev.Returns), len(next.Returns))
	params := max(len(prev.NormalReturnParams), len(next.NormalReturnParams))
	conditions := max(normalReturnParamCount(reg, prev), normalReturnParamCount(reg, next))
	if summaryBottom(prev) {
		return Normalize(reg, next)
	}
	if summaryBottom(next) {
		return Normalize(reg, prev)
	}
	if returns == 0 && params == 0 && conditions == 0 &&
		len(prev.NormalReturnParamEqualities) == 0 && len(next.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(prev.NormalReturnFacts) && normalReturnFactsEmpty(next.NormalReturnFacts) &&
		len(prev.ReturnConditionParamRefinements) == 0 && len(next.ReturnConditionParamRefinements) == 0 &&
		len(prev.ReturnPresenceRelations) == 0 && len(next.ReturnPresenceRelations) == 0 {
		return Summary{}
	}
	out := Summary{}
	if returns > 0 {
		out.Returns = make([]product.Value, returns)
	}
	for i := range returns {
		out.Returns[i] = product.Widen(reg, returnAt(reg, prev, i), returnAt(reg, next, i))
	}
	if params > 0 {
		out.NormalReturnParams = make([]product.Value, params)
	}
	for i := range params {
		out.NormalReturnParams[i] = product.Widen(
			reg,
			normalReturnParamAt(reg, prev, i),
			normalReturnParamAt(reg, next, i),
		)
	}
	if conditions > 0 {
		out.NormalReturnParamConditions = make([]ParamCondition, conditions)
	}
	for i := range conditions {
		out.NormalReturnParamConditions[i] = widenParamCondition(
			normalReturnParamConditionAt(reg, prev, i),
			normalReturnParamConditionAt(reg, next, i),
		)
	}
	out.NormalReturnParamEqualities = joinParamEqualities(reg, prev, next)
	out.NormalReturnFacts = widenNormalReturnFacts(reg, prev.NormalReturnFacts, next.NormalReturnFacts)
	out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
		reg,
		prev.ReturnConditionParamRefinements,
		next.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = joinReturnPresenceRelations(prev.ReturnPresenceRelations, next.ReturnPresenceRelations)
	return Normalize(reg, out)
}

// Clone returns an independent copy of s.
func (s Summary) Clone() Summary {
	if len(s.Returns) == 0 &&
		len(s.NormalReturnParams) == 0 &&
		len(s.NormalReturnParamConditions) == 0 &&
		len(s.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(s.NormalReturnFacts) &&
		len(s.ReturnConditionParamRefinements) == 0 &&
		len(s.ReturnPresenceRelations) == 0 {
		return Summary{}
	}
	out := Summary{}
	if len(s.Returns) > 0 {
		out.Returns = make([]product.Value, len(s.Returns))
		copy(out.Returns, s.Returns)
	}
	if len(s.NormalReturnParams) > 0 {
		out.NormalReturnParams = make([]product.Value, len(s.NormalReturnParams))
		copy(out.NormalReturnParams, s.NormalReturnParams)
	}
	if len(s.NormalReturnParamConditions) > 0 {
		out.NormalReturnParamConditions = make([]ParamCondition, len(s.NormalReturnParamConditions))
		copy(out.NormalReturnParamConditions, s.NormalReturnParamConditions)
	}
	if len(s.NormalReturnParamEqualities) > 0 {
		out.NormalReturnParamEqualities = make([]ParamEquality, len(s.NormalReturnParamEqualities))
		copy(out.NormalReturnParamEqualities, s.NormalReturnParamEqualities)
	}
	out.NormalReturnFacts = cloneNormalReturnFacts(s.NormalReturnFacts)
	out.ReturnConditionParamRefinements = cloneReturnConditionParamRefinements(s.ReturnConditionParamRefinements)
	out.ReturnPresenceRelations = cloneReturnPresenceRelations(s.ReturnPresenceRelations)
	return out
}

func returnAt(reg *axis.Registry, s Summary, i int) product.Value {
	if i < len(s.Returns) {
		return s.Returns[i]
	}
	return product.Bottom(reg)
}

func normalReturnParamAt(reg *axis.Registry, s Summary, i int) product.Value {
	if i < len(s.NormalReturnParams) {
		return s.NormalReturnParams[i]
	}
	return product.Bottom(reg)
}

func normalReturnParamConditionAt(reg *axis.Registry, s Summary, i int) ParamCondition {
	if i < len(s.NormalReturnParamConditions) {
		return s.NormalReturnParamConditions[i]
	}
	if i < normalReturnParamCount(reg, s) {
		return ParamConditionTop
	}
	return ParamConditionBottom
}

func normalReturnParamCount(reg *axis.Registry, s Summary) int {
	paramCount := len(s.NormalReturnParams)
	bottom := product.Bottom(reg)
	for paramCount > 0 && product.Equal(reg, s.NormalReturnParams[paramCount-1], bottom) {
		paramCount--
	}
	conditionCount := len(s.NormalReturnParamConditions)
	for conditionCount > 0 && !s.NormalReturnParamConditions[conditionCount-1].IsUseful() {
		conditionCount--
	}
	return max(paramCount, conditionCount)
}

func paramConditionLessOrEq(a, b ParamCondition) bool {
	return a == b || a == ParamConditionBottom || b == ParamConditionTop
}

func joinParamCondition(a, b ParamCondition) ParamCondition {
	if a == b {
		return a
	}
	if a == ParamConditionBottom {
		return b
	}
	if b == ParamConditionBottom {
		return a
	}
	return ParamConditionTop
}

func widenParamCondition(prev, next ParamCondition) ParamCondition {
	if prev == next {
		return next
	}
	if prev == ParamConditionBottom {
		return next
	}
	return ParamConditionTop
}

func normalizeParamEqualities(in []ParamEquality) []ParamEquality {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[ParamEquality]struct{}, len(in))
	out := make([]ParamEquality, 0, len(in))
	for _, eq := range in {
		if eq.Left == eq.Right || eq.Left < 0 || eq.Right < 0 {
			continue
		}
		if eq.Right < eq.Left {
			eq.Left, eq.Right = eq.Right, eq.Left
		}
		if _, ok := seen[eq]; ok {
			continue
		}
		seen[eq] = struct{}{}
		out = append(out, eq)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Left != out[j].Left {
			return out[i].Left < out[j].Left
		}
		return out[i].Right < out[j].Right
	})
	return out
}

func paramEqualitiesEqual(a, b []ParamEquality) bool {
	a = normalizeParamEqualities(a)
	b = normalizeParamEqualities(b)
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

func paramEqualitiesLessOrEq(a, b []ParamEquality) bool {
	a = normalizeParamEqualities(a)
	b = normalizeParamEqualities(b)
	if len(b) == 0 {
		return true
	}
	seen := make(map[ParamEquality]struct{}, len(a))
	for _, eq := range a {
		seen[eq] = struct{}{}
	}
	for _, eq := range b {
		if _, ok := seen[eq]; !ok {
			return false
		}
	}
	return true
}

func paramEqualitiesSummaryEqual(reg *axis.Registry, a, b Summary) bool {
	if paramEqualitiesBottom(reg, a) || paramEqualitiesBottom(reg, b) {
		return paramEqualitiesBottom(reg, a) && paramEqualitiesBottom(reg, b)
	}
	return paramEqualitiesEqual(a.NormalReturnParamEqualities, b.NormalReturnParamEqualities)
}

func paramEqualitiesSummaryLessOrEq(reg *axis.Registry, a, b Summary) bool {
	if paramEqualitiesBottom(reg, a) {
		return true
	}
	if paramEqualitiesBottom(reg, b) {
		return false
	}
	return paramEqualitiesLessOrEq(a.NormalReturnParamEqualities, b.NormalReturnParamEqualities)
}

func joinParamEqualities(reg *axis.Registry, a, b Summary) []ParamEquality {
	switch {
	case paramEqualitiesBottom(reg, a):
		return normalizeParamEqualities(b.NormalReturnParamEqualities)
	case paramEqualitiesBottom(reg, b):
		return normalizeParamEqualities(a.NormalReturnParamEqualities)
	}
	aEqualities := normalizeParamEqualities(a.NormalReturnParamEqualities)
	bEqualities := normalizeParamEqualities(b.NormalReturnParamEqualities)
	if len(aEqualities) == 0 || len(bEqualities) == 0 {
		return nil
	}
	seen := make(map[ParamEquality]struct{}, len(bEqualities))
	for _, eq := range bEqualities {
		seen[eq] = struct{}{}
	}
	out := make([]ParamEquality, 0, min(len(aEqualities), len(bEqualities)))
	for _, eq := range aEqualities {
		if _, ok := seen[eq]; ok {
			out = append(out, eq)
		}
	}
	return normalizeParamEqualities(out)
}

func paramEqualitiesBottom(reg *axis.Registry, s Summary) bool {
	return normalReturnParamCount(reg, s) == 0 && len(s.NormalReturnParamEqualities) == 0
}

func summaryBottom(s Summary) bool {
	return len(s.Returns) == 0 &&
		len(s.NormalReturnParams) == 0 &&
		len(s.NormalReturnParamConditions) == 0 &&
		len(s.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(s.NormalReturnFacts) &&
		len(s.ReturnConditionParamRefinements) == 0 &&
		len(s.ReturnPresenceRelations) == 0
}

// Reader reads exact summary keys.
type Reader interface {
	Read(SummaryKey) (Summary, bool)
}

// EntrySummary binds a key to a summary for snapshot construction.
type EntrySummary struct {
	Key     SummaryKey
	Summary Summary
}

// Snapshot is an immutable exact-key summary reader.
type Snapshot struct {
	reg     *axis.Registry
	entries map[SummaryKey]Summary
}

// NewSnapshot returns a snapshot containing entries.
func NewSnapshot(reg *axis.Registry, entries ...EntrySummary) Snapshot {
	if len(entries) == 0 {
		return Snapshot{reg: reg}
	}
	out := Snapshot{
		reg:     reg,
		entries: make(map[SummaryKey]Summary, len(entries)),
	}
	for _, entry := range entries {
		out.entries[entry.Key] = Normalize(reg, entry.Summary)
	}
	return out
}

// Read returns the summary for k. It never falls back to other entries for the
// same function reference.
func (s Snapshot) Read(k SummaryKey) (Summary, bool) {
	if len(s.entries) == 0 {
		return Summary{}, false
	}
	got, ok := s.entries[k]
	if !ok {
		return Summary{}, false
	}
	return got.Clone(), true
}
