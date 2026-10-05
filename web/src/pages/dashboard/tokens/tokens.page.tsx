// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { CSSProperties, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { listTokens, revokeToken } from '@shared/api/endpoints/token'
import { getNamespace } from '@shared/auth/token'
import {
    ConfirmModal,
    copyText,
    CountUp,
    cx,
    Icon,
    prefersReducedMotion,
    StateBlock,
    TablePager,
    toast
} from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Pill, Plate, PlateBar } from '@shared/ui/tuck/tuck'

import { CreateTokenModal } from './create-token-modal'
import { PrefixRevokeModal } from './prefix-revoke-modal'
import { RevealPlate } from './reveal-plate'
import { SelfDrawer } from './self-drawer'
import { apiError, httpStatus, lookupTokenInfo, renewTokenById, TokenInfo } from './tokens.api'
import s from './tokens.module.css'
import {
    agoText,
    barColor,
    fmtCreated,
    fmtLeft,
    fmtTotal,
    leftSeconds,
    maskId,
    policyTone,
    shortAcc,
    toRow,
    TokenRow
} from './tokens.utils'

const PER_PAGE = 20
/** at most this many tokens get a detail lookup (one request each) */
const ENRICH_CAP = 100
const SOON = 15 * 60
const css = (v: Record<string, number | string>) => v as CSSProperties

