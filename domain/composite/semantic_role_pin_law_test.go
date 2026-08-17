package composite

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// pinnedSemanticRoles is the frozen semantic role catalog: every role the
// analyzer declares, and the exact digest its declared spelling derives.
//
// The identity of a role is a hash of the framing domain, the vocabulary
// format, and the role's own spelling. Every engine factor, rule slot, query
// codec, activation family, and contract payload in the analyzer is addressed
// by one of these digests, so a role whose spelling moves is a different
// identity everywhere it is bound, and one that is dropped is a binding no
// declaration reaches. The table below is what makes both visible: it is
// authored bytes, not a re-derivation, so a change to the derivation or to a
// spelling fails here rather than silently re-addressing the analyzer.
//
// A role is added by appending its row, and only by that.
func pinnedSemanticRoles() []struct {
	spelling string
	digest   string
} {
	return []struct {
		spelling string
		digest   string
	}{
		{"factor/value", "a4a0871ca0632b6ccf9c9be70fe5ab105f15ad4cfa514868201b4764563d9ade"},
		{"factor/value/summary-identity", "d5933a460b0cbf7226f6ce977744df7e1484a9cce7780ceababc384a83a84064"},
		{"factor/value/summary-coordinatewise", "326db6e5e34d18eb3afee9ab020b02ebe58c4a38d217e6621396bfd9e50f1ea1"},
		{"factor/call", "9886b56d905c067a64e7bae5f2d12e6fe7d979584a2ef43c6659837c245f0d4c"},
		{"factor/heap", "1f515724ed12315ac93af2328d0ec9e5d67c08b13305ec562118592bf8186012"},
		{"factor/pack", "f97ecfa99e2ff89952b20b830f3bebf32c70599ed587c86e8c64e06a3fcd0ad2"},
		{"factor/effect", "2deee72d175cafc112050245f018026b347208aef78e47cd10a4a5917353f8ea"},
		{"query/value-summary", "2cbe01e346b3a1e99978abbec40a84ef27b2f3e606ef7f913e6da7e22495450b"},
		{"query-result/value-summary", "414027b779753e9c960b11beaa1547bf2170134752cbdcbe9e9a609928d3b157"},
		{"query/effect-exact", "5d6c7c578a211eae1d3646bcc17a97eaa9c0f0a9a2a9340d0735f3a98ada4546"},
		{"query-result/effect-exact", "ce1fa5d4cb05e1ecc0412434bf2ee1a499a77e5de694f9add69ce5c4eb57406a"},
		{"activation/call-body", "497daa3410a748650435ebaa17f917445740c877095376a4e98e180fcfbace2a"},
		{"activation-family/call-body", "ede8d10431cea13e33cd897eaeca554977c24f3c8099bce4275cebcec78cee11"},
		{"activation-admission/call-body", "8e3b4292bbad23bcd67ef291f8e2e0e68fc6f38caf71216f4ca468c089d1d2fc"},
		{"rule/value/source", "656f0713f9651815f29e94dd8e279116fd0d611bdf7ac70124efb1b090bcccaf"},
		{"operand/value/source", "3794b586829df74ea1b67da5aa1af22275057af654ab4f1522c52ea5bac4c088"},
		{"evidence/value/source", "1daed20dbfc0bce65df3eb0ccab0ce5a56fb6f7bb693ad1784ce0a77bfb03390"},
		{"rule/pack/source", "fbe991525a006bd4974a399414c15de85fd9ad1cffe7292f103b959bcf970042"},
		{"operand/pack/source", "4f02746904b70a7d01f61631eedd99608c87697743810a7685da862a26b7b19b"},
		{"evidence/pack/source", "e29e7d2a159c3962a8511f87330790ce9d7264a2fe0fa4a92a05e798cffd6e1b"},
		{"rule/heap/allocation-ingress", "059cf5b5d6aab33801633e4fd8f99f82daa61c2deaf79b759777ea4d6b45f582"},
		{"operand/heap/allocation-ingress", "74c6db4186a212b252615455dc1b341877110ab9478fc25e963751d5ccb34fdc"},
		{"evidence/heap/allocation-ingress", "4d88d7a392c6e0d15f84634acdb782950c4e69a692612cb169836d3ebac4b532"},
		{"rule/heap/index-get-raw", "342d784b9d7ae1d039cbfca12f3d14f89154a51368cefbba42ae3d30509b0c04"},
		{"operand/heap/index-get-raw", "06678e9a8064dffa884aa6805399ad3b60a1c78bd6e50745ac214f423b470e6f"},
		{"evidence/heap/index-get-raw", "c3ab4763310e261a81b3b99a757379d52392c348940d33061722d3a09c05a0a5"},
		{"rule/heap/index-set-raw", "49885694873919bfabcb6cb268f01b44a16715030ddd1dfca4012457a5b151ad"},
		{"operand/heap/index-set-raw", "fbf57e4b4e786a8665cd2a127eae667ec0b8133775562a5a4641a85ecec96310"},
		{"evidence/heap/index-set-raw", "295bb7bfdd1971da855b0ebc0de5bbdca8b9392766cc7875c8a09f6e6fca7d11"},
		{"rule/call/dispatch", "686d572cd9322b8cc5365790ff544ecd3dcf840d2a8e199a95c0281264de17ba"},
		{"operand/call/dispatch", "51a3d67b5b63729ac9d1cdbc7df19f2ca5f46bc59e5292d213cfc7a86e2291a8"},
		{"evidence/call/dispatch", "f8ba9e2b74bfd7dd9e532ef0e30dc73b2621e79aa8a5c3b6d25121753e353ebd"},
		{"rule/effect/callsite-selected", "c405b9d78b84cf49ff45235a1b2f907ce6dfb0eb69549035271577feee3deaf2"},
		{"operand/effect/callsite-selected", "0ba8c514f0d32cdedebd40f3ec11e13f8aa00f25ca98876784037fa0dfa73ea5"},
		{"evidence/effect/callsite-selected", "5006d77de618986376bb32517786449b8be855c1ab4aec04c78e0474313092ee"},
		{"rule/effect/callsite-opaque", "1931cbc6eabdb6aa2f667e7abf5dc7b93efd33555eadcf5a289c7a01e42a4dfc"},
		{"operand/effect/callsite-opaque", "f25ad67906f004cfd9b99ec53233aa3f47c70d73bf2d9128d6e6a907613e00e2"},
		{"evidence/effect/callsite-opaque", "e03f886171fed98b8aa204e62630f72b69c1b049923039896905b0506507a320"},
		{"rule/effect/callsite-body", "f5310eb9c8fbbae8413d3cb479fb0c060b31521689bcbb6c90c539d5fc92dd0b"},
		{"operand/effect/callsite-body", "36dc11676b1966b82440a7b992c421294cb14876b4fe057d17a11a35a6a511af"},
		{"evidence/effect/callsite-body", "9d0e21afadba8c83a67b6298eb4f67f8ea78645c0f2e6c9d05f40217af76647d"},
		{"rule/value/host-global-bootstrap", "eac90237197362b11d8b814ced389fce335c02daa23a3f0db3b0374a1b9677e6"},
		{"operand/value/host-global-bootstrap", "387097d9f18e04d9c0a99b102bb11820fc7b58551fe007e341631e96f271d899"},
		{"evidence/value/host-global-bootstrap", "0672b4353ce78d1f7046915d9489c76ec92039af2dc7b92b3fe7b275a043fe36"},
		{"rule/heap/host-bootstrap", "ed7f18c425ec7800df5aa88f23b374748fed70c90e24eaf97c8070572c79104a"},
		{"operand/heap/host-bootstrap", "e49da3b6adb29c787c44e5430792dc4c2afcd94ce2d06cab311c46f879f40698"},
		{"evidence/heap/host-bootstrap", "bfcff90cc646face35e139312822169739114b71c324a7bf7653386e5f943443"},
		{"rule/value/storage-transfer", "e5edcb04cd92ee1aeb650976a6657cd671b4671807e6b872454c453434d4d86e"},
		{"operand/value/storage-transfer", "9720fcf25fe104c3d3b852c0c50178ca15325ffc6678447641d3905edfcb4bd2"},
		{"evidence/value/storage-transfer", "584dc3b6c0d32eadc4b58818f5c163589bf6ff1744385c6ae97ba7cec817390b"},
		{"rule/value/binary-arithmetic", "636d70a2c34bf701517aea8250dcbf4f3bf70418814a3248782089464d7109b4"},
		{"operand/value/binary-arithmetic", "4b18b5b6c945b2d879eaeaf93fe43ee827aca3bab9f8a08cf3d55fe910b4c5b6"},
		{"evidence/value/binary-arithmetic", "2d9e6b0efb4b33c0cd449410804ab82dbd8dcec71751e084d93c40a825b0931e"},
		{"rule/value/binary-equality", "56d180a7b0e6dedfeb259d7a19ba62f37f7c750e35831c0f09f43c490642645f"},
		{"operand/value/binary-equality", "696fdcd75fa39629c1b54cb808d4f5bf2766984a04cfd35a5df944f5641eff24"},
		{"evidence/value/binary-equality", "be0c50978092348e3c035312d44862eed3334236dfee8ae430036d57e0c62d2b"},
		{"rule/value/binary-order", "4d48bd4327b03a5c8b443eafd137602b7e2395bb8a4e754d7372f1305ad47a54"},
		{"operand/value/binary-order", "5c44879c104b138a1742ff2d403113a3467016a2886cd7dd523d0f3b65ee1f7c"},
		{"evidence/value/binary-order", "ff51fd2623bf3103ada9b6c6b04e0ac3854345b45167e0cd1e831ba34dd7565a"},
		{"rule/value/presence-refinement", "0f23cc7b39ddf321c5735e89a00519f0cb6ac7b7c3d910f281706179a21294ed"},
		{"operand/value/presence-refinement", "ed271fa1ee7045a93f267ef30db6366b3e7cf8b69bbb21634cfbf2d3f442f164"},
		{"evidence/value/presence-refinement", "23231d379d51404faa7d43d4db2996b2aed14c5e32e71a6661dec47df09de9df"},
		{"rule/value/allocation", "d282257d618cadf0af74da35110725e0661975589864be1fa391cd8aed7ebe00"},
		{"operand/value/allocation", "9fe53547b25c5d3cacb7f8f54b0331491aaf0f66aaabaea0375196dee401d7b1"},
		{"evidence/value/allocation", "c7508fc5ac235d3b07ab939a5a5241a96304e405d8ee9935584ac4c24c05f62a"},
		{"transform/value/allocation", "162604a098f75c215558c361ec5f5366a8f07ee0dcdcc0ad214f47acb8033138"},
		{"rule/heap/allocation-empty", "2c643d5dd01d168e9b4b054ec66640764b7f55c8df0890f59f640d35af41f6a6"},
		{"operand/heap/allocation-empty", "99d6722def85c1c4bcfb775babc846a2d3a2b24e7f5b9cf5c2caf982034fa634"},
		{"evidence/heap/allocation-empty", "1e51b725165827c5fec8d9cd52371ca3d7d51d13ba2ac9d8ab90c707831d2c4b"},
		{"transform/heap/allocation-empty", "b582b33607bfb31c1301876f4ed1096ee6c088e7295a7e25319f87406ec71da4"},
		{"rule/heap/allocation-closed", "cafa7b0934ffabf1c42dc96a132bf20731eb6db01b2addf45f97dacd60afde3d"},
		{"operand/heap/allocation-closed", "5e666958a3bdf06ae661eb777a657090ebc93024006667988f4cb16a79667ba6"},
		{"evidence/heap/allocation-closed", "0c3422593466ff9b40a98bab7cf39e1bafe7db9d5444488f1ce22a584c8dea41"},
		{"transform/heap/allocation-closed", "44673b10251ac80327461319eed21fb7ab2c01a00dfb488c53459f8649888d9a"},
		{"axis/execution-reachability", "d31d901aa14d3abfc6eb87d1ffd0cc75cf908cff5d353cf7290912ce55658132"},
		{"axis/denominator-count", "61d6e0d98ed9eaf888181252ec8d1fb420b8220ffa43560d04c82e63c714136f"},
	}
}

