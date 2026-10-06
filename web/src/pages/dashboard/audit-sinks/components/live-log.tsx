// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AuditEvent, getAuditEvents, getAuditStats, postAuditVerify } from '@shared/api/endpoints/audit'
import { CopyButton, CountUp, Icon, SkeletonLine, Sparkline } from '@shared/ui/kit'
import { Plate, Pill } from '@shared/ui/tuck/tuck'

import {
    downloadEventsNdjson,
    filterStateToApi,
    LogFilterState,
    OPERATIONS,
    operationOf,
    severityOf,
    STATUS_FILTERS,
    TIME_RANGES
} from '../lib'
import { useLiveLog, LIVE_LOG_CAP } from '../use-live-log'
import styles from '../audit-sinks.module.css'

const eventKey = (e: AuditEvent) => (e.seq ? `s${e.seq}` : e.hash || `${e.time}-${e.path}`)

function PauseIcon() {
    return (
        <svg fill="currentColor" height="13" viewBox="0 0 24 24" width="13">
            <path d="M6 4h4v16H6zM14 4h4v16h-4z" />
        </svg>
    )
}

function PlayIcon() {
    return (
        <svg fill="currentColor" height="13" viewBox="0 0 24 24" width="13">
            <path d="M7 4l13 8-13 8z" />
        </svg>
    )
}

const DEFAULT_FILTER: LogFilterState = { path: '', operation: 'all', status: 'all', namespace: 'all', q: '', range: '1h' }

