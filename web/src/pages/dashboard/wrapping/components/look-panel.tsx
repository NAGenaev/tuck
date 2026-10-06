// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { lookupWrap, revokeWrap, WrapLookup } from '@shared/api/endpoints/wrapping'
import { ConfirmModal, Icon, toast } from '@shared/ui/kit'

import { fmtLeft, OP_PATH, problemOf, short } from '../lib'
import styles from '../wrapping.module.css'
import { WrapCtx } from './ctx'
import { WIcon } from './icons'
import { Msg, ProblemAlert } from './parts'
import { SessionChips } from './session-chips'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

export function LookPanel({ ctx, hidden }: { ctx: WrapCtx; hidden: boolean }) {
    const { t, i18n } = useTranslation('wrapping')
    const { addLog, now, setCur, upsert } = ctx.session
    const token = ctx.tokens.look.trim()
    const [msg, setMsg] = useState('')
    const [shake, setShake] = useState(0)
    const [info, setInfo] = useState<(WrapLookup & { token: string }) | null>(null)
    const [confirm, setConfirm] = useState(false)

    const fail = (text: string) => {
        setMsg(text)
        setShake((n) => n + 1)
    }

    const lookup = useMutation({
        mutationFn: (tok: string) => lookupWrap(tok),
        onError: (err, tok) => {
            const p = problemOf(err)
            setInfo(null)
            ctx.setProblem('lookup', p)
            ctx.failed(tok, p.status)
            setCur(tok)
            addLog('lookup', tok, false, p.status === 410 ? t('log.expired') : String(p.status ?? 'net'))
            ctx.shake()
            toast.error(t('toast.error'), t('toast.lookupFailed'))
        },
        onSuccess: (res, tok) => {
            const expires = Date.parse(res.expires_at)
            upsert(tok, {
                created: Date.parse(res.creation_time),
                expires,
                status: 'active',
                ttl: res.creation_ttl
            })
            setCur(tok)
            ctx.clearProblem('lookup')
            addLog('lookup', tok, true, t('log.found'))
            setInfo({ ...res, token: tok })
        }
    })

    const submit = () => {
        if (!token) return fail(t('look.errEmpty'))
        if (lookup.isPending) return
        ctx.clearProblem('lookup')
        ctx.clearProblem('revoke')
        setInfo(null)
        lookup.mutate(token)
    }

    const revoke = async () => {
        ctx.clearProblem('revoke')
        try {
            await revokeWrap(token)
        } catch (err) {
            const p = problemOf(err)
            ctx.setProblem('revoke', p)
            ctx.failed(token, p.status)
            setCur(token)
            addLog('revoke', token, false, String(p.status ?? 'net'))
            toast.error(t('toast.error'), t('toast.revokeFailed'))
            setConfirm(false)
            return
        }
        upsert(token, { revokedAt: Date.now(), status: 'revoked' })
        setCur(token)
        addLog('revoke', token, true, t('log.revoked'))
        setInfo(null)
        toast.success(t('toast.revoked'), t('toast.revokedMsg'))
    }

    const expires = info ? Date.parse(info.expires_at) : 0

    return (
        <div aria-labelledby="tab-look" className={styles.panel} hidden={hidden} id="p-look" role="tabpanel">
            <div className={styles.field}>
                <label className={styles.lab} htmlFor="look-token">
                    {t('look.token')} <em>{t('look.hint')}</em>
                </label>
                <div className={shake ? styles.shake : undefined} key={shake}>
                    <input
                        autoComplete="off"
                        className={cx(styles.inp, msg && styles.bad)}
                        id="look-token"
                        onChange={(e) => {
                            ctx.setToken('look', e.currentTarget.value)
                            setMsg('')
                        }}
                        onKeyDown={(e) => e.key === 'Enter' && submit()}
                        placeholder="tuck_wrap_…"
                        spellCheck={false}
                        value={ctx.tokens.look}
                    />
                </div>
                <SessionChips
                    onPick={(tok) => {
                        ctx.setToken('look', tok)
                        setMsg('')
                    }}
                    session={ctx.session}
                />
                <Msg>{msg}</Msg>
            </div>

            <div className={styles.row}>
                <Button
                    leftSection={<WIcon name="look" />}
                    loading={lookup.isPending}
                    onClick={submit}
                    size="md"
                >
                    {t('look.find')}
                </Button>
                <Button
                    color="red"
                    leftSection={<Icon name="x" size={16} />}
                    onClick={() => (token ? setConfirm(true) : fail(t('look.errRevoke')))}
                    size="md"
                    variant="default"
                >
                    {t('look.revoke')}
                </Button>
            </div>

            {ctx.problems.lookup && <ProblemAlert op="lookup" problem={ctx.problems.lookup} />}
            {ctx.problems.revoke && <ProblemAlert op="revoke" problem={ctx.problems.revoke} />}

            {info && (
                <dl className={cx(styles.dl, styles.rise)}>
                    <div>
                        <dt>{t('look.path')}</dt>
                        <dd>{OP_PATH.wrap}</dd>
                    </div>
                    <div>
                        <dt>{t('meta.created')}</dt>
                        <dd>{new Date(info.creation_time).toLocaleString(i18n.language)}</dd>
                    </div>
                    <div>
                        <dt>{t('look.expires')}</dt>
                        <dd>{new Date(expires).toLocaleString(i18n.language)}</dd>
                    </div>
                    <div>
                        <dt>{t('look.ttl')}</dt>
                        <dd>{t('look.ttlValue', { ttl: info.creation_ttl })}</dd>
                    </div>
                    <div>
                        <dt>{t('meta.left')}</dt>
                        <dd className={styles.brassL}>{fmtLeft(t, expires - now)}</dd>
                    </div>
                </dl>
            )}

            <ConfirmModal
                confirmLabel={t('look.revoke')}
                message={t('look.revokeText', { token: short(token) })}
                onClose={() => setConfirm(false)}
                onConfirm={revoke}
                opened={confirm}
                title={t('look.revokeTitle')}
            />
        </div>
    )
}
