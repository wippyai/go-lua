package runtimekind

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

var Key = axis.NewKey[Value]("runtimekind")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:      Key,
		Bottom:   Bottom,
		Top:      Top,
		Equal:    Equal,
		LessOrEq: LessOrEq,
		Join:     Join,
		Meet:     Meet,
		Widen:    Widen,
		Hash:     Value.Hash,
	}
}

// Tag is one Lua runtime type() tag.
type Tag uint8

const (
	Nil Tag = iota
	Boolean
	Number
	String
	Table
	Function
	Thread
	Userdata

	tagCount
)

const allKnownMask uint16 = (1 << uint(tagCount)) - 1

var allTags = []Tag{
	Nil,
	Boolean,
	Number,
	String,
	Table,
	Function,
	Thread,
	Userdata,
}

var tagNames = [...]string{
	Nil:      "nil",
	Boolean:  "boolean",
	Number:   "number",
	String:   "string",
	Table:    "table",
	Function: "function",
	Thread:   "thread",
	Userdata: "userdata",
}

// Value is the RuntimeKind finite-set axis for Lua runtime type() evidence.
//
// Bottom is the empty set, Top is the set of all known Lua runtime tags, Join is
// set union, and refinement is set intersection.
type Value struct {
	mask uint16
}

func Bottom() Value {
	return Value{}
}

func Top() Value {
	return Value{mask: allKnownMask}
}

func Singleton(tag Tag) Value {
	return Value{mask: tagMask(tag)}
}

func (v Value) Without(tags ...Tag) Value {
	mask := v.mask & allKnownMask
	for _, tag := range tags {
		mask &^= tagMask(tag)
	}
	return Value{mask: mask}
}

func Intersect(a, b Value) Value {
	return Value{mask: a.mask & b.mask & allKnownMask}
}

func Meet(a, b Value) Value {
	return Intersect(a, b)
}

func (v Value) Contains(tag Tag) bool {
	if !validTag(tag) {
		return false
	}
	return v.mask&tagMask(tag) != 0
}

func (v Value) IsBottom() bool {
	return v.mask&allKnownMask == 0
}

func (v Value) IsTop() bool {
	return v.mask&allKnownMask == allKnownMask
}

func (v Value) Tags() []Tag {
	tags := make([]Tag, 0, len(allTags))
	for _, tag := range allTags {
		if v.Contains(tag) {
			tags = append(tags, tag)
		}
	}
	return tags
}

func Equal(a, b Value) bool {
	return a.mask&allKnownMask == b.mask&allKnownMask
}

func LessOrEq(a, b Value) bool {
	return b.Covers(a)
}

func Join(a, b Value) Value {
	return Value{mask: (a.mask | b.mask) & allKnownMask}
}

func Widen(prev, next Value) Value {
	return Join(prev, next)
}

func (v Value) Covers(other Value) bool {
	return other.mask&allKnownMask&^(v.mask&allKnownMask) == 0
}

func (v Value) Hash() uint64 {
	return internal.MixHash(internal.FnvString("runtimekind"), uint64(v.mask&allKnownMask))
}

func (t Tag) String() string {
	if !validTag(t) {
		return "runtimekind-tag(invalid)"
	}
	return tagNames[t]
}

// ParseTag converts a Lua runtime type() result string to its runtime kind tag.
func ParseTag(name string) (Tag, bool) {
	for _, tag := range allTags {
		if tagNames[tag] == name {
			return tag, true
		}
	}
	return 0, false
}

func (v Value) String() string {
	if v.IsBottom() {
		return "bottom"
	}
	if v.IsTop() {
		return "top"
	}
	names := make([]string, 0, len(allTags))
	for _, tag := range v.Tags() {
		names = append(names, tag.String())
	}
	return "runtimekind(" + strings.Join(names, "|") + ")"
}

func validTag(tag Tag) bool {
	return tag < tagCount
}

func tagMask(tag Tag) uint16 {
	if !validTag(tag) {
		panic("runtimekind: invalid tag")
	}
	return 1 << uint(tag)
}
