package manifest

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/type/projection"
)

type effectRowWire struct {
	Labels []effectLabelWire `json:"labels,omitempty"`
	Tail   *string           `json:"tail,omitempty"`
}

type effectLabelWire struct {
	Kind string `json:"kind"`

	ReturnIndex int   `json:"returnIndex,omitempty"`
	ValueIndex  int   `json:"valueIndex,omitempty"`
	ErrorIndex  int   `json:"errorIndex,omitempty"`
	Indices     []int `json:"indices,omitempty"`
	FromParam   int   `json:"fromParam,omitempty"`
	Delta       int   `json:"delta,omitempty"`

	Target *paramRefWire `json:"target,omitempty"`
	Source *paramRefWire `json:"source,omitempty"`
	Param  *paramRefWire `json:"param,omitempty"`
	Into   *paramRefWire `json:"into,omitempty"`
	Value  *paramRefWire `json:"value,omitempty"`

	IteratorKind string                `json:"iteratorKind,omitempty"`
	Transform    *effectTransformWire  `json:"transform,omitempty"`
	ReturnType   *effectReturnWire     `json:"returnType,omitempty"`
	Length       *exprWire             `json:"length,omitempty"`
	Refinement   *effectRefinementWire `json:"refinement,omitempty"`
}

type effectTransformWire struct {
	Kind      string        `json:"kind"`
	Source    *paramRefWire `json:"source,omitempty"`
	Container *paramRefWire `json:"container,omitempty"`
	Value     *paramRefWire `json:"value,omitempty"`
	Element   *paramRefWire `json:"element,omitempty"`
}

type effectRefinementWire struct {
	Kind string `json:"kind"`
}

type effectReturnWire struct {
	Kind          string               `json:"kind"`
	Source        *paramRefWire        `json:"source,omitempty"`
	Cases         *paramRefWire        `json:"cases,omitempty"`
	Default       *paramRefWire        `json:"default,omitempty"`
	CallbackParam *paramRefWire        `json:"callbackParam,omitempty"`
	Format        *paramRefWire        `json:"format,omitempty"`
	Projection    []projectionStepWire `json:"projection,omitempty"`
}

type paramRefWire struct {
	Index int `json:"index"`
}

type exprWire struct {
	Kind  string    `json:"kind"`
	Name  string    `json:"name,omitempty"`
	Value int64     `json:"value,omitempty"`
	Index int       `json:"index,omitempty"`
	Op    string    `json:"op,omitempty"`
	Left  *exprWire `json:"left,omitempty"`
	Right *exprWire `json:"right,omitempty"`
}

type projectionStepWire struct {
	Kind  string    `json:"kind"`
	Field string    `json:"field,omitempty"`
	Index int       `json:"index,omitempty"`
	Type  *typeWire `json:"type,omitempty"`
}

func encodeEffectRow(row effect.Row) (*effectRowWire, error) {
	if row.Pure() {
		return nil, nil
	}

	out := &effectRowWire{Labels: make([]effectLabelWire, 0, len(row.Labels))}
	for _, label := range row.Labels {
		encoded, err := encodeEffectLabel(label)
		if err != nil {
			return nil, err
		}
		out.Labels = append(out.Labels, encoded)
	}
	sort.Slice(out.Labels, func(i, j int) bool {
		return effectLabelWireKey(out.Labels[i]) < effectLabelWireKey(out.Labels[j])
	})
	if row.Tail != nil {
		tail := row.Tail.Name
		out.Tail = &tail
	}
	return out, nil
}

func decodeEffectRow(w *effectRowWire) (effect.Row, error) {
	if w == nil {
		return effect.Empty, nil
	}

	labels := make([]effect.Label, 0, len(w.Labels))
	for _, label := range w.Labels {
		decoded, err := decodeEffectLabel(label)
		if err != nil {
			return effect.Row{}, err
		}
		labels = append(labels, decoded)
	}
	var tail *effect.Var
	if w.Tail != nil {
		tail = &effect.Var{Name: *w.Tail}
	}
	return effect.Row{Labels: labels, Tail: tail}, nil
}

