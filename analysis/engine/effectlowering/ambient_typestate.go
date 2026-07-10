package effectlowering

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// AmbientTypestateEscapeOutcomeProviderConfig contains the call-boundary
// identity views required to transfer ownership of any tracked protocol at an
// opaque call. A declared signature is authoritative and therefore does not
// receive this conservative escape.
type AmbientTypestateEscapeOutcomeProviderConfig struct {
	NameForSite SignatureSiteNameFunc
	Signatures  SignatureLookup
	Facts       factflow.Facts
	KeySpace    *keyspace.KeySpace
	Resolver    *visibility.Resolver
}

// AmbientTypestateEscapeOutcomeProvider conservatively transfers locally
// tracked lifecycle ownership when an unknown callee receives a path-backed
// resource. This is protocol-independent and prevents an unmodeled callee from
// being treated as preserving db, file, lock, or channel ownership.
func AmbientTypestateEscapeOutcomeProvider(config AmbientTypestateEscapeOutcomeProviderConfig) callpayload.CallOutcomeProvider {
	args := signatureArgumentReader{keySpace: config.KeySpace}
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, _ func(cfg.Point) state.State) callpayload.CallOutcome {
		if typestateCallHasKnownSignature(ctx, site, config.NameForSite, config.Signatures) {
			return callpayload.CallOutcome{}
		}
		facts := typestateOpaqueEscapeFacts(ctx, site, in, config.Facts, config.Resolver, config.KeySpace, args)
		if len(facts) == 0 {
			return callpayload.CallOutcome{}
		}
		return callpayload.CallOutcome{NormalReturnFacts: callboundary.NormalReturnFacts{LifecycleFacts: facts}}
	}
}

func typestateCallHasKnownSignature(ctx transfer.NodeContext, site factflow.CallSiteView, nameForSite SignatureSiteNameFunc, signatures SignatureLookup) bool {
	if signatures == nil || nameForSite == nil {
		return false
	}
	name, ok := nameForSite(ctx, site)
	if !ok || name == "" {
		return false
	}
	_, ok = signatures.Lookup(name)
	return ok
}

func typestateOpaqueEscapeFacts(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	ks *keyspace.KeySpace,
	args signatureArgumentReader,
) []callboundary.LifecycleFact {
	if resolver == nil || ks == nil {
		return nil
	}
	open := in.OpenTypestateObligations()
	if len(open) == 0 {
		return nil
	}
	targets := typestateOpaqueEscapeTargets(site, facts, ks, args)
	if len(targets) == 0 {
		return nil
	}
	seen := make(map[typestate.Resource]struct{}, len(open))
	var out []callboundary.LifecycleFact
	for _, target := range targets {
		stateKeys := typestateVisibleStateKeys(ctx.Point, resolver, target.path)
		if len(stateKeys) == 0 {
			continue
		}
		for _, obligation := range open {
			if typestateOpaqueCallModelsProtocol(site, obligation.Resource.Protocol) {
				continue
			}
			matched := false
			for _, stateKey := range stateKeys {
				resource := in.CanonicalTypestateResource(ks, stateKey, obligation.Resource.Protocol)
				if resource == obligation.Resource {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			if _, duplicate := seen[obligation.Resource]; duplicate {
				continue
			}
			seen[obligation.Resource] = struct{}{}
			out = append(out, callboundary.LifecycleFact{
				Target:   pathdom.NewPlaceholder(target.index),
				Kind:     callboundary.LifecycleEscape,
				Protocol: obligation.Resource.Protocol,
			})
		}
	}
	return out
}

// Channel methods are modeled by the ambient channel lifecycle provider even
// though they do not have manifest signatures. Do not pre-escape that resource
// as opaque before its declared open-state transition can run.
func typestateOpaqueCallModelsProtocol(site factflow.CallSiteView, protocol typestate.Protocol) bool {
	if protocol != ChannelLifecycleProtocol {
		return false
	}
	switch site.MethodName() {
	case "send", "close", "receive":
		return true
	default:
		return false
	}
}

type typestateOpaqueEscapeTarget struct {
	index int
	path  pathdom.Path
}

func typestateOpaqueEscapeTargets(site factflow.CallSiteView, facts factflow.Facts, ks *keyspace.KeySpace, args signatureArgumentReader) []typestateOpaqueEscapeTarget {
	var out []typestateOpaqueEscapeTarget
	if receiver, ok := site.ReceiverPath(); ok && !receiver.IsEmpty() {
		out = append(out, typestateOpaqueEscapeTarget{index: 0, path: receiver})
	}
	offset := 0
	if _, ok := site.ReceiverPath(); ok {
		offset = 1
	}
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		if !args.callArgumentSourceCanBindPath(source) {
			return true
		}
		path, ok := typestateSourcePath(facts, ks, source)
		if !ok || path.IsEmpty() {
			return true
		}
		out = append(out, typestateOpaqueEscapeTarget{index: index + offset, path: path})
		return true
	})
	return out
}

func typestateSourcePath(facts factflow.Facts, ks *keyspace.KeySpace, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		return facts.ExpressionPathRef(source.ExprRef)
	}
	if source.Kind != factflow.ValueSourcePath {
		return pathdom.Path{}, false
	}
	return sourcePathFromPathKey(ks, source.PathKey)
}

func typestateVisibleStateKeys(point cfg.Point, resolver *visibility.Resolver, target pathdom.Path) []pathaddr.StateKey {
	if target.Symbol == 0 || target.IsEmpty() {
		return nil
	}
	address := visibility.AddressAt(resolver, point, target)
	visible, visibleOK := address.VisibleStateKey()
	rootOrVisible, rootOrVisibleOK := address.RootOrVisibleStateKey()
	if !visibleOK && !rootOrVisibleOK {
		return nil
	}
	if visibleOK && rootOrVisibleOK && visible == rootOrVisible {
		return []pathaddr.StateKey{visible}
	}
	if visibleOK && rootOrVisibleOK {
		return []pathaddr.StateKey{visible, rootOrVisible}
	}
	if visibleOK {
		return []pathaddr.StateKey{visible}
	}
	return []pathaddr.StateKey{rootOrVisible}
}
