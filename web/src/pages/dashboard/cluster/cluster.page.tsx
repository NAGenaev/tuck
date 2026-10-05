// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CSSProperties, ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { ClusterStatus, getClusterStatus, removeClusterNode } from '@shared/api/endpoints/cluster'
import {
    ConfirmModal,
    CopyButton,
    CountUp,
    Icon,
    SkeletonLine,
    StateBlock,
    toast
} from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Plate, Pill } from '@shared/ui/tuck/tuck'

import { JoinModal } from './components/join-modal'
import { NodeDrawer, RolePill } from './components/node-drawer'
import { Topology } from './components/topology'
import {
    buildModel,
    diffStatus,
    fmtBytes,
    fmtTime,
    Health,
    httpStatus,
    LogEntry,
    NodeVM,
    serverMessage,
    sortNodes
} from './lib'
import { downloadSnapshot, SnapshotResult } from './snapshot'
import styles from './cluster.module.css'

type Filter = 'all' | 'follower' | 'leader'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')
const d = (n: number) => ({ '--d': n }) as CSSProperties

const BANNER_ICON: Record<Health, ReactNode> = {
    down: (
        <>
            <rect height="11" width="16" x="4" y="10" />
            <path d="M8 10V7a4 4 0 018 0v3" />
        </>
    ),
    electing: (
        <>
            <circle cx="12" cy="12" r="8" />
            <path d="M12 7v5l3 2" />
        </>
    ),
    nofault: (
        <>
            <path d="M12 4l9 16H3z" />
            <path d="M12 10v5M12 18v.01" />
        </>
    ),
    ok: <path d="M5 12l5 5 9-10" />
}

const BANNER_CLASS: Record<Health, string> = {
    down: styles.bad,
    electing: styles.elect,
    nofault: styles.warn,
    ok: styles.ok
}

const rich = { b: <b />, br: <br />, code: <code /> }

