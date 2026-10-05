// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ReactNode, useEffect, useMemo, useState } from 'react'

import { putTokenRole, TokenRolePut } from '@shared/api/endpoints/token-roles'
import { listPolicies } from '@shared/api/endpoints/policy'
import { cx, Drawer, Icon, IconName, toast } from '@shared/ui/kit'
import { Plate, PlateBar } from '@shared/ui/tuck/tuck'

import { errorMessage } from '../hooks'
import { fmtDur, fmtShort, NAME_RE, parseDur, POLICY_RE, RoleView, toGoDuration, useTr } from '../lib'
import { LifetimeRing } from './lifetime-ring'
import styles from './role-drawer.module.css'
import { TagInput } from './tag-input'

export type DrawerMode = 'dup' | 'edit' | 'new'

interface Form {
    max: string
    name: string
    period: string
    policies: string[]
    renew: boolean
    ttl: string
    uses: string
}

type Errors = Partial<Record<'max' | 'name' | 'period' | 'policies' | 'ttl' | 'uses', string>>

const EMPTY: Form = { max: '24h', name: '', period: '', policies: [], renew: true, ttl: '1h', uses: '' }

function seedForm(mode: DrawerMode, role: null | RoleView): Form {
    if (!role) return EMPTY
    return {
        max: fmtShort(role.maxTtl),
        name: mode === 'dup' ? `${role.name}-copy` : role.name,
        period: fmtShort(role.period),
        policies: [...role.policies],
        renew: role.renewable,
        ttl: fmtShort(role.ttl),
        uses: role.maxUses ? String(role.maxUses) : ''
    }
}

function Group({
    icon,
    title,
    hint,
    children
}: {
    children: ReactNode
    hint?: ReactNode
    icon: IconName
    title: string
}) {
    const [closed, setClosed] = useState(false)
    return (
        <section className={cx(styles.grp, closed && styles.grpClosed)}>
            <header onClick={() => setClosed((c) => !c)}>
                <span className={styles.gico}>
                    <Icon name={icon} size={15} />
                </span>
                <h4>{title}</h4>
                <span className={styles.ghint}>
                    {hint}
                    <Icon className={styles.chev} name="down" size={12} />
                </span>
            </header>
            {!closed && <div className={styles.gb}>{children}</div>}
        </section>
    )
}

function Fld({
    id,
    label,
    code,
    error,
    help,
    children
}: {
    children: ReactNode
    code?: string
    error?: string
    help?: ReactNode
    id: string
    label: string
}) {
    return (
        <div className={cx(styles.fld, error && styles.fldBad)}>
            <label htmlFor={id}>
                {label}
                {code && <i>{code}</i>}
            </label>
            {children}
            {error && (
                <div className={styles.err} key={error} role="alert">
                    <Icon name="alert" size={13} />
                    {error}
                </div>
            )}
            {help && <div className={styles.help}>{help}</div>}
        </div>
    )
}

function Json({ body }: { body: TokenRolePut }) {
    const lines: Array<[string, ReactNode]> = []
    const str = (v: string) => <span className={styles.js}>"{v}"</span>
    const key = (k: string) => <span className={styles.jk}>"{k}"</span>
    const entries: Array<[string, ReactNode, string]> = [
        [
            'policies',
            body.policies?.length ? (
                <>
                    [{body.policies.map((p, i) => <span key={p}>{i > 0 && ', '}{str(p)}</span>)}]
                </>
            ) : (
                '[]'
            ),
            (body.policies ?? []).join()
        ]
    ]
    if (body.ttl) entries.push(['ttl', str(body.ttl), body.ttl])
    if (body.max_ttl) entries.push(['max_ttl', str(body.max_ttl), body.max_ttl])
    if (body.max_uses) entries.push(['max_uses', <span className={styles.jn}>{body.max_uses}</span>, String(body.max_uses)])
    if (body.period) entries.push(['period', str(body.period), body.period])
    entries.push(['renewable', <span className={styles.jb}>{String(!!body.renewable)}</span>, String(!!body.renewable)])
    entries.forEach(([k, v, sig], i) => {
        lines.push([
            `${k}:${sig}`,
            <>
                {'  '}
                {key(k)}: {v}
                {i < entries.length - 1 ? ',' : ''}
            </>
        ])
    })
    return (
        <pre className={styles.json}>
            <span>{'{'}</span>
            {'\n'}
            {lines.map(([sig, node]) => (
                <span className={styles.flash} key={sig}>
                    {node}
                    {'\n'}
                </span>
            ))}
            <span>{'}'}</span>
        </pre>
    )
}

