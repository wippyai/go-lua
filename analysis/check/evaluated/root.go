// Package evaluated owns the neutral, immutable checker projection produced
// after symbolic specialization. It contains only sealed observation products;
// solver State, symbolic terms, arenas, callbacks, and presentation metadata
// are deliberately outside this boundary.
package evaluated

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// Digest is one full-width canonical semantic identity.
type Digest [32]byte

type AuthorityStatus uint8

const (
	AuthorityInvalid AuthorityStatus = iota
	// AuthorityUnavailable is an explicit shadow-only state. It cannot authorize
	// publication, reuse, or replacement of a concrete result.
	AuthorityUnavailable
	AuthorityAvailable
)

// AuthorityDigest distinguishes a canonical full-width identity from an
// explicitly unavailable shadow authority. Compact legacy hashes must never be
// widened into Value.
type AuthorityDigest struct {
	Status AuthorityStatus
	Value  Digest
}

func (d AuthorityDigest) Valid() bool {
	switch d.Status {
	case AuthorityUnavailable:
		return d.Value == (Digest{})
	case AuthorityAvailable:
		return d.Value != (Digest{})
	default:
		return false
	}
}

func (d AuthorityDigest) Available() bool { return d.Status == AuthorityAvailable && d.Valid() }

// Identity fences a root to one body, entry, call census, graph width, and
// sealed observation consumer schema.
type Identity struct {
	Body        lexicalidentity.StableLexicalBodyID
	Relation    AuthorityDigest
	Entry       AuthorityDigest
	Lineage     AuthorityDigest
	Registry    AuthorityDigest
	CallSurface operationplan.CallSurfaceDigest
	Schema      operationplan.ObservationSchemaID
	Inventory   operationplan.ObservationConsumerInventoryID
	View        ProjectionView
	PointCount  uint32
}

// B3 deliberately keeps Registry unavailable. Before C2 may publish this
// authority, RetentionMode must participate in the canonical registry/schema
// identity; changing an axis from immutable to validated changes artifact
// admissibility even when its lattice operations are otherwise unchanged.

func (i Identity) Valid() bool {
	return i.Body != (lexicalidentity.StableLexicalBodyID{}) && i.Relation.Valid() && i.Entry.Valid() && i.Lineage.Valid() && i.Registry.Valid() &&
		i.CallSurface.Available() && i.Schema != (operationplan.ObservationSchemaID{}) &&
		i.Inventory != (operationplan.ObservationConsumerInventoryID{}) && i.View.Valid() && i.PointCount != 0
}

// Authoritative reports whether this root may eventually authorize reuse or
// replace a concrete Result. Shadow roots may be structurally valid while the
// relation/entry/lineage codecs are explicitly unavailable.
func (i Identity) Authoritative() bool {
	return i.Valid() && i.Relation.Available() && i.Entry.Available() && i.Lineage.Available() && i.Registry.Available()
}

// ShadowValid is the only identity state admitted by the current foundation.
// Canonical relation, entry, and lineage producers are not wired yet, so even
// self-consistent non-zero caller digests must be rejected as forgeable.
func (i Identity) ShadowValid() bool {
	return i.Valid() && i.Relation.Status == AuthorityUnavailable &&
		i.Entry.Status == AuthorityUnavailable && i.Lineage.Status == AuthorityUnavailable && i.Registry.Status == AuthorityUnavailable
}

type ExpressionID uint32
type ExpressionOp uint8

const (
	ExpressionInvalid ExpressionOp = iota
	ExpressionRoot
	ExpressionConstant
	ExpressionJoin
	ExpressionRefinement
	ExpressionRuntimeValidation
	ExpressionStringConcat
	ExpressionScalarEqual
	ExpressionScalarNotEqual
	ExpressionScalarAnd
	ExpressionScalarOr
	ExpressionStaticIndex
	ExpressionLuaTypeName
)

type RootKind uint8

