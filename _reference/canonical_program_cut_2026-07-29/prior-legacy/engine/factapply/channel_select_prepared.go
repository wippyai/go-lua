package factapply

import (
	"context"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// PreparedChannelSelectPath is one syntax path bound once to the canonical
// StateKey carrier used by the registered ChannelSelect lane.  Formal roots
// use keyspace's injective reserved spelling, so the same carrier round-trips
// through ordinary boundary transport without a second fact representation.
type PreparedChannelSelectPath struct {
	source pathdom.Path
	key    pathaddr.StateKey
	bound  bool
}

func (p PreparedChannelSelectPath) SourcePath() pathdom.Path    { return p.source.Clone() }
func (p PreparedChannelSelectPath) StateKey() pathaddr.StateKey { return p.key }
func (p PreparedChannelSelectPath) Bound() bool                 { return p.bound }

type ChannelSelectPathBinder func(pathdom.Path) (pathaddr.StateKey, bool)
type ChannelSelectResultBinder[K comparable] func(cfg.Point, int) (K, bool)

type preparedChannelSelectStep struct {
	fact        channelselectfact.Fact
	publish     bool
	casePath    PreparedChannelSelectPath
	hasCasePath bool
	payload     product.Value
	hasPayload  bool
}

type preparedChannelSelectGroup[K comparable] struct {
	selectID   factflow.ChannelSelectID
	result     K
	hasResult  bool
	hasDefault bool
	cases      []preparedChannelSelectStep
}

// PreparedChannelSelectTransaction is the sole bound executable form of the
// ordered N3 syntax.  It contains no State and no resolver; concrete and
// formal execution supply only path reads and result-slot vocabulary.
type PreparedChannelSelectTransaction[K comparable] struct {
	steps    []preparedChannelSelectStep
	groups   []preparedChannelSelectGroup[K]
	complete bool
}

func (p PreparedChannelSelectTransaction[K]) Len() int { return len(p.steps) }
func (p PreparedChannelSelectTransaction[K]) Complete() bool {
	return p.complete && len(p.steps) != 0
}

func PrepareChannelSelectTransaction[K comparable](reg *axis.Registry, transaction ChannelSelectTransaction, bind ChannelSelectPathBinder, bindResult ChannelSelectResultBinder[K]) (PreparedChannelSelectTransaction[K], error) {
	if reg == nil || bind == nil || bindResult == nil || !transaction.Valid(reg) || !transaction.HasPublicationSteps() {
		return PreparedChannelSelectTransaction[K]{}, fmt.Errorf("factapply: invalid channel-select transaction")
	}
	out := PreparedChannelSelectTransaction[K]{steps: make([]preparedChannelSelectStep, 0, transaction.Len()), complete: true}
	groupIndex := make(map[factflow.ChannelSelectID]int)
	for index := 0; index < transaction.Len(); index++ {
		step, ok := transaction.Step(index)
		if !ok {
			return PreparedChannelSelectTransaction[K]{}, fmt.Errorf("factapply: channel-select step %d is missing", index)
		}
		event := step.Event()
		kind, ok := channelSelectKind(event.Kind())
		if !ok {
			return PreparedChannelSelectTransaction[K]{}, fmt.Errorf("factapply: channel-select step %d has invalid kind", index)
		}
		prepared := preparedChannelSelectStep{publish: true, fact: channelselectfact.Fact{
			Select: channelselectfact.ID(event.SelectID()), Kind: kind,
			Index: event.Index(), HasDefault: event.HasDefault(),
		}}
		if path, present := event.ResultPath(); present {
			bound := bindPreparedChannelSelectPath(path, bind)
			prepared.fact.Result = bound.key
			prepared.publish = prepared.publish && bound.bound
			out.complete = out.complete && bound.bound
		}
		if path, present := event.CasePath(); present {
			bound := bindPreparedChannelSelectPath(path, bind)
			prepared.fact.Case, prepared.casePath, prepared.hasCasePath = bound.key, bound, true
			prepared.publish = prepared.publish && bound.bound
			out.complete = out.complete && bound.bound
		}
		if payload, present := event.PayloadValue(); present {
			prepared.fact.Payload, prepared.fact.HasPayload = payload, true
			prepared.payload, prepared.hasPayload = payload, true
		}
		out.steps = append(out.steps, prepared)

		groupAt, exists := groupIndex[event.SelectID()]
		if !exists {
			groupAt = len(out.groups)
			groupIndex[event.SelectID()] = groupAt
			out.groups = append(out.groups, preparedChannelSelectGroup[K]{selectID: event.SelectID()})
		}
		group := &out.groups[groupAt]
		switch event.Kind() {
		case factflow.ChannelSelectSelect:
			result, bound := bindResult(transaction.Point(), event.Index())
			if event.Index() < 0 || !bound {
				return PreparedChannelSelectTransaction[K]{}, fmt.Errorf("factapply: channel-select result %d is unbound", event.Index())
			}
			group.result, group.hasResult, group.hasDefault = result, true, event.HasDefault()
		case factflow.ChannelSelectReceive:
			if prepared.hasPayload || prepared.hasCasePath {
				group.cases = append(group.cases, prepared)
			}
		}
	}
	return out, nil
}

func bindPreparedChannelSelectPath(path pathdom.Path, bind ChannelSelectPathBinder) PreparedChannelSelectPath {
	key, ok := bind(path.Clone())
	if !ok || key == "" {
		return PreparedChannelSelectPath{source: path.Clone()}
	}
	return PreparedChannelSelectPath{source: path.Clone(), key: key, bound: true}
}

type ChannelSelectPathValueReader func(PreparedChannelSelectPath) (product.Value, bool)

type ChannelSelectResultWrite[K comparable] struct {
	Target K
	Value  product.Value
}

type ChannelSelectEvaluation[K comparable] struct {
	facts  []channelselectfact.Fact
	writes []ChannelSelectResultWrite[K]
}

func (e ChannelSelectEvaluation[K]) Facts() []channelselectfact.Fact {
	return append([]channelselectfact.Fact(nil), e.facts...)
}

func (e ChannelSelectEvaluation[K]) ResultWrites() []ChannelSelectResultWrite[K] {
	return append([]ChannelSelectResultWrite[K](nil), e.writes...)
}

// EvaluatePreparedChannelSelect is the sole semantic evaluator for concrete
// and formal ChannelSelect execution. Facts retain lexical N3 order; result
// groups retain first-occurrence order and duplicate cases remain distinct.
func EvaluatePreparedChannelSelect[K comparable](ctx context.Context, reg *axis.Registry, typeValues *typevalue.Cache, prepared PreparedChannelSelectTransaction[K], read ChannelSelectPathValueReader) (ChannelSelectEvaluation[K], error) {
	if ctx == nil || reg == nil || prepared.Len() == 0 {
		return ChannelSelectEvaluation[K]{}, fmt.Errorf("factapply: invalid prepared channel-select evaluation")
	}
	out := ChannelSelectEvaluation[K]{facts: make([]channelselectfact.Fact, 0, len(prepared.steps))}
	for _, step := range prepared.steps {
		if err := ctx.Err(); err != nil {
			return ChannelSelectEvaluation[K]{}, err
		}
		if step.publish {
			out.facts = append(out.facts, step.fact)
		}
	}
	for _, group := range prepared.groups {
		if typeValues == nil || !group.hasResult || len(group.cases) == 0 && !group.hasDefault {
			continue
		}
		cases := make([]channelselect.ResultCase, 0, len(group.cases))
		for _, item := range group.cases {
			payloadType := typ.Unknown
			if item.hasPayload {
				if resolved, ok := valueWitnessType(reg, item.payload); ok {
					payloadType = resolved
				}
			} else if item.hasCasePath && read != nil {
				if value, ok := read(item.casePath); ok {
					if resolved, ok := channelPayloadTypeFromValue(reg, value); ok {
						payloadType = resolved
					}
				}
			}
			cases = append(cases, channelselect.ResultCase{Index: item.fact.Index, Payload: payloadType})
		}
		resultType, ok := channelselect.ResultValueTypeWithDefault(string(group.selectID), cases, group.hasDefault)
		if !ok {
			continue
		}
		out.writes = append(out.writes, ChannelSelectResultWrite[K]{Target: group.result, Value: typeValues.FromTypeWithWitness(reg, resultType)})
	}
	return out, nil
}
