package workbench

import (
	"context"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/render"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

func (bench Bench) prepare(ctx context.Context, intent cutplan.Intent) (Prepared, error) {
	if err := cutplan.ValidateIntent(intent); err != nil {
		return Prepared{}, err
	}
	canonical, err := cutplan.CanonicalIntent(intent)
	if err != nil {
		return Prepared{}, err
	}
	requirements, err := cutplan.ResolutionRequirements(canonical)
	if err != nil {
		return Prepared{}, err
	}
	inputs, err := cutplan.Fingerprint(bench.config.Root, cutplan.ReadPaths(canonical), absentWrites(canonical))
	if err != nil {
		return Prepared{}, err
	}
	session, err := semantic.NewSession(bench.config.Semantic)
	if err != nil {
		return Prepared{}, err
	}
	defer session.Close()
	// Diagnostics are a complete workspace obligation, not merely a changed
	// file check: a package-level type failure can be caused by a cut in a
	// different source file. Nil asks semantic collection for the whole loaded
	// workspace denominator.
	source, err := session.Collect(ctx, canonical, nil)
	if err != nil {
		return Prepared{}, err
	}
	output, err := render.New().Render(render.Input{Intent: canonical, Snapshot: source, Registry: bench.config.Registry})
	if err != nil {
		return Prepared{}, err
	}
	target, err := session.CollectVirtual(ctx, canonical, nil, output.Files)
	if err != nil {
		return Prepared{}, err
	}
	merged, err := semantic.Merge(source, target, requirements)
	if err != nil {
		return Prepared{}, err
	}
	routes, err := deriveRoutes(canonical, output.Witnesses, source, target)
	if err != nil {
		return Prepared{}, err
	}
	gates, err := verifyGates(canonical, source, target)
	if err != nil {
		return Prepared{}, err
	}
	diff, err := renderDiff(output.Diffs)
	if err != nil {
		return Prepared{}, err
	}
	execution, err := executionEvidence(output.Files, diff)
	if err != nil {
		return Prepared{}, err
	}
	toolchain, err := bench.toolchainAt(source)
	if err != nil {
		return Prepared{}, err
	}
	lock, err := cutplan.BuildLock(canonical, toolchain, cutplan.LockEvidence{
		Inputs:     inputs,
		Resolution: cutplan.ResolutionEvidence{Objects: merged.Objects, Providers: output.Providers},
		Routes:     routes, Gates: gates, Execution: execution, Hazards: output.Hazards,
	})
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Lock: lock, rendered: rendered{files: cloneFiles(output.Files), diff: append([]byte(nil), diff...), source: source}}, nil
}

func absentWrites(intent cutplan.Intent) []string {
	reads := map[string]bool{}
	for _, path := range cutplan.ReadPaths(intent) {
		reads[path] = true
	}
	result := make([]string, 0)
	for _, path := range cutplan.WritePaths(intent) {
		if !reads[path] {
			result = append(result, path)
		}
	}
	return result
}

func (bench Bench) toolchainAt(source semantic.Snapshot) (cutplan.Toolchain, error) {
	fromSemantic := source.Toolchain
	goVersion, err := canonicalGoVersion(fromSemantic.GoVersion)
	if err != nil {
		return cutplan.Toolchain{}, err
	}
	helperHash, err := helperIdentity()
	if err != nil {
		return cutplan.Toolchain{}, err
	}
	result := bench.config.Toolchain
	result.GoVersion = goVersion
	result.GoExecutableSHA256 = fromSemantic.GoExecutableSHA256
	result.Resolver = fromSemantic.Resolver
	if result.HelperBuild == "" {
		result.HelperBuild = fromSemantic.HelperBuild
	}
	if result.HelperBuild != fromSemantic.HelperBuild {
		return cutplan.Toolchain{}, errConfig("helper build does not equal semantic authority")
	}
	result.HelperSHA256 = helperHash
	result.BuildEnvSHA256 = fromSemantic.BuildEnvSHA256
	result.ModuleGraphSHA256 = fromSemantic.ModuleGraphSHA256
	return result, nil
}
