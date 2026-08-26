// Package selectapply applies channel.select on a sealed Program.
package selectapply

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/domain/type/ambient"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typecall"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

const (
	selectMethod      = "select"
	caseReceiveMethod = "case_receive"
	caseSendMethod    = "case_send"
	walkLimit         = 8
)

// Application is one specialized channel.select on a parent-issued call site.
type Application struct {
	Index  int
	Site   identity.ContentID
	Result typ.Type
	Facts  channelselect.CaseSet
}

// Apply specializes each channel.select call on program. The site is the
// schema-issued call identity. Facts come from typecall.ApplyCall. A user select
// or a lookalike table member is not an accepted arm.
func Apply(prog *program.Program) []Application {
	if prog == nil {
		return nil
	}
	calls := prog.Flow().Authored().Calls()
	var apps []Application
	for index := 0; index < calls.Count(); index++ {
		call, callOK := calls.At(index)
		_, callee, receiver, actuals, rowOK := calls.Get(call)
		if !callOK || !rowOK || !isChannelModuleSelect(prog, callee, receiver) {
			continue
		}
		identities, siteOK := prog.CallIdentityAt(index)
		site := identities.Call
		if !siteOK || !site.Available() {
			continue
		}
		tableType, tableOK := selectArgumentType(prog, actuals)
		if !tableOK {
			continue
		}
		result, facts, ok := typecall.ApplyCall(channelselect.SelectFunction(), site, []typ.Type{tableType})
		if !ok {
			continue
		}
		apps = append(apps, Application{Index: index, Site: site, Result: result, Facts: facts})
	}
	return apps
}

// isChannelModuleSelect names the call by the exact member the callee selects.
// The selected name is the authored Lens key, so a qualified callee spelling
// and a debug spelling are both outside this admission.
func isChannelModuleSelect(prog *program.Program, callee, receiver keyspace.Term) bool {
	if receiver != 0 {
		return false
	}
	if keyspace.TermFamily(callee) != keyspace.FamilyRead {
		return false
	}
	_, source, _, readOK := prog.Flow().Authored().Storage().Reads().Get(callee)
	if !readOK || keyspace.TermFamily(source) != keyspace.FamilyLensExact {
		return false
	}
	_, base, key, fieldKind, lensOK := prog.Flow().Authored().Access().Exact().Get(source)
	if !lensOK || fieldKind != flowkind.FieldName {
		return false
	}
	_, member, _, memberOK := prog.Source().Keys().Name(key)
	if !memberOK || member != selectMethod {
		return false
	}
	cell, cellOK := cellOf(prog, base, 0)
	if !cellOK {
		return false
	}
	kind, _, _, kindOK := prog.Flow().Authored().Storage().Cells().Get(cell)
	if !kindOK || kind != authored.CellGlobal {
		return false
	}
	cellName, namedCell := prog.Source().Spellings().CellName(cell)
	return namedCell && cellName == channelselect.ModuleName
}

func selectArgumentType(prog *program.Program, actuals keyspace.Term) (typ.Type, bool) {
	first, ok := prog.Flow().Authored().Values().Member(actuals, 0)
	if !ok || keyspace.TermFamily(first) != keyspace.FamilyTable {
		return nil, false
	}
	tables := prog.Flow().Authored().Tables()
	fields := prog.Flow().Authored().Fields()
	keys := prog.Source().Keys()
	count, countOK := tables.FieldCount(first)
	if !countOK || count == 0 {
		return nil, false
	}
	var entries []typetable.ConstructorEntry
	listOrdinal := int64(1)
	for index := 0; index < count; index++ {
		field, fieldOK := tables.FieldAt(first, index)
		if !fieldOK {
			return nil, false
		}
		_, key, values, fieldKind, fieldRowOK := fields.Get(field)
		if !fieldRowOK {
			return nil, false
		}
		switch fieldKind {
		case flowkind.FieldList:
			_, ordinal, _, listOK := keys.List(key)
			if !listOK {
				ordinal = listOrdinal
			}
			listOrdinal++
			value, valueOK := valuesAt(prog, values, 0)
			if !valueOK {
				return nil, false
			}
			entries = append(entries, typetable.ConstructorEntry{
				Path: []typetable.ConstructorKey{{Kind: typetable.ConstructorIntIndex, Index: ordinal}},
				Type: caseTypeOrAny(prog, value),
			})
		case flowkind.FieldName:
			_, name, _, nameOK := keys.Name(key)
			if !nameOK || name != channelselect.ResultDefaultField {
				continue
			}
			value, valueOK := valuesAt(prog, values, 0)
			if !valueOK || !literalTrue(prog, value) {
				continue
			}
			entries = append(entries, typetable.ConstructorEntry{
				Path: []typetable.ConstructorKey{{Kind: typetable.ConstructorField, Name: name}},
				Type: typ.LiteralBool(true),
			})
		}
	}
	return typetable.ConstructorType(entries)
}

func caseTypeOrAny(prog *program.Program, value keyspace.Term) typ.Type {
	if keyspace.TermFamily(value) != keyspace.FamilyCall {
		return typ.Any
	}
	name, named := prog.Source().Spellings().CallName(value)
	if !named || (name != caseReceiveMethod && name != caseSendMethod) {
		return typ.Any
	}
	_, _, receiver, _, rowOK := prog.Flow().Authored().Calls().Get(value)
	if !rowOK || receiver == 0 {
		return typ.Any
	}
	cell, cellOK := cellOf(prog, receiver, 0)
	if !cellOK {
		return typ.Any
	}
	receiverType, typeOK := declaredType(prog, cell)
	if !typeOK {
		return typ.Any
	}
	member, status := typecall.MemberCall(receiverType, name)
	if status != typecall.MemberCallOK {
		return typ.Any
	}
	fn, ok := typecall.Callable(member)
	if !ok || fn == nil || len(fn.Returns) == 0 || fn.Returns[0] == nil {
		return typ.Any
	}
	if _, isCase := channelselect.CaseFromType(fn.Returns[0]); !isCase {
		return typ.Any
	}
	return fn.Returns[0]
}

