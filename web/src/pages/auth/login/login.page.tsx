// SPDX-License-Identifier: AGPL-3.0-only
// Forked (layout) from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00);
// submit handler rewritten against Tuck's token-auth model (paste a token, validate via lookup-self —
// Tuck's built-in token auth has no username/password login, matching the previous UI's behavior).
import { Alert, Button, PasswordInput } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'

import { lookupSelf } from '@shared/api/endpoints/auth'
import { setToken } from '@shared/auth/token'
import { ROUTES } from '@shared/constants/routes'
import { useAuth } from '@shared/hooks/use-auth'
import { useSealStatus } from '@shared/api/hooks/use-seal-status'
import { Logo } from '@shared/ui/logo/logo'
import { Cursor, Plate, PlateBar, Term } from '@shared/ui/tuck/tuck'

import styles from './login.module.css'

export function LoginPage() {
    const { t } = useTranslation()
    const [token, setTokenInput] = useState('')
    const [error, setError] = useState<null | string>(null)
    const [isSubmitting, setIsSubmitting] = useState(false)
    const { setIsAuthenticated } = useAuth()
    const navigate = useNavigate()
    const { data: seal } = useSealStatus()

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault()
        setError(null)
        setIsSubmitting(true)
        try {
            setToken(token)
            await lookupSelf()
            setIsAuthenticated(true)
            navigate(ROUTES.DASHBOARD.STATUS)
        } catch {
            setToken('')
            setError(t('login.invalidToken'))
        } finally {
            setIsSubmitting(false)
        }
    }

    const sealed = seal?.sealed
    const sealLine = seal
        ? `${seal.type ?? '—'} · ${seal.received_shards ?? 0} / ${seal.required_shards ?? 0}`
        : '…'

    return (
        <div className={styles.grid}>
            <div>
                <div className={styles.badge}>
                    <i /> {t('login.badge')}
                </div>
                <h1 className={styles.h1}>
                    {t('login.title')}
                    <span className={styles.gold}>{t('login.titleGold')}</span>
                </h1>
                <p className={styles.lead}>{t('login.lead')}</p>
                <div className={styles.facts}>
                    <div>
                        <b>AES-256</b>
                        {t('login.factEnc')}
                    </div>
                    <div>
                        <b>Shamir</b>
                        {t('login.factSeal')}
                    </div>
                    <div>
                        <b>Raft</b>
                        {t('login.factRaft')}
                    </div>
                </div>
            </div>

            <Plate>
                <PlateBar>{t('login.plate')}</PlateBar>
                <Term>
                    <span className="c"># tuck login</span>
                    {'\n'}
                    <span className="p">$</span> <span className="w">tuck token lookup-self</span>
                    <Cursor />
                </Term>
                <form className={styles.form} onSubmit={handleSubmit}>
                    <Logo />
                    {error && (
                        <Alert color="red" variant="light">
                            {error}
                        </Alert>
                    )}
                    <PasswordInput
                        autoFocus
                        label={t('tokens.token')}
                        onChange={(e) => setTokenInput(e.currentTarget.value)}
                        placeholder={t('login.tokenPlaceholder')}
                        size="md"
                        value={token}
                    />
                    <Button fullWidth loading={isSubmitting} size="md" type="submit">
                        {t('login.signIn')}
                    </Button>
                    <span className={styles.hint}>{t('login.tokenHint')}</span>
                </form>
                <div className={styles.seal}>
                    <svg fill="none" viewBox="0 0 44 44">
                        <circle cx="22" cy="22" r="19" stroke="#3b4154" strokeWidth="2" />
                        <circle
                            cx="22"
                            cy="22"
                            r="12"
                            stroke={sealed ? '#ff5a4d' : '#d7a94b'}
                            strokeDasharray="4 5"
                            strokeWidth="2"
                        />
                        <circle cx="22" cy="22" fill={sealed ? '#ff5a4d' : '#49e0a4'} r="3" />
                    </svg>
                    <div>
                        <b style={{ color: sealed ? '#ff5a4d' : '#49e0a4' }}>
                            {sealed ? 'Sealed' : seal ? 'Unsealed' : '…'}
                        </b>
                        seal: {sealLine}
                    </div>
                </div>
            </Plate>
        </div>
    )
}
