package pack

// This file is the receipt-native Pack construction path. It accepts mounted
// artifacts plus Link-owned substitution authorities and never reopens the
// source graph after sealing.

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/static"
)

// ArtifactMount is Pack's exact Link-local placement of one reusable artifact.
// Module is opaque and keeps repeated Program artifacts mounted at distinct
// Link locations without introducing a Shard coordinate.
type ArtifactMount struct {
	artifact *programartifact.Artifact
	module   identity.ContentID
	program  identity.ContentID
}

func NewArtifactMount(artifact *programartifact.Artifact, module, program identity.ContentID) (ArtifactMount, bool) {
	if artifact == nil || !artifact.Available() || !module.Available() || !program.Available() || artifact.CompileKey().ProgramID() != program {
		return ArtifactMount{}, false
	}
	return ArtifactMount{artifact: artifact, module: module, program: program}, true
}
func (mount ArtifactMount) Available() bool {
	return mount.artifact != nil && mount.artifact.Available() && mount.module.Available() && mount.program.Available() && mount.artifact.CompileKey().ProgramID() == mount.program
}
func (mount ArtifactMount) Module() identity.ContentID {
	if !mount.Available() {
		return identity.ContentID{}
	}
	return mount.module
}
func (mount ArtifactMount) Artifact() *programartifact.Artifact {
	if !mount.Available() {
		return nil
	}
	return mount.artifact
}

type artifactValuesKey struct{ module, values identity.ContentID }
type artifactCallKey struct{ module, call identity.ContentID }
type artifactBodyKey struct{ module, body identity.ContentID }
type artifactOutcomeKey struct{ module, outcome identity.ContentID }

// FormalCallRoot and FormalCallTypeArguments are portable Pack receipts. They
// are issued only from the mounted artifact constructor below; no Program
// proof-taking compatibility method remains.
type FormalCallRoot struct {
	call, id identity.ContentID
	sealed   bool
}

func (root FormalCallRoot) Valid() bool {
	return root.sealed && root.call.Available() && root.id.Available()
}
func (root FormalCallRoot) ContentID() (identity.ContentID, bool) { return root.id, root.Valid() }
func (root FormalCallRoot) Same(other FormalCallRoot) bool {
	return root.Valid() && other.Valid() && root == other
}

type FormalCallTypeArguments struct {
	id     identity.ContentID
	count  uint32
	sealed bool
}

func (formal FormalCallTypeArguments) Available() bool { return formal.sealed && formal.id.Available() }
func (formal FormalCallTypeArguments) ContentID() (identity.ContentID, bool) {
	return formal.id, formal.Available()
}
func (formal FormalCallTypeArguments) Count() int {
	if !formal.Available() {
		return 0
	}
	return int(formal.count)
}
func (formal FormalCallTypeArguments) Same(other FormalCallTypeArguments) bool {
	return formal.Available() && other.Available() && formal == other
}

func formalCallRootID(call identity.ContentID) identity.ContentID {
	if !call.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.pack.formal-call-root.v1\x00"))
	_, _ = hash.Write(call[:])
	return identity.ContentID(sha256.Sum256(hash.Sum(nil)))
}

