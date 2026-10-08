// SPDX-License-Identifier: AGPL-3.0-only
import { isAxiosError } from 'axios'
import { CSSProperties, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useSearchParams } from 'react-router'

import { useAuthTone } from '@app/layouts/auth/auth.layout'
import { lookupSelf, oidcCallback } from '@shared/api/endpoints/auth'
import { removeToken, setToken } from '@shared/auth/token'
import { OIDC_STATE_KEY } from '@shared/auth/oidc'
import { ROUTES } from '@shared/constants/routes'
import { useAuth } from '@shared/hooks/use-auth'
import c from '@shared/ui/shell-controls/console.module.css'
import { vaultDoor } from '@shared/ui/shell-controls/vault-door'

import { clearSessionExpired } from '../session-expired'

const delay = (n: number) => ({ '--d': n }) as CSSProperties

export function OidcCallbackPage() {
    const { t } = useTranslation('login')
    const { setIsAuthenticated } = useAuth()
    const navigate = useNavigate()
    const [params] = useSearchParams()
    const [error, setError] = useState<null | string>(null)
    const started = useRef(false)
    useAuthTone(error ? 'red' : 'brass')

    useEffect(() => {
        // StrictMode runs effects twice; the state and the code are single-use
        if (started.current) return
        started.current = true

        const code = params.get('code') ?? ''
        const state = params.get('state') ?? ''
        const providerError = params.get('error')
        let expected: null | string = null
        try {
            expected = sessionStorage.getItem(OIDC_STATE_KEY)
            sessionStorage.removeItem(OIDC_STATE_KEY)
        } catch {
            /* storage unavailable: treated as a missing session below */
        }
        // keep the one-time code out of the address bar and browser history
        window.history.replaceState(null, '', window.location.pathname)

        const run = async () => {
            if (providerError) {
                setError(t('sso.errProvider', { error: providerError }))
                return
            }
            // login-CSRF guard: only finish a login this browser started
            if (!code || !state || !expected || state !== expected) {
                setError(t('sso.errState'))
                return
            }
            try {
                removeToken()
                const res = await oidcCallback(code, state)
                setToken(res.token)
                await lookupSelf()
            } catch (err) {
                removeToken()
                const status = isAxiosError(err) ? err.response?.status : undefined
                const msg = isAxiosError(err) ? String(err.response?.data?.error ?? '') : ''
                if (!isAxiosError(err) || !err.response) setError(t('errNetwork'))
                else if (status === 400 && msg.includes('code')) setError(t('sso.errCode'))
                else if (status === 400) setError(t('sso.errInvalidState'))
                else if (status === 401) setError(t('sso.errToken'))
                else if (status === 403) setError(t('sso.errNoRole'))
                else if (status === 502) setError(t('sso.errIdp'))
                else if (status === 503) setError(t('sso.notConfigured'))
                else if (status === 429) setError(t('sso.throttled'))
                else setError(t('errServer'))
                return
            }
            clearSessionExpired()
            const door = vaultDoor(t('door'))
            await door.shut()
            setIsAuthenticated(true)
            navigate(ROUTES.DASHBOARD.STATUS, { replace: true })
            void door.open()
        }
        void run()
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [])

    return (
        <div className={`${c.wrap} ${c.errGrid}`}>
            <div>
                <div className={`${c.badge} ${c.rv}`} data-tone={error ? 'red' : undefined} style={delay(0)}>
                    <i />
                    {t('sso.badge')}
                </div>
                <h2 className={`${c.errTitle} ${c.rv}`} style={delay(1)}>
                    {error ? t('sso.failTitle') : t('sso.working')}
                </h2>
                <p className={`${c.lead} ${c.rv}`} role={error ? 'alert' : 'status'} style={delay(2)}>
                    {error ?? t('sso.workingText')}
                </p>
                {error && (
                    <div className={`${c.ctaRow} ${c.rv}`} style={delay(3)}>
                        <Link className={`${c.btn} ${c.hot}`} to={ROUTES.AUTH.LOGIN}>
                            {t('sso.back')}
                        </Link>
                    </div>
                )}
            </div>
        </div>
    )
}
