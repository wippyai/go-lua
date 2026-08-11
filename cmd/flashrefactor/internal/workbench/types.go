package workbench

import (
	"context"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/generate"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// Config supplies the explicit capabilities required for one workbench run.
// It is deliberately not a second cut declaration: all ownership still comes
// solely from cutplan.Intent.
type Config struct {
	Root      string
	Semantic  semantic.Config
	Registry  generate.Registry
	Toolchain cutplan.Toolchain
}

// Bench owns no mutable global state. Each Prepare opens one isolated semantic
// session; each Apply re-prepares evidence while transaction owns the lease.
type Bench struct{ config Config }

// Prepared is a complete write-free, reviewed dry-run result. Lock is the
// sole mutation authority; Rendered is retained only to build its exact plan.
type Prepared struct {
	Lock     cutplan.Lock
	rendered rendered
}

// rendered keeps executor-only final bytes outside lock vocabulary. Its
// contents are verified against the lock before every mutation.
type rendered struct {
	files  []semantic.VirtualFile
	diff   []byte
	source semantic.Snapshot // immutable preflight baseline for physical gates
}

// New validates only ambient configuration shape. Intent-specific validation
// remains at the authoritative Prepare boundary.
func New(config Config) (Bench, error) {
	if config.Root == "" {
		return Bench{}, errConfig("root is required")
	}
	if config.Semantic.Root == "" {
		config.Semantic.Root = config.Root
	}
	if config.Semantic.Root != config.Root {
		return Bench{}, errConfig("semantic root must equal workbench root")
	}
	if config.Semantic.Flashrefactor == "" || config.Toolchain.HelperBuild == "" {
		return Bench{}, errConfig("semantic and workbench helper build identities are required")
	}
	if config.Semantic.Flashrefactor != config.Toolchain.HelperBuild {
		return Bench{}, errConfig("semantic helper build must equal workbench helper build")
	}
	return Bench{config: config}, nil
}

// Prepare creates the only acceptable lock from a canonical reviewed intent.
func (bench Bench) Prepare(ctx context.Context, intent cutplan.Intent) (Prepared, error) {
	return bench.prepare(ctx, intent)
}

// Replay rebuilds all evidence from the current checkout and rejects every
// drift, including a byte-identical-looking output with changed provenance.
func (bench Bench) Replay(ctx context.Context, lock cutplan.Lock) (Prepared, error) {
	if err := cutplan.VerifyLock(bench.config.Root, lock); err != nil {
		return Prepared{}, err
	}
	prepared, err := bench.prepare(ctx, lock.Intent)
	if err != nil {
		return Prepared{}, err
	}
	if err := compareLocks(lock, prepared.Lock); err != nil {
		return Prepared{}, err
	}
	return prepared, nil
}