func sealMountedFormalCallTypeArguments(arguments static.MountedTypeArguments) (FormalCallTypeArguments, bool) {
	if !arguments.Available() || arguments.Count() < 0 || uint64(arguments.Count()) > uint64(^uint32(0)) {
		return FormalCallTypeArguments{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.pack.formal-call-type-arguments.v1\x00"))
	for index := 0; index < arguments.Count(); index++ {
		argument, ok := arguments.At(index)
		id, idOK := argument.ContentID()
		if !ok || !idOK {
			return FormalCallTypeArguments{}, false
		}
		_, _ = hash.Write(id[:])
	}
	return FormalCallTypeArguments{id: identity.ContentID(sha256.Sum256(hash.Sum(nil))), count: uint32(arguments.Count()), sealed: true}, true
}

// MountedPayload is Heap's closed mounted Values projection. Exactly one
// alternative is present, so Heap cannot fabricate a tail from a fixed value.
type MountedPayloadKind uint8

const (
	MountedPayloadInvalid MountedPayloadKind = iota
	MountedPayloadFixed
	MountedPayloadTail
	MountedPayloadNil
)

type MountedPayload struct {
	schema  *schema
	kind    MountedPayloadKind
	fixed   SemanticSource
	payload Payload
}

func (payload MountedPayload) Available() bool {
	if payload.schema == nil {
		return false
	}
	switch payload.kind {
	case MountedPayloadFixed:
		return payload.fixed.Available()
	case MountedPayloadTail:
		return payload.payload.schema == payload.schema
	case MountedPayloadNil:
		return !payload.fixed.Available() && payload.payload.schema == nil
	default:
		return false
	}
}
func (payload MountedPayload) Kind() MountedPayloadKind {
	if !payload.Available() {
		return MountedPayloadInvalid
	}
	return payload.kind
}
func (payload MountedPayload) Fixed() (SemanticSource, bool) {
	return payload.fixed, payload.Available() && payload.kind == MountedPayloadFixed
}
func (payload MountedPayload) Tail() (Payload, bool) {
	return payload.payload, payload.Available() && payload.kind == MountedPayloadTail
}

// PayloadForMounted is the only Heap-facing Pack source projection. It takes
// the mounted artifact Values ID, never a Shard or raw Program Term.
func (schema *Schema) PayloadForMounted(module, valuesID identity.ContentID, offset int) (MountedPayload, bool) {
	if schema == nil || schema.state == nil || !module.Available() || !valuesID.Available() || offset < 0 {
		return MountedPayload{}, false
	}
	index, found := schema.state.artifactValues[artifactValuesKey{module, valuesID}]
	if !found || uint64(index) >= uint64(len(schema.state.values)) {
		return MountedPayload{}, false
	}
	row := schema.state.values[index]
	if offset < len(row.fixed) {
		endpoint := row.fixed[offset]
		if endpoint.index == 0 || uint64(endpoint.index) > uint64(len(schema.state.endpointSources)) {
			return MountedPayload{}, false
		}
		return MountedPayload{schema: schema.state, kind: MountedPayloadFixed, fixed: schema.state.endpointSources[endpoint.index-1]}, true
	}
	if row.tail.valid() {
		payloads, ok := schema.Payloads([]PayloadRequest{{Values: Values{schema: schema.state, index: index}, Index: offset}})
		if !ok || len(payloads) != 1 {
			return MountedPayload{}, false
		}
		return MountedPayload{schema: schema.state, kind: MountedPayloadTail, payload: payloads[0]}, true
	}
	return MountedPayload{schema: schema.state, kind: MountedPayloadNil}, true
}

// CallRootForMountedSemantic replaces the Project/Program proof projection
// with Boundary's exact mounted artifact identity.
func (schema *Schema) CallRootForMountedSemantic(module, callID identity.ContentID) (Root, bool) {
	if schema == nil || schema.state == nil {
		return Root{}, false
	}
	index, found := schema.state.artifactCalls[artifactCallKey{module, callID}]
	if !found || uint64(index) >= uint64(len(schema.state.calls)) {
		return Root{}, false
	}
	root := Root{schema: schema.state, index: schema.state.calls[index].root}
	return root, root.valid()
}

// MountedInputSemanticSource projects one exact fixed call input into the
// mounted semantic-source plane. It accepts only this Schema's presealed
// scalar selector and never manufactures a fixed source from an open actual
// tail. This is cold operand evidence for later Effect publication admission,
// not a runtime allocation or placement proof.
func (schema *Schema) MountedInputSemanticSource(module, callID identity.ContentID, selector InputSelector) (SemanticSource, bool) {
	if schema == nil || schema.state == nil || !module.Available() || !callID.Available() || !schema.OwnsInputSelector(selector) || selector.kind != inputSelectionScalar {
		return SemanticSource{}, false
	}
	index, found := schema.state.artifactCalls[artifactCallKey{module, callID}]
	if !found || uint64(index) >= uint64(len(schema.state.calls)) {
		return SemanticSource{}, false
	}
	row := schema.state.calls[index]
	if !schema.state.validMountedCall(row) || row.moduleKey != module || row.occurrenceID != callID || selector.start < 0 || selector.start >= len(row.fixed) {
		return SemanticSource{}, false
	}
	endpoint := row.fixed[selector.start]
	if !endpoint.valid() || endpoint.owner != schema.state.owner || endpoint.index == 0 || uint64(endpoint.index) > uint64(len(schema.state.endpointSources)) {
		return SemanticSource{}, false
	}
	source := schema.state.endpointSources[endpoint.index-1]
	return source, source.Available() && source.module == module
}

func (schema *Schema) FormalCallRootForMountedSemantic(module, callID identity.ContentID) (FormalCallRoot, bool) {
	root, ok := schema.CallRootForMountedSemantic(module, callID)
	if !ok {
		return FormalCallRoot{}, false
	}
	row := schema.state.calls[schema.state.roots[root.index].sourceIndex]
	formal := FormalCallRoot{call: row.formalID, id: formalCallRootID(row.formalID), sealed: true}
	return formal, formal.Valid()
}
func (schema *Schema) FormalTypeArgumentsForMountedSemantic(module, callID identity.ContentID) (FormalCallTypeArguments, bool) {
	root, ok := schema.CallRootForMountedSemantic(module, callID)
	if !ok {
		return FormalCallTypeArguments{}, false
	}
	row := schema.state.calls[schema.state.roots[root.index].sourceIndex]
	return row.typeFormal, row.typeFormal.Available()
}

// SealMountedArtifacts is the sole Pack production constructor.  Artifact
// rows are the complete Program fact plane; Boundary and Static only issue
// their existing opaque mounted substitutions.
func SealMountedArtifacts(source *link.Link, authority *static.Authority, mounts []ArtifactMount) (*Schema, bool) {
	if source == nil || authority == nil || authority.LinkID() != source.ContentID() || len(mounts) == 0 || source.Boundary() == nil || source.Project() == nil || authority.Classes() == nil {
		return nil, false
	}
	// Target is consumed at the Pack seal boundary to compile reusable input
	// selectors. The resulting Schema retains only Pack-owned templates, never
	// this Contract or a Link/Boundary backpointer.
	contract, contractOK := source.Boundary().Target()
	if !contractOK || contract == nil || !contract.ContentID().Available() {
		return nil, false
	}
	// The Link mount directory is the sole denominator and ordering authority.
	// A caller may supply artifact handles, but cannot omit a mounted module or
	// permute the canonical mount sequence before Pack assigns coordinates.
	linkMounts := source.Project().Mounts()
	if linkMounts.Count() != len(mounts) {
		return nil, false
	}
	maximum := 0
	for index := 0; index < contract.OperationCount(); index++ {
		operation, operationOK := contract.OperationAt(index)
		if !operationOK {
			return nil, false
		}
		if fixed := contract.ValueFormalCount(operation); fixed > maximum {
			maximum = fixed
		}
	}
	seenModules := make(map[identity.ContentID]struct{}, len(mounts))
	for index, mount := range mounts {
		if !mount.Available() {
			return nil, false
		}
		if !mountedArtifactMatchesLink(source, index, mount) {
			return nil, false
		}
		if _, duplicate := seenModules[mount.module]; duplicate {
			return nil, false
		}
		seenModules[mount.module] = struct{}{}
		artifact := mount.artifact
		for i := 0; i < artifact.ValuesCount(); i++ {
			row, ok := artifact.ValuesAt(i)
			if !ok {
				return nil, false
			}
			if row.MemberCount()+1 > maximum {
				maximum = row.MemberCount() + 1
			}
		}
		for i := 0; i < artifact.HeapIndexCount(); i++ {
			row, ok := artifact.HeapIndexAt(i)
			if !ok {
				return nil, false
			}
			if _, position, write := row.Values(); write && position > maximum {
				maximum = position
			}
		}
		for i := 0; i < artifact.OccurrenceKindCount(programartifact.OccurrenceStorageBind); i++ {
			row, ok := artifact.OccurrenceKindAt(programartifact.OccurrenceStorageBind, i)
			if !ok || row.InputCount() == 0 {
				return nil, false
			}
			if width := row.InputCount() - 1; width > maximum {
				maximum = width
			}
		}
		for i := 0; i < artifact.FunctionBoundaryCount(); i++ {
			row, ok := artifact.FunctionBoundaryAt(i)
			if !ok {
				return nil, false
			}
			if row.FormalCount() > maximum {
				maximum = row.FormalCount()
			}
		}
	}
	if maximum < 0 {
		return nil, false
	}
	offsets := make([]nat, maximum+1)
	for i := range offsets {
		offsets[i] = natFromUint64(uint64(i))
	}
	owner, ok := newAlgebraWithOffsets(authority.Classes(), nil, offsets)
	if !ok {
		return nil, false
	}
	state := &schema{linkOwner: source.OwnerCapability(), owner: owner,
		relationIndex: make(map[*relation]uint32), endpointIndex: make(map[SemanticSource]Endpoint), semanticEndpoints: make(map[artifactValuesKey]Endpoint), artifactValues: make(map[artifactValuesKey]uint32), artifactTails: make(map[artifactValuesKey]uint32), artifactCalls: make(map[artifactCallKey]uint32), artifactBodies: make(map[artifactBodyKey]uint32), artifactOutcomes: make(map[artifactOutcomeKey]uint32), inputSelectors: make(map[inputSelectorKey]InputSelector),
	}
	if !sealInputSelectors(state, contract) {
		return nil, false
	}
	for _, mount := range mounts {
		if !sealMountedSemanticEndpoints(state, mount) {
			return nil, false
		}
	}
	sealed := &Schema{state: state}
	for _, mount := range mounts {
		if !sealMountedArtifactValues(sealed, mount) || !sealMountedArtifactBodies(sealed, mount) || !sealMountedArtifactBinds(sealed, mount) || !sealMountedArtifactCalls(sealed, authority, mount) {
			return nil, false
		}
	}
	if len(state.roots) == 0 || !sealed.sealSourceResults() {
		return nil, false
	}
	return sealed, true
}

// sealInputSelectors lowers the complete authenticated Target input ABI into
// Pack-owned selection templates. A ValueFormal is fixed only when it names a
// fixed Target input slot. A ValuesVar is available only for that operation's
// actual input tail; AllInputs is the synthesized opaque operation's sole
// authority. Consequently a scalar selector never silently turns a tail-only
// input into an exact fixed semantic source.
func sealInputSelectors(state *schema, contract *target.Contract) bool {
	if state == nil || state.owner == nil || contract == nil || !contract.ContentID().Available() || state.inputSelectors == nil {
		return false
	}
	opaque, opaqueOK := contract.Opaque()
	if !opaqueOK {
		return false
	}
	add := func(operation target.Operation, source target.InputSource, selector InputSelector) bool {
		key := inputSelectorKey{operation: operation, source: source}
		if _, duplicate := state.inputSelectors[key]; duplicate || !selector.valid() || selector.schema != state {
			return false
		}
		state.inputSelectors[key] = selector
		return true
	}
	for index := 0; index < contract.OperationCount(); index++ {
		operation, operationOK := contract.OperationAt(index)
		if !operationOK {
			return false
		}
		fixed := contract.ValueFormalCount(operation)
		input, inputOK := contract.Input(operation)
		if !inputOK || fixed != contract.ValuesCount(input) {
			return false
		}
		for formal := 0; formal < fixed; formal++ {
			offset, offsetOK := offsetForUint64(state.owner, uint64(formal))
			table, tableOK := tableIndexForOffset(offset)
			selector := InputSelector{schema: state, kind: inputSelectionScalar, table: table, start: formal, sealed: true}
			if !offsetOK || !tableOK || !add(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(formal)}, selector) {
				return false
			}
		}
		tail, variable, tailOK := contract.ValuesTail(input)
		if !tailOK {
			return false
		}
		if tail == target.ValuesVariable {
			selector := InputSelector{schema: state, kind: inputSelectionTail, start: fixed, sealed: true}
			if !add(operation, target.InputSource{Kind: target.InputSourceValuesVar, Ordinal: uint32(variable)}, selector) {
				return false
			}
		}
		if operation == opaque {
			selector := InputSelector{schema: state, kind: inputSelectionWhole, start: 0, sealed: true}
			if !add(operation, target.InputSource{Kind: target.InputSourceAllInputs}, selector) {
				return false
			}
		}
	}
	return true
}

// mountedArtifactMatchesLink authenticates one caller-supplied artifact
// against the exact owner-fenced Project mount position. It reads only the
// canonical ModuleKey and ProgramID; it never opens the mounted Program.
func mountedArtifactMatchesLink(source *link.Link, index int, mount ArtifactMount) bool {
	if source == nil || source.Project() == nil || index < 0 || !mount.Available() {
		return false
	}
	mounts := source.Project().Mounts()
	if index >= mounts.Count() {
		return false
	}
	shard, shardOK := mounts.At(index)
	module, moduleOK := source.Project().ModuleKey(shard)
	program, programOK := mounts.ProgramID(shard)
	return shardOK && moduleOK && programOK && module == mount.module && program == mount.program
}

func sealMountedArtifactValues(schema *Schema, mount ArtifactMount) bool {
	state, artifact := schema.state, mount.artifact
	for i := 0; i < artifact.ValuesCount(); i++ {
		row, ok := artifact.ValuesAt(i)
		if !ok {
			return false
		}
		key := artifactValuesKey{mount.module, row.ID()}
		if _, duplicate := state.artifactValues[key]; duplicate {
			return false
		}
		rootID := mountedArtifactRootID(rootValues, mount.module, row.ID())
		root, port, ok := state.addRootWithID(state.owner.classes, rootRow{kind: rootValues, id: rootID})
		if !ok {
			return false
		}
		out := valuesRow{root: root, moduleKey: mount.module, occurrenceID: row.ID(), port: port}
		for member := 0; member < row.MemberCount(); member++ {
			m, memberOK := row.MemberAt(member)
			endpoint, endpointOK := state.semanticEndpoints[artifactValuesKey{mount.module, m.ID()}]
			if !memberOK || !endpointOK {
				return false
			}
			out.fixed = append(out.fixed, endpoint)
		}
		if tail, open := row.Tail(); open {
			port, tailOK := sealMountedArtifactTail(schema, mount.module, tail)
			if !tailOK {
				return false
			}
			out.tail = port
		}
		state.artifactValues[key] = uint32(len(state.values))
		state.values = append(state.values, out)
		state.roots[root].sourceIndex = uint32(len(state.values) - 1)
	}
	return true
}

func sealMountedArtifactTail(schema *Schema, module identity.ContentID, tail programartifact.ValuesTail) (Port, bool) {
	state := schema.state
	id := tail.ID()
	endpoint, endpointOK := state.semanticEndpoints[artifactValuesKey{module, id}]
	if !tail.Present() || !id.Available() || !endpointOK {
		return Port{}, false
	}
	key := artifactValuesKey{module, id}
	if index, exists := state.artifactTails[key]; exists {
		row := state.tails[index]
		return row.port, row.sealed && row.port.valid() && row.valueID == id
	}
	kind := TailProducerInvalid
	switch tail.Kind() {
	case programartifact.ValuesTailCall:
		kind = TailProducerCall
	case programartifact.ValuesTailVararg:
		kind = TailProducerVararg
	default:
		return Port{}, false
	}
	root, port, ok := state.addRootWithID(state.owner.classes, rootRow{kind: rootTail, id: mountedArtifactRootID(rootTail, module, id)})
	if !ok || !endpoint.valid() {
		return Port{}, false
	}
	index := uint32(len(state.tails))
	state.artifactTails[key] = index
	state.tails = append(state.tails, tailRow{root: root, moduleKey: module, valueID: id, port: port, kind: kind, sealed: true})
	state.roots[root].sourceIndex = index
	return port, true
}

func (state *schema) addArtifactRoot(classes *static.ClassSet, kind rootKind, module, id identity.ContentID, port Port, scalars []Endpoint) (uint32, bool) {
	if state == nil || classes == nil || !id.Available() || !port.valid() || port.owner != state.owner {
		return 0, false
	}
	index := uint32(len(state.roots))
	state.roots = append(state.roots, rootRow{kind: kind, id: mountedArtifactRootID(kind, module, id), port: port})
	targets := make([]equationTarget, 0, len(scalars)+1)
	for _, endpoint := range scalars {
		if !endpoint.valid() || endpoint.owner != state.owner {
			return 0, false
		}
		targets = append(targets, equationTarget{kind: EquationScalar, index: endpoint.index})
	}
	targets = append(targets, equationTarget{kind: EquationPack, index: port.index})
	relation, ok := sealRelation(state.owner, index+1, targets)
	if !ok {
		return 0, false
	}
	state.relations = append(state.relations, relation)
	state.relationIndex[relation] = index
	return index, true
}

func sealMountedArtifactBinds(schema *Schema, mount ArtifactMount) bool {
	state, artifact := schema.state, mount.artifact
	for i := 0; i < artifact.OccurrenceKindCount(programartifact.OccurrenceStorageBind); i++ {
		row, ok := artifact.OccurrenceKindAt(programartifact.OccurrenceStorageBind, i)
		if !ok || row.InputCount() == 0 {
			return false
		}
		valuesID, valuesOK := row.InputAt(0)
		bodyID, bodyOK := row.BodyID()
		if !valuesOK || !bodyOK {
			return false
		}
		valuesIndex, exists := state.artifactValues[artifactValuesKey{mount.module, valuesID}]
		if !exists {
			return false
		}
		cells := make([]Endpoint, row.InputCount()-1)
		for j := range cells {
			id, ok := row.InputAt(j + 1)
			endpoint, endpointOK := state.semanticEndpoints[artifactValuesKey{mount.module, id}]
			if !ok || !endpointOK {
				return false
			}
			cells[j] = endpoint
		}
		port, portOK := newPort(state.owner, uint32(len(state.roots)+1), state.owner.classes.AnyValue(), false)
		if !portOK {
			return false
		}
		root, rootOK := state.addArtifactRoot(state.owner.classes, rootBind, mount.module, row.ID(), port, cells)
		if !rootOK {
			return false
		}
		state.binds = append(state.binds, bindRow{root: root, moduleKey: mount.module, bindID: row.ID(), bodyID: bodyID, values: Values{schema: state, index: valuesIndex}, port: port, cells: cells})
		state.roots[root].sourceIndex = uint32(len(state.binds) - 1)
	}
	return true
}

// Bodies are mounted directly from the canonical Artifact Body and callable
// boundary columns. Pack retains only its runtime substitution state.
func sealMountedArtifactBodies(schema *Schema, mount ArtifactMount) bool {
	state, artifact := schema.state, mount.artifact
	for i := 0; i < artifact.BodyCount(); i++ {
		row, rowOK := artifact.BodyAt(i)
		if !rowOK {
			return false
		}
		key := artifactBodyKey{mount.module, row.ID()}
		if _, duplicate := state.artifactBodies[key]; duplicate {
			return false
		}
		boundary, callable := artifact.FunctionBoundaryForBody(row.ID())
		if row.Callable() != callable {
			return false
		}
		formalCount := 0
		if callable {
			formalCount = boundary.FormalCount()
		}
		formals := make([]Endpoint, formalCount)
		formalIDs := make([]identity.ContentID, formalCount)
		for j := range formals {
			formal, formalOK := boundary.FormalAt(j)
			endpoint, endpointOK := state.semanticEndpoints[artifactValuesKey{mount.module, formal.StorageCellID()}]
			if !formalOK || !endpointOK {
				return false
			}
			formals[j], formalIDs[j] = endpoint, formal.ID()
		}
		port, portOK := newPort(state.owner, uint32(len(state.roots)+1), state.owner.classes.AnyValue(), true)
		if !portOK {
			return false
		}
		root, rootOK := state.addArtifactRoot(state.owner.classes, rootBody, mount.module, row.ID(), port, formals)
		if !rootOK {
			return false
		}
		state.artifactBodies[key] = uint32(len(state.bodies))
		state.bodies = append(state.bodies, bodyRow{root: root, bodyID: row.ID(), context: row.ContextID(), moduleKey: mount.module, port: port, formals: formals, formalIDs: formalIDs, sealed: true})
		state.roots[root].sourceIndex = uint32(len(state.bodies) - 1)
	}
	// Outcomes have no scalar targets.  Return Values are already values-root
	// identities; retain only the root links needed by boundary rules.
	for i := 0; i < artifact.OutcomeCount(); i++ {
		row, rowOK := artifact.OutcomeAt(i)
		if !rowOK {
			return false
		}
		bodyIndex, bodyOK := state.artifactBodies[artifactBodyKey{mount.module, row.BodyID()}]
		if !bodyOK {
			return false
		}
		// Pack owns only normal and return boundary roots. The artifact owns
		// other terminal geometry; validate Body ownership but mint no Pack root.
		if row.Kind() != programartifact.OutcomeNormal && row.Kind() != programartifact.OutcomeReturn {
			continue
		}
		key := artifactOutcomeKey{mount.module, row.ID()}
		if _, duplicate := state.artifactOutcomes[key]; duplicate {
			return false
		}
		port, portOK := newPort(state.owner, uint32(len(state.roots)+1), state.owner.classes.AnyValue(), false)
		if !portOK {
			return false
		}
		root, rootOK := state.addArtifactRoot(state.owner.classes, rootOutcome, mount.module, row.ID(), port, nil)
		if !rootOK {
			return false
		}
		out := outcomeRow{root: root, bodyIndex: bodyIndex, moduleKey: mount.module, bodyID: row.BodyID(), outcomeID: row.ID(), port: port, sealed: true}
		switch row.Kind() {
		case programartifact.OutcomeNormal:
			out.kind = 1
		case programartifact.OutcomeReturn:
			out.kind = 2
		}
		if out.kind == 2 {
			for j := 0; j < row.ReturnValueCount(); j++ {
				value, valueOK := artifact.OutcomeReturnValueAt(i, j)
				valuesIndex, valuesOK := state.artifactValues[artifactValuesKey{mount.module, value.ID()}]
				if !valueOK || !valuesOK {
					return false
				}
				out.valueRoots = append(out.valueRoots, state.values[valuesIndex].root)
			}
		}
		state.artifactOutcomes[key] = uint32(len(state.outcomes))
		state.outcomes = append(state.outcomes, out)
		state.roots[root].sourceIndex = uint32(len(state.outcomes) - 1)
	}
	return true
}

func sealMountedArtifactCalls(schema *Schema, authority *static.Authority, mount ArtifactMount) bool {
	state, artifact := schema.state, mount.artifact
	for i := 0; i < artifact.CallCount(); i++ {
		row, rowOK := artifact.CallAt(i)
		if !rowOK {
			return false
		}
		key := artifactCallKey{mount.module, row.ID()}
		if _, duplicate := state.artifactCalls[key]; duplicate {
			return false
		}
		types, typesOK := authority.MountedCallTypeArguments(mount.module, row.TypeArgumentsID())
		typeFormal, typeFormalOK := sealMountedFormalCallTypeArguments(types)
		if !typesOK || !typeFormalOK {
			return false
		}
		rootID := mountedCallRootID(mount.module, row.FormalID())
		root, port, rootOK := state.addRootWithID(state.owner.classes, rootRow{kind: rootCall, id: rootID})
		if !rootOK {
			return false
		}
		out := callRow{root: root, mountedID: row.ID(), occurrenceID: row.ID(), valuesID: row.ValuesID(), typesID: row.TypeArgumentsID(), form: row.Form(), moduleKey: mount.module, formalID: row.FormalID(), typeFormal: typeFormal, port: port}
		if row.Form() == 2 {
			receiverID, receiverOK := row.ReceiverID()
			endpoint, endpointOK := state.semanticEndpoints[artifactValuesKey{mount.module, receiverID}]
			if !receiverOK || !endpointOK {
				return false
			}
			out.receiverID = receiverID
			out.fixed = append(out.fixed, endpoint)
		}
		for j := 0; j < row.ArgumentCount(); j++ {
			argument, argumentOK := artifact.CallArgumentFor(i, j)
			endpoint, endpointOK := state.semanticEndpoints[artifactValuesKey{mount.module, argument.ValueID()}]
			if !argumentOK || !endpointOK {
				return false
			}
			out.fixed = append(out.fixed, endpoint)
		}
		if tailID, open := row.TailID(); open {
			tailIndex, tailOK := state.artifactTails[artifactValuesKey{mount.module, tailID}]
			if !tailOK {
				return false
			}
			out.actualTailID = tailID
			out.tailContext = tailID
			out.tail = state.tails[tailIndex].port
		}
		// A call-result tail is identified by the artifact's exact producer
		// occurrence ID. Boundary Value IDs are a different namespace and must
		// never be guessed as Pack tail lookup keys.
		if tailIndex, tailOK := state.artifactTails[artifactValuesKey{mount.module, row.ID()}]; tailOK {
			out.resultTail, out.hasResult = state.tails[tailIndex].root, true
		}
		state.artifactCalls[key] = uint32(len(state.calls))
		state.calls = append(state.calls, out)
		state.roots[root].sourceIndex = uint32(len(state.calls) - 1)
	}
	return true
}

func mountedArtifactRootID(kind rootKind, module, semantic identity.ContentID) identity.ContentID {
	if kind == rootInvalid || !module.Available() || !semantic.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.pack.mounted-artifact-root.v1\x00"))
	_, _ = hash.Write([]byte{byte(kind)})
	_, _ = hash.Write(module[:])
	_, _ = hash.Write(semantic[:])
	return identity.ContentID(sha256.Sum256(hash.Sum(nil)))
}

func (state *schema) addRootWithID(classes *static.ClassSet, row rootRow) (uint32, Port, bool) {
	if state == nil || state.owner == nil || classes == nil || row.kind == rootInvalid || !row.id.Available() {
		return 0, Port{}, false
	}
	port, ok := newPort(state.owner, uint32(len(state.roots)+1), classes.AnyValue(), true)
	if !ok {
		return 0, Port{}, false
	}
	index := uint32(len(state.roots))
	row.port = port
	state.roots = append(state.roots, row)
	relation, ok := sealRelation(state.owner, index+1, []equationTarget{{kind: EquationPack, index: port.index}})
	if !ok {
		return 0, Port{}, false
	}
	state.relations = append(state.relations, relation)
	state.relationIndex[relation] = index
	return index, port, true
}

func mountedCallRootID(module, formal identity.ContentID) identity.ContentID {
	return mountedArtifactRootID(rootCall, module, formal)
}

func sealMountedSemanticEndpoints(state *schema, mount ArtifactMount) bool {
	if state == nil || !mount.Available() {
		return false
	}
	add := func(id identity.ContentID) bool {
		source, sourceOK := newSemanticSource(mount.module, id)
		if !sourceOK {
			return false
		}
		key := artifactValuesKey{mount.module, id}
		if _, exists := state.semanticEndpoints[key]; exists {
			return true
		}
		if uint64(len(state.endpointSources)) >= uint64(^uint32(0)) {
			return false
		}
		endpoint, endpointOK := newEndpoint(state.owner, uint32(len(state.endpointSources)+1), state.owner.classes.AnyValue())
		if !endpointOK {
			return false
		}
		state.endpointSources = append(state.endpointSources, source)
		state.endpointIndex[source] = endpoint
		state.semanticEndpoints[key] = endpoint
		return true
	}
	artifact := mount.artifact
	for i := 0; i < artifact.ValuesCount(); i++ {
		row, ok := artifact.ValuesAt(i)
		if !ok {
			return false
		}
		for j := 0; j < row.MemberCount(); j++ {
			member, ok := row.MemberAt(j)
			if !ok || !add(member.ID()) {
				return false
			}
		}
		if tail, open := row.Tail(); open && !add(tail.ID()) {
			return false
		}
	}
	for i := 0; i < artifact.OccurrenceKindCount(programartifact.OccurrenceStorageBind); i++ {
		row, ok := artifact.OccurrenceKindAt(programartifact.OccurrenceStorageBind, i)
		if !ok || row.InputCount() == 0 {
			return false
		}
		for j := 1; j < row.InputCount(); j++ {
			id, idOK := row.InputAt(j)
			if !idOK || !add(id) {
				return false
			}
		}
	}
	for i := 0; i < artifact.FunctionBoundaryCount(); i++ {
		row, ok := artifact.FunctionBoundaryAt(i)
		if !ok {
			return false
		}
		for j := 0; j < row.FormalCount(); j++ {
			formal, formalOK := row.FormalAt(j)
			if !formalOK || !add(formal.StorageCellID()) {
				return false
			}
		}
	}
	for i := 0; i < artifact.CallCount(); i++ {
		row, ok := artifact.CallAt(i)
		if !ok {
			return false
		}
		if receiver, present := row.ReceiverID(); present && !add(receiver) {
			return false
		}
		for j := 0; j < row.ArgumentCount(); j++ {
			argument, argumentOK := artifact.CallArgumentFor(i, j)
			if !argumentOK || !add(argument.ValueID()) {
				return false
			}
		}
	}
	return true
}
