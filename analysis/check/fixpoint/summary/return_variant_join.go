package summary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	typelit "github.com/wippyai/go-lua/analysis/type/literal"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func joinedTaggedReturnValue(reg *axis.Registry, joined, left, right product.Value) (product.Value, bool) {
	t, ok := joinedReturnVariantType(reg, left, right)
	if !ok {
		return product.Value{}, false
	}
	value := typevalue.WithWitness(reg, joined, t)
	value = product.Set(reg, value, variantorigin.Key, variantorigin.Top())
	return value, true
}

func joinedReturnVariantType(reg *axis.Registry, left, right product.Value) (typ.Type, bool) {
	leftType, leftOK := typevalue.TypeOf(reg, left)
	rightType, rightOK := typevalue.TypeOf(reg, right)
	if !leftOK || !rightOK {
		return nil, false
	}
	records, ok := returnRecordAlternatives(leftType, rightType)
	if !ok || len(records) < 2 {
		return nil, false
	}
	if field, ok := commonLiteralDiscriminantField(records); ok {
		members, ok := groupReturnRecordsByLiteralField(records, field)
		if !ok {
			return nil, false
		}
		// Each tag originates at a syntactic return site, so the set stays finite.
		return returnVariantCandidate(records, typenormalize.UnionForEvidence(members...))
	}
	return nil, false
}

func returnRecordAlternatives(types ...typ.Type) ([]*typ.Record, bool) {
	var records []*typ.Record
	var collect func(typ.Type) bool
	collect = func(t typ.Type) bool {
		switch t := typ.UnwrapTransparentWrappers(t).(type) {
		case *typ.Union:
			for _, member := range t.Members {
				if !collect(member) {
					return false
				}
			}
			return true
		case *typ.Record:
			records = append(records, t)
			return true
		default:
			return false
		}
	}
	for _, t := range types {
		if !collect(t) {
			return nil, false
		}
	}
	return records, true
}