export function TokensPage() {
    const { t, i18n } = useTranslation('tokens')
    const qc = useQueryClient()

    const [now, setNow] = useState(() => Date.now())
    const [query, setQuery] = useState('')
    const [policy, setPolicy] = useState<null | string>(null)
    const [page, setPage] = useState(1)
    const [sel, setSel] = useState<Set<string>>(() => new Set())
    const [dissolving, setDissolving] = useState<Set<string>>(() => new Set())
    const [gone, setGone] = useState<Set<string>>(() => new Set())
    const [fresh, setFresh] = useState<null | string>(null)
    const [created, setCreated] = useState<null | TokenInfo>(null)
    const [createOpen, setCreateOpen] = useState(false)
    const [selfOpen, setSelfOpen] = useState(false)
    const [prefixOpen, setPrefixOpen] = useState(false)
    const [confirmIds, setConfirmIds] = useState<string[]>([])
    const [confirmOpen, setConfirmOpen] = useState(false)
    const [renewing, setRenewing] = useState<null | string>(null)
    const [copiedAcc, setCopiedAcc] = useState<null | string>(null)

    const tt = useCallback((k: string, o?: Record<string, unknown>) => t(k, o) as string, [t])
    const [dU, hU, mU] = [t('time.d'), t('time.h'), t('time.m')]

    useEffect(() => {
        const id = window.setInterval(() => setNow(Date.now()), 1000)
        return () => window.clearInterval(id)
    }, [])

    // ---------- data ----------
    const listQ = useQuery({ queryFn: listTokens, queryKey: ['tokens', 'list'], retry: false })
    const ids = useMemo(() => (listQ.data ?? []).filter((id) => !gone.has(id)), [listQ.data, gone])
    const enrichIds = ids.slice(0, ENRICH_CAP)
    const infos = useQueries({
        combine: (rs) => rs.map((r) => ({ data: r.data, pending: r.isPending })),
        queries: enrichIds.map((id) => ({
            queryFn: () => lookupTokenInfo(id),
            queryKey: ['tokens', 'info', id],
            refetchOnWindowFocus: false,
            retry: false,
            staleTime: 30_000
        }))
    })
    const detailsPending = infos.some((i) => i.pending)

    const rows: TokenRow[] = useMemo(() => {
        const list = ids.map((id, i) => toRow(id, infos[i]?.data, infos[i]?.pending ?? false))
        // keep the server order while lookups are still landing so rows do not jump around
        if (infos.some((i) => i.pending)) return list
        return list.sort((a, b) => (b.createdAt ?? 0) - (a.createdAt ?? 0))
    }, [ids, infos])

    const rowName = (r: TokenRow) => r.name || maskId(r.id)

    // ---------- dissolve / revoke ----------
    const dissolve = useCallback(
        (targets: string[]) => {
            setDissolving((cur) => new Set([...cur, ...targets]))
            window.setTimeout(
                () => {
                    setDissolving((cur) => {
                        const next = new Set(cur)
                        targets.forEach((id) => next.delete(id))
                        return next
                    })
                    setGone((cur) => new Set([...cur, ...targets]))
                    void qc.invalidateQueries({ queryKey: ['tokens', 'list'] })
                },
                prefersReducedMotion() ? 10 : 900
            )
        },
        [qc]
    )

    const rowsRef = useRef(rows)
    rowsRef.current = rows

    const revoke = useCallback(
        async (targets: string[]) => {
            const results = await Promise.allSettled(targets.map((id) => revokeToken(id)))
            const ok: string[] = []
            let failed = 0
            let lastErr: unknown
            results.forEach((r, i) => {
                // 404: already gone, e.g. revoked together with its parent
                if (r.status === 'fulfilled' || httpStatus(r.reason) === 404) ok.push(targets[i])
                else {
                    failed++
                    lastErr = r.reason
                }
            })
            if (ok.length) {
                dissolve(ok)
                setSel((cur) => {
                    const next = new Set(cur)
                    ok.forEach((id) => next.delete(id))
                    return next
                })
                if (ok.length === 1) {
                    const r = rowsRef.current.find((x) => x.id === ok[0])
                    toast.success(t('toast.revoked'), t('toast.revokedText', { name: r ? rowName(r) : '' }))
                } else {
                    toast.success(t('toast.revokedMany', { count: ok.length }), t('toast.revokedManyText'))
                }
            }
            if (failed) {
                if (targets.length === 1) toast.error(t('toast.revokeFailed'), apiError(lastErr))
                else toast.error(t('toast.revokePartial'), t('toast.revokePartialText', { count: failed }))
            }
        },
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [dissolve, t]
    )

    // expiry sweep: tokens whose TTL ran out dissolve on their own
    useEffect(() => {
        const expired = rows.filter((r) => r.expiresAt !== null && r.expiresAt <= now && !dissolving.has(r.id))
        if (!expired.length) return
        dissolve(expired.map((r) => r.id))
        if (expired.length === 1) toast.info(t('toast.expired'), t('toast.expiredText', { name: rowName(expired[0]) }))
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [now, rows])

    const renew = async (r: TokenRow) => {
        setRenewing(r.id)
        try {
            await renewTokenById(r.id, r.total || undefined)
            await qc.invalidateQueries({ queryKey: ['tokens', 'info', r.id] })
            toast.success(t('toast.renewed'), t('toast.renewedText', { name: rowName(r), total: fmtTotal(r.total, dU, hU, mU) }))
        } catch (e) {
            toast.error(t('toast.renewFailed'), apiError(e))
        } finally {
            setRenewing(null)
        }
    }

    const copyAccessor = async (r: TokenRow) => {
        if (!r.accessor) return
        if (await copyText(r.accessor)) {
            setCopiedAcc(r.id)
            toast.success(t('toast.accCopied'))
            window.setTimeout(() => setCopiedAcc((cur) => (cur === r.id ? null : cur)), 1600)
        }
    }

    const askRevoke = (targets: string[]) => {
        setConfirmIds(targets)
        setConfirmOpen(true)
    }

    const onCreated = (token: TokenInfo) => {
        setCreateOpen(false)
        setCreated(token)
        setFresh(token.id)
        qc.setQueryData(['tokens', 'info', token.id], token)
        void qc.invalidateQueries({ queryKey: ['tokens', 'list'] })
        window.setTimeout(() => setFresh(null), 2600)
        window.setTimeout(
            () =>
                document
                    .getElementById('tk-reveal')
                    ?.scrollIntoView({ behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'nearest' }),
            120
        )
    }
    const dismissReveal = useCallback(() => setCreated(null), [])

    // ---------- derived ----------
    const policyCounts = useMemo(() => {
        const m = new Map<string, number>()
        rows.forEach((r) => r.policies.forEach((p) => m.set(p, (m.get(p) ?? 0) + 1)))
        return [...m.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, 7)
    }, [rows])

    const q = query.trim().toLowerCase()
    const filtered = rows.filter(
        (r) =>
            (!policy || r.policies.includes(policy)) &&
            (!q ||
                r.name.toLowerCase().includes(q) ||
                (r.accessor ?? '').toLowerCase().includes(q) ||
                r.policies.some((p) => p.toLowerCase().includes(q)))
    )
    const pages = Math.max(1, Math.ceil(filtered.length / PER_PAGE))
    const pg = Math.min(page, pages)
    const pageRows = filtered.slice((pg - 1) * PER_PAGE, pg * PER_PAGE)
    const pageIds = pageRows.map((r) => r.id)
    const allSel = pageIds.length > 0 && pageIds.every((id) => sel.has(id))
    const someSel = pageIds.some((id) => sel.has(id))
    const selIds = rows.filter((r) => sel.has(r.id)).map((r) => r.id)

    const soon = rows.filter((r) => r.expiresAt !== null && leftSeconds(r, now) > 0 && leftSeconds(r, now) < SOON).length
    const renewable = rows.filter((r) => r.renewable).length
    const uses = rows.reduce((n, r) => n + r.uses, 0)

    const resetFilters = () => {
        setQuery('')
        setPolicy(null)
        setPage(1)
    }

    const listReady = listQ.isSuccess
    const forbidden = listQ.isError && httpStatus(listQ.error) === 403
    const errored = listQ.isError && !forbidden
    const stat = (n: number) => (listReady ? <CountUp value={n} /> : '—')
    const ns = getNamespace() || 'root'

    // ---------- table body ----------
    const renderRow = (r: TokenRow, i: number) => {
        const left = leftSeconds(r, now)
        const never = r.resolved && r.expiresAt === null
        const pct = never ? 1 : r.total ? Math.min(1, left / r.total) : 0
        const label = rowName(r)
        return (
            <tr
                className={cx(s.enter, dissolving.has(r.id) && s.dissolving, fresh === r.id && s.fresh)}
                data-sel={sel.has(r.id)}
                key={r.id}
                style={css({ '--i': Math.min(i, 12) })}
            >
                <td>
                    <input
                        aria-label={t('col.select', { name: label })}
                        checked={sel.has(r.id)}
                        className={s.ck}
                        onChange={() =>
                            setSel((cur) => {
                                const next = new Set(cur)
                                if (next.has(r.id)) next.delete(r.id)
                                else next.add(r.id)
                                return next
                            })
                        }
                        type="checkbox"
                    />
                </td>
                <td>
                    {r.loading ? (
                        <div aria-busy style={{ display: 'grid', gap: 6, width: 160 }}>
                            <div style={{ background: 'var(--line2)', height: 12 }} />
                            <div style={{ background: 'var(--line)', height: 10, width: '60%' }} />
                        </div>
                    ) : (
                        <>
                            <span className={cx(s.nm, !r.resolved || !r.name ? s.nmEmpty : undefined)}>
                                {r.resolved ? r.name || t('row.noName') : maskId(r.id)}
                            </span>
                            <span className={s.acc}>
                                {r.accessor ? (
                                    <>
                                        <span title={r.accessor}>{shortAcc(r.accessor)}</span>
                                        <button
                                            aria-label={t('row.copyAccessor')}
                                            data-ok={copiedAcc === r.id}
                                            onClick={() => void copyAccessor(r)}
                                            title={t('row.copyAccessor')}
                                            type="button"
                                        >
                                            <Icon name={copiedAcc === r.id ? 'check' : 'copy'} size={13} />
                                        </button>
                                    </>
                                ) : (
                                    <span>{t('row.unresolved')}</span>
                                )}
                            </span>
                        </>
                    )}
                </td>
                <td>
                    <div className={s.pills}>
                        {r.policies.length
                            ? r.policies.map((p) => (
                                  <Pill key={p} size="sm" tone={policyTone(p)} variant="soft">
                                      {p}
                                  </Pill>
                              ))
                            : r.resolved && '—'}
                    </div>
                </td>
                <td>
                    {r.resolved ? (
                        <div
                            className={s.ttl}
                            data-crit={!never && left > 0 && left < 60}
                            data-inf={never}
                            data-low={!never && left < SOON}
                        >
                            <div className={s.ttlTop}>
                                <span>{fmtLeft(left, dU, hU, mU)}</span>
                                <em>{t('row.ttlOf', { total: fmtTotal(r.total, dU, hU, mU) })}</em>
                            </div>
                            <div aria-hidden className={s.bar}>
                                <i style={css({ '--c': barColor(pct), width: `${pct * 100}%` })} />
                            </div>
                        </div>
                    ) : (
                        <span>—</span>
                    )}
                </td>
                <td>
                    {r.createdAt ? (
                        <>
                            <div className={s.date}>{fmtCreated(r.createdAt, now, i18n.language, tt)}</div>
                            <div className={s.ago}>{agoText(r.createdAt, now, tt)}</div>
                        </>
                    ) : (
                        '—'
                    )}
                </td>
                <td className={s.ns}>{r.resolved ? r.ns : '—'}</td>
                <td className={s.uses}>
                    {r.resolved ? (
                        <>
                            {r.uses}
                            <small> / {r.maxUses || '∞'}</small>
                        </>
                    ) : (
                        '—'
                    )}
                </td>
                <td>
                    <div className={s.acts}>
                        <button
                            aria-label={t('row.renewLabel', { name: label })}
                            className={s.iconBtn}
                            data-busy={renewing === r.id}
                            disabled={!r.resolved || !r.renewable || r.expiresAt === null || renewing !== null}
                            onClick={() => void renew(r)}
                            title={r.resolved && !r.renewable ? t('row.notRenewable') : t('row.renew')}
                            type="button"
                        >
                            <Icon name="refresh" size={15} />
                        </button>
                        <button
                            aria-label={t('row.revokeLabel', { name: label })}
                            className={s.iconBtn}
                            data-d
                            onClick={() => askRevoke([r.id])}
                            title={t('row.revoke')}
                            type="button"
                        >
                            <Icon name="trash" size={15} />
                        </button>
                    </div>
                </td>
            </tr>
        )
    }

    const confirmRows = confirmIds.map((id) => rows.find((r) => r.id === id)).filter((r): r is TokenRow => !!r)

    return (
        <Page title="Tokens">
            <header className={s.head}>
                <div className={s.headGrid}>
                    <div>
                        <Eyebrow>{t('head.eyebrow')}</Eyebrow>
                        <h1 className={s.h1} style={{ marginTop: 18 }}>
                            <span className={s.ln} style={css({ '--d': 0 })}>
                                <span>{t('head.title1')}</span>
                            </span>
                            <span className={s.ln} style={css({ '--d': 1 })}>
                                <span className={s.gold}>{t('head.title2')}</span>
                            </span>
                        </h1>
                        <p className={cx(s.lead, s.ld)} style={css({ '--d': 3 })}>
                            <Trans components={{ b: <b /> }} i18nKey="head.lead" t={t} />
                        </p>
                    </div>
                    <div className={cx(s.cta, s.ld)} style={css({ '--d': 4 })}>
                        <Button leftSection={<Icon name="user" size={15} />} onClick={() => setSelfOpen(true)} variant="default">
                            {t('head.self')}
                        </Button>
                        <Button leftSection={<Icon name="plus" size={15} />} onClick={() => setCreateOpen(true)}>
                            {t('head.create')}
                        </Button>
                    </div>
                </div>
            </header>

            <div className={cx(s.stats, s.ld)} style={css({ '--d': 5 })}>
                <div className={s.stat}>
                    <small>
                        <span className={s.dot} />
                        {t('stats.total')}
                    </small>
                    <strong>{stat(rows.length)}</strong>
                </div>
                <div className={s.stat}>
                    <small>
                        <span className={s.dot} data-t={soon > 0 ? 'r' : 'b'} />
                        {t('stats.soon')}
                    </small>
                    <strong className={soon > 0 ? s.warn : undefined}>{stat(soon)}</strong>
                </div>
                <div className={s.stat}>
                    <small>
                        <span className={s.dot} data-t="b" />
                        {t('stats.renewable')}
                    </small>
                    <strong className={s.goldv}>{stat(renewable)}</strong>
                </div>
                <div className={s.stat}>
                    <small>
                        <span className={s.dot} data-t="b" />
                        {t('stats.uses')}
                    </small>
                    <strong>{stat(uses)}</strong>
                </div>
            </div>

            <RevealPlate onDismiss={dismissReveal} token={created} />

            <div className={s.ld} style={css({ '--d': 6 })}>
                <Plate variant="flat">
                    <PlateBar>{t('plate.endpoint', { ns })}</PlateBar>

                    {listQ.isPending || listReady ? (
                        <>
                            <div className={s.toolbar}>
                                <div className={s.search}>
                                    <Icon name="search" size={15} />
                                    <input
                                        aria-label={t('toolbar.searchLabel')}
                                        autoComplete="off"
                                        className={s.searchIn}
                                        onChange={(e) => {
                                            setQuery(e.currentTarget.value)
                                            setPage(1)
                                        }}
                                        placeholder={t('toolbar.search')}
                                        spellCheck={false}
                                        value={query}
                                    />
                                </div>
                                <div aria-label={t('toolbar.filterLabel')} className={s.chips} role="group">
                                    <button
                                        aria-pressed={policy === null}
                                        className={s.chip}
                                        onClick={() => {
                                            setPolicy(null)
                                            setPage(1)
                                        }}
                                        type="button"
                                    >
                                        {t('toolbar.all')}
                                    </button>
                                    {policyCounts.map(([p, n]) => (
                                        <button
                                            aria-pressed={policy === p}
                                            className={s.chip}
                                            key={p}
                                            onClick={() => {
                                                setPolicy(policy === p ? null : p)
                                                setPage(1)
                                            }}
                                            type="button"
                                        >
                                            {p} · {n}
                                        </button>
                                    ))}
                                </div>
                                <div className={s.tbRight}>
                                    <Button
                                        color="red"
                                        disabled={rows.length === 0}
                                        leftSection={<Icon name="trash" size={14} />}
                                        onClick={() => setPrefixOpen(true)}
                                        size="xs"
                                        variant="default"
                                    >
                                        {t('toolbar.prefix')}
                                    </Button>
                                </div>
                            </div>

                            <div className={s.bulk} data-on={selIds.length > 0} inert={selIds.length === 0}>
                                <div>
                                    <div className={s.bulkIn}>
                                        <b>{t('bulk.selected', { count: selIds.length })}</b>
                                        <span>{t('bulk.hint')}</span>
                                        <span className={s.sp} />
                                        <Button onClick={() => setSel(new Set())} size="xs" variant="default">
                                            {t('bulk.clear')}
                                        </Button>
                                        <Button
                                            color="red"
                                            leftSection={<Icon name="trash" size={14} />}
                                            onClick={() => askRevoke(selIds)}
                                            size="xs"
                                        >
                                            {t('bulk.revoke')}
                                        </Button>
                                    </div>
                                </div>
                            </div>
                        </>
                    ) : null}

                    {forbidden && (
                        <div className={cx(s.panel, s.panelErr)} role="alert">
                            <div className={s.code}>403</div>
                            <h3>{t('forbidden.title')}</h3>
                            <p>
                                <Trans components={{ code: <code /> }} i18nKey="forbidden.text" t={t} />
                            </p>
                            <div className={s.acts2}>
                                <Button
                                    leftSection={<Icon name="user" size={15} />}
                                    onClick={() => setSelfOpen(true)}
                                    variant="default"
                                >
                                    {t('head.self')}
                                </Button>
                                <Button
                                    leftSection={<Icon name="refresh" size={15} />}
                                    loading={listQ.isFetching}
                                    onClick={() => void listQ.refetch()}
                                >
                                    {t('forbidden.retry')}
                                </Button>
                            </div>
                        </div>
                    )}

                    {errored && (
                        <StateBlock
                            details={apiError(listQ.error)}
                            kind="error"
                            message={t('error.text')}
                            onRetry={() => void listQ.refetch()}
                            retrying={listQ.isFetching}
                            title={t('error.title')}
                        />
                    )}

                    {listReady && ids.length === 0 && (
                        <StateBlock
                            action={
                                <Button leftSection={<Icon name="plus" size={15} />} onClick={() => setCreateOpen(true)}>
                                    {t('head.create')}
                                </Button>
                            }
                            icon="key"
                            kind="empty"
                            message={t('empty.text')}
                            title={t('empty.title')}
                        />
                    )}

                    {(listQ.isPending || (listReady && ids.length > 0)) && (
                        <div className={s.tbl}>
                            <table>
                                <thead>
                                    <tr>
                                        <th>
                                            <input
                                                aria-label={t('col.selectAll')}
                                                checked={allSel}
                                                className={s.ck}
                                                disabled={pageIds.length === 0}
                                                onChange={() =>
                                                    setSel((cur) => {
                                                        const next = new Set(cur)
                                                        pageIds.forEach((id) => (allSel ? next.delete(id) : next.add(id)))
                                                        return next
                                                    })
                                                }
                                                ref={(el) => {
                                                    if (el) el.indeterminate = someSel && !allSel
                                                }}
                                                type="checkbox"
                                            />
                                        </th>
                                        <th>{t('col.token')}</th>
                                        <th>{t('col.policies')}</th>
                                        <th>{t('col.ttl')}</th>
                                        <th>{t('col.created')}</th>
                                        <th>{t('col.ns')}</th>
                                        <th>{t('col.uses')}</th>
                                        <th style={{ textAlign: 'right' }}>{t('col.actions')}</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {listQ.isPending
                                        ? Array.from({ length: 5 }, (_, i) => (
                                              <tr aria-hidden className={s.skel} key={i}>
                                                  {Array.from({ length: 8 }, (_, j) => (
                                                      <td key={j}>
                                                          <div style={{ width: j === 1 ? 150 : j === 0 ? 16 : 70 }} />
                                                      </td>
                                                  ))}
                                              </tr>
                                          ))
                                        : pageRows.map(renderRow)}
                                    {listReady && pageRows.length === 0 && (
                                        <tr>
                                            <td className={s.noMatch} colSpan={8}>
                                                {t('empty.noMatch')}{' '}
                                                <button className={s.linkBtn} onClick={resetFilters} type="button">
                                                    {t('empty.reset')}
                                                </button>
                                            </td>
                                        </tr>
                                    )}
                                </tbody>
                            </table>
                        </div>
                    )}

                    {listReady && ids.length > 0 && (
                        <>
                            <div className={s.tfoot}>
                                <span>
                                    {t('foot.count', { shown: filtered.length, total: rows.length })}
                                    {detailsPending && ` · ${t('foot.loading')}`}
                                    {ids.length > ENRICH_CAP && ` · ${t('foot.enrichCap', { n: ENRICH_CAP })}`}
                                </span>
                                <span>{t('foot.live')}</span>
                            </div>
                            <TablePager onChange={setPage} page={pg} perPage={PER_PAGE} total={filtered.length} />
                        </>
                    )}
                </Plate>
            </div>

            <CreateTokenModal onClose={() => setCreateOpen(false)} onCreated={onCreated} opened={createOpen} />
            <SelfDrawer onClose={() => setSelfOpen(false)} opened={selfOpen} />
            <PrefixRevokeModal
                onClose={() => setPrefixOpen(false)}
                onRevoke={revoke}
                opened={prefixOpen}
                rows={rows}
            />
            <ConfirmModal
                confirmLabel={t('confirm.label')}
                message={
                    <>
                        <p style={{ margin: 0 }}>{t('confirm.text')}</p>
                        {confirmRows.length === 1 && (
                            <p style={{ margin: '10px 0 0' }}>
                                <code>{rowName(confirmRows[0])}</code>
                            </p>
                        )}
                    </>
                }
                onClose={() => setConfirmOpen(false)}
                onConfirm={() => revoke(confirmIds)}
                opened={confirmOpen}
                title={confirmIds.length === 1 ? t('confirm.titleOne') : t('confirm.titleMany', { count: confirmIds.length })}
            />
        </Page>
    )
}
