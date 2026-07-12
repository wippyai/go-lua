// Package summaryadapter is an isolated proof that compact guarded transformer
// rows can specialize into the existing Summary boundary. Production does not
// import this package.
package summaryadapter

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

var (
	ErrInvalidPlan           = errors.New("summaryadapter: invalid plan")
	ErrConditionalObligation = errors.New("summaryadapter: conditional obligation requires concrete fallback")
)

// GuardKind is the deliberately small guard vocabulary proven by this POC.
// Unknown guard forms are rejected when the plan is constructed.
type GuardKind uint8

const (
	GuardTruthy GuardKind = iota + 1
	GuardFalsy
)

// Guard is one parameter predicate. Guards in a row form a conjunction.
type Guard struct {
	Param int
	Kind  GuardKind
}

// Value is either a constant product value or a read of one caller-bound
// parameter. The zero value is invalid rather than silently meaning Top.
type Value struct {
	constant product.Value
	param    int
	kind     valueKind
}

type valueKind uint8

const (
	valueConstant valueKind = iota + 1
	valueParam
)

func Constant(value product.Value) Value { return Value{constant: value, kind: valueConstant} }
func Parameter(index int) Value          { return Value{param: index, kind: valueParam} }

// PathRefinement is a value refinement published on normal return.
type PathRefinement struct {
	Path  pathdom.Path
	Value Value
}

// PathInvalidation is a normal-return descendant invalidation.
type PathInvalidation struct {
	Path                      pathdom.Path
	PreserveStructuralWitness bool
	ClearTarget               bool
}

// EffectDelta is a normal-return effect whose value may depend on caller
// parameters. Site and Kind retain their existing engine meanings.
type EffectDelta struct {
	Target pathdom.Path
	Site   effectdelta.Site
	Kind   effectdelta.Kind
	Before Value
	After  Value
	Change effectdelta.Change
}

// Row is one correlated may-return behavior. Requirements are contravariant:
// because Summary has no conditional entry-obligation carrier, a guarded row
// carrying one must fail the entire specialization atomically.
type Row struct {
	Guards            []Guard
	Returns           []Value
	ParamObligations  []Value
	PathRefinements   []PathRefinement
	PathInvalidations []PathInvalidation
	EffectDeltas      []EffectDelta
}

// Plan is immutable after NewPlan. All slices and paths are owned copies.
type Plan struct {
	reg               *axis.Registry
	params            int
	commonObligations []Value
	rows              []Row
}

// Spec is the constructor vocabulary. CommonParamObligations are the
// guard-independent entry contract. Row-local obligations are retained only so
// unsupported conditional contracts can be detected and rejected atomically.
type Spec struct {
	Params                 int
	CommonParamObligations []Value
	Rows                   []Row
}

// NewPlan validates and takes ownership-independent copies of spec.
func NewPlan(reg *axis.Registry, spec Spec) (Plan, error) {
	if reg == nil || spec.Params < 0 || len(spec.Rows) == 0 {
		return Plan{}, ErrInvalidPlan
	}
	for _, obligation := range spec.CommonParamObligations {
		if !validValue(spec.Params, obligation) {
			return Plan{}, fmt.Errorf("%w: invalid common obligation", ErrInvalidPlan)
		}
	}
	cloned := make([]Row, len(spec.Rows))
	for i, row := range spec.Rows {
		if err := validateRow(spec.Params, row); err != nil {
			return Plan{}, fmt.Errorf("%w: row %d: %v", ErrInvalidPlan, i, err)
		}
		cloned[i] = cloneRow(row)
	}
	return Plan{
		reg:               reg,
		params:            spec.Params,
		commonObligations: append([]Value(nil), spec.CommonParamObligations...),
		rows:              cloned,
	}, nil
}

// Specialize binds caller parameter values, retains every feasible row, and
// combines their existing boundary payloads through Summary.Join. It returns
// false only when no row is feasible. Any unsupported conditional obligation
// returns an error and an empty Summary; no partial row is ever published.
func (p Plan) Specialize(params []product.Value) (summary.Summary, bool, error) {
	if p.reg == nil || len(params) != p.params {
		return summary.Summary{}, false, ErrInvalidPlan
	}
	// Validate the whole transformer before evaluating feasibility. Otherwise a
	// currently-infeasible conditional requirement could become a latent
	// unsoundness when a later caller makes its row feasible.
	for _, row := range p.rows {
		if len(row.Guards) != 0 && len(row.ParamObligations) != 0 {
			return summary.Summary{}, false, ErrConditionalObligation
		}
	}

	var out summary.Summary
	seen := false
	for _, row := range p.rows {
		if !guardsMayHold(p.reg, row.Guards, params) {
			continue
		}
		got, err := p.specializeRow(row, params)
		if err != nil {
			return summary.Summary{}, false, err
		}
		if !seen {
			out = got
			seen = true
			continue
		}
		out = summary.Join(p.reg, out, got)
	}
	if !seen {
		return summary.Summary{}, false, nil
	}
	common, err := evalValues(p.commonObligations, params)
	if err != nil {
		return summary.Summary{}, false, err
	}
	if len(common) != 0 {
		width := max(len(common), len(out.ParamObligations))
		merged := make([]product.Value, width)
		for i := range width {
			left, right := product.Top(), product.Top()
			if i < len(common) {
				left = common[i]
			}
			if i < len(out.ParamObligations) {
				right = out.ParamObligations[i]
			}
			merged[i] = product.Meet(p.reg, left, right)
		}
		out.ParamObligations = merged
	}
	return summary.NormalizeOwned(p.reg, out), true, nil
}