func encodeEffectLabel(label effect.Label) (effectLabelWire, error) {
	label = effect.NormalizeLabel(label)
	switch l := label.(type) {
	case control.Throw:
		return effectLabelWire{Kind: "control.throw"}, nil
	case control.Diverge:
		return effectLabelWire{Kind: "control.diverge"}, nil
	case control.IO:
		return effectLabelWire{Kind: "control.io"}, nil
	case dispatch.ModuleLoad:
		return effectLabelWire{Kind: "dispatch.moduleLoad"}, nil
	case dispatch.VariadicTransform:
		return effectLabelWire{Kind: "dispatch.variadicTransform"}, nil
	case dispatch.TypePredicate:
		return effectLabelWire{Kind: "dispatch.typePredicate"}, nil
	case dispatch.TypeValueMethod:
		return effectLabelWire{Kind: "dispatch.typeValueMethod"}, nil
	case dispatch.CallableType:
		return effectLabelWire{Kind: "dispatch.callableType"}, nil
	case iteration.Iterator:
		kind, err := encodeIteratorKind(l.Kind)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:         "iteration.iterator",
			Source:       encodeParamRef(l.Source),
			IteratorKind: kind,
		}, nil
	case mutation.Mutate:
		transform, err := encodeEffectTransform(l.Transform)
		if err != nil {
			return effectLabelWire{}, err
		}
		length, err := encodeExpr(l.LengthDelta)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:      "mutation.mutate",
			Target:    encodeParamRef(l.Target),
			Transform: transform,
			Length:    length,
		}, nil
	case mutation.LengthChange:
		return effectLabelWire{
			Kind:   "mutation.lengthChange",
			Target: encodeParamRef(l.Target),
			Delta:  l.Delta,
		}, nil
	case mutation.TableMutator:
		return effectLabelWire{
			Kind:   "mutation.tableMutator",
			Target: encodeParamRef(l.Target),
			Value:  encodeParamRef(l.Value),
		}, nil
	case ownership.Borrow:
		return effectLabelWire{Kind: "ownership.borrow", Param: encodeParamRef(l.Param)}, nil
	case ownership.Store:
		return effectLabelWire{Kind: "ownership.store", Param: encodeParamRef(l.Param), Into: encodeParamRef(l.Into)}, nil
	case ownership.BorrowAll:
		return effectLabelWire{Kind: "ownership.borrowAll"}, nil
	case ownership.Send:
		return effectLabelWire{Kind: "ownership.send", FromParam: l.FromParam}, nil
	case ownership.Freeze:
		return effectLabelWire{Kind: "ownership.freeze", Param: encodeParamRef(l.Param)}, nil
	case postcondition.NormalReturnRefinement:
		refinement, err := encodeEffectRefinement(l.Refinement)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:       postcondition.NormalReturnRefinementKind,
			Target:     encodeParamRef(l.Target),
			Refinement: refinement,
		}, nil
	case returns.Return:
		transform, err := encodeEffectReturn(l.Transform)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{Kind: "returns.return", ReturnIndex: l.ReturnIndex, ReturnType: transform}, nil
	case returns.ErrorReturn:
		return effectLabelWire{Kind: "returns.errorReturn", ValueIndex: l.ValueIndex, ErrorIndex: l.ErrorIndex}, nil
	case returns.ReturnLength:
		length, err := encodeExpr(l.Length)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{Kind: "returns.returnLength", ReturnIndex: l.ReturnIndex, Length: length}, nil
	case returns.CorrelatedReturn:
		indices := append([]int(nil), l.Indices...)
		return effectLabelWire{Kind: "returns.correlatedReturn", Indices: indices}, nil
	default:
		return effectLabelWire{}, fmt.Errorf("manifest: unsupported effect label %T", label)
	}
}

