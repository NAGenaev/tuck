// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { revokeWrap } from '@shared/api/endpoints/wrapping'
import { ConfirmModal, toast } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import { fmtTime, LogRow, OP_PATH, OP_TONE, short, statusOf } from '../lib'
import { Session } from '../use-session'
import styles from '../wrapping.module.css'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

type Filter = 'all' | 'err' | 'lookup' | 'unwrap' | 'wrap'
const FILTERS: Filter[] = ['all', 'wrap', 'lookup', 'unwrap', 'err']

/** Session journal: every real call this tab made, with bulk revoke of the selected tokens. */
export function LogPlate({ session }: { session: Session }) {
    const { t } = useTranslation('wrapping')
    const [filter, setFilter] = useState<Filter>('all')
    const [sel, setSel] = useState<Set<string>>(new Set())
    const [confirm, setConfirm] = useState(false)

    const rows = session.log.filter((r) =>
        filter === 'all' ? true : filter === 'err' ? !r.ok : r.op === filter
    )
    const selectable = (r: LogRow) => r.token !== ''
    const active = [...sel].filter((tok) => statusOf(session.capsules[tok], session.now) === 'active')
    const allChecked = rows.filter(selectable).length > 0 && rows.filter(selectable).every((r) => sel.has(r.token))

    const toggle = (tok: string) =>
        setSel((s) => {
            const n = new Set(s)
            if (n.has(tok)) n.delete(tok)
            else n.add(tok)
            return n
        })

    const toggleAll = () =>
        setSel((s) => {
            const n = new Set(s)
            for (const r of rows.filter(selectable)) {
                if (allChecked) n.delete(r.token)
                else n.add(r.token)
            }
            return n
        })

    const bulkRevoke = async () => {
        const results = await Promise.allSettled(active.map((tok) => revokeWrap(tok)))
        let ok = 0
        results.forEach((r, i) => {
            const tok = active[i]
            if (r.status === 'fulfilled') {
                ok++
                session.upsert(tok, { revokedAt: Date.now(), status: 'revoked' })
                session.addLog('revoke', tok, true, t('log.revoked'))
            } else {
                session.addLog('revoke', tok, false, t('log.failed'))
            }
        })
        setSel(new Set())
        if (ok > 0) toast.success(t('toast.revoked'), t('toast.revokedCount', { count: ok }))
        if (ok < active.length) toast.error(t('toast.error'), t('toast.revokeFailed'))
    }

    return (
        <Plate className={styles.logwrap}>
            <div className={styles.logh}>
                <h3>{t('log.title')}</h3>
                <div className={styles.row} style={{ gap: 10 }}>
                    <div aria-label={t('log.filter')} className={styles.seg} role="group">
                        {FILTERS.map((f) => (
                            <button
                                aria-pressed={filter === f}
                                key={f}
                                onClick={() => setFilter(f)}
                                type="button"
                            >
                                {t(`log.f.${f}`)}
                            </button>
                        ))}
                    </div>
                    <Button
                        color="red"
                        disabled={sel.size === 0}
                        onClick={() => setConfirm(true)}
                        size="xs"
                        variant="default"
                    >
                        {t('log.bulk')} {sel.size > 0 && `(${sel.size})`}
                    </Button>
                </div>
            </div>
            <div className={styles.tbl}>
                <table>
                    <thead>
                        <tr>
                            <th style={{ width: 44 }}>
                                <input
                                    aria-label={t('log.selectAll')}
                                    checked={allChecked}
                                    onChange={toggleAll}
                                    type="checkbox"
                                />
                            </th>
                            <th>{t('log.time')}</th>
                            <th>{t('log.op')}</th>
                            <th>{t('log.token')}</th>
                            <th>{t('log.path')}</th>
                            <th>{t('log.result')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {rows.length === 0 ? (
                            <tr>
                                <td className={styles.tblEmpty} colSpan={6}>
                                    {session.log.length === 0 ? t('log.empty') : t('log.emptyFilter')}
                                </td>
                            </tr>
                        ) : (
                            rows.map((r) => {
                                const on = selectable(r) && sel.has(r.token)
                                return (
                                    <tr
                                        className={cx(on && styles.sel, r.fresh && styles.rowNew)}
                                        key={r.id}
                                        onClick={() => selectable(r) && toggle(r.token)}
                                    >
                                        <td>
                                            <input
                                                aria-label={t('log.select')}
                                                checked={on}
                                                disabled={!selectable(r)}
                                                onChange={() => selectable(r) && toggle(r.token)}
                                                onClick={(e) => e.stopPropagation()}
                                                type="checkbox"
                                            />
                                        </td>
                                        <td>{fmtTime(r.t)}</td>
                                        <td>
                                            <Pill tone={OP_TONE[r.op]}>{r.op}</Pill>
                                        </td>
                                        <td title={r.token}>{r.token ? short(r.token) : '—'}</td>
                                        <td>{OP_PATH[r.op]}</td>
                                        <td>
                                            <Pill tone={r.ok ? 'g' : 'r'}>{r.text}</Pill>
                                        </td>
                                    </tr>
                                )
                            })
                        )}
                    </tbody>
                </table>
            </div>
            <div className={styles.logNote}>{t('log.note')}</div>

            <ConfirmModal
                confirmLabel={t('log.bulk')}
                message={active.length ? t('log.bulkText', { count: active.length }) : t('log.bulkNone')}
                onClose={() => setConfirm(false)}
                onConfirm={async () => {
                    if (active.length) await bulkRevoke()
                    else setSel(new Set())
                }}
                opened={confirm}
                title={t('log.bulkTitle')}
            />
        </Plate>
    )
}
