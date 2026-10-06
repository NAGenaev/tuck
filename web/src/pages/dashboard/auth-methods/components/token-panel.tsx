// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { Trans, useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { Icon } from '@shared/ui/kit'
import { ROUTES } from '@shared/constants/routes'
import { Plate } from '@shared/ui/tuck/tuck'

import styles from '../auth-methods.module.css'
import { rich, SectionHead } from './shared'

/** Token auth is built in and always on; there is nothing to configure here, so this points at the Tokens screen. */
export function TokenPanel() {
    const { t } = useTranslation('auth')
    return (
        <>
            <SectionHead
                eyebrow={t('token.eyebrow')}
                first
                text={<Trans components={rich} i18nKey="token.text" ns="auth" />}
                title={t('token.title')}
            />
            <Plate>
                <div className={styles.plateIn}>
                    <div className={styles.two}>
                        <div className={styles.form}>
                            <div className={styles.cap}>{t('token.builtIn')}</div>
                            <div className={styles.big}>token/</div>
                            <p className={styles.pText}>{t('token.always')}</p>
                            <div>
                                <Button
                                    component={Link}
                                    leftSection={<Icon name="key" size={14} />}
                                    to={ROUTES.DASHBOARD.TOKENS}
                                >
                                    {t('token.open')}
                                </Button>
                            </div>
                        </div>
                        <div className={styles.kv}>
                            <div>
                                <span>{t('token.login')}</span>
                                <code>POST /v1/auth/token</code>
                            </div>
                            <div>
                                <span>{t('token.lookup')}</span>
                                <code>GET /v1/auth/token/lookup-self</code>
                            </div>
                            <div>
                                <span>{t('token.header')}</span>
                                <code>X-Tuck-Token</code>
                            </div>
                        </div>
                    </div>
                </div>
            </Plate>
        </>
    )
}
