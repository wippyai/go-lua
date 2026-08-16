package link

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	"github.com/wippyai/go-lua/analysis/program/target"
)

const (
	linkArtifactDomain = "program/link/artifact"
	// v35 replaces authored Host coordinates with its portable detached replay
	// contract. The Link content policy remains independent at v34.
	linkArtifactCodecVersion   = 35
	linkArtifactMaxBytes       = 256 << 20
	linkArtifactMaxEvents      = 16 << 20
	linkArtifactMaxStringBytes = 64 << 20
	linkArtifactMaxModules     = 1 << 20

	// These fixed portable admission charges govern hostile artifact opening
	// only. They are deliberately conservative policy units, not a claim about
	// exact Go heap bytes. They are never analysis convergence limits and never
	// affect ordinary Link Seal. Every charged dimension comes exactly from the
	// serialized rows and their resolved immutable Program/target identities.
	linkArtifactMaxReconstructionBytes = 128 << 20
	linkArtifactModuleBytes            = 4096
	linkArtifactTargetOperationBytes   = 4096
	linkArtifactTargetPathBytes        = 1024
	linkArtifactProgramTermBytes       = 128
	linkArtifactProgramKeyBytes        = 256
	linkArtifactProjectRowBytes        = 512
	// Cache deployment rows are decoded from untrusted artifact bytes, then
	// copied into the sealed Link projection. Charge the complete structural
	// path before a decoded collection receives capacity; payload bytes are
	// charged separately by linkArtifactDecoder.string before conversion.
	linkArtifactActorSpecBytes              = 512
	linkArtifactAliasClassBytes             = 1024
	linkArtifactAliasMemberBytes            = 512
	linkArtifactAnalysisRootSpecBytes       = 768
	linkArtifactModuleCacheEntrySpecBytes   = 512
	linkArtifactHostEndpointBytes           = 768
	linkArtifactProviderCapabilityBytes     = 384
	linkArtifactProviderCapabilitySeedBytes = 896
	linkArtifactHostSelectorBytes           = 512
)

var (
	ErrArtifactUnavailable = errors.New("link artifact: unavailable Link")
	ErrArtifactTarget      = errors.New("link artifact: target identity mismatch")
	ErrArtifactProgram     = errors.New("link artifact: unavailable Program")
	ErrArtifactCanonical   = errors.New("link artifact: noncanonical encoding")
	ErrArtifactLimit       = errors.New("link artifact: resource limit")
)

// EncodeArtifact produces Link's sole persistence representation. It stores
// only the identities needed to run the ordinary Link Seal again: target,
// claimed Link, and the canonical module rows. Project relations, sigma
// projections, seeds, keys, and caches remain Seal-derived and absent.
func EncodeArtifact(link *Link) ([]byte, error) {
	if err := validateArtifactLink(link); err != nil {
		return nil, err
	}
	return encodeLinkArtifactBounded(link, linkArtifactMaxBytes)
}

// DecodeArtifact resolves already-sealed Programs by ContentID and performs
// exactly one ordinary Link Seal. Nothing is returned until the target,
// dependencies, claimed Link identity, and canonical wire bytes all agree.
func DecodeArtifact(data []byte, contract *target.Contract, programs map[identity.ContentID]*program.Program) (*Link, error) {
	if contract == nil || !contract.ContentID().Available() {
		return nil, ErrArtifactTarget
	}
	if len(data) > linkArtifactMaxBytes {
		return nil, ErrArtifactLimit
	}
	link, err := decodeLinkArtifact(data, contract, programs)
	if err != nil {
		if errors.Is(err, framing.ErrLimit) {
			return nil, ErrArtifactLimit
		}
		if errors.Is(err, ErrArtifactTarget) || errors.Is(err, ErrArtifactProgram) || errors.Is(err, ErrArtifactLimit) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrArtifactCanonical, err)
	}
	return link, nil
}

