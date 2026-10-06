// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { revokeWrapByAccessor, WrappedCapsule } from '@shared/api/endpoints/wrapping'
import { ConfirmModal, Icon, toast } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import { clock, fmtBytes, fmtTime, short } from '../lib'
import { Wrapped, WRAPPED_POLL } from '../use-wrapped'
import styles from '../wrapping.module.css'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

/** Seconds left on a capsule below which it is flagged as expiring. */
const LOW_MS = 30_000

/**
 * Live wrapped responses as the server lists them: metadata only, never a token or a payload.
 * Each capsule is addressed by its accessor, which can revoke it but cannot unwrap it.
 */
export function LivePlate({
    now,
    onRevoked,
    onSelect,
    selected,
    wrapped
}: {
    now: number
    onRevoked: (accessors: string[]) => void
    onSelect: (c: WrappedCapsule) => void
    selected: null | string
    wrapped: Wrapped
}) {
    const { t, i18n } = useTranslation('wrapping')
    const [sel, setSel] = useState<Set<string>>(new Set())
    const [targets, setTargets] = useState<null | string[]>(null)

    const rows = wrapped.capsules
    // checkboxes that point at capsules the server no longer lists are dropped
    const checked = [...sel].filter((a) => rows.some((r) => r.accessor === a))
    const allChecked = rows.length > 0 && checked.length === rows.length

    const toggle = (acc: string) =>
        setSel((s) => {
            const n = new Set(s)
            if (n.has(acc)) n.delete(acc)
            else n.add(acc)
            return n
        })

    const revoke = async (accs: string[]) => {
        const results = await Promise.allSettled(accs.map((a) => revokeWrapByAccessor(a)))
        const done = accs.filter((_, i) => results[i].status === 'fulfilled')
        if (done.length > 0) {
            toast.success(t('toast.revoked'), t('toast.revokedCount', { count: done.length }))
            onRevoked(done)
        }
        if (done.length < accs.length) toast.error(t('toast.error'), t('toast.revokeFailed'))
        setSel(new Set())
        await wrapped.refetch()
    }

    const upd = wrapped.updatedAt ? fmtTime(wrapped.updatedAt) : '—'

    return (
        <Plate className={styles.logwrap}>
            <div className={styles.logh}>
                <h3>{t('live.title')}</h3>
                <div className={styles.row} style={{ gap: 10 }}>
                    {wrapped.scope && (
                        <Pill tone={wrapped.scope === 'all' ? 'b' : 'n'}>{t(`live.scope.${wrapped.scope}`)}</Pill>
                    )}
                    <Button
                        leftSection={<Icon name="refresh" size={14} />}
                        loading={wrapped.refreshing}
                        onClick={() => void wrapped.refetch()}
                        size="xs"
                        variant="default"
                    >
                        {t('live.refresh')}
                    </Button>
                    <Button
                        color="red"
                        disabled={checked.length === 0}
                        onClick={() => setTargets(checked)}
                        size="xs"
                        variant="default"
                    >
                        {t('live.bulk')} {checked.length > 0 && `(${checked.length})`}
                    </Button>
                </div>
            </div>
            <div className={styles.tbl}>
                <table>
                    <thead>
                        <tr>
                            <th style={{ width: 44 }}>
                                <input
                                    aria-label={t('live.selectAll')}
                                    checked={allChecked}
                                    disabled={rows.length === 0}
                                    onChange={() => setSel(allChecked ? new Set() : new Set(rows.map((r) => r.accessor)))}
                                    type="checkbox"
                                />
                            </th>
                            <th>{t('live.created')}</th>
                            <th>{t('live.accessor')}</th>
                            <th>{t('live.creator')}</th>
                            <th>{t('live.path')}</th>
                            <th>{t('live.size')}</th>
                            <th>{t('live.ns')}</th>
                            <th>{t('live.left')}</th>
                            <th aria-label={t('live.actions')} style={{ width: 52 }} />
                        </tr>
                    </thead>
                    <tbody>
                        {rows.length === 0 ? (
                            <tr>
                                <td className={styles.tblEmpty} colSpan={9}>
                                    {wrapped.error
                                        ? t('live.error')
                                        : wrapped.loading
                                          ? t('live.loading')
                                          : t('live.empty')}
                                </td>
                            </tr>
                        ) : (
                            rows.map((r) => {
                                const on = sel.has(r.accessor)
                                const left = Date.parse(r.expires_at) - now
                                return (
                                    <tr
                                        className={cx((on || selected === r.accessor) && styles.sel)}
                                        key={r.accessor}
                                        onClick={() => onSelect(r)}
                                    >
                                        <td>
                                            <input
                                                aria-label={t('live.select')}
                                                checked={on}
                                                onChange={() => toggle(r.accessor)}
                                                onClick={(e) => e.stopPropagation()}
                                                type="checkbox"
                                            />
                                        </td>
                                        <td title={new Date(r.creation_time).toLocaleString(i18n.language)}>
                                            {fmtTime(Date.parse(r.creation_time))}
                                        </td>
                                        <td title={r.accessor}>{short(r.accessor)}</td>
                                        <td title={r.creator_accessor}>
                                            {r.creator_name || r.creator_fingerprint || '—'}
                                            {r.creator_name && r.creator_fingerprint && (
                                                <span className={styles.fp}> {r.creator_fingerprint}</span>
                                            )}
                                        </td>
                                        <td>{r.creation_path || '—'}</td>
                                        <td>{fmtBytes(r.size, t('bytes'))}</td>
                                        <td>{r.namespace}</td>
                                        <td>
                                            <Pill tone={left <= LOW_MS ? 'r' : 'g'}>{clock(left)}</Pill>
                                        </td>
                                        <td>
                                            <button
                                                aria-label={t('live.revokeOne')}
                                                className={styles.rowAct}
                                                onClick={(e) => {
                                                    e.stopPropagation()
                                                    setTargets([r.accessor])
                                                }}
                                                title={t('live.revokeOne')}
                                                type="button"
                                            >
                                                <Icon name="trash" size={14} />
                                            </button>
                                        </td>
                                    </tr>
                                )
                            })
                        )}
                    </tbody>
                </table>
            </div>
            <div className={styles.logNote}>
                {t('live.note', { sec: WRAPPED_POLL / 1000, time: upd })}
            </div>

            <ConfirmModal
                confirmLabel={t('live.bulk')}
                message={targets ? t('live.revokeText', { count: targets.length }) : ''}
                onClose={() => setTargets(null)}
                onConfirm={async () => {
                    if (targets?.length) await revoke(targets)
                }}
                opened={targets !== null}
                title={t('live.revokeTitle')}
            />
        </Plate>
    )
}
