// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { KeyboardEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { CountUp, Icon, StateBlock } from '@shared/ui/kit'
import { Plate } from '@shared/ui/tuck/tuck'

import styles from '../kv1.module.css'
import { errLine, errStatus } from '../lib'

function FolderIcon() {
    return (
        <svg aria-hidden fill="none" height="15" stroke="currentColor" strokeWidth="1.8" viewBox="0 0 24 24" width="15">
            <path d="M3 6h6l2 2h10v11H3z" />
        </svg>
    )
}

export function SecretList({
    prefix,
    keys,
    loading,
    error,
    fetching,
    selected,
    flash,
    leaving,
    onUp,
    onFolder,
    onSelect,
    onWrite,
    onRetry
}: {
    error: unknown
    fetching: boolean
    flash: null | string
    keys: string[] | undefined
    leaving: null | string
    loading: boolean
    onFolder: (key: string) => void
    onRetry: () => void
    onSelect: (key: string) => void
    onUp: () => void
    onWrite: () => void
    prefix: string
    selected: null | string
}) {
    const { t } = useTranslation('kv1')
    const folders = keys?.filter((k) => k.endsWith('/')).length ?? 0
    const files = (keys?.length ?? 0) - folders

    const activate = (fn: () => void) => ({
        onClick: fn,
        onKeyDown: (e: KeyboardEvent) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                fn()
            }
        }
    })

    let meta: ReactNode = '—'
    let body: ReactNode

    if (loading) {
        meta = t('loading')
        body = (
            <div aria-busy className={styles.skel}>
                {[0, 1, 2, 3, 4, 5].map((i) => (
                    <div key={i}>
                        <i />
                        <b style={{ width: `${40 + ((i * 37) % 45)}%` }} />
                    </div>
                ))}
            </div>
        )
    } else if (error) {
        const forbidden = errStatus(error) === 403
        meta = forbidden ? '403' : 'error'
        body = (
            <div className={styles.stateWrap}>
                <StateBlock
                    action={
                        <span className={styles.stateCode}>{`secret/${prefix}*`}</span>
                    }
                    details={forbidden ? undefined : errLine(error)}
                    kind={forbidden ? 'forbidden' : 'error'}
                    message={forbidden ? t('forbiddenText') : undefined}
                    onRetry={onRetry}
                    retrying={fetching}
                    title={forbidden ? t('forbiddenTitle') : t('errorTitle')}
                />
            </div>
        )
    } else if (!keys || keys.length === 0) {
        meta = t('zeroKeys')
        body = (
            <>
                <div className={styles.stateWrap}>
                    <StateBlock
                        action={
                            <Button onClick={onWrite} size="sm">
                                {t('write')}
                            </Button>
                        }
                        kind="empty"
                        message={t('emptyText', { path: `secret/${prefix}` })}
                        title={t('emptyTitle')}
                    />
                </div>
                {prefix && (
                    <div className={styles.upWrap}>
                        <Button leftSection={<Icon name="left" size={14} />} onClick={onUp} size="xs" variant="default">
                            {t('up')}
                        </Button>
                    </div>
                )}
            </>
        )
    } else {
        meta = (
            <>
                <CountUp duration={700} value={keys.length} /> {t('recordsInFolder')}
            </>
        )
        let i = 0
        body = (
            <>
                <div className={styles.listHead}>
                    <span />
                    <span>{t('colName')}</span>
                    <span>{t('colType')}</span>
                </div>
                <ul className={styles.rows} role="list">
                    {prefix && (
                        <li
                            className={styles.row}
                            data-folder
                            role="button"
                            style={{ ['--i' as string]: i++ }}
                            tabIndex={0}
                            {...activate(onUp)}
                        >
                            <span className={styles.ico}>
                                <Icon name="up" size={15} />
                            </span>
                            <span className={styles.nm}>
                                ..<small>{t('upHint')}</small>
                            </span>
                            <span className={styles.go}>←</span>
                        </li>
                    )}
                    {keys.map((k) => {
                        const isFolder = k.endsWith('/')
                        const full = prefix + k
                        return (
                            <li
                                className={styles.row}
                                data-flash={flash === full || undefined}
                                data-folder={isFolder || undefined}
                                data-leave={leaving === full || undefined}
                                data-sel={(!isFolder && selected === full) || undefined}
                                key={k}
                                role="button"
                                style={{ ['--i' as string]: i++ }}
                                tabIndex={0}
                                {...activate(() => (isFolder ? onFolder(k) : onSelect(k)))}
                            >
                                <span className={styles.ico}>
                                    {isFolder ? <FolderIcon /> : <Icon name="key" size={15} />}
                                </span>
                                <span className={styles.nm} title={k}>
                                    {k}
                                </span>
                                <span className={styles.go}>{isFolder ? t('folderGo') : t('keyTag')}</span>
                            </li>
                        )
                    })}
                </ul>
                <div className={styles.listFoot}>
                    <span>
                        <CountUp duration={700} value={files} /> {t('keysCount', { folders })}
                    </span>
                    <span>{t('sortNote')}</span>
                </div>
            </>
        )
    }

    return (
        <Plate>
            <div className={styles.plateTop}>
                <b>{t('colName')}</b>
                <em>{meta}</em>
            </div>
            {body}
        </Plate>
    )
}
