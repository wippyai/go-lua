package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
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

	Index int
}

// ChannelSelect describes select/case/result path evidence at a CFG point.
type ChannelSelect struct {
	selectID ChannelSelectID
	kind     ChannelSelectKind

	resultPath    path.Path
	hasResultPath bool

	casePath    path.Path
	hasCasePath bool

	index int
}

// ChannelSelectSet groups channel-select facts emitted at the same CFG point.
type ChannelSelectSet struct {
	events []ChannelSelect
}

// NewChannelSelect creates a channel-select evidence event.
func NewChannelSelect(config ChannelSelectConfig) ChannelSelect {
	return ChannelSelect{
		selectID:      config.SelectID,
		kind:          config.Kind,
		resultPath:    copyPath(config.ResultPath),
		hasResultPath: config.HasResultPath,
		casePath:      copyPath(config.CasePath),
		hasCasePath:   config.HasCasePath,
		index:         config.Index,
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
	return copyPath(s.resultPath), true
}

// CasePath returns case path evidence, if present.
func (s ChannelSelect) CasePath() (path.Path, bool) {
	if !s.hasCasePath {
		return path.Path{}, false
	}
	return copyPath(s.casePath), true
}

// Index returns the select case index.
func (s ChannelSelect) Index() int { return s.index }

func (s ChannelSelect) copy() ChannelSelect {
	s.resultPath = copyPath(s.resultPath)
	s.casePath = copyPath(s.casePath)
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
