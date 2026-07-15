package evaluated

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// RootArtifact is the ownership boundary published by the evaluated program
// transaction. It contains canonical bytes and closed structural DTOs only;
// no product node, type witness, registry, symbolic arena, or callback is
// retained. Its fields remain private so only a fully validated Root can be
// sealed into one.
type RootArtifact struct {
	identity     Identity
	requirements []operationplan.ObservationRequirement
	proof        artifactWorldProof
	points       []PointReachability
	boundaries   []artifactBoundary
	callOutcomes []artifactCallOutcomeBoundary
	edges        []EdgeReachability
	observations []artifactObservationSlot
	routes       []Route
	summary      summary.CanonicalArtifact
	coverage     Coverage
	schema       axis.SchemaIdentity
}

type artifactExpression struct {
	ID       ExpressionID
	Op       ExpressionOp
	RootKind RootKind
	Root     uint32
	Args     []ExpressionID
	Constant product.CanonicalArtifact
	Scalar   Scalar
}

type artifactWorldProof struct {
	Expressions []artifactExpression
	Predicates  []Predicate
	Decisions   []Decision
}

type artifactIndexedValue struct {
	Index uint32
	Value product.CanonicalArtifact
}

type artifactBoundaryFragment struct {
	Worlds  WorldSet
	Values  []artifactIndexedValue
	Summary summary.CanonicalArtifact
}

type artifactBoundary struct {
	Slot      uint32
	Point     cfg.Point
	Fragments []artifactBoundaryFragment
}

type artifactCallOutcomeFragment struct {
	Worlds  WorldSet
	Results []artifactIndexedValue
	Summary summary.CanonicalArtifact
	Roles   []CallOutcomeRole
}

type artifactCallOutcomeBoundary struct {
	Slot       uint32
	Point      cfg.Point
	Owner      lexicalidentity.StableLexicalBodyID
	Occurrence engineobservation.Occurrence
	Target     lexicalidentity.StableLexicalBodyID
	Fragments  []artifactCallOutcomeFragment
}

type artifactObservation struct {
	worlds      WorldSet
	owner       lexicalidentity.StableLexicalBodyID
	invocation  engineobservation.InvocationID
	kind        engineobservation.Kind
	anchor      engineobservation.Occurrence
	slot        uint32
	actual      product.CanonicalArtifact
	expected    product.CanonicalArtifact
	hasExpected bool
}

type artifactObservationSlot struct {
	Slot        uint32
	Point       cfg.Point
	Observed    []artifactObservation
	Obligations []Obligation
}

// SealRoot transactionally converts a validated transient Root into its
// ownership-isolated representation. Any cancellation or unsupported product
// returns the zero artifact.
func SealRoot(ctx context.Context, reg *axis.Registry, root Root) (RootArtifact, error) {
	if ctx == nil || reg == nil || !reg.Frozen() {
		return RootArtifact{}, fmt.Errorf("evaluated: root artifact requires context and frozen registry")
	}
	if err := ctx.Err(); err != nil {
		return RootArtifact{}, err
	}
	if !root.coverage.Complete() || !root.identity.ShadowValid() {
		return RootArtifact{}, fmt.Errorf("evaluated: incomplete root cannot be sealed")
	}
	probe, err := product.SealCanonical(ctx, reg, product.Top())
	if err != nil || !probe.Valid() {
		return RootArtifact{}, fmt.Errorf("evaluated: product schema is not materializable: %w", err)
	}
	out := RootArtifact{
		identity: root.identity, requirements: append([]operationplan.ObservationRequirement(nil), root.requirements...),
		points: clonePoints(root.points), edges: cloneEdges(root.edges), routes: cloneRoutes(root.routes),
		coverage: root.coverage, schema: probe.SchemaIdentity(),
	}
	if out.proof, err = sealArtifactWorldProof(ctx, reg, root.proof, out.schema); err != nil {
		return RootArtifact{}, err
	}
	if out.boundaries, err = sealArtifactBoundaries(ctx, reg, root.boundaries, out.schema); err != nil {
		return RootArtifact{}, err
	}
	if out.callOutcomes, err = sealArtifactCallOutcomes(ctx, reg, root.callOutcomes, out.schema); err != nil {
		return RootArtifact{}, err
	}
	if out.observations, err = sealArtifactObservations(ctx, reg, root.observations, out.schema); err != nil {
		return RootArtifact{}, err
	}
	if out.summary, err = summary.SealCanonical(ctx, reg, root.summary); err != nil {
		return RootArtifact{}, err
	}
	if err := ctx.Err(); err != nil {
		return RootArtifact{}, err
	}
	return out, nil
}

