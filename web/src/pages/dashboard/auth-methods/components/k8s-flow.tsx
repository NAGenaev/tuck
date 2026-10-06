// SPDX-License-Identifier: AGPL-3.0-only
// The Kubernetes login schema. It is an explanation, not telemetry: the "run" only steps through the
// documented sequence (service-account JWT -> TokenReview -> role lookup -> token); nothing is
// measured and no token material is shown.
import { Button } from '@mantine/core'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cx, Icon, prefersReducedMotion } from '@shared/ui/kit'
import { Plate, PlateBar } from '@shared/ui/tuck/tuck'

import styles from '../auth-methods.module.css'

type Scenario = 'denied' | 'ok'

// step 0..4 are the five phases; the denied scenario stops at the role lookup (index 3)
const STEPS = 5
const FAIL_AT = 3
const STEP_MS = 1100

const svg = (children: React.ReactNode) => (
    <svg fill="none" stroke="currentColor" strokeWidth="1.8" viewBox="0 0 24 24">
        {children}
    </svg>
)

const IC_POD = svg(
    <>
        <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z" />
        <path d="M12 12l8-4.5M12 12v9M12 12L4 7.5" />
    </>
)
const IC_TUCK = svg(
    <>
        <rect height="11" rx="1" width="16" x="4" y="10" />
        <path d="M8 10V7a4 4 0 018 0v3" />
    </>
)
const IC_API = svg(
    <>
        <circle cx="12" cy="12" r="8" />
        <path d="M12 4v16M4 12h16" />
    </>
)
const IC_TOKEN = svg(
    <>
        <circle cx="8" cy="12" r="4" />
        <path d="M12 12h9M18 12v3M21 12v2" />
    </>
)
const IC_SECRET = svg(
    <>
        <path d="M6 3h9l4 4v14H6z" />
        <path d="M9 12h7M9 16h7" />
    </>
)

