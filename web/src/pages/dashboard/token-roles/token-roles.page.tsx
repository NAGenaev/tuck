// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ReactNode, useEffect, useMemo, useState } from 'react'

import { createTokenFromRole, deleteTokenRole } from '@shared/api/endpoints/token-roles'
import { Token } from '@shared/api/endpoints/token'
import { ConfirmModal, CountUp, cx, Icon, IconName, Modal, Reveal, toast } from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Pill, Plate } from '@shared/ui/tuck/tuck'

import { Dial } from './components/dial'
import { DrawerMode, RoleDrawer } from './components/role-drawer'
import { errorMessage, useRoles } from './hooks'
import { fmtDate, fmtDur, fmtShort, RoleRow, RoleView, Tr, useTr } from './lib'
import styles from './token-roles.module.css'

type Filter = 'all' | 'limited' | 'period' | 'renew'
type View = 'cards' | 'table'

const VIEW_KEY = 'tuck.tokenRoles.view'

function Glyph({ kind }: { kind: 'empty' | 'error' | 'lock' }) {
    const c = kind === 'lock' ? 'var(--alarm)' : 'var(--brass)'
    return (
        <svg className={cx(styles.glyph, kind === 'lock' && styles.lockring)} viewBox="0 0 96 96">
            <rect fill="none" height="84" stroke="var(--line2)" width="84" x="6" y="6" />
            <circle cx="48" cy="48" fill="none" r="38" stroke="var(--line2)" strokeDasharray="3 6" />
            {kind === 'lock' ? (
                <>
                    <rect fill="var(--vault2)" height="22" stroke={c} strokeWidth="2" width="28" x="34" y="46" />
                    <path d="M40 46V38a8 8 0 0116 0v8" fill="none" stroke={c} strokeWidth="2" />
                    <path d="M48 54v8" stroke={c} strokeWidth="2" />
                </>
            ) : (
                <>
                    <circle cx="48" cy="48" fill="none" r="9" stroke={c} strokeWidth="2" />
                    <path d="M48 14v18M48 64v18M14 48h18M64 48h18" stroke={c} strokeWidth="2" />
                </>
            )}
        </svg>
    )
}

function Panel({
    kind,
    pill,
    title,
    children,
    actions
}: {
    actions?: ReactNode
    children?: ReactNode
    kind: 'empty' | 'error' | 'lock'
    pill?: string
    title: string
}) {
    return (
        <Plate variant={kind === 'empty' ? 'default' : 'danger'}>
            <div className={styles.panel}>
                <Glyph kind={kind} />
                {pill && <Pill tone="bad">{pill}</Pill>}
                <h3 className={cx(kind !== 'empty' && styles.panelErr)}>{title}</h3>
                {children && <p>{children}</p>}
                {actions && <div className={styles.panelActions}>{actions}</div>}
            </div>
        </Plate>
    )
}

function Pols({ list }: { list: string[] }) {
    if (!list.length) return <span className={styles.none}>—</span>
    return (
        <>
            {list.map((p) => (
                <span className={styles.pol} key={p}>
                    {p}
                </span>
            ))}
        </>
    )
}

function Flags({ role, tr }: { role: RoleView; tr: Tr }) {
    return (
        <>
            <Pill tone={role.renewable ? 'ok' : 'mute'}>{role.renewable ? 'renew' : 'no-renew'}</Pill>{' '}
            {role.maxUses > 0 && <Pill tone="brass" title={tr('maxUses', 'Макс. использований')}>×{role.maxUses}</Pill>}
        </>
    )
}

function Life({ role, tr }: { role: RoleView; tr: Tr }) {
    if (role.period) return <Pill tone="ok">{tr('period', 'период')} {fmtDur(tr, role.period)}</Pill>
    return (
        <span className={styles.ttl}>
            <Dial role={role} />
            <span>{role.ttl ? fmtDur(tr, role.ttl) : tr('forever', 'бессрочно')}</span>
        </span>
    )
}