export function RoleDrawer({
    opened,
    mode,
    role,
    existing,
    onClose
}: {
    existing: string[]
    mode: DrawerMode
    onClose: () => void
    opened: boolean
    role: null | RoleView
}) {
    const { tr } = useTr()
    const queryClient = useQueryClient()
    const [form, setForm] = useState<Form>(EMPTY)
    const [snap, setSnap] = useState('')
    const [touched, setTouched] = useState<Record<string, boolean>>({})
    const [attempted, setAttempted] = useState(false)
    const [armed, setArmed] = useState(false)
    const [pane, setPane] = useState<'form' | 'prev'>('form')

    useEffect(() => {
        if (!opened) return
        const f = seedForm(mode, role)
        setForm(f)
        setSnap(mode === 'dup' ? '' : JSON.stringify(f))
        setTouched({})
        setAttempted(false)
        setArmed(false)
        setPane('form')
    }, [opened, mode, role])

    const { data: policyNames } = useQuery({
        enabled: opened,
        queryFn: listPolicies,
        queryKey: ['policies', 'list'],
        retry: false,
        staleTime: 60_000
    })

    const set = <K extends keyof Form>(k: K, v: Form[K]) => {
        setForm((f) => ({ ...f, [k]: v }))
        setArmed(false)
    }
    const touch = (k: string) => setTouched((t) => ({ ...t, [k]: true }))

    const ttl = parseDur(form.ttl)
    const max = parseDur(form.max)
    const period = parseDur(form.period)

    const errors = useMemo<Errors>(() => {
        const e: Errors = {}
        const n = form.name.trim()
        if (!n) e.name = tr('e.nameEmpty', 'Укажите имя роли.')
        else if (!NAME_RE.test(n))
            e.name = tr('e.nameFormat', 'Только строчные латинские буквы, цифры, дефис и «_»: payments-api.')
        else if (mode !== 'edit' && existing.includes(n))
            e.name = tr('e.nameTaken', 'Роль «{{n}}» уже существует. Выберите другое имя или откройте её на изменение.', { n })
        if (!form.policies.length)
            e.policies = tr('e.policiesEmpty', 'Добавьте хотя бы одну политику, иначе токен получит только default.')
        else {
            const bad = form.policies.find((p) => !POLICY_RE.test(p))
            if (bad) e.policies = tr('e.policyBad', 'Недопустимое имя политики: {{p}}.', { p: bad })
        }
        const fmt = tr('e.durFormat', 'Формат: число и единица — 30s, 15m, 1h, 7d.')
        if (Number.isNaN(ttl)) e.ttl = fmt
        if (Number.isNaN(max)) e.max = fmt
        if (Number.isNaN(period)) e.period = fmt
        if (!e.ttl && !e.max && ttl && max && ttl > max)
            e.ttl = tr('e.ttlOverMax', 'TTL ({{a}}) больше явного максимума ({{b}}). Уменьшите TTL или увеличьте максимум.', {
                a: fmtDur(tr, ttl),
                b: fmtDur(tr, max)
            })
        if (!e.period && !e.max && period && max && period > max)
            e.period = tr('e.periodOverMax', 'Период ({{a}}) больше явного максимума ({{b}}).', {
                a: fmtDur(tr, period),
                b: fmtDur(tr, max)
            })
        if (form.uses !== '' && (!/^\d+$/.test(form.uses)))
            e.uses = tr('e.uses', 'Целое число от 0. Пусто или 0 — без ограничений.')
        return e
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [form, mode, existing, ttl, max, period])

    const shown = (k: keyof Errors) => (attempted || touched[k] ? errors[k] : undefined)
    const errCount = Object.keys(errors).length
    const dirty = JSON.stringify(form) !== snap

    const body: TokenRolePut = {
        max_ttl: toGoDuration(max || 0),
        max_uses: Number(form.uses) || undefined,
        period: toGoDuration(period || 0),
        policies: form.policies,
        renewable: form.renew,
        ttl: toGoDuration(ttl || 0)
    }

    const mutation = useMutation({
        mutationFn: () => putTokenRole(form.name.trim(), body),
        onError: (e) => toast.error(tr('saveFailed', 'Роль не сохранена'), errorMessage(e)),
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ['token-roles'] })
            toast.success(tr('saved', 'Сохранено'), tr('savedMsg', 'Роль {{name}} записана в auth/token/roles', { name: form.name.trim() }))
            onClose()
        }
    })

    const save = () => {
        setAttempted(true)
        if (errCount) {
            toast.error(tr('invalidTitle', 'Роль не сохранена'), tr('invalidMsg', 'Исправьте отмеченные поля: {{n}}', { n: errCount }))
            setPane('form')
            return
        }
        mutation.mutate()
    }

    const requestClose = () => {
        if (mutation.isPending) return
        if (dirty && !armed) {
            setArmed(true)
            return
        }
        onClose()
    }

    const renderDur = (id: string, k: 'max' | 'period' | 'ttl', presets?: string[]) => (
        <>
            <div className={styles.suffix}>
                <input
                    className={styles.inp}
                    id={id}
                    onBlur={() => touch(k)}
                    onChange={(e) => {
                        set(k, e.currentTarget.value)
                        touch(k)
                    }}
                    placeholder={k === 'period' ? tr('noPeriod', 'без периода') : k === 'ttl' ? '1h' : '24h'}
                    spellCheck={false}
                    value={form[k]}
                />
                <span>{tr('durUnits', 'с/м/ч/д')}</span>
            </div>
            {presets && (
                <div className={styles.dur}>
                    {presets.map((p) => (
                        <button
                            key={p}
                            onClick={() => {
                                set(k, p)
                                touch(k)
                            }}
                            type="button"
                        >
                            {p}
                        </button>
                    ))}
                </div>
            )}
        </>
    )

    const summary = armed
        ? tr('sumArmed', 'Несохранённые изменения будут потеряны — нажмите ещё раз, чтобы закрыть')
        : errCount && attempted
          ? tr('sumErrors', 'Ошибок: {{n}} — исправьте поля с отметкой', { n: errCount })
          : dirty
            ? tr('sumDirty', 'Есть несохранённые изменения')
            : tr('sumClean', 'Изменений нет')

    const title =
        mode === 'edit'
            ? tr('drawerEdit', 'Изменение роли · {{name}}', { name: role?.name })
            : mode === 'dup'
              ? tr('drawerDup', 'Копия роли · {{name}}', { name: role?.name })
              : tr('drawerNew', 'Новая роль · конструктор')

    return (
        <Drawer
            busy={mutation.isPending}
            footer={
                <div className={styles.foot}>
                    <span className={cx(styles.sum, (armed || (errCount > 0 && attempted)) && styles.sumBad)}>{summary}</span>
                    <Button disabled={mutation.isPending} onClick={requestClose} variant="default">
                        {tr('cancel', 'Отмена')}
                    </Button>
                    <Button leftSection={<Icon name="check" size={15} />} loading={mutation.isPending} onClick={save}>
                        {mode === 'edit' ? tr('saveEdit', 'Сохранить изменения') : tr('saveNew', 'Сохранить роль')}
                    </Button>
                </div>
            }
            icon="key"
            onClose={requestClose}
            opened={opened}
            size={1180}
            title={title}
        >
            <div className={styles.mtabs}>
                <button className={cx(pane === 'form' && styles.on)} onClick={() => setPane('form')} type="button">
                    {tr('tabForm', 'Параметры')}
                </button>
                <button className={cx(pane === 'prev' && styles.on)} onClick={() => setPane('prev')} type="button">
                    {tr('tabPrev', 'Предпросмотр')}
                </button>
            </div>
            <div className={styles.body} data-pane={pane}>
                <form
                    autoComplete="off"
                    className={styles.form}
                    noValidate
                    onKeyDown={(e) => {
                        if (e.key === 'Enter' && (e.target as HTMLElement).tagName === 'INPUT' && !e.defaultPrevented) {
                            e.preventDefault()
                            save()
                        }
                    }}
                    onSubmit={(e) => {
                        e.preventDefault()
                        save()
                    }}
                >
                    <Group icon="user" title={tr('gId', 'Идентификация')}>
                        <Fld
                            code="PUT /roles/{name}"
                            error={shown('name')}
                            help={tr('nameHelp', 'Строчные латинские буквы, цифры, дефис. Имя нельзя изменить после создания.')}
                            id="tr-name"
                            label={tr('name', 'Имя роли')}
                        >
                            <input
                                autoFocus={mode !== 'edit'}
                                className={styles.inp}
                                data-autofocus={mode !== 'edit' ? true : undefined}
                                disabled={mode === 'edit'}
                                id="tr-name"
                                onBlur={() => touch('name')}
                                onChange={(e) => {
                                    set('name', e.currentTarget.value)
                                    touch('name')
                                }}
                                placeholder="payments-api"
                                spellCheck={false}
                                value={form.name}
                            />
                        </Fld>
                    </Group>

                    <Group hint={<span>{form.policies.length}</span>} icon="lock" title={tr('gPol', 'Политики')}>
                        <Fld
                            code="policies"
                            error={shown('policies')}
                            help={tr('polHelp', 'Токен, выпущенный по роли, получит эти политики и default.')}
                            id="tr-pol"
                            label={tr('policies', 'Политики роли')}
                        >
                            <TagInput
                                id="tr-pol"
                                isValid={(v) => POLICY_RE.test(v)}
                                onChange={(v) => set('policies', v)}
                                onTouch={() => touch('policies')}
                                placeholder={tr('polPlaceholder', 'имя политики и Enter')}
                                suggestions={(policyNames ?? []).filter((p) => p !== 'root').slice(0, 10)}
                                value={form.policies}
                            />
                        </Fld>
                    </Group>

                    <Group icon="refresh" title={tr('gTtl', 'Время жизни')}>
                        <div className={styles.row2}>
                            <Fld code="ttl" error={shown('ttl')} id="tr-ttl" label="TTL">
                                {renderDur('tr-ttl', 'ttl', ['15m', '1h', '8h', '24h'])}
                            </Fld>
                            <Fld code="max_ttl" error={shown('max')} id="tr-max" label={tr('maxTtl', 'Максимум')}>
                                {renderDur('tr-max', 'max', ['4h', '24h', '72h', '7d'])}
                            </Fld>
                        </div>
                        <div className={styles.row2}>
                            <Fld
                                code="period"
                                error={shown('period')}
                                help={tr('periodHelp', 'Токен с периодом продлевается бессрочно, пока его обновляют вовремя.')}
                                id="tr-period"
                                label={tr('periodLabel', 'Период')}
                            >
                                {renderDur('tr-period', 'period')}
                            </Fld>
                            <Fld code="max_uses" error={shown('uses')} id="tr-uses" label={tr('maxUses', 'Макс. использований')}>
                                <input
                                    className={styles.inp}
                                    id="tr-uses"
                                    inputMode="numeric"
                                    min={0}
                                    onBlur={() => touch('uses')}
                                    onChange={(e) => {
                                        set('uses', e.currentTarget.value)
                                        touch('uses')
                                    }}
                                    placeholder={tr('unlimitedPh', 'без ограничений')}
                                    type="number"
                                    value={form.uses}
                                />
                            </Fld>
                        </div>
                        <div
                            aria-checked={form.renew}
                            className={cx(styles.tgl, form.renew && styles.tglOn)}
                            onClick={() => set('renew', !form.renew)}
                            onKeyDown={(e) => {
                                if (e.key === ' ' || e.key === 'Enter') {
                                    e.preventDefault()
                                    e.stopPropagation()
                                    set('renew', !form.renew)
                                }
                            }}
                            role="switch"
                            tabIndex={0}
                        >
                            <span className={styles.sw} />
                            <span>
                                <b>{tr('renewable', 'Продлеваемый')}</b>
                                <small>{tr('renewHelp', 'Держатель токена может продлевать его до явного максимума.')}</small>
                            </span>
                        </div>
                        {!!period && !!ttl && (
                            <div className={cx(styles.callout, styles.warn)}>
                                <Icon name="info" size={15} />
                                <span>
                                    {tr('perWarn', 'Задан период: TTL игнорируется при продлении, а явный максимум ограничивает суммарную жизнь токена.')}
                                </span>
                            </div>
                        )}
                    </Group>
                </form>

                <div className={styles.prev}>
                    <div className={styles.pv}>
                        <div className={styles.pvSec}>
                            <h5>
                                {tr('pvLife', 'Срок жизни')}
                                <em>
                                    {max
                                        ? tr('pvMax', 'максимум {{v}}', { v: fmtDur(tr, max) })
                                        : tr('pvSysMax', 'системный максимум 32 д')}
                                </em>
                            </h5>
                            <LifetimeRing bad={!!errors.ttl || !!errors.period} max={max || 0} period={period || 0} ttl={ttl || 0} uses={form.uses} />
                        </div>
                        <Plate variant="flat">
                            <PlateBar>PUT /v1/auth/token/roles/{form.name.trim() || '…'}</PlateBar>
                            <Json body={body} />
                        </Plate>
                    </div>
                </div>
            </div>
        </Drawer>
    )
}
