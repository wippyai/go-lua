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
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	key, keyOK := programartifact.NewCompileKey(input, grammar)
	if !shardOK || !inputOK || input == nil || !compilationOK || !grammar.Available() || !keyOK {
		t.Fatal("workspace artifact fixture is unavailable")
	}

	artifacts := NewArtifacts()
	product, failure, compiled := artifacts.Compile(input, compilation)
	if !compiled || product.Artifact == nil || product.Snapshot == nil || product.Template == nil || product.Bindings == nil {
		_, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
		t.Fatalf("workspace artifact product did not compile: compiled=%v artifact=%t snapshot=%t template=%t bindings=%t issuance=%v failure=%v", compiled, product.Artifact != nil, product.Snapshot != nil, product.Template != nil, product.Bindings != nil, issuanceOK, failure)
	}
	artifacts.mu.Lock()
	entry := artifacts.entries[key.ID()]
	retained := entry != nil && entry.valid && entry.product.Artifact == product.Artifact && entry.product.Snapshot == product.Snapshot && entry.product.Template == product.Template && entry.product.Bindings == product.Bindings
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
	if next, _, ok := artifacts.Compile(input, compilation); ok || next != (ArtifactProduct{}) {
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
		_, _, _ = artifacts.compile(key, func() (ArtifactProduct, artifactcompiler.CompileFailure, bool) {
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
		_, _, _ = artifacts.compile(key, func() (ArtifactProduct, artifactcompiler.CompileFailure, bool) {
			close(started)
			<-release
			runtime.Goexit()
			return ArtifactProduct{}, artifactcompiler.CompileFailure{}, false
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

// TestArtifactsServeEqualKeyWithoutASecondSeal is the directory's count law.
// A product is addressed by the complete CompileKey of its Program, so two
// independent Links carrying equal content name one key, and the second caller
// must be answered with the first caller's product rather than with an equal
// one compiled again. The final probe states that as a count: a warm key never
// reaches the builder at all.
func TestArtifactsServeEqualKeyWithoutASecondSeal(t *testing.T) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	const source = `local reuse_probe = 29
return reuse_probe`
	left, err := testfixture.SealSource(target, "workspace-reuse-law", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	right, err := testfixture.SealSource(target, "workspace-reuse-law", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	leftShard, leftShardOK := left.Project().Mounts().At(0)
	rightShard, rightShardOK := right.Project().Mounts().At(0)
	leftInput, leftInputOK := left.Project().Mounts().Program(leftShard)
	rightInput, rightInputOK := right.Project().Mounts().Program(rightShard)
	compilation, compilationOK := composite.Build()
	key, keyOK := programartifact.NewCompileKey(leftInput, compilation.ExecutionSchemaID())
	if !leftShardOK || !rightShardOK || !leftInputOK || !rightInputOK || !compilationOK || !keyOK {
		t.Fatal("workspace reuse fixture is unavailable")
	}
	if leftInput == rightInput {
		t.Fatal("independent Link fixtures aliased one Program")
	}

	artifacts := NewArtifacts()
	defer artifacts.Close()
	first, _, firstOK := artifacts.Compile(leftInput, compilation)
	second, _, secondOK := artifacts.Compile(rightInput, compilation)
	if !firstOK || !secondOK || first.Artifact == nil {
		t.Fatalf("equal-content compile = %t/%t", firstOK, secondOK)
	}
	if second.Artifact != first.Artifact || second.Snapshot != first.Snapshot || second.Template != first.Template || second.Bindings != first.Bindings {
		t.Fatal("equal CompileKeys sealed two independent products")
	}
	artifacts.mu.Lock()
	entries := len(artifacts.entries)
	artifacts.mu.Unlock()
	if entries != 1 {
		t.Fatalf("workspace directory holds %d entries for one CompileKey", entries)
	}

	seals := 0
	served, _, servedOK := artifacts.compile(key.ID(), func() (ArtifactProduct, artifactcompiler.CompileFailure, bool) {
		seals++
		return ArtifactProduct{}, artifactcompiler.CompileFailure{}, true
	})
	if !servedOK || served.Artifact != first.Artifact {
		t.Fatal("warm CompileKey was not served the retained product")
	}
	if seals != 0 {
		t.Fatalf("warm CompileKey reached the compiler %d times", seals)
	}
}

// TestArtifactsPublishTheCompilerRefusalToEveryCaller states the observability
// law of the workspace compiler directory: a refused Program-to-Artifact
// compilation is published at the artifact compiler's own evidence type, and
// the directory entry a joining caller is served carries that same refusal.
//
// The directory serializes equal keys, so a caller that joins a compile in
// flight never runs the compiler itself. Publishing only the invalid product
// to that caller would leave its item-issuance refusal unnamed while the
// leader's is named, making the same refused key observable or unobservable
// depending on arrival order.
func TestArtifactsPublishTheCompilerRefusalToEveryCaller(t *testing.T) {
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("artifact compilation is unavailable")
	}
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !issuanceOK {
		t.Fatal("artifact issuance directory is unavailable")
	}
	// The refusal a real compiler raises: an artifact compilation that cannot
	// resolve its mounted Program names that act, not a bare bool.
	_, refusal := artifactcompiler.CompileDetailed(nil, compilation.ExecutionSchemaID(), issuance)
	if !refusal.Available() {
		t.Fatal("the artifact compiler published no evidence for a refused compilation")
	}

	artifacts := NewArtifacts()
	defer artifacts.Close()
	key := identity.ContentID{0x43}
	entered := make(chan struct{})
	release := make(chan struct{})
	published := make(chan artifactcompiler.CompileFailure, 1)
	go func() {
		_, failure, valid := artifacts.compile(key, func() (ArtifactProduct, artifactcompiler.CompileFailure, bool) {
			close(entered)
			<-release
			return ArtifactProduct{}, refusal, false
		})
		if valid {
			t.Error("a refused compilation was published as valid")
		}
		published <- failure
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("leader did not enter compiler product construction")
	}
	// A joining caller holds exactly this entry and reads what the leader
	// publishes into it. Taking the entry while the compile is in flight is
	// the join, without a second racing goroutine.
	artifacts.mu.Lock()
	entry := artifacts.entries[key]
	artifacts.mu.Unlock()
	if entry == nil {
		t.Fatal("an in-flight CompileKey has no directory entry to join")
	}
	close(release)

	select {
	case failure := <-published:
		assertPublishedArtifactRefusal(t, "leader", failure, refusal)
	case <-time.After(2 * time.Second):
		t.Fatal("the leader of a refused CompileKey was never published a verdict")
	}
	select {
	case <-entry.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("a joined entry was never woken")
	}
	if entry.valid {
		t.Fatal("a refused compilation was published to a joining caller as valid")
	}
	assertPublishedArtifactRefusal(t, "joining caller", entry.failure, refusal)
}

func assertPublishedArtifactRefusal(t *testing.T, caller string, published, expected artifactcompiler.CompileFailure) {
	t.Helper()
	if !published.Available() {
		t.Fatalf("the %s of a refused compilation received no named evidence", caller)
	}
	if published.Stage() != expected.Stage() || published.RowKind() != expected.RowKind() || published.Reason() != expected.Reason() {
		t.Fatalf("the %s received %q, which is not the compiler's own refusal %q", caller, published.Error(), expected.Error())
	}
}