export function TokenRolesPage() {
    const { lang, tr } = useTr()
    const queryClient = useQueryClient()
    const { rows, forbidden, isError, isLoading, list } = useRoles()

    const [q, setQ] = useState('')
    const [filter, setFilter] = useState<Filter>('all')
    const [view, setView] = useState<View>(() => {
        try {
            return localStorage.getItem(VIEW_KEY) === 'cards' ? 'cards' : 'table'
        } catch {
            return 'table'
        }
    })
    const [selected, setSelected] = useState<Set<string>>(new Set())
    const [drawerOpen, setDrawerOpen] = useState(false)
    const [drawer, setDrawer] = useState<{ mode: DrawerMode; role: null | RoleView }>({ mode: 'new', role: null })
    const [delOpen, setDelOpen] = useState(false)
    const [delNames, setDelNames] = useState<string[]>([])
    const [minted, setMinted] = useState<null | { role: RoleView | undefined; token: Token }>(null)

    useEffect(() => {
        try {
            localStorage.setItem(VIEW_KEY, view)
        } catch {
            /* storage may be unavailable */
        }
    }, [view])

    // forget selections of roles that no longer exist
    useEffect(() => {
        if (!list.data) return
        setSelected((prev) => {
            const next = new Set([...prev].filter((n) => list.data?.includes(n)))
            return next.size === prev.size ? prev : next
        })
    }, [list.data])

    const mintMutation = useMutation({
        mutationFn: (name: string) => createTokenFromRole(name),
        onError: (e) => toast.error(tr('mintFailed', 'Не удалось выпустить токен'), errorMessage(e)),
        onSuccess: (token, name) => setMinted({ role: rows.find((r) => r.name === name)?.role, token })
    })

    const loaded = rows.filter((r) => r.role).map((r) => r.role as RoleView)
    const stats = useMemo(() => {
        const lifetimes = loaded.map((r) => r.period || r.ttl).filter(Boolean)
        const avg = lifetimes.length ? lifetimes.reduce((a, b) => a + b, 0) / lifetimes.length / 3600 : 0
        return {
            avg,
            limited: loaded.filter((r) => r.maxUses > 0).length,
            period: loaded.filter((r) => r.period).length,
            renew: loaded.filter((r) => r.renewable).length,
            total: rows.length
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [rows])

    const visible = rows.filter((r) => {
        const role = r.role
        if (filter !== 'all' && !role) return false
        if (filter === 'renew' && !role?.renewable) return false
        if (filter === 'period' && !role?.period) return false
        if (filter === 'limited' && !role?.maxUses) return false
        const needle = q.trim().toLowerCase()
        if (needle && !`${r.name} ${role?.policies.join(' ') ?? ''}`.toLowerCase().includes(needle)) return false
        return true
    })

    const failedAll = forbidden || isError
    const canCreate = !isLoading && !failedAll

    const openDrawer = (mode: DrawerMode, role: null | RoleView) => {
        setDrawer({ mode, role })
        setDrawerOpen(true)
    }
    const askDelete = (names: string[]) => {
        setDelNames(names)
        setDelOpen(true)
    }
    const toggle = (name: string, on: boolean) =>
        setSelected((prev) => {
            const next = new Set(prev)
            if (on) next.add(name)
            else next.delete(name)
            return next
        })
    const allVisibleSelected = visible.length > 0 && visible.every((r) => selected.has(r.name))
    const toggleAll = (on: boolean) =>
        setSelected((prev) => {
            const next = new Set(prev)
            visible.forEach((r) => (on ? next.add(r.name) : next.delete(r.name)))
            return next
        })

    const doDelete = async () => {
        const names = delNames
        const results = await Promise.allSettled(names.map((n) => deleteTokenRole(n)))
        const failed = results.filter((r) => r.status === 'rejected')
        await queryClient.invalidateQueries({ queryKey: ['token-roles'] })
        setSelected((prev) => {
            const next = new Set(prev)
            names.forEach((n, i) => results[i].status === 'fulfilled' && next.delete(n))
            return next
        })
        if (failed.length) {
            toast.error(
                tr('delFailed', 'Не удалось удалить'),
                errorMessage((failed[0] as PromiseRejectedResult).reason)
            )
            if (failed.length === names.length) throw new Error('delete failed')
        } else {
            toast.success(
                tr('deleted', 'Удалено'),
                names.length > 1
                    ? tr('deletedMany', 'Удалено ролей: {{n}}', { n: names.length })
                    : tr('deletedOne', 'Роль {{name}} удалена', { name: names[0] })
            )
        }
    }

    const actions = (r: RoleRow) => (
        <>
            <button
                aria-label={tr('actMint', 'Выпустить токен')}
                className={styles.ib}
                data-tip={tr('actMint', 'Выпустить токен')}
                disabled={mintMutation.isPending}
                onClick={() => mintMutation.mutate(r.name)}
                type="button"
            >
                <Icon name={mintMutation.isPending && mintMutation.variables === r.name ? 'refresh' : 'user'} size={16} />
            </button>
            <button
                aria-label={tr('actEdit', 'Изменить')}
                className={styles.ib}
                data-tip={tr('actEdit', 'Изменить')}
                disabled={!r.role}
                onClick={() => openDrawer('edit', r.role ?? null)}
                type="button"
            >
                <Icon name="gear" size={16} />
            </button>
            <button
                aria-label={tr('actDup', 'Дублировать')}
                className={styles.ib}
                data-tip={tr('actDup', 'Дублировать')}
                disabled={!r.role}
                onClick={() => openDrawer('dup', r.role ?? null)}
                type="button"
            >
                <Icon name="copy" size={16} />
            </button>
            <button
                aria-label={tr('actDel', 'Удалить')}
                className={cx(styles.ib, styles.red)}
                data-tip={tr('actDel', 'Удалить')}
                onClick={() => askDelete([r.name])}
                type="button"
            >
                <Icon name="trash" size={16} />
            </button>
        </>
    )

    const updated = (r: RoleRow) =>
        r.role?.updatedAt ? tr('updated', 'обновлена {{d}}', { d: fmtDate(r.role.updatedAt, lang) }) : ''

    const body = () => {
        if (isLoading)
            return (
                <div aria-busy className={styles.skelwrap}>
                    {Array.from({ length: 6 }, (_, i) => (
                        <div className={styles.skel} key={i}>
                            <div style={{ left: 16, width: 16 }} />
                            <div style={{ left: 56, width: 110 + i * 8 }} />
                            <div style={{ left: 240, width: 160 }} />
                            <div style={{ left: 440, width: 90 }} />
                            <div style={{ right: 20, width: 100 }} />
                        </div>
                    ))}
                </div>
            )
        if (forbidden)
            return (
                <Panel
                    actions={
                        <Button onClick={() => void list.refetch()}>{tr('retry', 'Повторить')}</Button>
                    }
                    kind="lock"
                    pill="403 · permission denied"
                    title={tr('forbiddenTitle', 'Нет доступа к ролям')}
                >
                    {tr('forbiddenText', 'Вашему токену не хватает права list на путь auth/token/roles/. Попросите администратора добавить политику с этим путём.')}
                </Panel>
            )
        if (isError)
            return (
                <Panel
                    actions={
                        <Button leftSection={<Icon name="refresh" size={15} />} loading={list.isFetching} onClick={() => void list.refetch()}>
                            {tr('retry', 'Повторить')}
                        </Button>
                    }
                    kind="error"
                    pill={tr('errPill', 'ошибка')}
                    title={tr('errTitle', 'Не удалось загрузить роли')}
                >
                    {errorMessage(list.error) ?? tr('errText', 'Сервер не ответил. Проверьте, что хранилище распечатано, и повторите запрос.')}
                </Panel>
            )
        if (!rows.length)
            return (
                <Panel
                    actions={
                        <Button leftSection={<Icon name="plus" size={15} />} onClick={() => openDrawer('new', null)}>
                            {tr('createFirst', 'Создать первую роль')}
                        </Button>
                    }
                    kind="empty"
                    title={tr('emptyTitle', 'Нет ролей')}
                >
                    {tr('emptyText', 'Роль задаёт шаблон выпуска токенов: политики, срок жизни и лимиты. Создайте первую — сервисы получат токены без права менять свои свойства.')}
                </Panel>
            )
        if (!visible.length)
            return (
                <Panel
                    actions={
                        <Button
                            onClick={() => {
                                setQ('')
                                setFilter('all')
                            }}
                            variant="default"
                        >
                            {tr('resetFilters', 'Сбросить фильтры')}
                        </Button>
                    }
                    kind="empty"
                    title={tr('noMatch', 'Ничего не найдено')}
                >
                    {tr('noMatchText', 'Нет ролей, подходящих под запрос и выбранный фильтр.')}
                </Panel>
            )
        if (view === 'cards')
            return (
                <div className={styles.grid}>
                    {visible.map((r, i) => (
                        <article className={cx(styles.rc, selected.has(r.name) && styles.sel)} key={r.name} style={{ ['--r' as string]: i }}>
                            <div className={styles.rcTop}>
                                {r.role ? <Dial big role={r.role} /> : <span className={styles.dialGhost} />}
                                <div style={{ minWidth: 0 }}>
                                    <h3>{r.name}</h3>
                                    <div>{r.role && <Flags role={r.role} tr={tr} />}</div>
                                </div>
                            </div>
                            {r.role ? (
                                <dl>
                                    <dt>{tr('colPolicies', 'Политики')}</dt>
                                    <dd>
                                        <Pols list={r.role.policies} />
                                    </dd>
                                    <dt>{tr('colLife', 'Срок')}</dt>
                                    <dd>
                                        {r.role.period
                                            ? `${tr('period', 'период')} ${fmtDur(tr, r.role.period)}`
                                            : `TTL ${r.role.ttl ? fmtDur(tr, r.role.ttl) : tr('forever', 'бессрочно')}`}
                                        {' · '}
                                        {tr('colMax', 'макс.')} {r.role.maxTtl ? fmtDur(tr, r.role.maxTtl) : tr('none', 'нет')}
                                    </dd>
                                    {updated(r) && (
                                        <>
                                            <dt>{tr('colUpdated', 'Изм.')}</dt>
                                            <dd>{updated(r)}</dd>
                                        </>
                                    )}
                                </dl>
                            ) : (
                                <span className={styles.none}>{r.failed ? tr('detailsFailed', 'не удалось загрузить параметры роли') : '…'}</span>
                            )}
                            <div className={styles.rcFoot}>
                                <input
                                    aria-label={tr('select', 'Выбрать {{name}}', { name: r.name })}
                                    checked={selected.has(r.name)}
                                    className={styles.ck}
                                    onChange={(e) => toggle(r.name, e.currentTarget.checked)}
                                    style={{ marginRight: 'auto' }}
                                    type="checkbox"
                                />
                                {actions(r)}
                            </div>
                        </article>
                    ))}
                </div>
            )
        return (
            <div className={styles.tbl}>
                <table>
                    <thead>
                        <tr>
                            <th style={{ width: 36 }}>
                                <input
                                    aria-label={tr('selectAll', 'Выбрать все')}
                                    checked={allVisibleSelected}
                                    className={styles.ck}
                                    onChange={(e) => toggleAll(e.currentTarget.checked)}
                                    type="checkbox"
                                />
                            </th>
                            <th>{tr('colRole', 'Роль')}</th>
                            <th>{tr('colPolicies', 'Политики')}</th>
                            <th>{tr('colLife', 'Срок')}</th>
                            <th>{tr('colMaxTtl', 'Макс. TTL')}</th>
                            <th>{tr('colFlags', 'Флаги')}</th>
                            <th style={{ width: 180 }} />
                        </tr>
                    </thead>
                    <tbody>
                        {visible.map((r, i) => (
                            <tr className={cx(styles.in, selected.has(r.name) && styles.sel)} key={r.name} style={{ ['--r' as string]: i }}>
                                <td>
                                    <input
                                        aria-label={tr('select', 'Выбрать {{name}}', { name: r.name })}
                                        checked={selected.has(r.name)}
                                        className={styles.ck}
                                        onChange={(e) => toggle(r.name, e.currentTarget.checked)}
                                        type="checkbox"
                                    />
                                </td>
                                <td className={styles.nm}>
                                    {r.name}
                                    {updated(r) && <small>{updated(r)}</small>}
                                </td>
                                {r.role ? (
                                    <>
                                        <td>
                                            <Pols list={r.role.policies} />
                                        </td>
                                        <td>
                                            <Life role={r.role} tr={tr} />
                                        </td>
                                        <td>{r.role.maxTtl ? fmtDur(tr, r.role.maxTtl) : <span className={styles.none}>—</span>}</td>
                                        <td>
                                            <Flags role={r.role} tr={tr} />
                                        </td>
                                    </>
                                ) : (
                                    <td colSpan={4}>
                                        <span className={styles.none}>
                                            {r.failed ? tr('detailsFailed', 'не удалось загрузить параметры роли') : '…'}
                                        </span>
                                    </td>
                                )}
                                <td className={styles.act}>{actions(r)}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        )
    }

    const titleWords = tr('title', 'Роли токенов').split(' ')
    const lastWord = titleWords.pop()

    const chips: Array<[Filter, string, number]> = [
        ['all', tr('fAll', 'Все'), stats.total],
        ['renew', tr('fRenew', 'Продлеваемые'), stats.renew],
        ['period', tr('fPeriod', 'С периодом'), stats.period],
        ['limited', tr('fLimited', 'С лимитом'), stats.limited]
    ]

    const dash = failedAll || isLoading
    const metric = (label: string, value: ReactNode, on: number, total: number, unit?: string) => (
        <div className={styles.metric}>
            <small>{label}</small>
            <strong>
                {dash ? '—' : value}
                {unit && !dash && <sub>{unit}</sub>}
            </strong>
            <div className={styles.bar}>
                {Array.from({ length: Math.max(total, 1) }, (_, i) => (
                    <i className={i < on && !dash ? styles.barOn : undefined} key={i} />
                ))}
            </div>
        </div>
    )

    return (
        <Page title="Token Roles">
            <div className={styles.root}>
                <div className={styles.head}>
                    <div>
                        <Eyebrow className={styles.rv}>
                            {tr('eyebrow', 'Доступ')} · /v1/auth/token/roles
                        </Eyebrow>
                        <h1 className={styles.rv} style={{ ['--i' as string]: 1 }}>
                            {titleWords.join(' ')} <em>{lastWord}</em>
                        </h1>
                        <p className={styles.rv} style={{ ['--i' as string]: 2 }}>
                            {tr('lead1', 'Шаблон выпуска: роль фиксирует политики, срок жизни и лимиты,')}{' '}
                            <b>{tr('leadB', 'а токен получает их без права изменить')}</b>.{' '}
                            {tr('lead2', 'Сервис просит не «токен на всё», а токен по роли.')}
                        </p>
                    </div>
                    <div className={styles.rv} style={{ ['--i' as string]: 3 }}>
                        <Button disabled={!canCreate} leftSection={<Icon name="plus" size={15} />} onClick={() => openDrawer('new', null)}>
                            {tr('newRole', 'Новая роль')}
                        </Button>
                    </div>
                </div>

                <div className={cx(styles.metrics, styles.rv)} style={{ ['--i' as string]: 4 }}>
                    {metric(tr('mTotal', 'Всего ролей'), <CountUp value={stats.total} />, stats.total, Math.max(stats.total, 6))}
                    {metric(tr('mRenew', 'Продлеваемые'), <CountUp value={stats.renew} />, stats.renew, stats.total)}
                    {metric(tr('mPeriod', 'С периодом'), <CountUp value={stats.period} />, stats.period, stats.total)}
                    {metric(
                        tr('mAvg', 'Средний срок'),
                        <CountUp decimals={1} value={stats.avg} />,
                        Math.min(6, Math.round(stats.avg / 4)),
                        6,
                        tr('u.hShort', 'ч')
                    )}
                </div>

                {!failedAll && !isLoading && rows.length > 0 && (
                    <div className={cx(styles.toolbar, styles.rv)} style={{ ['--i' as string]: 5 }}>
                        <div className={styles.search}>
                            <Icon name="search" size={15} />
                            <input
                                aria-label={tr('search', 'Поиск по имени или политике')}
                                className={styles.inp}
                                onChange={(e) => setQ(e.currentTarget.value)}
                                placeholder={tr('search', 'Поиск по имени или политике')}
                                value={q}
                            />
                        </div>
                        <div className={styles.chips}>
                            {chips.map(([k, label, n]) => (
                                <button className={cx(styles.chip, filter === k && styles.chipOn)} key={k} onClick={() => setFilter(k)} type="button">
                                    {label}
                                    <b>{n}</b>
                                </button>
                            ))}
                        </div>
                        <div className={styles.spacer} />
                        <div className={styles.seg}>
                            {(['table', 'cards'] as const).map((v) => (
                                <button className={cx(view === v && styles.segOn)} key={v} onClick={() => setView(v)} type="button">
                                    {v === 'table' ? tr('viewTable', 'Таблица') : tr('viewCards', 'Карточки')}
                                </button>
                            ))}
                        </div>
                    </div>
                )}

                <div className={styles.rv} style={{ ['--i' as string]: 6 }}>
                    {body()}
                </div>
            </div>

            <div className={cx(styles.bulk, selected.size > 0 && styles.bulkShow)} role="region">
                <b>{tr('selected', 'Выбрано: {{n}}', { n: selected.size })}</b>
                <Button onClick={() => setSelected(new Set())} size="xs" variant="default">
                    {tr('clearSel', 'Снять')}
                </Button>
                <Button color="red" leftSection={<Icon name="trash" size={14} />} onClick={() => askDelete([...selected])} size="xs">
                    {tr('actDel', 'Удалить')}
                </Button>
            </div>

            <RoleDrawer
                existing={rows.map((r) => r.name)}
                mode={drawer.mode}
                onClose={() => setDrawerOpen(false)}
                opened={drawerOpen}
                role={drawer.role}
            />

            <ConfirmModal
                confirmText={delNames.length > 1 ? tr('delWord', 'удалить') : delNames[0]}
                message={
                    <>
                        <p style={{ margin: '0 0 12px' }}>
                            {delNames.length > 1
                                ? tr('delMany', 'Будут удалены роли: {{names}}.', { names: delNames.join(', ') })
                                : tr('delOne', 'Это удалит роль «{{name}}».', { name: delNames[0] })}
                        </p>
                        <div className={styles.callout}>
                            <Icon name="info" size={15} />
                            <span>
                                {tr('delWarn', 'Уже выпущенные токены продолжат действовать до истечения срока. Новые выпустить по роли будет нельзя.')}
                            </span>
                        </div>
                    </>
                }
                onClose={() => setDelOpen(false)}
                onConfirm={doDelete}
                opened={delOpen}
                title={
                    delNames.length > 1
                        ? tr('delTitleMany', 'Удалить роли: {{n}}?', { n: delNames.length })
                        : tr('delTitle', 'Удалить роль?')
                }
            />

            <Modal
                footer={<Button onClick={() => setMinted(null)}>{tr('done', 'Готово')}</Button>}
                icon={'key' satisfies IconName}
                onClose={() => setMinted(null)}
                opened={!!minted}
                title={tr('mintTitle', 'Токен выпущен')}
            >
                {minted && (
                    <div className={styles.mint}>
                        <p>
                            {tr('mintFor', 'Роль')} <b>{minted.token.display_name ?? ''}</b>
                            {minted.role && (
                                <>
                                    {' · '}
                                    {minted.role.period
                                        ? `${tr('period', 'период')} ${fmtDur(tr, minted.role.period)}`
                                        : `TTL ${minted.role.ttl ? fmtDur(tr, minted.role.ttl) : tr('forever', 'бессрочно')}`}
                                </>
                            )}
                        </p>
                        <Reveal autoMask={60} value={minted.token.id} />
                        <div className={cx(styles.callout, styles.warn)}>
                            <Icon name="info" size={15} />
                            <span>{tr('copyNow', 'Скопируйте токен сейчас — повторно он не будет показан.')}</span>
                        </div>
                        <div>
                            <div className={styles.carryLabel}>{tr('carries', 'Токен получил политики')}</div>
                            <div className={styles.carry}>
                                {(minted.token.policies ?? []).length ? (
                                    (minted.token.policies ?? []).map((p, i) => (
                                        <span className={styles.pol} key={p} style={{ animationDelay: `${i * 80 + 150}ms` }}>
                                            {p}
                                        </span>
                                    ))
                                ) : (
                                    <span className={styles.none}>—</span>
                                )}
                            </div>
                        </div>
                        {minted.token.ttl ? (
                            <div className={styles.none}>
                                TTL: {fmtDur(tr, Math.round(minted.token.ttl / 1e9))}
                                {minted.token.expires_at ? ` · ${fmtDate(minted.token.expires_at, lang)}` : ''}
                            </div>
                        ) : null}
                    </div>
                )}
            </Modal>
        </Page>
    )
}

export { fmtShort }
