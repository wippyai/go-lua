package relinput

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
)

// Placement is the composition's answer for one mounted rule: the decision
// scope its candidate rows are decided at, and the decision scope each
// declared input port observes, in the rule's own port order.
//
// Scope is composition data rather than rule-declaration data. A port is an
// observation point, and which relation-schema conjunction that point stands
// in is decided where the rule is composed, so it is stated here instead of
// being inferred from a read's port ordinal.
type Placement struct {
	Candidate model.ScopeID
	Ports     []model.ScopeID
}

// Composition is the one boundary Seal consults. The composition that mounts
// a rule answers where that rule is placed and what region evidence each
// scope it named stands on. Seal snapshots every answer; it holds no
// composition authority after it returns.
type Composition interface {
	// Placement states one mounted rule's decision scopes by its dense rule
	// ordinal. A composition that cannot place the rule answers false, and
	// Seal refuses with that ordinal named.
	Placement(ordinal int) (Placement, bool)
	// ScopeRegion states the owner-issued region evidence one named scope
	// stands on.
	ScopeRegion(scope model.ScopeID) (identity.ContentID, bool)
}

// Reason names the exact seal boundary that refused a bundle. It is an
// identity rather than free-form text, so equal refusals stay equal across
// compositions.
type Reason uint8

const (
	// ReasonCatalog reports a rule catalog that is not sealed.
	ReasonCatalog Reason = iota + 1
	// ReasonOwner reports a scope-issuing owner that names no authority.
	ReasonOwner
	// ReasonComposition reports that no composition was supplied to answer
	// the placements the catalog requires.
	ReasonComposition
	// ReasonPlan reports a rule ordinal the catalog does not hold.
	ReasonPlan
	// ReasonUnplaced reports a mounted rule the composition cannot place.
	ReasonUnplaced
	// ReasonPortWidth reports a placement whose port vector is not the width
	// the rule declared.
	ReasonPortWidth
	// ReasonScope reports a placement naming a scope that is unavailable.
	ReasonScope
	// ReasonForeignOwner reports a placement naming a scope issued by an
	// authority other than the bundle's own.
	ReasonForeignOwner
	// ReasonRegion reports a named scope the composition supplies no region
	// evidence for.
	ReasonRegion
)

func (reason Reason) String() string {
	switch reason {
	case ReasonCatalog:
		return "rule catalog"
	case ReasonOwner:
		return "scope owner"
	case ReasonComposition:
		return "composition"
	case ReasonPlan:
		return "rule plan"
	case ReasonUnplaced:
		return "unplaced rule"
	case ReasonPortWidth:
		return "port width"
	case ReasonScope:
		return "scope"
	case ReasonForeignOwner:
		return "foreign scope owner"
	case ReasonRegion:
		return "scope region evidence"
	default:
		return "unknown"
	}
}

// Refusal is one typed seal refusal. It names the boundary and, where the
// refusal belongs to one rule, the dense ordinal of that rule.
type Refusal struct {
	reason  Reason
	ordinal int
	ruled   bool
}

// Reason identifies the seal boundary that refused.
func (refusal *Refusal) Reason() Reason {
	if refusal == nil {
		return 0
	}
	return refusal.reason
}

// Ordinal returns the dense rule ordinal the refusal belongs to. A refusal
// that belongs to the whole seal rather than one rule reports false.
func (refusal *Refusal) Ordinal() (int, bool) {
	if refusal == nil || !refusal.ruled {
		return 0, false
	}
	return refusal.ordinal, true
}

// Error implements error while keeping the typed detail on the accessors.
func (refusal *Refusal) Error() string {
	if refusal == nil {
		return ""
	}
	if refusal.ruled {
		return fmt.Sprintf("relinput: rule %d refused: %s", refusal.ordinal, refusal.reason)
	}
	return "relinput: refused: " + refusal.reason.String()
}

func refuse(reason Reason) *Refusal { return &Refusal{reason: reason} }

func refuseRule(reason Reason, ordinal int) *Refusal {
	return &Refusal{reason: reason, ordinal: ordinal, ruled: true}
}

// fitsOrdinal reports whether a span coordinate is addressable by the frozen
// column's ordinal width. A bundle that cannot be frozen is never sealed.
func fitsOrdinal(value int) bool {
	return value >= 0 && uint64(value) <= uint64(^uint32(0))
}

// Seal publishes the composition's placements as one immutable bundle sealed
// against catalog. The table is total over the catalog: every rule ordinal
// takes a row in ordinal order, a rule that declared no execution program
// takes an explicitly absent row, and a mounted rule the composition cannot
// place refuses the seal rather than leaving a hole.
//
// Region evidence is collected in first-named order over the same traversal,
// so two bundles sealed from equal answers carry equal columns.
func Seal(catalog ruleplan.Catalog, owner model.OwnerID, composition Composition) (*Bundle, *Refusal) {
	if !catalog.Available() || !catalog.Digest().Available() {
		return nil, refuse(ReasonCatalog)
	}
	if !owner.Available() {
		return nil, refuse(ReasonOwner)
	}
	if composition == nil {
		return nil, refuse(ReasonComposition)
	}

	bundle := &Bundle{
		catalog: catalog.Digest(),
		owner:   owner,
		rows:    make([]Row, 0, catalog.Count()),
	}
	named := make(map[model.ScopeID]struct{}, catalog.Count())

	adopt := func(scope model.ScopeID, ordinal int) *Refusal {
		if !scope.Available() {
			return refuseRule(ReasonScope, ordinal)
		}
		if scope.Owner() != owner {
			return refuseRule(ReasonForeignOwner, ordinal)
		}
		if _, held := named[scope]; held {
			return nil
		}
		evidence, issued := composition.ScopeRegion(scope)
		if !issued || !evidence.Available() {
			return refuseRule(ReasonRegion, ordinal)
		}
		named[scope] = struct{}{}
		bundle.regions = append(bundle.regions, RegionRow{scope: scope, evidence: evidence})
		return nil
	}

	for ordinal := 0; ordinal < catalog.Count(); ordinal++ {
		compiled, held := catalog.At(ordinal)
		if !held {
			return nil, refuseRule(ReasonPlan, ordinal)
		}
		if !compiled.Present() {
			// The ordinal is retained as an explicit absent row. A rule that
			// declared no program has no candidate and no port to observe
			// from, so there is no placement for a composition to answer.
			bundle.rows = append(bundle.rows, Row{})
			continue
		}
		placement, placed := composition.Placement(ordinal)
		if !placed {
			return nil, refuseRule(ReasonUnplaced, ordinal)
		}
		if len(placement.Ports) != compiled.InputCount() {
			return nil, refuseRule(ReasonPortWidth, ordinal)
		}
		if refusal := adopt(placement.Candidate, ordinal); refusal != nil {
			return nil, refusal
		}
		offset := len(bundle.ports)
		if !fitsOrdinal(offset) || !fitsOrdinal(len(placement.Ports)) {
			return nil, refuseRule(ReasonPortWidth, ordinal)
		}
		for _, port := range placement.Ports {
			if refusal := adopt(port, ordinal); refusal != nil {
				return nil, refusal
			}
			bundle.ports = append(bundle.ports, PortRow{scope: port})
		}
		bundle.rows = append(bundle.rows, Row{
			present:    true,
			candidate:  placement.Candidate,
			portOffset: uint32(offset),
			portCount:  uint32(len(placement.Ports)),
		})
	}
	return bundle, nil
}