export function LiveLogSection() {
    const { t } = useTranslation('audit')
    const [filter, setFilter] = useState<LogFilterState>(DEFAULT_FILTER)
    const apiFilter = useMemo(() => filterStateToApi(filter), [filter])
    const [openId, setOpenId] = useState<null | string>(null)

    const eventsQuery = useQuery({
        queryKey: ['audit', 'events', apiFilter],
        queryFn: () => getAuditEvents({ ...apiFilter, limit: 200 }),
        refetchInterval: 20_000,
        retry: (count, err) => !(isAxiosError(err) && err.response?.status === 403) && count < 2
    })
    const forbidden = eventsQuery.isError && isAxiosError(eventsQuery.error) && eventsQuery.error.response?.status === 403

    const statsQuery = useQuery({
        queryKey: ['audit', 'stats'],
        queryFn: () => getAuditStats(30),
        refetchInterval: 15_000,
        enabled: !forbidden,
        retry: false
    })

    const live = useLiveLog(apiFilter, !forbidden)

    const merged = useMemo(() => {
        const map = new Map<string, AuditEvent>()
        for (const e of live.rows) map.set(eventKey(e), e)
        for (const e of eventsQuery.data?.events ?? []) {
            const k = eventKey(e)
            if (!map.has(k)) map.set(k, e)
        }
        return Array.from(map.values())
            .sort((a, b) => (b.seq ?? 0) - (a.seq ?? 0))
            .slice(0, LIVE_LOG_CAP)
    }, [live.rows, eventsQuery.data])

    const [verifyBusy, setVerifyBusy] = useState(false)
    const [verifyResult, setVerifyResult] = useState<null | { brokenAt?: number; ok: boolean; when: string }>(null)
    const runVerify = async () => {
        setVerifyBusy(true)
        try {
            const res = await postAuditVerify('memory')
            setVerifyResult({ ok: res.ok, brokenAt: res.memory?.first_broken, when: res.verified_at })
        } catch {
            setVerifyResult({ ok: false, when: new Date().toISOString() })
        } finally {
            setVerifyBusy(false)
        }
    }

    const stats = statsQuery.data?.stats
    const perMinute = stats?.per_minute ?? []
    const peak = perMinute.length ? Math.max(...perMinute.map((m) => m.count)) : 0

    if (forbidden) return null

    return (
        <>
            <section aria-label={t('tiles.label')} className={styles.tiles2}>
                <div className={styles.tile}>
                    <small>{t('live.today')}</small>
                    <strong>{stats ? <CountUp value={stats.today} /> : <SkeletonLine height={34} width="60%" />}</strong>
                    <p>{t('live.todaySub')}</p>
                </div>
                <div className={styles.tile}>
                    <small>{t('live.rate')}</small>
                    <strong>
                        {stats ? <CountUp value={stats.last_minute} /> : <SkeletonLine height={34} width="40%" />}
                        <em>{t('live.rateUnit')}</em>
                    </strong>
                    <p>{t('live.ratePeak', { n: peak })}</p>
                </div>
            </section>

            <section aria-label={t('live.title')} className={styles.sec}>
                <Plate className={styles.logplate}>
                    <div className={styles.plateTop}>
                        <h3>{t('live.title')}</h3>
                        <span
                            className={styles.livebadge}
                            data-state={live.paused ? 'pause' : live.status === 'live' ? 'live' : 'off'}
                        >
                            <i />
                            {live.paused
                                ? t('live.paused')
                                : live.status === 'live'
                                  ? t('live.live')
                                  : live.status === 'connecting'
                                    ? t('live.connecting')
                                    : t('live.offline')}
                        </span>
                        <div className={styles.sp}>
                            <button
                                className={styles.ib}
                                onClick={live.togglePause}
                                title={live.paused ? t('live.resume') : t('live.pause')}
                                type="button"
                            >
                                {live.paused ? <PlayIcon /> : <PauseIcon />}
                            </button>
                            <button
                                className={styles.ib}
                                disabled={merged.length === 0}
                                onClick={() => downloadEventsNdjson(merged)}
                                title={t('live.export')}
                                type="button"
                            >
                                <Icon name="download" size={15} />
                            </button>
                        </div>
                    </div>

                    <div className={styles.hist}>
                        <div className={styles.rate}>
                            <small>{t('live.histLabel')}</small>
                            <strong>{stats?.last_minute ?? 0}</strong>
                            <p>{t('live.histSub')}</p>
                        </div>
                        <div className={styles.chart}>
                            {perMinute.length > 0 && (
                                <Sparkline
                                    data={perMinute.map((m) => m.count)}
                                    fill
                                    height={90}
                                    label={t('live.histLabel')}
                                    tone="brass"
                                />
                            )}
                        </div>
                    </div>

                    <div className={styles.chain}>
                        <small>{t('live.chain')}</small>
                        <div className={styles.blks}>
                            {verifyResult ? (
                                <Pill tone={verifyResult.ok ? 'ok' : 'bad'}>
                                    {verifyResult.ok
                                        ? t('live.chainOk')
                                        : t('live.chainBroken', { n: verifyResult.brokenAt ?? '?' })}
                                </Pill>
                            ) : (
                                <span className={styles.chainHint}>{t('live.chainHint')}</span>
                            )}
                        </div>
                        <button className={styles.btnSm} disabled={verifyBusy} onClick={() => void runVerify()} type="button">
                            <Icon name="lock" size={14} />
                            {verifyBusy ? t('live.verifying') : t('live.verify')}
                        </button>
                    </div>

                    <div className={styles.ctl}>
                        <input
                            className={styles.fld}
                            onChange={(e) => setFilter((f) => ({ ...f, path: e.target.value }))}
                            placeholder={t('live.pathPlaceholder')}
                            spellCheck={false}
                            type="search"
                            value={filter.path}
                        />
                        <select
                            aria-label={t('live.operation')}
                            className={styles.fld}
                            onChange={(e) => setFilter((f) => ({ ...f, operation: e.target.value }))}
                            value={filter.operation}
                        >
                            {OPERATIONS.map((op) => (
                                <option key={op} value={op}>
                                    {op === 'all' ? t('live.allOps') : op}
                                </option>
                            ))}
                        </select>
                        <select
                            aria-label={t('live.status')}
                            className={styles.fld}
                            onChange={(e) => setFilter((f) => ({ ...f, status: e.target.value }))}
                            value={filter.status}
                        >
                            {STATUS_FILTERS.map((s) => (
                                <option key={s} value={s}>
                                    {s === 'all' ? t('live.allStatus') : s === 'error' ? t('live.errorStatus') : s}
                                </option>
                            ))}
                        </select>
                        <input
                            className={styles.fld}
                            onChange={(e) => setFilter((f) => ({ ...f, namespace: e.target.value || 'all' }))}
                            placeholder={t('live.namespacePlaceholder')}
                            spellCheck={false}
                            value={filter.namespace === 'all' ? '' : filter.namespace}
                        />
                        <input
                            className={styles.fld}
                            onChange={(e) => setFilter((f) => ({ ...f, q: e.target.value }))}
                            placeholder={t('live.qPlaceholder')}
                            spellCheck={false}
                            type="search"
                            value={filter.q}
                        />
                        <div className={styles.seg} role="group">
                            {TIME_RANGES.map((r) => (
                                <button
                                    className={filter.range === r ? styles.on : undefined}
                                    key={r}
                                    onClick={() => setFilter((f) => ({ ...f, range: r }))}
                                    type="button"
                                >
                                    {t(`live.range.${r}`)}
                                </button>
                            ))}
                        </div>
                    </div>

                    {live.gap && (
                        <div className={styles.gapNote} role="status">
                            {t('live.gapNote')}
                            <button onClick={live.clearGap} type="button">
                                {t('live.gapDismiss')}
                            </button>
                        </div>
                    )}

                    <div className={styles.cnt}>
                        <span>{t('live.shown', { n: merged.length, cap: LIVE_LOG_CAP })}</span>
                        {eventsQuery.data && <span>{t('live.matched', { n: eventsQuery.data.matched })}</span>}
                    </div>

                    <div className={styles.scroller}>
                        {eventsQuery.isLoading ? (
                            <div className={styles.skRow}>
                                <SkeletonLine />
                            </div>
                        ) : merged.length === 0 ? (
                            <div className={styles.sinkEmpty}>{t('live.empty')}</div>
                        ) : (
                            <div className={styles.rows}>
                                {merged.map((e) => {
                                    const id = eventKey(e)
                                    const open = openId === id
                                    const sev = severityOf(e.status)
                                    return (
                                        <div className={styles.logRow} data-sev={sev} key={id}>
                                            <button
                                                className={styles.logLine}
                                                onClick={() => setOpenId(open ? null : id)}
                                                type="button"
                                            >
                                                <span className={styles.lTime}>{e.time.slice(11, 23)}</span>
                                                <span className={styles.lOp}>{operationOf(e)}</span>
                                                <span className={styles.lPath}>{e.path}</span>
                                                <span className={styles.lNs}>{e.namespace || 'root'}</span>
                                                <span className={styles.lStatus} data-sev={sev}>
                                                    {e.status}
                                                </span>
                                                <span className={styles.lFp}>{e.fingerprint || '—'}</span>
                                                <Icon
                                                    className={open ? styles.chevOpen : undefined}
                                                    name="right"
                                                    size={14}
                                                />
                                            </button>
                                            {open && (
                                                <div className={styles.logDetail}>
                                                    <pre>{JSON.stringify(e, null, 2)}</pre>
                                                    <div className={styles.logDetailSide}>
                                                        <div>
                                                            <b>{t('live.prevHash')}</b>
                                                            <span>{e.prev_hash.slice(0, 20)}…</span>
                                                        </div>
                                                        <div>
                                                            <b>{t('live.hash')}</b>
                                                            <span>{e.hash.slice(0, 20)}…</span>
                                                        </div>
                                                        {e.request_id && <CopyButton label={t('live.copyRid')} size={24} value={e.request_id} />}
                                                    </div>
                                                </div>
                                            )}
                                        </div>
                                    )
                                })}
                            </div>
                        )}
                    </div>
                </Plate>
            </section>
        </>
    )
}
