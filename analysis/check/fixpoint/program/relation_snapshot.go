package program

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
)

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
