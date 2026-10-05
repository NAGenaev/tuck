// SPDX-License-Identifier: AGPL-3.0-only
import { useTranslation } from 'react-i18next'

import c from '@shared/ui/shell-controls/console.module.css'
import ctl from '@shared/ui/shell-controls/controls.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'
import { usePopover } from '@shared/ui/shell-controls/use-popover'

const LANGUAGES = [
    { code: 'en', label: 'English' },
    { code: 'ru', label: 'Русский' }
]

/**
 * variant "chip": compact RU/EN toggle for the logged-out screens.
 * variant "menu": icon button with a language dropdown for the dashboard header.
 */
export function LanguageSwitcher({ variant = 'menu' }: { variant?: 'chip' | 'menu' }) {
    const { t, i18n } = useTranslation('shell')
    const pop = usePopover()
    const current = i18n.resolvedLanguage === 'en' ? 'en' : 'ru'

    if (variant === 'chip') {
        const next = current === 'ru' ? 'en' : 'ru'
        return (
            <button
                aria-label={t('language')}
                className={c.chip}
                onClick={() => void i18n.changeLanguage(next)}
                title={t('language')}
                type="button"
            >
                <ShellIcon name="lang" size={15} />
                {current.toUpperCase()}
            </button>
        )
    }

    return (
        <div className={ctl.pop} ref={pop.ref}>
            <button
                aria-expanded={pop.open}
                aria-haspopup="menu"
                aria-label={t('language')}
                className={`${ctl.ctrl} ${ctl.tip}`}
                data-open={pop.open}
                data-tip={t('language')}
                onClick={pop.toggle}
                type="button"
            >
                <ShellIcon name="lang" />
            </button>
            <div className={`${ctl.menu} ${ctl.sm}`} data-open={pop.open} role="menu">
                {LANGUAGES.map((lang) => (
                    <button
                        className={ctl.item}
                        data-current={current === lang.code}
                        key={lang.code}
                        onClick={() => {
                            void i18n.changeLanguage(lang.code)
                            pop.close()
                        }}
                        role="menuitem"
                        type="button"
                    >
                        {lang.label}
                    </button>
                ))}
            </div>
        </div>
    )
}
