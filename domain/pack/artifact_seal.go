package pack

// This file is Pack's mounted-row sealing path. It accepts mounted
// artifacts plus Link-owned substitution authorities and never reopens the
// source graph after sealing.

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/static"
)

type artifactValuesKey struct{ module, values identity.ContentID }
type artifactCallKey struct{ module, call identity.ContentID }

func endpointForMountedSemantic(state *schema, module, id identity.ContentID) (Endpoint, bool) {
	if state == nil || state.owner == nil || !state.owner.valid() {
		return Endpoint{}, false
	}
	source, sourceOK := newSemanticSource(module, id)
	if !sourceOK {
		return Endpoint{}, false
	}
	endpoint, found := state.endpointIndex[source]
	if !found {
		return Endpoint{}, false
	}
	issued, issuedOK := state.sourceForEndpoint(endpoint)
	return endpoint, issuedOK && issued == source
}

// FormalCallRoot is Pack's portable call identity. Type-argument sequence
// semantics are issued by Static and retained directly; Pack does not wrap or
// re-hash them.
type FormalCallRoot struct {
	id     identity.ContentID
	sealed bool
}

func (root FormalCallRoot) Valid() bool {
	return root.sealed && root.id.Available()
}
func (root FormalCallRoot) ContentID() (identity.ContentID, bool) { return root.id, root.Valid() }
func (root FormalCallRoot) Same(other FormalCallRoot) bool {
	return root.Valid() && other.Valid() && root == other
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
	sealed  bool
}

func (payload MountedPayload) available() bool {
	return payload.sealed && payload.schema != nil && payload.schema.owner != nil && payload.schema.owner.valid() &&
		(payload.kind == MountedPayloadFixed || payload.kind == MountedPayloadTail || payload.kind == MountedPayloadNil)
}

// valid is the construction-only union proof. Published descriptors are
// immutable, so accessors need not replay endpoint or selection validation.
func (payload MountedPayload) valid() bool {
	if !payload.available() {
		return false
	}
	switch payload.kind {
	case MountedPayloadFixed:
		endpoint, ok := payload.schema.endpointIndex[payload.fixed]
		issued, issuedOK := payload.schema.sourceForEndpoint(endpoint)
		return ok && issuedOK && issued == payload.fixed && payload.payload == (Payload{})
	case MountedPayloadTail:
		return !payload.fixed.Available() && payload.payload.valid() && payload.payload.selection.schema == payload.schema
	case MountedPayloadNil:
		return !payload.fixed.Available() && payload.payload == (Payload{})
	default:
		return false
	}
}
func (payload MountedPayload) Available() bool { return payload.available() }
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
		fixed, fixedOK := schema.state.sourceForEndpoint(row.fixed[offset])
		if !fixedOK {
			return MountedPayload{}, false
		}
		mounted := MountedPayload{schema: schema.state, kind: MountedPayloadFixed, fixed: fixed, sealed: true}
		return mounted, mounted.valid()
	}
	if row.hasTail {
		values := Values{schema: schema.state, index: index}
		table, tableOK := schema.TableIndex(int64(offset))
		if !values.valid() || !tableOK {
			return MountedPayload{}, false
		}
		for _, endpoint := range row.fixed {
			if _, sourceOK := schema.state.sourceForEndpoint(endpoint); !sourceOK {
				return MountedPayload{}, false
			}
		}
		selection := ScalarSelection{schema: schema.state, values: values, kind: scalarSelectionTableIndex, tableIndex: table, sealed: true}
		mounted := MountedPayload{schema: schema.state, kind: MountedPayloadTail, payload: Payload{selection: selection}, sealed: true}
		return mounted, mounted.valid()
	}
	mounted := MountedPayload{schema: schema.state, kind: MountedPayloadNil, sealed: true}
	return mounted, mounted.valid()
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

func (schema *Schema) FormalCallRootForMountedSemantic(module, callID identity.ContentID) (FormalCallRoot, bool) {
	root, ok := schema.CallRootForMountedSemantic(module, callID)
	if !ok {
		return FormalCallRoot{}, false
	}
	row := schema.state.calls[schema.state.roots[root.index].sourceIndex]
	formal := FormalCallRoot{id: formalCallRootID(row.formalID), sealed: true}
	return formal, formal.Valid()
}
func (schema *Schema) TypeArgumentSequenceForMountedSemantic(module, callID identity.ContentID) (static.TypeArgumentSequence, bool) {
	root, ok := schema.CallRootForMountedSemantic(module, callID)
	if !ok {
		return static.TypeArgumentSequence{}, false
	}
	row := schema.state.calls[schema.state.roots[root.index].sourceIndex]
	return row.typeArguments, row.typeArguments.Available()
}

