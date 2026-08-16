package boundary

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

const seedRelationVersion = 1

// sealSeeds admits only the semantic external families.  Source path/trie
// geometry is intentionally absent: it belongs to neither Target nor this
// finite Boundary vocabulary.
func sealSeeds(a *authority, requests []EndpointRequest) error {
	if a == nil || a.project == nil || a.target == nil || a.seedTable != nil {
		return errors.New("link/boundary: invalid seed authority")
	}
	mounts := a.project.Mounts()
	if mounts.Count() < 0 || uint64(mounts.Count()) > uint64(^uint32(0)) {
		return errors.New("link/boundary: seed mount overflow")
	}
	table := &seedTable{
		operation:     make([]uint32, a.target.OperationCount()),
		loaderByMount: make([]uint32, mounts.Count()),
	}
	appendRow := func(row seedRow) (uint32, error) {
		if uint64(len(table.rows)) >= uint64(^uint32(0)) {
			return 0, errors.New("link/boundary: seed handle overflow")
		}
		ordinal := uint32(len(table.rows))
		table.rows = append(table.rows, row)
		return ordinal, nil
	}
	for index := 0; index < a.target.OperationCount(); index++ {
		op, ok := a.target.OperationAt(index)
		if !ok || op == 0 {
			return errors.New("link/boundary: malformed Target operation")
		}
		if op == a.require {
			continue
		}
		ordinal, err := appendRow(seedRow{kind: seedOperation, op: op})
		if err != nil {
			return err
		}
		table.operation[index] = ordinal + 1
	}
	if a.require != 0 {
		for index := 0; index < mounts.Count(); index++ {
			shard, ok := mounts.At(index)
			if !ok {
				return errors.New("link/boundary: malformed Project mount")
			}
			if _, ok := mounts.Program(shard); !ok {
				return errors.New("link/boundary: unavailable mounted Program")
			}
			ordinal, err := appendRow(seedRow{kind: seedLoader, op: a.require, mount: uint32(index + 1)})
			if err != nil {
				return err
			}
			table.loaderByMount[index] = ordinal + 1
		}
	}
	denied, err := bootstrapDeniedInitialValues(a.target)
	if err != nil {
		return err
	}
	table.deniedStart = uint32(len(table.rows))
	for _, value := range denied {
		if _, err := appendRow(seedRow{kind: seedDeniedBootstrap, denied: value}); err != nil {
			return err
		}
	}
	table.deniedCount = uint32(len(denied))
	endpointDrafts, err := canonicalEndpointRequests(a.target, requests)
	if err != nil {
		return err
	}
	table.endpoints = make([]endpointRow, 0, len(endpointDrafts))
	table.requests = make([]endpointRequestRow, 0, len(endpointDrafts))
	for endpointIndex, item := range endpointDrafts {
		if uint64(endpointIndex) >= uint64(^uint32(0)) {
			return errors.New("link/boundary: endpoint handle overflow")
		}
		ordinal, err := appendRow(seedRow{kind: seedEndpoint, op: item.op, endpoint: uint32(endpointIndex + 1)})
		if err != nil {
			return err
		}
		table.endpoints = append(table.endpoints, endpointRow{seed: ordinal, op: item.op})
		table.requests = append(table.requests, endpointRequestRow{identity: item.identity, binding: item.binding})
	}
	table.relation = seedRelationID(a, denied)
	table.endpointRelation = endpointRelationID(a.target, endpointDrafts)
	if !table.relation.Available() || !table.endpointRelation.Available() {
		return errors.New("link/boundary: unavailable seed relation identity")
	}
	if err := sealEndpointIDs(a, table); err != nil {
		return err
	}
	a.seedTable = table
	return nil
}

