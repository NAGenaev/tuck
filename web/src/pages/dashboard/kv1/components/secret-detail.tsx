// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { ReactNode, useEffect, useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { deleteKVv1, getKVv1 } from '@shared/api/endpoints/kv'
import { ConfirmModal, CopyButton, Icon, StateBlock, toast, useCopy } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import styles from '../kv1.module.css'
import { bullets, byteLength, errLine, errStatus, maskValue, parseObject, stringify } from '../lib'

const AUTO_MASK_MS = 30_000
const TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|\b(true|false|null)\b|([{}[\],])/g

function Highlighted({ text, masked }: { masked: boolean; text: string }) {
    const out: ReactNode[] = []
    let last = 0
    let n = 0
    for (const m of text.matchAll(TOKEN)) {
        const at = m.index ?? 0
        if (at > last) out.push(text.slice(last, at))
        if (m[1]) {
            if (m[2]) {
                out.push(
                    <span className={styles.tkKey} key={n++}>
                        {m[1]}
                    </span>,
                    <span className={styles.tkPunc} key={n++}>
                        {m[2]}
                    </span>
                )
            } else {
                out.push(
                    <span className={styles.tkStr} key={n++}>
                        {masked ? `"${bullets(10)}"` : m[1]}
                    </span>
                )
            }
        } else if (m[3] || m[4]) {
            out.push(
                <span className={styles.tkNum} key={n++}>
                    {masked ? bullets(4) : m[0]}
                </span>
            )
        } else {
            out.push(
                <span className={styles.tkPunc} key={n++}>
                    {m[0]}
                </span>
            )
        }
        last = at + m[0].length
    }
    if (last < text.length) out.push(text.slice(last))
    return <>{out}</>
}

/** Small bordered button that copies and flips to a mint check (the artifact's .cp). */
function CopyText({ value, label, title }: { label?: string; title: string; value: string }) {
    const { copied, copy } = useCopy({ toast: false })
    return (
        <button
            aria-label={title}
            className={styles.cp}
            data-done={copied || undefined}
            onClick={() => void copy(value)}
            title={title}
            type="button"
        >
            <Icon name={copied ? 'check' : 'copy'} size={13} />
            {label}
        </button>
    )
}