// SealMountedArtifacts is the sole Pack production constructor.  Artifact
// rows are the complete Program fact plane; Boundary and Static only issue
// their existing opaque mounted substitutions.
func SealMountedArtifacts(source *link.Link, authority *static.Authority, mounts []programmount.MountedArtifact) (*Schema, bool) {
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
	for index := 0; index < contract.Operations.OperationCount(); index++ {
		operation, operationOK := contract.Operations.OperationAt(index)
		if !operationOK {
			return nil, false
		}
		if fixed := contract.Operations.ValueFormalCount(operation); fixed > maximum {
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
		if _, duplicate := seenModules[mount.ModuleKey]; duplicate {
			return nil, false
		}
		seenModules[mount.ModuleKey] = struct{}{}
		mountedProgram := mount.Program
		if !mountedProgram.Available() {
			return nil, false
		}
		catalog, catalogOK := programcatalog.CatalogID(mountedProgram.Program.SchemaID)
		valuesCount, valuesPublished := programschema.ValuesFamily().Count(&mountedProgram.Program.Frozen, catalog)
		heapIndexCount, heapIndexesPublished := heapindex.Family().Count(&mountedProgram.Program.Frozen, catalog)
		if !catalogOK || !valuesPublished || !heapIndexesPublished {
			return nil, false
		}
		for i := 0; i < valuesCount; i++ {
			row, ok := programschema.ValuesFamily().At(&mountedProgram.Program.Frozen, catalog, i)
			if !ok {
				return nil, false
			}
			if row.MemberCount()+1 > maximum {
				maximum = row.MemberCount() + 1
			}
		}
		for i := 0; i < heapIndexCount; i++ {
			row, ok := heapindex.Family().At(&mountedProgram.Program.Frozen, catalog, i)
			if !ok {
				return nil, false
			}
			if _, position, write := row.Values(); write && position > maximum {
				maximum = position
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
		endpointIndex: make(map[SemanticSource]Endpoint), artifactValues: make(map[artifactValuesKey]uint32), artifactCalls: make(map[artifactCallKey]uint32), inputSelectors: make(map[inputSelectorKey]InputSelector),
	}
	tailIndex := make(map[artifactValuesKey]uint32)
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
		if !sealMountedArtifactValues(sealed, mount, tailIndex) || !sealMountedArtifactCalls(sealed, authority, mount, tailIndex) {
			return nil, false
		}
	}
	if !sealed.sealSourceValues() {
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
func sealInputSelectors(state *schema, contract *contract.Contract) bool {
	if state == nil || state.owner == nil || contract == nil || !contract.ContentID().Available() || state.inputSelectors == nil {
		return false
	}
	opaque, opaqueOK := contract.Operations.Opaque()
	if !opaqueOK {
		return false
	}
	add := func(operation vocabulary.Operation, source vocabulary.InputSource, selector InputSelector) bool {
		key := inputSelectorKey{operation: operation, source: source}
		if _, duplicate := state.inputSelectors[key]; duplicate || !selector.valid() || selector.schema != state {
			return false
		}
		state.inputSelectors[key] = selector
		return true
	}
	for index := 0; index < contract.Operations.OperationCount(); index++ {
		operation, operationOK := contract.Operations.OperationAt(index)
		if !operationOK {
			return false
		}
		fixed := contract.Operations.ValueFormalCount(operation)
		input, inputOK := contract.Operations.Input(operation)
		if !inputOK || fixed != contract.Operations.ValuesCount(input) {
			return false
		}
		for formal := 0; formal < fixed; formal++ {
			offset, offsetOK := offsetForUint64(state.owner, uint64(formal))
			table, tableOK := tableIndexForOffset(offset)
			selector := InputSelector{schema: state, kind: inputSelectionScalar, table: table, start: formal, sealed: true}
			if !offsetOK || !tableOK || !add(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(formal)}, selector) {
				return false
			}
		}
		tail, variable, tailOK := contract.Operations.ValuesTail(input)
		if !tailOK {
			return false
		}
		if tail == vocabulary.ValuesVariable {
			selector := InputSelector{schema: state, kind: inputSelectionTail, start: fixed, sealed: true}
			if !add(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: uint32(variable)}, selector) {
				return false
			}
		}
		if operation == opaque {
			selector := InputSelector{schema: state, kind: inputSelectionWhole, start: 0, sealed: true}
			if !add(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}, selector) {
				return false
			}
		}
	}
	return true
}

// mountedArtifactMatchesLink authenticates one caller-supplied artifact
// against the exact owner-fenced Project mount position. It reads only the
// canonical ModuleKey and ProgramID; it never opens the mounted Program.
func mountedArtifactMatchesLink(source *link.Link, index int, mount programmount.MountedArtifact) bool {
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
	return shardOK && moduleOK && programOK && module == mount.ModuleKey && program == mount.ProgramID
}

func sealMountedArtifactValues(schema *Schema, mount programmount.MountedArtifact, tailIndex map[artifactValuesKey]uint32) bool {
	if tailIndex == nil {
		return false
	}
	state := schema.state
	program := mount.Program.Program
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	valuesCount, valuesPublished := programschema.ValuesFamily().Count(&program.Frozen, catalog)
	if !program.Available() || !catalogOK || !valuesPublished {
		return false
	}
	for i := 0; i < valuesCount; i++ {
		row, ok := programschema.ValuesFamily().At(&program.Frozen, catalog, i)
		if !ok {
			return false
		}
		key := artifactValuesKey{mount.ModuleKey, row.ID()}
		if _, duplicate := state.artifactValues[key]; duplicate {
			return false
		}
		rootID := mountedArtifactRootID(rootValues, mount.ModuleKey, row.ID())
		root, _, ok := state.addRootWithID(state.owner.classes, rootRow{kind: rootValues, id: rootID})
		if !ok {
			return false
		}
		out := valuesRow{root: root, moduleKey: mount.ModuleKey, occurrenceID: row.ID()}
		memberOffset, memberCount, membersOK := row.MemberSpan()
		if !membersOK {
			return false
		}
		for member := 0; member < int(memberCount); member++ {
			m, memberOK := programschema.ValuesMemberFamily().At(&program.Frozen, catalog, int(memberOffset)+member)
			endpoint, endpointOK := endpointForMountedSemantic(state, mount.ModuleKey, m.ID())
			if !memberOK || !endpointOK {
				return false
			}
			out.fixed = append(out.fixed, endpoint)
		}
		if tail, open := row.Tail(); open {
			tailRoot, tailOK := sealMountedArtifactTail(schema, mount.ModuleKey, tail, tailIndex)
			if !tailOK {
				return false
			}
			out.tailRoot, out.hasTail = tailRoot, true
		}
		state.artifactValues[key] = uint32(len(state.values))
		state.values = append(state.values, out)
		state.roots[root].sourceIndex = uint32(len(state.values) - 1)
	}
	return true
}

func sealMountedArtifactTail(schema *Schema, module identity.ContentID, tail programschema.ValuesTail, tailIndex map[artifactValuesKey]uint32) (uint32, bool) {
	if tailIndex == nil {
		return 0, false
	}
	state := schema.state
	id := tail.ID()
	endpoint, endpointOK := endpointForMountedSemantic(state, module, id)
	if !tail.Present() || !id.Available() || !endpointOK {
		return 0, false
	}
	key := artifactValuesKey{module, id}
	if index, exists := tailIndex[key]; exists {
		if uint64(index) >= uint64(len(state.tails)) {
			return 0, false
		}
		row := state.tails[index]
		if uint64(row.root) >= uint64(len(state.roots)) {
			return 0, false
		}
		port := state.roots[row.root].port
		return row.root, port.valid() && row.valueID == id
	}
	kind := TailProducerInvalid
	switch tail.Kind() {
	case programschema.ValuesTailCall:
		kind = TailProducerCall
	case programschema.ValuesTailVararg:
		kind = TailProducerVararg
	default:
		return 0, false
	}
	root, _, ok := state.addRootWithID(state.owner.classes, rootRow{kind: rootTail, id: mountedArtifactRootID(rootTail, module, id)})
	if !ok || !endpoint.valid() {
		return 0, false
	}
	index := uint32(len(state.tails))
	tailIndex[key] = index
	state.tails = append(state.tails, tailRow{root: root, moduleKey: module, valueID: id, kind: kind})
	state.roots[root].sourceIndex = index
	return root, true
}

func sealMountedArtifactCalls(schema *Schema, authority *static.Authority, mount programmount.MountedArtifact, tailIndex map[artifactValuesKey]uint32) bool {
	if tailIndex == nil {
		return false
	}
	state, program := schema.state, mount.Program
	if !mount.Available() || !program.Available() {
		return false
	}
	callCount, callsOK := program.CallCount()
	if !callsOK {
		return false
	}
	for i := 0; i < callCount; i++ {
		row, rowOK := program.CallAt(i)
		if !rowOK {
			return false
		}
		key := artifactCallKey{mount.ModuleKey, row.ID()}
		if _, duplicate := state.artifactCalls[key]; duplicate {
			return false
		}
		typeArguments, typeArgumentsOK := authority.MountedCallTypeArgumentSequence(mount.ModuleKey, row.TypeArgumentsID())
		if !typeArgumentsOK {
			return false
		}
		rootID := mountedCallRootID(mount.ModuleKey, row.FormalID())
		root, _, rootOK := state.addRootWithID(state.owner.classes, rootRow{kind: rootCall, id: rootID})
		if !rootOK {
			return false
		}
		out := callRow{root: root, occurrenceID: row.ID(), moduleKey: mount.ModuleKey, formalID: row.FormalID(), typeArguments: typeArguments}
		if row.Form() == programschema.CallFormMethod {
			receiverID, receiverOK := row.ReceiverID()
			endpoint, endpointOK := endpointForMountedSemantic(state, mount.ModuleKey, receiverID)
			if !receiverOK || !endpointOK {
				return false
			}
			out.fixed = append(out.fixed, endpoint)
		}
		for j := 0; j < row.ArgumentCount(); j++ {
			argument, argumentOK := program.CallArgumentFor(i, j)
			endpoint, endpointOK := endpointForMountedSemantic(state, mount.ModuleKey, argument.ValueID())
			if !argumentOK || !endpointOK {
				return false
			}
			out.fixed = append(out.fixed, endpoint)
		}
		if tailID, open := row.TailID(); open {
			tailRowIndex, tailOK := tailIndex[artifactValuesKey{mount.ModuleKey, tailID}]
			if !tailOK || uint64(tailRowIndex) >= uint64(len(state.tails)) {
				return false
			}
			tail := state.tails[tailRowIndex]
			if _, tailPortOK := state.tailPort(tail.root); !tailPortOK || tail.moduleKey != mount.ModuleKey || tail.valueID != tailID {
				return false
			}
			out.tailRoot, out.hasTail = tail.root, true
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
	return index, port, true
}

func mountedCallRootID(module, formal identity.ContentID) identity.ContentID {
	return mountedArtifactRootID(rootCall, module, formal)
}

func sealMountedSemanticEndpoints(state *schema, mount programmount.MountedArtifact) bool {
	if state == nil || !mount.Available() {
		return false
	}
	add := func(id identity.ContentID) bool {
		source, sourceOK := newSemanticSource(mount.ModuleKey, id)
		if !sourceOK {
			return false
		}
		if endpoint, exists := state.endpointIndex[source]; exists {
			return endpoint.valid() && endpoint.owner == state.owner && endpoint.index != 0 &&
				uint64(endpoint.index) <= uint64(len(state.endpointSources)) && state.endpointSources[endpoint.index-1] == source
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
		return true
	}
	program := mount.Program
	if !program.Available() {
		return false
	}
	canonical := program.Program
	catalog, catalogOK := programcatalog.CatalogID(canonical.SchemaID)
	valuesCount, valuesPublished := programschema.ValuesFamily().Count(&canonical.Frozen, catalog)
	if !catalogOK || !valuesPublished {
		return false
	}
	for i := 0; i < valuesCount; i++ {
		row, ok := programschema.ValuesFamily().At(&canonical.Frozen, catalog, i)
		if !ok {
			return false
		}
		memberOffset, memberCount, membersOK := row.MemberSpan()
		if !membersOK {
			return false
		}
		for j := 0; j < int(memberCount); j++ {
			member, ok := programschema.ValuesMemberFamily().At(&canonical.Frozen, catalog, int(memberOffset)+j)
			if !ok || !add(member.ID()) {
				return false
			}
		}
		if tail, open := row.Tail(); open && !add(tail.ID()) {
			return false
		}
	}
	callCount, callsOK := program.CallCount()
	if !callsOK {
		return false
	}
	for i := 0; i < callCount; i++ {
		row, ok := program.CallAt(i)
		if !ok {
			return false
		}
		if receiver, present := row.ReceiverID(); present && !add(receiver) {
			return false
		}
		for j := 0; j < row.ArgumentCount(); j++ {
			argument, argumentOK := program.CallArgumentFor(i, j)
			if !argumentOK || !add(argument.ValueID()) {
				return false
			}
		}
	}
	return true
}
