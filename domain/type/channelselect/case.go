package channelselect

import (
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

const (
	receiveCaseName = "channel.SelectReceiveCase"
	sendCaseName    = "channel.SelectSendCase"
)

// ReceiveCaseType is the type of a case_receive result. It is a nominal
// channel case, not a user record and not a select result member.
func ReceiveCaseType(channel, payload typ.Type) typ.Type {
	return instantiateCase(receiveCaseName, channel, payload)
}

// SendCaseType is the type of a case_send result.
func SendCaseType(channel, payload typ.Type) typ.Type {
	return instantiateCase(sendCaseName, channel, payload)
}

// CaseFromType decodes a nominal receive or send case. A user record with
// the public select-result fields is not a case.
func CaseFromType(t typ.Type) (ResultCase, bool) {
	inst, ok := unwrap.Alias(unwrap.Annotations(t)).(*typ.Instantiated)
	if !ok || inst.Generic == nil || !isSelectCaseName(inst.Generic.Name) || len(inst.TypeArgs) != 2 {
		return ResultCase{}, false
	}
	channel, payload := inst.TypeArgs[0], inst.TypeArgs[1]
	if channel == nil || payload == nil {
		return ResultCase{}, false
	}
	return ResultCase{Channel: channel, Payload: payload}, true
}

func isSelectCaseName(name string) bool {
	return name == receiveCaseName || name == sendCaseName
}

// CasesFromTable reads nominal receive/send cases from a select argument
// table. Array-only constructors are tuples; mixed tables are records.
// Array positions are case ordinals. A user record is skipped, not
// admitted. A literal default=true field is the default arm.
func CasesFromTable(t typ.Type) ([]ResultCase, bool, bool) {
	switch typed := unwrap.Alias(unwrap.Annotations(t)).(type) {
	case *typ.Tuple:
		return casesFromElements(typed.Elements), false, true
	case *typ.Record:
		return casesFromRecord(typed)
	default:
		return nil, false, false
	}
}

func casesFromRecord(record *typ.Record) ([]ResultCase, bool, bool) {
	if record == nil {
		return nil, false, false
	}
	var elements []typ.Type
	for ordinal := 0; ; ordinal++ {
		member := record.GetStaticIntIndex(int64(ordinal + 1))
		if member == nil || member.Type == nil {
			break
		}
		elements = append(elements, member.Type)
	}
	hasDefault := false
	if field := record.GetField(ResultDefaultField); field != nil {
		hasDefault = typ.TypeEquals(field.Type, typ.LiteralBool(true))
	}
	return casesFromElements(elements), hasDefault, true
}

func casesFromElements(elements []typ.Type) []ResultCase {
	var cases []ResultCase
	for ordinal, elem := range elements {
		arm, isCase := CaseFromType(elem)
		if !isCase {
			continue
		}
		arm.Index = ordinal
		cases = append(cases, arm)
	}
	return cases
}

func instantiateCase(name string, channel, payload typ.Type) typ.Type {
	if channel == nil {
		channel = typ.Never
	}
	if payload == nil {
		payload = typ.Never
	}
	c := typ.NewTypeParam("C", nil)
	p := typ.NewTypeParam("P", nil)
	return typ.Instantiate(typ.NewGeneric(name, []*typ.TypeParam{c, p}, typ.NewInterface(name, nil)), channel, payload)
}
