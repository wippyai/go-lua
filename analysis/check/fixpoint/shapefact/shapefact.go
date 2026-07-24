// Package shapefact owns the closed value-shape transport shared by the
// equation front and its existing publication kernels.
package shapefact

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const (
	tablePrefix  = "shape/table/v1/"
	targetPrefix = "shape/target/v1/"
)

// Member records the proven presence state for one static literal path. Value
// is meaningful only when Present. A false Present is Lua's nil-removal
// semantics, not an unknown member value.
type Member struct {
	Suffix  string `json:"suffix"`
	Present bool   `json:"present"`
	Value   string `json:"value,omitempty"`
}

// Table is a finite literal shape. Closed means the constructor had no
// unclassified key or open tail, so omitted static fields are proven absent.
type Table struct {
	Closed  bool     `json:"closed"`
	Members []Member `json:"members"`
}

func EncodeTable(in Table) ([]byte, bool) {
	out := Table{Closed: in.Closed, Members: append([]Member(nil), in.Members...)}
	sort.Slice(out.Members, func(i, j int) bool { return out.Members[i].Suffix < out.Members[j].Suffix })
	for index, member := range out.Members {
		if member.Suffix == "" || !segment.ValidFormattedSegments(member.Suffix) ||
			(index > 0 && out.Members[index-1].Suffix == member.Suffix) ||
			(member.Present && member.Value == "") || (!member.Present && member.Value != "") {
			return nil, false
		}
	}
	wire, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return []byte(tablePrefix + base64.RawURLEncoding.EncodeToString(wire)), true
}

func DecodeTable(value []byte) (Table, bool) {
	encoded := strings.TrimPrefix(string(value), tablePrefix)
	if encoded == string(value) || encoded == "" {
		return Table{}, false
	}
	wire, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Table{}, false
	}
	var out Table
	if json.Unmarshal(wire, &out) != nil {
		return Table{}, false
	}
	canonical, ok := EncodeTable(out)
	if !ok || string(canonical) != string(value) {
		return Table{}, false
	}
	return out, true
}

func IsTable(value []byte) bool { _, ok := DecodeTable(value); return ok }

// Lookup returns the exact static member state. If a closed shape has no such
// member, it proves the member absent.
func (t Table) Lookup(suffix string) (Member, bool) {
	index := sort.Search(len(t.Members), func(i int) bool { return t.Members[i].Suffix >= suffix })
	if index < len(t.Members) && t.Members[index].Suffix == suffix {
		return t.Members[index], true
	}
	if t.Closed && segment.ValidFormattedSegments(suffix) {
		return Member{Suffix: suffix}, true
	}
	return Member{}, false
}

func EncodeTarget(target typ.Type) ([]byte, bool) {
	if target == nil {
		return nil, false
	}
	wire, err := typ.EncodeCanonical(context.Background(), target)
	if err != nil || len(wire) == 0 {
		return nil, false
	}
	return []byte(targetPrefix + base64.RawURLEncoding.EncodeToString(wire)), true
}

func DecodeTarget(value []byte) (typ.Type, bool) {
	encoded := strings.TrimPrefix(string(value), targetPrefix)
	if encoded == string(value) || encoded == "" {
		return nil, false
	}
	wire, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false
	}
	// A shape target is a closed structural publication, not a declaration
	// identity witness. Recursive aliases therefore use the structural decoder:
	// it reconstructs fresh placeholders solely to compare the published graph.
	target, err := typ.DecodeCanonicalStructural(context.Background(), wire)
	if err != nil || target == nil {
		return nil, false
	}
	canonical, ok := EncodeTarget(target)
	if !ok || string(canonical) != string(value) {
		return nil, false
	}
	return target, true
}