func declaredType(prog *program.Program, cell keyspace.Term) (typ.Type, bool) {
	declared, ok := prog.Static().Declarations().DeclaredTypes().ForCell(cell)
	if !ok {
		return nil, false
	}
	_, target, rowOK := prog.Static().Declarations().DeclaredTypes().Get(declared)
	if !rowOK || target == 0 {
		return nil, false
	}
	decoded := decodeStaticType(prog, target, 0)
	return decoded, decoded != nil
}

func decodeStaticType(prog *program.Program, term keyspace.Term, depth int) typ.Type {
	if prog == nil || term == 0 || depth > walkLimit {
		return nil
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyTypeOptional:
		inner, ok := prog.Static().Types().Optionals().Get(term)
		if !ok {
			return nil
		}
		decoded := decodeStaticType(prog, inner, depth+1)
		if decoded == nil {
			return nil
		}
		return typeexpr.Optional(decoded)
	case keyspace.FamilyTypeGeneric:
		base, args, ok := prog.Static().Types().Generics().Get(term)
		if !ok || args != 1 {
			return nil
		}
		name, named := typeRefName(prog, base)
		if !named || !ambient.IsRuntimeChannelName(name) {
			return nil
		}
		arg, argOK := prog.Static().Types().Generics().ArgAt(term, 0)
		if !argOK {
			return nil
		}
		payload := decodeStaticType(prog, arg, depth+1)
		if payload == nil {
			return nil
		}
		return typ.Instantiate(ambient.ChannelGeneric(), payload)
	case keyspace.FamilyTypePrimitive:
		kind, ok := prog.Static().Types().Primitives().Get(term)
		if !ok {
			return nil
		}
		return primitiveType(kind)
	case keyspace.FamilyTypeAlias:
		_, _, nameKey, _, ok := prog.Static().Declarations().Aliases().Get(term)
		if !ok {
			return nil
		}
		name, named := keyString(prog, nameKey)
		if !named {
			return nil
		}
		return typ.NewRef("", name)
	case keyspace.FamilyTypeRef:
		resolution, target, _, ok := prog.Static().References().Get(term)
		if ok && resolution == staticrefs.Declaration && target != 0 {
			return decodeStaticType(prog, target, depth+1)
		}
		name, named := typeRefName(prog, term)
		if !named {
			return nil
		}
		if i := strings.LastIndex(name, "."); i >= 0 {
			return typ.NewRef(name[:i], name[i+1:])
		}
		return typ.NewRef("", name)
	default:
		return nil
	}
}

func primitiveType(kind statictypes.PrimitiveKind) typ.Type {
	switch kind {
	case statictypes.PrimitiveNil:
		return typ.Nil
	case statictypes.PrimitiveBoolean:
		return typ.Boolean
	case statictypes.PrimitiveNumber:
		return typ.Number
	case statictypes.PrimitiveInteger:
		return typ.Integer
	case statictypes.PrimitiveString:
		return typ.String
	case statictypes.PrimitiveAny:
		return typ.Any
	case statictypes.PrimitiveUnknown:
		return typ.Unknown
	case statictypes.PrimitiveNever:
		return typ.Never
	case statictypes.PrimitiveSelf:
		return typ.Self
	default:
		return nil
	}
}

func typeRefName(prog *program.Program, term keyspace.Term) (string, bool) {
	refs := prog.Static().References()
	count, ok := refs.SourceCount(term)
	if !ok || count == 0 {
		return "", false
	}
	parts := make([]string, 0, count)
	for index := 0; index < count; index++ {
		key, keyOK := refs.SourceAt(term, index)
		if !keyOK {
			return "", false
		}
		name, named := keyString(prog, key)
		if !named {
			return "", false
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, "."), true
}

func keyString(prog *program.Program, key keyspace.Key) (string, bool) {
	value, ok := prog.Source().Keys().Exact(key)
	return value.String, ok && value.Kind == keyspace.LiteralString && value.String != ""
}

func cellOf(prog *program.Program, term keyspace.Term, depth int) (keyspace.Term, bool) {
	if prog == nil || term == 0 || depth > walkLimit {
		return 0, false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyCell:
		return term, true
	case keyspace.FamilyRead:
		_, source, _, ok := prog.Flow().Authored().Storage().Reads().Get(term)
		if !ok {
			return 0, false
		}
		return cellOf(prog, source, depth+1)
	case keyspace.FamilyLensExact:
		_, base, _, _, ok := prog.Flow().Authored().Access().Exact().Get(term)
		if !ok {
			return 0, false
		}
		return cellOf(prog, base, depth+1)
	default:
		return 0, false
	}
}

func valuesAt(prog *program.Program, values keyspace.Term, index int) (keyspace.Term, bool) {
	pos, ok := prog.Flow().Authored().Values().Position(values, index)
	if !ok {
		return 0, false
	}
	if pos.Fixed != 0 {
		return pos.Fixed, true
	}
	if pos.Tail != 0 && pos.TailOffset == 0 {
		return pos.Tail, true
	}
	return 0, false
}

func literalTrue(prog *program.Program, term keyspace.Term) bool {
	if keyspace.TermFamily(term) != keyspace.FamilyBool {
		return false
	}
	bools := prog.Source().Literals().Bools()
	for index := 0; index < bools.Count(); index++ {
		got, _, value, ok := bools.At(index)
		if ok && got == term {
			return value
		}
	}
	return false
}
