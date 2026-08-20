package composite

import (
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
