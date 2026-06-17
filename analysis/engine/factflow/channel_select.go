package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ChannelSelectID identifies one select operation in factflow event space.
type ChannelSelectID string

// ChannelSelectKind identifies channel-select evidence emitted at a CFG point.
type ChannelSelectKind uint8

const (
	ChannelSelectUnknown ChannelSelectKind = iota
	ChannelSelectSelect
	ChannelSelectReceive
	ChannelSelectCase
)

// ChannelSelectConfig carries optional path evidence for NewChannelSelect.
type ChannelSelectConfig struct {
	SelectID ChannelSelectID
	Kind     ChannelSelectKind

	ResultPath    path.Path
	HasResultPath bool

	CasePath    path.Path
	HasCasePath bool

	PayloadValue    product.Value
	HasPayloadValue bool

	Index      int
	HasDefault bool
}

// ChannelSelect describes select/case/result path evidence at a CFG point.
type ChannelSelect struct {
	selectID ChannelSelectID
	kind     ChannelSelectKind

	resultPath    path.Path
	hasResultPath bool

	casePath    path.Path
	hasCasePath bool

	payloadValue    product.Value
	hasPayloadValue bool

	index      int
	hasDefault bool
}

// ChannelSelectSet groups channel-select facts emitted at the same CFG point.
type ChannelSelectSet struct {
	events []ChannelSelect
}

// NewChannelSelect creates a channel-select evidence event.
func NewChannelSelect(config ChannelSelectConfig) ChannelSelect {
	return ChannelSelect{
		selectID:        config.SelectID,
		kind:            config.Kind,
		resultPath:      config.ResultPath.Clone(),
		hasResultPath:   config.HasResultPath,
		casePath:        config.CasePath.Clone(),
		hasCasePath:     config.HasCasePath,
		payloadValue:    config.PayloadValue,
		hasPayloadValue: config.HasPayloadValue,
		index:           config.Index,
		hasDefault:      config.HasDefault,
	}
}

// NewChannelSelectSet creates a channel-select evidence set.
func NewChannelSelectSet(events ...ChannelSelect) ChannelSelectSet {
	return ChannelSelectSet{events: copyChannelSelectSlice(events)}
}

// SelectID returns the select operation identity.
func (s ChannelSelect) SelectID() ChannelSelectID { return s.selectID }

// Kind returns the channel-select evidence kind.
func (s ChannelSelect) Kind() ChannelSelectKind { return s.kind }

// ResultPath returns result path evidence, if present.
func (s ChannelSelect) ResultPath() (path.Path, bool) {
	if !s.hasResultPath {
		return path.Path{}, false
	}
	return s.resultPath.Clone(), true
}

// CasePath returns case path evidence, if present.
func (s ChannelSelect) CasePath() (path.Path, bool) {
	if !s.hasCasePath {
		return path.Path{}, false
	}
	return s.casePath.Clone(), true
}

// PayloadValue returns payload value evidence for receive cases, if present.
func (s ChannelSelect) PayloadValue() (product.Value, bool) {
	if !s.hasPayloadValue {
		return product.Value{}, false
	}
	return s.payloadValue, true
}

// Index returns the select case index.
func (s ChannelSelect) Index() int { return s.index }

// HasDefault reports whether this select event had an explicit default case.
func (s ChannelSelect) HasDefault() bool { return s.hasDefault }

func (s ChannelSelect) copy() ChannelSelect {
	s.resultPath = s.resultPath.Clone()
	s.casePath = s.casePath.Clone()
	return s
}

// Events returns the channel-select events in deterministic order.
func (s ChannelSelectSet) Events() []ChannelSelect {
	return copyChannelSelectSlice(s.events)
}

func (s ChannelSelectSet) copy() ChannelSelectSet {
	return ChannelSelectSet{events: copyChannelSelectSlice(s.events)}
}

func copyChannelSelectMap(in map[cfg.Point]ChannelSelectSet) map[cfg.Point]ChannelSelectSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]ChannelSelectSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func copyChannelSelectSlice(in []ChannelSelect) []ChannelSelect {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChannelSelect, len(in))
	for i, event := range in {
		out[i] = event.copy()
	}
	return out
}
