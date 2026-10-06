// SPDX-License-Identifier: AGPL-3.0-only
import { useTranslation } from 'react-i18next'

import { short, statusOf } from '../lib'
import styles from '../wrapping.module.css'
import { Session } from '../use-session'

/** Quick-pick chips with the tokens this session wrapped or looked up (newest first). */
export function SessionChips({ onPick, session }: { onPick: (token: string) => void; session: Session }) {
    const { t } = useTranslation('wrapping')
    const list = Object.values(session.capsules)
        .sort((a, b) => (b.created ?? 0) - (a.created ?? 0))
        .slice(0, 4)
    if (list.length === 0) return null
    return (
        <div className={styles.chips}>
            <span className={styles.lab} style={{ alignSelf: 'center', marginRight: 4 }}>
                {t('sessionTokens')}
            </span>
            {list.map((c) => {
                const st = statusOf(c, session.now)
                return (
                    <button
                        className={styles.chip}
                        data-st={st}
                        key={c.token}
                        onClick={() => onPick(c.token)}
                        title={c.token}
                        type="button"
                    >
                        {short(c.token)}
                    </button>
                )
            })}
        </div>
    )
}
