package callboundary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
)

type channelSelectFactKey struct {
	selectID   string
	kind       channelselectfact.Kind
	result     pathdom.PathKey
	casePath   pathdom.PathKey
	index      int
	hasDefault bool
}

// channelSelectLane is the canonical must (intersection) lattice for channel
// select facts: a fact survives a join only when present on every path.
var channelSelectLane = factset.Set[channelSelectFactKey, ChannelSelectFact]{
	Key: channelSelectFactKeyOf,
	EqualFact: func(a, b ChannelSelectFact) bool {
		return channelSelectFactKeyOf(a) == channelSelectFactKeyOf(b)
	},
	Less: channelSelectFactLess,
	Valid: func(f ChannelSelectFact) bool {
		return f.Select != "" && f.Kind != 0 && f.Index >= 0 &&
			optionalPlaceholderPath(f.Result) && optionalPlaceholderPath(f.Case)
	},
	CloneFact: func(f ChannelSelectFact) ChannelSelectFact {
		f.Result = f.Result.Clone()
		f.Case = f.Case.Clone()
		return f
	},
	Prefer:    func(kept, incoming ChannelSelectFact) bool { return true },
	Intersect: true,
}

func optionalPlaceholderPath(path pathdom.Path) bool {
	return path.IsEmpty() || path.IsPlaceholder()
}

func channelSelectFactKeyOf(fact ChannelSelectFact) channelSelectFactKey {
	return channelSelectFactKey{
		selectID:   string(fact.Select),
		kind:       fact.Kind,
		result:     fact.Result.Key(),
		casePath:   fact.Case.Key(),
		index:      fact.Index,
		hasDefault: fact.HasDefault,
	}
}

func channelSelectFactLess(a, b ChannelSelectFact) bool {
	left := channelSelectFactKeyOf(a)
	right := channelSelectFactKeyOf(b)
	if left.selectID != right.selectID {
		return left.selectID < right.selectID
	}
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	if left.result != right.result {
		return left.result < right.result
	}
	if left.casePath != right.casePath {
		return left.casePath < right.casePath
	}
	if left.index != right.index {
		return left.index < right.index
	}
	return !left.hasDefault && right.hasDefault
}
