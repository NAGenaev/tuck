// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { CSSProperties, Fragment, useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { TuckSecretState, TuckSecretView, listTuckSecrets } from '@shared/api/endpoints/tucksecrets'
import { CountUp, Icon, SkeletonLine, StateBlock } from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Pill, PillTone } from '@shared/ui/tuck/tuck'

import styles from './tucksecrets.module.css'

const REFRESH_MS = 30_000
const STATES: TuckSecretState[] = ['synced', 'error', 'pending', 'deleting']
const STATE_TONE: Record<TuckSecretState, PillTone> = {
    deleting: 'violet',
    error: 'bad',
    pending: 'brass',
    synced: 'ok'
}

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

const rich = { b: <b />, br: <br />, code: <code />, em: <em /> }

interface ApiFailure {
    status?: number
    code?: string
    message?: string
}

function failureOf(err: unknown): ApiFailure {
    if (isAxiosError(err)) {
        const d = err.response?.data as { code?: string; error?: string } | undefined
        return { code: d?.code, message: d?.error ?? err.message, status: err.response?.status }
    }
    return { message: (err as Error)?.message }
}

function useRelative() {
    const { i18n } = useTranslation()
    return (iso?: string): string => {
        if (!iso) return ''
        const ms = new Date(iso).getTime()
        if (Number.isNaN(ms)) return ''
        const sec = Math.round((ms - Date.now()) / 1000)
        const rtf = new Intl.RelativeTimeFormat(i18n.language, { numeric: 'auto' })
        const abs = Math.abs(sec)
        if (abs < 60) return rtf.format(sec, 'second')
        if (abs < 3600) return rtf.format(Math.round(sec / 60), 'minute')
        if (abs < 86400) return rtf.format(Math.round(sec / 3600), 'hour')
        return rtf.format(Math.round(sec / 86400), 'day')
    }
}

function rowKey(it: TuckSecretView) {
    return `${it.cluster ?? ''}/${it.k8s_namespace}/${it.name}`
}