// Materialize reconstructs a fresh consumer Root under the exact registry
// schema. It publishes no partial root on cancellation or decode failure.
func (a RootArtifact) Materialize(ctx context.Context, reg *axis.Registry) (Root, error) {
	if ctx == nil || reg == nil || !reg.Frozen() || !a.coverage.Complete() || !a.identity.ShadowValid() || a.schema == (axis.SchemaIdentity{}) {
		return Root{}, fmt.Errorf("evaluated: invalid root artifact or materialization authority")
	}
	if err := ctx.Err(); err != nil {
		return Root{}, err
	}
	probe, err := product.SealCanonical(ctx, reg, product.Top())
	if err != nil || probe.SchemaIdentity() != a.schema {
		return Root{}, fmt.Errorf("evaluated: root artifact registry schema mismatch: %w", err)
	}
	parts := Parts{Identity: a.identity, Points: clonePoints(a.points), Edges: cloneEdges(a.edges), Routes: cloneRoutes(a.routes)}
	if parts.Proof, err = materializeArtifactWorldProof(ctx, reg, a.proof, a.schema); err != nil {
		return Root{}, err
	}
	if parts.Boundaries, err = materializeArtifactBoundaries(ctx, reg, a.boundaries, a.schema); err != nil {
		return Root{}, err
	}
	if parts.CallOutcomes, err = materializeArtifactCallOutcomes(ctx, reg, a.callOutcomes, a.schema); err != nil {
		return Root{}, err
	}
	if parts.Observations, err = materializeArtifactObservations(ctx, reg, a.observations, a.schema); err != nil {
		return Root{}, err
	}
	if parts.Summary, err = summary.DecodeCanonical(ctx, reg, cloneSummaryArtifact(a.summary)); err != nil {
		return Root{}, err
	}
	if err := validateWorldProof(reg, parts.Proof); err != nil {
		return Root{}, err
	}
	if err := ctx.Err(); err != nil {
		return Root{}, err
	}
	// The artifact is privately constructible only from a fully validated Root.
	// Canonical decoders and the schema fence above validate every value-bearing
	// field again; cloning here avoids retaining the temporary decode slices.
	return cloneRoot(parts, append([]operationplan.ObservationRequirement(nil), a.requirements...), a.coverage), nil
}

func sealArtifactWorldProof(ctx context.Context, reg *axis.Registry, in WorldProof, schema axis.SchemaIdentity) (artifactWorldProof, error) {
	out := artifactWorldProof{Predicates: append([]Predicate(nil), in.Predicates...), Decisions: append([]Decision(nil), in.Decisions...)}
	out.Expressions = make([]artifactExpression, len(in.Expressions))
	for index, expression := range in.Expressions {
		if err := ctx.Err(); err != nil {
			return artifactWorldProof{}, err
		}
		item := artifactExpression{ID: expression.ID, Op: expression.Op, RootKind: expression.RootKind, Root: expression.Root, Args: append([]ExpressionID(nil), expression.Args...), Scalar: expression.Scalar}
		artifact, err := sealArtifactValue(ctx, reg, expression.Constant, schema)
		if err != nil {
			return artifactWorldProof{}, err
		}
		item.Constant = artifact
		out.Expressions[index] = item
	}
	return out, nil
}

func materializeArtifactWorldProof(ctx context.Context, reg *axis.Registry, in artifactWorldProof, schema axis.SchemaIdentity) (WorldProof, error) {
	out := WorldProof{Predicates: append([]Predicate(nil), in.Predicates...), Decisions: append([]Decision(nil), in.Decisions...)}
	out.Expressions = make([]Expression, len(in.Expressions))
	for index, expression := range in.Expressions {
		item := Expression{ID: expression.ID, Op: expression.Op, RootKind: expression.RootKind, Root: expression.Root, Args: append([]ExpressionID(nil), expression.Args...), Scalar: expression.Scalar}
		value, err := materializeArtifactValue(ctx, reg, expression.Constant, schema)
		if err != nil {
			return WorldProof{}, err
		}
		item.Constant = value
		out.Expressions[index] = item
	}
	return out, nil
}