const (
	RootInvalid RootKind = iota
	RootParam
	RootCapture
	RootGlobal
	RootResult
	RootHeapTemplate
)

// ScalarKind identifies an owned scalar literal. Scalar is deliberately a
// closed value DTO: evaluated artifacts never retain pointer-backed typ.Literal
// nodes merely to represent constants in their expression proof.
type ScalarKind uint8

const (
	ScalarInvalid ScalarKind = iota
	ScalarBoolean
	ScalarInteger
	ScalarNumber
	ScalarString
)

type Scalar struct {
	Kind    ScalarKind
	Boolean bool
	Integer int64
	Number  float64
	String  string
}

func (s Scalar) Valid() bool {
	switch s.Kind {
	case ScalarBoolean:
		return s.Integer == 0 && s.Number == 0 && s.String == ""
	case ScalarInteger:
		return !s.Boolean && s.Number == 0 && s.String == ""
	case ScalarNumber:
		return !s.Boolean && s.Integer == 0 && s.String == ""
	case ScalarString:
		return !s.Boolean && s.Integer == 0 && s.Number == 0
	default:
		return false
	}
}

func (s Scalar) IsZero() bool { return s == (Scalar{}) }

type Expression struct {
	ID       ExpressionID
	Op       ExpressionOp
	RootKind RootKind
	Root     uint32
	Args     []ExpressionID
	Constant product.Value
	Scalar   Scalar
}

type Predicate struct {
	ID    uint32
	Value ExpressionID
}

type DecisionID uint32

const (
	DecisionFalse DecisionID = iota
	DecisionTrue
)

// Decision is one reduced ordered decision node. High is the predicate's
// truthy branch and Low is its exact falsy complement.
type Decision struct {
	ID        DecisionID
	Predicate uint32
	Low       DecisionID
	High      DecisionID
}

type WorldSet struct{ Root DecisionID }

// WorldProof compactly owns the exact finite guard partition. It contains no
// transformer handles or Arena references.
type WorldProof struct {
	Expressions []Expression
	Predicates  []Predicate
	Decisions   []Decision
}

type IndexedValue struct {
	Index uint32
	Value product.Value
}

type PointReachability struct {
	Slot   uint32
	Point  cfg.Point
	Worlds WorldSet
}

type EdgeReachability struct {
	Slot   uint32
	From   cfg.Point
	To     cfg.Point
	Worlds WorldSet
}

type BoundaryFragment struct {
	Worlds  WorldSet
	Values  []IndexedValue
	Summary summary.Summary
}

type Boundary struct {
	Slot      uint32
	Point     cfg.Point
	Fragments []BoundaryFragment
}

type Observation struct {
	Worlds      WorldSet
	Owner       lexicalidentity.StableLexicalBodyID
	Invocation  observation.InvocationID
	Kind        observation.Kind
	Anchor      observation.Occurrence
	Slot        uint32
	Actual      product.Value
	Expected    product.Value
	HasExpected bool
}

type Obligation struct {
	Worlds     WorldSet
	Owner      lexicalidentity.StableLexicalBodyID
	Invocation observation.InvocationID
	Anchor     observation.Occurrence
}

type ObservationSlot struct {
	Slot        uint32
	Point       cfg.Point
	Observed    []Observation
	Obligations []Obligation
}

type Route struct {
	Slot   uint32
	Point  cfg.Point
	Anchor observation.Occurrence
	Worlds WorldSet
}

type Coverage struct {
	Required     uint32
	Points       uint32
	Boundaries   uint32
	Edges        uint32
	Observations uint32
	Routes       uint32
}

func (c Coverage) Complete() bool {
	return c.Required != 0 && c.Required == c.Points+c.Boundaries+c.Edges+c.Observations+c.Routes
}

// Parts is transient constructor input. NewRoot validates and ownership-copies
// every slice before publishing an immutable Root.
type Parts struct {
	Identity     Identity
	Proof        WorldProof
	Points       []PointReachability
	Boundaries   []Boundary
	Edges        []EdgeReachability
	Observations []ObservationSlot
	Routes       []Route
	Summary      summary.Summary
}

