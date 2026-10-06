// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { wrap } from '@shared/api/endpoints/wrapping'
import { checkJson, CodeField, prefersReducedMotion, toast } from '@shared/ui/kit'

import { byteLength, parseTtl, problemOf, short, TEMPLATES, TemplateKey } from '../lib'
import styles from '../wrapping.module.css'
import { WrapCtx } from './ctx'
import { WIcon } from './icons'
import { Msg, ProblemAlert, TokBox } from './parts'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')
const TTL_CHIPS = ['1m', '5m', '1h', '24h']
const SEAL_MS = 1500

export function WrapPanel({ ctx, hidden }: { ctx: WrapCtx; hidden: boolean }) {
    const { t, i18n } = useTranslation('wrapping')
    const { upsert, setCur } = ctx.session
    const [payload, setPayload] = useState('')
    const [ttl, setTtl] = useState('5m')
    const [path, setPath] = useState('')
    const [jsonErr, setJsonErr] = useState('')
    const [ttlErr, setTtlErr] = useState('')
    const [jsonShake, setJsonShake] = useState(0)
    const [ttlShake, setTtlShake] = useState(0)
    const [result, setResult] = useState<null | { expires: number; token: string; n: number }>(null)

    const mutation = useMutation({
        mutationFn: async (vars: { data: unknown; path: string; ttl: string }) => {
            // hold the sealing animation long enough to read, the request itself is instant
            const hold = prefersReducedMotion() ? 0 : SEAL_MS
            const started = Date.now()
            const res = await wrap(vars.data, vars.ttl, vars.path)
            await new Promise((r) => window.setTimeout(r, Math.max(0, hold - (Date.now() - started))))
            return res
        },
        onError: (err) => {
            ctx.seal(false)
            const p = problemOf(err)
            ctx.setProblem('wrap', p)
            toast.error(t('toast.error'), t('toast.wrapFailed'))
        },
        onSuccess: (res, vars) => {
            const created = Date.now()
            const expires = Date.parse(res.expires_at)
            upsert(res.token, {
                created,
                expires: Number.isNaN(expires) ? created + (parseTtl(vars.ttl) ?? 0) * 1000 : expires,
                status: 'active',
                ttl: parseTtl(vars.ttl) ?? undefined
            })
            setCur(res.token)
            ctx.seal(false)
            setResult({ expires: Number.isNaN(expires) ? created : expires, n: created, token: res.token })
            toast.success(t('toast.wrapped'), t('toast.wrappedMsg', { ttl: vars.ttl.trim() }))
        }
    })

    const submit = () => {
        if (mutation.isPending) return
        let data: unknown
        try {
            data = JSON.parse(payload.trim() || '{}')
        } catch (err) {
            setJsonErr(t('wrap.errJson', { message: checkJson(payload).message ?? (err as Error).message }))
            setJsonShake((n) => n + 1)
            toast.error(t('toast.error'), t('toast.wrapFailed'))
            return
        }
        if (parseTtl(ttl) === null) {
            setTtlErr(t('wrap.errTtl'))
            setTtlShake((n) => n + 1)
            return
        }
        ctx.clearProblem('wrap')
        setResult(null)
        ctx.seal(true)
        mutation.mutate({ data, path: path.trim(), ttl: ttl.trim() })
    }

    const pick = (k: TemplateKey) => {
        setPayload(JSON.stringify(TEMPLATES[k], null, 2))
        setJsonErr('')
    }

    return (
        <div aria-labelledby="tab-wrap" className={styles.panel} hidden={hidden} id="p-wrap" role="tabpanel">
            {ctx.problems.wrap && <ProblemAlert op="wrap" problem={ctx.problems.wrap} />}

            <div className={styles.field}>
                <div className={styles.lab}>
                    {t('wrap.payload')} <em>{byteLength(payload)} {t('bytes')}</em>
                </div>
                <div className={jsonShake ? styles.shake : undefined} key={`j${jsonShake}`}>
                    <CodeField
                        language="json"
                        highlight
                        minRows={8}
                        onChange={(v) => {
                            setPayload(v)
                            setJsonErr('')
                        }}
                        placeholder={t('wrap.payloadPlaceholder')}
                        value={payload}
                    />
                </div>
                <div aria-label={t('wrap.templates')} className={styles.chips}>
                    <span className={styles.lab} style={{ alignSelf: 'center', marginRight: 4 }}>
                        {t('wrap.templates')}
                    </span>
                    {(['db', 'api', 'otp'] as const).map((k) => (
                        <button className={styles.chip} key={k} onClick={() => pick(k)} type="button">
                            {t(`wrap.ex.${k}`)}
                        </button>
                    ))}
                </div>
                <Msg>{jsonErr}</Msg>
            </div>

            <div className={styles.field}>
                <label className={styles.lab} htmlFor="wrap-ttl">
                    TTL <em>{t('wrap.ttlHint')}</em>
                </label>
                <div className={ttlShake ? styles.shake : undefined} key={`t${ttlShake}`}>
                    <input
                        autoComplete="off"
                        className={cx(styles.inp, ttlErr && styles.bad)}
                        id="wrap-ttl"
                        inputMode="text"
                        onChange={(e) => {
                            setTtl(e.currentTarget.value)
                            setTtlErr('')
                        }}
                        onKeyDown={(e) => e.key === 'Enter' && submit()}
                        value={ttl}
                    />
                </div>
                <div className={styles.chips}>
                    {TTL_CHIPS.map((v) => (
                        <button
                            aria-pressed={ttl.trim() === v}
                            className={styles.chip}
                            key={v}
                            onClick={() => {
                                setTtl(v)
                                setTtlErr('')
                            }}
                            type="button"
                        >
                            {v}
                        </button>
                    ))}
                </div>
                <Msg>{ttlErr}</Msg>
            </div>

            <div className={styles.field}>
                <label className={styles.lab} htmlFor="wrap-path">
                    {t('wrap.path')} <em>{t('wrap.pathHint')}</em>
                </label>
                <input
                    autoComplete="off"
                    className={styles.inp}
                    id="wrap-path"
                    maxLength={256}
                    onChange={(e) => setPath(e.currentTarget.value)}
                    onKeyDown={(e) => e.key === 'Enter' && submit()}
                    placeholder="secret/db/prod"
                    value={path}
                />
            </div>

            <div className={styles.row}>
                <Button
                    leftSection={<WIcon name="lock" />}
                    loading={mutation.isPending}
                    onClick={submit}
                    size="md"
                >
                    {t('wrap.submit')}
                </Button>
            </div>

            {result && (
                <div className={styles.result} key={result.n}>
                    <h4>
                        <i />
                        {t('wrap.resultTitle')}
                    </h4>
                    <TokBox
                        onCopied={() => {
                            const c = ctx.session.capsules[result.token]
                            if (c && !c.delivered) {
                                ctx.session.upsert(result.token, { delivered: Date.now() })
                                toast.info(t('toast.delivered'), t('toast.deliveredMsg', { token: short(result.token) }))
                            }
                        }}
                        value={result.token}
                    />
                    <div className={styles.monoSmall}>
                        {t('wrap.expires', { date: new Date(result.expires).toLocaleString(i18n.language) })}
                    </div>
                    <div className={styles.warn}>{t('wrap.warn')}</div>
                    <div className={styles.row}>
                        <Button onClick={() => ctx.goto('look', result.token)} size="xs" variant="default">
                            {t('tabs.look')}
                        </Button>
                        <Button onClick={() => ctx.goto('unwrap', result.token)} size="xs" variant="default">
                            {t('tabs.unwrap')}
                        </Button>
                    </div>
                </div>
            )}
        </div>
    )
}
