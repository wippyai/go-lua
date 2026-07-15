package transformer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const canonicalTransformerIdentityVersion uint64 = 2

var ErrNonportableCanonicalRelation = errors.New("transformer: relation is not canonically portable")

// NonportableCanonicalRelationError names a relation feature outside the exact
// evaluated-root subset. It is a cache/admission failure only; callers retain
// the ordinary contextual solver and must not erase the feature.
type NonportableCanonicalRelationError struct{ Feature string }

func (e *NonportableCanonicalRelationError) Error() string {
	if e == nil || e.Feature == "" {
		return ErrNonportableCanonicalRelation.Error()
	}
	return fmt.Sprintf("%s: %s", ErrNonportableCanonicalRelation, e.Feature)
}
func (*NonportableCanonicalRelationError) Unwrap() error { return ErrNonportableCanonicalRelation }

// EvaluatedRootAuthority is one internally-derived authority tuple. Its fields
// are private: consumers cannot widen compact hashes or caller bytes into an
// evaluated-root identity. Copies are immutable values.
type EvaluatedRootAuthority struct {
	relation evaluated.AuthorityDigest
	entry    evaluated.AuthorityDigest
	lineage  evaluated.AuthorityDigest
	registry evaluated.AuthorityDigest
}

func (a EvaluatedRootAuthority) Valid() bool {
	return a.relation.Available() && a.entry.Available() && a.lineage.Available() && a.registry.Available()
}

func (a EvaluatedRootAuthority) RelationIdentity() evaluated.AuthorityDigest { return a.relation }
func (a EvaluatedRootAuthority) EntryIdentity() evaluated.AuthorityDigest    { return a.entry }
func (a EvaluatedRootAuthority) LineageIdentity() evaluated.AuthorityDigest  { return a.lineage }
func (a EvaluatedRootAuthority) RegistryIdentity() evaluated.AuthorityDigest { return a.registry }

// DeriveEvaluatedRootAuthority transactionally derives every canonical fence
// from owned semantic inputs. Zero authority is returned on cancellation,
// nonportable products/summaries/terms, foreign registries, or malformed
// dependency authorities. Dependencies are an unordered set: duplicate roots
// do not change lineage; occurrence and edge identity is fenced separately by
// the relation and sealed call surface.
func (r Relation) DeriveEvaluatedRootAuthority(ctx context.Context, cursor BindingCursor, dependencies []EvaluatedRootAuthority) (EvaluatedRootAuthority, error) {
	if ctx == nil {
		return EvaluatedRootAuthority{}, fmt.Errorf("transformer: canonical authority requires a context")
	}
	if err := ctx.Err(); err != nil {
		return EvaluatedRootAuthority{}, err
	}
	if r.arena == nil || r.arena.reg == nil || cursor.shape != r.shape ||
		len(cursor.values) != cursor.shape.ValueCount() ||
		(cursor.paths != nil && len(cursor.paths) != cursor.shape.ValueCount()) {
		return EvaluatedRootAuthority{}, &NonportableCanonicalRelationError{Feature: "missing registry or entry shape mismatch"}
	}
	registry, err := canonicalRegistryAuthority(r.arena.reg)
	if err != nil {
		return EvaluatedRootAuthority{}, err
	}
	relation, err := r.canonicalRelationIdentity(ctx, registry)
	if err != nil {
		return EvaluatedRootAuthority{}, err
	}
	entry, err := canonicalEntryIdentity(ctx, r.arena.reg, registry, cursor)
	if err != nil {
		return EvaluatedRootAuthority{}, err
	}
	lineage, err := canonicalLineageIdentity(ctx, registry, dependencies)
	if err != nil {
		return EvaluatedRootAuthority{}, err
	}
	if err := ctx.Err(); err != nil {
		return EvaluatedRootAuthority{}, err
	}
	out := EvaluatedRootAuthority{relation: relation, entry: entry, lineage: lineage, registry: registry}
	if !out.Valid() {
		return EvaluatedRootAuthority{}, fmt.Errorf("transformer: canonical authority derivation produced an invalid digest")
	}
	return out, nil
}

func canonicalRegistryAuthority(reg *axis.Registry) (evaluated.AuthorityDigest, error) {
	plan, err := reg.CanonicalPlan()
	if err != nil {
		return evaluated.AuthorityDigest{}, fmt.Errorf("transformer: canonical registry plan: %w", err)
	}
	identity, ok := plan.AuthorityIdentity()
	if !ok {
		return evaluated.AuthorityDigest{}, fmt.Errorf("transformer: canonical registry authority unavailable")
	}
	return availableAuthority(identity[:])
}

