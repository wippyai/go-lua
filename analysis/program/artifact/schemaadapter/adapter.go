// Package schemaadapter is the sole bridge from the exact Program schema
// receipt to programartifact's compiler-only grammar capability.
package schemaadapter

import (
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactgrammar "github.com/wippyai/go-lua/analysis/program/artifact/internal/grammar"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
)

func Compile(input program.TransformerInput, receipt grammar.CompilationReceipt) (*programartifact.Artifact, bool) {
	artifact, failure := CompileDetailed(input, receipt)
	return artifact, artifact != nil && !failure.Available()
}

// CompileDetailed preserves exact receipt validation and returns only the
// closed immutable failure projection from the authorized compiler.
func CompileDetailed(input program.TransformerInput, receipt grammar.CompilationReceipt) (*programartifact.Artifact, programartifact.CompileFailure) {
	capability, _ := capability(receipt)
	return programartifact.CompileDetailedAuthorized(input, capability)
}

func NewCompileKey(input program.TransformerInput, receipt grammar.CompilationReceipt) (programartifact.CompileKey, bool) {
	capability, ok := capability(receipt)
	if !ok {
		return programartifact.CompileKey{}, false
	}
	return programartifact.NewCompileKeyAuthorized(input, capability)
}

func capability(receipt grammar.CompilationReceipt) (artifactgrammar.Capability, bool) {
	if !receipt.Available() {
		return artifactgrammar.Capability{}, false
	}
	return artifactgrammar.Issue(receipt.Digest(), receipt.Version())
}
