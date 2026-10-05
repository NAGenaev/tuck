// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CSSProperties, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Drawer, Icon, MetaGrid, SkeletonLine, StateBlock, toast } from '@shared/ui/kit'
import { Pill } from '@shared/ui/tuck/tuck'

import { apiError, httpStatus, lookupSelf, renewSelf } from './tokens.api'
import s from './tokens.module.css'
import { barColor, fmtDateTime, fmtLeft, fmtTotal, leftSeconds, policyTone, toRow } from './tokens.utils'

const R = 40
const C = 2 * Math.PI * R

export function SelfDrawer({ opened, onClose }: { onClose: () => void; opened: boolean }) {
    const { t, i18n } = useTranslation('tokens')
    const qc = useQueryClient()
    const [now, setNow] = useState(() => Date.now())

    const q = useQuery({
        enabled: opened,
        queryFn: lookupSelf,
        queryKey: ['tokens', 'self'],
        refetchOnWindowFocus: false,
        retry: false,
        staleTime: 0
    })

    useEffect(() => {
        if (!opened) return
        setNow(Date.now())
        const id = window.setInterval(() => setNow(Date.now()), 1000)
        return () => window.clearInterval(id)
    }, [opened])

    const row = q.data ? toRow(q.data.id, q.data, false) : null
    const [d, h, m] = [t('time.d'), t('time.h'), t('time.m')]

    const renew = useMutation({
        mutationFn: () => renewSelf(row?.total || undefined),
        onError: (e) => toast.error(t('self.renewFailed'), apiError(e)),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: ['tokens', 'self'] })
            toast.success(t('self.renewed'), t('self.renewedText', { total: fmtTotal(row?.total ?? 0, d, h, m) }))
        }
    })

    const never = !!row && row.expiresAt === null
    const left = row ? leftSeconds(row, now) : 0
    const frac = !row || never || !row.total ? 1 : Math.min(1, left / row.total)
    const stagger = (i: number) => ({ '--i': i }) as CSSProperties

    let body
    if (q.isPending) {
        body = (
            <div className={s.selfBody}>
                <SkeletonLine width="100%" />
                <SkeletonLine width="70%" />
                <SkeletonLine width="85%" />
            </div>
        )
    } else if (q.isError || !row) {
        body = (
            <StateBlock
                compact
                kind={httpStatus(q.error) === 403 ? 'forbidden' : 'error'}
                message={t('self.loadFailed')}
                onRetry={() => void q.refetch()}
                retrying={q.isFetching}
            />
        )
    } else {
        const data = q.data!
        const yes = t('self.yes')
        const no = t('self.no')
        body = (
            <div className={`${s.selfBody} ${s.stagger}`}>
                <div className={s.sessionRing} style={stagger(0)}>
                    <svg aria-hidden viewBox="0 0 92 92">
                        <circle cx="46" cy="46" fill="none" r={R} stroke="var(--line2)" strokeWidth="6" />
                        <circle
                            cx="46"
                            cy="46"
                            fill="none"
                            r={R}
                            stroke={never ? 'var(--ice)' : barColor(frac)}
                            strokeDasharray={never ? '4 6' : C}
                            strokeDashoffset={never ? 0 : C * (1 - frac)}
                            strokeLinecap="butt"
                            strokeWidth="6"
                            style={{ transition: 'stroke-dashoffset 1s linear' }}
                            transform="rotate(-90 46 46)"
                        />
                    </svg>
                    <div className={s.sessionT}>
                        <small>{t('self.left')}</small>
                        {never ? t('self.never') : fmtLeft(left, d, h, m)}
                        {!never && row.total > 0 && (
                            <small style={{ marginTop: 4, marginBottom: 0 }}>
                                {t('self.ttlOf', { total: fmtTotal(row.total, d, h, m) })}
                            </small>
                        )}
                    </div>
                </div>

                <div style={stagger(1)}>
                    <MetaGrid
                        items={[
                            ['name', row.name || '—'],
                            ['accessor', row.accessor || '—'],
                            ['namespace', row.ns],
                            [t('self.created'), row.createdAt ? fmtDateTime(row.createdAt, i18n.language) : '—'],
                            [t('self.expires'), row.expiresAt ? fmtDateTime(row.expiresAt, i18n.language) : t('self.never')],
                            ['renewable', row.renewable ? yes : no],
                            ['uses', `${row.uses} / ${row.maxUses || '∞'}`],
                            ['orphan', data.orphan ? yes : no]
                        ]}
                    />
                </div>

                <div className={s.pills} style={stagger(2)}>
                    {row.policies.map((p) => (
                        <Pill key={p} size="sm" tone={policyTone(p)} variant="soft">
                            {p}
                        </Pill>
                    ))}
                </div>

                <p className={s.hint} style={{ ...stagger(3), margin: 0 }}>
                    {t('self.hint')}
                </p>
            </div>
        )
    }

    return (
        <Drawer
            busy={renew.isPending}
            footer={
                <>
                    <Button onClick={onClose} variant="default">
                        {t('self.close')}
                    </Button>
                    <Button
                        disabled={!row || !row.renewable}
                        leftSection={<Icon name="refresh" size={14} />}
                        loading={renew.isPending}
                        onClick={() => renew.mutate()}
                    >
                        {t('self.renew')}
                    </Button>
                </>
            }
            icon="user"
            onClose={onClose}
            opened={opened}
            title={t('self.title')}
        >
            {body}
        </Drawer>
    )
}
