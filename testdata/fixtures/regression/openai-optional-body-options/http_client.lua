type RequestOptions = {body?: string, headers?: {[string]: string}, timeout?: number, stream?: boolean}
type Client = {
    get: (string, RequestOptions) -> (any?, any?),
    delete: (string, RequestOptions) -> (any?, any?),
    put: (string, RequestOptions) -> (any?, any?),
    patch: (string, RequestOptions) -> (any?, any?),
    post: (string, RequestOptions) -> (any?, any?),
}
return {} :: Client