func decodeEffectLabel(w effectLabelWire) (effect.Label, error) {
	switch w.Kind {
	case "control.throw":
		return control.Throw{}, nil
	case "control.diverge":
		return control.Diverge{}, nil
	case "control.io":
		return control.IO{}, nil
	case "dispatch.moduleLoad":
		return dispatch.ModuleLoad{}, nil
	case "dispatch.variadicTransform":
		return dispatch.VariadicTransform{}, nil
	case "dispatch.typePredicate":
		return dispatch.TypePredicate{}, nil
	case "dispatch.typeValueMethod":
		return dispatch.TypeValueMethod{}, nil
	case "dispatch.callableType":
		return dispatch.CallableType{}, nil
	case "iteration.iterator":
		kind, err := decodeIteratorKind(w.IteratorKind)
		if err != nil {
			return nil, err
		}
		return iteration.Iterator{Source: decodeParamRef(w.Source), Kind: kind}, nil
	case "mutation.mutate":
		transform, err := decodeEffectTransform(w.Transform)
		if err != nil {
			return nil, err
		}
		length, err := decodeExpr(w.Length)
		if err != nil {
			return nil, err
		}
		return mutation.Mutate{Target: decodeParamRef(w.Target), Transform: transform, LengthDelta: length}, nil
	case "mutation.lengthChange":
		return mutation.LengthChange{Target: decodeParamRef(w.Target), Delta: w.Delta}, nil
	case "mutation.tableMutator":
		return mutation.TableMutator{Target: decodeParamRef(w.Target), Value: decodeParamRef(w.Value)}, nil
	case "ownership.borrow":
		return ownership.Borrow{Param: decodeParamRef(w.Param)}, nil
	case "ownership.store":
		return ownership.Store{Param: decodeParamRef(w.Param), Into: decodeParamRef(w.Into)}, nil
	case "ownership.borrowAll":
		return ownership.BorrowAll{}, nil
	case "ownership.send":
		return ownership.Send{FromParam: w.FromParam}, nil
	case "ownership.freeze":
		return ownership.Freeze{Param: decodeParamRef(w.Param)}, nil
	case postcondition.NormalReturnRefinementKind:
		refinement, err := decodeEffectRefinement(w.Refinement)
		if err != nil {
			return nil, err
		}
		return postcondition.NormalReturnRefinement{Target: decodeParamRef(w.Target), Refinement: refinement}, nil
	case "returns.return":
		transform, err := decodeEffectReturn(w.ReturnType)
		if err != nil {
			return nil, err
		}
		return returns.Return{ReturnIndex: w.ReturnIndex, Transform: transform}, nil
	case "returns.errorReturn":
		return returns.ErrorReturn{ValueIndex: w.ValueIndex, ErrorIndex: w.ErrorIndex}, nil
	case "returns.returnLength":
		length, err := decodeExpr(w.Length)
		if err != nil {
			return nil, err
		}
		return returns.ReturnLength{ReturnIndex: w.ReturnIndex, Length: length}, nil
	case "returns.correlatedReturn":
		indices := append([]int(nil), w.Indices...)
		return returns.CorrelatedReturn{Indices: indices}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown effect label kind %q", w.Kind)
	}
}

func encodeEffectRefinement(refinement postcondition.Refinement) (*effectRefinementWire, error) {
	if refinement == nil {
		return nil, fmt.Errorf("manifest: missing effect refinement")
	}
	switch r := refinement.(type) {
	case postcondition.Present:
		return &effectRefinementWire{Kind: r.Kind()}, nil
	case *postcondition.Present:
		if r == nil {
			return nil, fmt.Errorf("manifest: missing effect refinement")
		}
		return encodeEffectRefinement(*r)
	default:
		return nil, fmt.Errorf("manifest: unsupported effect refinement %T", refinement)
	}
}

func decodeEffectRefinement(w *effectRefinementWire) (postcondition.Refinement, error) {
	if w == nil {
		return nil, fmt.Errorf("manifest: missing effect refinement")
	}
	switch w.Kind {
	case postcondition.PresentKind:
		return postcondition.Present{}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown effect refinement kind %q", w.Kind)
	}
}

func encodeEffectTransform(transform mutation.TypeTransform) (*effectTransformWire, error) {
	if transform == nil {
		return nil, nil
	}
	switch t := transform.(type) {
	case mutation.Unchanged:
		return &effectTransformWire{Kind: "mutation.unchanged"}, nil
	case *mutation.Unchanged:
		return encodeEffectTransform(*t)
	case mutation.ElementUnion:
		return &effectTransformWire{Kind: "mutation.elementUnion", Source: encodeParamRef(t.Source)}, nil
	case *mutation.ElementUnion:
		return encodeEffectTransform(*t)
	case mutation.ContainerElementUnion:
		return &effectTransformWire{
			Kind:      "mutation.containerElementUnion",
			Container: encodeParamRef(t.Container),
			Value:     encodeParamRef(t.Value),
		}, nil
	case *mutation.ContainerElementUnion:
		return encodeEffectTransform(*t)
	case mutation.ToArray:
		return &effectTransformWire{Kind: "mutation.toArray", Element: encodeParamRef(t.Element)}, nil
	case *mutation.ToArray:
		return encodeEffectTransform(*t)
	default:
		return nil, fmt.Errorf("manifest: unsupported effect transform %T", transform)
	}
}

