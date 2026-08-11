// Command flashrefactor executes one reviewed, fail-closed Go ownership cut.
//
// An Intent is the only authored declaration. Prepare derives an immutable
// Lock without writing source. A Lock can be replayed dry-run or applied by
// the workbench. Discovery is deliberately read-only and recovery remains an
// explicit, separately selected transaction action.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/discover"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/generate"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/transaction"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/workbench"
)

const helperBuild = "flashrefactor-v3"

type commandMode string

const (
	modePrepare  commandMode = "prepare"
	modeReplay   commandMode = "replay"
	modeApply    commandMode = "apply"
	modeDiscover commandMode = "discover"
	modeSurvey   commandMode = "survey"
	modeRecover  commandMode = "recover"
)

type options struct {
	root            string
	intent          string
	lock            string
	lockOut         string
	reportOut       string
	discoverDir     string
	discoverCallers string
	survey          string
	recovery        string
	apply           bool
}

type report struct {
	Mode      commandMode   `json:"mode"`
	Applied   bool          `json:"applied,omitempty"`
	Lock      *cutplan.Lock `json:"lock,omitempty"`
	Discovery any           `json:"discovery,omitempty"`
	Recovery  any           `json:"recovery,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type execution struct {
	reportPath string
	value      report
	err        error
	code       int
}

func main() {
	result := execute(context.Background(), os.Args[1:])
	if result.err != nil {
		result.value.Error = result.err.Error()
		if result.code == 0 {
			result.code = 1
		}
	}
	if err := emit(result.reportPath, result.value); err != nil {
		fmt.Fprintf(os.Stderr, "flashrefactor: write report: %v\n", err)
		if result.code == 0 {
			result.code = 2
		}
	}
	if result.code != 0 {
		os.Exit(result.code)
	}
}

func execute(ctx context.Context, args []string) execution {
	options, err := parseOptions(args)
	if err != nil {
		return execution{err: err, code: 2}
	}
	root, err := canonicalRoot(options.root)
	if err != nil {
		return execution{err: err, code: 2}
	}
	mode, err := options.mode()
	if err != nil {
		return execution{err: err, code: 2}
	}
	reportPath, err := resolveArtifact(root, options.reportOut, artifactReport)
	if err != nil {
		return execution{err: err, code: 2}
	}
	result := execution{reportPath: reportPath, value: report{Mode: mode}}

	if mode == modeDiscover {
		return discoverOnly(root, options, result)
	}
	if mode == modeSurvey {
		return surveyOnly(ctx, root, options, result)
	}
	bench, err := newWorkbench(root)
	if err != nil {
		result.err, result.code = err, 2
		return result
	}
	switch mode {
	case modePrepare:
		return prepare(ctx, root, bench, options, result)
	case modeReplay:
		return replay(ctx, root, bench, options, result, false)
	case modeApply:
		return replay(ctx, root, bench, options, result, true)
	case modeRecover:
		return recover(ctx, root, bench, options, result)
	default:
		result.err, result.code = fmt.Errorf("unknown command mode %q", mode), 2
		return result
	}
}

func parseOptions(args []string) (options, error) {
	var value options
	flags := flag.NewFlagSet("flashrefactor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&value.root, "root", ".", "repository root")
	flags.StringVar(&value.intent, "intent", "", "reviewed cutplan Intent JSON")
	flags.StringVar(&value.lock, "lock", "", "immutable cutplan Lock JSON")
	flags.StringVar(&value.lockOut, "lock-out", "", "explicit lock artifact destination")
	flags.StringVar(&value.reportOut, "report-out", "", "explicit report artifact destination")
	flags.StringVar(&value.discoverDir, "discover", "", "read-only package directory")
	flags.StringVar(&value.discoverCallers, "discover-callers", "", "read-only import path caller inventory")
	flags.StringVar(&value.survey, "survey", "", "read-only survey proposal JSON")
	flags.StringVar(&value.recovery, "recovery", "", "explicit recovery action: inspect, rollback, or complete")
	flags.BoolVar(&value.apply, "apply", false, "apply a replayed lock")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if len(flags.Args()) != 0 {
		return options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	return value, nil
}

func (value options) mode() (commandMode, error) {
	discovery := value.discoverDir != "" || value.discoverCallers != ""
	if (value.discoverDir != "" && value.discoverCallers != "") || (value.survey != "" && discovery) {
		return "", fmt.Errorf("-discover and -discover-callers are mutually exclusive")
	}
	if value.survey != "" {
		if value.intent != "" || value.lock != "" || value.lockOut != "" || value.apply || value.recovery != "" {
			return "", fmt.Errorf("survey is mutually exclusive with cut and recovery options")
		}
		return modeSurvey, nil
	}
	if discovery {
		if value.intent != "" || value.lock != "" || value.lockOut != "" || value.apply || value.recovery != "" {
			return "", fmt.Errorf("discovery is mutually exclusive with cut and recovery options")
		}
		return modeDiscover, nil
	}
	if value.recovery != "" {
		switch value.recovery {
		case "inspect", "rollback":
			if value.intent != "" || value.lock != "" || value.lockOut != "" || value.apply {
				return "", fmt.Errorf("recovery %s accepts no cut input or apply option", value.recovery)
			}
		case "complete":
			if value.intent != "" || value.lock == "" || value.lockOut != "" || value.apply {
				return "", fmt.Errorf("recovery complete requires exactly -lock and no apply option")
			}
		default:
			return "", fmt.Errorf("unknown recovery action %q", value.recovery)
		}
		return modeRecover, nil
	}
	if (value.intent == "") == (value.lock == "") {
		return "", fmt.Errorf("exactly one of -intent or -lock is required")
	}
	if value.apply && value.lock == "" {
		return "", fmt.Errorf("-apply requires -lock")
	}
	if value.lockOut != "" && value.intent == "" {
		return "", fmt.Errorf("-lock-out requires -intent")
	}
	if value.intent != "" {
		return modePrepare, nil
	}
	if value.apply {
		return modeApply, nil
	}
	return modeReplay, nil
}

func newWorkbench(root string) (workbench.Bench, error) {
	registry, err := generate.NewRegistry(nil)
	if err != nil {
		return workbench.Bench{}, err
	}
	return workbench.New(workbench.Config{
		Root: root,
		Semantic: semantic.Config{
			Root:          root,
			Flashrefactor: helperBuild,
		},
		Registry:  registry,
		Toolchain: cutplan.Toolchain{HelperBuild: helperBuild},
	})
}

func prepare(ctx context.Context, root string, bench workbench.Bench, options options, result execution) execution {
	intent, err := readIntent(options.intent)
	if err != nil {
		result.err, result.code = err, 1
		return result
	}
	lockOut, err := resolveArtifact(root, options.lockOut, artifactLock)
	if err != nil {
		result.err, result.code = err, 2
		return result
	}
	if err := rejectArtifactAuthority(root, intent, lockOut, result.reportPath); err != nil {
		result.err, result.code = err, 2
		return result
	}
	prepared, err := bench.Prepare(ctx, intent)
	if err != nil {
		result.err, result.code = err, 1
		return result
	}
	if lockOut != "" {
		encoded, encodeErr := cutplan.CanonicalJSON(prepared.Lock)
		if encodeErr != nil {
			result.err, result.code = encodeErr, 1
			return result
		}
		if writeErr := writeAtomic(lockOut, append(encoded, '\n')); writeErr != nil {
			result.err, result.code = writeErr, 2
			return result
		}
	}
	result.value.Lock = &prepared.Lock
	return result
}

func replay(ctx context.Context, root string, bench workbench.Bench, options options, result execution, apply bool) execution {
	lock, err := readLock(options.lock)
	if err != nil {
		result.err, result.code = err, 1
		return result
	}
	if err := rejectArtifactAuthority(root, lock.Intent, result.reportPath); err != nil {
		result.err, result.code = err, 2
		return result
	}
	if !apply {
		prepared, replayErr := bench.Replay(ctx, lock)
		if replayErr != nil {
			result.err, result.code = replayErr, 1
			return result
		}
		result.value.Lock = &prepared.Lock
		return result
	}
	if err := bench.Apply(ctx, lock); err != nil {
		result.err, result.code = err, commandErrorCode(err)
		return result
	}
	result.value.Applied = true
	result.value.Lock = &lock
	return result
}

func recover(ctx context.Context, root string, bench workbench.Bench, options options, result execution) execution {
	switch options.recovery {
	case "inspect":
		value, err := bench.InspectRecovery()
		if err != nil {
			result.err, result.code = err, 1
			return result
		}
		result.value.Recovery = value
	case "rollback":
		if err := bench.RollbackRecovery(); err != nil {
			result.err, result.code = err, 1
			return result
		}
		result.value.Recovery = map[string]string{"action": "rollback", "status": "completed"}
	case "complete":
		lock, err := readLock(options.lock)
		if err != nil {
			result.err, result.code = err, 1
			return result
		}
		if err := rejectArtifactAuthority(root, lock.Intent, result.reportPath); err != nil {
			result.err, result.code = err, 2
			return result
		}
		if err := bench.CompleteRecovery(ctx, lock); err != nil {
			result.err, result.code = err, commandErrorCode(err)
			return result
		}
		result.value.Recovery = map[string]string{"action": "complete", "status": "completed"}
	default:
		result.err, result.code = fmt.Errorf("unknown recovery action %q", options.recovery), 2
	}
	return result
}

func discoverOnly(root string, options options, result execution) execution {
	if options.discoverDir != "" {
		path, err := resolveDiscoveryPath(root, options.discoverDir)
		if err != nil {
			result.err, result.code = err, 2
			return result
		}
		value, err := discover.AnalyzeDir(path)
		if err != nil {
			result.err, result.code = err, 1
			return result
		}
		result.value.Discovery = value
		return result
	}
	value, err := discover.CallerPackages(root, options.discoverCallers)
	if err != nil {
		result.err, result.code = err, 1
		return result
	}
	result.value.Discovery = value
	return result
}

func surveyOnly(ctx context.Context, root string, options options, result execution) execution {
	data, err := os.ReadFile(options.survey)
	if err != nil {
		result.err, result.code = fmt.Errorf("read survey: %w", err), 1
		return result
	}
	var input discover.SurveyInput
	if err := json.Unmarshal(data, &input); err != nil {
		result.err, result.code = fmt.Errorf("decode survey: %w", err), 1
		return result
	}
	symbols := make([]cutplan.SymbolRef, 0, len(input.Symbols))
	for _, object := range input.Symbols {
		symbols = append(symbols, cutplan.SymbolRef{Object: object})
	}
	session, err := semantic.NewSession(semantic.Config{Root: root, Flashrefactor: helperBuild})
	if err != nil {
		result.err, result.code = err, 1
		return result
	}
	defer session.Close()
	snapshot, err := session.Survey(ctx, symbols)
	if err != nil {
		result.err, result.code = err, 1
		return result
	}
	proposal, err := discover.Propose(input, snapshot)
	if err != nil {
		result.err, result.code = err, 1
		return result
	}
	result.value.Discovery = proposal
	return result
}

func readIntent(path string) (cutplan.Intent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cutplan.Intent{}, fmt.Errorf("read intent: %w", err)
	}
	if proposalArtifact(data) {
		return cutplan.Intent{}, fmt.Errorf("survey proposal is not an Intent")
	}
	intent, err := cutplan.DecodeIntent(data)
	if err != nil {
		return cutplan.Intent{}, fmt.Errorf("decode intent: %w", err)
	}
	return intent, nil
}

func readLock(path string) (cutplan.Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cutplan.Lock{}, fmt.Errorf("read lock: %w", err)
	}
	if proposalArtifact(data) {
		return cutplan.Lock{}, fmt.Errorf("survey proposal is not a Lock and cannot be replayed or applied")
	}
	lock, err := cutplan.DecodeLock(data)
	if err != nil {
		return cutplan.Lock{}, fmt.Errorf("decode lock: %w", err)
	}
	return lock, nil
}

func proposalArtifact(data []byte) bool {
	var header struct {
		Kind string `json:"kind"`
	}
	return json.Unmarshal(data, &header) == nil && header.Kind == "flashrefactor-survey-proposal-v1"
}

func commandErrorCode(err error) int {
	if errors.Is(err, transaction.ErrSafetyFailure) {
		return 125
	}
	return 1
}

func emit(path string, value report) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	encoded = append(encoded, '\n')
	if path != "" {
		if err := writeAtomic(path, encoded); err != nil {
			return err
		}
	}
	_, err = os.Stdout.Write(encoded)
	return err
}

func canonicalRoot(path string) (string, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root is not a directory")
	}
	return filepath.Clean(root), nil
}
