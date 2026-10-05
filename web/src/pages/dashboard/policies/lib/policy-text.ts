// SPDX-License-Identifier: AGPL-3.0-only
// Client-side policy text helpers: a line-based `path "x" { capabilities = [...] }` view of the
// rules array the API stores, a parser with diagnostics, and a port of internal/policy/policy.go's
// glob matcher so the access tester gives the same answer as the server for a single policy.
import type { PolicyRule } from '@shared/api/endpoints/policy'

export const CAPS = ['read', 'write', 'delete', 'list', 'deny'] as const
export type Cap = (typeof CAPS)[number]

export interface ParsedRule {
    caps: string[]
    line: number
    n: number
    path: string
}

export interface Diag {
    /** i18n key under `diag.` */
    key: string
    /** i18n key under `fix.` */
    fixKey?: string
    /** literal suggestion, shown as is */
    fixRaw?: string
    line: number
    n: number
    params?: Record<string, number | string>
    sev: 'e' | 'w'
}

export interface Parsed {
    diags: Diag[]
    errors: number
    lines: string[]
    rules: ParsedRule[]
    warns: number
}

export const NAME_RE = /^[a-z0-9][a-z0-9_-]{0,62}$/
export const RESERVED_NAMES = ['root']

export const TEMPLATES = {
    admin: { caps: ['read', 'write', 'delete', 'list'], path: 'secret/**' },
    ro: { caps: ['read', 'list'], path: 'secret/*' },
    rw: { caps: ['read', 'write', 'list'], path: 'secret/*' }
} as const
export type TemplateId = keyof typeof TEMPLATES

export function fmtRule(path: string, caps: readonly string[]): string {
    const ordered = CAPS.filter((c) => caps.includes(c))
    return `path "${path}" { capabilities = [${ordered.map((c) => `"${c}"`).join(', ')}] }`
}

export function rulesToText(rules: PolicyRule[] | undefined): string {
    return (rules ?? []).map((r) => fmtRule(r.path ?? '', r.capabilities ?? [])).join('\n')
}

const LINE_RE = /^path\s+"([^"]*)"\s*\{\s*capabilities\s*=\s*\[([^\]]*)\]\s*\}$/