func decodeEffectTransform(w *effectTransformWire) (mutation.TypeTransform, error) {
	if w == nil {
		return nil, nil
	}
	switch w.Kind {
	case "mutation.unchanged":
		return mutation.Unchanged{}, nil
	case "mutation.elementUnion":
		return mutation.ElementUnion{Source: decodeParamRef(w.Source)}, nil
	case "mutation.containerElementUnion":
		return mutation.ContainerElementUnion{Container: decodeParamRef(w.Container), Value: decodeParamRef(w.Value)}, nil
	case "mutation.toArray":
		return mutation.ToArray{Element: decodeParamRef(w.Element)}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown effect transform kind %q", w.Kind)
	}
}

func encodeEffectReturn(ret returns.ReturnType) (*effectReturnWire, error) {
	if ret == nil {
		return nil, nil
	}
	switch r := ret.(type) {
	case returns.SelectCaseOfParam:
		return &effectReturnWire{Kind: "returns.selectCaseOfParam", Source: encodeParamRef(r.Source)}, nil
	case *returns.SelectCaseOfParam:
		return encodeEffectReturn(*r)
	case returns.SelectResultOfCases:
		return &effectReturnWire{
			Kind:    "returns.selectResultOfCases",
			Cases:   encodeParamRef(r.Cases),
			Default: encodeParamRef(r.Default),
		}, nil
	case *returns.SelectResultOfCases:
		return encodeEffectReturn(*r)
	case returns.ElementOf:
		return &effectReturnWire{Kind: "returns.elementOf", Source: encodeParamRef(r.Source)}, nil
	case *returns.ElementOf:
		return encodeEffectReturn(*r)
	case returns.OptionalElementOf:
		return &effectReturnWire{Kind: "returns.optionalElementOf", Source: encodeParamRef(r.Source)}, nil
	case *returns.OptionalElementOf:
		return encodeEffectReturn(*r)
	case returns.CallbackReturn:
		return &effectReturnWire{Kind: "returns.callbackReturn", CallbackParam: encodeParamRef(r.CallbackParam)}, nil
	case *returns.CallbackReturn:
		return encodeEffectReturn(*r)
	case returns.ArrayOfCallbackReturn:
		return &effectReturnWire{Kind: "returns.arrayOfCallbackReturn", CallbackParam: encodeParamRef(r.CallbackParam)}, nil
	case *returns.ArrayOfCallbackReturn:
		return encodeEffectReturn(*r)
	case returns.SameAs:
		return &effectReturnWire{Kind: "returns.sameAs", Source: encodeParamRef(r.Source)}, nil
	case *returns.SameAs:
		return encodeEffectReturn(*r)
	case returns.DeepElementOf:
		return &effectReturnWire{Kind: "returns.deepElementOf", Source: encodeParamRef(r.Source)}, nil
	case *returns.DeepElementOf:
		return encodeEffectReturn(*r)
	case returns.StringUnpackValue:
		return &effectReturnWire{Kind: "returns.stringUnpackValue", Format: encodeParamRef(r.Format)}, nil
	case *returns.StringUnpackValue:
		return encodeEffectReturn(*r)
	case returns.TypeProjection:
		steps, err := encodeProjectionSteps(r.Projection.Steps)
		if err != nil {
			return nil, err
		}
		return &effectReturnWire{Kind: "returns.typeProjection", Source: encodeParamRef(r.Source), Projection: steps}, nil
	case *returns.TypeProjection:
		return encodeEffectReturn(*r)
	default:
		return nil, fmt.Errorf("manifest: unsupported return effect transform %T", ret)
	}
}