export function K8sFlow({ defaultCluster }: { defaultCluster: string }) {
    const { t } = useTranslation('auth')
    const [scenario, setScenario] = useState<Scenario>('ok')
    const [step, setStep] = useState(-1) // -1 idle, 0..STEPS-1 current, STEPS finished
    const [running, setRunning] = useState(false)
    const timers = useRef<number[]>([])

    const clear = () => {
        timers.current.forEach((id) => window.clearTimeout(id))
        timers.current = []
    }
    useEffect(() => clear, [])

    const run = useCallback((sc: Scenario) => {
        clear()
        const last = sc === 'denied' ? FAIL_AT : STEPS
        if (prefersReducedMotion()) {
            setStep(last)
            setRunning(false)
            return
        }
        setRunning(true)
        setStep(0)
        for (let i = 1; i <= last; i++) {
            timers.current.push(
                window.setTimeout(() => {
                    setStep(i)
                    if (i === last) setRunning(false)
                }, i * STEP_MS)
            )
        }
    }, [])

    // play once on arrival
    useEffect(() => {
        run('ok')
    }, [run])

    const failed = scenario === 'denied' && step >= FAIL_AT
    const finished = step >= (scenario === 'denied' ? FAIL_AT : STEPS)
    const ok = scenario === 'ok' && step >= STEPS

    // which phase owns which node
    const state = (phase: number): string => {
        if (step < 0) return ''
        if (scenario === 'denied' && phase === FAIL_AT && step >= FAIL_AT) return styles.fail
        if (step === phase && !finished) return styles.act
        return step > phase || (step === phase && finished && !failed) ? styles.done : ''
    }
    const nodeStatus = (cls: string) =>
        cls === styles.act
            ? t('flow.state.act')
            : cls === styles.done
              ? t('flow.state.done')
              : cls === styles.fail
                ? t('flow.state.fail')
                : t('flow.state.idle')

    const nPod = state(0)
    const nTuck = failed
        ? styles.fail
        : step < 0
          ? ''
          : (step === 1 || step === 3) && !finished
            ? styles.act
            : step > 1
              ? styles.done
              : ''
    const nApi = state(2)
    const nTok = state(4)

    const conn = (phase: number) => {
        if (step < 0) return ''
        if (scenario === 'denied' && phase >= FAIL_AT && step >= FAIL_AT) return phase === FAIL_AT ? styles.bad : ''
        if (step === phase && !finished) return styles.on
        return step > phase ? styles.ok : ''
    }

    const stepList = [0, 1, 2, 3, 4].map((i) => ({
        cls:
            scenario === 'denied' && i === FAIL_AT && step >= FAIL_AT
                ? styles.fail
                : step === i && !finished
                  ? styles.act
                  : step > i
                    ? styles.done
                    : '',
        i
    }))

    return (
        <Plate>
            <PlateBar>{t('flow.bar')}</PlateBar>
            <div className={cx(styles.prog, failed && styles.bad)}>
                <i
                    style={{
                        width: `${step < 0 ? 0 : Math.min(100, ((step >= STEPS ? STEPS : step + (running ? 0.5 : 0)) / STEPS) * 100)}%`
                    }}
                />
            </div>
            <div className={styles.plateIn}>
                <div className={styles.flowCtl}>
                    <div aria-label={t('flow.scenario')} className={styles.seg} role="group">
                        {(['ok', 'denied'] as const).map((sc) => (
                            <button
                                aria-pressed={scenario === sc}
                                className={cx(scenario === sc && styles.on)}
                                key={sc}
                                onClick={() => {
                                    setScenario(sc)
                                    run(sc)
                                }}
                                type="button"
                            >
                                {t(`flow.sc.${sc}`)}
                            </button>
                        ))}
                    </div>
                    <Button
                        disabled={running}
                        leftSection={<Icon name="refresh" size={14} />}
                        onClick={() => run(scenario)}
                        size="xs"
                        variant="default"
                    >
                        {t('flow.replay')}
                    </Button>
                </div>

                <div className={styles.stage}>
                    <div className={styles.flow}>
                        <div className={cx(styles.node, styles.nPod, nPod)}>
                            <span className={styles.nst}>{nodeStatus(nPod)}</span>
                            <div className={styles.nico}>{IC_POD}</div>
                            <b>{t('flow.pod')}</b>
                            <small>{t('flow.podSub')}</small>
                            <span className={styles.tokline}>{'<service-account JWT>'}</span>
                        </div>
                        <div className={cx(styles.conn, styles.h, styles.c0, conn(0))}>
                            <span className={styles.lb}>POST /login</span>
                            <div className={styles.w} />
                        </div>
                        <div className={cx(styles.node, styles.nTuck, nTuck)}>
                            <span className={styles.nst}>{nodeStatus(nTuck)}</span>
                            <div className={styles.nico}>{IC_TUCK}</div>
                            <b>Tuck</b>
                            <small>{t('flow.tuckSub')}</small>
                        </div>
                        <div className={cx(styles.conn, styles.h, styles.c1, conn(4))}>
                            <span className={styles.lb}>{t('flow.issue')}</span>
                            <div className={styles.w} />
                        </div>
                        <div className={cx(styles.node, styles.nTok, nTok)}>
                            <span className={styles.nst}>{nodeStatus(nTok)}</span>
                            <div className={styles.nico}>{IC_TOKEN}</div>
                            <b>{t('flow.token')}</b>
                            <small>{t('flow.tokenSub')}</small>
                            <span className={styles.tokline}>{'<tuck token>'}</span>
                        </div>
                        <div className={cx(styles.conn, styles.h, styles.c2, ok && styles.ok)}>
                            <span className={styles.lb}>GET /v1/kv/…</span>
                            <div className={styles.w} />
                        </div>
                        <div className={cx(styles.node, styles.nSec, ok && styles.done)}>
                            <span className={styles.nst}>{ok ? t('flow.state.done') : t('flow.state.idle')}</span>
                            <div className={styles.nico}>{IC_SECRET}</div>
                            <b>{t('flow.secret')}</b>
                            <small>{t('flow.secretSub')}</small>
                        </div>
                        <div className={cx(styles.conn, styles.v, styles.cv, conn(2))}>
                            <span className={styles.lb}>TokenReview</span>
                            <div className={styles.w} />
                        </div>
                        <div className={cx(styles.node, styles.nApi, nApi)}>
                            <span className={styles.nst}>{nodeStatus(nApi)}</span>
                            <div className={styles.nico}>{IC_API}</div>
                            <b>{t('flow.api')}</b>
                            <small>{defaultCluster}</small>
                        </div>
                    </div>
                </div>

                <div className={styles.below}>
                    <ol aria-label={t('flow.steps')} className={styles.stepl}>
                        {stepList.map(({ i, cls }) => (
                            <li className={cls} key={i}>
                                <i>{`0${i + 1}`}</i>
                                <span>{t(`flow.step.${i}`)}</span>
                            </li>
                        ))}
                    </ol>
                    <div>
                        <div
                            aria-live="polite"
                            className={cx(styles.verdict, ok && styles.ok, failed && styles.no)}
                            role="status"
                        >
                            <b>{ok ? t('flow.verdict.ok') : failed ? t('flow.verdict.no') : t('flow.verdict.idle')}</b>
                            <span>{ok ? t('flow.verdict.okText') : failed ? t('flow.verdict.noText') : t('flow.verdict.idleText')}</span>
                        </div>
                        <p className={styles.note}>{t('flow.note')}</p>
                    </div>
                </div>
            </div>
        </Plate>
    )
}
