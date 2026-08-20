package workspace

import (
	"runtime"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestArtifactsCloseReleasesEveryOwnedProductReference(t *testing.T) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(target, "workspace-lifetime-law", []byte(`return 41`))
	if err != nil {
		t.Fatal(err)
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	input, inputOK := mounts.Program(shard)
	compilation, compilationOK := composite.Global()
	grammar, grammarOK := composite.ArtifactGrammar(compilation)
	key, keyOK := programartifact.NewCompileKey(input, grammar)
	if !shardOK || !inputOK || input == nil || !compilationOK || !grammarOK || !keyOK {
		t.Fatal("workspace artifact fixture is unavailable")
	}

	artifacts := NewArtifacts()
	product, compiled := artifacts.Compile(input, compilation)
	if !compiled || product.Artifact == nil || product.Snapshot == nil || product.Template == nil || product.Roles == nil {
		issuance, issuanceOK := composite.ArtifactIssuanceDirectory()
		_, failure := artifactcompiler.CompileDetailed(input, grammar, issuance)
		t.Fatalf("workspace artifact product did not compile: compiled=%v artifact=%t snapshot=%t template=%t roles=%t issuance=%v failure=%v", compiled, product.Artifact != nil, product.Snapshot != nil, product.Template != nil, product.Roles != nil, issuanceOK, failure)
	}
	artifacts.mu.Lock()
	entry := artifacts.entries[key.ID()]
	retained := entry != nil && entry.valid && entry.product.Artifact == product.Artifact && entry.product.Snapshot == product.Snapshot && entry.product.Template == product.Template && entry.product.Roles == product.Roles
	artifacts.mu.Unlock()
	if !retained {
		t.Fatal("workspace did not retain its compiled product")
	}
	if !artifacts.Close() {
		t.Fatal("Artifacts.Close failed")
	}
	artifacts.mu.Lock()
	released := artifacts.closed && artifacts.entries == nil && entry.product == (ArtifactProduct{}) && !entry.valid && entry.ready == nil
	artifacts.mu.Unlock()
	if !released {
		t.Fatal("Artifacts.Close retained a strong product reference")
	}
	if next, ok := artifacts.Compile(input, compilation); ok || next != (ArtifactProduct{}) {
		t.Fatal("closed Artifacts admitted another compiler product")
	}
}

func TestArtifactsLeaderPanicTerminatesFailedEntryBeforeRethrow(t *testing.T) {
	artifacts := NewArtifacts()
	key := identity.ContentID{0x41}
	started := make(chan struct{})
	release := make(chan struct{})
	recovered := make(chan any, 1)
	go func() {
		defer func() { recovered <- recover() }()
		_, _ = artifacts.compile(key, func() (ArtifactProduct, bool) {
			close(started)
			<-release
			panic("workspace compiler panic")
		})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("leader did not enter compiler product construction")
	}
	artifacts.mu.Lock()
	entry := artifacts.entries[key]
	artifacts.mu.Unlock()
	if entry == nil {
		close(release)
		t.Fatal("leader did not publish its pending entry")
	}
	close(release)
	select {
	case value := <-recovered:
		if value != "workspace compiler panic" {
			t.Fatalf("recovered panic = %v", value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("compiler panic did not unwind")
	}
	select {
	case <-entry.ready:
	default:
		t.Fatal("compiler panic left the pending entry open")
	}
	artifacts.mu.Lock()
	_, retained := artifacts.entries[key]
	terminal := !retained && !entry.valid && entry.product == (ArtifactProduct{})
	artifacts.mu.Unlock()
	if !terminal {
		t.Fatal("compiler panic retained a failed product entry")
	}
	if !artifacts.Close() {
		t.Fatal("Artifacts.Close failed after compiler panic")
	}
}

func TestArtifactsLeaderGoexitTerminatesFailedEntry(t *testing.T) {
	artifacts := NewArtifacts()
	key := identity.ContentID{0x42}
	started := make(chan struct{})
	release := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_, _ = artifacts.compile(key, func() (ArtifactProduct, bool) {
			close(started)
			<-release
			runtime.Goexit()
			return ArtifactProduct{}, false
		})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("leader did not enter compiler product construction")
	}
	artifacts.mu.Lock()
	entry := artifacts.entries[key]
	artifacts.mu.Unlock()
	if entry == nil {
		close(release)
		t.Fatal("leader did not publish its pending entry")
	}
	close(release)
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("compiler Goexit did not unwind")
	}
	select {
	case <-entry.ready:
	default:
		t.Fatal("compiler Goexit left the pending entry open")
	}
	artifacts.mu.Lock()
	_, retained := artifacts.entries[key]
	terminal := !retained && !entry.valid && entry.product == (ArtifactProduct{})
	artifacts.mu.Unlock()
	if !terminal {
		t.Fatal("compiler Goexit retained a failed product entry")
	}
	if !artifacts.Close() {
		t.Fatal("Artifacts.Close failed after compiler Goexit")
	}
}
