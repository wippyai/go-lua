package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type channelSelectFactKey struct {
	selectID string
	kind     ChannelSelectFactKind
	result   pathdom.PathKey
	casePath pathdom.PathKey
	index    int
}

func normalizeChannelSelectFacts(in []ChannelSelectFact) []ChannelSelectFact {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[channelSelectFactKey]ChannelSelectFact, len(in))
	for _, fact := range in {
		if fact.Select == "" || fact.Kind == 0 || fact.Index < 0 {
			continue
		}
		if !optionalPlaceholderPath(fact.Result) || !optionalPlaceholderPath(fact.Case) {
			continue
		}
		fact.Result = cloneSummaryPath(fact.Result)
		fact.Case = cloneSummaryPath(fact.Case)
		seen[channelSelectFactKeyOf(fact)] = fact
	}
	return sortedChannelSelectFacts(seen)
}

func cloneChannelSelectFacts(in []ChannelSelectFact) []ChannelSelectFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChannelSelectFact, len(in))
	for i, fact := range in {
		fact.Result = cloneSummaryPath(fact.Result)
		fact.Case = cloneSummaryPath(fact.Case)
		out[i] = fact
	}
	return out
}

func channelSelectFactsEqual(a, b []ChannelSelectFact) bool {
	a = normalizeChannelSelectFacts(a)
	b = normalizeChannelSelectFacts(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if channelSelectFactKeyOf(a[i]) != channelSelectFactKeyOf(b[i]) {
			return false
		}
	}
	return true
}

func channelSelectFactsLessOrEq(a, b []ChannelSelectFact) bool {
	aSet := channelSelectFactsSet(a)
	for _, fact := range normalizeChannelSelectFacts(b) {
		if _, ok := aSet[channelSelectFactKeyOf(fact)]; !ok {
			return false
		}
	}
	return true
}

func joinChannelSelectFacts(a, b []ChannelSelectFact) []ChannelSelectFact {
	aSet := channelSelectFactsSet(a)
	bSet := channelSelectFactsSet(b)
	if len(aSet) == 0 || len(bSet) == 0 {
		return nil
	}
	out := make(map[channelSelectFactKey]ChannelSelectFact)
	for key, fact := range aSet {
		if _, ok := bSet[key]; ok {
			out[key] = fact
		}
	}
	return sortedChannelSelectFacts(out)
}

func channelSelectFactsSet(in []ChannelSelectFact) map[channelSelectFactKey]ChannelSelectFact {
	out := normalizeChannelSelectFacts(in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[channelSelectFactKey]ChannelSelectFact, len(out))
	for _, fact := range out {
		m[channelSelectFactKeyOf(fact)] = fact
	}
	return m
}

func optionalPlaceholderPath(path pathdom.Path) bool {
	return path.IsEmpty() || path.IsPlaceholder()
}

func channelSelectFactKeyOf(fact ChannelSelectFact) channelSelectFactKey {
	return channelSelectFactKey{
		selectID: fact.Select,
		kind:     fact.Kind,
		result:   fact.Result.Key(),
		casePath: fact.Case.Key(),
		index:    fact.Index,
	}
}

func sortedChannelSelectFacts(in map[channelSelectFactKey]ChannelSelectFact) []ChannelSelectFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChannelSelectFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		left := channelSelectFactKeyOf(out[i])
		right := channelSelectFactKeyOf(out[j])
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
		return left.index < right.index
	})
	return out
}
