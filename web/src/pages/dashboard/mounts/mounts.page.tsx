// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { CSSProperties, KeyboardEvent, ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { deleteMount, getMountConfig, listMounts, type MountEntry } from '@shared/api/endpoints/mounts'
import {
    ConfirmModal,
    CopyButton,
    CountUp,
    cx,
    Icon,
    StateBlock,
    toast,
    useCopy
} from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Pill } from '@shared/ui/tuck/tuck'

import { CreateWizard } from './components/create-wizard'
import { TuneDrawer } from './components/tune-drawer'
import {
    CAT_COLOR,
    CAT_PILL,
    type Cat,
    catOf,
    type Filter,
    fmtTtl,
    isExperimental,
    isForbidden,
    matchesFilter,
    nsToSec,
    reducedMotion,
    serverMessage
} from './lib'
import styles from './mounts.module.css'

const FILTERS: Filter[] = ['all', 'secrets', 'auth', 'sys']
const POLICY = 'path "sys/mounts" { capabilities = ["read", "write", "delete"] }'

const rich = { b: <b />, em: <em />, code: <code /> }
const rv = (d: number) => ({ '--d': d }) as CSSProperties

const sleep = (ms: number) => new Promise<void>((r) => window.setTimeout(r, ms))

function catClass(c: Cat): string | undefined {
    return c === 'engine' ? undefined : styles[c]
}

function fmtDate(iso: string, lang: string): string {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return iso || '—'
    return d.toLocaleString(lang, { dateStyle: 'medium', timeStyle: 'short' })
}

function Rivets() {
    return (
        <>
            <i className={styles.rivet} />
            <i className={styles.rivet} />
            <i className={styles.rivet} />
            <i className={styles.rivet} />
        </>
    )
}

