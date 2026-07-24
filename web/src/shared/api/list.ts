import { instance } from './axios'

// Tuck's list endpoints use a custom HTTP "LIST" method, with a GET+?list=true
// fallback for browser clients (fetch can send arbitrary methods, but many
// proxies/environments don't tolerate non-standard verbs) — see
// internal/api/list_compat.go. The prefix is part of the URL path, not a query param.
export async function listRequest(path: string): Promise<string[]> {
    const { data } = await instance.get<{ keys: string[] }>(path, { params: { list: 'true' } })
    return data.keys ?? []
}