export function SecretDetail({
    path,
    onOverwrite,
    onDeleted
}: {
    onDeleted: (path: string) => void
    onOverwrite: (path: string, value: null | string) => void
    path: string
}) {
    const { t } = useTranslation('kv1')
    const q = useQuery({
        queryKey: ['kv1', 'get', path],
        queryFn: () => getKVv1(path),
        retry: false
    })
    const [tab, setTab] = useState<'kv' | 'raw'>('kv')
    const [revealAll, setRevealAll] = useState(false)
    const [revealed, setRevealed] = useState<Record<string, boolean>>({})
    const [confirming, setConfirming] = useState(false)

    // re-mask on its own so a forgotten tab does not keep a secret on screen
    useEffect(() => {
        if (!revealAll) return
        const id = window.setTimeout(() => setRevealAll(false), AUTO_MASK_MS)
        return () => window.clearTimeout(id)
    }, [revealAll])

    const value = q.data?.value ?? ''
    const isB64 = q.data?.encoding === 'base64'
    const obj = useMemo(() => (isB64 ? null : parseObject(value)), [isB64, value])
    const activeTab = obj ? tab : 'raw'

    const doDelete = async () => {
        try {
            await deleteKVv1(path)
        } catch (e) {
            toast.error(t('deleteFailed'), `DELETE secret/${path} → ${errLine(e)}`)
            throw e
        }
        onDeleted(path)
    }

    let body: ReactNode
    if (q.isLoading) {
        body = (
            <>
                <div className={styles.plateTop}>
                    <b>{t('valueTitle')}</b>
                    <em>{t('loading')}</em>
                </div>
                <div aria-busy className={`${styles.skel} ${styles.skelWide}`} style={{ padding: '14px 8px' }}>
                    {[70, 55, 85, 45].map((w) => (
                        <div key={w}>
                            <i />
                            <b style={{ width: `${w}%` }} />
                        </div>
                    ))}
                </div>
            </>
        )
    } else if (q.isError) {
        const status = errStatus(q.error)
        const forbidden = status === 403
        const missing = status === 404
        body = (
            <>
                <div className={styles.plateTop}>
                    <b>{t('valueTitle')}</b>
                    <em>{status ?? 'error'}</em>
                </div>
                <div className={styles.detailState}>
                    <StateBlock
                        details={forbidden || missing ? undefined : errLine(q.error)}
                        kind={forbidden ? 'forbidden' : 'error'}
                        message={
                            forbidden
                                ? t('readForbiddenText', { path: `secret/${path}` })
                                : missing
                                  ? t('notFoundText', { path: `secret/${path}` })
                                  : undefined
                        }
                        onRetry={missing ? undefined : () => void q.refetch()}
                        retrying={q.isFetching}
                        title={forbidden ? t('readForbidden') : missing ? t('notFound') : t('readError')}
                    />
                </div>
            </>
        )
    } else {
        const pretty = obj ? JSON.stringify(obj, null, 2) : value
        body = (
            <>
                <div className={styles.plateTop}>
                    <b>{t('valueTitle')}</b>
                    <em>GET /v1/secret/…</em>
                </div>
                <div className={styles.dhead}>
                    <div className={styles.dpath}>
                        <span>{`secret/${path}`}</span>
                        <CopyButton label={t('copyPath')} size={28} toast={false} value={`secret/${path}`} />
                    </div>
                    <div className={styles.meta}>
                        <Pill tone={obj ? 'c' : 'v'}>
                            {isB64 ? t('typeBase64') : obj ? t('typeJson') : t('typeText')}
                        </Pill>
                        <span>{t('size', { n: byteLength(value) })}</span>
                        {obj && <span>{t('fields', { n: Object.keys(obj).length })}</span>}
                        <Pill tone="r">{t('noVersions')}</Pill>
                    </div>
                </div>
                <div className={styles.tabs}>
                    {obj && (
                        <button data-on={activeTab === 'kv' || undefined} onClick={() => setTab('kv')} type="button">
                            {t('tabKv')}
                        </button>
                    )}
                    <button data-on={activeTab === 'raw' || undefined} onClick={() => setTab('raw')} type="button">
                        {t('tabRaw')}
                    </button>
                    <div className={styles.tools}>
                        <button
                            aria-pressed={revealAll}
                            className={styles.cp}
                            onClick={() => {
                                setRevealAll((r) => !r)
                                setRevealed({})
                            }}
                            title={`${t('show')} / ${t('hide')}`}
                            type="button"
                        >
                            <Icon name={revealAll ? 'eyeoff' : 'eye'} size={13} />
                            <span>{revealAll ? t('hide') : t('show')}</span>
                        </button>
                    </div>
                </div>
                <div className={styles.dbody}>
                    {activeTab === 'kv' && obj ? (
                        <div className={styles.kvs}>
                            {Object.entries(obj).map(([k, v], i) => {
                                const sv = stringify(v)
                                const shown = revealAll || revealed[k]
                                return (
                                    <div className={styles.kv} key={k} style={{ ['--i' as string]: i }}>
                                        <span className={styles.k}>{k}</span>
                                        <button
                                            className={styles.v}
                                            data-masked={!shown || undefined}
                                            onClick={() => setRevealed((r) => ({ ...r, [k]: !r[k] }))}
                                            title={t('toggleTitle')}
                                            type="button"
                                        >
                                            {shown ? sv : maskValue(sv)}
                                        </button>
                                        <CopyButton label={t('copyField')} size={28} toast={false} value={sv} />
                                    </div>
                                )
                            })}
                        </div>
                    ) : (
                        <>
                            <div className={styles.code} data-masked={!revealAll || undefined}>
                                {obj ? (
                                    <Highlighted masked={!revealAll} text={pretty} />
                                ) : revealAll ? (
                                    value
                                ) : (
                                    maskValue(value)
                                )}
                            </div>
                            <div className={styles.row2}>
                                <CopyText label={t('copyValue')} title={t('copyValue')} value={value} />
                            </div>
                        </>
                    )}
                    <div className={styles.dangerZone}>
                        <div>
                            <b>{t('dangerZone')}</b>
                            <p>{t('dangerText')}</p>
                        </div>
                        <div className={styles.row2}>
                            <Button
                                leftSection={<Icon name="key" size={14} />}
                                onClick={() => onOverwrite(path, isB64 ? null : value)}
                                size="xs"
                                variant="default"
                            >
                                {t('overwrite')}
                            </Button>
                            <Button
                                color="red"
                                leftSection={<Icon name="trash" size={14} />}
                                onClick={() => setConfirming(true)}
                                size="xs"
                                variant="outline"
                            >
                                {t('delete')}
                            </Button>
                        </div>
                    </div>
                </div>
            </>
        )
    }

    return (
        <Plate>
            {body}
            <ConfirmModal
                confirmLabel={t('delete')}
                confirmText={`secret/${path}`}
                message={
                    <>
                        <Trans components={{ b: <b /> }} i18nKey="deleteText" ns="kv1" />
                        <div className={styles.typed}>{`secret/${path}`}</div>
                    </>
                }
                onClose={() => setConfirming(false)}
                onConfirm={doDelete}
                opened={confirming}
                title={t('deleteTitle')}
            />
        </Plate>
    )
}

export function DetailPlaceholder() {
    const { t } = useTranslation('kv1')
    return (
        <Plate>
            <div className={styles.plateTop}>
                <b>{t('valueTitle')}</b>
                <em>{t('notSelected')}</em>
            </div>
            <div className={styles.detailEmpty}>
                <Icon name="key" size={44} />
                <p>{t('pickKey')}</p>
            </div>
        </Plate>
    )
}