func sealArtifactBoundaries(ctx context.Context, reg *axis.Registry, in []Boundary, schema axis.SchemaIdentity) ([]artifactBoundary, error) {
	out := make([]artifactBoundary, len(in))
	for index, boundary := range in {
		out[index] = artifactBoundary{Slot: boundary.Slot, Point: boundary.Point, Fragments: make([]artifactBoundaryFragment, len(boundary.Fragments))}
		for fragmentIndex, fragment := range boundary.Fragments {
			sealed := artifactBoundaryFragment{Worlds: fragment.Worlds, Values: make([]artifactIndexedValue, len(fragment.Values))}
			var err error
			sealed.Summary, err = summary.SealCanonical(ctx, reg, fragment.Summary)
			if err != nil {
				return nil, err
			}
			for valueIndex, value := range fragment.Values {
				artifact, err := sealArtifactValue(ctx, reg, value.Value, schema)
				if err != nil {
					return nil, err
				}
				sealed.Values[valueIndex] = artifactIndexedValue{Index: value.Index, Value: artifact}
			}
			out[index].Fragments[fragmentIndex] = sealed
		}
	}
	return out, nil
}

func materializeArtifactBoundaries(ctx context.Context, reg *axis.Registry, in []artifactBoundary, schema axis.SchemaIdentity) ([]Boundary, error) {
	out := make([]Boundary, len(in))
	for index, boundary := range in {
		out[index] = Boundary{Slot: boundary.Slot, Point: boundary.Point, Fragments: make([]BoundaryFragment, len(boundary.Fragments))}
		for fragmentIndex, fragment := range boundary.Fragments {
			materialized := BoundaryFragment{Worlds: fragment.Worlds, Values: make([]IndexedValue, len(fragment.Values))}
			var err error
			materialized.Summary, err = summary.DecodeCanonical(ctx, reg, cloneSummaryArtifact(fragment.Summary))
			if err != nil {
				return nil, err
			}
			for valueIndex, value := range fragment.Values {
				decoded, err := materializeArtifactValue(ctx, reg, value.Value, schema)
				if err != nil {
					return nil, err
				}
				materialized.Values[valueIndex] = IndexedValue{Index: value.Index, Value: decoded}
			}
			out[index].Fragments[fragmentIndex] = materialized
		}
	}
	return out, nil
}

func sealArtifactCallOutcomes(ctx context.Context, reg *axis.Registry, in []CallOutcomeBoundary, schema axis.SchemaIdentity) ([]artifactCallOutcomeBoundary, error) {
	out := make([]artifactCallOutcomeBoundary, len(in))
	for index, boundary := range in {
		out[index] = artifactCallOutcomeBoundary{
			Slot: boundary.Slot, Point: boundary.Point, Owner: boundary.Owner,
			Occurrence: boundary.Occurrence, Target: boundary.Target,
			Fragments: make([]artifactCallOutcomeFragment, len(boundary.Fragments)),
		}
		for fragmentIndex, fragment := range boundary.Fragments {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			sealed := artifactCallOutcomeFragment{
				Worlds: fragment.Worlds, Results: make([]artifactIndexedValue, len(fragment.Results)),
				Roles: cloneCallOutcomeRoles(fragment.Roles),
			}
			var err error
			sealed.Summary, err = summary.SealCanonical(ctx, reg, fragment.Summary)
			if err != nil {
				return nil, err
			}
			for resultIndex, result := range fragment.Results {
				artifact, err := sealArtifactValue(ctx, reg, result.Value, schema)
				if err != nil {
					return nil, err
				}
				sealed.Results[resultIndex] = artifactIndexedValue{Index: result.Index, Value: artifact}
			}
			out[index].Fragments[fragmentIndex] = sealed
		}
	}
	return out, nil
}

