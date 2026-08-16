package body

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
)

func TestBoundaryEnvironmentDigestTracksSemanticEnvironment(t *testing.T) {
	base := boundaryDigestConfig(typ.String, typ.Number, typ.Boolean, typ.Integer)
	want := prepareBoundaryDigest(t, base)
	tests := []struct {
		name   string
		config Config
	}{
		{"global-type", boundaryDigestConfig(typ.Number, typ.Number, typ.Boolean, typ.Integer)},
		{"signature", boundaryDigestConfig(typ.String, typ.String, typ.Boolean, typ.Integer)},
		{"module-type", boundaryDigestConfig(typ.String, typ.Number, typ.String, typ.Integer)},
		{"module-export", boundaryDigestConfig(typ.String, typ.Number, typ.Boolean, typ.String)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := prepareBoundaryDigest(t, test.config); got == want {
				t.Fatalf("semantic %s change retained digest %x", test.name, got)
			}
		})
	}
}

func TestBoundaryEnvironmentDigestCanonicalizesUnorderedInputs(t *testing.T) {
	left := boundaryDigestConfig(typ.String, typ.Number, typ.Boolean, typ.Integer)
	left.Globals = []string{"zeta", "alpha"}
	left.GlobalTypes = map[string]typ.Type{"zeta": typ.String, "alpha": typ.Number}
	right := boundaryDigestConfig(typ.String, typ.Number, typ.Boolean, typ.Integer)
	right.Globals = []string{"alpha", "zeta"}
	right.GlobalTypes = map[string]typ.Type{"alpha": typ.Number, "zeta": typ.String}

	if got, want := prepareBoundaryDigest(t, right), prepareBoundaryDigest(t, left); got != want {
		t.Fatalf("unordered equivalent environments differ\nleft:  %x\nright: %x", want, got)
	}
}

func TestBoundaryEnvironmentDigestExcludesBodySemantics(t *testing.T) {
	config := boundaryDigestConfig(typ.String, typ.Number, typ.Boolean, typ.Integer)
	left := prepareBoundaryStatic(t, config)
	right, err := PrepareChunk(parseChunk(t, "local unrelated: number = 42; return unrelated"), config)
	if err != nil {
		t.Fatalf("PrepareChunk: %v", err)
	}
	if got, want := right.BoundaryEnvironmentDigest(), left.BoundaryEnvironmentDigest(); got != want {
		t.Fatalf("body-local semantics changed boundary environment\nleft:  %x\nright: %x", want, got)
	}
}

func TestBoundaryEnvironmentDigestDeterministicAndCancellationAware(t *testing.T) {
	config := boundaryDigestConfig(typ.String, typ.Number, typ.Boolean, typ.Integer)
	left := prepareBoundaryStatic(t, config)
	right := prepareBoundaryStatic(t, config)
	if got, want := left.BoundaryEnvironmentDigest(), right.BoundaryEnvironmentDigest(); got != want {
		t.Fatalf("independent preparations differ\nleft:  %x\nright: %x", want, got)
	}
	want := left.BoundaryEnvironmentDigest()
	concurrent := prepareBoundaryStatic(t, config)
	var group sync.WaitGroup
	for index := 0; index < 16; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if got := concurrent.BoundaryEnvironmentDigest(); got != want {
				t.Errorf("concurrent digest = %x, want %x", got, want)
			}
		}()
	}
	group.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := left.BoundaryEnvironmentDigestContext(ctx); !errors.Is(err, context.Canceled) || !errors.Is(err, solve.ErrCanceled) {
		t.Fatalf("cached digest error = %v, want solve and context cancellation", err)
	}
}

func prepareBoundaryDigest(t *testing.T, config Config) BoundaryEnvironmentDigest {
	t.Helper()
	return prepareBoundaryStatic(t, config).BoundaryEnvironmentDigest()
}

func prepareBoundaryStatic(t *testing.T, config Config) *Static {
	t.Helper()
	prepared, err := PrepareChunk(parseChunk(t, "return service"), config)
	if err != nil {
		t.Fatalf("PrepareChunk: %v", err)
	}
	return prepared
}

func boundaryDigestConfig(globalType, signatureType, moduleType, exportType typ.Type) Config {
	signatures := manifest.New("provider.signatures")
	signatures.DefineFunctionSignature("service.call", signature.Function{Type: typ.Func().Returns(signatureType).Build()})
	types := manifest.New("provider.types")
	types.DefineType("Payload", moduleType)
	exports := manifest.New("provider.exports")
	exports.SetExport(exportType)
	return Config{
		Registry:      standard.Registry(),
		Globals:       []string{"service"},
		GlobalTypes:   map[string]typ.Type{"service": globalType},
		Signatures:    signaturelookup.Source{Manifests: []*manifest.Manifest{signatures}, IncludeStdlib: true},
		ModuleTypes:   typelookup.Source{Manifests: []*manifest.Manifest{types}},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{exports}},
	}
}
