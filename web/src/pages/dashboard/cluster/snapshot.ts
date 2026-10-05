// SPDX-License-Identifier: AGPL-3.0-only
import { instance } from '@shared/api/axios'

import { pad2 } from './lib'

export interface SnapshotResult {
    bytes: number
    filename: string
    savedAt: number
}

/**
 * Downloads GET /v1/sys/snapshot and hands the file to the browser.
 * `onProgress` receives 0..1 when the server sends Content-Length, otherwise bytes received.
 */
export async function downloadSnapshot(
    onProgress: (p: { bytes: number; ratio: null | number }) => void
): Promise<SnapshotResult> {
    const { data } = await instance.get<Blob>('/v1/sys/snapshot', {
        onDownloadProgress: (e) => onProgress({ bytes: e.loaded, ratio: e.total ? e.loaded / e.total : null }),
        responseType: 'blob'
    })
    const now = new Date()
    const filename = `tuck-snapshot-${now.getFullYear()}${pad2(now.getMonth() + 1)}${pad2(now.getDate())}-${pad2(now.getHours())}${pad2(now.getMinutes())}.db`
    const url = URL.createObjectURL(data)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 10_000)
    return { bytes: data.size, filename, savedAt: now.getTime() }
}