function Detail({ m, open }: { m: MountEntry; open: boolean }) {
    const { t, i18n } = useTranslation('mounts')
    const { copied, copy } = useCopy({ toast: false })
    const { data, isError, isPending } = useQuery({
        enabled: open,
        queryFn: () => getMountConfig(m.path),
        queryKey: ['mounts', 'config', m.path],
        retry: 1
    })
    const cfg = (v: ReactNode) => (isPending ? '…' : isError ? t('detail.loadFailed') : v)

    return (
        <div className={styles.detail}>
            <div>
                <div className={styles.detailIn}>
                    <div className={styles.kv}>
                        <small>{t('detail.accessor')}</small>
                        <code>{m.accessor}</code>
                    </div>
                    <div className={styles.kv}>
                        <small>{t('detail.mountedAt')}</small>
                        <span>{fmtDate(m.created_at, i18n.language)}</span>
                    </div>
                    <div className={styles.kv}>
                        <small>{t('detail.defaultTtl')}</small>
                        <span>{cfg(fmtTtl(nsToSec(data?.default_lease_ttl)))}</span>
                    </div>
                    <div className={styles.kv}>
                        <small>{t('detail.maxTtl')}</small>
                        <span>{cfg(fmtTtl(nsToSec(data?.max_lease_ttl)))}</span>
                    </div>
                    <div className={styles.kv}>
                        <small>{t('detail.noCache')}</small>
                        <span>{cfg(data?.force_no_cache ? t('detail.yes') : t('detail.no'))}</span>
                    </div>
                    <div className={styles.kv}>
                        <small>{t('detail.api')}</small>
                        <span>GET /v1/sys/mounts-tune/{m.path}</span>
                    </div>
                    <div className={styles.kv}>
                        <small>&nbsp;</small>
                        <div className={styles.kvBtns}>
                            <Button
                                leftSection={<Icon name={copied ? 'check' : 'copy'} size={13} />}
                                onClick={() => void copy(m.path)}
                                size="xs"
                                tabIndex={open ? 0 : -1}
                                variant="default"
                            >
                                {t('detail.copyPath')}
                            </Button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

export function MountsPage() {
    const { t } = useTranslation('mounts')
    const queryClient = useQueryClient()

    const [filter, setFilter] = useState<Filter>('all')
    const [q, setQ] = useState('')
    const [open, setOpen] = useState<null | string>(null)
    const [creating, setCreating] = useState(false)
    const [tuning, setTuning] = useState<MountEntry | null>(null)
    const [toDelete, setToDelete] = useState<MountEntry | null>(null)
    const [dying, setDying] = useState<null | string>(null)
    const [fresh, setFresh] = useState<null | string>(null)
    const [zap, setZap] = useState(false)
    const [sparkY, setSparkY] = useState<null | number>(null)
    const [still, setStill] = useState(false)
    const rackRef = useRef<HTMLDivElement>(null)
    const policyCopy = useCopy({ toast: true })

    const { data, error, isError, isFetching, isLoading, refetch } = useQuery({
        queryFn: listMounts,
        queryKey: ['mounts', 'list'],
        retry: (count, err) => !isForbidden(err) && count < 2
    })

    const forbidden = isError && isForbidden(error)
    const all = useMemo(() => [...(data ?? [])].sort((a, b) => a.path.localeCompare(b.path)), [data])
    const needle = q.trim().toLowerCase()
    const list = useMemo(
        () =>
            all.filter(
                (m) =>
                    matchesFilter(m, filter) &&
                    (!needle || `${m.path} ${m.type} ${m.description ?? ''}`.toLowerCase().includes(needle))
            ),
        [all, filter, needle]
    )

    const showData = !isLoading && !isError
    const isEmpty = showData && all.length === 0

    const stats = useMemo(() => {
        const by = new Map<string, { cat: Cat; n: number }>()
        for (const m of all) {
            const e = by.get(m.type)
            if (e) e.n += 1
            else by.set(m.type, { cat: catOf(m), n: 1 })
        }
        const builtin = all.filter((m) => m.builtin).length
        return { builtin, custom: all.length - builtin, types: [...by.entries()] }
    }, [all])

    // the cable draw-in plays once on first paint; later renders stay still
    useEffect(() => {
        if (!showData || all.length === 0 || still) return
        const id = window.setTimeout(() => setStill(true), 2600)
        return () => window.clearTimeout(id)
    }, [showData, all.length, still])

    // plug-in animation for a freshly mounted engine (the artifact's signature)
    useEffect(() => {
        if (!fresh) return
        const el = rackRef.current?.querySelector<HTMLElement>(`[data-path="${CSS.escape(fresh)}"]`)
        const reduce = reducedMotion()
        if (el) el.scrollIntoView({ behavior: reduce ? 'auto' : 'smooth', block: 'center' })
        const timers: number[] = []
        if (el && !reduce) {
            timers.push(
                window.setTimeout(() => {
                    setZap(true)
                    setSparkY(el.offsetTop + 35)
                }, 1000),
                window.setTimeout(() => setZap(false), 1500),
                window.setTimeout(() => setSparkY(null), 1800)
            )
        }
        timers.push(window.setTimeout(() => setFresh(null), 3800))
        return () => timers.forEach((id) => window.clearTimeout(id))
    }, [fresh])

    const deleteMutation = useMutation({ mutationFn: (path: string) => deleteMount(path) })

    const confirmDelete = async (m: MountEntry) => {
        try {
            await deleteMutation.mutateAsync(m.path)
        } catch (err) {
            toast.error(t('toast.unmountFailed'), serverMessage(err))
            throw err
        }
        toast.warning(t('toast.unmounted'), t('toast.unmountedMsg', { path: m.path }))
        if (open === m.path) setOpen(null)
        // let the unit play its unplug animation after the dialog closes, then drop it from the list
        void (async () => {
            setDying(m.path)
            await sleep(reducedMotion() ? 0 : 480)
            await queryClient.invalidateQueries({ queryKey: ['mounts', 'list'] })
            queryClient.removeQueries({ queryKey: ['mounts', 'config', m.path] })
            setDying(null)
        })()
    }

    const onCreated = (path: string) => {
        setFilter('all')
        setQ('')
        setOpen(null)
        setFresh(path)
    }

    const toggle = (path: string) => setOpen((cur) => (cur === path ? null : path))
    const onKey = (e: KeyboardEvent<HTMLDivElement>, path: string) => {
        if (e.target !== e.currentTarget) return
        if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            toggle(path)
        }
    }

    const unit = (m: MountEntry, i: number) => {
        const cat = catOf(m)
        const isOpen = open === m.path
        const p = m.path.replace(/\/+$/, '')
        return (
            <article
                className={cx(
                    styles.unit,
                    catClass(cat),
                    m.builtin && styles.builtin,
                    isOpen && styles.open,
                    dying === m.path && styles.leaving,
                    fresh === m.path && styles.fresh
                )}
                data-path={m.path}
                key={m.path}
                style={{ '--i': Math.min(i, 12) } as CSSProperties}
            >
                <span className={styles.cable} />
                <div
                    aria-expanded={isOpen}
                    className={styles.unitMain}
                    onClick={() => toggle(m.path)}
                    onKeyDown={(e) => onKey(e, m.path)}
                    role="button"
                    tabIndex={0}
                >
                    <div className={styles.pth}>
                        <div className={styles.row1}>
                            <code>
                                {p}
                                <u>/</u>
                            </code>
                            <Pill tone={CAT_PILL[cat]}>{m.type}</Pill>
                            {isExperimental(m.type) && (
                                <Pill size="sm" title={t('unit.experimentalHint')} tone="r">
                                    {t('unit.experimental')}
                                </Pill>
                            )}
                            {m.builtin && (
                                <Pill tone="v">
                                    <Icon name="lock" size={11} />
                                    {t('unit.builtin')}
                                </Pill>
                            )}
                        </div>
                        <div className={styles.desc}>{m.description || t('unit.noDesc')}</div>
                    </div>
                    <div className={styles.mnt}>
                        <small>{t('unit.created')}</small>
                        <MountedAt iso={m.created_at} />
                    </div>
                    <div className={styles.acc}>
                        <div>
                            <small>{t('unit.accessor')}</small>
                            <span className={styles.v}>{m.accessor}</span>
                        </div>
                        <span onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()}>
                            <CopyButton label={t('unit.copyAccessor')} size={36} value={m.accessor} />
                        </span>
                    </div>
                    <div className={styles.acts} onClick={(e) => e.stopPropagation()}>
                        <button
                            aria-label={t('unit.tune', { path: m.path })}
                            className={styles.ib}
                            onClick={() => setTuning(m)}
                            title={t('unit.tune', { path: m.path })}
                            type="button"
                        >
                            <Icon name="gear" size={16} />
                        </button>
                        {m.builtin ? (
                            <span
                                aria-label={t('unit.builtinLocked')}
                                className={cx(styles.ib, styles.lockd)}
                                role="img"
                                title={t('unit.builtinLocked')}
                            >
                                <Icon name="lock" size={16} />
                            </span>
                        ) : (
                            <button
                                aria-label={t('unit.unmount', { path: m.path })}
                                className={cx(styles.ib, styles.del)}
                                onClick={() => setToDelete(m)}
                                title={t('unit.unmount', { path: m.path })}
                                type="button"
                            >
                                <Icon name="trash" size={16} />
                            </button>
                        )}
                    </div>
                </div>
                <Detail m={m} open={isOpen} />
            </article>
        )
    }

    let body: ReactNode
    if (isLoading) {
        body = (
            <>
                <div className={styles.loadLine}>
                    <Eyebrow>
                        <span className={styles.spin} /> {t('loading')}
                    </Eyebrow>
                </div>
                <div aria-busy className={styles.rack}>
                    <div className={styles.bus} />
                    {[0, 1, 2, 3, 4].map((i) => (
                        <div className={styles.sk} key={i} style={{ animationDelay: `${i * 60}ms` }}>
                            <i />
                            <i />
                        </div>
                    ))}
                </div>
            </>
        )
    } else if (forbidden) {
        body = (
            <div className={cx(styles.panel, styles.bad)}>
                <Rivets />
                <div className={styles.code403}>403</div>
                <div>
                    <Eyebrow className={styles.alarmEyebrow}>{t('forbidden.eyebrow')}</Eyebrow>
                    <h3>
                        <Trans components={rich} i18nKey="forbidden.title" ns="mounts" />
                    </h3>
                    <p>{t('forbidden.text')}</p>
                    <div className={styles.term}>
                        <div className={styles.termTop}>
                            <span>{t('forbidden.policyTitle')}</span>
                            <em>{t('forbidden.policyState')}</em>
                        </div>
                        <pre>
                            <span className="k">path</span> <span className="s">"sys/mounts"</span> {'{ '}
                            <span className="k">capabilities</span> = [<span className="s">"read"</span>,{' '}
                            <span className="s">"write"</span>, <span className="s">"delete"</span>]{' }'}
                        </pre>
                    </div>
                    <div className={styles.ctaRow}>
                        <Button loading={isFetching} onClick={() => void refetch()}>
                            {t('forbidden.retry')}
                        </Button>
                        <Button
                            leftSection={<Icon name={policyCopy.copied ? 'check' : 'copy'} size={14} />}
                            onClick={() => void policyCopy.copy(POLICY)}
                            variant="default"
                        >
                            {t('forbidden.copyPolicy')}
                        </Button>
                    </div>
                </div>
            </div>
        )
    } else if (isError) {
        body = (
            <StateBlock
                details={serverMessage(error) ?? (isAxiosError(error) ? error.message : undefined)}
                kind="error"
                message={t('error.message')}
                onRetry={() => void refetch()}
                retrying={isFetching}
                title={t('error.title')}
            />
        )
    } else if (isEmpty) {
        body = (
            <div className={styles.panel}>
                <Rivets />
                <div className={styles.bigArt}>
                    <svg fill="none" viewBox="0 0 220 180">
                        <line stroke="#3B4154" strokeWidth="2" x1="30" x2="30" y1="0" y2="180" />
                        <circle cx="30" cy="90" fill="#0A0B0E" r="6" stroke="#3B4154" strokeWidth="2" />
                        <rect height="64" stroke="#3B4154" strokeDasharray="5 5" width="108" x="100" y="58" />
                        <path d="M36 90h56" stroke="#3B4154" strokeDasharray="4 5" />
                        <g className={styles.float}>
                            <rect fill="#17150c" height="20" stroke="#D7A94B" width="16" x="86" y="80" />
                            <path d="M60 90h26" stroke="#D7A94B" strokeWidth="2" />
                        </g>
                    </svg>
                </div>
                <div>
                    <Eyebrow>{t('empty.eyebrow')}</Eyebrow>
                    <h3>
                        <Trans components={rich} i18nKey="empty.title" ns="mounts" />
                    </h3>
                    <p>{t('empty.text')}</p>
                    <div className={styles.ctaRow}>
                        <Button leftSection={<Icon name="plus" size={14} />} onClick={() => setCreating(true)}>
                            {t('empty.cta')}
                        </Button>
                    </div>
                </div>
            </div>
        )
    } else {
        body = (
            <>
                {list.length === 0 ? (
                    <div className={styles.panel} style={{ gridTemplateColumns: '1fr' }}>
                        <Rivets />
                        <div>
                            <h3>
                                <Trans components={rich} i18nKey="noMatch.title" ns="mounts" />
                            </h3>
                            <p>{t('noMatch.text')}</p>
                            <Button
                                onClick={() => {
                                    setQ('')
                                    setFilter('all')
                                }}
                                variant="default"
                            >
                                {t('noMatch.reset')}
                            </Button>
                        </div>
                    </div>
                ) : (
                    <div className={cx(styles.rack, still && styles.still)} ref={rackRef}>
                        <div className={cx(styles.bus, zap && styles.zap)} />
                        {list.map(unit)}
                        {sparkY !== null && (
                            <div aria-hidden className={styles.spark} style={{ top: sparkY - 6 }}>
                                {Array.from({ length: 8 }, (_, i) => (
                                    <i key={i} style={{ '--r': `${i * 45}deg` } as CSSProperties} />
                                ))}
                            </div>
                        )}
                    </div>
                )}
            </>
        )
    }

    return (
        <Page title={t('title')}>
            <div className={styles.phead}>
                <div>
                    <Eyebrow className={styles.rv}>{t('eyebrow')}</Eyebrow>
                    <h1 className={cx(styles.h1, styles.rv)} style={rv(1)}>
                        {t('h1a')}
                        <span className={styles.gold}>{t('h1b')}</span>
                    </h1>
                    <p className={cx(styles.lead, styles.rv)} style={rv(2)}>
                        <Trans components={rich} i18nKey="lead" ns="mounts" />
                    </p>
                </div>
                {!forbidden && (
                    <div className={styles.rv} style={rv(3)}>
                        <Button leftSection={<Icon name="plus" size={14} />} onClick={() => setCreating(true)}>
                            {t('create')}
                        </Button>
                    </div>
                )}
            </div>

            {!forbidden && (
                <div className={cx(styles.stats, styles.rv)} style={rv(4)}>
                    <div className={styles.stat}>
                        <small>{t('stats.total')}</small>
                        <div className={styles.big}>
                            {showData ? <CountUp value={all.length} /> : '—'}
                            {showData && <em>{t('stats.totalUnit')}</em>}
                        </div>
                        {showData && (
                            <p>{t('stats.totalHint', { builtin: stats.builtin, custom: stats.custom })}</p>
                        )}
                    </div>
                    <div className={styles.stat}>
                        <small>{t('stats.types')}</small>
                        <div className={styles.tbar}>
                            {showData && stats.types.length > 0 ? (
                                stats.types.map(([type, s]) => (
                                    <i
                                        key={type}
                                        style={{ '--c': CAT_COLOR[s.cat], '--n': s.n } as CSSProperties}
                                        title={`${type} · ${s.n}`}
                                    />
                                ))
                            ) : (
                                <i style={{ '--c': 'var(--line2)', '--n': 1 } as CSSProperties} />
                            )}
                        </div>
                        <div className={styles.legend}>
                            {showData &&
                                stats.types.map(([type, s]) => (
                                    <span key={type} style={{ '--c': CAT_COLOR[s.cat] } as CSSProperties}>
                                        {type} · {s.n}
                                    </span>
                                ))}
                        </div>
                    </div>
                    <div className={styles.stat}>
                        <small>{t('stats.custom')}</small>
                        <div className={styles.big}>
                            {showData ? <CountUp value={stats.custom} /> : '—'}
                            {showData && <em>{t('stats.customUnit')}</em>}
                        </div>
                        {showData && <p>{t('stats.customHint')}</p>}
                    </div>
                </div>
            )}

            {showData && !isEmpty && (
                <div className={cx(styles.toolbar, styles.rv)} style={rv(5)}>
                    <div className={styles.seg} role="group">
                        {FILTERS.map((f) => (
                            <button
                                aria-pressed={filter === f}
                                className={filter === f ? styles.on : undefined}
                                key={f}
                                onClick={() => setFilter(f)}
                                type="button"
                            >
                                {t(`filter.${f}`)}
                            </button>
                        ))}
                    </div>
                    <label className={styles.search}>
                        <Icon name="search" size={15} />
                        <input
                            aria-label={t('search.label')}
                            autoComplete="off"
                            onChange={(e) => setQ(e.currentTarget.value)}
                            placeholder={t('search.placeholder')}
                            type="search"
                            value={q}
                        />
                    </label>
                </div>
            )}

            <div className={styles.rv} style={rv(6)}>
                {body}
            </div>

            <CreateWizard
                mounts={all}
                onClose={() => setCreating(false)}
                onCreated={onCreated}
                opened={creating}
            />
            <TuneDrawer mount={tuning} onClose={() => setTuning(null)} />

            <ConfirmModal
                confirmLabel={t('unmount.confirm')}
                confirmText={toDelete?.path}
                message={
                    toDelete && (
                        <>
                            {t('unmount.message', { path: toDelete.path })}
                            <div className={styles.warn}>
                                <b>{t('unmount.warnTitle')}</b> {t('unmount.warn', { path: toDelete.path })}
                            </div>
                        </>
                    )
                }
                onClose={() => setToDelete(null)}
                onConfirm={() => confirmDelete(toDelete as MountEntry)}
                opened={toDelete !== null}
                title={t('unmount.title')}
            />
        </Page>
    )
}

function MountedAt({ iso }: { iso: string }) {
    const { i18n } = useTranslation('mounts')
    return <span>{fmtDate(iso, i18n.language)}</span>
}
