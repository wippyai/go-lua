package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: a dynamic write into a declared map reached through a field
// path keeps the map's declared value type. Storing a refinement of that type
// must not widen the value domain into a union with the declaration.
func TestDeclaredMapFieldIndexerWriteKeepsValueType(t *testing.T) {
	source := `
		type OrderAggregate = {
			id: string,
			customer: string,
			updated_at: number?,
		}
		type StoreState = {
			orders: {[string]: OrderAggregate},
		}
		type OrderStore = {
			state: StoreState,
			ensure_order: (self: OrderStore, id: string, customer: string, at: number) -> OrderAggregate,
		}
		type Store = OrderStore

		local Store = {}
		Store.__index = Store

		function Store:ensure_order(id: string, customer: string, at: number): OrderAggregate
			local current = self.state.orders[id]
			if current then
				if current.updated_at == nil then
					current.updated_at = at
				end
				return current
			end

			local created: OrderAggregate = {
				id = id,
				customer = customer,
				updated_at = at,
			}
			self.state.orders[id] = created
			return created
		end

		return {Store = Store}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
