export function toBase64Url(text: string): string {
    const b64 = btoa(unescape(encodeURIComponent(text)))
    return b64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function fromBase64Url(encoded: string): string {
    const padded = encoded.replace(/-/g, '+').replace(/_/g, '/')
    const b64 = padded + '='.repeat((4 - (padded.length % 4)) % 4)
    return decodeURIComponent(escape(atob(b64)))
}
