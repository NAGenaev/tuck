// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties, KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { CountUp, prefersReducedMotion } from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Pill, Plate, PlateBar } from '@shared/ui/tuck/tuck'

import { WrapCtx } from './components/ctx'
import { WIcon } from './components/icons'
import { Lifecycle } from './components/lifecycle'
import { LogPlate } from './components/log-plate'
import { LookPanel } from './components/look-panel'
import { rich, TokBox } from './components/parts'
import { Stage, StageState } from './components/stage'
import { UnwrapPanel } from './components/unwrap-panel'
import { WrapPanel } from './components/wrap-panel'
import {
    clock,
    fmtDate,
    fmtLeft,
    Op,
    Particle,
    Problem,
    randomParticles,
    RING,
    STATUS_TONE,
    statusOf,
    Tab
} from './lib'
import { useSession } from './use-session'
import styles from './wrapping.module.css'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')
const d = (n: number) => ({ '--d': n }) as CSSProperties

const TABS: Tab[] = ['wrap', 'look', 'unwrap']

export function WrappingPage() {
    const { t, i18n } = useTranslation('wrapping')
    const session = useSession()
    const { capsules, cur, now, setCur, upsert } = session

    const [tab, setTab] = useState<Tab>('wrap')
    const [tokens, setTokens] = useState({ look: '', unwrap: '' })
    const [problems, setProblems] = useState<Partial<Record<Op, Problem>>>({})
    const [override, setOverride] = useState<null | StageState>(null)
    const [particles, setParticles] = useState<Particle[]>([])
    const [shake, setShake] = useState(0)
    const timers = useRef<number[]>([])
    const tabRefs = useRef<Record<Tab, HTMLButtonElement | null>>({ look: null, unwrap: null, wrap: null })

    const later = useCallback((fn: () => void, ms: number) => {
        timers.current.push(window.setTimeout(fn, ms))
    }, [])

    useEffect(() => {
        const list = timers.current
        return () => list.forEach((id) => window.clearTimeout(id))
    }, [])

    const ctx: WrapCtx = useMemo(
        () => ({
            clearProblem: (op) =>
                setProblems((p) => {
                    if (!p[op]) return p
                    const n = { ...p }
                    delete n[op]
                    return n
                }),
            failed: (token, status) => {
                // 404 can mean "already unwrapped" or "never existed": the server does not say which
                if (status === 404) {
                    if (capsules[token]?.status !== 'used') upsert(token, { status: 'missing' })
                } else if (status === 410) {
                    if (capsules[token]?.status !== 'used') upsert(token, { status: 'expired' })
                }
            },
            goto: (to, token) => {
                setTab(to)
                if (token) {
                    if (to !== 'wrap') setTokens((s) => ({ ...s, [to]: token }))
                    setCur(token)
                }
            },
            playUnwrap: () => {
                if (prefersReducedMotion()) return
                timers.current.forEach((id) => window.clearTimeout(id))
                timers.current.length = 0
                setParticles([])
                setOverride('breaking')
                later(() => {
                    setOverride('dissolved')
                    setParticles(randomParticles())
                }, 1100)
                later(() => {
                    setOverride(null)
                    setParticles([])
                }, 2000)
            },
            problems,
            seal: (on) => setOverride(on ? 'sealing' : null),
            session,
            setProblem: (op, p) => setProblems((s) => ({ ...s, [op]: p })),
            setToken: (to, value) => setTokens((s) => ({ ...s, [to]: value })),
            shake: () => setShake((n) => n + 1),
            tokens
        }),
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [capsules, later, problems, session, setCur, tokens, upsert]
    )

    const capsule = cur ? capsules[cur] : undefined
    const status = statusOf(capsule, now)
    const left = capsule?.expires !== undefined ? capsule.expires - now : 0
    const total = capsule?.expires !== undefined && capsule.created !== undefined ? capsule.expires - capsule.created : 0
    const forbidden = Object.values(problems).some((p) => p?.status === 403)

    let state: StageState = 'idle'
    if (override) state = override
    else if (status === 'active') state = left <= 30000 || (total > 0 && left / total <= 0.15) ? 'low' : 'sealed'
    else if (status) state = 'void'
    else if (forbidden) state = 'forbidden'

    const labelKey = override ?? (status && status !== 'active' ? status : state)
    const active = status === 'active' && !override
    const offset = active && total > 0 ? Math.min(RING, Math.max(0, RING * (1 - left / total))) : 0

    const caps = Object.values(capsules)
    const wrapped = caps.filter((c) => c.created !== undefined).length
    const unwrapped = caps.filter((c) => c.status === 'used').length
    const live = caps.filter((c) => statusOf(c, now) === 'active').length

    const onTabKey = (e: KeyboardEvent<HTMLDivElement>) => {
        const i = TABS.indexOf(tab)
        let n = -1
        if (e.key === 'ArrowRight') n = (i + 1) % TABS.length
        else if (e.key === 'ArrowLeft') n = (i + TABS.length - 1) % TABS.length
        else if (e.key === 'Home') n = 0
        else if (e.key === 'End') n = TABS.length - 1
        if (n < 0) return
        e.preventDefault()
        setTab(TABS[n])
        tabRefs.current[TABS[n]]?.focus()
    }

    return (
        <Page title={t('title')}>
            <div className={styles.root}>
                <div className={styles.phead}>
                    <div>
                        <Eyebrow className={styles.rv}>{t('eyebrow')}</Eyebrow>
                        <h1 className={styles.h1}>
                            <span>{t('h1a')}</span>
                            <span className={styles.gold}>{t('h1b')}</span>
                        </h1>
                        <p className={cx(styles.lead, styles.rv)} style={d(3)}>
                            <Trans components={rich} i18nKey="lead" ns="wrapping" />
                        </p>
                    </div>
                    <div className={cx(styles.facts, styles.rv)} style={d(4)}>
                        <div>
                            <b>
                                <CountUp value={wrapped} />
                            </b>
                            {t('facts.wrapped')}
                        </div>
                        <div>
                            <b>
                                <CountUp value={unwrapped} />
                            </b>
                            {t('facts.unwrapped')}
                        </div>
                        <div>
                            <b>
                                <CountUp value={live} />
                            </b>
                            {t('facts.active')}
                        </div>
                    </div>
                </div>

                <div className={styles.grid}>
                    <Plate className={cx(styles.sweep, styles.rv)} style={d(5)}>
                        <PlateBar>/v1/sys/wrapping</PlateBar>
                        <div
                            aria-label={t('tabsLabel')}
                            className={styles.tabs}
                            onKeyDown={onTabKey}
                            role="tablist"
                        >
                            {TABS.map((k) => (
                                <button
                                    aria-controls={`p-${k}`}
                                    aria-selected={tab === k}
                                    id={`tab-${k}`}
                                    key={k}
                                    onClick={() => setTab(k)}
                                    ref={(el) => {
                                        tabRefs.current[k] = el
                                    }}
                                    role="tab"
                                    tabIndex={tab === k ? 0 : -1}
                                    type="button"
                                >
                                    <WIcon name={k === 'wrap' ? 'wrap' : k === 'look' ? 'look' : 'unwrap'} />
                                    {t(`tabs.${k}`)}
                                </button>
                            ))}
                        </div>
                        <WrapPanel ctx={ctx} hidden={tab !== 'wrap'} />
                        <LookPanel ctx={ctx} hidden={tab !== 'look'} />
                        <UnwrapPanel ctx={ctx} hidden={tab !== 'unwrap'} />
                    </Plate>

                    <Plate className={cx(styles.sweep, styles.rv)} style={d(6)}>
                        <div className={styles.stageWrap}>
                            <Stage
                                label={t(`stage.${labelKey}`)}
                                offset={offset}
                                particles={particles}
                                shake={shake}
                                state={state}
                                tag={t('stage.tag')}
                                time={active ? clock(left) : '00:00'}
                                timeSub={t('stage.until')}
                            />
                            <div className={styles.meta}>
                                <div className={styles.lab} style={{ marginBottom: 8 }}>
                                    {t('meta.selected')}
                                </div>
                                <div style={{ marginBottom: 14 }}>
                                    <TokBox emptyText={t('meta.none')} value={cur} />
                                </div>
                                <dl className={styles.dl}>
                                    <div>
                                        <dt>{t('meta.status')}</dt>
                                        <dd>
                                            <Pill tone={status ? STATUS_TONE[status] : 'n'}>
                                                {t(`status.${status ?? 'none'}`)}
                                            </Pill>
                                        </dd>
                                    </div>
                                    <div>
                                        <dt>{t('meta.created')}</dt>
                                        <dd>{capsule?.created ? fmtDate(capsule.created, i18n.language) : '—'}</dd>
                                    </div>
                                    <div>
                                        <dt>{t('meta.left')}</dt>
                                        <dd className={status === 'active' ? styles.brassL : undefined}>
                                            {status === 'active' && capsule?.expires !== undefined
                                                ? fmtLeft(t, left)
                                                : '—'}
                                        </dd>
                                    </div>
                                </dl>
                            </div>
                            <Lifecycle capsule={capsule} status={status} />
                            <div className={styles.stageNote}>{t('life.note')}</div>
                        </div>
                    </Plate>
                </div>

                <div className={styles.rv} style={d(7)}>
                    <LogPlate session={session} />
                </div>
            </div>
        </Page>
    )
}