// Root is an immutable specialized observation snapshot. Its typed slices are
// intentionally closed over requirement stages; there is no generic product
// map and no whole-State escape hatch.
type Root struct {
	identity     Identity
	requirements []operationplan.ObservationRequirement
	proof        WorldProof
	points       []PointReachability
	boundaries   []Boundary
	edges        []EdgeReachability
	observations []ObservationSlot
	routes       []Route
	summary      summary.Summary
	coverage     Coverage
}

func NewShadowRoot(ctx context.Context, reg *axis.Registry, requirements operationplan.ObservationRequirements, callOutcome bool, parts Parts) (Root, error) {
	if ctx == nil || reg == nil || !reg.Frozen() {
		return Root{}, fmt.Errorf("evaluated: shadow root requires context and frozen registry")
	}
	if err := ctx.Err(); err != nil {
		return Root{}, err
	}
	if !parts.Identity.ShadowValid() {
		return Root{}, fmt.Errorf("evaluated: incomplete or forgeable shadow root identity")
	}
	view, err := SealProjectionView(requirements, callOutcome)
	if err != nil || parts.Identity.Schema != requirements.SchemaID() || parts.Identity.Inventory != requirements.ConsumerInventoryID() || parts.Identity.View != view {
		return Root{}, fmt.Errorf("evaluated: sealed requirement/view identity mismatch")
	}
	entries := requirements.Entries(callOutcome)
	if len(entries) == 0 {
		return Root{}, fmt.Errorf("evaluated: empty requirement view")
	}
	if err := validateWorldProof(reg, parts.Proof); err != nil {
		return Root{}, err
	}
	parts.Summary, err = normalizeArtifactSafeSummary(ctx, reg, parts.Summary)
	if err != nil {
		return Root{}, err
	}
	for boundaryIndex := range parts.Boundaries {
		if boundaryIndex&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Root{}, err
			}
		}
		for fragmentIndex := range parts.Boundaries[boundaryIndex].Fragments {
			if fragmentIndex&63 == 0 {
				if err := ctx.Err(); err != nil {
					return Root{}, err
				}
			}
			fragment := &parts.Boundaries[boundaryIndex].Fragments[fragmentIndex]
			fragment.Summary, err = normalizeArtifactSafeSummary(ctx, reg, fragment.Summary)
			if err != nil {
				return Root{}, err
			}
			for valueIndex, value := range fragment.Values {
				if valueIndex&63 == 0 {
					if err := ctx.Err(); err != nil {
						return Root{}, err
					}
				}
				if !artifactSafeValue(reg, value.Value) {
					return Root{}, fmt.Errorf("evaluated: boundary value is not artifact-safe")
				}
			}
		}
	}
	seen := make([]bool, len(entries))
	coverage := Coverage{Required: uint32(len(entries))}
	claim := func(slot uint32, stage operationplan.RequirementStage) error {
		if int(slot) >= len(entries) || seen[slot] {
			return fmt.Errorf("evaluated: missing, duplicate, or foreign slot %d", slot)
		}
		if entries[slot].Stage() != stage {
			return fmt.Errorf("evaluated: slot %d has wrong typed stage", slot)
		}
		seen[slot] = true
		return nil
	}
	validWorlds := func(worlds WorldSet) bool { return validWorldSet(parts.Proof, worlds) }
	for index, point := range parts.Points {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Root{}, err
			}
		}
		if index != 0 && parts.Points[index-1].Slot >= point.Slot || claim(point.Slot, operationplan.RequirementPoint) != nil ||
			entries[point.Slot].Point() != point.Point || !validWorlds(point.Worlds) {
			return Root{}, fmt.Errorf("evaluated: invalid point slot %d", point.Slot)
		}
		coverage.Points++
	}
	for index, boundary := range parts.Boundaries {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Root{}, err
			}
		}
		if index != 0 && parts.Boundaries[index-1].Slot >= boundary.Slot || claim(boundary.Slot, operationplan.RequirementBoundary) != nil ||
			entries[boundary.Slot].Point() != boundary.Point {
			return Root{}, fmt.Errorf("evaluated: invalid boundary slot %d", boundary.Slot)
		}
		for fragmentIndex, fragment := range boundary.Fragments {
			if fragmentIndex&63 == 0 {
				if err := ctx.Err(); err != nil {
					return Root{}, err
				}
			}
			if !validWorlds(fragment.Worlds) {
				return Root{}, fmt.Errorf("evaluated: invalid boundary world at slot %d", boundary.Slot)
			}
			for valueIndex, value := range fragment.Values {
				if valueIndex != 0 && fragment.Values[valueIndex-1].Index >= value.Index {
					return Root{}, fmt.Errorf("evaluated: duplicate or unordered boundary value at slot %d", boundary.Slot)
				}
			}
		}
		coverage.Boundaries++
	}
	for index, edge := range parts.Edges {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Root{}, err
			}
		}
		to, hasTo := cfg.Point(0), false
		if int(edge.Slot) < len(entries) {
			to, hasTo = entries[edge.Slot].EdgeTarget()
		}
		if index != 0 && parts.Edges[index-1].Slot >= edge.Slot || claim(edge.Slot, operationplan.RequirementEdge) != nil ||
			entries[edge.Slot].Point() != edge.From || !hasTo || to != edge.To || !validWorlds(edge.Worlds) {
			return Root{}, fmt.Errorf("evaluated: invalid edge slot %d", edge.Slot)
		}
		coverage.Edges++
	}
	for index, observed := range parts.Observations {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Root{}, err
			}
		}
		anchor, hasAnchor := observation.Occurrence{}, false
		kind, hasKind := observation.Invalid, false
		if int(observed.Slot) < len(entries) {
			anchor, hasAnchor = entries[observed.Slot].Anchor()
			kind, hasKind = entries[observed.Slot].ObservationKind()
		}
		if index != 0 && parts.Observations[index-1].Slot >= observed.Slot || claim(observed.Slot, operationplan.RequirementObservation) != nil ||
			entries[observed.Slot].Point() != observed.Point || !hasAnchor || !hasKind {
			return Root{}, fmt.Errorf("evaluated: invalid observation slot %d", observed.Slot)
		}
		for itemIndex, item := range observed.Observed {
			if itemIndex&63 == 0 {
				if err := ctx.Err(); err != nil {
					return Root{}, err
				}
			}
			if !validWorlds(item.Worlds) || item.Owner == (lexicalidentity.StableLexicalBodyID{}) ||
				!item.Anchor.Valid() || item.Anchor != anchor || item.Kind != kind || item.Anchor.Kind != item.Kind || item.Anchor.Slot != item.Slot ||
				!artifactSafeValue(reg, item.Actual) || item.HasExpected && !artifactSafeValue(reg, item.Expected) {
				return Root{}, fmt.Errorf("evaluated: invalid observation at slot %d", observed.Slot)
			}
		}
		for itemIndex, item := range observed.Obligations {
			if itemIndex&63 == 0 {
				if err := ctx.Err(); err != nil {
					return Root{}, err
				}
			}
			if !validWorlds(item.Worlds) || item.Owner == (lexicalidentity.StableLexicalBodyID{}) || item.Anchor != anchor {
				return Root{}, fmt.Errorf("evaluated: invalid obligation at slot %d", observed.Slot)
			}
		}
		coverage.Observations++
	}
	for index, route := range parts.Routes {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Root{}, err
			}
		}
		anchor, hasAnchor := observation.Occurrence{}, false
		if int(route.Slot) < len(entries) {
			anchor, hasAnchor = entries[route.Slot].Anchor()
		}
		if index != 0 && parts.Routes[index-1].Slot >= route.Slot || claim(route.Slot, operationplan.RequirementRoute) != nil ||
			entries[route.Slot].Point() != route.Point || !hasAnchor || route.Anchor != anchor || !validWorlds(route.Worlds) {
			return Root{}, fmt.Errorf("evaluated: invalid route slot %d", route.Slot)
		}
		coverage.Routes++
	}
	if !coverage.Complete() {
		return Root{}, fmt.Errorf("evaluated: incomplete typed coverage %d/%d", coverage.Points+coverage.Boundaries+coverage.Edges+coverage.Observations+coverage.Routes, coverage.Required)
	}
	for slot, present := range seen {
		if !present {
			return Root{}, fmt.Errorf("evaluated: requirement slot %d has no typed result", slot)
		}
	}
	if err := ctx.Err(); err != nil {
		return Root{}, err
	}
	return cloneRoot(parts, entries, coverage), nil
}

