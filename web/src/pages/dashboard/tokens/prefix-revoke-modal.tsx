// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Modal } from '@shared/ui/kit'

import s from './tokens.module.css'
import { shortAcc, TokenRow } from './tokens.utils'

/** Revoke every token whose display name starts with a prefix; gated by typing a confirmation word. */
export function PrefixRevokeModal({
    opened,
    onClose,
    rows,
    onRevoke
}: {
    onClose: () => void
    onRevoke: (ids: string[]) => Promise<void>
    opened: boolean
    rows: TokenRow[]
}) {
    const { t } = useTranslation('tokens')
    const [prefix, setPrefix] = useState('')
    const [typed, setTyped] = useState('')
    const [busy, setBusy] = useState(false)
    const word = t('prefix.word')

    useEffect(() => {
        if (opened) {
            setPrefix('')
            setTyped('')
        }
    }, [opened])

    const p = prefix.trim().toLowerCase()
    const matches = p.length >= 2 ? rows.filter((r) => r.name.toLowerCase().startsWith(p)) : []
    const ready = matches.length > 0 && typed.trim().toLowerCase() === word.toLowerCase() && !busy

    const run = async () => {
        if (!ready) return
        setBusy(true)
        try {
            await onRevoke(matches.map((r) => r.id))
            onClose()
        } finally {
            setBusy(false)
        }
    }

    return (
        <Modal
            busy={busy}
            footer={
                <>
                    <Button disabled={busy} onClick={onClose} variant="default">
                        {t('create.cancel')}
                    </Button>
                    <Button color="red" disabled={!ready} loading={busy} onClick={run}>
                        {t('prefix.label', { count: matches.length })}
                    </Button>
                </>
            }
            icon="trash"
            onClose={onClose}
            opened={opened}
            size={520}
            title={t('prefix.title')}
            tone="danger"
        >
            <div className={s.form}>
                <p className={s.hint} style={{ margin: 0 }}>
                    {t('prefix.text')}
                </p>
                <div className={s.fld}>
                    <label className={s.lbl} htmlFor="tk-prefix">
                        {t('prefix.field')}
                    </label>
                    <input
                        autoComplete="off"
                        autoFocus
                        className={s.field}
                        disabled={busy}
                        id="tk-prefix"
                        onChange={(e) => setPrefix(e.currentTarget.value)}
                        placeholder={t('prefix.placeholder')}
                        spellCheck={false}
                        value={prefix}
                    />
                </div>

                {p.length < 2 ? (
                    <span className={s.hint}>{t('prefix.minChars')}</span>
                ) : matches.length === 0 ? (
                    <span className={s.hint}>{t('prefix.noMatch')}</span>
                ) : (
                    <div className={s.listRm}>
                        {matches.map((r) => (
                            <div key={r.id}>
                                <b>{r.name}</b>
                                <span>{r.accessor ? shortAcc(r.accessor) : ''}</span>
                            </div>
                        ))}
                    </div>
                )}

                {matches.length > 0 && (
                    <div className={s.fld}>
                        <label className={s.lbl} htmlFor="tk-prefix-word">
                            {t('prefix.wordLabel', { word })}
                        </label>
                        <input
                            autoComplete="off"
                            className={s.field}
                            disabled={busy}
                            id="tk-prefix-word"
                            onChange={(e) => setTyped(e.currentTarget.value)}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter') void run()
                            }}
                            spellCheck={false}
                            value={typed}
                        />
                    </div>
                )}
            </div>
        </Modal>
    )
}