func (r Relation) canonicalRelationIdentity(ctx context.Context, registry evaluated.AuthorityDigest) (evaluated.AuthorityDigest, error) {
	if r.contextual != "" || !r.observationComplete || r.projectionTrace == nil || r.projectionTraceReason != "" {
		return evaluated.AuthorityDigest{}, &NonportableCanonicalRelationError{Feature: "relation is contextual or projection-incomplete"}
	}
	if !r.descriptors.validEvaluatedRootSchema(r.arena.reg) {
		return evaluated.AuthorityDigest{}, &NonportableCanonicalRelationError{Feature: "descriptor schema is not sealed"}
	}
	codec := newRelationCanonicalCodec(ctx, r)
	return digestCanonical(ctx, "analysis.transformer.relation", func(w *canonical.Writer) error {
		if err := w.Bytes(registry.Value[:]); err != nil {
			return err
		}
		if err := codec.writeShape(w, r.shape); err != nil {
			return err
		}
		for _, flag := range []bool{r.inferReturnCorrelations, r.widened, r.observationComplete} {
			if err := w.Bool(flag); err != nil {
				return err
			}
		}
		if err := codec.writeOutputAuthority(w, r.authority); err != nil {
			return err
		}
		if err := codec.writeDescriptorRegistry(w, r.descriptors); err != nil {
			return err
		}
		if err := codec.writeProducts(w, r.paramContracts); err != nil {
			return err
		}
		if err := codec.writeProjection(w, r.projection); err != nil {
			return err
		}
		if err := codec.writeRelationAnnotations(w, r.annotations); err != nil {
			return err
		}
		rows := make([][]byte, len(r.rows))
		for i := range r.rows {
			var err error
			rows[i], err = codec.rowBytes(r.rows[i])
			if err != nil {
				return err
			}
		}
		sortByteSlices(rows)
		if err := writeByteSlices(w, rows); err != nil {
			return err
		}
		return codec.writeProjectionTrace(w, r.projectionTrace)
	})
}

func canonicalEntryIdentity(ctx context.Context, reg *axis.Registry, registry evaluated.AuthorityDigest, cursor BindingCursor) (evaluated.AuthorityDigest, error) {
	return digestCanonical(ctx, "analysis.transformer.entry", func(w *canonical.Writer) error {
		if err := w.Bytes(registry.Value[:]); err != nil {
			return err
		}
		codec := relationCanonicalCodec{ctx: ctx, relation: Relation{arena: &Arena{reg: reg}}}
		if err := codec.writeShape(w, cursor.shape); err != nil {
			return err
		}
		if err := codec.writeProducts(w, cursor.values); err != nil {
			return err
		}
		if err := w.Bool(cursor.paths != nil); err != nil {
			return err
		}
		if cursor.paths == nil {
			return nil
		}
		if err := w.Count(uint64(len(cursor.paths))); err != nil {
			return err
		}
		for _, path := range cursor.paths {
			if err := writeConcretePath(w, path); err != nil {
				return err
			}
		}
		return nil
	})
}

