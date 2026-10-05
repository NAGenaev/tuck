// SPDX-License-Identifier: AGPL-3.0-only
// 404 screen ("Консоль Tuck" design). Standalone: it brings its own backdrop and topbar so it can be
// mounted as a catch-all route outside both the auth layout and the dashboard shell.
import { CSSProperties, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router'

import authStyles from '@app/layouts/auth/auth.module.css'
import { useDesktopMenuSections } from '@app/layouts/dashboard/main-layout/menu-sections/desktop-menu-sections'
import { ROUTES } from '@shared/constants/routes'
import { useAuth } from '@shared/hooks/use-auth'
import { LanguageSwitcher } from '@shared/ui/language-switcher/language-switcher'
import { Logo } from '@shared/ui/logo/logo'
import c from '@shared/ui/shell-controls/console.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'

import styles from './not-found.module.css'

const delay = (n: number) => ({ '--d': n }) as CSSProperties

export function NotFoundPage() {
    const { t } = useTranslation('shell')
    const { pathname, search } = useLocation()
    const { isAuthenticated } = useAuth()
    const menu = useDesktopMenuSections()
    const [copied, setCopied] = useState(false)
    const timer = useRef<number | undefined>(undefined)

    useEffect(() => () => window.clearTimeout(timer.current), [])

    // real routes of the console, offered as "maybe you meant"
    const suggestions = [
        'status',
        'kv2',
        'tokens',
        'audit-sinks'
    ]
        .map((id) => menu.flatMap((g) => g.section).find((item) => item.id === id))
        .filter((item): item is NonNullable<typeof item> => item !== undefined)

    const home = isAuthenticated ? ROUTES.DASHBOARD.STATUS : ROUTES.AUTH.LOGIN
    const requested = pathname + search

    const copy = async () => {
        try {
            await navigator.clipboard.writeText(window.location.href)
            setCopied(true)
            window.clearTimeout(timer.current)
            timer.current = window.setTimeout(() => setCopied(false), 1800)
        } catch {
            /* clipboard unavailable: nothing to do */
        }
    }

    return (
        <div className={`${c.view} ${authStyles.root}`}>
            <div className={c.backdrop} />
            <header className={authStyles.topbar}>
                <div className={`${c.wrap} ${authStyles.topRow}`}>
                    <Logo to={home} />
                    <span className={authStyles.sp} />
                    <LanguageSwitcher variant="chip" />
                    <Link className={c.chip} to={home}>
                        {isAuthenticated ? t('notFound.toConsole') : t('notFound.toLogin')}
                    </Link>
                </div>
            </header>

            <div className={`${c.wrap} ${c.errGrid}`}>
                <div>
                    <div className={`${c.badge} ${c.rv}`} style={delay(0)}>
                        <i />
                        {t('notFound.badge')}
                    </div>
                    <br />
                    <div className={`${c.big} ${c.rv}`} data-t="404" style={delay(1)}>
                        404
                    </div>
                    <h1 className={`${c.errTitle} ${c.rv}`} style={delay(2)}>
                        {t('notFound.title')} <em>{t('notFound.titleEm')}</em> {t('notFound.titleTail')}
                    </h1>
                    <p className={`${c.lead} ${c.rv}`} style={delay(3)}>
                        {t('notFound.lead')}
                    </p>
                    <div className={`${c.ctaRow} ${c.rv}`} style={delay(4)}>
                        <Link className={`${c.btn} ${c.hot}`} to={home}>
                            {isAuthenticated ? t('notFound.toStatus') : t('notFound.toLogin')}
                        </Link>
                        <button className={c.btn} onClick={copy} type="button">
                            <ShellIcon name={copied ? 'check' : 'copy'} size={15} />
                            {copied ? t('notFound.copied') : t('notFound.copy')}
                        </button>
                    </div>
                </div>

                <div className={`${c.plate} ${c.rv}`} style={delay(3)}>
                    <i className={c.rivet} />
                    <i className={c.rivet} />
                    <i className={c.rivet} />
                    <i className={c.rivet} />
                    <div className={c.plateTop}>
                        <span />
                        <span />
                        <span />
                        <em>router</em>
                    </div>
                    <div className={c.term} style={{ borderBottom: isAuthenticated ? undefined : 0 }}>
                        <span className="c">{`# ${t('notFound.termComment')}`}</span>
                        {'\n'}
                        <span className="p">$</span> <span className="w">{`tuck ui open ${requested}`}</span>
                        {'\n'}
                        <span className="k">route</span>
                        {': '}
                        <span className="e">{`no handler for ${pathname}`}</span>
                        {isAuthenticated && (
                            <>
                                {'\n'}
                                <span className="k">hint</span>
                                {':  '}
                                <span className="s">{t('notFound.hint')}</span> <i className={c.cur} />
                            </>
                        )}
                    </div>
                    {isAuthenticated && (
                        <div className={styles.sugg}>
                            {suggestions.map((item) => (
                                <Link key={item.id} to={item.href}>
                                    {item.href}
                                    <span>{item.name}</span>
                                </Link>
                            ))}
                        </div>
                    )}
                </div>
            </div>
        </div>
    )
}