func sealEndpointIDs(a *authority, table *seedTable) error {
	if a == nil || a.target == nil || table == nil || len(table.endpoints) != len(table.requests) || len(table.endpoints) > int(^uint32(0)) {
		return errors.New("link/boundary: invalid endpoint identity table")
	}
	table.endpointIDs = make([]endpointIDRow, len(table.endpoints))
	for ordinal, endpoint := range table.endpoints {
		id, ok := endpointLocalID(a.target, endpoint.op, table.requests[ordinal])
		if !ok {
			return errors.New("link/boundary: unavailable endpoint identity")
		}
		table.endpointIDs[ordinal] = endpointIDRow{id: id, ordinal: uint32(ordinal)}
	}
	sort.Slice(table.endpointIDs, func(left, right int) bool {
		return bytes.Compare(table.endpointIDs[left].id[:], table.endpointIDs[right].id[:]) < 0
	})
	for index := 1; index < len(table.endpointIDs); index++ {
		if table.endpointIDs[index-1].id == table.endpointIDs[index].id {
			return errors.New("link/boundary: duplicate endpoint identity")
		}
	}
	return nil
}

type endpointDraft struct {
	identity string
	op       target.Operation
	binding  target.BindingSpec
}

func canonicalEndpointRequests(contract *target.Contract, requests []EndpointRequest) ([]endpointDraft, error) {
	result := make([]endpointDraft, len(requests))
	for index, request := range requests {
		if request.Identity == "" || request.Binding.Namespace != target.BindingProvider {
			return nil, errors.New("link/boundary: endpoint needs a provider binding")
		}
		op, ok := contract.Lookup(request.Binding)
		if !ok || op == 0 {
			return nil, errors.New("link/boundary: endpoint has unknown provider binding")
		}
		result[index] = endpointDraft{identity: request.Identity, op: op, binding: cloneBinding(request.Binding)}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].identity < result[right].identity })
	for index := 1; index < len(result); index++ {
		if result[index-1].identity == result[index].identity {
			return nil, errors.New("link/boundary: duplicate endpoint identity")
		}
	}
	return result, nil
}

func cloneBinding(binding target.BindingSpec) target.BindingSpec {
	return target.BindingSpec{Namespace: binding.Namespace, Owner: append([]string(nil), binding.Owner...), Member: append([]string(nil), binding.Member...)}
}