function Details({ item }: { item: TuckSecretView }) {
    const { t, i18n } = useTranslation('tucksecrets')
    const rel = useRelative()
    return (
        <div className={styles.details}>
            <section>
                <h4>{t('details.entries')}</h4>
                {item.entries.length === 0 ? (
                    <p className={styles.dim}>—</p>
                ) : (
                    <table className={styles.mini}>
                        <thead>
                            <tr>
                                <th>{t('details.entryKey')}</th>
                                <th>{t('details.entryPath')}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {item.entries.map((e) => (
                                <tr key={e.key}>
                                    <td className={styles.mono}>{e.key}</td>
                                    <td className={styles.mono}>{e.path}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </section>

            <section>
                <h4>{t('details.settings')}</h4>
                <dl className={styles.kv}>
                    <dt>{t('details.kvVersion')}</dt>
                    <dd>v{item.kv_version}</dd>
                    <dt>{t('details.refreshInterval')}</dt>
                    <dd>{item.refresh_interval || t('details.refreshDefault')}</dd>
                    <dt>{t('details.deletionPolicy')}</dt>
                    <dd>{item.deletion_policy}</dd>
                    {item.synced_revision && (
                        <>
                            <dt>{t('details.revision')}</dt>
                            <dd className={styles.mono}>{item.synced_revision}</dd>
                        </>
                    )}
                    {item.last_sync_time && (
                        <>
                            <dt>{t('details.lastSync')}</dt>
                            <dd title={new Date(item.last_sync_time).toLocaleString(i18n.language)}>
                                {rel(item.last_sync_time)}
                            </dd>
                        </>
                    )}
                    {item.restart_targets && item.restart_targets.length > 0 && (
                        <>
                            <dt>{t('details.restart')}</dt>
                            <dd className={styles.mono}>
                                {item.restart_targets.map((r) => `${r.kind}/${r.name}`).join(', ')}
                            </dd>
                        </>
                    )}
                </dl>
            </section>

            {(item.last_sync_error || (item.conditions && item.conditions.length > 0)) && (
                <section className={styles.wide}>
                    {item.last_sync_error && (
                        <>
                            <h4>{t('details.lastError')}</h4>
                            <pre className={styles.errBox}>{item.last_sync_error}</pre>
                        </>
                    )}
                    {item.conditions && item.conditions.length > 0 && (
                        <>
                            <h4>{t('details.conditions')}</h4>
                            <ul className={styles.conds}>
                                {item.conditions.map((c) => (
                                    <li key={c.type}>
                                        <b>{c.type}</b>
                                        <Pill tone={c.status === 'True' ? 'ok' : 'mute'}>{c.status}</Pill>
                                        {(c.reason || c.message) && (
                                            <span>{[c.reason, c.message].filter(Boolean).join(': ')}</span>
                                        )}
                                    </li>
                                ))}
                            </ul>
                        </>
                    )}
                </section>
            )}
        </div>
    )
}

export function TuckSecretsPage() {
    const { t } = useTranslation('tucksecrets')
    const rel = useRelative()

    const [q, setQ] = useState('')
    const [k8sNs, setK8sNs] = useState('')
    const [cluster, setCluster] = useState('')
    const [stateFilter, setStateFilter] = useState<'' | TuckSecretState>('')
    const [open, setOpen] = useState<ReadonlySet<string>>(new Set())

    const { data, error, isError, isFetching, isLoading, refetch } = useQuery({
        queryKey: ['k8s', 'tucksecrets'],
        queryFn: listTuckSecrets,
        refetchInterval: REFRESH_MS,
        retry: (count, err) => {
            const s = failureOf(err).status
            return s !== 403 && s !== 503 && count < 2
        }
    })

    const items = useMemo(() => data?.items ?? [], [data])
    const clusters = useMemo(() => [...new Set(items.map((i) => i.cluster ?? ''))].sort(), [items])
    const multi = clusters.length > 1 || clusters.some((c) => c !== '')
    const failedClusters = useMemo(() => (data?.clusters ?? []).filter((c) => !c.ok), [data])
    const namespaces = useMemo(() => [...new Set(items.map((i) => i.k8s_namespace))].sort(), [items])
    const counts = useMemo(() => {
        const c: Record<TuckSecretState, number> = { deleting: 0, error: 0, pending: 0, synced: 0 }
        for (const i of items) c[i.state] += 1
        return c
    }, [items])

    const filter = q.trim().toLowerCase()
    const shown = useMemo(
        () =>
            items.filter((i) => {
                if (cluster && (i.cluster ?? '') !== cluster) return false
                if (k8sNs && i.k8s_namespace !== k8sNs) return false
                if (stateFilter && i.state !== stateFilter) return false
                if (!filter) return true
                return (
                    i.name.toLowerCase().includes(filter) ||
                    i.k8s_namespace.toLowerCase().includes(filter) ||
                    i.tuck_namespace.toLowerCase().includes(filter) ||
                    i.secret_name.toLowerCase().includes(filter) ||
                    i.entries.some((e) => e.path.toLowerCase().includes(filter) || e.key.toLowerCase().includes(filter))
                )
            }),
        [items, cluster, k8sNs, stateFilter, filter]
    )

    const toggle = (key: string) =>
        setOpen((prev) => {
            const next = new Set(prev)
            if (next.has(key)) next.delete(key)
            else next.add(key)
            return next
        })

    const failure = isError ? failureOf(error) : null
    const forbidden = failure?.status === 403
    const k8sProblem = failure?.status === 503 ? (failure.code ?? 'k8s_error') : null
    const resetFilters = () => {
        setQ('')
        setK8sNs('')
        setCluster('')
        setStateFilter('')
    }

    let body
    if (isLoading) {
        body = (
            <div className={styles.panel}>
                <SkeletonLine height={18} width="40%" />
                <SkeletonLine height={46} />
                <SkeletonLine height={46} />
                <SkeletonLine height={46} />
            </div>
        )
    } else if (forbidden) {
        body = (
            <div className={cx(styles.panel, styles.bad)}>
                <Eyebrow className={styles.alarmEyebrow}>{t('forbidden.eyebrow')}</Eyebrow>
                <h3>
                    <Trans components={rich} i18nKey="forbidden.title" ns="tucksecrets" />
                </h3>
                <p>{t('forbidden.text')}</p>
                <div className={styles.callout}>
                    <Trans components={rich} i18nKey="forbidden.callout" ns="tucksecrets" />
                </div>
                <Button loading={isFetching} onClick={() => void refetch()} variant="default">
                    {t('forbidden.retry')}
                </Button>
            </div>
        )
    } else if (k8sProblem === 'k8s_unavailable' || k8sProblem === 'k8s_forbidden') {
        const key = k8sProblem === 'k8s_unavailable' ? 'unavailable' : 'k8sForbidden'
        body = (
            <div className={styles.panel}>
                <Eyebrow>{t(`${key}.eyebrow`)}</Eyebrow>
                <h3>
                    <Trans components={rich} i18nKey={`${key}.title`} ns="tucksecrets" />
                </h3>
                <p>
                    <Trans components={rich} i18nKey={`${key}.text`} ns="tucksecrets" />
                </p>
                <div className={cx(styles.callout, styles.calloutNeutral)}>{t(`${key}.callout`)}</div>
                <Button loading={isFetching} onClick={() => void refetch()} variant="default">
                    {t('refresh')}
                </Button>
            </div>
        )
    } else if (isError) {
        body = (
            <StateBlock
                details={failure?.message}
                kind="error"
                message={k8sProblem ? t('k8sError.message') : t('error.message')}
                onRetry={() => void refetch()}
                retrying={isFetching}
                title={k8sProblem ? t('k8sError.title') : t('error.title')}
            />
        )
    } else if (items.length === 0) {
        body = (
            <div className={styles.panel}>
                <Eyebrow>{t('empty.eyebrow')}</Eyebrow>
                <h3>
                    <Trans components={rich} i18nKey="empty.title" ns="tucksecrets" />
                </h3>
                <p>{t('empty.text')}</p>
                <p className={styles.dim}>
                    <Trans components={rich} i18nKey="empty.binding" ns="tucksecrets" />
                </p>
            </div>
        )
    } else if (shown.length === 0) {
        body = (
            <div className={styles.panel}>
                <h3>
                    <Trans components={rich} i18nKey="noMatch.title" ns="tucksecrets" />
                </h3>
                <p>{t('noMatch.text')}</p>
                <Button onClick={resetFilters} variant="default">
                    {t('noMatch.reset')}
                </Button>
            </div>
        )
    } else {
        body = (
            <div className={styles.tableWrap}>
                <table className={styles.table}>
                    <thead>
                        <tr>
                            <th aria-label="expand" className={styles.chev} />
                            <th>{t('col.name')}</th>
                            {multi && <th>{t('col.cluster')}</th>}
                            <th>{t('col.k8sNamespace')}</th>
                            <th>{t('col.tuckNamespace')}</th>
                            <th>{t('col.secret')}</th>
                            <th>{t('col.state')}</th>
                            <th>{t('col.synced')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {shown.map((it) => {
                            const key = rowKey(it)
                            const isOpen = open.has(key)
                            return (
                                <Fragment key={key}>
                                    <tr className={cx(styles.row, isOpen && styles.rowOpen)}>
                                        <td className={styles.chev}>
                                            <button
                                                aria-expanded={isOpen}
                                                aria-label={t(isOpen ? 'details.collapse' : 'details.expand', {
                                                    name: key
                                                })}
                                                onClick={() => toggle(key)}
                                                type="button"
                                            >
                                                <Icon name={isOpen ? 'down' : 'right'} size={14} />
                                            </button>
                                        </td>
                                        <td className={styles.mono}>
                                            <b>{it.name}</b>
                                        </td>
                                        {multi && <td className={styles.mono}>{it.cluster || t('localCluster')}</td>}
                                        <td className={styles.mono}>{it.k8s_namespace}</td>
                                        <td className={styles.mono}>{it.tuck_namespace || t('root')}</td>
                                        <td className={styles.mono}>{it.secret_name}</td>
                                        <td>
                                            <Pill dot tone={STATE_TONE[it.state]}>
                                                {t(`state.${it.state}`)}
                                            </Pill>
                                        </td>
                                        <td className={styles.dim}>
                                            {it.last_sync_time ? rel(it.last_sync_time) : t('never')}
                                        </td>
                                    </tr>
                                    {isOpen && (
                                        <tr className={styles.detailRow}>
                                            <td colSpan={multi ? 8 : 7}>
                                                <Details item={it} />
                                            </td>
                                        </tr>
                                    )}
                                </Fragment>
                            )
                        })}
                    </tbody>
                </table>
            </div>
        )
    }

    const showData = !isLoading && !isError && items.length > 0

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
                        <Trans components={rich} i18nKey="lead" ns="tucksecrets" />
                    </p>
                </div>
                <div className={styles.rv} style={{ '--d': 3 } as CSSProperties}>
                    <Button
                        leftSection={<Icon name="refresh" size={14} />}
                        loading={isFetching && !isLoading}
                        onClick={() => void refetch()}
                        variant="default"
                    >
                        {t('refresh')}
                    </Button>
                    <div className={styles.auto}>{t('autoRefresh')}</div>
                </div>
            </div>

            {showData && (
                <div className={cx(styles.metrics, styles.rv)} style={{ '--d': 4 } as CSSProperties}>
                    <div className={styles.metric}>
                        <small>{t('metrics.total')}</small>
                        <strong>
                            <CountUp value={items.length} />
                        </strong>
                        <em>{t('metrics.totalHint')}</em>
                    </div>
                    <div className={styles.metric}>
                        <small>{t('metrics.synced')}</small>
                        <strong data-tone="ok">
                            <CountUp value={counts.synced} />
                        </strong>
                        <em>{t('metrics.syncedHint')}</em>
                    </div>
                    <div className={styles.metric}>
                        <small>{t('metrics.error')}</small>
                        <strong data-tone={counts.error > 0 ? 'bad' : undefined}>
                            <CountUp value={counts.error} />
                        </strong>
                        <em>{t('metrics.errorHint')}</em>
                    </div>
                    <div className={styles.metric}>
                        <small>{t('metrics.pending')}</small>
                        <strong>
                            <CountUp value={counts.pending} />
                        </strong>
                        <em>{t('metrics.pendingHint')}</em>
                    </div>
                </div>
            )}

            {failedClusters.length > 0 && (
                <div className={cx(styles.callout, styles.calloutWarn)} role="alert">
                    <Trans
                        components={rich}
                        i18nKey="clusterDown"
                        ns="tucksecrets"
                        values={{ names: failedClusters.map((c) => `${c.name || t('localCluster')} (${c.code ?? 'k8s_error'})`).join(', ') }}
                    />
                </div>
            )}

            {showData && (
                <div className={styles.toolbar}>
                    <label className={styles.search}>
                        <Icon name="search" size={15} />
                        <input
                            aria-label={t('search.label')}
                            id="tucksecrets-search"
                            onChange={(e) => setQ(e.target.value)}
                            placeholder={t('search.placeholder')}
                            value={q}
                        />
                    </label>
                    {multi && (
                        <select
                            aria-label={t('filter.cluster')}
                            className={styles.select}
                            id="tucksecrets-cluster"
                            onChange={(e) => setCluster(e.target.value)}
                            value={cluster}
                        >
                            <option value="">{t('filter.allClusters')}</option>
                            {clusters.map((c) => (
                                <option key={c} value={c}>
                                    {c || t('localCluster')}
                                </option>
                            ))}
                        </select>
                    )}
                    <select
                        aria-label={t('filter.namespace')}
                        className={styles.select}
                        id="tucksecrets-ns"
                        onChange={(e) => setK8sNs(e.target.value)}
                        value={k8sNs}
                    >
                        <option value="">{t('filter.allNamespaces')}</option>
                        {namespaces.map((n) => (
                            <option key={n} value={n}>
                                {n}
                            </option>
                        ))}
                    </select>
                    <div aria-label={t('filter.state')} className={styles.seg} role="group">
                        <button
                            aria-pressed={stateFilter === ''}
                            data-on={stateFilter === '' || undefined}
                            onClick={() => setStateFilter('')}
                            type="button"
                        >
                            {t('filter.all')}
                        </button>
                        {STATES.filter((s) => counts[s] > 0 || stateFilter === s).map((s) => (
                            <button
                                aria-pressed={stateFilter === s}
                                data-on={stateFilter === s || undefined}
                                key={s}
                                onClick={() => setStateFilter(s)}
                                type="button"
                            >
                                {t(`state.${s}`)} · {counts[s]}
                            </button>
                        ))}
                    </div>
                </div>
            )}

            {data?.truncated && <div className={styles.notice}>{t('truncated')}</div>}

            {body}
        </Page>
    )
}
