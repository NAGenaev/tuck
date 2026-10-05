// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import styles from '../detail.module.css'
import common from '../common.module.css'
import { fmtDate, VerRow } from '../utils'

const MAX_LIMIT = 1000

export function MetaTab({
    rows,
    current,
    maxVersions,
    onSaveMax,
    saving
}: {
    current: number
    maxVersions: number
    onSaveMax: (n: number) => void
    rows: VerRow[]
    saving: boolean
}) {
    const { t, i18n } = useTranslation('kv')
    const [draft, setDraft] = useState(maxVersions)
    const oldest = rows[0]
    const cur = rows.find((r) => r.version === current)
    const dirty = draft !== maxVersions

    return (
        <div className={styles.tp}>
            <div className={styles.mg}>
                <div>
                    <small>{t('kv2.meta.current')}</small>
                    <strong>{current || '—'}</strong>
                </div>
                <div>
                    <small>{t('kv2.meta.oldest')}</small>
                    <strong>{oldest?.version ?? '—'}</strong>
                </div>
                <div>
                    <small>{t('kv2.meta.created')}</small>
                    <strong>{fmtDate(oldest?.createdAt, i18n.language)}</strong>
                </div>
                <div>
                    <small>{t('kv2.meta.updated')}</small>
                    <strong>{fmtDate(cur?.createdAt, i18n.language)}</strong>
                </div>
                <div style={{ gridColumn: '1 / -1' }}>
                    <small>max_versions</small>
                    <div className={styles.mini}>
                        <div className={styles.stepper}>
                            <button
                                aria-label={t('kv2.meta.less')}
                                disabled={draft <= 0 || saving}
                                onClick={() => setDraft((n) => Math.max(0, n - 1))}
                                type="button"
                            >
                                −
                            </button>
                            <span aria-live="polite">{draft === 0 ? '∞' : draft}</span>
                            <button
                                aria-label={t('kv2.meta.more')}
                                disabled={draft >= MAX_LIMIT || saving}
                                onClick={() => setDraft((n) => Math.min(MAX_LIMIT, n + 1))}
                                type="button"
                            >
                                +
                            </button>
                        </div>
                        <Button disabled={!dirty} loading={saving} onClick={() => onSaveMax(draft)} size="compact-sm">
                            {t('kv2.meta.save')}
                        </Button>
                        {dirty && (
                            <Button disabled={saving} onClick={() => setDraft(maxVersions)} size="compact-sm" variant="default">
                                {t('kv2.meta.reset')}
                            </Button>
                        )}
                    </div>
                </div>
            </div>
            <div className={common.msg}>{t('kv2.meta.hint')}</div>
        </div>
    )
}
