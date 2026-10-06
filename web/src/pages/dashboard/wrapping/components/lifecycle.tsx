// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'

import { Capsule, fmtTime, Status } from '../lib'
import styles from '../wrapping.module.css'
import { WIcon, WrapIconName } from './icons'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

/** Created, Delivered, then Unwrapped or Expired/Revoked: the two final nodes are mutually exclusive. */
export function Lifecycle({ capsule, status }: { capsule?: Capsule; status?: Status }) {
    const { t } = useTranslation('wrapping')
    const done = [false, false, false, false]
    const now = [false, false, false, false]
    const small = ['', '', '', '']
    let f = 0
    let tone: '' | 'bad' | 'ok' = ''

    if (capsule && status && status !== 'missing') {
        done[0] = true
        small[0] = capsule.created ? fmtTime(capsule.created) : ''
        if (capsule.delivered) {
            done[1] = true
            small[1] = fmtTime(capsule.delivered)
        }
        if (status === 'active') {
            f = capsule.delivered ? 0.333 : 0
            const at = capsule.delivered ? 1 : 0
            now[at] = true
            done[at] = true
            if (capsule.expires) small[3] = t('life.until', { time: fmtTime(capsule.expires) })
        } else if (status === 'used') {
            f = 0.667
            done[1] = true
            done[2] = now[2] = true
            small[2] = capsule.usedAt ? fmtTime(capsule.usedAt) : ''
            tone = 'ok'
        } else {
            f = 1
            done[1] = true
            done[3] = now[3] = true
            const at = status === 'revoked' ? capsule.revokedAt : capsule.expires
            small[3] = at ? fmtTime(at) : ''
            tone = 'bad'
        }
    }

    const revoked = status === 'revoked'
    const nodes: Array<{ end?: 'bad' | 'ok'; icon: WrapIconName; label: string }> = [
        { icon: 'lock', label: t('life.created') },
        { icon: 'send', label: t('life.delivered') },
        { end: 'ok', icon: 'check', label: t('life.unwrapped') },
        { end: 'bad', icon: revoked ? 'x' : 'clock', label: revoked ? t('life.revoked') : t('life.expired') }
    ]

    return (
        <div
            className={cx(styles.life, tone === 'bad' && styles.lifeBad, tone === 'ok' && styles.lifeOk)}
            style={{ '--f': f } as CSSProperties}
        >
            <div className={styles.lifeFill} />
            {nodes.map((n, i) => (
                <div
                    className={cx(
                        styles.node,
                        done[i] && styles.nodeDone,
                        now[i] && styles.nodeNow,
                        n.end === 'ok' && styles.endOk,
                        n.end === 'bad' && styles.endBad
                    )}
                    key={i}
                >
                    <div className={styles.nodeDot}>
                        <WIcon name={n.icon} size={18} />
                    </div>
                    <b>{n.label}</b>
                    <small>{small[i]}</small>
                </div>
            ))}
        </div>
    )
}
