// Package workspace owns the immutable compiler products retained for one
// caller-owned analysis workspace. It has no process-global state: closing an
// Artifacts directory releases every Artifact, ingress Snapshot, scalar
// template, and role directory it retained.
package workspace

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/engine/rows/scalarlower"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
)

// ArtifactProduct is the immutable cold compiler product shared by equal
// CompileKeys inside one analysis Workspace. The four pointers are independent
// projections with one lifetime; no Link or runtime authority enters them.
type ArtifactProduct struct {
	Artifact *programartifact.Artifact
	Snapshot *ingress.Snapshot
	Template *rows.ArtifactScalarTemplate
	Roles    *scalarlower.RoleDirectory
}

type artifactEntry struct {
	ready   chan struct{}
	product ArtifactProduct
	valid   bool
}

// Artifacts is the content-addressed Program compiler directory for exactly
// one caller-owned analysis Workspace. It serializes equal concurrent keys and
// admits no new compilation once closed.
type Artifacts struct {
	mu      sync.Mutex
	entries map[identity.ContentID]*artifactEntry
	closed  bool
}

// NewArtifacts opens an empty workspace-owned compiler directory.
func NewArtifacts() *Artifacts {
	return &Artifacts{entries: make(map[identity.ContentID]*artifactEntry)}
}

// Compile returns the one immutable product for input's complete CompileKey in
// this directory. Concurrent callers for an equal key join the first compile.
func (artifacts *Artifacts) Compile(input *program.Program, compilation composite.Compilation) (ArtifactProduct, bool) {
	grammar := compilation.ExecutionSchemaID()
	grammarOK := compilation.Available() && grammar.Available()
	structural, structuralOK := compilation.Structure()
	compileKey, keyOK := programartifact.NewCompileKey(input, grammar)
	if !grammarOK || !structuralOK || !keyOK || !compileKey.Available() || input == nil || !input.Available() || !compilation.Available() {
		return ArtifactProduct{}, false
	}
	programID := input.ContentID()
	executionSchemaID := compileKey.ExecutionSchemaID().ContentID()
	if !programID.Available() || !executionSchemaID.Available() || artifacts == nil {
		return ArtifactProduct{}, false
	}
	return artifacts.compile(compileKey.ID(), func() (ArtifactProduct, bool) {
		return compileArtifactProduct(input, compileKey, programID, executionSchemaID, structural, compilation)
	})
}

// compile owns one per-key publication. Its terminal defer is deliberately
// outside the compiler call: a panic or Goexit must still wake every joiner,
// remove the failed entry, and then continue unwinding.
func (artifacts *Artifacts) compile(key identity.ContentID, build func() (ArtifactProduct, bool)) (product ArtifactProduct, valid bool) {
	if artifacts == nil || !key.Available() || build == nil {
		return ArtifactProduct{}, false
	}
	artifacts.mu.Lock()
	if artifacts.closed || artifacts.entries == nil {
		artifacts.mu.Unlock()
		return ArtifactProduct{}, false
	}
	entry := artifacts.entries[key]
	if entry == nil {
		entry = &artifactEntry{ready: make(chan struct{})}
		artifacts.entries[key] = entry
		artifacts.mu.Unlock()

		published := false
		defer func() {
			if published {
				return
			}
			panicValue := recover()
			artifacts.mu.Lock()
			entry.product = ArtifactProduct{}
			entry.valid = false
			close(entry.ready)
			if artifacts.entries != nil && artifacts.entries[key] == entry {
				delete(artifacts.entries, key)
			}
			artifacts.mu.Unlock()
			if panicValue != nil {
				panic(panicValue)
			}
		}()

		product, valid = build()

		artifacts.mu.Lock()
		if valid {
			entry.product = product
		}
		entry.valid = valid
		close(entry.ready)
		if !valid {
			delete(artifacts.entries, key)
		}
		artifacts.mu.Unlock()
		published = true
		return product, valid
	}
	ready := entry.ready
	artifacts.mu.Unlock()

	<-ready
	return entry.product, entry.valid
}

func compileArtifactProduct(input *program.Program, compileKey programartifact.CompileKey, programID, executionSchemaID identity.ContentID, structural structure.Table, compilation composite.Compilation) (ArtifactProduct, bool) {
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	artifact, failure := artifactcompiler.CompileDetailed(input, compileKey.ExecutionSchemaID(), issuance)
	if !issuanceOK || artifact == nil || failure.Available() {
		return ArtifactProduct{}, false
	}
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !lowered {
		return ArtifactProduct{}, false
	}
	template, roles, lowered := scalarlower.Lower(snapshot, structural)
	product := ArtifactProduct{Artifact: artifact, Snapshot: snapshot, Template: template, Roles: roles}
	return product, lowered && artifactProductMatches(product, compileKey, programID, executionSchemaID)
}

func artifactProductMatches(product ArtifactProduct, compileKey programartifact.CompileKey, programID, executionSchemaID identity.ContentID) bool {
	if !(compileKey.Available() && programID.Available() && executionSchemaID.Available() &&
		product.Artifact != nil && product.Artifact.Available() &&
		product.Artifact.CompileKey().ID() == compileKey.ID() &&
		product.Artifact.CompileKey().ProgramID() == programID &&
		product.Artifact.CompileKey().ExecutionSchemaID().ContentID() == executionSchemaID &&
		product.Snapshot != nil && product.Snapshot.Available() &&
		product.Snapshot.ArtifactID() == product.Artifact.ID() &&
		product.Snapshot.ProgramID() == programID && product.Snapshot.SchemaID() == executionSchemaID &&
		product.Template != nil && product.Template.Available() &&
		product.Template.ArtifactID() == product.Artifact.ID() &&
		product.Template.ProgramID() == programID && product.Template.SchemaID() == executionSchemaID &&
		product.Roles != nil && product.Roles.Count() == product.Template.RoleCount()) {
		return false
	}
	for index := 0; index < product.Roles.Count(); index++ {
		key, role, available := product.Roles.At(index)
		if !available || !key.Available() || !role.Available() || !product.Template.OwnsRole(role) {
			return false
		}
	}
	return true
}

// Close rejects future compiles and releases all strong references owned by
// this directory. Workspace calls it only after its sole compile-lifetime
// counter reaches zero, so this lower owner needs no duplicate active-work
// protocol. It is terminal.
func (artifacts *Artifacts) Close() bool {
	if artifacts == nil {
		return false
	}
	artifacts.mu.Lock()
	if artifacts.closed {
		artifacts.mu.Unlock()
		return false
	}
	artifacts.closed = true
	for key, entry := range artifacts.entries {
		if entry != nil {
			entry.product = ArtifactProduct{}
			entry.valid = false
			entry.ready = nil
		}
		delete(artifacts.entries, key)
	}
	artifacts.entries = nil
	artifacts.mu.Unlock()
	return true
}