func bootstrapDeniedInitialValues(contract *target.Contract) ([]target.InitialValue, error) {
	if contract == nil {
		return nil, errors.New("link/boundary: unavailable bootstrap callable authority")
	}
	seen := make(map[target.InitialValue]struct{})
	add := func(value target.InitialValue) error {
		kind, ok := contract.InitialValueKind(value)
		if !ok {
			return errors.New("link/boundary: malformed Target initial value")
		}
		if kind == target.InitialValueDeniedOperation {
			seen[value] = struct{}{}
		}
		return nil
	}
	for index := 0; index < contract.InitialRootCount(); index++ {
		root, ok := contract.InitialRootAt(index)
		shape, shapeOK := contract.InitialRootBootShape(root)
		value, valueOK := contract.BootShapeValue(shape)
		if !ok || !shapeOK || !valueOK {
			return nil, errors.New("link/boundary: malformed Target initial root")
		}
		if err := add(value); err != nil {
			return nil, err
		}
	}
	for index := 0; index < contract.InitialEntryCount(); index++ {
		_, _, value, _, ok := contract.InitialEntryAt(index)
		if !ok {
			return nil, errors.New("link/boundary: malformed Target initial entry")
		}
		if err := add(value); err != nil {
			return nil, err
		}
	}
	for index := 0; index < contract.InitialBindingCount(); index++ {
		_, _, value, _, _, ok := contract.InitialBindingAt(index)
		if !ok {
			return nil, errors.New("link/boundary: malformed Target initial binding")
		}
		if err := add(value); err != nil {
			return nil, err
		}
	}
	result := make([]target.InitialValue, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result, nil
}

func seedRelationID(a *authority, denied []target.InitialValue) (id identity.ContentID) {
	if a == nil || a.target == nil || !a.target.ContentID().Available() {
		return id
	}
	targetID := a.target.ContentID()
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/boundary/seeds", seedRelationVersion) != nil ||
		writer.Record(1) != nil || writer.Bytes(targetID[:]) != nil ||
		writer.Count(uint64(a.project.Mounts().Count())) != nil {
		return id
	}
	for index := 0; index < a.project.Mounts().Count(); index++ {
		shard, ok := a.project.Mounts().At(index)
		if !ok {
			return id
		}
		name, nameOK := a.project.Mounts().Name(shard)
		program, programOK := a.project.Mounts().Program(shard)
		if !nameOK || !programOK || program == nil || writer.String(name) != nil {
			return id
		}
		programID := program.ContentID()
		if !programID.Available() || writer.Bytes(programID[:]) != nil {
			return id
		}
	}
	if writer.Count(uint64(len(denied))) != nil {
		return id
	}
	for _, value := range denied {
		if writer.Uint(uint64(value)) != nil {
			return id
		}
	}
	if writer.Finish() != nil {
		return id
	}
	sum := h.Sum(id[:0])
	if len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func endpointRelationID(contract *target.Contract, endpoints []endpointDraft) (id identity.ContentID) {
	if contract == nil {
		return id
	}
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/boundary/endpoints", seedRelationVersion) != nil || writer.Record(1) != nil ||
		writer.Count(uint64(len(endpoints))) != nil {
		return id
	}
	for _, endpoint := range endpoints {
		opID, ok := contract.OperationContentID(endpoint.op)
		if !ok || !opID.Available() || endpoint.identity == "" || writer.String(endpoint.identity) != nil || writer.Bytes(opID[:]) != nil || writeBinding(&writer, endpoint.binding) != nil {
			return id
		}
	}
	if writer.Finish() != nil {
		return id
	}
	sum := h.Sum(id[:0])
	if len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func writeBinding(writer *framing.Writer, binding target.BindingSpec) error {
	if writer == nil || binding.Namespace != target.BindingProvider || writer.Uint(uint64(binding.Namespace)) != nil || writer.Count(uint64(len(binding.Owner))) != nil {
		return errors.New("invalid endpoint binding")
	}
	for _, segment := range binding.Owner {
		if segment == "" || writer.String(segment) != nil {
			return errors.New("invalid endpoint owner")
		}
	}
	if writer.Count(uint64(len(binding.Member))) != nil {
		return errors.New("invalid endpoint binding")
	}
	for _, segment := range binding.Member {
		if segment == "" || writer.String(segment) != nil {
			return errors.New("invalid endpoint member")
		}
	}
	return nil
}

func endpointLocalID(contract *target.Contract, op target.Operation, request endpointRequestRow) (id identity.ContentID, ok bool) {
	if contract == nil || op == 0 || request.identity == "" {
		return id, false
	}
	opID, opOK := contract.OperationContentID(op)
	if !opOK || !opID.Available() {
		return id, false
	}
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/boundary/endpoint", seedRelationVersion) != nil || writer.Record(1) != nil || writer.String(request.identity) != nil || writer.Bytes(opID[:]) != nil || writeBinding(&writer, request.binding) != nil || writer.Finish() != nil {
		return id, false
	}
	sum := h.Sum(id[:0])
	return id, len(sum) == len(id)
}

func seededID(domain string, relation identity.ContentID, ordinal uint32) (id identity.ContentID, ok bool) {
	if !relation.Available() {
		return id, false
	}
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, domain, seedRelationVersion) != nil || writer.Record(1) != nil || writer.Bytes(relation[:]) != nil || writer.Uint(uint64(ordinal)) != nil || writer.Finish() != nil {
		return id, false
	}
	sum := h.Sum(id[:0])
	return id, len(sum) == len(id)
}

func operationSeedID(contract *target.Contract, op target.Operation) (identity.ContentID, bool) {
	if contract == nil || op == 0 {
		return identity.ContentID{}, false
	}
	opID, ok := contract.OperationContentID(op)
	if !ok {
		return identity.ContentID{}, false
	}
	return seededID("program/link/boundary/seed-operation", opID, 0)
}

func loaderSeedID(a *authority, mount uint32, op target.Operation) (id identity.ContentID, ok bool) {
	if a == nil || a.target == nil || a.project == nil || mount == 0 || op == 0 {
		return id, false
	}
	shard, valid := a.project.Mounts().At(int(mount) - 1)
	if !valid {
		return id, false
	}
	name, nameOK := a.project.Mounts().Name(shard)
	program, programOK := a.project.Mounts().Program(shard)
	if !nameOK || !programOK || name == "" || program == nil {
		return id, false
	}
	opID, opOK := a.target.OperationContentID(op)
	programID := program.ContentID()
	if !opOK || !opID.Available() || !programID.Available() {
		return id, false
	}
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/boundary/seed-loader", seedRelationVersion) != nil || writer.Record(1) != nil || writer.Bytes(opID[:]) != nil || writer.String(name) != nil || writer.Bytes(programID[:]) != nil || writer.Finish() != nil {
		return id, false
	}
	sum := h.Sum(id[:0])
	return id, len(sum) == len(id)
}