func cloneRoot(parts Parts, requirements []operationplan.ObservationRequirement, coverage Coverage) Root {
	out := Root{
		identity: parts.Identity, requirements: append([]operationplan.ObservationRequirement(nil), requirements...),
		proof: cloneWorldProof(parts.Proof), summary: parts.Summary.Clone(), coverage: coverage,
	}
	out.points = clonePoints(parts.Points)
	out.boundaries = cloneBoundaries(parts.Boundaries)
	out.edges = cloneEdges(parts.Edges)
	out.observations = cloneObservations(parts.Observations)
	out.routes = cloneRoutes(parts.Routes)
	return out
}

func clonePoints(in []PointReachability) []PointReachability {
	return append([]PointReachability(nil), in...)
}

func cloneBoundaries(in []Boundary) []Boundary {
	out := make([]Boundary, len(in))
	for index, boundary := range in {
		out[index] = Boundary{Slot: boundary.Slot, Point: boundary.Point, Fragments: make([]BoundaryFragment, len(boundary.Fragments))}
		for fragmentIndex, fragment := range boundary.Fragments {
			out[index].Fragments[fragmentIndex] = BoundaryFragment{
				Worlds: fragment.Worlds, Values: append([]IndexedValue(nil), fragment.Values...), Summary: fragment.Summary.Clone(),
			}
		}
	}
	return out
}