func validateArtifactLink(link *Link) error {
	if link == nil || link.boundary == nil || !link.id.Available() {
		return ErrArtifactUnavailable
	}
	contract, targetOK := link.boundary.Target()
	if !targetOK || contract == nil || !contract.ContentID().Available() {
		return ErrArtifactUnavailable
	}
	if link.project == nil || link.project.Mounts().Count() > linkArtifactMaxModules {
		return ErrArtifactLimit
	}
	budget, ok := newLinkArtifactBudget(contract)
	if !ok {
		return ErrArtifactLimit
	}
	mounts := link.project.Mounts()
	seen := make(map[string]struct{}, mounts.Count())
	var priorID identity.ContentID
	priorName := ""
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		name, nameOK := mounts.Name(shard)
		mounted, programOK := mounts.Program(shard)
		if !shardOK || !nameOK || !programOK || name == "" || mounted == nil {
			return ErrArtifactUnavailable
		}
		id := mounted.ContentID()
		if !id.Available() || (index != 0 && compareArtifactModule(priorID, priorName, id, name) >= 0) {
			return ErrArtifactUnavailable
		}
		if !budget.module(name, mounted) {
			return ErrArtifactLimit
		}
		if _, duplicate := seen[name]; duplicate {
			return ErrArtifactUnavailable
		}
		seen[name] = struct{}{}
		priorID, priorName = id, name
	}
	moduleSpec, moduleOK := link.module.Cold().Spec()
	if !moduleOK || !budget.cache(moduleSpec) {
		return ErrArtifactLimit
	}
	hostInput, hostOK := link.host.Cold().ReplaySpec()
	if !hostOK || !budget.host(hostInput) {
		return ErrArtifactLimit
	}
	if !budget.endpointRequests(link.boundary.EndpointRequests()) {
		return ErrArtifactLimit
	}
	return nil
}

func (budget *linkArtifactBudget) host(host linkhost.ReplaySpec) bool {
	if budget == nil || !budget.reserve(uint64(len(host.Capabilities)), linkArtifactProviderCapabilityBytes) ||
		!budget.reserve(uint64(len(host.Seeds)), linkArtifactProviderCapabilitySeedBytes) ||
		!budget.reserve(uint64(len(host.Exposures)+len(host.Members)), linkArtifactHostSelectorBytes) {
		return false
	}
	for _, capability := range host.Capabilities {
		if capability == "" || !budget.string(uint64(len(capability))) {
			return false
		}
	}
	for _, seed := range host.Seeds {
		if seed.Capability == "" || !budget.string(uint64(len(seed.Capability))) || !budget.string(uint64(len(seed.InitialRoot))) {
			return false
		}
		switch seed.Source {
		case linkhost.ProviderCapabilitySourceInitialRoot:
			if seed.InitialRoot == "" || seed.InputFormal.Available() || seed.OutcomeResult.Available() || seed.Value.Available() {
				return false
			}
		case linkhost.ProviderCapabilitySourceABIInput:
			if seed.InitialRoot != "" || !seed.InputFormal.Available() || seed.OutcomeResult.Available() || seed.Value.Available() {
				return false
			}
		case linkhost.ProviderCapabilitySourceResult:
			if seed.InitialRoot != "" || seed.InputFormal.Available() || !seed.OutcomeResult.Available() || seed.Value.Available() {
				return false
			}
		case linkhost.ProviderCapabilitySourceExposure:
			if seed.InitialRoot != "" || seed.InputFormal.Available() || seed.OutcomeResult.Available() || !seed.Value.Available() {
				return false
			}
		default:
			return false
		}
	}
	for _, item := range host.Exposures {
		if !item.Value.Available() || !item.Endpoint.Available() || item.Dispatch != linkhost.HostDispatchLookup {
			return false
		}
	}
	for _, item := range host.Members {
		if item.Capability == "" || !item.Value.Available() || !item.Endpoint.Available() || item.Dispatch != linkhost.HostDispatchLookup || !budget.string(uint64(len(item.Capability))) {
			return false
		}
	}
	return true
}

