// SPDX-License-Identifier: AGPL-3.0-only
// Forked (structure) from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Outlet, useOutletContext } from 'react-router'

import { LanguageSwitcher } from '@shared/ui/language-switcher/language-switcher'
import { Logo } from '@shared/ui/logo/logo'
import c from '@shared/ui/shell-controls/console.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'

import styles from './auth.module.css'
import { ROUTES } from '@shared/constants/routes'

export type AuthTone = 'brass' | 'red'

interface AuthOutletContext {
    setTone: (tone: AuthTone) => void
}

/** Lets a logged-out screen switch the backdrop glow (brass / red alarm). */
export function useAuthTone(tone: AuthTone) {
    const ctx = useOutletContext<AuthOutletContext | undefined>()
    const setTone = ctx?.setTone
    useEffect(() => {
        setTone?.(tone)
        return () => setTone?.('brass')
    }, [setTone, tone])
}

export function AuthLayout() {
    const { t } = useTranslation('shell')
    const [tone, setTone] = useState<AuthTone>('brass')

    return (
        <div className={`${c.view} ${styles.root}`}>
            <div className={c.backdrop} data-tone={tone === 'red' ? 'red' : undefined} />
            <header className={styles.topbar}>
                <div className={`${c.wrap} ${styles.topRow}`}>
                    <Logo to={ROUTES.AUTH.LOGIN} />
                    <span className={styles.sp} />
                    <LanguageSwitcher variant="chip" />
                    <a
                        className={c.chip}
                        href="https://github.com/NAGenaev/tuck"
                        rel="noopener noreferrer"
                        target="_blank"
                        title={t('source')}
                    >
                        <ShellIcon name="gh" size={15} />
                        GitHub
                    </a>
                </div>
            </header>
            <Outlet context={{ setTone } satisfies AuthOutletContext} />
        </div>
    )
}
