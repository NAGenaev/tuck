// SPDX-License-Identifier: AGPL-3.0-only
// Forked (layout) from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00);
// submit handler rewritten against Tuck's token-auth model (paste a token, validate via lookup-self —
// Tuck's built-in token auth has no username/password login, matching the previous UI's behavior).
import { Alert, Box, Button, Center, PasswordInput, Stack } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'

import { lookupSelf } from '@shared/api/endpoints/auth'
import { setToken } from '@shared/auth/token'
import { ROUTES } from '@shared/constants/routes'
import { useAuth } from '@shared/hooks/use-auth'
import { Logo } from '@shared/ui/logo/logo'

export function LoginPage() {
    const { t } = useTranslation()
    const [token, setTokenInput] = useState('')
    const [error, setError] = useState<null | string>(null)
    const [isSubmitting, setIsSubmitting] = useState(false)
    const { setIsAuthenticated } = useAuth()
    const navigate = useNavigate()

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

    return (
        <Box component="form" maw={360} onSubmit={handleSubmit} w="100%">
            <Stack gap="md">
                <Center mb="sm">
                    <Logo />
                </Center>
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
                    value={token}
                />
                <Button fullWidth loading={isSubmitting} type="submit">
                    {t('login.signIn')}
                </Button>
            </Stack>
        </Box>
    )
}