export function ClusterPage() {
    const { t, i18n } = useTranslation('cluster')
    const queryClient = useQueryClient()

    const [openId, setOpenId] = useState<null | string>(null)
    const [joining, setJoining] = useState(false)
    const [toRemove, setToRemove] = useState<null | string[]>(null)
    const [selected, setSelected] = useState<Set<string>>(new Set())
    const [filter, setFilter] = useState<Filter>('all')
    const [log, setLog] = useState<LogEntry[]>([])
    const [dl, setDl] = useState<{ bytes: number; ratio: null | number } | null>(null)
    const [downloads, setDownloads] = useState<SnapshotResult[]>([])

    const {
        data: status,
        error,
        isError,
        isFetching,
        isLoading,
        refetch
    } = useQuery({
        queryKey: ['cluster', 'status'],
        queryFn: getClusterStatus,
        refetchInterval: 5_000,
        retry: false
    })

    const code = isError && !status ? httpStatus(error) : undefined
    const forbidden = code === 403
    const notEnabled = code === 501
    const failed = isError && !status && !forbidden && !notEnabled
    const live = !!status

    const model = useMemo(() => (status ? buildModel(status) : null), [status])
    const sorted = useMemo(() => sortNodes(model?.nodes ?? []), [model])
    const rows = useMemo(
        () =>
            sorted.filter((n) => filter === 'all' || (filter === 'leader' ? n.role === 'leader' : n.role !== 'leader')),
        [sorted, filter]
    )
    const openNode = sorted.find((n) => n.id === openId) ?? null

    // --- events observed by this tab: diff of consecutive polls ---
    const prevRef = useRef<ClusterStatus | null>(null)
    const idRef = useRef(0)
    useEffect(() => {
        if (!status) return
        const events = diffStatus(prevRef.current, status)
        prevRef.current = status
        if (!events.length) return
        const ts = Date.now()
        setLog((l) => [...l, ...events.map((e) => ({ ...e, id: ++idRef.current, ts }))].slice(-200))
    }, [status])

    const logRef = useRef<HTMLDivElement>(null)
    useEffect(() => {
        const el = logRef.current
        if (el) el.scrollTop = el.scrollHeight
    }, [log.length])

    // drop selection / drawer for nodes that no longer exist
    useEffect(() => {
        if (!model) return
        const ids = new Set(model.nodes.map((n) => n.id))
        setSelected((s) => {
            const next = new Set([...s].filter((id) => ids.has(id)))
            return next.size === s.size ? s : next
        })
        setOpenId((o) => (o && !ids.has(o) ? null : o))
    }, [model])

    // --- remove ---
    const removeMutation = useMutation({ mutationFn: removeClusterNode })

    const confirmRemove = async (ids: string[]) => {
        const done: string[] = []
        try {
            for (const id of ids) {
                await removeMutation.mutateAsync(id)
                done.push(id)
            }
        } catch (err) {
            toast.error(t('remove.failed'), serverMessage(err) ?? t('remove.failedHint'))
            throw err
        } finally {
            if (done.length) {
                toast.success(t('remove.done'), done.join(', '))
                setSelected(new Set())
                setOpenId((o) => (o && done.includes(o) ? null : o))
                await queryClient.invalidateQueries({ queryKey: ['cluster', 'status'] })
            }
        }
    }

    const toggleSel = (id: string) =>
        setSelected((s) => {
            const next = new Set(s)
            if (next.has(id)) next.delete(id)
            else next.add(id)
            return next
        })
    const allChecked = rows.length > 0 && rows.every((r) => selected.has(r.id))
    const toggleAll = () =>
        setSelected((s) => {
            const next = new Set(s)
            rows.forEach((r) => (allChecked ? next.delete(r.id) : next.add(r.id)))
            return next
        })

    // --- snapshot ---
    const snapBusy = dl !== null
    const snapshotNow = async () => {
        if (snapBusy) return
        setDl({ bytes: 0, ratio: null })
        try {
            const res = await downloadSnapshot(setDl)
            setDownloads((l) => [res, ...l].slice(0, 4))
            toast.success(t('snap.done'), res.filename)
        } catch (err) {
            const s = httpStatus(err)
            toast.error(
                t('snap.failed'),
                s === 403 ? t('snap.forbidden') : s === 501 ? t('snap.unsupported') : (serverMessage(err) ?? t('snap.failedHint'))
            )
        } finally {
            setDl(null)
        }
    }

    const fmt = (n: number) => n.toLocaleString(i18n.language)
    const lastSnap = downloads[0]

    // --- remove dialog content ---
    const removeNodes = toRemove ? (model?.nodes ?? []).filter((n) => toRemove.includes(n.id)) : []
    const bulk = !!toRemove && toRemove.length > 1
    const removeVotersLeft = model ? model.voters - removeNodes.filter((n) => n.suffrage !== 'Nonvoter').length : 0
    const removeHasLeader = removeNodes.some((n) => n.role === 'leader')

    const startRemove = (ids: string[]) => setToRemove(ids)
    const bulkIds = [...selected].filter((id) => model?.nodes.some((n) => n.id === id))

    // ================= body =================
    let body: ReactNode
    if (isLoading) {
        body = (
            <div aria-busy>
                <div className={styles.grid}>
                    <Plate className={styles.skelPlate}>
                        <SkeletonLine height={14} width={180} />
                        <svg aria-hidden className={styles.skelSvg} viewBox="0 0 760 300">
                            <ellipse className={styles.dial} cx="380" cy="150" rx="270" ry="120" />
                            <g className={styles.sweep} style={{ transformOrigin: '380px 150px' }}>
                                <line stroke="rgba(215,169,75,.3)" x1="380" x2="380" y1="150" y2="40" />
                            </g>
                            <rect fill="#0d0f17" height="70" stroke="#3B4154" width="168" x="296" y="10" />
                            <rect fill="#0d0f17" height="70" stroke="#3B4154" width="168" x="40" y="170" />
                            <rect fill="#0d0f17" height="70" stroke="#3B4154" width="168" x="552" y="170" />
                        </svg>
                        <SkeletonLine width="60%" />
                    </Plate>
                    <div className={styles.side}>
                        <div className={styles.metrics}>
                            {[0, 1, 2, 3].map((i) => (
                                <div className={styles.m} key={i}>
                                    <SkeletonLine width="50%" />
                                    <div style={{ height: 12 }} />
                                    <SkeletonLine height={24} />
                                </div>
                            ))}
                        </div>
                        <Plate className={styles.panel}>
                            <SkeletonLine width="40%" />
                            <div style={{ height: 16 }} />
                            <SkeletonLine height={10} />
                        </Plate>
                    </div>
                </div>
                <div className={styles.tbl} style={{ display: 'grid', gap: 14, padding: 18 }}>
                    <SkeletonLine />
                    <SkeletonLine />
                    <SkeletonLine />
                </div>
                <p className={styles.skelHint}>GET /v1/sys/cluster · {t('loading')}</p>
            </div>
        )
    } else if (forbidden) {
        body = (
            <Plate variant="danger">
                <div className={styles.plateTop}>
                    <span className={styles.dot} />
                    <span className={styles.dot} />
                    <span className={styles.dot} />
                    <b style={{ color: 'var(--alarm)' }}>{t('forbidden.bar')}</b>
                    <em>HTTP 403</em>
                </div>
                <div className={styles.centerPlate}>
                    <div>
                        <Eyebrow className={styles.alarmEyebrow}>{t('forbidden.eyebrow')}</Eyebrow>
                        <h2 className={styles.h2big}>
                            <Trans components={rich} i18nKey="forbidden.title" ns="cluster" />
                        </h2>
                        <p className={styles.lead0}>
                            <Trans components={{ code: <code className={styles.mono} /> }} i18nKey="forbidden.text" ns="cluster" />
                        </p>
                        <div className={styles.term} style={{ marginTop: 20 }}>
                            <span className={styles.c}># {t('forbidden.termComment')}</span>
                            {'\n'}
                            <span className={styles.k}>path</span>:{'   '}
                            <span className={styles.s}>sys/cluster</span>
                            {'\n'}
                            <span className={styles.k}>need</span>:{'   '}
                            <span className={styles.s}>read</span>
                            {'\n'}
                            <span className={styles.k}>result</span>: <span style={{ color: 'var(--alarm)' }}>permission denied</span>
                        </div>
                        <div className={styles.actions}>
                            <Button loading={isFetching} onClick={() => void refetch()}>
                                {t('forbidden.retry')}
                            </Button>
                        </div>
                    </div>
                    <svg aria-hidden className={styles.lock403} fill="none" viewBox="0 0 200 220">
                        <rect height="140" stroke="var(--line2)" strokeDasharray="4 6" width="140" x="30" y="20" />
                        <path d="M70 90V64a30 30 0 0160 0v26" stroke="var(--alarm)" strokeWidth="3" />
                        <rect fill="rgba(255,90,77,.08)" height="70" stroke="var(--alarm)" strokeWidth="3" width="96" x="52" y="90" />
                        <circle cx="100" cy="122" fill="var(--alarm)" r="8" />
                        <path d="M100 128v16" stroke="var(--alarm)" strokeWidth="3" />
                        <text
                            fill="var(--dim)"
                            fontFamily="JetBrains Mono"
                            fontSize="11"
                            letterSpacing="3"
                            textAnchor="middle"
                            x="100"
                            y="200"
                        >
                            403 FORBIDDEN
                        </text>
                    </svg>
                </div>
            </Plate>
        )
    } else if (notEnabled) {
        body = (
            <Plate>
                <div className={styles.plateTop}>
                    <span className={styles.dot} />
                    <span className={styles.dot} />
                    <span className={styles.dot} />
                    <b>{t('single.bar')}</b>
                    <em>{t('single.barState')}</em>
                </div>
                <div className={styles.centerPlate}>
                    <div>
                        <Eyebrow>{t('single.eyebrow')}</Eyebrow>
                        <h2 className={styles.h2big}>
                            <Trans components={{ em: <em /> }} i18nKey="single.title" ns="cluster" />
                        </h2>
                        <p className={styles.lead0}>{t('single.text')}</p>
                        <ol className={styles.obSteps}>
                            <li>
                                <i>01</i>
                                <span>
                                    <Trans components={rich} i18nKey="single.s1" ns="cluster" />
                                </span>
                            </li>
                            <li>
                                <i>02</i>
                                <span>
                                    <Trans components={rich} i18nKey="single.s2" ns="cluster" />
                                </span>
                            </li>
                            <li>
                                <i>03</i>
                                <span>
                                    <Trans components={rich} i18nKey="single.s3" ns="cluster" />
                                </span>
                            </li>
                        </ol>
                    </div>
                    <TermBlock lines={SINGLE_CMD} />
                </div>
            </Plate>
        )
    } else if (failed) {
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
    } else if (status && model) {
        const h = model.health
        const stateTxt = status.state || '—'
        const roleTile =
            status.state === 'Shutdown'
                ? { tone: styles.bad, v: t('metrics.roleDown') }
                : status.is_leader
                  ? { tone: styles.warn, v: t('role.leader') }
                  : status.state === 'Candidate'
                    ? { tone: '', v: t('role.candidate') }
                    : { tone: styles.ok, v: t('role.follower') }
        const writeTile =
            h === 'down'
                ? { tone: styles.bad, v: t('metrics.writeDown') }
                : h === 'electing'
                  ? { tone: styles.warn, v: t('metrics.writeElect') }
                  : { tone: h === 'nofault' ? styles.warn : styles.ok, v: t('metrics.writeOk') }
        const gap = t('banner.' + h + '.text', {
            count: model.voters,
            leader: status.leader || '—',
            quorum: model.quorum
        })

        body = (
            <>
                <div
                    aria-live="polite"
                    className={cx(styles.banner, BANNER_CLASS[h], styles.rv)}
                    role="status"
                    style={d(3)}
                >
                    <div className={styles.bi}>
                        <svg fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24">
                            {BANNER_ICON[h]}
                        </svg>
                    </div>
                    <div>
                        <b>{t(`banner.${h}.title`)}</b>
                        <span>{gap}</span>
                    </div>
                </div>

                {!status.is_leader && !!status.leader && (
                    <p className={cx(styles.note, styles.rv)} style={d(3)}>
                        <Icon name="info" size={14} />
                        <span>{t('followerNote', { addr: status.leader_addr || status.leader })}</span>
                    </p>
                )}

                <div className={styles.grid}>
                    <Plate className={cx(styles.hero, styles.rv)} style={d(4)} variant={h === 'down' ? 'danger' : h === 'nofault' ? 'brass' : 'default'}>
                        <div className={styles.plateTop} data-health={h}>
                            <span className={styles.dot} />
                            <span className={styles.dot} />
                            <span className={styles.dot} />
                            <b>{t('topo.title')}</b>
                            <em>{t('topo.caption', { count: model.nodes.length })}</em>
                        </div>
                        <div className={styles.topoScroll} data-health={h}>
                            <Topology model={model} onOpen={setOpenId} openId={openId} state={stateTxt} />
                        </div>
                        <div className={styles.legend}>
                            <span>
                                <i style={{ background: 'var(--brass)' }} />
                                {t('legend.leader')}
                            </span>
                            <span>
                                <i style={{ background: 'var(--mint)' }} />
                                {t('legend.follower')}
                            </span>
                            <span>
                                <i style={{ background: 'var(--ice)' }} />
                                {t('legend.nonvoter')}
                            </span>
                            <span className={styles.legendHint}>{t('legend.hint')}</span>
                        </div>
                    </Plate>

                    <div className={styles.side}>
                        <div className={cx(styles.metrics, styles.rv)} style={d(5)}>
                            <div className={cx(styles.m, roleTile.tone)}>
                                <small>{t('metrics.role')}</small>
                                <strong className={styles.disp}>{roleTile.v}</strong>
                                <em>{status.leader && status.is_leader ? status.leader : t('metrics.roleHint')}</em>
                            </div>
                            <div className={styles.m}>
                                <small>{t('metrics.leaderAddr')}</small>
                                <strong className={styles.smallMono} title={status.leader_addr}>
                                    {status.leader_addr || '—'}
                                </strong>
                                <em>{status.leader || t('metrics.noLeader')}</em>
                            </div>
                            <div className={cx(styles.m, writeTile.tone)}>
                                <small>{t('metrics.state')}</small>
                                <strong className={cx(styles.disp, styles.disp15)}>{writeTile.v}</strong>
                                <em>state: {stateTxt}</em>
                            </div>
                            <div className={cx(styles.m, model.tolerated >= 1 ? styles.ok : styles.warn)}>
                                <small>{t('metrics.tolerance')}</small>
                                <strong className={styles.num}>
                                    <CountUp value={model.tolerated} />
                                </strong>
                                <em>{t('metrics.toleranceHint', { quorum: model.quorum, voters: model.voters })}</em>
                            </div>
                        </div>

                        <Plate className={cx(styles.panel, styles.rv)} style={d(6)}>
                            <h2 className={styles.sec}>{t('quorum.title')}</h2>
                            <div aria-hidden className={styles.quorum}>
                                {sorted.map((n) => (
                                    <i
                                        className={cx(
                                            n.role === 'leader' ? styles.ld : n.role === 'nonvoter' ? styles.nv : styles.on
                                        )}
                                        key={n.id}
                                        title={n.id}
                                    />
                                ))}
                            </div>
                            <div className={styles.qtxt}>
                                <span>{t('quorum.voters', { count: model.voters })}</span>
                                <span>{t('quorum.rule', { quorum: model.quorum })}</span>
                            </div>
                            {model.voters % 2 === 0 && model.voters > 0 && <p className={styles.qwarn}>{t('quorum.even')}</p>}
                            {model.voters < 3 && model.voters > 0 && <p className={styles.qwarn}>{t('quorum.lt3')}</p>}
                        </Plate>
                    </div>
                </div>

                <section className={cx(styles.blk, styles.rv)} style={d(8)}>
                    <div className={styles.secHead}>
                        <div className={styles.l}>
                            <Eyebrow className={styles.eb0}>{t('nodes.eyebrow')}</Eyebrow>
                            <h2 className={styles.sec}>{t('nodes.title')}</h2>
                        </div>
                        <div className={styles.seg} role="group">
                            {(['all', 'leader', 'follower'] as const).map((f) => (
                                <button
                                    aria-pressed={filter === f}
                                    className={cx(styles.chip, filter === f && styles.on)}
                                    key={f}
                                    onClick={() => setFilter(f)}
                                    type="button"
                                >
                                    {t(`filter.${f}`)}
                                </button>
                            ))}
                        </div>
                    </div>
                    {bulkIds.length > 0 && (
                        <div className={styles.bulk} role="status">
                            <span>{t('bulk.selected', { count: bulkIds.length })}</span>
                            <Button color="red" onClick={() => startRemove(bulkIds)} size="compact-sm" variant="outline">
                                {t('bulk.remove')}
                            </Button>
                            <Button onClick={() => setSelected(new Set())} size="compact-sm" variant="default">
                                {t('bulk.clear')}
                            </Button>
                        </div>
                    )}
                    <div className={styles.tbl}>
                        <table>
                            <thead>
                                <tr>
                                    <th style={{ width: 36 }}>
                                        <input
                                            aria-label={t('bulk.all')}
                                            checked={allChecked}
                                            className={styles.chk}
                                            onChange={toggleAll}
                                            type="checkbox"
                                        />
                                    </th>
                                    <th>{t('table.id')}</th>
                                    <th>{t('table.addr')}</th>
                                    <th>{t('table.role')}</th>
                                    <th>{t('table.suffrage')}</th>
                                    <th />
                                </tr>
                            </thead>
                            <tbody>
                                {rows.length === 0 ? (
                                    <tr className={styles.emptyRow}>
                                        <td colSpan={6}>{model.nodes.length === 0 ? t('table.none') : t('table.noMatch')}</td>
                                    </tr>
                                ) : (
                                    rows.map((n) => (
                                        <NodeRow
                                            checked={selected.has(n.id)}
                                            key={n.id}
                                            node={n}
                                            onOpen={() => setOpenId(n.id)}
                                            onRemove={() => startRemove([n.id])}
                                            onToggle={() => toggleSel(n.id)}
                                            open={openId === n.id}
                                        />
                                    ))
                                )}
                            </tbody>
                        </table>
                    </div>
                </section>

                <div className={styles.low}>
                    <Plate className={cx(styles.panel, styles.rv)} style={d(9)}>
                        <div className={styles.secHead} style={{ margin: '0 0 4px' }}>
                            <h2 className={styles.sec}>{t('snap.title')}</h2>
                            <Pill dot tone={lastSnap ? 'ok' : 'mute'}>
                                {lastSnap ? t('snap.pillDone') : t('snap.pillNone')}
                            </Pill>
                        </div>
                        <p className={styles.sub}>{t('snap.sub')}</p>
                        <div className={styles.snapNow}>
                            <div>
                                <small>{t('snap.last')}</small>
                                <strong>{lastSnap ? fmtTime(lastSnap.savedAt) : '—'}</strong>
                            </div>
                            <div>
                                <small>{t('snap.size')}</small>
                                <strong className={styles.num}>{lastSnap ? fmtBytes(lastSnap.bytes, i18n.language) : '—'}</strong>
                            </div>
                            <div>
                                <small>{t('snap.session')}</small>
                                <strong className={styles.num}>{fmt(downloads.length)}</strong>
                            </div>
                        </div>
                        {snapBusy && (
                            <div
                                aria-valuemax={100}
                                aria-valuemin={0}
                                aria-valuenow={dl?.ratio != null ? Math.round(dl.ratio * 100) : undefined}
                                className={styles.prog}
                                role="progressbar"
                            >
                                <i
                                    className={dl?.ratio == null ? styles.indet : undefined}
                                    style={dl?.ratio != null ? { width: `${Math.round(dl.ratio * 100)}%` } : undefined}
                                />
                            </div>
                        )}
                        <div className={styles.row}>
                            <Button
                                leftSection={<Icon name="download" size={15} />}
                                loading={snapBusy}
                                onClick={() => void snapshotNow()}
                            >
                                {snapBusy ? t('snap.busy', { size: fmtBytes(dl?.bytes ?? 0, i18n.language) }) : t('snap.download')}
                            </Button>
                        </div>
                        {downloads.length > 0 && (
                            <ul className={styles.snaplist}>
                                {downloads.map((x) => (
                                    <li key={x.savedAt}>
                                        <b>{x.filename}</b>
                                        <span className={styles.grow}>
                                            {fmtTime(x.savedAt)} · {fmtBytes(x.bytes, i18n.language)}
                                        </span>
                                    </li>
                                ))}
                            </ul>
                        )}
                    </Plate>

                    <Plate className={cx(styles.panel, styles.rv)} style={d(10)}>
                        <h2 className={styles.sec}>{t('add.title')}</h2>
                        <p className={styles.sub}>{t('add.sub')}</p>
                        <ol className={styles.steps}>
                            <li>
                                <Trans components={rich} i18nKey="add.s1" ns="cluster" />
                            </li>
                            <li>
                                <Trans components={rich} i18nKey="add.s2" ns="cluster" />
                            </li>
                            <li>
                                <Trans components={rich} i18nKey="add.s3" ns="cluster" />
                            </li>
                        </ol>
                        <TermBlock lines={JOIN_CMD} />
                    </Plate>
                </div>

                <section className={cx(styles.blk, styles.rv)} style={d(11)}>
                    <div className={styles.secHead}>
                        <div className={styles.l}>
                            <Eyebrow className={styles.eb0}>{t('log.eyebrow')}</Eyebrow>
                            <h2 className={styles.sec}>{t('log.title')}</h2>
                        </div>
                        <Pill tone="mute">{t('log.count', { count: log.length })}</Pill>
                    </div>
                    <div className={styles.log} ref={logRef} role="log">
                        {log.map((e) => (
                            <div key={e.id}>
                                <span className={styles.lt}>{fmtTime(e.ts)}</span>
                                <span className={cx(styles.lv, styles[`lv_${e.level}`])}>{t(`log.level.${e.level}`)}</span>
                                <span className={styles.m2}>{t(`log.ev.${e.key}`, e.params)}</span>
                            </div>
                        ))}
                    </div>
                    <p className={styles.logNote}>{t('log.note')}</p>
                </section>
            </>
        )
    }

    const canAct = live && !!model
    return (
        <Page title={t('title')}>
            <header className={styles.head}>
                <div className={styles.rv} style={d(0)}>
                    <Eyebrow>{t('eyebrow')}</Eyebrow>
                    <h1 className={styles.h1}>
                        {t('h1a')} <span className={styles.gold}>{t('h1b')}</span>
                    </h1>
                    <p className={styles.lead}>
                        <Trans components={rich} i18nKey="lead" ns="cluster" />
                    </p>
                </div>
                <div className={cx(styles.headCta, styles.rv)} style={d(1)}>
                    <Button
                        disabled={!canAct}
                        leftSection={<Icon name="download" size={15} />}
                        loading={snapBusy}
                        onClick={() => void snapshotNow()}
                        variant="default"
                    >
                        {t('snapshot')}
                    </Button>
                    <Button disabled={!canAct} leftSection={<Icon name="plus" size={14} />} onClick={() => setJoining(true)}>
                        {t('join.open')}
                    </Button>
                </div>
            </header>

            {body}

            <JoinModal
                isLeader={!!status?.is_leader}
                leaderAddr={status?.leader_addr ?? ''}
                nodes={model?.nodes ?? []}
                onClose={() => setJoining(false)}
                onJoined={(_, votersAfter) => {
                    void queryClient.invalidateQueries({ queryKey: ['cluster', 'status'] })
                    if (votersAfter % 2 === 0) toast.info(t('toast.evenTitle'), t('toast.evenMsg', { count: votersAfter }))
                }}
                opened={joining}
            />

            <NodeDrawer node={openNode} onClose={() => setOpenId(null)} onRemove={(id) => startRemove([id])} />

            <ConfirmModal
                confirmLabel={bulk ? t('remove.confirmMany', { count: toRemove?.length ?? 0 }) : t('remove.confirm')}
                confirmText={toRemove ? (bulk ? t('bulk.word') : toRemove[0]) : undefined}
                message={
                    toRemove && (
                        <>
                            {bulk ? (
                                <>{t('remove.textMany', { ids: toRemove.join(', ') })}</>
                            ) : (
                                t('remove.text', { id: toRemove[0] })
                            )}
                            <div className={styles.rmWarn}>
                                <b>{t('remove.warnTitle')}</b>{' '}
                                {t('remove.left', { count: Math.max(0, removeVotersLeft) })}
                                {removeVotersLeft < 3 ? ` ${t('remove.warnLt3')}` : removeVotersLeft % 2 === 0 ? ` ${t('remove.warnEven')}` : ''}
                                {removeHasLeader && (
                                    <>
                                        <br />
                                        {t('remove.warnLeader')}
                                    </>
                                )}
                                {status && !status.is_leader && (
                                    <>
                                        <br />
                                        {t('remove.warnNotLeader', { addr: status.leader_addr || '—' })}
                                    </>
                                )}
                            </div>
                        </>
                    )
                }
                onClose={() => setToRemove(null)}
                onConfirm={() => confirmRemove(toRemove as string[])}
                opened={toRemove !== null}
                title={bulk ? t('remove.titleMany') : t('remove.title')}
            />
        </Page>
    )
}

