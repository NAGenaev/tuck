// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Switch } from '@mantine/core'
import { ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { cx, Icon } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import {
    CAPS,
    Diag,
    evaluate,
    fmtRule,
    highlightLine,
    ParsedRule,
    Parsed,
    pathScope,
    Seg,
    SegKind,
    samplePath,
    TEMPLATES,
    TemplateId
} from '../lib/policy-text'
import s from '../policies.module.css'
import { AccessTester } from './access-tester'

const SEG_CLASS: Record<SegKind, string | undefined> = {
    c: s.tc,
    cap: s.tcap,
    capbad: s.tcapBad,
    capdeny: s.tcapDeny,
    k: s.tk,
    kw2: s.tkw,
    p: s.tp,
    s: s.ts,
    text: undefined
}

function HlLine({ segs }: { segs: Seg[] }) {
    if (!segs.length) return <>{' '}</>
    return (
        <>
            {segs.map((seg, i) => (
                <span className={SEG_CLASS[seg.kind]} key={i}>
                    {seg.parts
                        ? seg.parts.map((p, j) => (
                              <span
                                  className={p.kind === 'g' ? s.tg : p.kind === 'badp' ? s.tbadp : undefined}
                                  key={j}
                              >
                                  {p.text}
                              </span>
                          ))
                        : seg.text}
                </span>
            ))}
        </>
    )
}

function DiagMessage({ d }: { d: Diag }) {
    const { t } = useTranslation('policies')
    return (
        <span className={s.diagMsg}>
            <Trans components={{ b: <b /> }} i18nKey={`diag.${d.key}`} ns="policies" t={t} values={d.params} />
            {(d.fixKey || d.fixRaw) && (
                <span className={s.fix}> → {d.fixKey ? t(`fix.${d.fixKey}`) : d.fixRaw}</span>
            )}
        </span>
    )
}

const LH = 22

export function PolicyEditor({
    name,
    isNew,
    text,
    inheritable,
    parsed,
    dirty,
    saving,
    onText,
    onInheritable,
    onSave,
    onReset,
    onDelete
}: {
    dirty: boolean
    inheritable: boolean
    isNew: boolean
    name: string
    onDelete: () => void
    onInheritable: (v: boolean) => void
    onReset: () => void
    onSave: () => void
    onText: (text: string) => void
    parsed: Parsed
    saving: boolean
    text: string
}) {
    const { t } = useTranslation('policies')
    const taRef = useRef<HTMLTextAreaElement>(null)
    const hlRef = useRef<HTMLDivElement>(null)
    const gutRef = useRef<HTMLDivElement>(null)
    const mxRef = useRef<HTMLTableSectionElement>(null)

    const [focused, setFocused] = useState(false)
    const [caret, setCaret] = useState({ col: 1, line: 0 })
    const [flash, setFlash] = useState<null | { id: number; line: number }>(null)
    const [mflash, setMflash] = useState<null | { id: number; line: number }>(null)
    const flashId = useRef(0)
    const pendingFocus = useRef<null | number>(null)
    const pendingScroll = useRef(false)

    // tester state: until the user touches it, it follows the first sample derived from the rules
    const samples = useMemo(() => {
        const out: Array<{ cap: string; path: string }> = []
        const seen = new Set<string>()
        for (const r of parsed.rules) {
            if (!r.path || r.path[0] === '/') continue
            const path = samplePath(r.path)
            const cap = r.caps.find((c) => c !== 'deny') ?? 'read'
            const key = `${path}|${cap}`
            if (seen.has(key)) continue
            seen.add(key)
            out.push({ cap, path })
            if (out.length >= 6) break
        }
        return out
    }, [parsed.rules])
    const [tPath, setTPath] = useState<null | string>(null)
    const [tCap, setTCap] = useState<null | string>(null)
    const [beam, setBeam] = useState(0)
    const path = (tPath ?? samples[0]?.path ?? '').trim()
    const cap = tCap ?? samples[0]?.cap ?? 'read'
    const ev = useMemo(() => (path ? evaluate(parsed.rules, path, cap) : null), [parsed.rules, path, cap])
    const hit = ev?.rule?.line ?? null

    const userPath = (v: string) => {
        setTCap(cap)
        setTPath(v)
        setBeam((b) => b + 1)
    }
    const userCap = (c: string) => {
        setTPath(tPath ?? path)
        setTCap(c)
        setBeam((b) => b + 1)
    }
    const userSample = (p: string, c: string) => {
        setTPath(p)
        setTCap(c)
        setBeam((b) => b + 1)
    }

    const sev = useMemo(() => {
        const m: Record<number, 'e' | 'w'> = {}
        parsed.diags.forEach((d) => {
            if (m[d.line] !== 'e') m[d.line] = d.sev
        })
        return m
    }, [parsed.diags])
    const sortedDiags = useMemo(
        () =>
            parsed.diags
                .slice()
                .sort((a, b) => (a.sev === b.sev ? a.line - b.line : a.sev === 'e' ? -1 : 1)),
        [parsed.diags]
    )
    const hlLines = useMemo(() => parsed.lines.map((l) => highlightLine(l)), [parsed.lines])

    const syncScroll = () => {
        const ta = taRef.current
        if (!ta) return
        if (hlRef.current) {
            hlRef.current.scrollTop = ta.scrollTop
            hlRef.current.scrollLeft = ta.scrollLeft
        }
        if (gutRef.current) gutRef.current.scrollTop = ta.scrollTop
    }

    const readCaret = () => {
        const ta = taRef.current
        if (!ta) return
        const before = ta.value.slice(0, ta.selectionStart).split('\n')
        setCaret({ col: before[before.length - 1].length + 1, line: before.length - 1 })
    }

    const doFlash = (line: number) => {
        const id = ++flashId.current
        setFlash({ id, line })
        window.setTimeout(() => setFlash((f) => (f?.id === id ? null : f)), 1000)
        return id
    }
    const doMflash = (line: number) => {
        const id = ++flashId.current
        setMflash({ id, line })
        window.setTimeout(() => setMflash((f) => (f?.id === id ? null : f)), 1000)
    }

    const gotoLine = (i: number) => {
        const ta = taRef.current
        if (!ta) return
        const lines = ta.value.split('\n')
        let pos = 0
        for (let k = 0; k < i; k++) pos += lines[k].length + 1
        ta.focus()
        ta.setSelectionRange(pos, pos + (lines[i]?.length ?? 0))
        ta.scrollTop = Math.max(0, i * LH - 90)
        syncScroll()
        setCaret({ col: 1, line: i })
        doFlash(i)
    }

    const appendLine = (line: string) => {
        const base = text.replace(/\s+$/, '')
        const next = (base ? `${base}\n` : '') + line
        const n = next.split('\n').length - 1
        onText(next)
        doFlash(n)
        doMflash(n)
        pendingScroll.current = true
        return n
    }

    useEffect(() => {
        if (pendingScroll.current && taRef.current) {
            pendingScroll.current = false
            taRef.current.scrollTop = taRef.current.scrollHeight
            syncScroll()
        }
        if (pendingFocus.current !== null) {
            const el = mxRef.current?.querySelector<HTMLInputElement>(`input[data-line="${pendingFocus.current}"]`)
            pendingFocus.current = null
            el?.focus()
            el?.select()
        }
    })

    const addTemplate = (id: TemplateId) => {
        const tpl = TEMPLATES[id]
        appendLine(fmtRule(tpl.path, tpl.caps))
    }

    const toggleCap = (r: ParsedRule, c: string) => {
        const lines = text.split('\n')
        let caps = r.caps.slice()
        const i = caps.indexOf(c)
        if (i > -1) caps.splice(i, 1)
        else {
            caps.push(c)
            if (c === 'deny') caps = ['deny']
        }
        lines[r.line] = fmtRule(r.path, caps)
        onText(lines.join('\n'))
        doFlash(r.line)
        doMflash(r.line)
    }
    const removeRule = (r: ParsedRule) => {
        const lines = text.split('\n')
        lines.splice(r.line, 1)
        onText(lines.join('\n'))
    }
    const setRulePath = (r: ParsedRule, v: string) => {
        const lines = text.split('\n')
        lines[r.line] = fmtRule(v.replace(/["\n]/g, ''), r.caps)
        onText(lines.join('\n'))
    }

    const hiddenErrLines = useMemo(() => {
        const ruleLines = new Set(parsed.rules.map((r) => r.line))
        const hidden = new Set<number>()
        parsed.diags.forEach((d) => {
            if (d.sev === 'e' && !ruleLines.has(d.line)) hidden.add(d.line)
        })
        return hidden.size
    }, [parsed])

    const status: ReactNode = parsed.errors ? (
        <span className={s.no}>● {t('code.errors', { count: parsed.errors })}</span>
    ) : parsed.warns ? (
        <span className={s.wn}>● {t('code.warnings', { count: parsed.warns })}</span>
    ) : (
        <span className={s.ok}>● {t('code.ok')}</span>
    )

    return (
        <div className={s.edcol}>
            <Plate>
                <div className={s.edtop}>
                    <h2 className={s.edName}>{name}</h2>
                    <span className={s.pills}>
                        {isNew && <Pill tone="brass">{t('pill.new')}</Pill>}
                        {inheritable && <Pill tone="violet">{t('pill.inheritable')}</Pill>}
                    </span>
                    <span className={s.dirty} data-show={dirty}>
                        <i />
                        {t('unsaved')}
                    </span>
                    <div className={s.acts}>
                        <Button disabled={!dirty && !isNew} onClick={onReset} size="xs" variant="default">
                            {isNew ? t('discard') : t('reset')}
                        </Button>
                        {!isNew && (
                            <Button color="red" onClick={onDelete} size="xs" variant="outline">
                                {t('delete')}
                            </Button>
                        )}
                        <Button
                            disabled={!dirty || parsed.errors > 0}
                            loading={saving}
                            onClick={onSave}
                            size="xs"
                            title={parsed.errors > 0 ? t('fixToSave') : undefined}
                        >
                            {t('save')}
                        </Button>
                    </div>
                </div>
                <div className={s.split}>
                    <div className={s.pane}>
                        <div className={s.paneH}>
                            <span>{t('code.title')}</span>
                            <span className={s.sp}>
                                <button className={s.mini} onClick={() => addTemplate('ro')} type="button">
                                    + {t('tpl.ro')}
                                </button>
                                <button className={s.mini} onClick={() => addTemplate('rw')} type="button">
                                    + {t('tpl.rw')}
                                </button>
                                <button className={s.mini} onClick={() => addTemplate('admin')} type="button">
                                    + {t('tpl.admin')}
                                </button>
                            </span>
                        </div>
                        <div className={s.code}>
                            <div aria-hidden className={s.gut} ref={gutRef}>
                                {parsed.lines.map((_, i) => (
                                    <div data-sev={sev[i]} key={i}>
                                        {i + 1}
                                    </div>
                                ))}
                            </div>
                            <div className={s.edwrap}>
                                <div aria-hidden className={s.hl} ref={hlRef}>
                                    {hlLines.map((segs, i) => {
                                        const isHit = i === hit
                                        const cls = cx(
                                            s.ln,
                                            sev[i] === 'e' ? s.lnErr : sev[i] === 'w' ? s.lnWarn : undefined,
                                            i === caret.line && focused && s.lnCur,
                                            isHit && s.lnHit,
                                            isHit && beam > 0 && s.lnBeam,
                                            flash?.line === i && s.lnFlash
                                        )
                                        return (
                                            <div
                                                className={cls}
                                                key={isHit ? `h${i}-${beam}` : `l${i}`}
                                            >
                                                <HlLine segs={segs} />
                                            </div>
                                        )
                                    })}
                                </div>
                                <textarea
                                    aria-label={t('code.aria')}
                                    autoCapitalize="off"
                                    autoComplete="off"
                                    className={s.ta}
                                    onBlur={() => setFocused(false)}
                                    onChange={(e) => {
                                        onText(e.currentTarget.value)
                                        readCaret()
                                    }}
                                    onFocus={() => {
                                        setFocused(true)
                                        readCaret()
                                    }}
                                    onKeyDown={(e) => {
                                        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
                                            e.preventDefault()
                                            if (dirty && parsed.errors === 0 && !saving) onSave()
                                        }
                                    }}
                                    onKeyUp={readCaret}
                                    onScroll={syncScroll}
                                    onSelect={readCaret}
                                    ref={taRef}
                                    spellCheck={false}
                                    value={text}
                                    wrap="off"
                                />
                            </div>
                        </div>
                        <div className={s.codefoot}>
                            <span>{status}</span>
                            <span>{t('code.pos', { col: caret.col, line: caret.line + 1 })}</span>
                            <span style={{ marginLeft: 'auto' }}>
                                {t('rules', { count: parsed.rules.length })} ·{' '}
                                {t('code.lines', { count: parsed.lines.length })}
                            </span>
                        </div>
                        {sortedDiags.length > 0 && (
                            <div className={s.diag}>
                                {sortedDiags.map((d, i) => (
                                    <button
                                        className={s.diagItem}
                                        data-sev={d.sev}
                                        key={`${d.line}-${d.key}-${i}`}
                                        onClick={() => gotoLine(d.line)}
                                        type="button"
                                    >
                                        <span className={s.sev}>{d.sev === 'e' ? t('sev.error') : t('sev.warn')}</span>
                                        <span className={s.diagLn}>{t('code.lineShort', { n: d.n })}</span>
                                        <DiagMessage d={d} />
                                    </button>
                                ))}
                            </div>
                        )}
                    </div>

                    <div className={s.pane}>
                        <div className={s.paneH}>
                            <span>{t('matrix.title')}</span>
                            <span className={cx(s.sp, s.monoNote)}>{t('rules', { count: parsed.rules.length })}</span>
                        </div>
                        <div className={s.mx}>
                            <table>
                                <thead>
                                    <tr>
                                        <th>{t('matrix.path')}</th>
                                        {CAPS.map((c) => (
                                            <th data-deny={c === 'deny'} key={c}>
                                                {c}
                                            </th>
                                        ))}
                                        <th />
                                    </tr>
                                </thead>
                                <tbody ref={mxRef}>
                                    {parsed.rules.length === 0 && (
                                        <tr>
                                            <td className={s.mxEmpty} colSpan={7}>
                                                {t('matrix.empty')}
                                            </td>
                                        </tr>
                                    )}
                                    {parsed.rules.map((r) => (
                                        <tr
                                            data-flash={mflash?.line === r.line || undefined}
                                            data-hit={r.line === hit}
                                            key={r.line}
                                        >
                                            <td>
                                                <input
                                                    aria-label={t('matrix.pathAria', { n: r.n })}
                                                    className={s.pin}
                                                    data-line={r.line}
                                                    onChange={(e) => setRulePath(r, e.currentTarget.value)}
                                                    spellCheck={false}
                                                    value={r.path}
                                                />
                                                <span className={s.scope}>{t(`scope.${pathScope(r.path)}`)}</span>
                                            </td>
                                            {CAPS.map((c) => {
                                                const on = r.caps.includes(c)
                                                return (
                                                    <td key={c}>
                                                        <button
                                                            aria-label={t('matrix.capAria', { cap: c, path: r.path })}
                                                            aria-pressed={on}
                                                            className={s.cell}
                                                            data-cap={c}
                                                            data-on={on}
                                                            onClick={() => toggleCap(r, c)}
                                                            type="button"
                                                        >
                                                            <Icon name={c === 'deny' ? 'x' : 'check'} size={14} />
                                                        </button>
                                                    </td>
                                                )
                                            })}
                                            <td>
                                                <button
                                                    aria-label={t('matrix.removeAria')}
                                                    className={s.delrule}
                                                    onClick={() => removeRule(r)}
                                                    type="button"
                                                >
                                                    <Icon name="trash" size={14} />
                                                </button>
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                        {hiddenErrLines > 0 && (
                            <div className={s.mxnote}>{t('matrix.hidden', { count: hiddenErrLines })}</div>
                        )}
                        <div className={s.mxfoot}>
                            <Button
                                leftSection={<Icon name="plus" size={14} />}
                                onClick={() => {
                                    pendingFocus.current = appendLine(fmtRule('secret/new/*', ['read']))
                                }}
                                size="xs"
                                variant="default"
                            >
                                {t('addRule')}
                            </Button>
                            <p className={s.hint}>
                                <Trans components={{ code: <code /> }} i18nKey="globHint" ns="policies" t={t} />
                            </p>
                        </div>
                        <div className={s.mxfoot} style={{ borderTop: '1px solid var(--line)', paddingTop: 14 }}>
                            <Switch
                                checked={inheritable}
                                label={t('inheritable')}
                                onChange={(e) => onInheritable(e.currentTarget.checked)}
                            />
                        </div>
                    </div>
                </div>
            </Plate>

            <AccessTester
                cap={cap}
                ev={ev}
                hasRules={parsed.rules.length > 0}
                onCap={userCap}
                onJump={gotoLine}
                onPath={userPath}
                onSample={userSample}
                path={tPath ?? path}
                samples={samples}
            />
        </div>
    )
}
