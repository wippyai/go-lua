type Node = {confidence: any, content: any, created_at: any, embedded: unknown, id: any, kb_id: any, metadata: unknown, node_type: any, title: any, name: any, summary: any, refs: any, source: any, node_count: any}
type Repo = {list_kbs: () -> ({Node}?, any?), list: (any) -> ({Node}?, any?), search_text: (any, any) -> ({Node}?, any?), get: (any) -> Node?}
return {} :: Repo
