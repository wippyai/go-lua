package link

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	"github.com/wippyai/go-lua/program/target"
)

// dependencyKind and dependencyRow are construction-local digest witnesses.
// They are rebuilt from the three owning child authorities while Link is
// sealed, then discarded; no mixed dependency relation is retained by Link.
type dependencyKind uint8

const (
	dependencyInvalid dependencyKind = iota
	dependencyProvider
	dependencySchema
	dependencyBindingWorld
)

type dependencyRow struct {
	kind      dependencyKind
	id        keyspace.ContentID
	operation target.Operation
}

// deriveDependencyRows builds the exact sorted dependency witness used by the
// Link identity. Boundary, Static, and Module remain the sole authorities for
// their respective rows; this slice exists only for the duration of sealing.
func deriveDependencyRows(boundary *linkboundary.Component, mounts linkproject.Mounts, static linkstatic.Cold, module linkmodule.Cold) ([]dependencyRow, error) {
	if boundary == nil || static.SchemaContentCount() != mounts.Count() {
		return nil, errors.New("link: dependency authorities unavailable")
	}
	contract, ok := boundary.Target()
	if !ok || contract == nil || !contract.ContentID().Available() {
		return nil, errors.New("link: target dependency authority unavailable")
	}

	endpoints := boundary.Endpoints()
	rows := make([]dependencyRow, 0, endpoints.Count()+static.SchemaContentCount()+1)
	seenProvider := make(map[target.Operation]struct{}, endpoints.Count())
	for index := 0; index < endpoints.Count(); index++ {
		endpoint, endpointOK := endpoints.At(index)
		operation, operationOK := endpoints.Operation(endpoint)
		if !endpointOK || !operationOK || operation == 0 || !validTargetOperation(contract, operation) {
			return nil, errors.New("link: malformed provider dependency authority")
		}
		if _, duplicate := seenProvider[operation]; duplicate {
			continue
		}
		seenProvider[operation] = struct{}{}
		id := providerDependencyID(contract.ContentID(), operation)
		if !id.Available() {
			return nil, errors.New("link: unavailable provider dependency identity")
		}
		rows = append(rows, dependencyRow{kind: dependencyProvider, id: id, operation: operation})
	}

	seenSchema := make(map[keyspace.ContentID]struct{}, static.SchemaContentCount())
	for index := 0; index < static.SchemaContentCount(); index++ {
		content, contentOK := static.SchemaContentAt(index)
		if !contentOK || !content.Available() {
			return nil, errors.New("link: malformed schema dependency authority")
		}
		if _, duplicate := seenSchema[content]; duplicate {
			continue
		}
		seenSchema[content] = struct{}{}
		rows = append(rows, dependencyRow{kind: dependencySchema, id: content})
	}

	world := module.ContentID()
	if !world.Available() {
		return nil, errors.New("link: unavailable binding-world dependency identity")
	}
	rows = append(rows, dependencyRow{kind: dependencyBindingWorld, id: world})
	sort.Slice(rows, func(left, right int) bool { return compareDependencyRow(rows[left], rows[right]) < 0 })
	for index, row := range rows {
		if !validDependencyRow(contract, row) || (index != 0 && compareDependencyRow(rows[index-1], row) >= 0) {
			return nil, errors.New("link: malformed or duplicate dependency witness")
		}
	}
	return rows, nil
}

func validDependencyKind(kind dependencyKind) bool {
	return kind == dependencyProvider || kind == dependencySchema || kind == dependencyBindingWorld
}

func validDependencyRow(contract *target.Contract, row dependencyRow) bool {
	if !validDependencyKind(row.kind) || !row.id.Available() {
		return false
	}
	switch row.kind {
	case dependencyProvider:
		return row.operation != 0 && validTargetOperation(contract, row.operation)
	case dependencySchema, dependencyBindingWorld:
		return row.operation == 0
	default:
		return false
	}
}

func validTargetOperation(contract *target.Contract, operation target.Operation) bool {
	if contract == nil || operation == 0 {
		return false
	}
	got, ok := contract.OperationAt(int(operation - 1))
	return ok && got == operation
}

func compareDependencyRow(left, right dependencyRow) int {
	if left.kind != right.kind {
		if left.kind < right.kind {
			return -1
		}
		return 1
	}
	if order := bytes.Compare(left.id[:], right.id[:]); order != 0 {
		return order
	}
	if left.operation < right.operation {
		return -1
	}
	if left.operation > right.operation {
		return 1
	}
	return 0
}

func providerDependencyID(targetID keyspace.ContentID, operation target.Operation) (id keyspace.ContentID) {
	if !targetID.Available() || operation == 0 {
		return keyspace.ContentID{}
	}
	hash := sha256.New()
	var writer canonical.Writer
	if writer.Reset(hash, "program/link/provider-dependency", 1) != nil ||
		writer.Record(1) != nil || writer.Bytes(targetID[:]) != nil ||
		writer.Uint(uint64(operation)) != nil || writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}