func cloneEdges(in []EdgeReachability) []EdgeReachability {
	return append([]EdgeReachability(nil), in...)
}

func cloneObservations(in []ObservationSlot) []ObservationSlot {
	out := make([]ObservationSlot, len(in))
	for index, slot := range in {
		out[index] = slot
		out[index].Observed = append([]Observation(nil), slot.Observed...)
		out[index].Obligations = append([]Obligation(nil), slot.Obligations...)
	}
	return out
}

func cloneRoutes(in []Route) []Route {
	return append([]Route(nil), in...)
}

func (r Root) Identity() Identity { return r.identity }
func (r Root) Coverage() Coverage { return r.coverage }

// Authoritative is false for shadow roots carrying any explicitly unavailable
// semantic identity. Consumers must fail closed rather than treating them as
// cache or publication authority.
func (r Root) Authoritative() bool { return r.identity.Authoritative() }
func (r Root) Requirements() []operationplan.ObservationRequirement {
	return append([]operationplan.ObservationRequirement(nil), r.requirements...)
}
func (r Root) Proof() WorldProof               { return cloneWorldProof(r.proof) }
func (r Root) Points() []PointReachability     { return clonePoints(r.points) }
func (r Root) Boundaries() []Boundary          { return cloneBoundaries(r.boundaries) }
func (r Root) Edges() []EdgeReachability       { return cloneEdges(r.edges) }
func (r Root) Observations() []ObservationSlot { return cloneObservations(r.observations) }
func (r Root) Routes() []Route                 { return cloneRoutes(r.routes) }
func (r Root) Summary() summary.Summary        { return r.summary.Clone() }
