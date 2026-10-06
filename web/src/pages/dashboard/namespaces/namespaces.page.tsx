// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { CSSProperties, FormEvent, ReactNode, useEffect, useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { lookupSelf } from '@shared/api/endpoints/auth'
import {
    createNamespace,
    deleteNamespace,
    listNamespaceDetails,
    NamespaceStats,
    NamespaceView,
    updateNamespace
} from '@shared/api/endpoints/namespace'
import { getNamespace, setNamespace } from '@shared/auth/token'
import {
    ConfirmModal,
    CopyButton,
    CountUp,
    Icon,
    KeyValueEditor,
    KvPair,
    Modal,
    SkeletonLine,
    StateBlock,
    toast,
    validateKv
} from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Pill } from '@shared/ui/tuck/tuck'

import styles from './namespaces.module.css'

// Mirrors internal/namespace: ^[a-z0-9][a-z0-9-]*$ and not "root".
const NAME_RE = /^[a-z0-9][a-z0-9-]*$/
const VIEW_KEY = 'tuck.ns.view'
// Mirrors namespace.Max* on the server.
const MAX_DESC = 512
const MAX_OWNER = 128
const MAX_LABELS = 32
const LABEL_KEY_RE = /^[A-Za-z0-9][A-Za-z0-9_./-]{0,63}$/

type View = 'cards' | 'list'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

function readView(): View {
    try {
        return localStorage.getItem(VIEW_KEY) === 'list' ? 'list' : 'cards'
    } catch {
        return 'cards'
    }
}

function saveView(v: View) {
    try {
        localStorage.setItem(VIEW_KEY, v)
    } catch {
        // storage unavailable: the choice just won't persist
    }
}

function serverMessage(err: unknown): string | undefined {
    if (isAxiosError(err)) {
        const d = err.response?.data as { error?: string } | undefined
        return d?.error
    }
    return undefined
}

const STAT_KEYS = ['secrets', 'tokens', 'policies', 'auth_roles'] as const
type StatKey = (typeof STAT_KEYS)[number]

/** Sum of one counter over namespaces; undefined when any of them hides it. */
function sumStat(items: NamespaceView[], key: StatKey): number | undefined {
    let total = 0
    for (const n of items) {
        const v = n.stats?.[key]
        if (v === undefined) return undefined
        total += v
    }
    return items.length ? total : undefined
}

function pairsOf(labels?: Record<string, string>): KvPair[] {
    return Object.entries(labels ?? {})
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, value]) => ({ key, value }))
}

function labelsOf(pairs: KvPair[]): Record<string, string> {
    const out: Record<string, string> = {}
    for (const p of pairs) {
        const k = p.key.trim()
        if (k) out[k] = p.value
    }
    return out
}

/** What lives in a namespace about to be deleted (only the counters the caller may see). */
function DeleteImpact({ stats }: { stats?: NamespaceStats }) {
    const { t } = useTranslation('namespaces')
    if (!stats) return null
    const shown = STAT_KEYS.filter((k) => stats[k] !== undefined)
    if (shown.length === 0) return null
    return (
        <ul className={styles.delList}>
            {shown.map((k) => (
                <li key={k}>
                    <span>{t(`counts.${k}`)}</span>
                    <b>{stats[k]}</b>
                </li>
            ))}
        </ul>
    )
}

/** Counters of one namespace; a counter the caller may not see is not rendered at all. */
function Counts({ stats }: { stats?: NamespaceStats }) {
    const { t } = useTranslation('namespaces')
    if (!stats) return null
    const shown = STAT_KEYS.filter((k) => stats[k] !== undefined)
    if (shown.length === 0) return null
    return (
        <div className={styles.counts}>
            {shown.map((k) => (
                <div key={k}>
                    <small>{t(`counts.${k}`)}</small>
                    <b>
                        <CountUp value={stats[k] as number} />
                    </b>
                </div>
            ))}
        </div>
    )
}

function NsName({ name }: { name: string }) {
    const i = name.lastIndexOf('/')
    return i < 0 ? (
        <b>{name}</b>
    ) : (
        <>
            <span>{name.slice(0, i + 1)}</span>
            <b>{name.slice(i + 1)}</b>
        </>
    )
}

