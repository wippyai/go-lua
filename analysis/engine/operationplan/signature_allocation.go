package operationplan

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SignatureAllocationSite is durable lexical provenance. Caller scope and
// concrete identities are supplied only during Relation specialization.
type SignatureAllocationSite struct {
	Template signature.AllocationTemplateID
	Ordinal  uint32
}

type SignatureAllocationOperation struct {
	site     SignatureAllocationSite
	template signature.ReturnAllocationTemplate
}

func NewSignatureAllocationOperation(site SignatureAllocationSite, template signature.ReturnAllocationTemplate) (SignatureAllocationOperation, bool) {
	if site.Template == "" || site.Ordinal == 0 || template.Root == "" || site.Template != template.Root || len(template.Objects) == 0 {
		return SignatureAllocationOperation{}, false
	}
	return SignatureAllocationOperation{site: site, template: cloneReturnAllocationTemplate(template)}, true
}

func (o SignatureAllocationOperation) Site() SignatureAllocationSite { return o.site }
func (o SignatureAllocationOperation) Template() signature.ReturnAllocationTemplate {
	return cloneReturnAllocationTemplate(o.template)
}
func (o SignatureAllocationOperation) valid() bool {
	return o.site.Template != "" && o.site.Ordinal != 0 && o.template.Root == o.site.Template && len(o.template.Objects) != 0
}
func (o SignatureAllocationOperation) clone() SignatureAllocationOperation {
	o.template = cloneReturnAllocationTemplate(o.template)
	return o
}

func cloneReturnAllocationTemplate(in signature.ReturnAllocationTemplate) signature.ReturnAllocationTemplate {
	out := in
	out.Objects = make([]signature.AllocationObjectTemplate, len(in.Objects))
	for i, object := range in.Objects {
		out.Objects[i] = object
		out.Objects[i].StaticMembers = make([]signature.AllocationStaticMemberTemplate, len(object.StaticMembers))
		for j, member := range object.StaticMembers {
			out.Objects[i].StaticMembers[j] = member
			out.Objects[i].StaticMembers[j].Suffix = append([]segment.Segment(nil), member.Suffix...)
		}
		out.Objects[i].DynamicEntries = append([]signature.AllocationDynamicEntryTemplate(nil), object.DynamicEntries...)
	}
	return out
}

func (p *Plan) WithSignatureAllocations(input map[cfg.Point]SignatureAllocationOperation) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.signatureAllocationRefs = make([]uint32, len(p.rows))
	out.signatureAllocationOrdinals = make([]uint32, len(p.rows))
	out.signatureAllocationTemplates = make([]signature.ReturnAllocationTemplate, 0, len(input))
	buckets := make(map[uint64][]uint32, len(input))
	for rawPoint := range p.rows {
		op, ok := input[cfg.Point(rawPoint)]
		if !ok || !op.valid() {
			continue
		}
		digest := returnAllocationTemplateDigest(op.template)
		var ref uint32
		for _, candidate := range buckets[digest] {
			if returnAllocationTemplateEqual(out.signatureAllocationTemplates[candidate-1], op.template) {
				ref = candidate
				break
			}
		}
		if ref == 0 {
			out.signatureAllocationTemplates = append(out.signatureAllocationTemplates, cloneReturnAllocationTemplate(op.template))
			ref = uint32(len(out.signatureAllocationTemplates))
			buckets[digest] = append(buckets[digest], ref)
		}
		out.signatureAllocationRefs[rawPoint] = ref
		out.signatureAllocationOrdinals[rawPoint] = op.site.Ordinal
	}
	return &out
}

func (p *Plan) SignatureAllocationOperation(point cfg.Point) (SignatureAllocationOperation, bool) {
	if p == nil || uint64(point) >= uint64(len(p.signatureAllocationRefs)) || len(p.signatureAllocationOrdinals) != len(p.signatureAllocationRefs) {
		return SignatureAllocationOperation{}, false
	}
	ref := p.signatureAllocationRefs[point]
	if ref == 0 || int(ref) > len(p.signatureAllocationTemplates) || p.signatureAllocationOrdinals[point] == 0 {
		return SignatureAllocationOperation{}, false
	}
	template := p.signatureAllocationTemplates[ref-1]
	return SignatureAllocationOperation{
		site:     SignatureAllocationSite{Template: template.Root, Ordinal: p.signatureAllocationOrdinals[point]},
		template: cloneReturnAllocationTemplate(template),
	}, true
}

func returnAllocationTemplateEqual(a, b signature.ReturnAllocationTemplate) bool {
	return (signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{a}}).Equals(
		signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{b}},
	)
}

func returnAllocationTemplateDigest(template signature.ReturnAllocationTemplate) uint64 {
	h := internalhash.FnvString(string(template.Root))
	h = internalhash.MixHash(h, uint64(template.ReturnIndex+1))
	for _, object := range template.Objects {
		h = internalhash.MixHash(h, internalhash.FnvString(string(object.ID)))
		if object.Type != nil {
			h = internalhash.MixHash(h, typ.EqualityHash(object.Type))
		}
		if object.StableShape {
			h = internalhash.MixHash(h, 1)
		}
		if object.PrefixStable {
			h = internalhash.MixHash(h, 2)
		}
		for _, member := range object.StaticMembers {
			h = internalhash.MixHash(h, internalhash.FnvString(string(member.Value)))
			for _, suffix := range member.Suffix {
				h = internalhash.MixHash(h, uint64(suffix.Kind))
				h = internalhash.MixHash(h, internalhash.FnvString(suffix.Name))
				h = internalhash.MixHash(h, uint64(suffix.Index))
			}
		}
		for _, entry := range object.DynamicEntries {
			h = internalhash.MixHash(h, internalhash.FnvString(string(entry.Key)))
			h = internalhash.MixHash(h, internalhash.FnvString(string(entry.Value)))
			if entry.KeyType != nil {
				h = internalhash.MixHash(h, typ.EqualityHash(entry.KeyType))
			}
		}
	}
	return h
}
