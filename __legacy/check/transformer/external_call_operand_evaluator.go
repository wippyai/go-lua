package transformer

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type externalCallOperandSelection struct {
	preferred    cfg.Point
	hasPreferred bool
	fallback     cfg.Point
	hasFallback  bool
}

type externalCallOperandWire struct {
	wire              callpayload.ExternalCallInputWire
	layout            callpayload.ExternalCallInputWireLayout[statekey.Value]
	dynamicProjection *state.DynamicReadFactorProjection
}

// externalCallOperandSuspension is the deliberate hand-off between canonical
// ValueTerm evaluation and the formal external-call adapter.  It is not an
// evaluation failure: the adapter must partition the current guarded leaf by
// exactly Query's next cursor demand, resume the cursor with that evidence,
// and invoke the evaluator again.  Re-evaluation is safe because ValueTerms
// are immutable and every DynamicReadDemandCursor round is monotone.
//
// The type stays private to keep the suspension protocol owned by the
// transformer rather than leaking a second dynamic-read API to providers.
type externalCallOperandSuspension struct {
	point cfg.Point
	query state.DynamicReadQuery
}

func (s externalCallOperandSuspension) Error() string {
	return "transformer: external-call operand dynamic read suspended"
}

// externalCallDynamicEvidence resolves one exact DynamicRead query from the
// current guarded formal leaf.  complete=false requests suspension; returning
// an error is a malformed adapter/evidence failure.  The callback receives no
// State and cannot widen the frozen read authority.
type externalCallDynamicEvidence func(cfg.Point, state.DynamicReadQuery) (evidence state.DynamicReadEvidence, complete bool, err error)

func (w externalCallOperandWire) value(slot statekey.Value) (product.Value, bool) {
	ordinal, ok := w.layout.ValueOrdinal(slot)
	if !ok {
		return product.Value{}, false
	}
	return w.wire.Value(ordinal)
}

// evaluateExternalCallOperands interprets each retained top term exactly once
// through the shared ValueTerm algebra. The only observations supplied here
// are dense Values roots and registered lane factors from CallPayload's sealed
// frame; no State is reconstructed.
func evaluateExternalCallOperands(
	body *relationProgramBody,
	terms callOutcomeOperandTerms,
	access []valueAccessTerm,
	frame callpayload.ExternalCallInputFrame[statekey.Value],
	dynamicQuery func(valueNode, []product.Value) (state.DynamicReadQuery, error),
) (callpayload.CallOutcomeValueOperands, error) {
	return evaluateExternalCallOperandsWithDynamicEvidence(body, terms, access, frame, dynamicQuery, nil)
}