/** Decorative sketch: the highlighted vault sits between two walls. */
function MiniIso() {
    return (
        <svg aria-hidden fill="none" viewBox="0 0 132 44">
            <rect height="32" stroke="#3B4154" width="34" x="2" y="6" />
            <rect fill="rgba(215,169,75,.14)" height="32" stroke="#D7A94B" width="34" x="49" y="6" />
            <circle cx="66" cy="22" fill="#D7A94B" r="4" />
            <rect height="32" stroke="#3B4154" width="34" x="96" y="6" />
            <path d="M42 10v24M89 10v24" stroke="#FF5A4D" strokeWidth="1.5" />
            <path d="M37 22h3M44 22h3M84 22h3M91 22h3" stroke="#575C70" />
        </svg>
    )
}

function SwitchIcon() {
    return (
        <svg fill="none" height="15" stroke="currentColor" strokeWidth="1.6" viewBox="0 0 16 16" width="15">
            <path d="M2 5h10l-3-3M14 11H4l3 3" />
        </svg>
    )
}

function OnbLogo() {
    return (
        <svg className={styles.logo} fill="none" height="56" viewBox="0 0 44 44" width="56">
            <circle cx="22" cy="22" r="19" stroke="#3B4154" strokeWidth="2" />
            <circle cx="22" cy="22" r="12" stroke="#D7A94B" strokeDasharray="4 5" strokeWidth="2">
                <animateTransform
                    attributeName="transform"
                    dur="14s"
                    from="0 22 22"
                    repeatCount="indefinite"
                    to="360 22 22"
                    type="rotate"
                />
            </circle>
            <circle cx="22" cy="22" fill="#49E0A4" r="3" />
        </svg>
    )
}

const rich = {
    b: <b />,
    br: <br />,
    code: <code />,
    em: <em />
}

