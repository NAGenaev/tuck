// SPDX-License-Identifier: AGPL-3.0-only
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SkeletonLine } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import { CAPS, Parsed } from '../lib/policy-text'
import s from '../policies.module.css'

export interface PolicyRow {
    dirty: boolean
    failed: boolean
    isNew: boolean
    name: string
    /** null while the policy body is loading */
    parsed: null | Parsed
}

type Filter = 'all' | 'changed' | 'deny' | 'errors'
const FILTERS: Filter[] = ['all', 'deny', 'changed', 'errors']

export function PolicyList({
    rows,
    loading,
    selected,
    onSelect
}: {
    loading: boolean
    onSelect: (name: string) => void
    rows: PolicyRow[]
    selected: null | string
}) {
    const { t } = useTranslation('policies')
    const [q, setQ] = useState('')
    const [filter, setFilter] = useState<Filter>('all')

    const visible = useMemo(() => {
        const needle = q.trim().toLowerCase()
        return rows.filter((r) => {
            if (needle && !r.name.toLowerCase().includes(needle)) return false
            if (filter === 'deny') return !!r.parsed?.rules.some((x) => x.caps.includes('deny'))
            if (filter === 'changed') return r.dirty || r.isNew
            if (filter === 'errors') return (r.parsed?.errors ?? 0) > 0
            return true
        })
    }, [rows, q, filter])

    return (
        <Plate className={s.plist}>
            <div className={s.plateTop}>
                <span className={s.ttl}>{t('list.title')}</span>
                <span className={s.sp}>{!loading && t('policiesCount', { count: rows.length })}</span>
            </div>
            <div className={s.tools}>
                <input
                    aria-label={t('list.searchAria')}
                    autoComplete="off"
                    className={s.search}
                    onChange={(e) => setQ(e.currentTarget.value)}
                    placeholder={t('list.search')}
                    type="search"
                    value={q}
                />
                <div className={s.chips}>
                    {FILTERS.map((f) => (
                        <button
                            className={s.chip}
                            data-on={f === filter}
                            key={f}
                            onClick={() => setFilter(f)}
                            type="button"
                        >
                            {t(`filter.${f}`)}
                        </button>
                    ))}
                </div>
            </div>

            {loading ? (
                [0, 1, 2, 3, 4].map((i) => (
                    <div aria-busy className={s.skRow} key={i}>
                        <SkeletonLine height={14} width="55%" />
                        <SkeletonLine width="80%" />
                    </div>
                ))
            ) : rows.length === 0 ? (
                <div className={s.listEmpty}>{t('list.none')}</div>
            ) : visible.length === 0 ? (
                <div className={s.listEmpty}>{t('list.noMatch')}</div>
            ) : (
                visible.map((r, i) => {
                    const used = new Set<string>()
                    r.parsed?.rules.forEach((x) => x.caps.forEach((c) => used.add(c)))
                    const bad = (r.parsed?.errors ?? 0) > 0 || r.failed
                    return (
                        <button
                            className={s.prow}
                            data-on={r.name === selected}
                            key={r.name}
                            onClick={() => onSelect(r.name)}
                            style={{ ['--i' as string]: i }}
                            type="button"
                        >
                            <span className={s.nm}>
                                {r.name}
                            </span>
                            <span>
                                {bad ? (
                                    <Pill size="sm" tone="r">
                                        {t('pill.error')}
                                    </Pill>
                                ) : r.isNew ? (
                                    <Pill size="sm" tone="b">
                                        {t('pill.new')}
                                    </Pill>
                                ) : r.dirty ? (
                                    <Pill size="sm" tone="b">
                                        {t('pill.changed')}
                                    </Pill>
                                ) : null}
                            </span>
                            <span className={s.st}>
                                {r.parsed ? (
                                    <>
                                        <span aria-label={CAPS.join(' · ')} className={s.bars} title={CAPS.join(' · ')}>
                                            {CAPS.map((c) => (
                                                <i
                                                    data-deny={c === 'deny' && used.has(c)}
                                                    key={c}
                                                    style={{ height: used.has(c) ? 14 : 4 }}
                                                />
                                            ))}
                                        </span>
                                        <span>
                                            <b>{t('rules', { count: r.parsed.rules.length })}</b>
                                        </span>
                                    </>
                                ) : (
                                    <SkeletonLine width={120} />
                                )}
                            </span>
                        </button>
                    )
                })
            )}
            {!loading && (
                <div className={s.legend}>
                    {rows.length > 0 && <div>{t('list.shown', { shown: visible.length, total: rows.length })}</div>}
                    <div>{t('list.rootNote')}</div>
                </div>
            )}
        </Plate>
    )
}