func (p Plan) specializeRow(row Row, params []product.Value) (summary.Summary, error) {
	out := summary.Summary{}
	var err error
	out.Returns, err = evalValues(row.Returns, params)
	if err != nil {
		return summary.Summary{}, err
	}
	out.ParamObligations, err = evalValues(row.ParamObligations, params)
	if err != nil {
		return summary.Summary{}, err
	}
	for _, refinement := range row.PathRefinements {
		value, evalErr := evalValue(refinement.Value, params)
		if evalErr != nil {
			return summary.Summary{}, evalErr
		}
		out.NormalReturnFacts.PathRefinements = append(out.NormalReturnFacts.PathRefinements,
			callboundary.PathValueFact{Path: refinement.Path.Clone(), Value: value})
	}
	for _, invalidation := range row.PathInvalidations {
		out.NormalReturnFacts.PathInvalidations = append(out.NormalReturnFacts.PathInvalidations,
			callboundary.PathInvalidationFact{
				Path:                      invalidation.Path.Clone(),
				PreserveStructuralWitness: invalidation.PreserveStructuralWitness,
				ClearTarget:               invalidation.ClearTarget,
			})
	}
	for _, delta := range row.EffectDeltas {
		before, evalErr := evalValue(delta.Before, params)
		if evalErr != nil {
			return summary.Summary{}, evalErr
		}
		after, evalErr := evalValue(delta.After, params)
		if evalErr != nil {
			return summary.Summary{}, evalErr
		}
		out.NormalReturnFacts.EffectDeltas = append(out.NormalReturnFacts.EffectDeltas,
			callboundary.EffectDelta{
				Target: delta.Target.Clone(), Site: delta.Site, Kind: delta.Kind,
				Value: effectdelta.Value{Before: before, After: after, Change: delta.Change},
			})
	}
	return summary.NormalizeOwned(p.reg, out), nil
}

func guardsMayHold(reg *axis.Registry, guards []Guard, params []product.Value) bool {
	for _, guard := range guards {
		switch guard.Kind {
		case GuardTruthy:
			if !valueref.CanBeTruthy(reg, params[guard.Param]) {
				return false
			}
		case GuardFalsy:
			if !valueref.CanBeFalsy(reg, params[guard.Param]) {
				return false
			}
		}
	}
	return true
}

func evalValues(values []Value, params []product.Value) ([]product.Value, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]product.Value, len(values))
	for i, value := range values {
		got, err := evalValue(value, params)
		if err != nil {
			return nil, err
		}
		out[i] = got
	}
	return out, nil
}

func evalValue(value Value, params []product.Value) (product.Value, error) {
	switch value.kind {
	case valueConstant:
		return value.constant, nil
	case valueParam:
		if value.param < 0 || value.param >= len(params) {
			return product.Value{}, ErrInvalidPlan
		}
		return params[value.param], nil
	default:
		return product.Value{}, ErrInvalidPlan
	}
}

func validateRow(params int, row Row) error {
	for _, guard := range row.Guards {
		if guard.Param < 0 || guard.Param >= params || guard.Kind != GuardTruthy && guard.Kind != GuardFalsy {
			return errors.New("invalid guard")
		}
	}
	for _, value := range row.Returns {
		if !validValue(params, value) {
			return errors.New("invalid return")
		}
	}
	for _, value := range row.ParamObligations {
		if !validValue(params, value) {
			return errors.New("invalid obligation")
		}
	}
	for _, refinement := range row.PathRefinements {
		if !refinement.Path.IsPlaceholder() || !validValue(params, refinement.Value) {
			return errors.New("invalid path refinement")
		}
	}
	for _, invalidation := range row.PathInvalidations {
		if invalidation.Path.IsEmpty() {
			return errors.New("invalid path invalidation")
		}
	}
	for _, delta := range row.EffectDeltas {
		if !delta.Target.IsPlaceholder() || delta.Site == "" || delta.Kind == 0 || delta.Change == effectdelta.ChangeBottom ||
			!validValue(params, delta.Before) || !validValue(params, delta.After) {
			return errors.New("invalid effect delta")
		}
	}
	return nil
}

func validValue(params int, value Value) bool {
	return value.kind == valueConstant || value.kind == valueParam && value.param >= 0 && value.param < params
}

func cloneRow(row Row) Row {
	out := row
	out.Guards = append([]Guard(nil), row.Guards...)
	out.Returns = append([]Value(nil), row.Returns...)
	out.ParamObligations = append([]Value(nil), row.ParamObligations...)
	out.PathRefinements = append([]PathRefinement(nil), row.PathRefinements...)
	for i := range out.PathRefinements {
		out.PathRefinements[i].Path = out.PathRefinements[i].Path.Clone()
	}
	out.PathInvalidations = append([]PathInvalidation(nil), row.PathInvalidations...)
	for i := range out.PathInvalidations {
		out.PathInvalidations[i].Path = out.PathInvalidations[i].Path.Clone()
	}
	out.EffectDeltas = append([]EffectDelta(nil), row.EffectDeltas...)
	for i := range out.EffectDeltas {
		out.EffectDeltas[i].Target = out.EffectDeltas[i].Target.Clone()
	}
	return out
}
