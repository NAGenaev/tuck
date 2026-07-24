// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import i18n from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import Backend from 'i18next-http-backend'
import { initReactI18next } from 'react-i18next'

void i18n
    .use(Backend)
    .use(LanguageDetector)
    .use(initReactI18next)
    .init({
        fallbackLng: 'en',
        supportedLngs: ['en', 'ru'],
        ns: ['translation'],
        defaultNS: 'translation',
        backend: {
            // NOTE: must be base-path aware — the app is served under /ui/, and a
            // hardcoded "/locales/..." would 404 at the domain root.
            loadPath: `${import.meta.env.BASE_URL}locales/{{lng}}/{{ns}}.json`
        },
        interpolation: {
            escapeValue: false
        }
    })

export default i18n
