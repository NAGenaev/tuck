const TOKEN_KEY = 'tuck_token'
const NAMESPACE_KEY = 'tuck_namespace'

export function getToken(): null | string {
    return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string) {
    localStorage.setItem(TOKEN_KEY, token)
}

export function removeToken() {
    localStorage.removeItem(TOKEN_KEY)
}

export function getNamespace(): null | string {
    return localStorage.getItem(NAMESPACE_KEY)
}

export function setNamespace(namespace: string) {
    if (namespace) {
        localStorage.setItem(NAMESPACE_KEY, namespace)
    } else {
        localStorage.removeItem(NAMESPACE_KEY)
    }
}
