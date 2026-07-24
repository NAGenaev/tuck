// SPDX-License-Identifier: AGPL-3.0-only
// Forked (provider nesting shape) from github.com/remnawave/frontend
// (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00), trimmed to what Tuck needs.
import '@fontsource/montserrat/400.css'
import '@fontsource/montserrat/500.css'
import '@fontsource/montserrat/600.css'
import '@fontsource/montserrat/700.css'
import '@fontsource/montserrat/800.css'
import '@fontsource/fira-mono/400.css'
import '@mantine/core/styles.css'
import '@mantine/notifications/styles.css'
import './global.css'

import { MantineProvider } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { Notifications } from '@mantine/notifications'
import { QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'

import { AuthProvider } from '@shared/hocs/auth-provider/auth-provider'
import { theme } from '@shared/constants/theme/theme'
import { queryClient } from '@shared/api/query-client'

import i18n from './app/i18n/i18n'
import { Router } from './app/router/router'

export function App() {
    return (
        <I18nextProvider defaultNS="translation" i18n={i18n}>
            <QueryClientProvider client={queryClient}>
                <AuthProvider>
                    <MantineProvider defaultColorScheme="dark" theme={theme}>
                        <ModalsProvider>
                            <Notifications position="top-right" />
                            <Router />
                        </ModalsProvider>
                    </MantineProvider>
                </AuthProvider>
            </QueryClientProvider>
        </I18nextProvider>
    )
}