// evaluateExternalCallOperandsWithDynamicEvidence is the resumable form used
// by formal external-call execution.  The legacy direct projection remains
// available for non-formal callers and focused evaluator tests.
func evaluateExternalCallOperandsWithDynamicEvidence(
	body *relationProgramBody,
	terms callOutcomeOperandTerms,
	access []valueAccessTerm,
	frame callpayload.ExternalCallInputFrame[statekey.Value],
	dynamicQuery func(valueNode, []product.Value) (state.DynamicReadQuery, error),
	resolveDynamic externalCallDynamicEvidence,
) (callpayload.CallOutcomeValueOperands, error) {
	if body == nil || body.relation.arena == nil || !frame.Domain().Valid() || dynamicQuery == nil {
		return callpayload.CallOutcomeValueOperands{}, fmt.Errorf("transformer: external-call operand evaluator is unowned")
	}
	primaryWire, primaryLayout, ok := frame.Primary()
	if !ok {
		return callpayload.CallOutcomeValueOperands{}, fmt.Errorf("transformer: external-call primary operand wire is absent")
	}
	primary := &externalCallOperandWire{wire: primaryWire, layout: primaryLayout}
	selections := make(map[ValueTerm]externalCallOperandSelection)
	for _, item := range access {
		selection := selections[item.term]
		if item.fallback {
			if selection.hasFallback && selection.fallback != item.point {
				return callpayload.CallOutcomeValueOperands{}, fmt.Errorf("transformer: value term %d has multiple fallback wires", item.term)
			}
			selection.fallback, selection.hasFallback = item.point, true
		} else {
			if selection.hasPreferred && selection.preferred != item.point {
				return callpayload.CallOutcomeValueOperands{}, fmt.Errorf("transformer: value term %d has multiple preferred wires", item.term)
			}
			selection.preferred, selection.hasPreferred = item.point, true
		}
		selections[item.term] = selection
	}
	preparedWires := map[cfg.Point]*externalCallOperandWire{primaryLayout.Point(): primary}
	wireAt := func(point cfg.Point) (*externalCallOperandWire, bool, error) {
		if point == primaryLayout.Point() {
			return primary, true, nil
		}
		if prepared, present := preparedWires[point]; present {
			return prepared, true, nil
		}
		wire, layout, present, err := frame.JoinHistorical(point)
		if err != nil || !present {
			return nil, present, err
		}
		prepared := &externalCallOperandWire{wire: wire, layout: layout}
		preparedWires[point] = prepared
		return prepared, true, nil
	}
	selectWire := func(term ValueTerm, inherited *externalCallOperandWire) (*externalCallOperandWire, error) {
		selection, present := selections[term]
		if !present || !selection.hasPreferred {
			return inherited, nil
		}
		preferred, ok, err := wireAt(selection.preferred)
		if err != nil || !ok {
			if err == nil {
				err = fmt.Errorf("transformer: value term %d preferred wire is absent", term)
			}
			return nil, err
		}
		if preferred.wire.Reachable() || !selection.hasFallback {
			return preferred, nil
		}
		fallback, ok, err := wireAt(selection.fallback)
		if err != nil || !ok {
			if err == nil {
				err = fmt.Errorf("transformer: value term %d fallback wire is absent", term)
			}
			return nil, err
		}
		return fallback, nil
	}

	arena := body.relation.arena
	var leafErr error
	var projectionPlan state.DynamicReadFactorProjectionPlan
	projectionPlanSealed := false
	var makeResolver func(*externalCallOperandWire) valueNodeLeafResolver
	makeResolver = func(current *externalCallOperandWire) valueNodeLeafResolver {
		var resolver valueNodeLeafResolver
		resolver = valueNodeLeafResolver{
			scope: func(term ValueTerm, inherited valueNodeLeafResolver) valueNodeLeafResolver {
				selected, err := selectWire(term, current)
				if err != nil {
					return valueNodeLeafResolver{}
				}
				return makeResolver(selected)
			},
			root: func(root Root) (product.Value, bool) {
				slot, exact := body.rootValueSlot(root)
				if !exact {
					return product.Value{}, false
				}
				return current.value(slot)
			},
			slot: current.value,
			frameResult: func(node valueNode) (product.Value, bool) {
				if node.frame == 0 || int(node.frame) >= len(body.frames) || node.resultIndex < 0 {
					return product.Value{}, false
				}
				frame := body.frames[node.frame]
				if !frame.valid() || node.resultIndex >= len(frame.resultSelectors) {
					return product.Value{}, false
				}
				out := product.Bottom(body.productDomain.Registry())
				found := false
				for _, target := range frame.resultSelectors[node.resultIndex].targets {
					if !target.stateTarget || target.slot == 0 {
						continue
					}
					value, ok := current.value(target.slot)
					if !ok {
						return product.Value{}, false
					}
					out, found = product.Join(body.productDomain.Registry(), out, value), true
				}
				return out, found
			},
			dynamicRead: func(node valueNode, args []product.Value) (product.Value, bool) {
				queryArgs := append([]product.Value(nil), args...)
				if node.integerProof != 0 {
					proof, exact := arena.evalValueCanonicalWithLeaves(node.integerProof, resolver)
					if !exact {
						return product.Value{}, false
					}
					queryArgs = append(queryArgs, proof)
				}
				query, err := dynamicQuery(node, queryArgs)
				if err != nil {
					leafErr = err
					return product.Value{}, false
				}
				if resolveDynamic != nil {
					evidence, complete, resolveErr := resolveDynamic(current.layout.Point(), query)
					if resolveErr != nil {
						leafErr = resolveErr
						return product.Value{}, false
					}
					if !complete {
						leafErr = externalCallOperandSuspension{point: current.layout.Point(), query: query}
						return product.Value{}, false
					}
					value, exact := sourcevalue.ResolveDynamicRead(query, evidence)
					if !exact {
						leafErr = fmt.Errorf("dynamic-read evidence did not resolve")
					}
					return value, exact
				}
				if current.dynamicProjection == nil {
					if !projectionPlanSealed {
						projectionPlan, err = body.productDomain.SealDynamicReadFactorProjectionPlan(query.KeySpace)
						if err != nil {
							leafErr = err
							return product.Value{}, false
						}
						projectionPlanSealed = true
					}
					projection, bindErr := body.productDomain.BindDynamicReadFactorProjection(projectionPlan, current.wire.Factors())
					if bindErr != nil {
						leafErr = bindErr
						return product.Value{}, false
					}
					current.dynamicProjection = &projection
				}
				evidence, err := body.productDomain.ProjectDynamicReadEvidenceFromFactorProjection(query, current.dynamicProjection)
				if err != nil {
					leafErr = err
					return product.Value{}, false
				}
				value, exact := sourcevalue.ResolveDynamicRead(query, evidence)
				if !exact {
					leafErr = fmt.Errorf("dynamic-read evidence did not resolve")
				}
				return value, exact
			},
			allocationResult: func(node valueNode) (product.Value, bool) {
				return arena.allocationResult(node.allocation, node.resultIndex)
			},
		}
		return resolver
	}
	evaluate := func(term ValueTerm) (product.Value, error) {
		resolver := makeResolver(primary)
		value, exact := arena.evalValueCanonicalWithLeaves(term, resolver)
		if !exact {
			if leafErr != nil {
				return product.Value{}, fmt.Errorf("transformer: external-call operand term %d is not exact: %w", term, leafErr)
			}
			return product.Value{}, fmt.Errorf("transformer: external-call operand term %d is not exact", term)
		}
		return value, nil
	}
	out := callpayload.CallOutcomeValueOperands{HasCallee: terms.hasCallee, HasReceiver: terms.hasReceiver, Arguments: make([]callpayload.CallOutcomeArgumentOperand, len(terms.arguments))}
	var err error
	if terms.hasCallee {
		out.Callee, err = evaluate(terms.callee)
		if err != nil {
			return callpayload.CallOutcomeValueOperands{}, err
		}
	}
	if terms.hasReceiver {
		out.Receiver, err = evaluate(terms.receiver)
		if err != nil {
			return callpayload.CallOutcomeValueOperands{}, err
		}
	}
	for index, term := range terms.arguments {
		value, err := evaluate(term)
		if err != nil {
			return callpayload.CallOutcomeValueOperands{}, err
		}
		out.Arguments[index] = callpayload.CallOutcomeArgumentOperand{Value: value, Present: true}
	}
	return out, nil
}

func concreteExternalCallDynamicQuery(body *relationProgramBody) func(valueNode, []product.Value) (state.DynamicReadQuery, error) {
	return func(node valueNode, args []product.Value) (state.DynamicReadQuery, error) {
		if body == nil {
			return state.DynamicReadQuery{}, fmt.Errorf("transformer: external-call dynamic query is unowned")
		}
		keys := body.keys
		if (keys == nil || !keys.Valid()) && body.pathSemantics != nil {
			keys = body.pathSemantics.KeySpace()
		}
		return dynamicValueQuery(body, keys, node, args)
	}
}
