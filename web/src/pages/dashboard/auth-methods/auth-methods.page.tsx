// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from '@tanstack/react-query'
import { CSSProperties, KeyboardEvent, ReactNode } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'

import { CountUp, cx, SkeletonLine } from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Pill } from '@shared/ui/tuck/tuck'

import { status } from './api'
import styles from './auth-methods.module.css'
import { ApprolePanel } from './components/approle-panel'
import { JwtPanel } from './components/jwt-panel'
import { K8sPanel } from './components/k8s-panel'
import { LdapPanel } from './components/ldap-panel'
import { ForbiddenPlate, rich } from './components/shared'
import { TokenPanel } from './components/token-panel'
import { approleListQ, jwtConfigQ, k8sClustersQ, ldapConfigQ } from './queries'

type Method = 'approle' | 'jwt' | 'kubernetes' | 'ldap' | 'token'
const METHODS: Method[] = ['approle', 'jwt', 'ldap', 'kubernetes', 'token']

const PATH: Record<Method, string> = {
    approle: 'auth/approle/',
    jwt: 'auth/jwt/',
    kubernetes: 'auth/kubernetes/',
    ldap: 'auth/ldap/',
    token: 'auth/token/'
}

export function AuthMethodsPage() {
    const { t } = useTranslation('auth')
    const [params, setParams] = useSearchParams()
    const raw = params.get('method')
    const active: Method = METHODS.includes(raw as Method) ? (raw as Method) : 'approle'
    const select = (m: Method) => setParams(m === 'approle' ? {} : { method: m }, { replace: true })

    // probes: the same queries the panels use, so the rail and the panes share one cache entry
    const approle = useQuery(approleListQ())
    const jwt = useQuery(jwtConfigQ())
    const ldap = useQuery(ldapConfigQ())
    const k8s = useQuery(k8sClustersQ())

    const denied = (q: { error: unknown; isError: boolean }) => q.isError && status(q.error) === 403
    const missing = (q: { error: unknown; isError: boolean }) => q.isError && status(q.error) === 404
    const allDenied = denied(approle) && denied(jwt) && denied(ldap) && denied(k8s)
    const settled = !approle.isLoading && !jwt.isLoading && !ldap.isLoading && !k8s.isLoading

    const rail: Record<Method, { sub: string; tone: 'b' | 'g' | 'n' | 'r'; word: string }> = {
        approle: probe(approle, approle.data ? t('rail.roles', { count: approle.data.length }) : ''),
        jwt: probe(jwt, jwt.data ? t('rail.configured') : '', missing(jwt)),
        kubernetes: probe(k8s, k8s.data ? t('rail.clusters', { count: k8s.data.length + 1 }) : ''),
        ldap: probe(ldap, ldap.data ? t('rail.configured') : '', missing(ldap)),
        token: { sub: t('rail.builtIn'), tone: 'g', word: t('rail.on') }
    }

    function probe(
        q: { error: unknown; isError: boolean; isLoading: boolean },
        ok: string,
        notConfigured = false
    ): { sub: string; tone: 'b' | 'g' | 'n' | 'r'; word: string } {
        if (q.isLoading) return { sub: '…', tone: 'n', word: '…' }
        if (denied(q)) return { sub: t('rail.denied'), tone: 'r', word: '403' }
        if (notConfigured) return { sub: t('rail.notConfigured'), tone: 'n', word: t('rail.off') }
        if (q.isError) return { sub: t('rail.error'), tone: 'r', word: t('rail.err') }
        return { sub: ok, tone: 'g', word: t('rail.on') }
    }

    const onKey = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
        const d = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0
        if (!d) return
        e.preventDefault()
        const next = METHODS[(i + d + METHODS.length) % METHODS.length]
        select(next)
        document.getElementById(`auth-tab-${next}`)?.focus()
    }

    const roleCount = approle.data?.length
    const facts: Array<[ReactNode, string]> = [
        [roleCount === undefined ? <SkeletonLine height={24} width={40} /> : <CountUp value={roleCount} />, t('facts.approle')],
        [
            k8s.data ? <CountUp value={k8s.data.length + 1} /> : <SkeletonLine height={24} width={40} />,
            t('facts.clusters')
        ],
        [
            settled ? (
                <CountUp value={[jwt.data, ldap.data].filter(Boolean).length + 1} />
            ) : (
                <SkeletonLine height={24} width={40} />
            ),
            t('facts.methods')
        ]
    ]

    return (
        <Page title={t('title')}>
            <div className={styles.phead}>
                <div>
                    <Eyebrow>{t('eyebrow')}</Eyebrow>
                    <h1 className={styles.h1}>
                        <span>{t('h1a')}</span>
                        <span className={styles.gold}>{t('h1b')}</span>
                    </h1>
                    <p className={cx(styles.lead, styles.rz)} style={{ '--i': 2 } as CSSProperties}>
                        <Trans components={rich} i18nKey="lead" ns="auth" />
                    </p>
                </div>
                {!allDenied && (
                    <div aria-label={t('facts.label')} className={cx(styles.facts, styles.rz)} style={{ '--i': 3 } as CSSProperties} role="group">
                        {facts.map(([v, label]) => (
                            <div key={label}>
                                <b>{v}</b>
                                {label}
                            </div>
                        ))}
                    </div>
                )}
            </div>

            {allDenied ? (
                <ForbiddenPlate
                    need="auth/*"
                    onRetry={() => {
                        void approle.refetch()
                        void jwt.refetch()
                        void ldap.refetch()
                        void k8s.refetch()
                    }}
                    request="GET /v1/auth/approle/role/?list=true"
                    retrying={approle.isFetching || jwt.isFetching || ldap.isFetching || k8s.isFetching}
                />
            ) : (
                <>
                    <div aria-label={t('rail.label')} className={cx(styles.rail, styles.rz)} role="tablist" style={{ '--i': 4 } as CSSProperties}>
                        {METHODS.map((m, i) => (
                            <button
                                aria-controls={`auth-pane-${m}`}
                                aria-selected={active === m}
                                className={cx(styles.m, active === m && styles.on)}
                                id={`auth-tab-${m}`}
                                key={m}
                                onClick={() => select(m)}
                                onKeyDown={(e) => onKey(e, i)}
                                role="tab"
                                tabIndex={active === m ? 0 : -1}
                                type="button"
                            >
                                <span className={styles.mTop}>
                                    <span className={styles.n}>{t(`methods.${m}`)}</span>
                                    <Pill dot size="sm" tone={rail[m].tone}>
                                        {rail[m].word}
                                    </Pill>
                                </span>
                                <small>{PATH[m]}</small>
                                <small>{rail[m].sub}</small>
                            </button>
                        ))}
                    </div>

                    <div
                        aria-labelledby={`auth-tab-${active}`}
                        className={styles.rz}
                        id={`auth-pane-${active}`}
                        key={active}
                        role="tabpanel"
                        style={{ '--i': 0 } as CSSProperties}
                    >
                        {active === 'approle' && <ApprolePanel />}
                        {active === 'jwt' && <JwtPanel />}
                        {active === 'ldap' && <LdapPanel />}
                        {active === 'kubernetes' && <K8sPanel />}
                        {active === 'token' && <TokenPanel />}
                    </div>
                </>
            )}
        </Page>
    )
}
