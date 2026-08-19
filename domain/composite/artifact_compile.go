package composite

import (
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// ArtifactGrammar is the one translation from the sealed domain composition
// to the neutral cold identity consumed by Program Artifact. The digest and
// ABI are copied as data; no schema, rule, or domain authority crosses the
// artifact boundary.
func ArtifactGrammar(compilation Compilation) (programartifact.GrammarIdentity, bool) {
	if !compilation.Available() || !compilation.digest.Available() || compilation.version != ABIVersion {
		return programartifact.GrammarIdentity{}, false
	}
	return programartifact.NewGrammarIdentity(compilation.digest, compilation.version)
}

// NewArtifactCompileKey derives the reusable Program Artifact key from the
// sealed composition. Invalid or unavailable compositions fail closed.
func NewArtifactCompileKey(input *program.Program, compilation Compilation) (programartifact.CompileKey, bool) {
	grammar, ok := ArtifactGrammar(compilation)
	if !ok {
		return programartifact.CompileKey{}, false
	}
	return programartifact.NewCompileKey(input, grammar)
}

// CompileArtifactDetailed invokes the neutral Program Artifact compiler from
// the composition root and retains its exact immutable failure projection.
func CompileArtifactDetailed(input *program.Program, compilation Compilation) (*programartifact.Artifact, programartifact.CompileFailure) {
	grammar, _ := ArtifactGrammar(compilation)
	issuance, issuanceOK := ArtifactIssuanceDirectory()
	if !issuanceOK {
		return programartifact.CompileDetailed(input, grammar, programartifact.IssuanceDirectory{})
	}
	return programartifact.CompileDetailed(input, grammar, issuance)
}

// CompileArtifact compiles one Program under this sealed composition.
func CompileArtifact(input *program.Program, compilation Compilation) (*programartifact.Artifact, bool) {
	artifact, failure := CompileArtifactDetailed(input, compilation)
	return artifact, artifact != nil && !failure.Available()
}
