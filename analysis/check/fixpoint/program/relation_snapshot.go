package program

import (
	"fmt"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// PinnedSummaries returns the complete equation set discharged by a strict
// transaction. Zero-boundary producers publish their lexical summary;
// parameterized producers publish only exact certified context summaries, so
// their unbound lexical base remains a normal equation. False rejects the
// complete activation transaction.
func (s relationRunSnapshot) PinnedSummaries() ([]summary.EntrySummary, bool) {
	identities := s.Entries()
	out := make([]summary.EntrySummary, 0, len(identities)+len(s.contexts))
	contextCount := make(map[summary.SummaryKey]int, len(s.contexts))
	for _, contextual := range s.contexts {
		contextCount[contextual.base.Summary]++
		out = append(out, summary.EntrySummary{Key: contextual.context, Summary: contextual.summary})
	}
	for _, identity := range identities {
		relation, ok := s.Lookup(identity)
		if !ok {
			return nil, false
		}
		if relation.Shape() != (transformer.Shape{}) {
			if contextCount[identity.Summary] == 0 {
				return nil, false
			}
			continue
		}
		cursor, err := transformer.NewBindingCursor(relation.Shape(), nil, nil)
		if err != nil {
			return nil, false
		}
		projected, exact := relation.Specialize(cursor, nil, nil)
		if !exact {
			return nil, false
		}
		out = append(out, summary.EntrySummary{Key: identity.Summary, Summary: projected})
	}
	slices.SortFunc(out, func(a, b summary.EntrySummary) int {
		if a.Key.Less(b.Key) {
			return -1
		}
		if b.Key.Less(a.Key) {
			return 1
		}
		return 0
	})
	for i := 1; i < len(out); i++ {
		if out[i-1].Key == out[i].Key {
			return nil, false
		}
	}
	return out, true
}

type relationFreezeCategory string

const (
	relationFreezeIdentity   relationFreezeCategory = "identity"
	relationFreezeEquation   relationFreezeCategory = "equation"
	relationFreezeContextual relationFreezeCategory = "contextual"
	relationFreezeWidened    relationFreezeCategory = "widened"
	relationFreezeEmpty      relationFreezeCategory = "empty"
)

type relationFreezeError struct {
	Category relationFreezeCategory
	Identity relationCellIdentity
	Err      error
}

func (e relationFreezeError) Error() string {
	if e.Identity.Cell == (transformer.CellRef{}) {
		return fmt.Sprintf("relation freeze %s: %v", e.Category, e.Err)
	}
	return fmt.Sprintf("relation freeze %s for %v: %v", e.Category, e.Identity.Cell, e.Err)
}

func (e relationFreezeError) Unwrap() error { return e.Err }

// relationRunSnapshot is the immutable publication boundary for one catalog
// generation. Relations are addressable only through their complete producer
// identity; consumers similarly need an identity minted by this generation.
type relationRunSnapshot struct {
	generation *relationCatalogGeneration
	relations  transformer.RelationSnapshot
	identities map[transformer.CellRef]relationCellIdentity
	consumers  relationConsumerPolicy
	contexts   []relationContextSummary
}

type relationContextSummary struct {
	context             summary.SummaryKey
	base                relationCellIdentity
	discoveryGeneration uint64
	certificate         *relationContextEntryCertificate
	summary             summary.Summary
}

func (s relationRunSnapshot) ContextSummary(context summary.SummaryKey, certificate *relationContextEntryCertificate, preparedDigest uint64) (summary.Summary, bool) {
	if certificate == nil || certificate.context != context || certificate.preparedBodyDigest != preparedDigest || certificate.discoveryGeneration == 0 {
		return summary.Summary{}, false
	}
	i, ok := slices.BinarySearchFunc(s.contexts, context, func(entry relationContextSummary, target summary.SummaryKey) int {
		if entry.context.Less(target) {
			return -1
		}
		if target.Less(entry.context) {
			return 1
		}
		return 0
	})
	if !ok {
		return summary.Summary{}, false
	}
	entry := s.contexts[i]
	if entry.certificate != certificate || entry.context != certificate.context || entry.base.Summary != certificate.base || entry.base.BodyDigest != certificate.preparedBodyDigest || entry.discoveryGeneration != certificate.discoveryGeneration {
		return summary.Summary{}, false
	}
	return entry.summary, true
}

func (s relationRunSnapshot) contextSummaryByKey(context summary.SummaryKey) (summary.Summary, bool) {
	i, ok := slices.BinarySearchFunc(s.contexts, context, func(entry relationContextSummary, target summary.SummaryKey) int {
		if entry.context.Less(target) {
			return -1
		}
		if target.Less(entry.context) {
			return 1
		}
		return 0
	})
	if !ok {
		return summary.Summary{}, false
	}
	return s.contexts[i].summary, true
}

func (s relationRunSnapshot) Entries() []relationCellIdentity {
	entries := s.relations.Entries()
	out := make([]relationCellIdentity, 0, len(entries))
	for _, entry := range entries {
		if identity, ok := s.identities[entry.Ref]; ok {
			out = append(out, identity)
		}
	}
	return out
}

func (s relationRunSnapshot) Lookup(identity relationCellIdentity) (transformer.Relation, bool) {
	if s.generation == nil || identity.Generation != s.generation {
		return transformer.Relation{}, false
	}
	frozen, ok := s.identities[identity.Cell]
	if !ok || frozen != identity {
		return transformer.Relation{}, false
	}
	return s.relations.Lookup(identity.Cell)
}

// Identity binds a frozen cell reference back to the complete authority that
// minted it. Consumers must carry this value rather than pairing CellRef with
// an independently selected summary key.
func (s relationRunSnapshot) Identity(ref transformer.CellRef) (relationCellIdentity, bool) {
	identity, ok := s.identities[ref]
	return identity, ok && identity.Generation == s.generation
}

func (s relationRunSnapshot) DirectCalls(owner relationConsumerIdentity) (transformer.DirectCallCatalog, bool) {
	if s.generation == nil || owner.Generation != s.generation || s.consumers.generation != s.generation {
		return transformer.DirectCallCatalog{}, false
	}
	return s.consumers.DirectCalls(owner)
}

func (s relationRunSnapshot) DependencyKey(owner relationConsumerIdentity, point cfg.Point) (summary.SummaryKey, bool) {
	if s.generation == nil || owner.Generation != s.generation || s.consumers.generation != s.generation {
		return summary.SummaryKey{}, false
	}
	return s.consumers.DependencyKey(owner, point)
}