func deniedSeedID(contract *target.Contract, value target.InitialValue) (identity.ContentID, bool) {
	if contract == nil || value == 0 {
		return identity.ContentID{}, false
	}
	kind, ok := contract.InitialValueKind(value)
	if !ok || kind != target.InitialValueDeniedOperation {
		return identity.ContentID{}, false
	}
	namespace, ok := contract.InitialValueDeniedNamespace(value)
	if !ok {
		return identity.ContentID{}, false
	}
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/boundary/seed-denied", seedRelationVersion) != nil || writer.Record(1) != nil || writer.Uint(uint64(namespace)) != nil || writer.Count(uint64(contract.InitialValueDeniedOwnerCount(value))) != nil {
		return identity.ContentID{}, false
	}
	for index := 0; index < contract.InitialValueDeniedOwnerCount(value); index++ {
		segment, ok := contract.InitialValueDeniedOwnerAt(value, index)
		if !ok || segment == "" || writer.String(segment) != nil {
			return identity.ContentID{}, false
		}
	}
	if writer.Count(uint64(contract.InitialValueDeniedMemberCount(value))) != nil {
		return identity.ContentID{}, false
	}
	for index := 0; index < contract.InitialValueDeniedMemberCount(value); index++ {
		segment, ok := contract.InitialValueDeniedMemberAt(value, index)
		if !ok || segment == "" || writer.String(segment) != nil {
			return identity.ContentID{}, false
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	sum := h.Sum(id[:0])
	return id, len(sum) == len(id)
}

func (s Seeds) live() bool {
	return s.component != nil && s.component.authority != nil && s.component.authority.seedTable != nil
}
func (s Seeds) valid(seed Seed) bool {
	return s.live() && seed.component == s.component && uint64(seed.ordinal) < uint64(len(s.component.authority.seedTable.rows))
}

func (s Seeds) Count() int {
	if !s.live() {
		return 0
	}
	return len(s.component.authority.seedTable.rows)
}
func (s Seeds) At(index int) (Seed, bool) {
	if !s.live() || index < 0 || index >= len(s.component.authority.seedTable.rows) {
		return Seed{}, false
	}
	return Seed{component: s.component, ordinal: uint32(index)}, true
}
func (s Seeds) ForOperation(op target.Operation) (Seed, bool) {
	if !s.live() || op == 0 || uint64(op) > uint64(len(s.component.authority.seedTable.operation)) {
		return Seed{}, false
	}
	ordinal := s.component.authority.seedTable.operation[op-1]
	if ordinal == 0 {
		return Seed{}, false
	}
	return Seed{component: s.component, ordinal: ordinal - 1}, true
}
func (s Seeds) ScopedLoader(shard linkproject.Shard) (Seed, bool) {
	if !s.live() {
		return Seed{}, false
	}
	index, ok := s.component.authority.project.Mounts().Index(shard)
	if !ok || index < 0 || index >= len(s.component.authority.seedTable.loaderByMount) {
		return Seed{}, false
	}
	ordinal := s.component.authority.seedTable.loaderByMount[index]
	if ordinal == 0 {
		return Seed{}, false
	}
	return Seed{component: s.component, ordinal: ordinal - 1}, true
}
func (s Seeds) Operation(seed Seed) (target.Operation, bool) {
	if !s.valid(seed) {
		return 0, false
	}
	row := s.component.authority.seedTable.rows[seed.ordinal]
	return row.op, row.kind != seedDeniedBootstrap && row.op != 0
}
func (s Seeds) Loader(seed Seed) (linkproject.Shard, bool) {
	if !s.valid(seed) {
		return linkproject.Shard{}, false
	}
	row := s.component.authority.seedTable.rows[seed.ordinal]
	if row.kind != seedLoader || row.mount == 0 {
		return linkproject.Shard{}, false
	}
	return s.component.authority.project.Mounts().At(int(row.mount) - 1)
}
func (s Seeds) BootstrapCallable(value target.InitialValue) (Seed, CallableDisposition, bool) {
	if !s.live() || value == 0 {
		return Seed{}, CallableInvalid, false
	}
	kind, ok := s.component.authority.target.InitialValueKind(value)
	if !ok {
		return Seed{}, CallableInvalid, false
	}
	if kind == target.InitialValueOperation {
		op, ok := s.component.authority.target.InitialValueOperation(value)
		if !ok {
			return Seed{}, CallableInvalid, false
		}
		seed, ok := s.ForOperation(op)
		if !ok {
			return Seed{}, CallableInvalid, false
		}
		return seed, CallableAdmittedOperation, true
	}
	if kind != target.InitialValueDeniedOperation {
		return Seed{}, CallableInvalid, false
	}
	table := s.component.authority.seedTable
	start, count := int(table.deniedStart), int(table.deniedCount)
	if start < 0 || count < 0 || start > len(table.rows) || count > len(table.rows)-start {
		return Seed{}, CallableInvalid, false
	}
	offset := sort.Search(count, func(index int) bool { return table.rows[start+index].denied >= value })
	if offset < count && table.rows[start+offset].kind == seedDeniedBootstrap && table.rows[start+offset].denied == value {
		return Seed{component: s.component, ordinal: uint32(start + offset)}, CallableDeniedTarget, true
	}
	return Seed{}, CallableInvalid, false
}
func (s Seeds) CallableDisposition(seed Seed) (CallableDisposition, target.Operation, target.InitialValue, bool) {
	if !s.valid(seed) {
		return CallableInvalid, 0, 0, false
	}
	row := s.component.authority.seedTable.rows[seed.ordinal]
	if row.kind == seedDeniedBootstrap {
		return CallableDeniedTarget, 0, row.denied, true
	}
	if row.op != 0 {
		return CallableAdmittedOperation, row.op, 0, true
	}
	return CallableInvalid, 0, 0, false
}
func (s Seeds) ID(seed Seed) (identity.ContentID, bool) {
	if !s.valid(seed) {
		return identity.ContentID{}, false
	}
	row := s.component.authority.seedTable.rows[seed.ordinal]
	switch row.kind {
	case seedOperation:
		return operationSeedID(s.component.authority.target, row.op)
	case seedLoader:
		return loaderSeedID(s.component.authority, row.mount, row.op)
	case seedDeniedBootstrap:
		return deniedSeedID(s.component.authority.target, row.denied)
	case seedEndpoint:
		return endpointSeedID(s.component.authority, row.endpoint)
	default:
		return identity.ContentID{}, false
	}
}
func (s Seeds) Compare(left, right Seed) (int, bool) {
	if !s.valid(left) || !s.valid(right) {
		return 0, false
	}
	if left.ordinal < right.ordinal {
		return -1, true
	}
	if left.ordinal > right.ordinal {
		return 1, true
	}
	return 0, true
}

func (e Endpoints) live() bool {
	return e.component != nil && e.component.authority != nil && e.component.authority.seedTable != nil
}
func (e Endpoints) valid(endpoint Endpoint) bool {
	return e.live() && endpoint.component == e.component && uint64(endpoint.ordinal) < uint64(len(e.component.authority.seedTable.endpoints))
}
func (e Endpoints) Count() int {
	if !e.live() {
		return 0
	}
	return len(e.component.authority.seedTable.endpoints)
}
func (e Endpoints) At(index int) (Endpoint, bool) {
	if !e.live() || index < 0 || index >= len(e.component.authority.seedTable.endpoints) {
		return Endpoint{}, false
	}
	return Endpoint{component: e.component, ordinal: uint32(index)}, true
}
func (e Endpoints) Operation(endpoint Endpoint) (target.Operation, bool) {
	if !e.valid(endpoint) {
		return 0, false
	}
	row := e.component.authority.seedTable.endpoints[endpoint.ordinal]
	return row.op, row.op != 0
}
func (e Endpoints) Seed(endpoint Endpoint) (Seed, bool) {
	if !e.valid(endpoint) {
		return Seed{}, false
	}
	row := e.component.authority.seedTable.endpoints[endpoint.ordinal]
	if uint64(row.seed) >= uint64(len(e.component.authority.seedTable.rows)) {
		return Seed{}, false
	}
	return Seed{component: e.component, ordinal: row.seed}, true
}
func (e Endpoints) ID(endpoint Endpoint) (identity.ContentID, bool) {
	if !e.valid(endpoint) {
		return identity.ContentID{}, false
	}
	return endpointSeedID(e.component.authority, endpoint.ordinal+1)
}

func endpointSeedID(a *authority, endpoint uint32) (identity.ContentID, bool) {
	if a == nil || a.seedTable == nil || endpoint == 0 || uint64(endpoint) > uint64(len(a.seedTable.endpoints)) {
		return identity.ContentID{}, false
	}
	ordinal := endpoint - 1
	return endpointLocalID(a.target, a.seedTable.endpoints[ordinal].op, a.seedTable.requests[ordinal])
}

// FindID rebinds one portable nominal Endpoint identity through this exact
// finalized Boundary authority. The sealed index is sorted and map-free.
func (e Endpoints) FindID(id identity.ContentID) (Endpoint, bool) {
	if !e.live() || !id.Available() {
		return Endpoint{}, false
	}
	rows := e.component.authority.seedTable.endpointIDs
	index := sort.Search(len(rows), func(index int) bool { return bytes.Compare(rows[index].id[:], id[:]) >= 0 })
	if index >= len(rows) || rows[index].id != id || uint64(rows[index].ordinal) >= uint64(len(e.component.authority.seedTable.endpoints)) {
		return Endpoint{}, false
	}
	return Endpoint{component: e.component, ordinal: rows[index].ordinal}, true
}

func (r EndpointRequests) live() bool {
	return r.component != nil && r.component.authority != nil && r.component.authority.seedTable != nil
}

// Count reports authored endpoint rows in canonical identity order.
func (r EndpointRequests) Count() int {
	if !r.live() {
		return 0
	}
	return len(r.component.authority.seedTable.requests)
}

// At returns a defensive copy of one authored endpoint request. The returned
// Binding cannot mutate the sealed replay authority.
func (r EndpointRequests) At(index int) (EndpointRequest, bool) {
	if !r.live() || index < 0 || index >= len(r.component.authority.seedTable.requests) {
		return EndpointRequest{}, false
	}
	row := r.component.authority.seedTable.requests[index]
	return EndpointRequest{Identity: row.identity, Binding: cloneBinding(row.binding)}, true
}