function NodeRow({
    checked,
    node,
    onOpen,
    onRemove,
    onToggle,
    open
}: {
    checked: boolean
    node: NodeVM
    onOpen: () => void
    onRemove: () => void
    onToggle: () => void
    open: boolean
}) {
    const { t } = useTranslation('cluster')
    return (
        <tr className={cx(open && styles.sel)} onClick={onOpen}>
            <td onClick={(e) => e.stopPropagation()}>
                <input
                    aria-label={t('bulk.row', { id: node.id })}
                    checked={checked}
                    className={styles.chk}
                    onChange={onToggle}
                    type="checkbox"
                />
            </td>
            <td className={styles.id}>
                {node.id}
                {node.isMe && <small>{t('table.me')}</small>}
            </td>
            <td>{node.address}</td>
            <td>
                <RolePill role={node.role} />
            </td>
            <td>{node.suffrage}</td>
            <td className={styles.act} onClick={(e) => e.stopPropagation()}>
                <Button color="red" onClick={onRemove} size="compact-sm" variant="outline">
                    {t('table.remove')}
                </Button>
            </td>
        </tr>
    )
}

/* Real flags of cmd/tuck (see `tuck -h`): Raft HA backend. */
const SINGLE_CMD = [
    { c: '# node-1 (bootstrap)' },
    { p: '$', w: 'tuck', r: ' --cluster --cluster-bootstrap \\' },
    { r: '    --cluster-node-id ', s: 'node-1', after: ' \\' },
    { r: '    --cluster-bind-addr ', s: '0.0.0.0:8201', after: ' \\' },
    { r: '    --cluster-dir ', s: '/var/lib/tuck/raft' },
    { blank: true },
    { c: '# node-2, node-3' },
    { p: '$', w: 'tuck', r: ' --cluster --cluster-node-id ', s: 'node-2', after: ' \\' },
    { r: '    --cluster-join ', s: 'https://node-1:8200' }
]