func decodeEffectReturn(w *effectReturnWire) (returns.ReturnType, error) {
	if w == nil {
		return nil, nil
	}
	switch w.Kind {
	case "returns.selectCaseOfParam":
		return returns.SelectCaseOfParam{Source: decodeParamRef(w.Source)}, nil
	case "returns.selectResultOfCases":
		return returns.SelectResultOfCases{Cases: decodeParamRef(w.Cases), Default: decodeParamRef(w.Default)}, nil
	case "returns.elementOf":
		return returns.ElementOf{Source: decodeParamRef(w.Source)}, nil
	case "returns.optionalElementOf":
		return returns.OptionalElementOf{Source: decodeParamRef(w.Source)}, nil
	case "returns.callbackReturn":
		return returns.CallbackReturn{CallbackParam: decodeParamRef(w.CallbackParam)}, nil
	case "returns.arrayOfCallbackReturn":
		return returns.ArrayOfCallbackReturn{CallbackParam: decodeParamRef(w.CallbackParam)}, nil
	case "returns.sameAs":
		return returns.SameAs{Source: decodeParamRef(w.Source)}, nil
	case "returns.deepElementOf":
		return returns.DeepElementOf{Source: decodeParamRef(w.Source)}, nil
	case "returns.stringUnpackValue":
		return returns.StringUnpackValue{Format: decodeParamRef(w.Format)}, nil
	case "returns.typeProjection":
		steps, err := decodeProjectionSteps(w.Projection)
		if err != nil {
			return nil, err
		}
		return returns.TypeProjection{Source: decodeParamRef(w.Source), Projection: projection.Projection{Steps: steps}}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown return effect transform kind %q", w.Kind)
	}
}

func encodeExpr(e expr.Expr) (*exprWire, error) {
	if e == nil {
		return nil, nil
	}
	switch ex := e.(type) {
	case expr.Var:
		return &exprWire{Kind: "var", Name: ex.Name}, nil
	case *expr.Var:
		return encodeExpr(*ex)
	case expr.Const:
		return &exprWire{Kind: "const", Value: ex.Value}, nil
	case *expr.Const:
		return encodeExpr(*ex)
	case expr.BinOp:
		left, err := encodeExpr(ex.Left)
		if err != nil {
			return nil, err
		}
		right, err := encodeExpr(ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "binop", Op: ex.Op.String(), Left: left, Right: right}, nil
	case *expr.BinOp:
		return encodeExpr(*ex)
	case expr.Len:
		return &exprWire{Kind: "len", Name: ex.Of}, nil
	case *expr.Len:
		return encodeExpr(*ex)
	case expr.Param:
		return &exprWire{Kind: "param", Index: ex.Index}, nil
	case *expr.Param:
		return encodeExpr(*ex)
	case expr.Ret:
		return &exprWire{Kind: "ret", Index: ex.Index}, nil
	case *expr.Ret:
		return encodeExpr(*ex)
	case expr.ParamLen:
		return &exprWire{Kind: "paramLen", Index: ex.Index}, nil
	case *expr.ParamLen:
		return encodeExpr(*ex)
	case expr.RetLen:
		return &exprWire{Kind: "retLen", Index: ex.Index}, nil
	case *expr.RetLen:
		return encodeExpr(*ex)
	case expr.Min:
		left, err := encodeExpr(ex.Left)
		if err != nil {
			return nil, err
		}
		right, err := encodeExpr(ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "min", Left: left, Right: right}, nil
	case *expr.Min:
		return encodeExpr(*ex)
	case expr.Max:
		left, err := encodeExpr(ex.Left)
		if err != nil {
			return nil, err
		}
		right, err := encodeExpr(ex.Right)
		if err != nil {
			return nil, err
		}
		return &exprWire{Kind: "max", Left: left, Right: right}, nil
	case *expr.Max:
		return encodeExpr(*ex)
	default:
		return nil, fmt.Errorf("manifest: unsupported constraint expr %T", e)
	}
}