func (budget *linkArtifactBudget) endpointRequests(requests linkboundary.EndpointRequests) bool {
	if budget == nil || !budget.reserve(uint64(requests.Count()), linkArtifactHostEndpointBytes) {
		return false
	}
	for index := 0; index < requests.Count(); index++ {
		endpoint, ok := requests.At(index)
		if !ok {
			return false
		}
		// hostParts admits each untrusted owner/member element before it is
		// appended during Decode. Encode admission must reserve the same
		// structural rows, otherwise an emitted artifact can be unopenable.
		if !budget.reserve(uint64(len(endpoint.Binding.Owner)), linkArtifactHostSelectorBytes) ||
			!budget.reserve(uint64(len(endpoint.Binding.Member)), linkArtifactHostSelectorBytes) ||
			!budget.string(uint64(len(endpoint.Identity))) {
			return false
		}
		for _, part := range endpoint.Binding.Owner {
			if !budget.string(uint64(len(part))) {
				return false
			}
		}
		for _, part := range endpoint.Binding.Member {
			if !budget.string(uint64(len(part))) {
				return false
			}
		}
	}
	return true
}

type linkArtifactBudget struct {
	bytes uint64
}

func newLinkArtifactBudget(contract *target.Contract) (linkArtifactBudget, bool) {
	var budget linkArtifactBudget
	if contract == nil {
		return budget, false
	}
	return budget, true
}

func (budget *linkArtifactBudget) module(name string, sealed *program.Program) bool {
	if budget == nil || sealed == nil || name == "" ||
		!budget.reserve(1, linkArtifactModuleBytes) ||
		!budget.reserve(uint64(len(name)), 1) ||
		!budget.reserve(uint64(sealed.Source().Identity().TermCount()), linkArtifactProgramTermBytes) ||
		!budget.reserve(uint64(sealed.Source().Keys().ExactCount()), linkArtifactProgramKeyBytes) {
		return false
	}
	flow := sealed.Flow()
	// Link materializes Program Call/import rows and sealed function-style
	// meta/generic rows. Target endpoints remain Target-owned constituents;
	// Link never materializes their product with source Applications.
	projectRows := uint64(flow.Authored().Calls().Count())
	if imports := uint64(sealed.Module().Count()); projectRows > ^uint64(0)-imports {
		return false
	} else {
		projectRows += imports
	}
	candidates := flow.Candidates()
	metaRows := uint64(candidates.Unary().NumericCount() + candidates.Unary().LengthCount() + candidates.Binary().ArithmeticCount() + candidates.Binary().BitwiseCount() + candidates.Binary().ConcatCount() + candidates.Binary().EqualityCount() + candidates.Access().GetCount() + candidates.Access().SetCount())
	orders := candidates.Binary()
	binaries := flow.Authored().Operators().Binaries()
	successors := flow.Causal().Successors()
	outcomes := flow.Outcomes()
	for index := 0; index < orders.OrderCount(); index++ {
		term, ok := orders.OrderAt(index)
		if !ok {
			return false
		}
		metaRows++
		owner, op, _, _, binaryOK := binaries.Get(term)
		if !binaryOK {
			return false
		}
		if op == flowkind.BinaryLessEqual || op == flowkind.BinaryGreaterEqual {
			normalOK := successors.Count(term) > 0
			if normalOK {
				_, normalOK = successors.At(term, 0)
			}
			_, throwOK := outcomes.BodyExit(owner, flowkind.OutcomeThrow)
			_, yieldOK := outcomes.BodyExit(owner, flowkind.OutcomeYield)
			_, cancelOK := outcomes.BodyExit(owner, flowkind.OutcomeCancel)
			if normalOK && throwOK && yieldOK && cancelOK {
				metaRows++
			}
		}
	}
	loops := flow.Authored().Control().Loops()
	for index := 0; index < loops.Count(); index++ {
		term, ok := loops.At(index)
		if !ok {
			return false
		}
		_, _, loopKind, _, loopOK := loops.Get(term)
		if !loopOK {
			return false
		}
		if loopKind == flowkind.LoopGenericFor {
			metaRows++
		}
	}
	if projectRows > ^uint64(0)-metaRows {
		return false
	}
	projectRows += metaRows
	return budget.reserve(projectRows, linkArtifactProjectRowBytes)
}

