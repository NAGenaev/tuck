// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { unwrap } from '@shared/api/endpoints/wrapping'
import { ConfirmModal, Icon, toast, useCopy } from '@shared/ui/kit'

import { problemOf } from '../lib'
import styles from '../wrapping.module.css'
import { WrapCtx } from './ctx'
import { WIcon } from './icons'
import { MaskedJson, Msg, ProblemAlert, useAutoMask } from './parts'
import { SessionChips } from './session-chips'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

export function UnwrapPanel({ ctx, hidden }: { ctx: WrapCtx; hidden: boolean }) {
    const { t } = useTranslation('wrapping')
    const { addLog, setCur, upsert } = ctx.session
    const token = ctx.tokens.unwrap.trim()
    const [msg, setMsg] = useState('')
    const [shake, setShake] = useState(0)
    const [confirm, setConfirm] = useState(false)
    const [data, setData] = useState<null | { n: number; text: string }>(null)
    const [revealed, setRevealed] = useState(false)
    const { copied, copy } = useCopy({ toast: false })

    useAutoMask(revealed, () => setRevealed(false))

    const mutation = useMutation({
        mutationFn: (tok: string) => unwrap(tok),
        onError: (err, tok) => {
            const p = problemOf(err)
            setData(null)
            ctx.setProblem('unwrap', p)
            ctx.failed(tok, p.status)
            setCur(tok)
            addLog('unwrap', tok, false, p.status === 410 ? t('log.expired') : p.status === 404 ? t('log.gone') : String(p.status ?? 'net'))
            ctx.shake()
            toast.error(t('toast.error'), t('toast.unwrapFailed'))
        },
        onSuccess: (res, tok) => {
            const now = Date.now()
            upsert(tok, { delivered: ctx.session.capsules[tok]?.delivered ?? now, status: 'used', usedAt: now })
            setCur(tok)
            ctx.clearProblem('unwrap')
            addLog('unwrap', tok, true, t('log.unwrapped'))
            setRevealed(false)
            setData({ n: now, text: JSON.stringify(res ?? null, null, 2) })
            ctx.playUnwrap()
            toast.success(t('toast.unwrapped'), t('toast.unwrappedMsg'))
        }
    })

    const ask = () => {
        if (!token) {
            setMsg(t('unwrap.errEmpty'))
            setShake((n) => n + 1)
            return
        }
        if (mutation.isPending) return
        setConfirm(true)
    }

    const run = (tok: string) => {
        ctx.clearProblem('unwrap')
        setData(null)
        mutation.mutate(tok)
    }

    return (
        <div aria-labelledby="tab-unwrap" className={styles.panel} hidden={hidden} id="p-unwrap" role="tabpanel">
            <div className={styles.field}>
                <label className={styles.lab} htmlFor="unwrap-token">
                    {t('unwrap.token')} <em>{t('unwrap.hint')}</em>
                </label>
                <div className={shake ? styles.shake : undefined} key={shake}>
                    <input
                        autoComplete="off"
                        className={cx(styles.inp, msg && styles.bad)}
                        id="unwrap-token"
                        onChange={(e) => {
                            ctx.setToken('unwrap', e.currentTarget.value)
                            setMsg('')
                        }}
                        onKeyDown={(e) => e.key === 'Enter' && ask()}
                        placeholder="tuck_wrap_…"
                        spellCheck={false}
                        value={ctx.tokens.unwrap}
                    />
                </div>
                <SessionChips
                    onPick={(tok) => {
                        ctx.setToken('unwrap', tok)
                        setMsg('')
                    }}
                    session={ctx.session}
                />
                <Msg>{msg}</Msg>
            </div>

            <div className={styles.row}>
                <Button
                    leftSection={<WIcon name="unwrap" />}
                    loading={mutation.isPending}
                    onClick={ask}
                    size="md"
                >
                    {t('unwrap.submit')}
                </Button>
            </div>

            {ctx.problems.unwrap && <ProblemAlert op="unwrap" problem={ctx.problems.unwrap} />}

            {data && (
                <div className={styles.result} key={data.n}>
                    <h4>
                        <i />
                        {t('unwrap.resultTitle')}
                    </h4>
                    <div className={styles.jsonBar}>
                        <span className={styles.lab}>{t('unwrap.payload')}</span>
                        <div className={styles.row} style={{ gap: 8 }}>
                            <Button
                                leftSection={<Icon name={revealed ? 'eyeoff' : 'eye'} size={14} />}
                                onClick={() => setRevealed((v) => !v)}
                                size="xs"
                                variant="default"
                            >
                                {revealed ? t('unwrap.hide') : t('unwrap.show')}
                            </Button>
                            <Button
                                leftSection={<Icon name={copied ? 'check' : 'copy'} size={14} />}
                                onClick={() => void copy(data.text)}
                                size="xs"
                                variant="default"
                            >
                                {copied ? t('copied') : t('copy')}
                            </Button>
                        </div>
                    </div>
                    <MaskedJson revealed={revealed} text={data.text} />
                    <div className={styles.warn}>{t('unwrap.warn')}</div>
                    <div className={styles.row}>
                        <Button color="red" onClick={() => run(token)} size="xs" variant="default">
                            {t('unwrap.again')}
                        </Button>
                        <Button onClick={() => setData(null)} size="xs" variant="default">
                            {t('unwrap.clear')}
                        </Button>
                    </div>
                </div>
            )}

            <ConfirmModal
                confirmLabel={t('unwrap.submit')}
                destructive={false}
                message={t('unwrap.confirmText')}
                onClose={() => setConfirm(false)}
                onConfirm={() => {
                    setConfirm(false)
                    run(token)
                }}
                opened={confirm}
                title={t('unwrap.confirmTitle')}
            />
        </div>
    )
}