func decodeExpr(w *exprWire) (expr.Expr, error) {
	if w == nil {
		return nil, nil
	}
	switch w.Kind {
	case "var":
		return expr.Var{Name: w.Name}, nil
	case "const":
		return expr.Const{Value: w.Value}, nil
	case "binop":
		op, err := decodeExprOp(w.Op)
		if err != nil {
			return nil, err
		}
		left, err := decodeExpr(w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeExpr(w.Right)
		if err != nil {
			return nil, err
		}
		return expr.BinOp{Op: op, Left: left, Right: right}, nil
	case "len":
		return expr.Len{Of: w.Name}, nil
	case "param":
		return expr.Param{Index: w.Index}, nil
	case "ret":
		return expr.Ret{Index: w.Index}, nil
	case "paramLen":
		return expr.ParamLen{Index: w.Index}, nil
	case "retLen":
		return expr.RetLen{Index: w.Index}, nil
	case "min":
		left, err := decodeExpr(w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeExpr(w.Right)
		if err != nil {
			return nil, err
		}
		return expr.Min{Left: left, Right: right}, nil
	case "max":
		left, err := decodeExpr(w.Left)
		if err != nil {
			return nil, err
		}
		right, err := decodeExpr(w.Right)
		if err != nil {
			return nil, err
		}
		return expr.Max{Left: left, Right: right}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown constraint expr kind %q", w.Kind)
	}
}

func encodeProjectionSteps(steps []projection.Step) ([]projectionStepWire, error) {
	out := make([]projectionStepWire, 0, len(steps))
	for _, step := range steps {
		encoded := projectionStepWire{}
		switch step.Kind {
		case projection.StepField:
			encoded.Kind = "field"
			encoded.Field = step.Field
		case projection.StepCallableReturn:
			encoded.Kind = "callableReturn"
		case projection.StepGenericArg:
			encoded.Kind = "genericArg"
			encoded.Index = step.Index
		case projection.StepInstantiateGeneric:
			encoded.Kind = "instantiateGeneric"
			t, err := encodeType(step.Type)
			if err != nil {
				return nil, err
			}
			encoded.Type = t
		default:
			return nil, fmt.Errorf("manifest: unknown projection step kind %d", step.Kind)
		}
		out = append(out, encoded)
	}
	return out, nil
}

func decodeProjectionSteps(w []projectionStepWire) ([]projection.Step, error) {
	steps := make([]projection.Step, 0, len(w))
	for _, step := range w {
		switch step.Kind {
		case "field":
			steps = append(steps, projection.Field(step.Field))
		case "callableReturn":
			steps = append(steps, projection.CallableReturn())
		case "genericArg":
			steps = append(steps, projection.GenericArg(step.Index))
		case "instantiateGeneric":
			t, err := decodeType(step.Type)
			if err != nil {
				return nil, err
			}
			steps = append(steps, projection.InstantiateGeneric(t))
		default:
			return nil, fmt.Errorf("manifest: unknown projection step kind %q", step.Kind)
		}
	}
	return steps, nil
}

func encodeParamRef(ref effect.ParamRef) *paramRefWire {
	return &paramRefWire{Index: ref.Index}
}

func decodeParamRef(w *paramRefWire) effect.ParamRef {
	if w == nil {
		return effect.ParamRef{}
	}
	return effect.ParamRef{Index: w.Index}
}

func encodeIteratorKind(kind iteration.IteratorKind) (string, error) {
	switch kind {
	case iteration.IterateIndexed:
		return "indexed", nil
	case iteration.IterateKeyed:
		return "keyed", nil
	default:
		return "", fmt.Errorf("manifest: unknown iterator kind %d", kind)
	}
}

func decodeIteratorKind(kind string) (iteration.IteratorKind, error) {
	switch kind {
	case "indexed":
		return iteration.IterateIndexed, nil
	case "keyed":
		return iteration.IterateKeyed, nil
	default:
		return 0, fmt.Errorf("manifest: unknown iterator kind %q", kind)
	}
}

func decodeExprOp(op string) (expr.Op, error) {
	switch op {
	case "+":
		return expr.OpAdd, nil
	case "-":
		return expr.OpSub, nil
	case "*":
		return expr.OpMul, nil
	case "/":
		return expr.OpDiv, nil
	case "%":
		return expr.OpMod, nil
	default:
		return 0, fmt.Errorf("manifest: unknown expr op %q", op)
	}
}

func effectLabelWireKey(w effectLabelWire) string {
	data, err := json.Marshal(w)
	if err != nil {
		return w.Kind
	}
	return string(data)
}
