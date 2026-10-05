// SPDX-License-Identifier: AGPL-3.0-only
import { useFullscreenDocument } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { lookupSelf } from '@shared/api/endpoints/auth'
import { useHealth } from '@shared/api/hooks/use-health'
import { useSealStatus } from '@shared/api/hooks/use-seal-status'
import { ROUTES } from '@shared/constants/routes'
import { useLogout } from '@shared/hooks/use-auth'
import { LanguageSwitcher } from '@shared/ui/language-switcher/language-switcher'
import { NamespaceSwitcher } from '@shared/ui/namespace-switcher/namespace-switcher'
import c from '@shared/ui/shell-controls/console.module.css'
import ctl from '@shared/ui/shell-controls/controls.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'
import { ConsoleModal, layerStyles } from '@shared/ui/shell-controls/layer'

import styles from './shell.module.css'

const TUCK_REPO_URL = 'https://github.com/NAGenaev/tuck'
const LOW_TTL_MS = 5 * 60_000

/** Absolute expiry of the current token from lookup-self, or null if it never expires / is unknown. */
export function tokenExpiryMs(token: undefined | { expires_at?: string; ttl?: number }, fetchedAt: number): null | number {
    if (!token) return null
    if (token.expires_at) {
        const ms = new Date(token.expires_at).getTime()
        // Go zero time (0001-01-01) means "no expiry"
        if (!Number.isNaN(ms) && new Date(ms).getFullYear() >= 2000) return ms
    }
    if (typeof token.ttl === 'number' && token.ttl > 0) return fetchedAt + token.ttl * 1000
    return null
}

function formatRemaining(ms: number): string {
    const s = Math.max(Math.floor(ms / 1000), 0)
    const d = Math.floor(s / 86400)
    const h = Math.floor((s % 86400) / 3600)
    const m = Math.floor((s % 3600) / 60)
    const pad = (n: number) => String(n).padStart(2, '0')
    if (d > 0) return `${d}d ${h}h`
    if (h > 0) return `${h}:${pad(m)}:${pad(s % 60)}`
    return `${pad(m)}:${pad(s % 60)}`
}

/** Session countdown; rendered only when lookup-self reports a real expiry. */
function SessionTtl() {
    const { t } = useTranslation('shell')
    const { data, dataUpdatedAt } = useQuery({
        queryKey: ['auth', 'lookup-self'],
        queryFn: lookupSelf,
        refetchInterval: 60_000,
        staleTime: 60_000
    })
    const expiry = tokenExpiryMs(data, dataUpdatedAt)
    const [now, setNow] = useState(() => Date.now())

    useEffect(() => {
        if (expiry === null) return
        const id = window.setInterval(() => setNow(Date.now()), 1000)
        return () => window.clearInterval(id)
    }, [expiry])

    if (expiry === null) return null
    const left = expiry - now
    return (
        <span
            className={`${ctl.ctrl} ${ctl.ttl} ${ctl.tip} ${ctl.hideM}`}
            data-low={left < LOW_TTL_MS}
            data-tip={t('sessionTtl')}
            role="timer"
            tabIndex={0}
        >
            <ShellIcon name="clock" size={15} />
            {formatRemaining(left)}
        </span>
    )
}

export function HeaderControls() {
    const { t } = useTranslation('shell')
    const { data: sealStatus } = useSealStatus()
    const { data: health } = useHealth()
    const { fullscreen, toggle } = useFullscreenDocument()
    const logout = useLogout()
    const [confirmOpen, setConfirmOpen] = useState(false)

    const sealed = sealStatus?.sealed === true
    const version = health?.version

    return (
        <div className={ctl.ctrls}>
            <span
                className={`${ctl.ctrl} ${ctl.ver} ${ctl.tip} ${ctl.hideM}`}
                data-tip={t('version', { version: version ?? '—' })}
                tabIndex={0}
            >
                <ShellIcon name="tag" size={16} />
                {version ? (version.startsWith('v') ? version : `v${version}`) : '—'}
            </span>

            <SessionTtl />

            <Link
                aria-label={sealed ? t('sealed') : t('unsealed')}
                className={`${ctl.ctrl} ${ctl.sealInd} ${ctl.tip}`}
                data-sealed={sealed}
                data-tip={sealStatus ? (sealed ? t('sealed') : t('unsealed')) : t('sealUnknown')}
                to={ROUTES.DASHBOARD.STATUS}
            >
                <ShellIcon name={sealed ? 'lock' : 'unlock'} />
            </Link>

            <button
                aria-label={fullscreen ? t('exitFullscreen') : t('fullscreen')}
                className={`${ctl.ctrl} ${ctl.tip} ${ctl.hideM}`}
                data-tip={fullscreen ? t('exitFullscreen') : t('fullscreen')}
                onClick={() => void toggle()}
                type="button"
            >
                <ShellIcon name={fullscreen ? 'min' : 'max'} />
            </button>

            <NamespaceSwitcher />
            <LanguageSwitcher variant="menu" />

            <a
                aria-label="GitHub"
                className={`${ctl.ctrl} ${ctl.tip} ${ctl.hideM}`}
                data-tip={t('source')}
                href={TUCK_REPO_URL}
                rel="noopener noreferrer"
                target="_blank"
            >
                <ShellIcon name="gh" />
            </a>

            <span className={styles.sep} />

            <button
                aria-label={t('logout')}
                className={`${ctl.ctrl} ${ctl.red} ${ctl.tip}`}
                data-tip={t('logout')}
                onClick={() => setConfirmOpen(true)}
                type="button"
            >
                <ShellIcon name="out" />
            </button>

            <ConsoleModal
                closeLabel={t('close')}
                labelledBy="logout-title"
                onClose={() => setConfirmOpen(false)}
                opened={confirmOpen}
                width={440}
            >
                <div className={layerStyles.mb}>
                    <div className={c.eyebrow} style={{ color: '#ff5a4d' }}>
                        {t('logoutEyebrow')}
                    </div>
                    <h3 className={layerStyles.title} id="logout-title">
                        {t('logoutTitle')}
                    </h3>
                    <p className={layerStyles.text}>{t('logoutText')}</p>
                    <div className={layerStyles.acts}>
                        <button className={c.btn} onClick={() => setConfirmOpen(false)} type="button">
                            {t('cancel')}
                        </button>
                        <button
                            className={`${c.btn} ${c.danger}`}
                            onClick={() => {
                                setConfirmOpen(false)
                                logout()
                            }}
                            type="button"
                        >
                            {t('logout')}
                        </button>
                    </div>
                </div>
            </ConsoleModal>
        </div>
    )
}