const JOIN_CMD = [
    { c: '# new node' },
    { p: '$', w: 'tuck', r: ' --cluster --cluster-node-id ', s: 'node-4', after: ' \\' },
    { r: '    --cluster-bind-addr ', s: '0.0.0.0:8201', after: ' \\' },
    { r: '    --cluster-join ', s: 'https://leader:8200' }
]

type Line = { after?: string; blank?: boolean; c?: string; p?: string; r?: string; s?: string; w?: string }

function termText(lines: Line[]) {
    return lines
        .map((l) => (l.blank ? '' : l.c ?? `${l.p ? '$ ' : ''}${l.w ?? ''}${l.r ?? ''}${l.s ?? ''}${l.after ?? ''}`))
        .join('\n')
}

function TermBlock({ lines }: { lines: Line[] }) {
    return (
        <div className={styles.term}>
            <span className={styles.copy}>
                <CopyButton size={30} value={termText(lines)} />
            </span>
            {lines.map((l, i) => (
                <div key={i}>
                    {l.blank ? (
                        ' '
                    ) : l.c ? (
                        <span className={styles.c}>{l.c}</span>
                    ) : (
                        <>
                            {l.p && <span className={styles.p}>{l.p} </span>}
                            {l.w && <span className={styles.w}>{l.w}</span>}
                            {l.r}
                            {l.s && <span className={styles.s}>{l.s}</span>}
                            {l.after}
                        </>
                    )}
                </div>
            ))}
        </div>
    )
}
