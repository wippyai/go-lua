package pack

import "github.com/wippyai/go-lua/analysis/identity"

// MountedActualProjection is Pack's detached projection of one authored call
// actuals row.  The fixed sources are in the Program declaration order used
// by callRow: a method receiver first, followed by its arguments.  A
// variadic/open actuals tail is retained only as its existing semantic
// identity and Pack-issued whole-pack Port.
//
// Endpoint and callRow remain private.  In particular, this descriptor does
// not expose a raw endpoint, Program row, Link, or an inferred actual.
type MountedActualProjection struct {
	schema  *schema
	module  identity.ContentID
	call    identity.ContentID
	fixed   []Endpoint
	tailID  identity.ContentID
	tail    Port
	hasTail bool
	sealed  bool
}

// available is the hot receipt check. Construction validates every fixed
// source and the optional tail before setting sealed; immutable projections
// must not replay that O(n) proof on every accessor.
func (projection MountedActualProjection) available() bool {
	return projection.sealed && projection.schema != nil && projection.schema.owner != nil &&
		projection.schema.owner.valid() && projection.module.Available() && projection.call.Available()
}

// valid is construction-only full validation.
func (projection MountedActualProjection) valid() bool {
	if !projection.available() {
		return false
	}
	for _, endpoint := range projection.fixed {
		source, sourceOK := projection.sourceFor(endpoint)
		if !sourceOK || !source.Available() || source.Module() != projection.module || !projection.schemaOwnsSource(source) {
			return false
		}
	}
	if !projection.hasTail {
		return !projection.tailID.Available() && !projection.tail.valid()
	}
	if !projection.tailID.Available() || !projection.tail.valid() || projection.tail.owner != projection.schema.owner {
		return false
	}
	index, ok := projection.schema.artifactTails[artifactValuesKey{projection.module, projection.tailID}]
	if !ok || uint64(index) >= uint64(len(projection.schema.tails)) {
		return false
	}
	row := projection.schema.tails[index]
	return row.sealed && row.moduleKey == projection.module && row.valueID == projection.tailID && row.port == projection.tail
}

func (projection MountedActualProjection) sourceFor(endpoint Endpoint) (SemanticSource, bool) {
	if projection.schema == nil || !endpoint.valid() || endpoint.owner != projection.schema.owner || endpoint.index == 0 || uint64(endpoint.index) > uint64(len(projection.schema.endpointSources)) {
		return SemanticSource{}, false
	}
	source := projection.schema.endpointSources[endpoint.index-1]
	return source, source.Available() && projection.schema.endpointOwned(source)
}

// schemaOwnsSource keeps the owner check local to Pack without exposing the
// schema's private replay directory to consumers.
func (projection MountedActualProjection) schemaOwnsSource(source SemanticSource) bool {
	if projection.schema == nil || !source.Available() {
		return false
	}
	endpoint, ok := projection.schema.endpointIndex[source]
	return ok && endpoint.valid() && endpoint.owner == projection.schema.owner
}

// Valid reports whether this descriptor was issued by a sealed Pack Schema.
// Full row validation occurs once before issuance.
func (projection MountedActualProjection) Valid() bool { return projection.available() }

// OwnedBy proves that the descriptor was issued by this exact Pack Schema.
// Equivalent independently sealed schemas intentionally do not pass.
func (projection MountedActualProjection) OwnedBy(schema *Schema) bool {
	return schema != nil && schema.state != nil && projection.available() && projection.schema == schema.state
}

// ActualCount reports the number of fixed actual sources.  A receiver, when
// present, is included at index zero.
func (projection MountedActualProjection) ActualCount() int {
	if !projection.available() {
		return 0
	}
	return len(projection.fixed)
}

// ActualAt returns one fixed actual source in receiver-then-argument order.
func (projection MountedActualProjection) ActualAt(index int) (SemanticSource, bool) {
	if !projection.available() || index < 0 || index >= len(projection.fixed) {
		return SemanticSource{}, false
	}
	source, ok := projection.sourceFor(projection.fixed[index])
	return source, ok && source.Module() == projection.module
}

// TailID returns the exact mounted semantic identity of the actuals tail.
func (projection MountedActualProjection) TailID() (identity.ContentID, bool) {
	return projection.tailID, projection.available() && projection.hasTail
}

// MountedActualProjection projects one sealed mounted call in O(1) without
// copying its fixed actual row. The descriptor retains the schema's immutable
// sealed Endpoint image and resolves sources lazily in O(1); callers never
// receive callRow or Endpoint.
func (schema *Schema) MountedActualProjection(module, callID identity.ContentID) (MountedActualProjection, bool) {
	if schema == nil || schema.state == nil || schema.state.owner == nil || !schema.state.owner.valid() || !schema.state.linkOwner.Available() || !module.Available() || !callID.Available() {
		return MountedActualProjection{}, false
	}
	index, found := schema.state.artifactCalls[artifactCallKey{module, callID}]
	if !found || uint64(index) >= uint64(len(schema.state.calls)) {
		return MountedActualProjection{}, false
	}
	row := schema.state.calls[index]
	if !schema.state.validMountedCall(row) || row.moduleKey != module || row.occurrenceID != callID ||
		schema.state.artifactCalls[artifactCallKey{row.moduleKey, row.occurrenceID}] != index {
		return MountedActualProjection{}, false
	}

	projection := MountedActualProjection{
		schema: schema.state,
		module: module,
		call:   callID,
		fixed:  row.fixed,
	}

	if row.actualTailID.Available() {
		if !row.tail.valid() || row.tail.owner != schema.state.owner || row.tailContext != row.actualTailID {
			return MountedActualProjection{}, false
		}
		tailIndex, tailOK := schema.state.artifactTails[artifactValuesKey{module, row.actualTailID}]
		if !tailOK || uint64(tailIndex) >= uint64(len(schema.state.tails)) {
			return MountedActualProjection{}, false
		}
		tail := schema.state.tails[tailIndex]
		if !tail.sealed || tail.moduleKey != module || tail.valueID != row.actualTailID || tail.port != row.tail {
			return MountedActualProjection{}, false
		}
		projection.tailID = row.actualTailID
		projection.tail = row.tail
		projection.hasTail = true
	} else if row.tail.valid() || row.tailContext.Available() {
		return MountedActualProjection{}, false
	}
	projection.sealed = true
	return projection, projection.valid()
}

func (state *schema) endpointOwned(source SemanticSource) bool {
	if state == nil || !source.Available() {
		return false
	}
	endpoint, ok := state.endpointIndex[source]
	return ok && endpoint.valid() && endpoint.owner == state.owner
}
