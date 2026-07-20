package channelselectfact

import "testing"

func TestDomainJoinPreservesSharedFactSetIdentity(t *testing.T) {
	lane := Top().Add(Fact{Select: ID("select-1"), Kind: FactSelect})
	domain := Domain()
	if domain.Same == nil || !domain.Same(domain.Join(lane, lane), lane) {
		t.Fatal("channel-select domain did not preserve the shared persistent fact set")
	}
}