export function NamespacesPage() {
    const { t } = useTranslation('namespaces')
    const queryClient = useQueryClient()

    const [q, setQ] = useState('')
    const [view, setView] = useState<View>(readView)
    const [creating, setCreating] = useState(false)
    const [name, setName] = useState('')
    const [description, setDescription] = useState('')
    const [owner, setOwner] = useState('')
    const [labels, setLabels] = useState<KvPair[]>([])
    const [editing, setEditing] = useState<null | string>(null)
    const [serverErr, setServerErr] = useState('')
    const [toDelete, setToDelete] = useState<null | string>(null)
    const [dying, setDying] = useState<null | string>(null)
    const [fresh, setFresh] = useState<null | string>(null)

    const current = getNamespace() ?? ''

    const {
        data: details,
        error,
        isError,
        isFetching,
        isLoading,
        refetch
    } = useQuery({
        // a separate key from the shell switcher's name-only ['namespaces','list']
        queryKey: ['namespaces', 'details'],
        queryFn: () => listNamespaceDetails(true),
        retry: (count, err) => !(isAxiosError(err) && err.response?.status === 403) && count < 2
    })
    const { data: self } = useQuery({
        queryKey: ['auth', 'lookup-self'],
        queryFn: lookupSelf,
        staleTime: 60_000
    })
    const bound = (self as { namespace?: string } | undefined)?.namespace ?? ''

    const forbidden = isError && isAxiosError(error) && error.response?.status === 403
    const items = useMemo(() => [...(details ?? [])].sort((a, b) => a.name.localeCompare(b.name)), [details])
    const all = useMemo(() => items.map((n) => n.name), [items])
    const filter = q.trim().toLowerCase()
    const listItems = useMemo(
        () =>
            items.filter(
                (n) =>
                    !filter ||
                    n.name.includes(filter) ||
                    (n.owner ?? '').toLowerCase().includes(filter) ||
                    (n.description ?? '').toLowerCase().includes(filter) ||
                    Object.entries(n.labels ?? {}).some(([k, v]) => `${k}=${v}`.toLowerCase().includes(filter))
            ),
        [items, filter]
    )
    const list = useMemo(() => listItems.map((n) => n.name), [listItems])
    const byName = useMemo(() => new Map(items.map((n) => [n.name, n])), [items])
    const totals = useMemo(
        () => ({
            secrets: sumStat(items, 'secrets'),
            tokens: sumStat(items, 'tokens')
        }),
        [items]
    )

    // --- create ---
    const trimmed = name.trim()
    let nameErr = ''
    if (trimmed) {
        if (trimmed === 'root') nameErr = t('createModal.errRoot')
        else if (!NAME_RE.test(trimmed)) nameErr = t('createModal.errChars')
        else if (all.includes(trimmed)) nameErr = t('createModal.errExists', { name: trimmed })
    }
    const shownErr = nameErr || serverErr

    const kvCheck = validateKv(labels, LABEL_KEY_RE)
    const metaErr = description.length > MAX_DESC || owner.length > MAX_OWNER || !kvCheck.valid
    const meta = () => ({
        description: description.trim(),
        owner: owner.trim(),
        labels: labelsOf(labels)
    })

    const createMutation = useMutation({
        mutationFn: () => createNamespace(trimmed, meta()),
        onSuccess: async () => {
            const created = trimmed
            setCreating(false)
            setName('')
            setQ('')
            setFresh(created)
            toast.success(t('toast.created'), t('toast.createdMsg', { name: created }))
            await queryClient.invalidateQueries({ queryKey: ['namespaces'] })
        },
        onError: (err) => {
            const msg = serverMessage(err)
            setServerErr(msg ?? '')
            toast.error(t('toast.createFailed'), msg)
        }
    })

    const resetMeta = (n?: NamespaceView) => {
        setDescription(n?.description ?? '')
        setOwner(n?.owner ?? '')
        setLabels(pairsOf(n?.labels))
    }

    const openCreate = () => {
        setName('')
        resetMeta()
        setServerErr('')
        setCreating(true)
    }

    const submitCreate = (e?: FormEvent) => {
        e?.preventDefault()
        if (!trimmed || nameErr || metaErr || createMutation.isPending) return
        setServerErr('')
        createMutation.mutate()
    }

    // --- edit metadata ---
    const openEdit = (ns: string) => {
        resetMeta(byName.get(ns))
        setServerErr('')
        setEditing(ns)
    }

    const editMutation = useMutation({
        mutationFn: (ns: string) => updateNamespace(ns, meta()),
        onSuccess: async (_d, ns) => {
            setEditing(null)
            toast.success(t('toast.updated'), t('toast.updatedMsg', { name: ns }))
            await queryClient.invalidateQueries({ queryKey: ['namespaces'] })
        },
        onError: (err) => {
            const msg = serverMessage(err)
            setServerErr(msg ?? '')
            toast.error(t('toast.updateFailed'), msg)
        }
    })

    const submitEdit = (e?: FormEvent) => {
        e?.preventDefault()
        if (!editing || metaErr || editMutation.isPending) return
        setServerErr('')
        editMutation.mutate(editing)
    }

    useEffect(() => {
        if (!fresh) return
        const el = document.querySelector(`[data-ns="${CSS.escape(fresh)}"]`)
        if (el) {
            const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
            el.scrollIntoView({
                block: 'center',
                behavior: reduce ? 'auto' : 'smooth'
            })
            const id = window.setTimeout(() => setFresh(null), 2200)
            return () => window.clearTimeout(id)
        }
    }, [fresh, list])

    // --- delete ---
    const deleteMutation = useMutation({ mutationFn: deleteNamespace })

    const confirmDelete = async (ns: string) => {
        setDying(ns)
        try {
            await deleteMutation.mutateAsync(ns)
        } catch (err) {
            setDying(null)
            toast.error(t('toast.deleteFailed'), serverMessage(err))
            throw err
        }
        toast.success(t('toast.deleted'), t('toast.deletedMsg', { name: ns }))
        if (current === ns) {
            setNamespace('')
            window.location.reload()
            return
        }
        await queryClient.invalidateQueries({ queryKey: ['namespaces'] })
        setDying(null)
    }

    // --- current namespace (same mechanism as the shell switcher) ---
    const switchTo = (ns: string) => {
        if (ns === current) return
        setNamespace(ns)
        // many query keys aren't namespace-scoped: reload for a clean slate
        window.location.reload()
    }

    const changeView = (v: View) => {
        setView(v)
        saveView(v)
    }

    const showData = !isLoading && !isError
    const isEmpty = showData && all.length === 0

    const actions = (ns: string) => (
        <>
            <button
                aria-label={t('card.use')}
                className={styles.ib}
                disabled={ns === current}
                onClick={() => switchTo(ns)}
                title={t('card.use')}
                type="button"
            >
                <SwitchIcon />
            </button>
            <button
                aria-label={t('card.edit', { name: ns })}
                className={styles.ib}
                onClick={() => openEdit(ns)}
                title={t('card.edit', { name: ns })}
                type="button"
            >
                <Icon name="gear" size={15} />
            </button>
            <CopyButton label={t('card.copy')} size={32} value={ns} />
            <button
                aria-label={t('card.remove', { name: ns })}
                className={cx(styles.ib, styles.del)}
                onClick={() => setToDelete(ns)}
                title={t('card.remove', { name: ns })}
                type="button"
            >
                <Icon name="trash" size={15} />
            </button>
        </>
    )

    let body: ReactNode
    if (isLoading) {
        body = (
            <>
                <div className={styles.secTitle}>…</div>
                <div className={styles.grid}>
                    {[0, 1, 2].map((i) => (
                        <div aria-busy className={styles.skel} key={i} style={{ '--i': i } as CSSProperties}>
                            <SkeletonLine height={16} width="55%" />
                            <SkeletonLine width="35%" />
                            <SkeletonLine height={46} />
                            <SkeletonLine width="80%" />
                        </div>
                    ))}
                </div>
            </>
        )
    } else if (forbidden) {
        body = (
            <div className={cx(styles.panel, styles.bad)}>
                <div className={styles.err403}>
                    <div className={styles.big403}>403</div>
                    <div>
                        <Eyebrow className={styles.alarmEyebrow}>{t('forbidden.eyebrow')}</Eyebrow>
                        <h3>
                            <Trans components={rich} i18nKey="forbidden.title" ns="namespaces" />
                        </h3>
                        <p>
                            {t('forbidden.text')}{' '}
                            {bound && (
                                <Trans
                                    components={{ code: <code className={styles.mono} /> }}
                                    i18nKey="forbidden.bound"
                                    ns="namespaces"
                                    values={{ ns: bound }}
                                />
                            )}
                        </p>
                        <div className={styles.callout}>
                            <b>GET</b> /v1/sys/namespaces/
                            <br />
                            <Trans components={rich} i18nKey="forbidden.callout" ns="namespaces" />
                        </div>
                        <Button loading={isFetching} onClick={() => void refetch()} variant="default">
                            {t('forbidden.retry')}
                        </Button>
                    </div>
                </div>
            </div>
        )
    } else if (isError) {
        body = (
            <StateBlock
                details={serverMessage(error) ?? (error as Error)?.message}
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
                <OnbLogo />
                <Eyebrow>{t('empty.eyebrow')}</Eyebrow>
                <h3>
                    <Trans components={rich} i18nKey="empty.title" ns="namespaces" />
                </h3>
                <p>{t('empty.text')}</p>
                <div className={styles.onb}>
                    <div>
                        <b>{t('empty.s1t')}</b>
                        <Trans components={rich} i18nKey="empty.s1d" ns="namespaces" />
                    </div>
                    <div>
                        <b>{t('empty.s2t')}</b>
                        <Trans components={rich} i18nKey="empty.s2d" ns="namespaces" />
                    </div>
                    <div>
                        <b>{t('empty.s3t')}</b>
                        {t('empty.s3d')}
                    </div>
                </div>
                <Button onClick={openCreate}>{t('empty.cta')}</Button>
            </div>
        )
    } else {
        body = (
            <>
                <div className={styles.secTitle}>
                    {filter ? t('section.found', { count: list.length }) : t('section.all')}
                </div>
                {list.length === 0 ? (
                    <div className={styles.panel}>
                        <h3>
                            <Trans components={rich} i18nKey="noMatch.title" ns="namespaces" />
                        </h3>
                        <p>{t('noMatch.text', { q })}</p>
                        <Button onClick={() => setQ('')} variant="default">
                            {t('noMatch.reset')}
                        </Button>
                    </div>
                ) : view === 'cards' ? (
                    <div className={styles.grid}>
                        {listItems.map((item, i) => {
                            const ns = item.name
                            return (
                                <article
                                    className={cx(
                                        styles.ns,
                                        ns === current && styles.sel,
                                        dying === ns && styles.dying,
                                        fresh === ns && styles.fresh
                                    )}
                                    data-ns={ns}
                                    key={ns}
                                    style={{ '--i': Math.min(i, 12) } as CSSProperties}
                                >
                                    <div className={styles.top}>
                                        <div className={styles.topText}>
                                            <div className={styles.path}>
                                                <NsName name={ns} />
                                            </div>
                                            <div className={styles.meta}>
                                                {item.owner
                                                    ? `${t('card.owner')}: ${item.owner}`
                                                    : `X-Tuck-Namespace: ${ns}`}
                                            </div>
                                        </div>
                                        <div className={styles.acts}>{actions(ns)}</div>
                                    </div>
                                    {ns === current && (
                                        <div>
                                            <Pill dot tone="ok">
                                                {t('card.current')}
                                            </Pill>
                                        </div>
                                    )}
                                    {item.description && <p className={styles.desc}>{item.description}</p>}
                                    <Counts stats={item.stats} />
                                    {item.stats?.truncated && <p className={styles.partial}>{t('card.partial')}</p>}
                                    {Object.keys(item.labels ?? {}).length > 0 && (
                                        <div className={styles.labels}>
                                            {pairsOf(item.labels)
                                                .slice(0, 6)
                                                .map((l) => (
                                                    <span key={l.key} title={`${l.key}=${l.value}`}>
                                                        {l.key}
                                                        {l.value && <em>={l.value}</em>}
                                                    </span>
                                                ))}
                                            {Object.keys(item.labels ?? {}).length > 6 && (
                                                <span>+{Object.keys(item.labels ?? {}).length - 6}</span>
                                            )}
                                        </div>
                                    )}
                                    <div className={styles.iso}>
                                        <MiniIso />
                                        <p>
                                            <Trans components={rich} i18nKey="card.isolated" ns="namespaces" />
                                        </p>
                                    </div>
                                </article>
                            )
                        })}
                    </div>
                ) : (
                    <div className={styles.list}>
                        <div className={cx(styles.row, styles.rowWide, styles.head)}>
                            <span>{t('list.name')}</span>
                            <span className={styles.cOwner}>{t('list.owner')}</span>
                            {STAT_KEYS.map((k) => (
                                <span className={cx(styles.cNum, styles.cStat)} key={k}>
                                    {t(`counts.${k}`)}
                                </span>
                            ))}
                            <span />
                            <span />
                        </div>
                        {listItems.map((item) => {
                            const ns = item.name
                            return (
                                <div
                                    className={cx(
                                        styles.row,
                                        styles.rowWide,
                                        ns === current && styles.sel,
                                        dying === ns && styles.dying,
                                        fresh === ns && styles.fresh
                                    )}
                                    data-ns={ns}
                                    key={ns}
                                >
                                    <span className={styles.nm}>
                                        <i className={styles.dot} />
                                        <span className={styles.path}>
                                            <NsName name={ns} />
                                        </span>
                                    </span>
                                    <span className={styles.cOwner} title={item.description || undefined}>
                                        {item.owner || '—'}
                                    </span>
                                    {STAT_KEYS.map((k) => (
                                        <span className={cx(styles.cNum, styles.cStat)} key={k}>
                                            {item.stats?.[k] ?? '—'}
                                        </span>
                                    ))}
                                    <span>
                                        {ns === current && (
                                            <Pill dot tone="ok">
                                                {t('card.current')}
                                            </Pill>
                                        )}
                                    </span>
                                    <span className={styles.rowActs}>{actions(ns)}</span>
                                </div>
                            )
                        })}
                    </div>
                )}
            </>
        )
    }

    const metaFields = (
        <>
            <div className={styles.fld}>
                <label htmlFor="ns-owner">{t('meta.owner')}</label>
                <input
                    autoComplete="off"
                    className={cx(styles.inp, owner.length > MAX_OWNER && styles.bad)}
                    id="ns-owner"
                    onChange={(e) => setOwner(e.currentTarget.value)}
                    placeholder={t('meta.ownerPlaceholder')}
                    value={owner}
                />
            </div>
            <div className={styles.fld}>
                <label htmlFor="ns-desc">{t('meta.description')}</label>
                <textarea
                    className={cx(styles.inp, styles.ta, description.length > MAX_DESC && styles.bad)}
                    id="ns-desc"
                    onChange={(e) => setDescription(e.currentTarget.value)}
                    placeholder={t('meta.descriptionPlaceholder')}
                    rows={2}
                    value={description}
                />
                {description.length > MAX_DESC && (
                    <p className={styles.err}>{t('meta.descTooLong', { max: MAX_DESC })}</p>
                )}
            </div>
            <KeyValueEditor
                addLabel={t('meta.addLabel')}
                keyPattern={LABEL_KEY_RE}
                label={t('meta.labels')}
                maxPairs={MAX_LABELS}
                onChange={setLabels}
                value={labels}
            />
        </>
    )

    return (
        <Page title={t('title')}>
            <div className={styles.phead}>
                <div>
                    <Eyebrow className={styles.rv}>{t('eyebrow')}</Eyebrow>
                    <h1 className={cx(styles.h1, styles.rv)} style={{ '--d': 1 } as CSSProperties}>
                        {t('h1a')}
                        <br />
                        <span className={styles.gold}>{t('h1b')}</span>
                    </h1>
                    <p className={cx(styles.lead, styles.rv)} style={{ '--d': 2 } as CSSProperties}>
                        <Trans components={rich} i18nKey="lead" ns="namespaces" />
                    </p>
                </div>
                {!forbidden && (
                    <div className={styles.rv} style={{ '--d': 3 } as CSSProperties}>
                        <Button leftSection={<Icon name="plus" size={14} />} onClick={openCreate}>
                            {t('create')}
                        </Button>
                    </div>
                )}
            </div>

            <div className={cx(styles.ctx, styles.rv)} style={{ '--d': 4 } as CSSProperties}>
                <span className={styles.ctxLabel}>{t('ctx.label')}</span>
                <span className={styles.ctxVal}>
                    <i />
                    {current || t('ctx.root')}
                </span>
                {current && (
                    <span className={styles.ctxAct}>
                        <Button onClick={() => switchTo('')} size="xs" variant="default">
                            {t('ctx.toRoot')}
                        </Button>
                    </span>
                )}
            </div>

            {showData && (
                <div className={cx(styles.metrics, styles.rv)} style={{ '--d': 5 } as CSSProperties}>
                    <div className={styles.metric}>
                        <small>{t('metrics.total')}</small>
                        <strong>
                            <CountUp value={all.length} />
                        </strong>
                        <em>{t('metrics.totalHint')}</em>
                    </div>
                    <div className={styles.metric}>
                        <small>{t('metrics.shown')}</small>
                        <strong>
                            <CountUp value={list.length} />
                        </strong>
                        <em>{t('metrics.shownHint')}</em>
                    </div>
                    {totals.secrets !== undefined && (
                        <div className={styles.metric}>
                            <small>{t('metrics.secrets')}</small>
                            <strong>
                                <CountUp value={totals.secrets} />
                            </strong>
                            <em>{t('metrics.secretsHint')}</em>
                        </div>
                    )}
                    {totals.tokens !== undefined && (
                        <div className={styles.metric}>
                            <small>{t('metrics.tokens')}</small>
                            <strong>
                                <CountUp value={totals.tokens} />
                            </strong>
                            <em>{t('metrics.tokensHint')}</em>
                        </div>
                    )}
                    <div className={styles.metric}>
                        <small>{t('metrics.current')}</small>
                        <strong className={styles.mono} title={current || t('ctx.root')}>
                            {current || t('ctx.root')}
                        </strong>
                        <em>{t('metrics.currentHint')}</em>
                    </div>
                </div>
            )}

            {showData && !isEmpty && (
                <div className={cx(styles.toolbar, styles.rv)} style={{ '--d': 6 } as CSSProperties}>
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
                    <div className={styles.seg}>
                        <button
                            className={view === 'cards' ? styles.on : undefined}
                            onClick={() => changeView('cards')}
                            type="button"
                        >
                            {t('view.cards')}
                        </button>
                        <button
                            className={view === 'list' ? styles.on : undefined}
                            onClick={() => changeView('list')}
                            type="button"
                        >
                            {t('view.list')}
                        </button>
                    </div>
                </div>
            )}

            <div className={styles.rv} style={{ '--d': 7 } as CSSProperties}>
                {body}
            </div>

            <Modal
                busy={createMutation.isPending}
                footer={
                    <>
                        <Button
                            disabled={createMutation.isPending}
                            onClick={() => setCreating(false)}
                            variant="default"
                        >
                            {t('createModal.cancel')}
                        </Button>
                        <Button
                            disabled={!trimmed || !!nameErr || metaErr}
                            loading={createMutation.isPending}
                            onClick={() => submitCreate()}
                        >
                            {t('createModal.submit')}
                        </Button>
                    </>
                }
                icon="ns"
                onClose={() => setCreating(false)}
                opened={creating}
                title={t('createModal.title')}
            >
                <form className={styles.form} noValidate onSubmit={submitCreate}>
                    <p className={styles.hint}>{t('createModal.hint')}</p>
                    <div className={styles.fld}>
                        <label htmlFor="ns-name">{t('createModal.name')}</label>
                        <input
                            autoComplete="off"
                            autoFocus
                            className={cx(styles.inp, !!shownErr && styles.bad)}
                            data-autofocus
                            id="ns-name"
                            onChange={(e) => {
                                setName(e.currentTarget.value)
                                setServerErr('')
                            }}
                            placeholder={t('createModal.placeholder')}
                            spellCheck={false}
                            value={name}
                        />
                        {shownErr && <p className={styles.err}>{shownErr}</p>}
                    </div>
                    <div className={styles.prev}>
                        {t('createModal.preview')} <b>{trimmed || t('createModal.placeholder')}</b>
                    </div>
                    {metaFields}
                </form>
            </Modal>

            <Modal
                busy={editMutation.isPending}
                footer={
                    <>
                        <Button disabled={editMutation.isPending} onClick={() => setEditing(null)} variant="default">
                            {t('editModal.cancel')}
                        </Button>
                        <Button disabled={metaErr} loading={editMutation.isPending} onClick={() => submitEdit()}>
                            {t('editModal.submit')}
                        </Button>
                    </>
                }
                icon="gear"
                onClose={() => setEditing(null)}
                opened={editing !== null}
                title={t('editModal.title')}
            >
                <form className={styles.form} noValidate onSubmit={submitEdit}>
                    <p className={styles.hint}>{t('editModal.hint', { name: editing ?? '' })}</p>
                    {metaFields}
                    {serverErr && <p className={styles.err}>{serverErr}</p>}
                </form>
            </Modal>

            <ConfirmModal
                confirmText={toDelete ?? undefined}
                message={
                    toDelete && (
                        <>
                            {t('deleteModal.message', { name: toDelete })}
                            <DeleteImpact stats={byName.get(toDelete)?.stats} />
                            <div className={styles.warn}>
                                <b>{t('deleteModal.warnTitle')}</b> {t('deleteModal.warn')}
                                {toDelete === current && (
                                    <>
                                        <br />
                                        {t('deleteModal.current')}
                                    </>
                                )}
                            </div>
                        </>
                    )
                }
                onClose={() => setToDelete(null)}
                onConfirm={() => confirmDelete(toDelete as string)}
                opened={toDelete !== null}
                title={t('deleteModal.title')}
            />
        </Page>
    )
}
