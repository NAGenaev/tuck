// SPDX-License-Identifier: AGPL-3.0-only
// Forked (layout) from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00);
// submit handler rewritten against Tuck's token-auth model (paste a token, validate via lookup-self —
// Tuck's built-in token auth has no username/password login, matching the previous UI's behavior).
// Visual design: "Консоль Tuck" (login, sealed vault and session-expired variants).
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { CSSProperties, FormEvent, ReactNode, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'

import { useAuthTone } from '@app/layouts/auth/auth.layout'
import { lookupSelf } from '@shared/api/endpoints/auth'
import { unseal } from '@shared/api/endpoints/sys'
import { useSealStatus } from '@shared/api/hooks/use-seal-status'
import { removeToken, setToken } from '@shared/auth/token'
import { ROUTES } from '@shared/constants/routes'
import { useAuth } from '@shared/hooks/use-auth'
import c from '@shared/ui/shell-controls/console.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'
import { ConsoleModal, layerStyles } from '@shared/ui/shell-controls/layer'
import { vaultDoor } from '@shared/ui/shell-controls/vault-door'

import { clearSessionExpired, peekSessionExpired, SessionExpiredInfo } from '../session-expired'
import styles from './login.module.css'

const COMMAND = 'tuck token lookup-self'
const REQUEST_LINE = '→ GET /v1/auth/token/lookup-self'

type Phase = 'busy' | 'fail' | 'idle' | 'ok'
interface Hint {
    icon?: 'check' | 'warn'
    text: string
    tone?: 'err' | 'ok' | 'warn'
}

const reducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches
const delay = (n: number) => ({ '--d': n }) as CSSProperties

function Rivets() {
    return (
        <>
            <i className={c.rivet} />
            <i className={c.rivet} />
            <i className={c.rivet} />
            <i className={c.rivet} />
        </>
    )
}

function Pips({ n, ok, total }: { n: number; ok?: boolean; total: number }) {
    return (
        <div aria-hidden="true" className={c.pips} data-ok={ok ? 'true' : undefined}>
            {Array.from({ length: Math.max(total, 1) }, (_, i) => (
                <i data-on={i < n} key={i} />
            ))}
        </div>
    )
}

function Cursor() {
    return <i className={c.cur} />
}

function TermHead() {
    return (
        <>
            <span className="c"># tuck login</span>
            {'\n'}
            <span className="p">$</span> <span className="w">{COMMAND}</span>
            {'\n'}
        </>
    )
}

function formatTime(iso?: string) {
    if (!iso) return null
    const d = new Date(iso)
    if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return null
    return d.toLocaleString(undefined, { hour12: false })
}

export function LoginPage() {
    const { t } = useTranslation('login')
    const { setIsAuthenticated } = useAuth()
    const navigate = useNavigate()
    const queryClient = useQueryClient()
    const { data: seal } = useSealStatus()

    const [expired, setExpired] = useState<null | SessionExpiredInfo>(() => peekSessionExpired())
    const [token, setTokenInput] = useState('')
    const [reveal, setReveal] = useState(false)
    const [pasted, setPasted] = useState<null | { trimmed: boolean }>(null)
    const [phase, setPhase] = useState<Phase>('idle')
    const [failMsg, setFailMsg] = useState<null | { code: string; text: string }>(null)
    const [typed, setTyped] = useState(() => (reducedMotion() ? COMMAND.length : 0))
    const [unsealOpen, setUnsealOpen] = useState(false)
    const [shake, setShake] = useState(0)
    const inputRef = useRef<HTMLInputElement>(null)

    const sealed = seal?.sealed === true
    const showExpired = expired !== null && !sealed
    useAuthTone(sealed || showExpired ? 'red' : 'brass')

    // the flag is consumed once: a reload shows the plain login form
    useEffect(() => {
        if (expired) clearSessionExpired()
    }, [expired])

    // terminal "typing" of the command on first paint
    useEffect(() => {
        if (typed >= COMMAND.length) return
        const id = window.setTimeout(() => setTyped((n) => n + 1), typed === 0 ? 420 : 38 + Math.random() * 55)
        return () => window.clearTimeout(id)
    }, [typed])

    const hint: Hint = (() => {
        const n = token.length
        if (phase === 'fail' && failMsg) return { icon: 'warn', text: failMsg.text, tone: 'err' }
        if (phase === 'ok') return { icon: 'check', text: t('hintAccepted'), tone: 'ok' }
        if (!n) return { text: t('hintFormat') }
        if (pasted) {
            return {
                icon: 'check',
                text: t('hintPasted', { count: n }) + (pasted.trimmed ? t('hintTrimmed') : ''),
                tone: 'ok'
            }
        }
        if (/^tuck_.{16,}/.test(token)) return { icon: 'check', text: t('hintRecognized'), tone: 'ok' }
        if (/^tuck_/.test(token)) return { text: t('hintIncomplete'), tone: 'warn' }
        return { text: t('hintPrefix'), tone: 'warn' }
    })()

    const fieldState = phase === 'fail' ? 'bad' : phase === 'ok' ? 'good' : undefined
    const busy = phase === 'busy' || phase === 'ok'

    const onChange = (value: string) => {
        setTokenInput(value)
        setPasted(null)
        if (phase === 'fail') setPhase('idle')
    }

    const fail = (code: string, text: string) => {
        setPhase('fail')
        setFailMsg({ code, text })
        setShake((n) => n + 1)
        inputRef.current?.focus()
    }

    const paste = async () => {
        try {
            const raw = await navigator.clipboard.readText()
            const v = raw.trim()
            if (!v) return
            setTokenInput(v)
            setPasted({ trimmed: v !== raw })
            if (phase === 'fail') setPhase('idle')
            inputRef.current?.focus()
        } catch {
            fail('', t('clipboardDenied'))
        }
    }

    const handleSubmit = async (e: FormEvent) => {
        e.preventDefault()
        if (busy || sealed) return
        const value = token.trim()
        if (!value) {
            fail('', t('errEmpty'))
            return
        }
        setPhase('busy')
        setFailMsg(null)
        try {
            setToken(value)
            await lookupSelf()
        } catch (err) {
            removeToken()
            const status = isAxiosError(err) ? err.response?.status : undefined
            if (isAxiosError(err) && !err.response) fail('', t('errNetwork'))
            else if (status === 401 || status === 403) fail(String(status), t('invalidToken'))
            else fail(String(status ?? ''), t('errServer'))
            return
        }
        // success: close the "vault door", switch to the dashboard behind it, open it again
        setPhase('ok')
        clearSessionExpired()
        const door = vaultDoor(t('door'))
        await door.shut()
        setIsAuthenticated(true)
        navigate(ROUTES.DASHBOARD.STATUS)
        void door.open()
    }

    const received = seal?.received_shards ?? 0
    const total = seal?.required_shards ?? 0
    const sealLine = seal ? `${seal.type ?? '—'} · ${received} / ${total}` : '…'

    /* ------------------------------------------------------------ */
    /* 401 / session expired                                         */
    /* ------------------------------------------------------------ */
    if (showExpired) {
        const expiredAt = formatTime(expired.expiresAt)
        const seenAt = formatTime(expired.at)
        return (
            <div className={`${c.wrap} ${c.errGrid}`}>
                <div>
                    <div className={`${c.badge} ${c.rv}`} data-tone="red" style={delay(0)}>
                        <i />
                        {t('expired.badge')}
                    </div>
                    <br />
                    <div className={`${c.big} ${c.rv}`} data-t="401" data-tone="red" style={delay(1)}>
                        401
                    </div>
                    <h2 className={`${c.errTitle} ${c.rv}`} style={delay(2)}>
                        {t('expired.title')} <em style={{ color: '#ff5a4d' }}>{t('expired.titleEm')}</em>
                    </h2>
                    <p className={`${c.lead} ${c.rv}`} style={delay(3)}>
                        {t('expired.lead')}
                    </p>
                    <div className={`${c.ctaRow} ${c.rv}`} style={delay(4)}>
                        <button className={`${c.btn} ${c.hot}`} onClick={() => setExpired(null)} type="button">
                            {t('expired.again')}
                        </button>
                    </div>
                </div>
                <div className={`${c.plate} ${c.rv}`} data-tone="red" style={delay(3)}>
                    <Rivets />
                    <div className={c.plateTop}>
                        <span />
                        <span />
                        <span />
                        <em>auth · lookup-self</em>
                    </div>
                    <div className={c.term} style={{ borderBottom: 0 }}>
                        <span className="c">{`# ${t('expired.termComment')}`}</span>
                        {'\n'}
                        <span className="p">$</span> <span className="w">{COMMAND}</span>
                        {'\n'}
                        <span className="k">status</span>
                        {':  '}
                        <span className="e">401 unauthorized</span>
                        {'\n'}
                        {seenAt && (
                            <>
                                <span className="k">at</span>
                                {':      '}
                                <span className="w">{seenAt}</span>
                                {'\n'}
                            </>
                        )}
                        {expiredAt && (
                            <>
                                <span className="k">expired</span>
                                {': '}
                                <span className="e">{expiredAt}</span>
                                {'\n'}
                            </>
                        )}
                        <span className="k">token</span>
                        {':   '}
                        <span className="c">{t('expired.removed')}</span> <Cursor />
                    </div>
                    <div className={c.seal} data-tone="red" style={{ borderTop: 0 }}>
                        <svg className="ring" fill="none" viewBox="0 0 44 44">
                            <circle cx="22" cy="22" r="19" stroke="#3B4154" strokeWidth="2" />
                            <circle cx="22" cy="22" r="12" stroke="#FF5A4D" strokeDasharray="4 5" strokeWidth="2" />
                            <circle className={c.ringCore} cx="22" cy="22" fill="#FF5A4D" r="3" />
                        </svg>
                        <div className={c.sealTxt}>
                            <b>{t('expired.invalid')}</b>
                            {t('expired.invalidNote')}
                        </div>
                    </div>
                </div>
            </div>
        )
    }

    /* ------------------------------------------------------------ */
    /* login                                                         */
    /* ------------------------------------------------------------ */
    let termBody: ReactNode
    if (typed < COMMAND.length) {
        termBody = (
            <>
                <span className="c"># tuck login</span>
                {'\n'}
                <span className="p">$</span> <span className="w">{COMMAND.slice(0, typed)}</span>
                <Cursor />
            </>
        )
    } else if (phase === 'busy') {
        termBody = (
            <>
                <TermHead />
                <span className="c">{`${REQUEST_LINE}\n  X-Tuck-Token: tuck_••••••••`}</span> <Cursor />
            </>
        )
    } else if (phase === 'ok') {
        termBody = (
            <>
                <TermHead />
                <span className="s">200 ok</span>
                {'\n'}
                <span className="c">{`→ ${t('termOpening')}`}</span>
            </>
        )
    } else if (phase === 'fail' && failMsg?.code) {
        termBody = (
            <>
                <TermHead />
                <span className="c">{REQUEST_LINE}</span>
                {'\n'}
                <span className="e">{`${failMsg.code} ${failMsg.text}`}</span> <Cursor />
            </>
        )
    } else {
        let line: ReactNode
        if (sealed) {
            line = (
                <>
                    <span className="k">error</span>
                    {': '}
                    <span className="e">vault is sealed</span>{' '}
                    <span className="c">{`· ${received} / ${total}`}</span>
                </>
            )
        } else if (token.length) {
            line = (
                <>
                    <span className="k">token</span>
                    {': '}
                    <span className="s">tuck_</span>
                    <span className="w">{'•'.repeat(Math.min(Math.max(token.length - 5, 0), 22))}</span>{' '}
                    <span className="c">({t('chars', { count: token.length })})</span>
                </>
            )
        } else {
            line = <span className="c">{`→ ${t('termWaiting')}`}</span>
        }
        termBody = (
            <>
                <TermHead />
                {line} <Cursor />
            </>
        )
    }

    return (
        <div className={`${c.wrap} ${styles.grid}`}>
            <div>
                <div className={`${c.badge} ${c.rv}`} data-tone={sealed ? 'red' : undefined} style={delay(0)}>
                    <i />
                    {sealed ? t('badgeSealed') : t('badge')}
                </div>
                <h1 className={styles.h1}>
                    <span className={styles.w} style={{ '--i': 0 } as CSSProperties}>
                        <b>{t('title')}</b>
                    </span>
                    <span className={styles.gold} data-tone={sealed ? 'red' : undefined}>
                        {t('titleGold')
                            .split(' ')
                            .map((word, i) => (
                                <span key={i}>
                                    <span className={styles.w} style={{ '--i': i + 1 } as CSSProperties}>
                                        <b>{word}</b>
                                    </span>{' '}
                                </span>
                            ))}
                    </span>
                </h1>
                <p className={`${c.lead} ${c.rv}`} style={delay(5)}>
                    {t('lead')} <b>{t('leadBold')}</b>
                </p>
                <div className={`${styles.facts} ${c.rv}`} style={delay(6)}>
                    <div>
                        <b>AES-256</b>
                        {t('factEnc')}
                    </div>
                    <div>
                        <b>Shamir</b>
                        {t('factSeal')}
                    </div>
                    <div>
                        <b>Raft</b>
                        {t('factRaft')}
                    </div>
                </div>
            </div>

            <div
                className={`${c.plate} ${c.rv} ${shake ? c.shake : ''}`}
                data-tone={sealed ? 'red' : undefined}
                key={shake}
                style={delay(3)}
            >
                <Rivets />
                <div className={c.plateTop}>
                    <span />
                    <span />
                    <span />
                    <em>{t('plate')}</em>
                </div>
                <div aria-live="polite" className={c.term} style={{ minHeight: 150 }}>
                    {termBody}
                </div>

                {sealed && (
                    <div className={styles.sealbox} role="status">
                        <ShellIcon name="lock" size={22} />
                        <div>
                            <b>{t('sealed.title')}</b>
                            {t('sealed.text')}
                        </div>
                    </div>
                )}

                <form autoComplete="off" className={styles.form} noValidate onSubmit={handleSubmit}>
                    <div className={styles.formHead}>
                        <h2>{t('formTitle')}</h2>
                        <span className={c.pill}>lookup-self</span>
                    </div>
                    {phase === 'fail' && failMsg && failMsg.code !== '' && (
                        <div className={styles.alert} role="alert">
                            <ShellIcon name="warn" size={18} />
                            <div>
                                <b>{t('rejected')}</b>
                                {failMsg.text}
                            </div>
                        </div>
                    )}
                    <fieldset disabled={sealed || busy}>
                        <div className={c.field}>
                            <label htmlFor="login-token">
                                {t('token')} <em>{t('chars', { count: token.length })}</em>
                            </label>
                            <div className={c.input} data-state={fieldState}>
                                <input
                                    aria-describedby="login-token-hint"
                                    aria-invalid={phase === 'fail'}
                                    autoCapitalize="off"
                                    autoComplete="off"
                                    autoFocus
                                    id="login-token"
                                    name="token"
                                    onChange={(e) => onChange(e.currentTarget.value)}
                                    placeholder={t('tokenPlaceholder')}
                                    ref={inputRef}
                                    spellCheck={false}
                                    type={reveal ? 'text' : 'password'}
                                    value={token}
                                />
                                <button
                                    aria-label={t('paste')}
                                    className={c.ib}
                                    onClick={paste}
                                    title={t('paste')}
                                    type="button"
                                >
                                    <ShellIcon name="paste" />
                                </button>
                                <button
                                    aria-label={reveal ? t('hide') : t('show')}
                                    aria-pressed={reveal}
                                    className={c.ib}
                                    onClick={() => setReveal((v) => !v)}
                                    title={reveal ? t('hide') : t('show')}
                                    type="button"
                                >
                                    <ShellIcon name={reveal ? 'eyeoff' : 'eye'} />
                                </button>
                            </div>
                            <div className={c.hint} data-tone={hint.tone} id="login-token-hint">
                                {hint.icon && <ShellIcon name={hint.icon} size={14} />}
                                {hint.text}
                            </div>
                        </div>
                        <button
                            className={`${c.btn} ${c.hot} ${c.full} ${phase === 'busy' ? c.loading : ''}`}
                            type="submit"
                        >
                            <span className={c.spinner} />
                            {sealed ? t('submitSealed') : t('signIn')}
                        </button>
                    </fieldset>
                    <div className={styles.note}>{t('tokenHint')}</div>
                </form>

                <div className={c.seal} data-tone={sealed ? 'red' : undefined}>
                    <svg className="ring" fill="none" viewBox="0 0 44 44">
                        <circle cx="22" cy="22" r="19" stroke="#3B4154" strokeWidth="2" />
                        <circle
                            className={c.ringDash}
                            cx="22"
                            cy="22"
                            r="12"
                            stroke={sealed ? '#FF5A4D' : '#D7A94B'}
                            strokeDasharray="4 5"
                            strokeWidth="2"
                        />
                        <circle className={c.ringCore} cx="22" cy="22" fill={sealed ? '#FF5A4D' : '#49E0A4'} r="3" />
                    </svg>
                    <div className={c.sealTxt}>
                        <b>{sealed ? t('sealedState') : seal ? t('unsealedState') : '…'}</b>
                        {sealLine}
                    </div>
                    {seal && total > 0 && <Pips n={sealed ? received : total} ok={!sealed} total={total} />}
                    {sealed && (
                        <button className={c.link} onClick={() => setUnsealOpen(true)} type="button">
                            {t('unseal.open')}
                        </button>
                    )}
                </div>
            </div>

            <UnsealModal
                onClose={() => setUnsealOpen(false)}
                onDone={() => {
                    setUnsealOpen(false)
                    void queryClient.invalidateQueries({ queryKey: ['sys', 'seal-status'] })
                }}
                opened={unsealOpen && sealed}
            />
        </div>
    )
}

/* ------------------------------------------------------------------ */
/* unseal modal: same endpoint and query as the Status page            */
/* ------------------------------------------------------------------ */
function UnsealModal({ onClose, onDone, opened }: { onClose: () => void; onDone: () => void; opened: boolean }) {
    const { t } = useTranslation('login')
    const queryClient = useQueryClient()
    const { data: live } = useSealStatus()
    const [shard, setShard] = useState('')
    const [reveal, setReveal] = useState(false)
    const [hint, setHint] = useState<Hint | null>(null)
    const [shake, setShake] = useState(0)
    const isShamir = live?.type === 'shamir'
    const required = live?.required_shards ?? 0
    const received = live?.received_shards ?? 0

    useEffect(() => {
        if (opened) {
            setShard('')
            setHint(null)
            setReveal(false)
        }
    }, [opened])

    const mutation = useMutation({
        mutationFn: unseal,
        onError: () => {
            setHint({ icon: 'warn', text: t('unseal.failed'), tone: 'err' })
            setShake((n) => n + 1)
        },
        onSuccess: (status) => {
            setShard('')
            queryClient.setQueryData(['sys', 'seal-status'], status)
            if (status.sealed) {
                const got = status.received_shards ?? 0
                const need = status.required_shards ?? 0
                setHint({
                    icon: 'check',
                    text: t('unseal.accepted', { left: Math.max(need - got, 0), received: got, total: need }),
                    tone: 'ok'
                })
            } else {
                setHint({ icon: 'check', text: t('unseal.done'), tone: 'ok' })
                window.setTimeout(onDone, 650)
            }
        }
    })

    const submit = (e: FormEvent) => {
        e.preventDefault()
        if (mutation.isPending) return
        if (isShamir && !shard.trim()) {
            setHint({ icon: 'warn', text: t('unseal.empty'), tone: 'err' })
            setShake((n) => n + 1)
            return
        }
        setHint(null)
        mutation.mutate(isShamir ? shard.trim() : '')
    }

    return (
        <ConsoleModal
            closeLabel={t('close')}
            labelledBy="login-unseal-title"
            onClose={onClose}
            opened={opened}
            tone="red"
            width={480}
        >
            <form
                autoComplete="off"
                className={`${layerStyles.mb} ${shake ? c.shake : ''}`}
                key={shake}
                noValidate
                onSubmit={submit}
            >
                <div className={c.eyebrow} style={{ color: '#ff5a4d' }}>
                    {t('unseal.eyebrow')}
                </div>
                <h3 className={layerStyles.title} id="login-unseal-title">
                    {isShamir ? t('unseal.title') : t('unseal.titleAuto')}
                </h3>
                <p className={layerStyles.text}>
                    {isShamir ? (
                        <>
                            {t('unseal.need')} <b>{t('unseal.needOf', { count: required })}</b> {t('unseal.needTail')}
                        </>
                    ) : (
                        t('unseal.autoHint')
                    )}
                </p>
                {required > 0 && (
                    <div style={{ marginBottom: 18 }}>
                        <Pips n={received} total={required} />
                    </div>
                )}
                {isShamir && (
                    <div className={c.field}>
                        <label htmlFor="login-shard">
                            {t('unseal.shard')}{' '}
                            <em>
                                {received} / {required}
                            </em>
                        </label>
                        <div className={c.input} data-state={hint?.tone === 'err' ? 'bad' : undefined}>
                            <input
                                autoComplete="off"
                                id="login-shard"
                                onChange={(e) => {
                                    setShard(e.currentTarget.value)
                                    setHint(null)
                                }}
                                placeholder={t('unseal.placeholder')}
                                spellCheck={false}
                                type={reveal ? 'text' : 'password'}
                                value={shard}
                            />
                            <button
                                aria-label={reveal ? t('hide') : t('show')}
                                className={c.ib}
                                onClick={() => setReveal((v) => !v)}
                                type="button"
                            >
                                <ShellIcon name={reveal ? 'eyeoff' : 'eye'} />
                            </button>
                        </div>
                    </div>
                )}
                <div className={c.hint} data-tone={hint?.tone} role="status">
                    {hint?.icon && <ShellIcon name={hint.icon} size={14} />}
                    {hint?.text}
                </div>
                <div className={layerStyles.acts}>
                    <button className={c.btn} onClick={onClose} type="button">
                        {t('cancel')}
                    </button>
                    <button className={`${c.btn} ${c.hot} ${mutation.isPending ? c.loading : ''}`} type="submit">
                        <span className={c.spinner} />
                        {isShamir ? t('unseal.submit') : t('unseal.submitAuto')}
                    </button>
                </div>
            </form>
        </ConsoleModal>
    )
}
