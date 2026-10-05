// SPDX-License-Identifier: AGPL-3.0-only
// One-time reveal of a freshly minted token. The value exists only in this component's props:
// the server never returns it again, so it is dropped as soon as the plate is dismissed.
import { Button } from '@mantine/core'
import { useEffect, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { copyText, Icon, prefersReducedMotion, toast } from '@shared/ui/kit'
import { Eyebrow, Plate } from '@shared/ui/tuck/tuck'

import type { TokenInfo } from './tokens.api'
import s from './tokens.module.css'
import { fmtTotal, toRow } from './tokens.utils'

const GLYPHS = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz0123456789#$%&*+=?@'
/** seconds the value stays on screen before it is hidden automatically */
const LIFE = 120

const scramble = (n: number) => {
    let out = ''
    for (let i = 0; i < n; i++) out += GLYPHS[Math.floor(Math.random() * GLYPHS.length)]
    return out
}

function Body({ token, live, onDismiss }: { live: boolean; onDismiss: () => void; token: TokenInfo }) {
    const { t } = useTranslation('tokens')
    const real = token.id
    const row = toRow(real, token, false)
    const instant = prefersReducedMotion()
    const [blur, setBlur] = useState(false)
    const [copied, setCopied] = useState(false)
    const [done, setDone] = useState(instant)
    const [typed, setTyped] = useState(instant ? real.length : 0)
    const [junk, setJunk] = useState('')
    const [left, setLeft] = useState(LIFE)

    // decrypt animation: characters resolve left to right
    useEffect(() => {
        if (instant) return
        const start = performance.now()
        let raf = 0
        const step = (now: number) => {
            const p = Math.min(1, (now - start) / 900)
            const k = Math.floor(p * real.length)
            setTyped(k)
            if (p < 1) {
                setJunk(scramble(real.length - k))
                raf = requestAnimationFrame(step)
            } else {
                setTyped(real.length)
                setJunk('')
                setDone(true)
            }
        }
        raf = requestAnimationFrame(step)
        return () => cancelAnimationFrame(raf)
    }, [real, instant])

    useEffect(() => {
        if (!live) return
        const id = window.setInterval(() => setLeft((l) => l - 1), 1000)
        return () => window.clearInterval(id)
    }, [live])

    useEffect(() => {
        if (live && left <= 0) {
            toast.warning(t('toast.hidden'), t('toast.hiddenText'))
            onDismiss()
        }
    }, [left, live, onDismiss, t])

    const copy = async () => {
        if (await copyText(real)) {
            setCopied(true)
            toast.success(t('toast.copied'), t('toast.copiedText'))
            window.setTimeout(() => setCopied(false), 2200)
        } else {
            toast.error(t('reveal.copyManual'))
        }
    }

    const meta: Array<[string, string]> = [
        [t('reveal.metaName'), row.name || '—'],
        [t('reveal.metaAcc'), row.accessor || '—'],
        [t('reveal.metaTtl'), fmtTotal(row.total, t('time.d'), t('time.h'), t('time.m'))],
        [t('reveal.metaNs'), row.ns],
        [t('reveal.metaPol'), row.policies.join(', ') || '—']
    ]

    return (
        <Plate className={s.reveal} variant="brass">
            <div className={s.revealBody}>
                <div className={s.rvHead}>
                    <Eyebrow className={s.rvEyebrow}>{t('reveal.eyebrow')}</Eyebrow>
                    <div className={s.rvTools}>
                        <button
                            aria-label={blur ? t('reveal.show') : t('reveal.hide')}
                            aria-pressed={blur}
                            className={s.iconBtn}
                            onClick={() => setBlur((b) => !b)}
                            title={blur ? t('reveal.show') : t('reveal.hide')}
                            type="button"
                        >
                            <Icon name={blur ? 'eye' : 'eyeoff'} size={15} />
                        </button>
                        <button
                            aria-label={t('reveal.close')}
                            className={s.iconBtn}
                            onClick={onDismiss}
                            title={t('reveal.close')}
                            type="button"
                        >
                            <Icon name="x" size={15} />
                        </button>
                    </div>
                </div>

                <h2 className={s.rvTitle}>
                    <Trans components={{ em: <em /> }} i18nKey="reveal.title" t={t} />
                </h2>

                <div
                    aria-label={t('reveal.tokenLabel')}
                    className={s.tokenbox}
                    data-blur={blur}
                    data-done={done}
                    role="group"
                >
                    <span>{real.slice(0, typed)}</span>
                    <span aria-hidden className={s.junk}>
                        {junk}
                    </span>
                    {!done && <span aria-hidden className={s.cur} />}
                </div>

                <div className={s.rvActions}>
                    <Button
                        className={s.bigBtn}
                        data-copied={copied}
                        leftSection={<Icon name={copied ? 'check' : 'copy'} size={16} />}
                        onClick={copy}
                        size="lg"
                    >
                        {copied ? t('reveal.copiedBtn') : t('reveal.copy')}
                    </Button>
                </div>

                <div className={s.warnRow}>
                    <div>
                        <span>{t('reveal.warn')}</span>
                        <span>{t('reveal.left', { s: Math.max(0, left) })}</span>
                    </div>
                    <div aria-hidden className={s.bar}>
                        <i style={{ width: `${Math.max(0, left / LIFE) * 100}%` }} />
                    </div>
                </div>

                <div className={s.rvMeta}>
                    {meta.map(([k, v]) => (
                        <span key={k}>
                            {k} <b>{v}</b>
                        </span>
                    ))}
                </div>
            </div>
        </Plate>
    )
}

/**
 * Collapsible wrapper: expands when `token` is set, collapses (and forgets the value) when it is
 * cleared. `onDismiss` must be stable.
 */
export function RevealPlate({ token, onDismiss }: { onDismiss: () => void; token: null | TokenInfo }) {
    const [shown, setShown] = useState<null | TokenInfo>(null)
    const [on, setOn] = useState(false)

    useEffect(() => {
        if (token) {
            setShown(token)
            const id = window.setTimeout(() => setOn(true), 40)
            return () => window.clearTimeout(id)
        }
        setOn(false)
        const id = window.setTimeout(() => setShown(null), prefersReducedMotion() ? 0 : 750)
        return () => window.clearTimeout(id)
    }, [token])

    if (!shown) return null
    return (
        <div aria-hidden={!on} className={s.revealWrap} data-on={on} id="tk-reveal" inert={!on}>
            <div>
                <Body key={shown.id} live={!!token} onDismiss={onDismiss} token={shown} />
            </div>
        </div>
    )
}