export function parsePolicy(text: string): Parsed {
    const lines = text.split('\n')
    const rules: ParsedRule[] = []
    const diags: Diag[] = []
    const seen: Record<string, number> = {}

    lines.forEach((raw, i) => {
        const t = raw.trim()
        const n = i + 1
        if (!t || t[0] === '#') return
        const E = (key: string, extra: Partial<Diag> = {}) =>
            diags.push({ key, line: i, n, sev: 'e', ...extra })
        const W = (key: string, extra: Partial<Diag> = {}) =>
            diags.push({ key, line: i, n, sev: 'w', ...extra })

        if (((t.match(/"/g) ?? []).length & 1) === 1) {
            E('unclosedQuote', { fixKey: 'closeQuote' })
            return
        }
        const m = t.match(LINE_RE)
        if (!m) {
            if (!/^path\b/.test(t)) E('expectRule', { fixKey: 'startPath' })
            else if (!/\{/.test(t) || !/\}/.test(t)) E('noBrace', { fixKey: 'wrapBraces' })
            else if (!/capabilities/.test(t)) E('noCaps', { fixRaw: 'capabilities = ["read"]' })
            else if (!/\[/.test(t) || !/\]/.test(t)) E('capsArray', { fixRaw: '[ "read", "list" ]' })
            else E('syntax')
            return
        }
        const path = m[1]
        const rawCaps = m[2].trim()
        const items = rawCaps ? rawCaps.split(',') : []
        const caps: string[] = []
        let bad = false
        items.forEach((it) => {
            const c = it.trim()
            const mm = c.match(/^"([^"]*)"$/)
            if (!mm) {
                E('capQuoted', { fixRaw: '"read"', params: { cap: c } })
                bad = true
                return
            }
            const v = mm[1]
            if (!(CAPS as readonly string[]).includes(v)) {
                const key =
                    v === 'update' || v === 'create'
                        ? 'unknownCapWrite'
                        : v === 'sudo'
                          ? 'unknownCapSudo'
                          : 'unknownCap'
                E(key, { params: { cap: v } })
                bad = true
                return
            }
            if (!caps.includes(v)) caps.push(v)
        })
        if (path === '') E('emptyPath', { fixRaw: 'secret/…' })
        else if (path[0] === '/') E('leadingSlash', { fixKey: 'dropSlash' })
        if (/\*\*\*/.test(path)) E('tripleStar', { fixKey: 'useStars' })
        if (path.includes('+')) W('plusLiteral', { fixKey: 'useStars' })
        if (!bad) {
            if (!caps.length) W('emptyCaps')
            else if (caps.includes('deny') && caps.length > 1) W('denyOverrides', { fixKey: 'onlyDeny' })
            if (path && seen[path] !== undefined) W('dupPath', { fixKey: 'merge', params: { line: seen[path] } })
            if (path && seen[path] === undefined) seen[path] = n
            if (path && !/\*\*$/.test(path) && /\*$/.test(path) && path.split('/').length > 3) {
                W('singleStar', { fixKey: 'useDoubleStar' })
            }
            rules.push({ caps, line: i, n, path })
        }
    })
    const errors = diags.filter((d) => d.sev === 'e').length
    return { diags, errors, lines, rules, warns: diags.length - errors }
}

/** Port of globMatch in internal/policy/policy.go. */
export function globMatch(pat: string, s: string): boolean {
    while (pat.length > 0) {
        if (pat === '**') return true
        if (pat.startsWith('**/')) {
            const rest = pat.slice(3)
            if (globMatch(rest, s)) return true
            for (let i = 0; i < s.length; i++) {
                if (s[i] === '/' && globMatch(rest, s.slice(i + 1))) return true
            }
            return false
        }
        if (pat[0] === '*') {
            const rest = pat.slice(1)
            for (let k = 0; k <= s.length; k++) {
                if (k < s.length && s[k] === '/') break
                if (globMatch(rest, s.slice(k))) return true
            }
            return false
        }
        if (s.length === 0 || pat[0] !== s[0]) return false
        pat = pat.slice(1)
        s = s.slice(1)
    }
    return s.length === 0
}

export interface Evaluation {
    allow: boolean
    kind: 'allow' | 'deny' | 'nocap' | 'nomatch'
    matches: ParsedRule[]
    rule: null | ParsedRule
}

/** Same decision order as policy.Allowed: any matching deny wins, else any matching rule with the cap. */
export function evaluate(rules: ParsedRule[], path: string, cap: string): Evaluation {
    const matches = rules.filter((r) => globMatch(r.path, path))
    const deny = matches.find((r) => r.caps.includes('deny'))
    if (deny) return { allow: false, kind: 'deny', matches, rule: deny }
    const ok = matches.find((r) => r.caps.includes(cap))
    if (ok) return { allow: true, kind: 'allow', matches, rule: ok }
    return { allow: false, kind: matches.length ? 'nocap' : 'nomatch', matches, rule: null }
}

/** Concrete example path for a rule: wildcards replaced by a plausible segment. */
export function samplePath(pattern: string): string {
    return pattern.replace(/\*\*/g, 'example/item').replace(/\*/g, 'example')
}

export function pathScope(path: string): 'exact' | 'recursive' | 'segment' {
    if (path.includes('**')) return 'recursive'
    if (path.includes('*')) return 'segment'
    return 'exact'
}

/** Canonical form used to decide whether a draft differs from what is stored. */
export function canonical(rules: Array<{ caps: readonly string[]; path: string }>, inheritable: boolean): string {
    return JSON.stringify([
        inheritable,
        rules.map((r) => [r.path, CAPS.filter((c) => r.caps.includes(c))])
    ])
}

/* ---------- syntax highlighting ---------- */

export type SegKind = 'c' | 'cap' | 'capbad' | 'capdeny' | 'k' | 'kw2' | 'p' | 's' | 'text'
export interface Seg {
    kind: SegKind
    parts?: Array<{ kind?: 'badp' | 'g'; text: string }>
    text: string
}

const TOK = /(#.*$)|("[^"]*"?)|\b(path|capabilities)\b|([{}[\]=,])/g

export function highlightLine(raw: string): Seg[] {
    const out: Seg[] = []
    let inCaps = false
    let last = 0
    TOK.lastIndex = 0
    let m: null | RegExpExecArray
    while ((m = TOK.exec(raw)) !== null) {
        if (m.index > last) out.push({ kind: 'text', text: raw.slice(last, m.index) })
        last = m.index + m[0].length
        const [whole, comment, str, kw, punct] = m
        if (comment) out.push({ kind: 'c', text: whole })
        else if (kw) {
            if (kw === 'capabilities') inCaps = true
            out.push({ kind: kw === 'path' ? 'k' : 'kw2', text: kw })
        } else if (punct) {
            if (punct === ']') inCaps = false
            out.push({ kind: 'p', text: punct })
        } else if (str) {
            const inner = str.replace(/^"|"$/g, '')
            if (inCaps) {
                const known = (CAPS as readonly string[]).includes(inner)
                out.push({ kind: known ? (inner === 'deny' ? 'capdeny' : 'cap') : 'capbad', text: str })
            } else {
                const parts: Seg['parts'] = []
                for (const piece of str.split(/(\*+|\+)/)) {
                    if (!piece) continue
                    parts.push({ kind: piece[0] === '*' ? 'g' : piece === '+' ? 'badp' : undefined, text: piece })
                }
                out.push({ kind: 's', parts, text: str })
            }
        }
    }
    if (last < raw.length) out.push({ kind: 'text', text: raw.slice(last) })
    return out
}