func materializeArtifactCallOutcomes(ctx context.Context, reg *axis.Registry, in []artifactCallOutcomeBoundary, schema axis.SchemaIdentity) ([]CallOutcomeBoundary, error) {
	out := make([]CallOutcomeBoundary, len(in))
	for index, boundary := range in {
		out[index] = CallOutcomeBoundary{
			Slot: boundary.Slot, Point: boundary.Point, Owner: boundary.Owner,
			Occurrence: boundary.Occurrence, Target: boundary.Target,
			Fragments: make([]CallOutcomeFragment, len(boundary.Fragments)),
		}
		for fragmentIndex, fragment := range boundary.Fragments {
			materialized := CallOutcomeFragment{
				Worlds: fragment.Worlds, Results: make([]IndexedValue, len(fragment.Results)),
				Roles: cloneCallOutcomeRoles(fragment.Roles),
			}
			var err error
			materialized.Summary, err = summary.DecodeCanonical(ctx, reg, cloneSummaryArtifact(fragment.Summary))
			if err != nil {
				return nil, err
			}
			for resultIndex, result := range fragment.Results {
				value, err := materializeArtifactValue(ctx, reg, result.Value, schema)
				if err != nil {
					return nil, err
				}
				materialized.Results[resultIndex] = IndexedValue{Index: result.Index, Value: value}
			}
			out[index].Fragments[fragmentIndex] = materialized
		}
	}
	return out, nil
}

func sealArtifactObservations(ctx context.Context, reg *axis.Registry, in []ObservationSlot, schema axis.SchemaIdentity) ([]artifactObservationSlot, error) {
	out := make([]artifactObservationSlot, len(in))
	for index, slot := range in {
		out[index] = artifactObservationSlot{Slot: slot.Slot, Point: slot.Point, Observed: make([]artifactObservation, len(slot.Observed)), Obligations: append([]Obligation(nil), slot.Obligations...)}
		for itemIndex, item := range slot.Observed {
			actual, err := sealArtifactValue(ctx, reg, item.Actual, schema)
			if err != nil {
				return nil, err
			}
			sealed := artifactObservation{
				worlds: item.Worlds, owner: item.Owner, invocation: item.Invocation, kind: item.Kind,
				anchor: item.Anchor, slot: item.Slot, actual: actual, hasExpected: item.HasExpected,
			}
			if item.HasExpected {
				if sealed.expected, err = sealArtifactValue(ctx, reg, item.Expected, schema); err != nil {
					return nil, err
				}
			} else if item.Expected != (product.Value{}) {
				return nil, fmt.Errorf("evaluated: observation without expected flag retained a product")
			}
			out[index].Observed[itemIndex] = sealed
		}
	}
	return out, nil
}

func materializeArtifactObservations(ctx context.Context, reg *axis.Registry, in []artifactObservationSlot, schema axis.SchemaIdentity) ([]ObservationSlot, error) {
	out := make([]ObservationSlot, len(in))
	for index, slot := range in {
		out[index] = ObservationSlot{Slot: slot.Slot, Point: slot.Point, Observed: make([]Observation, len(slot.Observed)), Obligations: append([]Obligation(nil), slot.Obligations...)}
		for itemIndex, item := range slot.Observed {
			materialized := Observation{
				Worlds: item.worlds, Owner: item.owner, Invocation: item.invocation, Kind: item.kind,
				Anchor: item.anchor, Slot: item.slot, HasExpected: item.hasExpected,
			}
			var err error
			if materialized.Actual, err = materializeArtifactValue(ctx, reg, item.actual, schema); err != nil {
				return nil, err
			}
			if item.hasExpected {
				if materialized.Expected, err = materializeArtifactValue(ctx, reg, item.expected, schema); err != nil {
					return nil, err
				}
			}
			out[index].Observed[itemIndex] = materialized
		}
	}
	return out, nil
}

func sealArtifactValue(ctx context.Context, reg *axis.Registry, value product.Value, schema axis.SchemaIdentity) (product.CanonicalArtifact, error) {
	artifact, err := product.SealCanonical(ctx, reg, value)
	if err != nil {
		return product.CanonicalArtifact{}, err
	}
	if !artifact.Valid() || artifact.SchemaIdentity() != schema {
		return product.CanonicalArtifact{}, fmt.Errorf("evaluated: product artifact schema mismatch")
	}
	return artifact, nil
}

func materializeArtifactValue(ctx context.Context, reg *axis.Registry, artifact product.CanonicalArtifact, schema axis.SchemaIdentity) (product.Value, error) {
	if !artifact.Valid() || artifact.SchemaIdentity() != schema {
		return product.Value{}, fmt.Errorf("evaluated: product artifact schema mismatch")
	}
	return artifact.Materialize(ctx, reg)
}

func cloneSummaryArtifact(in summary.CanonicalArtifact) summary.CanonicalArtifact {
	out := in
	out.Bytes = append([]byte(nil), in.Bytes...)
	return out
}