// TestSemanticRoleDigestsArePinned states the derivation half of the freeze:
// every pinned spelling derives exactly the pinned bytes, under exactly the
// pinned format. The identity every engine binding is addressed by is these
// bytes, so a change to the framing, the format, or a spelling is caught here
// with the role that moved named.
func TestSemanticRoleDigestsArePinned(t *testing.T) {
	for _, pinned := range pinnedSemanticRoles() {
		semantic, ok := vocabulary.Key(pinned.spelling)
		if !ok {
			t.Fatalf("pinned role %q derives no identity", pinned.spelling)
		}
		digest := semantic.Digest()
		if got := hex.EncodeToString(digest[:]); got != pinned.digest {
			t.Fatalf("role %q derives %s, pinned %s", pinned.spelling, got, pinned.digest)
		}
		if semantic.Version() != vocabulary.SemanticFormat {
			t.Fatalf("role %q derives format %d, want %d", pinned.spelling, semantic.Version(), vocabulary.SemanticFormat)
		}
	}
}

// TestDeclaredSemanticRolesAreExactlyThePinnedCatalog states the declaration
// half: the sealed table's semantic role rows are the pinned catalog, member
// for member, and each row resolves to the identity its own pinned spelling
// derives. A role dropped by a domain, a role spelled differently, and a role
// added without a pin all fail here.
func TestDeclaredSemanticRolesAreExactlyThePinnedCatalog(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table published no structural surface")
	}
	roles, rolesOK := SemanticRoles()
	if !rolesOK {
		t.Fatal("sealed table resolved no semantic role vocabulary")
	}
	declared := make(map[string]schema.Key)
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*structure.Entry)
		if !rowOK || !entryOK || entry.Category() != structure.CategorySemanticRole {
			continue
		}
		if prior, duplicate := declared[entry.Spelling()]; duplicate {
			t.Fatalf("roles %q and %q share the spelling %q", prior, entry.Key(), entry.Spelling())
		}
		declared[entry.Spelling()] = entry.Key()
	}
	pinned := pinnedSemanticRoles()
	if len(declared) != len(pinned) {
		t.Fatalf("the table declares %d semantic roles, %d are pinned", len(declared), len(pinned))
	}
	for _, row := range pinned {
		key, present := declared[row.spelling]
		if !present {
			t.Fatalf("pinned role %q is declared by no row", row.spelling)
		}
		if key != vocabulary.RoleKey(row.spelling) {
			t.Fatalf("pinned role %q is declared under key %q", row.spelling, key)
		}
		semantic, resolved := roles.Key(key)
		if !resolved {
			t.Fatalf("declared role %q resolves to no identity", row.spelling)
		}
		digest := semantic.Digest()
		if got := hex.EncodeToString(digest[:]); got != row.digest {
			t.Fatalf("declared role %q resolves to %s, pinned %s", row.spelling, got, row.digest)
		}
	}
}

