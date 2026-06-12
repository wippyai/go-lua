package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SignatureMutationConfig carries signature lookup plus the generic fact model
// needed to lower declared active mutation/store effects.
type SignatureMutationConfig struct {
	Graph      cfg.Graph
	Signatures SignatureLookup
	NameFor    SignatureNameFunc
	Facts      factflow.Facts
}

// WithSignatureMutations returns Facts extended with conservative descendant
// invalidations lowered from declared active mutation/store effects.
func WithSignatureMutations(config SignatureMutationConfig) factflow.Facts {
	if config.Graph == nil || config.Signatures == nil || config.NameFor == nil {
		return config.Facts
	}
	invalidations := signatureMutationFacts(config)
	return config.Facts.WithPathDescendantInvalidations(invalidations)
}

func signatureMutationFacts(config SignatureMutationConfig) map[cfg.Point]factflow.PathDescendantInvalidation {
	out := make(map[cfg.Point]factflow.PathDescendantInvalidation)
	for _, point := range config.Graph.RPO() {
		site, ok := config.Facts.CallSite(point)
		if !ok {
			continue
		}
		sig, ok := signatureForMutationSite(config, point, site)
		if !ok {
			continue
		}
		for _, target := range activeMutationTargets(sig) {
			targetPath, ok := mutationTargetPath(config, site, target)
			if !ok {
				continue
			}
			if existing, ok := out[point]; ok && !existing.ContainerPath().Equal(targetPath) {
				continue
			}
			out[point] = factflow.NewPathDescendantInvalidation(targetPath)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func signatureForMutationSite(config SignatureMutationConfig, point cfg.Point, site factflow.CallSite) (signature.Function, bool) {
	name, ok := config.NameFor(transfer.NodeContext{
		Graph: config.Graph,
		Point: point,
		Node:  config.Graph.Node(point),
	}, factflow.CallProducerFromSite(site))
	if !ok {
		return signature.Function{}, false
	}
	return config.Signatures.Lookup(name)
}

func activeMutationTargets(sig signature.Function) []effect.ParamRef {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]effect.ParamRef, 0, len(sig.Effect.Labels))
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case mutation.TableMutator:
			out = append(out, normalized.Target)
		case *mutation.TableMutator:
			if normalized != nil {
				out = append(out, normalized.Target)
			}
		case mutation.LengthChange:
			out = append(out, normalized.Target)
		case *mutation.LengthChange:
			if normalized != nil {
				out = append(out, normalized.Target)
			}
		case ownership.Store:
			if normalized.Into.Index >= 0 {
				out = append(out, normalized.Into)
			}
		case *ownership.Store:
			if normalized != nil && normalized.Into.Index >= 0 {
				out = append(out, normalized.Into)
			}
		}
	}
	return out
}

func mutationTargetPath(config SignatureMutationConfig, site factflow.CallSite, ref effect.ParamRef) (path.Path, bool) {
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(ref, len(args))
	if !ok {
		return path.Path{}, false
	}
	arg := args[argIndex]
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return path.Path{}, false
	}
	targetPath, ok := config.Facts.ExpressionPath(arg.ExprRef)
	if !ok || targetPath.IsEmpty() {
		return path.Path{}, false
	}
	return targetPath, true
}