func commonLiteralDiscriminantField(records []*typ.Record) (string, bool) {
	if len(records) < 2 {
		return "", false
	}
	counts := make(map[string]map[uint64]struct{})
	for i, record := range records {
		fields := literalDiscriminantFields(record)
		if len(fields) == 0 {
			return "", false
		}
		if i == 0 {
			for name, lit := range fields {
				counts[name] = map[uint64]struct{}{typ.EqualityHash(lit): {}}
			}
			continue
		}
		for name, values := range counts {
			lit, ok := fields[name]
			if !ok {
				delete(counts, name)
				continue
			}
			values[typ.EqualityHash(lit)] = struct{}{}
		}
		if len(counts) == 0 {
			return "", false
		}
	}
	var names []string
	for name, values := range counts {
		if len(values) >= 2 {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", false
	}
	sort.Slice(names, func(i, j int) bool {
		left, right := len(counts[names[i]]), len(counts[names[j]])
		if left != right {
			return left > right
		}
		return names[i] < names[j]
	})
	return names[0], true
}

func literalDiscriminantFields(record *typ.Record) map[string]typ.Type {
	if record == nil {
		return nil
	}
	out := make(map[string]typ.Type)
	for _, field := range record.Fields {
		if field.Optional {
			continue
		}
		if lit, ok := typ.UnwrapTransparentWrappers(field.Type).(*typ.Literal); ok {
			out[field.Name] = lit
		}
	}
	return out
}

func groupReturnRecordsByLiteralField(records []*typ.Record, field string) ([]typ.Type, bool) {
	type group struct {
		records []*typ.Record
	}
	groupsByHash := make(map[uint64]*group)
	for _, record := range records {
		member := record.GetField(field)
		if member == nil || member.Optional {
			return nil, false
		}
		lit, ok := typ.UnwrapTransparentWrappers(member.Type).(*typ.Literal)
		if !ok {
			return nil, false
		}
		hash := typ.EqualityHash(lit)
		g := groupsByHash[hash]
		if g == nil {
			g = &group{}
			groupsByHash[hash] = g
		}
		g.records = append(g.records, record)
	}
	hashes := make([]uint64, 0, len(groupsByHash))
	for hash := range groupsByHash {
		hashes = append(hashes, hash)
	}
	sort.Slice(hashes, func(i, j int) bool { return hashes[i] < hashes[j] })
	out := make([]typ.Type, 0, len(hashes))
	for _, hash := range hashes {
		joined, ok := joinReturnRecords(groupsByHash[hash].records)
		if !ok {
			return nil, false
		}
		out = append(out, joined)
	}
	return out, true
}

func joinReturnRecords(records []*typ.Record) (typ.Type, bool) {
	if len(records) == 0 {
		return nil, false
	}
	if !returnRecordMetadataCompatible(records) {
		return nil, false
	}
	fieldAccs := make(map[string]returnRecordFieldAcc)
	staticAccs := make(map[returnRecordStaticKey]returnRecordStaticAcc)
	for _, record := range records {
		for _, field := range record.Fields {
			acc := fieldAccs[field.Name]
			if acc.count == 0 {
				acc.field = field
			} else {
				acc.field = joinReturnRecordField(acc.field, field)
			}
			acc.count++
			fieldAccs[field.Name] = acc
		}
		for _, member := range record.StaticMembers {
			key := returnRecordStaticKeyOf(member)
			acc := staticAccs[key]
			if acc.count == 0 {
				acc.member = member
			} else {
				acc.member = joinReturnRecordStaticMember(acc.member, member)
			}
			acc.count++
			staticAccs[key] = acc
		}
	}
	fields := make([]typ.Field, 0, len(fieldAccs))
	for _, acc := range fieldAccs {
		if acc.count < len(records) {
			acc.field.Optional = true
		}
		fields = append(fields, acc.field)
	}
	staticMembers := make([]typ.StaticMember, 0, len(staticAccs))
	for _, acc := range staticAccs {
		if acc.count < len(records) {
			acc.member.Optional = true
		}
		staticMembers = append(staticMembers, acc.member)
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	sort.Slice(staticMembers, func(i, j int) bool {
		return typ.CompareStaticMembers(staticMembers[i], staticMembers[j]) < 0
	})
	first := records[0]
	return typ.RebuildRecord(typ.RecordParts{
		Fields:        fields,
		StaticMembers: staticMembers,
		Metatable:     first.Metatable,
		MapKey:        first.MapKey,
		MapValue:      first.MapValue,
		Open:          first.Open,
		AssumeSorted:  true,
	}), true
}

type returnRecordFieldAcc struct {
	field typ.Field
	count int
}

type returnRecordStaticAcc struct {
	member typ.StaticMember
	count  int
}

type returnRecordStaticKey struct {
	kind  typ.StaticMemberKind
	name  string
	index int64
}

func returnRecordStaticKeyOf(member typ.StaticMember) returnRecordStaticKey {
	return returnRecordStaticKey{kind: member.Kind, name: member.Name, index: member.Index}
}

func joinReturnRecordField(left, right typ.Field) typ.Field {
	return typ.Field{
		Name:     left.Name,
		Type:     joinReturnRecordMemberType(left.Type, right.Type),
		Optional: left.Optional || right.Optional,
		Readonly: left.Readonly && right.Readonly,
	}
}

func joinReturnRecordStaticMember(left, right typ.StaticMember) typ.StaticMember {
	return typ.StaticMember{
		Kind:     left.Kind,
		Name:     left.Name,
		Index:    left.Index,
		Type:     joinReturnRecordMemberType(left.Type, right.Type),
		Optional: left.Optional || right.Optional,
		Readonly: left.Readonly && right.Readonly,
	}
}

func joinReturnRecordMemberType(left, right typ.Type) typ.Type {
	if typ.TypeEquals(left, right) {
		return left
	}
	return typenormalize.UnionForEvidence(left, right)
}

func returnRecordMetadataCompatible(records []*typ.Record) bool {
	first := records[0]
	for _, record := range records[1:] {
		if first.Open != record.Open ||
			!typ.TypeEquals(first.Metatable, record.Metatable) ||
			!typ.TypeEquals(first.MapKey, record.MapKey) ||
			!typ.TypeEquals(first.MapValue, record.MapValue) {
			return false
		}
	}
	return true
}

func returnVariantCandidate(records []*typ.Record, candidate typ.Type) (typ.Type, bool) {
	if candidate == nil {
		return nil, false
	}
	for _, record := range records {
		if !returnTypeLeq(record, candidate) {
			return nil, false
		}
	}
	return candidate, true
}

func returnTypeLeq(sub, super typ.Type) bool {
	if typ.SameNodeOrAcyclicEqual(sub, super) || typ.TypeEquals(sub, super) {
		return true
	}
	sub = typ.UnwrapTransparentWrappers(sub)
	super = typ.UnwrapTransparentWrappers(super)
	if typ.SameNodeOrAcyclicEqual(sub, super) || typ.TypeEquals(sub, super) {
		return true
	}
	if union, ok := super.(*typ.Union); ok {
		for _, member := range union.Members {
			if returnTypeLeq(sub, member) {
				return true
			}
		}
		return false
	}
	if union, ok := sub.(*typ.Union); ok {
		for _, member := range union.Members {
			if !returnTypeLeq(member, super) {
				return false
			}
		}
		return true
	}
	if opt, ok := super.(*typ.Optional); ok {
		return typ.TypeEquals(sub, typ.Nil) || returnTypeLeq(sub, opt.Inner)
	}
	if opt, ok := sub.(*typ.Optional); ok {
		return returnTypeLeq(typ.Nil, super) && returnTypeLeq(opt.Inner, super)
	}
	if base, ok := typelit.FamilyBase(sub); ok &&
		(typ.SameNodeOrAcyclicEqual(base, super) || typ.TypeEquals(base, super)) {
		return true
	}
	subRecord, subOK := sub.(*typ.Record)
	superRecord, superOK := super.(*typ.Record)
	if subOK && superOK {
		return returnRecordLeq(subRecord, superRecord)
	}
	return false
}

func returnRecordLeq(sub, super *typ.Record) bool {
	if sub == nil || super == nil {
		return false
	}
	if !super.Open && sub.Open {
		return false
	}
	if super.Metatable != nil && !returnTypeLeq(sub.Metatable, super.Metatable) {
		return false
	}
	if super.MapKey != nil {
		if sub.MapKey == nil || !returnTypeLeq(sub.MapKey, super.MapKey) {
			return false
		}
	}
	if super.MapValue != nil {
		if sub.MapValue == nil || !returnTypeLeq(sub.MapValue, super.MapValue) {
			return false
		}
	}
	for _, want := range super.Fields {
		got := sub.GetField(want.Name)
		if got == nil {
			if want.Optional {
				continue
			}
			return false
		}
		if got.Optional && !want.Optional {
			return false
		}
		if want.Readonly && !got.Readonly {
			return false
		}
		if !returnTypeLeq(got.Type, want.Type) {
			return false
		}
	}
	for _, want := range super.StaticMembers {
		got := sub.GetStaticMember(want.Kind, want.Name, want.Index)
		if got == nil {
			if want.Optional {
				continue
			}
			return false
		}
		if got.Optional && !want.Optional {
			return false
		}
		if want.Readonly && !got.Readonly {
			return false
		}
		if !returnTypeLeq(got.Type, want.Type) {
			return false
		}
	}
	return true
}