// TestEverySurfaceIdentityIsOneDeclaredRole is the cross-surface half of the
// uniqueness law. Each surface proves its own rows distinct, and the spelling
// law proves the roles distinct, but only a walk across the surfaces shows that
// no two entries anywhere in the table are declared under one role.
func TestEverySurfaceIdentityIsOneDeclaredRole(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	roles, rolesOK := SemanticRoles()
	if !rolesOK {
		t.Fatal("sealed table resolved no semantic role vocabulary")
	}
	claimed := make(map[schema.Key]schema.Key)
	claim := func(t *testing.T, owner, role schema.Key) {
		t.Helper()
		if _, resolved := roles.Key(role); !resolved {
			t.Fatalf("entry %q is declared under role %q, which no row declares", owner, role)
		}
		if prior, duplicate := claimed[role]; duplicate {
			t.Fatalf("entries %q and %q are both declared under role %q", prior, owner, role)
		}
		claimed[role] = owner
	}
	for _, entry := range registry.axes {
		claim(t, entry.Key(), entry.Semantic())
		for index := 0; index < entry.RoleCount(); index++ {
			role, roleOK := entry.RoleAt(index)
			if !roleOK {
				t.Fatalf("axis %q holds no role at %d", entry.Key(), index)
			}
			if _, resolved := roles.Key(role); !resolved {
				t.Fatalf("axis %q consumes role %q, which no row declares", entry.Key(), role)
			}
		}
	}
	for _, entry := range registry.templates {
		claim(t, entry.Key(), entry.Semantic())
		for index := 0; index < entry.RoleCount(); index++ {
			role, roleOK := entry.RoleAt(index)
			if !roleOK {
				t.Fatalf("rule %q holds no role at %d", entry.Key(), index)
			}
			if _, resolved := roles.Key(role); !resolved {
				t.Fatalf("rule %q consumes role %q, which no row declares", entry.Key(), role)
			}
		}
	}
	if len(claimed) == 0 {
		t.Fatal("no surface entry is declared under a role")
	}
}
