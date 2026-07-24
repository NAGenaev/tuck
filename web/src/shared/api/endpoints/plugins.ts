import { instance } from '../axios'

export type PluginType = 'auth' | 'database' | 'secret'

export interface PluginEntry {
    name: string
    type: PluginType
    command: string
    sha256: string
    args?: string[]
    env?: string[]
    builtin: boolean
    version?: string
    registered_at: string
}

// NOTE: both the by-type and all-types list variants return the same shape,
// {"plugins":[full entries]} — not {"keys":[names]} as the spec implies.
export async function listPlugins(type?: PluginType): Promise<PluginEntry[]> {
    const { data } = await instance.get<{ plugins: PluginEntry[] }>(
        `/v1/sys/plugins/catalog/${type ?? ''}`,
        { params: { list: 'true' } }
    )
    return data.plugins ?? []
}

export async function registerPlugin(
    type: PluginType,
    name: string,
    body: { command: string; sha256: string; args?: string[]; version?: string }
): Promise<void> {
    // The 204 response here has a literal "null" JSON body (unlike other 204s in
    // this API which are truly empty) — axios doesn't care either way.
    await instance.post(`/v1/sys/plugins/catalog/${type}/${name}`, body)
}

export async function deletePlugin(type: PluginType, name: string): Promise<void> {
    await instance.delete(`/v1/sys/plugins/catalog/${type}/${name}`)
}