// canonicalLineageIdentity treats dependencies as a mathematical set of
// authoritative roots. Occurrence and edge labels belong to the relation/call
// surface identity; they are intentionally not part of this invalidation set.
func canonicalLineageIdentity(ctx context.Context, registry evaluated.AuthorityDigest, dependencies []EvaluatedRootAuthority) (evaluated.AuthorityDigest, error) {
	encoded := make([][]byte, len(dependencies))
	for i, dependency := range dependencies {
		if !dependency.Valid() || dependency.registry != registry {
			return evaluated.AuthorityDigest{}, fmt.Errorf("transformer: dependency lineage contains invalid or foreign authority")
		}
		bytes, err := canonicalBytes(ctx, "analysis.transformer.dependency", func(w *canonical.Writer) error {
			for _, identity := range []evaluated.AuthorityDigest{dependency.relation, dependency.entry, dependency.lineage, dependency.registry} {
				if err := w.Bytes(identity.Value[:]); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return evaluated.AuthorityDigest{}, err
		}
		encoded[i] = bytes
	}
	sortByteSlices(encoded)
	encoded = compactByteSlices(encoded)
	return digestCanonical(ctx, "analysis.transformer.lineage", func(w *canonical.Writer) error {
		if err := w.Bytes(registry.Value[:]); err != nil {
			return err
		}
		return writeByteSlices(w, encoded)
	})
}

type relationCanonicalCodec struct {
	ctx        context.Context
	relation   Relation
	values     map[ValueTerm][]byte
	paths      map[PathTerm][]byte
	guards     map[Guard][]byte
	valueState map[ValueTerm]canonicalVisitState
	guardState map[Guard]canonicalVisitState
}

type canonicalVisitState uint8

const (
	canonicalUnseen canonicalVisitState = iota
	canonicalVisiting
	canonicalDone
)

func newRelationCanonicalCodec(ctx context.Context, relation Relation) relationCanonicalCodec {
	return relationCanonicalCodec{
		ctx: ctx, relation: relation,
		values: make(map[ValueTerm][]byte), paths: make(map[PathTerm][]byte), guards: make(map[Guard][]byte),
		valueState: make(map[ValueTerm]canonicalVisitState), guardState: make(map[Guard]canonicalVisitState),
	}
}

const (
	recordShape uint64 = iota + 1
	recordValue
	recordPath
	recordGuard
	recordRow
	recordOperation
	recordProof
	recordRefinement
	recordObservation
	recordObligation
	recordObserverCall
	recordTraceSlot
	recordTraceFragment
	recordTraceValue
)

func (c *relationCanonicalCodec) writeShape(w *canonical.Writer, shape Shape) error {
	if err := w.Record(recordShape); err != nil {
		return err
	}
	for _, width := range []uint32{shape.Params, shape.Captures, shape.Globals, shape.Results, shape.HeapTemplates} {
		if err := w.Uint(uint64(width)); err != nil {
			return err
		}
	}
	return nil
}

func (c *relationCanonicalCodec) writeProducts(w *canonical.Writer, values []product.Value) error {
	if err := w.Count(uint64(len(values))); err != nil {
		return err
	}
	for _, value := range values {
		encoded, schema, err := product.EncodeCanonical(c.ctx, c.relation.arena.reg, value)
		if err != nil {
			return err
		}
		if err := w.Bytes(schema[:]); err != nil {
			return err
		}
		if err := w.Bytes(encoded); err != nil {
			return err
		}
	}
	return nil
}

func (c *relationCanonicalCodec) writeOutputAuthority(w *canonical.Writer, authority *relationOutputAuthority) error {
	if err := w.Bool(authority != nil); err != nil || authority == nil {
		return err
	}
	summaryKinds := make([]string, 0, len(authority.summary))
	for kind := range authority.summary {
		summaryKinds = append(summaryKinds, string(kind))
	}
	sort.Strings(summaryKinds)
	if err := w.Count(uint64(len(summaryKinds))); err != nil {
		return err
	}
	for _, kind := range summaryKinds {
		if err := w.String(kind); err != nil {
			return err
		}
	}
	effectKinds := make([]int, 0, len(authority.effects))
	for kind := range authority.effects {
		effectKinds = append(effectKinds, int(kind))
	}
	sort.Ints(effectKinds)
	if err := w.Count(uint64(len(effectKinds))); err != nil {
		return err
	}
	for _, raw := range effectKinds {
		if err := w.Uint(uint64(raw)); err != nil {
			return err
		}
		allowed := make([]string, 0, len(authority.effects[EffectKind(raw)]))
		for kind := range authority.effects[EffectKind(raw)] {
			allowed = append(allowed, string(kind))
		}
		sort.Strings(allowed)
		if err := w.Count(uint64(len(allowed))); err != nil {
			return err
		}
		for _, kind := range allowed {
			if err := w.String(kind); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *relationCanonicalCodec) writeDescriptorRegistry(w *canonical.Writer, descriptors *DescriptorRegistry) error {
	returns, ok := descriptors.handlers[DescriptorReturn].(returnHandler)
	if !ok {
		return &NonportableCanonicalRelationError{Feature: "noncanonical return descriptor"}
	}
	if _, ok := descriptors.handlers[DescriptorObligation].(obligationHandler); !ok {
		return &NonportableCanonicalRelationError{Feature: "noncanonical obligation descriptor"}
	}
	return c.writeProducts(w, returns.declared)
}

func (c *relationCanonicalCodec) writeProjection(w *canonical.Writer, projection relationProjection) error {
	if err := w.Count(uint64(len(projection.returnParamPathAliases))); err != nil {
		return err
	}
	for _, alias := range projection.returnParamPathAliases {
		if err := w.Int(int64(alias.ReturnIndex)); err != nil {
			return err
		}
		if err := w.String(string(alias.Member)); err != nil {
			return err
		}
		if err := w.String(string(alias.Source)); err != nil {
			return err
		}
	}
	return nil
}

func (c *relationCanonicalCodec) writeRelationAnnotations(w *canonical.Writer, annotations relationAnnotations) error {
	observations := make([][]byte, len(annotations.observations))
	for i := range annotations.observations {
		var err error
		observations[i], err = c.observationBytes(annotations.observations[i])
		if err != nil {
			return err
		}
	}
	sortByteSlices(observations)
	if err := writeByteSlices(w, observations); err != nil {
		return err
	}
	obligations := make([][]byte, len(annotations.obligations))
	for i := range annotations.obligations {
		var err error
		obligations[i], err = c.obligationBytes(annotations.obligations[i])
		if err != nil {
			return err
		}
	}
	sortByteSlices(obligations)
	if err := writeByteSlices(w, obligations); err != nil {
		return err
	}
	calls := make([][]byte, len(annotations.calls))
	for i := range annotations.calls {
		var err error
		calls[i], err = c.observerCallBytes(annotations.calls[i])
		if err != nil {
			return err
		}
	}
	sortByteSlices(calls)
	return writeByteSlices(w, calls)
}

func (c *relationCanonicalCodec) observerCallBytes(call ObserverCallTemplate) ([]byte, error) {
	if !call.valid(c.relation.arena, c.relation.shape) {
		return nil, &NonportableCanonicalRelationError{Feature: "malformed observer call template"}
	}
	return canonicalBytes(c.ctx, "analysis.transformer.observer-call", func(w *canonical.Writer) error {
		if err := w.Record(recordObserverCall); err != nil {
			return err
		}
		if err := w.Bytes(call.owner[:]); err != nil {
			return err
		}
		if err := writeObserverCallOccurrence(w, call.occurrence); err != nil {
			return err
		}
		if err := w.Uint(uint64(call.point)); err != nil {
			return err
		}
		if err := w.Uint(call.target.Cell.Function); err != nil {
			return err
		}
		if err := w.Uint(uint64(call.target.Cell.Slot)); err != nil {
			return err
		}
		if err := c.writeShape(w, call.target.Shape); err != nil {
			return err
		}
		guard, err := c.guardBytes(call.guard)
		if err != nil {
			return err
		}
		if err := w.Bytes(guard); err != nil {
			return err
		}
		if err := w.Count(uint64(len(call.values))); err != nil {
			return err
		}
		for i, value := range call.values {
			encoded, err := c.valueBytes(value)
			if err != nil {
				return err
			}
			if err := w.Bytes(encoded); err != nil {
				return err
			}
			if err := w.Bool(call.paths[i] != 0); err != nil {
				return err
			}
			if call.paths[i] != 0 {
				encodedPath, err := c.pathBytes(call.paths[i])
				if err != nil {
					return err
				}
				if err := w.Bytes(encodedPath); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func writeObserverCallOccurrence(w *canonical.Writer, occurrence observation.Occurrence) error {
	if err := w.Uint(uint64(occurrence.Point.Ordinal)); err != nil {
		return err
	}
	if err := w.Uint(uint64(occurrence.Point.Phase)); err != nil {
		return err
	}
	if err := w.Uint(uint64(occurrence.Kind)); err != nil {
		return err
	}
	return w.Uint(uint64(occurrence.Slot))
}

func (c *relationCanonicalCodec) rowBytes(row Row) ([]byte, error) {
	return canonicalBytes(c.ctx, "analysis.transformer.row", func(w *canonical.Writer) error {
		if err := w.Record(recordRow); err != nil {
			return err
		}
		guard, err := c.guardBytes(row.Guard)
		if err != nil {
			return err
		}
		if err := w.Bytes(guard); err != nil {
			return err
		}
		artifact, err := summary.EncodeCanonical(c.ctx, c.relation.arena.reg, row.Output)
		if err != nil {
			return err
		}
		if err := w.Bytes(artifact.Schema[:]); err != nil {
			return err
		}
		if err := w.Bytes(artifact.Semantic[:]); err != nil {
			return err
		}
		if err := w.Bytes(artifact.Bytes); err != nil {
			return err
		}
		if err := c.writeOperations(w, row.Ops); err != nil {
			return err
		}
		if len(row.Effects) != 0 {
			return &NonportableCanonicalRelationError{Feature: "effect terms"}
		}
		if err := w.Count(uint64(len(row.Proofs))); err != nil {
			return err
		}
		for _, proof := range row.Proofs {
			if err := c.writeProof(w, proof); err != nil {
				return err
			}
		}
		if err := w.Count(uint64(len(row.PathRefinements))); err != nil {
			return err
		}
		for _, refinement := range row.PathRefinements {
			if err := c.writeRefinement(w, refinement); err != nil {
				return err
			}
		}
		observations := make([][]byte, len(row.Observations))
		for i := range row.Observations {
			observations[i], err = c.observationBytes(row.Observations[i])
			if err != nil {
				return err
			}
		}
		sortByteSlices(observations)
		if err := writeByteSlices(w, observations); err != nil {
			return err
		}
		obligations := make([][]byte, len(row.observationObligations))
		for i := range row.observationObligations {
			obligations[i], err = c.obligationBytes(row.observationObligations[i])
			if err != nil {
				return err
			}
		}
		sortByteSlices(obligations)
		return writeByteSlices(w, obligations)
	})
}

func (c *relationCanonicalCodec) writeOperations(w *canonical.Writer, operations []Operation) error {
	encoded := make([][]byte, len(operations))
	for i, operation := range operations {
		var err error
		encoded[i], err = canonicalBytes(c.ctx, "analysis.transformer.operation", func(nested *canonical.Writer) error {
			if err := nested.Record(recordOperation); err != nil {
				return err
			}
			if err := nested.Uint(uint64(operation.Kind)); err != nil {
				return err
			}
			if err := nested.String(string(operation.Descriptor)); err != nil {
				return err
			}
			if err := nested.Uint(uint64(operation.Slot)); err != nil {
				return err
			}
			value, err := c.valueBytes(operation.Value)
			if err != nil {
				return err
			}
			return nested.Bytes(value)
		})
		if err != nil {
			return err
		}
	}
	sortByteSlices(encoded)
	return writeByteSlices(w, encoded)
}

func (c *relationCanonicalCodec) writeProof(w *canonical.Writer, proof BranchProofTerm) error {
	if err := w.Record(recordProof); err != nil {
		return err
	}
	if err := w.Uint(uint64(proof.Kind)); err != nil {
		return err
	}
	path, err := c.pathBytes(proof.Table)
	if err != nil {
		return err
	}
	if err := w.Bytes(path); err != nil {
		return err
	}
	if err := w.Bool(proof.Key != 0); err != nil {
		return err
	}
	if proof.Key != 0 {
		value, err := c.valueBytes(proof.Key)
		if err != nil {
			return err
		}
		if err := w.Bytes(value); err != nil {
			return err
		}
	}
	return w.Uint(uint64(proof.Presence))
}

func (c *relationCanonicalCodec) writeRefinement(w *canonical.Writer, refinement PathRefinementTerm) error {
	if err := w.Record(recordRefinement); err != nil {
		return err
	}
	path, err := c.pathBytes(refinement.Path)
	if err != nil {
		return err
	}
	value, err := c.valueBytes(refinement.Value)
	if err != nil {
		return err
	}
	if err := w.Bytes(path); err != nil {
		return err
	}
	return w.Bytes(value)
}

func (c *relationCanonicalCodec) observationBytes(item ObservationTerm) ([]byte, error) {
	return canonicalBytes(c.ctx, "analysis.transformer.observation", func(w *canonical.Writer) error {
		if err := w.Record(recordObservation); err != nil {
			return err
		}
		if err := writeObservationIdentity(w, item.BodyOwner[:], item.Route[:], item.Anchor); err != nil {
			return err
		}
		if err := w.Uint(uint64(item.Kind)); err != nil {
			return err
		}
		if err := w.Uint(uint64(item.Slot)); err != nil {
			return err
		}
		guard, err := c.guardBytes(item.Guard)
		if err != nil {
			return err
		}
		actual, err := c.valueBytes(item.Actual)
		if err != nil {
			return err
		}
		if err := w.Bytes(guard); err != nil {
			return err
		}
		if err := w.Bytes(actual); err != nil {
			return err
		}
		if err := w.Bool(item.Expected != 0); err != nil {
			return err
		}
		if item.Expected == 0 {
			return nil
		}
		expected, err := c.valueBytes(item.Expected)
		if err != nil {
			return err
		}
		return w.Bytes(expected)
	})
}

func (c *relationCanonicalCodec) obligationBytes(item observationObligation) ([]byte, error) {
	return canonicalBytes(c.ctx, "analysis.transformer.obligation", func(w *canonical.Writer) error {
		if err := w.Record(recordObligation); err != nil {
			return err
		}
		if err := writeObservationIdentity(w, item.BodyOwner[:], item.Route[:], item.Anchor); err != nil {
			return err
		}
		guard, err := c.guardBytes(item.Guard)
		if err != nil {
			return err
		}
		return w.Bytes(guard)
	})
}

func (c *relationCanonicalCodec) writeProjectionTrace(w *canonical.Writer, trace *sparseProjectionTrace) error {
	if trace == nil {
		return &NonportableCanonicalRelationError{Feature: "missing projection trace"}
	}
	if err := w.Bytes(trace.schema[:]); err != nil {
		return err
	}
	if err := w.Bytes(trace.inventory[:]); err != nil {
		return err
	}
	if err := w.Bytes(trace.owner[:]); err != nil {
		return err
	}
	if err := w.Count(uint64(len(trace.slots))); err != nil {
		return err
	}
	for _, slot := range trace.slots {
		if err := w.Record(recordTraceSlot); err != nil {
			return err
		}
		if err := writeRequirement(w, slot.requirement); err != nil {
			return err
		}
		guard, err := c.guardBytes(slot.guard)
		if err != nil {
			return err
		}
		if err := w.Bytes(guard); err != nil {
			return err
		}
		fragments := make([][]byte, len(slot.fragments))
		for i := range slot.fragments {
			var err error
			fragments[i], err = c.projectionFragmentBytes(slot.fragments[i])
			if err != nil {
				return err
			}
		}
		sortByteSlices(fragments)
		if err := writeByteSlices(w, fragments); err != nil {
			return err
		}
		observed := make([][]byte, len(slot.observed))
		for i := range slot.observed {
			observed[i], err = c.observationBytes(slot.observed[i])
			if err != nil {
				return err
			}
		}
		sortByteSlices(observed)
		if err := writeByteSlices(w, observed); err != nil {
			return err
		}
		owed := make([][]byte, len(slot.owed))
		for i := range slot.owed {
			owed[i], err = c.obligationBytes(slot.owed[i])
			if err != nil {
				return err
			}
		}
		sortByteSlices(owed)
		if err := writeByteSlices(w, owed); err != nil {
			return err
		}
	}
	return nil
}

func (c *relationCanonicalCodec) projectionFragmentBytes(fragment sparseProjectionFragment) ([]byte, error) {
	return canonicalBytes(c.ctx, "analysis.transformer.projection-fragment", func(w *canonical.Writer) error {
		if err := w.Record(recordTraceFragment); err != nil {
			return err
		}
		fragmentGuard, err := c.guardBytes(fragment.guard)
		if err != nil {
			return err
		}
		if err := w.Bytes(fragmentGuard); err != nil {
			return err
		}
		values := make([][]byte, len(fragment.values))
		for i, item := range fragment.values {
			values[i], err = canonicalBytes(c.ctx, "analysis.transformer.projection-value", func(nested *canonical.Writer) error {
				if err := nested.Record(recordTraceValue); err != nil {
					return err
				}
				if err := nested.Uint(uint64(item.index)); err != nil {
					return err
				}
				value, err := c.valueBytes(item.value)
				if err != nil {
					return err
				}
				return nested.Bytes(value)
			})
			if err != nil {
				return err
			}
		}
		sortByteSlices(values)
		if err := writeByteSlices(w, values); err != nil {
			return err
		}
		if err := c.writeOperations(w, fragment.operations); err != nil {
			return err
		}
		artifact, err := summary.EncodeCanonical(c.ctx, c.relation.arena.reg, fragment.output)
		if err != nil {
			return err
		}
		if err := w.Bytes(artifact.Schema[:]); err != nil {
			return err
		}
		if err := w.Bytes(artifact.Semantic[:]); err != nil {
			return err
		}
		return w.Bytes(artifact.Bytes)
	})
}

func (c *relationCanonicalCodec) valueBytes(term ValueTerm) ([]byte, error) {
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if c.valueState == nil {
		c.valueState = make(map[ValueTerm]canonicalVisitState)
	}
	switch c.valueState[term] {
	case canonicalVisiting:
		return nil, &NonportableCanonicalRelationError{Feature: "cyclic value term DAG"}
	case canonicalDone:
		return c.values[term], nil
	}
	if c.relation.arena == nil || term == 0 || int(term) >= len(c.relation.arena.values) {
		return nil, &NonportableCanonicalRelationError{Feature: "invalid value term"}
	}
	node := c.relation.arena.values[term]
	if err := c.validateValueNode(node); err != nil {
		return nil, err
	}
	c.valueState[term] = canonicalVisiting
	encoded, err := canonicalBytes(c.ctx, "analysis.transformer.value-term", func(w *canonical.Writer) error {
		if err := w.Record(recordValue); err != nil {
			return err
		}
		if err := w.Uint(uint64(node.op)); err != nil {
			return err
		}
		if node.op == valueRoot {
			if err := w.Uint(uint64(node.root.Kind)); err != nil {
				return err
			}
			return w.Uint(uint64(node.root.Index))
		}
		if node.op == valueConstant || node.op == valueRefinement || node.op == valueRuntimeValidation {
			if err := c.writeProducts(w, []product.Value{node.value}); err != nil {
				return err
			}
		}
		if node.op == valueCallResult {
			if err := w.Uint(uint64(node.point)); err != nil {
				return err
			}
			if err := w.Int(int64(node.resultIndex)); err != nil {
				return err
			}
		}
		children := make([][]byte, len(node.args))
		for i, child := range node.args {
			var err error
			children[i], err = c.valueBytes(child)
			if err != nil {
				return err
			}
		}
		if node.op == valueJoin || node.op == valueScalarEqual || node.op == valueScalarNotEqual {
			sortByteSlices(children)
		}
		return writeByteSlices(w, children)
	})
	if err != nil {
		delete(c.valueState, term)
		return nil, err
	}
	c.values[term] = encoded
	c.valueState[term] = canonicalDone
	return encoded, nil
}

func (c *relationCanonicalCodec) validateValueNode(node valueNode) error {
	badArity := func(want string) error {
		return &NonportableCanonicalRelationError{Feature: fmt.Sprintf("value operation %d has arity %d, want %s", node.op, len(node.args), want)}
	}
	switch node.op {
	case valueRoot:
		if !c.relation.shape.validate(node.root) {
			return &NonportableCanonicalRelationError{Feature: "value root is outside relation shape"}
		}
		if len(node.args) != 0 {
			return badArity("0")
		}
	case valueConstant:
		if len(node.args) != 0 {
			return badArity("0")
		}
	case valueJoin:
		if len(node.args) < 2 {
			return badArity("at least 2")
		}
	case valueRefinement, valueRuntimeValidation, valueLuaTypeName, valueCallResult:
		if len(node.args) != 1 {
			return badArity("1")
		}
		if node.op == valueCallResult && node.resultIndex < 0 {
			return &NonportableCanonicalRelationError{Feature: "call result has invalid slot"}
		}
	case valueStringConcat, valueScalarEqual, valueScalarNotEqual, valueScalarAnd, valueScalarOr:
		if len(node.args) != 2 {
			return badArity("2")
		}
	case valueStaticIndex:
		if len(node.args) != 2 {
			return badArity("2")
		}
		if !c.relation.arena.validStaticIndexKey(node.args[1]) {
			return &NonportableCanonicalRelationError{Feature: "static index has a non-scalar key"}
		}
	default:
		return &NonportableCanonicalRelationError{Feature: fmt.Sprintf("value operation %d", node.op)}
	}
	return nil
}

func (c *relationCanonicalCodec) pathBytes(term PathTerm) ([]byte, error) {
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if encoded := c.paths[term]; encoded != nil {
		return encoded, nil
	}
	if c.relation.arena == nil || term == 0 || int(term) >= len(c.relation.arena.paths) {
		return nil, &NonportableCanonicalRelationError{Feature: "invalid path term"}
	}
	node := c.relation.arena.paths[term]
	if !c.relation.shape.validate(node.root) {
		return nil, &NonportableCanonicalRelationError{Feature: "path root is outside relation shape"}
	}
	encoded, err := canonicalBytes(c.ctx, "analysis.transformer.path-term", func(w *canonical.Writer) error {
		if err := w.Record(recordPath); err != nil {
			return err
		}
		if err := w.Uint(uint64(node.root.Kind)); err != nil {
			return err
		}
		if err := w.Uint(uint64(node.root.Index)); err != nil {
			return err
		}
		if err := w.Count(uint64(len(node.segments))); err != nil {
			return err
		}
		for _, segment := range node.segments {
			if err := w.Uint(uint64(segment.Kind)); err != nil {
				return err
			}
			if err := w.String(segment.Name); err != nil {
				return err
			}
			if err := w.Int(int64(segment.Index)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	c.paths[term] = encoded
	return encoded, nil
}

func (c *relationCanonicalCodec) guardBytes(term Guard) ([]byte, error) {
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if c.guardState == nil {
		c.guardState = make(map[Guard]canonicalVisitState)
	}
	switch c.guardState[term] {
	case canonicalVisiting:
		return nil, &NonportableCanonicalRelationError{Feature: "cyclic guard term DAG"}
	case canonicalDone:
		return c.guards[term], nil
	}
	if c.relation.arena == nil || term == 0 || int(term) >= len(c.relation.arena.guards) {
		return nil, &NonportableCanonicalRelationError{Feature: "invalid guard term"}
	}
	node := c.relation.arena.guards[term]
	switch node.op {
	case guardTrue, guardFalse:
		if node.value != 0 || len(node.args) != 0 {
			return nil, &NonportableCanonicalRelationError{Feature: "constant guard carries operands"}
		}
	case guardTruthy, guardFalsy:
		if node.value == 0 || len(node.args) != 0 {
			return nil, &NonportableCanonicalRelationError{Feature: "predicate guard has invalid arity"}
		}
	case guardAnd, guardOr:
		if node.value != 0 || len(node.args) < 2 {
			return nil, &NonportableCanonicalRelationError{Feature: "logical guard has invalid arity"}
		}
	default:
		return nil, &NonportableCanonicalRelationError{Feature: fmt.Sprintf("guard operation %d", node.op)}
	}
	c.guardState[term] = canonicalVisiting
	encoded, err := canonicalBytes(c.ctx, "analysis.transformer.guard-term", func(w *canonical.Writer) error {
		if err := w.Record(recordGuard); err != nil {
			return err
		}
		if err := w.Uint(uint64(node.op)); err != nil {
			return err
		}
		if node.op == guardTruthy || node.op == guardFalsy {
			value, err := c.valueBytes(node.value)
			if err != nil {
				return err
			}
			return w.Bytes(value)
		}
		children := make([][]byte, len(node.args))
		for i, child := range node.args {
			var err error
			children[i], err = c.guardBytes(child)
			if err != nil {
				return err
			}
		}
		if node.op == guardAnd || node.op == guardOr {
			sortByteSlices(children)
		}
		return writeByteSlices(w, children)
	})
	if err != nil {
		delete(c.guardState, term)
		return nil, err
	}
	c.guards[term] = encoded
	c.guardState[term] = canonicalDone
	return encoded, nil
}

func digestCanonical(ctx context.Context, domain string, encode func(*canonical.Writer) error) (evaluated.AuthorityDigest, error) {
	h := sha256.New()
	var writer canonical.Writer
	if err := writer.Reset(ctx, h, domain, canonicalTransformerIdentityVersion); err != nil {
		return evaluated.AuthorityDigest{}, err
	}
	if err := encode(&writer); err != nil {
		return evaluated.AuthorityDigest{}, err
	}
	if err := writer.Finish(); err != nil {
		return evaluated.AuthorityDigest{}, err
	}
	return availableAuthority(h.Sum(nil))
}

func canonicalBytes(ctx context.Context, domain string, encode func(*canonical.Writer) error) ([]byte, error) {
	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, domain, canonicalTransformerIdentityVersion); err != nil {
		return nil, err
	}
	if err := encode(&writer); err != nil {
		return nil, err
	}
	return writer.FinishBytes()
}

func availableAuthority(raw []byte) (evaluated.AuthorityDigest, error) {
	if len(raw) != sha256.Size {
		return evaluated.AuthorityDigest{}, fmt.Errorf("transformer: canonical digest has width %d", len(raw))
	}
	var digest evaluated.Digest
	copy(digest[:], raw)
	authority := evaluated.AuthorityDigest{Status: evaluated.AuthorityAvailable, Value: digest}
	if !authority.Available() {
		return evaluated.AuthorityDigest{}, fmt.Errorf("transformer: canonical digest is unavailable")
	}
	return authority, nil
}

func writeByteSlices(w *canonical.Writer, values [][]byte) error {
	if err := w.Count(uint64(len(values))); err != nil {
		return err
	}
	for _, value := range values {
		if err := w.Bytes(value); err != nil {
			return err
		}
	}
	return nil
}

func sortByteSlices(values [][]byte) {
	sort.Slice(values, func(i, j int) bool { return bytes.Compare(values[i], values[j]) < 0 })
}

func compactByteSlices(values [][]byte) [][]byte {
	if len(values) == 0 {
		return nil
	}
	out := values[:1]
	for _, value := range values[1:] {
		if !bytes.Equal(out[len(out)-1], value) {
			out = append(out, value)
		}
	}
	return out
}

func writeConcretePath(w *canonical.Writer, path pathdom.Path) error {
	if err := w.Bool(path.Symbol != 0); err != nil {
		return err
	}
	if path.Symbol != 0 {
		if err := w.Uint(uint64(path.Symbol)); err != nil {
			return err
		}
		if err := w.Int(int64(path.Version)); err != nil {
			return err
		}
	} else if err := w.String(path.Root); err != nil {
		return err
	}
	if err := w.Count(uint64(len(path.Segments))); err != nil {
		return err
	}
	for _, segment := range path.Segments {
		if err := w.Uint(uint64(segment.Kind)); err != nil {
			return err
		}
		if err := w.String(segment.Name); err != nil {
			return err
		}
		if err := w.Int(int64(segment.Index)); err != nil {
			return err
		}
	}
	return nil
}

func writeObservationIdentity(w *canonical.Writer, owner, route []byte, occurrence observation.Occurrence) error {
	if err := w.Bytes(owner); err != nil {
		return err
	}
	if err := w.Bytes(route); err != nil {
		return err
	}
	if err := w.Uint(uint64(occurrence.Point.Ordinal)); err != nil {
		return err
	}
	if err := w.Uint(uint64(occurrence.Point.Phase)); err != nil {
		return err
	}
	if err := w.Uint(uint64(occurrence.Kind)); err != nil {
		return err
	}
	return w.Uint(uint64(occurrence.Slot))
}

func writeRequirement(w *canonical.Writer, requirement operationplan.ObservationRequirement) error {
	if err := w.String(string(requirement.Projection())); err != nil {
		return err
	}
	if err := w.Uint(uint64(requirement.Stage())); err != nil {
		return err
	}
	if err := w.Uint(uint64(requirement.Point())); err != nil {
		return err
	}
	to, hasTo := requirement.EdgeTarget()
	if err := w.Bool(hasTo); err != nil {
		return err
	}
	if hasTo {
		if err := w.Uint(uint64(to)); err != nil {
			return err
		}
	}
	anchor, hasAnchor := requirement.Anchor()
	if err := w.Bool(hasAnchor); err != nil {
		return err
	}
	if hasAnchor {
		if err := writeObservationIdentity(w, nil, nil, anchor); err != nil {
			return err
		}
	}
	return w.Bool(requirement.RequiresCallOutcome())
}