func (budget *linkArtifactBudget) reserve(count, width uint64) bool {
	if budget == nil || width == 0 || count > ^uint64(0)/width {
		return false
	}
	need := count * width
	if budget.bytes > linkArtifactMaxReconstructionBytes || need > linkArtifactMaxReconstructionBytes-budget.bytes {
		return false
	}
	budget.bytes += need
	return true
}

// string reserves the one owned Go-string payload copy made by artifact
// opening. The row charges above cover headers, map buckets, and all Seal
// projection copies; this charge covers only attacker-selected payload bytes.
func (budget *linkArtifactBudget) string(width uint64) bool {
	return width == 0 || budget.reserve(1, width)
}

// cache is the encode-side mirror of cache decoding. An accepted artifact
// must fit the same fixed reconstruction ledger that opening will consume;
// otherwise persistence would publish a byte stream it can never reopen.
func (budget *linkArtifactBudget) cache(cache linkmodule.Spec) bool {
	if budget == nil ||
		!budget.reserve(uint64(len(cache.Actors)), linkArtifactActorSpecBytes) ||
		!budget.reserve(uint64(len(cache.ModuleCacheAliases)), linkArtifactAliasClassBytes) ||
		!budget.reserve(uint64(len(cache.AnalysisRoots)), linkArtifactAnalysisRootSpecBytes) ||
		!budget.reserve(uint64(len(cache.ModuleCacheEntries)), linkArtifactModuleCacheEntrySpecBytes) {
		return false
	}
	for _, actor := range cache.Actors {
		if !budget.string(uint64(len(actor.Name))) {
			return false
		}
	}
	for _, alias := range cache.ModuleCacheAliases {
		if !budget.string(uint64(len(alias.Actor))) ||
			!budget.string(uint64(len(alias.Representative))) ||
			!budget.reserve(uint64(len(alias.Instances)), linkArtifactAliasMemberBytes) {
			return false
		}
		for _, instance := range alias.Instances {
			if !budget.string(uint64(len(instance))) {
				return false
			}
		}
	}
	for _, root := range cache.AnalysisRoots {
		if !budget.string(uint64(len(root.Name))) ||
			!budget.string(uint64(len(root.Module))) ||
			!budget.string(uint64(len(root.Actor))) ||
			!budget.string(uint64(len(root.Instance))) {
			return false
		}
	}
	for _, entry := range cache.ModuleCacheEntries {
		if !budget.string(uint64(len(entry.Module))) ||
			!budget.string(uint64(len(entry.FromRoot))) ||
			!budget.string(uint64(len(entry.ToRoot))) {
			return false
		}
	}
	return true
}

func artifactMeasureAllowed(measure framing.StreamMeasure) bool {
	return measure.Events <= linkArtifactMaxEvents && measure.StringBytes <= linkArtifactMaxStringBytes
}

type linkArtifactBuffer struct {
	data  bytes.Buffer
	limit int
}

func (buffer *linkArtifactBuffer) Write(payload []byte) (int, error) {
	if buffer == nil || buffer.limit < 0 || len(payload) > buffer.limit-buffer.data.Len() {
		return 0, ErrArtifactLimit
	}
	return buffer.data.Write(payload)
}

func (buffer *linkArtifactBuffer) WriteString(value string) (int, error) {
	if buffer == nil || buffer.limit < 0 || len(value) > buffer.limit-buffer.data.Len() {
		return 0, ErrArtifactLimit
	}
	return buffer.data.WriteString(value)
}

func compareArtifactModule(leftID identity.ContentID, leftName string, rightID identity.ContentID, rightName string) int {
	if order := bytes.Compare(leftID[:], rightID[:]); order != 0 {
		return order
	}
	if leftName < rightName {
		return -1
	}
	if leftName > rightName {
		return 1
	}
	return 0
}
