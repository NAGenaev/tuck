// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { destroyKVv2, softDeleteKVv2, undeleteKVv2 } from '@shared/api/endpoints/kvv2'
import { ConfirmModal, copyText, Drawer, Icon, toast } from '@shared/ui/kit'
import { Pill } from '@shared/ui/tuck/tuck'

import { errMessage, updateMaxVersions } from '../api'
import common from '../common.module.css'
import styles from '../detail.module.css'
import { useKv2Meta, useKv2Version } from '../use-kv2'
import { dirName, fmtDate, fmtRel, parseValue, versionRows } from '../utils'
import { DiffTab } from './diff-tab'
import { MetaTab } from './meta-tab'
import { revealKeys, ValueTab } from './value-tab'
import { StatePill, VersionsTab } from './versions-tab'

const TTL_MS = 30000

type Tab = 'diff' | 'meta' | 'val' | 'ver'
type Pending = null | { kind: 'destroy' | 'soft'; version: number }

export interface EditSeed {
    path: string
    value?: string
    version?: number
}

export function SecretDrawer({
    path,
    onClose,
    onEdit,
    suspended
}: {
    onClose: () => void
    onEdit: (seed: EditSeed) => void
    path: null | string
    /** another overlay is on top: do not react to Esc / outside clicks */
    suspended: boolean
}) {
    const last = useRef(path ?? '')
    const seq = useRef(0)
    const wasOpen = useRef(false)
    if (path && !wasOpen.current) seq.current += 1
    wasOpen.current = !!path
    if (path) last.current = path
    const shownPath = last.current

    return shownPath ? (
        <DrawerBody key={`${shownPath}#${seq.current}`} onClose={onClose} onEdit={onEdit} opened={!!path} path={shownPath} suspended={suspended} />
    ) : null
}

function DrawerBody({
    path,
    opened,
    onClose,
    onEdit,
    suspended
}: {
    onClose: () => void
    onEdit: (seed: EditSeed) => void
    opened: boolean
    path: string
    suspended: boolean
}) {
    const { t, i18n } = useTranslation('kv')
    const qc = useQueryClient()
    const [tab, setTab] = useState<Tab>('val')
    const [view, setView] = useState<number | undefined>(undefined)
    const [rev, setRev] = useState<Set<string>>(() => new Set())
    const [raw, setRaw] = useState(false)
    const [da, setDa] = useState<number | undefined>(undefined)
    const [db, setDb] = useState<number | undefined>(undefined)
    const [dshow, setDshow] = useState(false)
    const [pending, setPending] = useState<Pending>(null)
    const [busyVersion, setBusyVersion] = useState<number | undefined>(undefined)
    const [savingMax, setSavingMax] = useState(false)
    const [leftMs, setLeftMs] = useState(TTL_MS)
    const end = useRef(0)
    const [pathCopied, setPathCopied] = useState(false)

    const metaQ = useKv2Meta(path)
    const rows = useMemo(() => versionRows(metaQ.data), [metaQ.data])
    const current = metaQ.data?.current_version ?? rows[rows.length - 1]?.version ?? 0
    const shownVersion = view ?? current
    const row = rows.find((r) => r.version === shownVersion)
    const curRow = rows.find((r) => r.version === current)
    const viewing = view !== undefined && view !== current

    const valQ = useKv2Version(path, viewing ? view : undefined, row?.state === 'active')
    const parsed = useMemo(() => {
        const v = valQ.data?.value
        if (v === undefined) return null
        if (valQ.data?.encoding === 'base64') return { kind: 'text' as const, text: v }
        return parseValue(v)
    }, [valQ.data])
    const keys = revealKeys(parsed)

    const refresh = useCallback(async () => {
        await Promise.all([
            qc.invalidateQueries({ queryKey: ['kv2', 'get', path] }),
            qc.invalidateQueries({ queryKey: ['kv2', 'meta', path] })
        ])
    }, [qc, path])

    // reveal bookkeeping: every reveal restarts the 30s window, then values hide again on their own
    const hideAll = useCallback(() => setRev(new Set()), [])
    useEffect(() => {
        if (rev.size === 0) {
            setLeftMs(TTL_MS)
            return
        }
        const id = window.setInterval(() => {
            const left = end.current - Date.now()
            if (left <= 0) {
                setRev(new Set())
                toast.warning(t('kv2.value.autoHiddenTitle'), t('kv2.value.autoHiddenText'))
            } else {
                setLeftMs(left)
            }
        }, 250)
        return () => window.clearInterval(id)
    }, [rev.size > 0, t]) // eslint-disable-line react-hooks/exhaustive-deps

    const toggleKey = (k: string) => {
        setRev((prev) => {
            const next = new Set(prev)
            if (next.has(k)) next.delete(k)
            else next.add(k)
            return next
        })
        end.current = Date.now() + TTL_MS
        setLeftMs(TTL_MS)
    }
    const toggleAll = () => {
        const all = keys.length > 0 && keys.every((k) => rev.has(k))
        if (all) {
            hideAll()
        } else {
            setRev(new Set(keys))
            end.current = Date.now() + TTL_MS
            setLeftMs(TTL_MS)
        }
    }

    const readVersion = (v: number) => {
        setView(v === current ? undefined : v)
        hideAll()
        setTab('val')
    }

    const run = async (version: number, fn: () => Promise<void>, title: string) => {
        setBusyVersion(version)
        try {
            await fn()
            await refresh()
            toast.success(title, `${path} · v${version}`)
        } catch (err) {
            toast.error(t('kv2.toast.failed'), errMessage(err))
            throw err
        } finally {
            setBusyVersion(undefined)
        }
    }

    const undelete = (v: number) => run(v, () => undeleteKVv2(path, [v]), t('kv2.toast.restored')).catch(() => undefined)

    const confirmAct = async () => {
        if (!pending) return
        const { kind, version } = pending
        if (kind === 'soft') {
            await run(version, () => softDeleteKVv2(path, [version]), t('kv2.toast.deleted'))
        } else {
            await run(version, () => destroyKVv2(path, [version]), t('kv2.toast.destroyed'))
            if (view === version) setView(undefined)
            hideAll()
        }
    }

    const saveMax = async (n: number) => {
        setSavingMax(true)
        try {
            await updateMaxVersions(path, n)
            await qc.invalidateQueries({ queryKey: ['kv2', 'meta', path] })
            toast.success(t('kv2.toast.metaSaved'), `max_versions = ${n === 0 ? '∞' : n}`)
        } catch (err) {
            toast.error(t('kv2.toast.failed'), errMessage(err))
        } finally {
            setSavingMax(false)
        }
    }

    const dir = dirName(path)
    const tabs: Array<[Tab, string, number | undefined]> = [
        ['val', t('kv2.tabs.value'), undefined],
        ['ver', t('kv2.tabs.versions'), rows.length || undefined],
        ['diff', t('kv2.tabs.diff'), undefined],
        ['meta', t('kv2.tabs.meta'), undefined]
    ]

    const metaLoading = metaQ.isLoading
    const metaErr = metaQ.error ? errMessage(metaQ.error) || t('kv2.toast.failed') : undefined

    return (
        <>
            <Drawer
                busy={!!pending || suspended}
                footer={
                    <>
                        <Button
                            leftSection={<Icon name={pathCopied ? 'check' : 'copy'} size={15} />}
                            onClick={async () => {
                                if (await copyText(`secret/${path}`)) {
                                    setPathCopied(true)
                                    window.setTimeout(() => setPathCopied(false), 1800)
                                } else {
                                    toast.error(t('kv2.value.copyFailed'))
                                }
                            }}
                            variant="default"
                        >
                            {t('kv2.drawer.copyPath')}
                        </Button>
                        <Button
                            leftSection={<Icon name="plus" size={15} />}
                            onClick={() =>
                                onEdit({
                                    path,
                                    value: !viewing && row?.state === 'active' ? valQ.data?.value : undefined,
                                    version: current || undefined
                                })
                            }
                        >
                            {t('kv2.drawer.newVersion')}
                        </Button>
                    </>
                }
                icon="key"
                onClose={onClose}
                opened={opened}
                size={680}
                title={t('kv2.drawer.title')}
            >
                <div style={{ display: 'grid', gap: 12 }}>
                    <div className={styles.dpath}>
                        <span className={styles.pre}>secret/{dir}</span>
                        {path.slice(dir.length)}
                    </div>
                    <div className={styles.dmeta}>
                        {curRow ? (
                            <>
                                <span className={common.ver} data-cur>
                                    v{curRow.version} · {t('kv2.state.current')}
                                </span>
                                <StatePill state={curRow.state} />
                                <Pill tone="mute">
                                    max_versions {metaQ.data?.max_versions ? metaQ.data.max_versions : '∞'}
                                </Pill>
                                <span className={styles.when} title={fmtDate(curRow.createdAt, i18n.language)}>
                                    {t('kv2.drawer.updated', { when: fmtRel(curRow.createdAt, i18n.language) })}
                                </span>
                            </>
                        ) : (
                            <span className={styles.when} style={{ marginLeft: 0 }}>
                                {metaLoading ? t('kv2.drawer.loading') : ''}
                            </span>
                        )}
                    </div>
                    <div className={styles.tabs} role="tablist">
                        {tabs.map(([id, label, n]) => (
                            <button
                                aria-selected={tab === id}
                                data-on={tab === id || undefined}
                                key={id}
                                onClick={() => setTab(id)}
                                role="tab"
                                type="button"
                            >
                                {label}
                                {n !== undefined && <i>{n}</i>}
                            </button>
                        ))}
                    </div>
                </div>

                {metaErr ? (
                    <div className={styles.tp}>
                        <div className={common.banner} data-tone="r">
                            <Icon name="warn" size={20} />
                            <h5>{t('kv2.drawer.loadFailed')}</h5>
                            <p>{metaErr}</p>
                            <div className={common.acts}>
                                <Button onClick={() => void metaQ.refetch()} size="compact-sm" variant="default">
                                    {t('kv2.retry')}
                                </Button>
                            </div>
                        </div>
                    </div>
                ) : tab === 'val' ? (
                    <ValueTab
                        base64={valQ.data?.encoding === 'base64'}
                        current={current}
                        error={valQ.error ? errMessage(valQ.error) || t('kv2.toast.failed') : undefined}
                        leftMs={leftMs}
                        loading={metaLoading || valQ.isLoading}
                        onBack={() => {
                            setView(undefined)
                            hideAll()
                        }}
                        onRaw={setRaw}
                        onRetry={() => void valQ.refetch()}
                        onToggle={toggleKey}
                        onToggleAll={toggleAll}
                        onUndelete={() => row && void undelete(row.version)}
                        parsed={parsed}
                        raw={raw}
                        rev={rev}
                        row={row}
                        ttlMs={TTL_MS}
                        undeleting={busyVersion === row?.version}
                        viewing={viewing}
                    />
                ) : tab === 'ver' ? (
                    <VersionsTab
                        busyVersion={busyVersion}
                        current={current}
                        onCompare={(v) => {
                            setDa(v)
                            setDb(current)
                            setTab('diff')
                        }}
                        onDestroy={(v) => setPending({ kind: 'destroy', version: v })}
                        onRead={readVersion}
                        onSoft={(v) => setPending({ kind: 'soft', version: v })}
                        onUndelete={(v) => void undelete(v)}
                        rows={rows}
                        viewing={viewing ? view : undefined}
                    />
                ) : tab === 'diff' ? (
                    <DiffTab a={da} b={db} onA={setDa} onB={setDb} onShow={setDshow} path={path} rows={rows} show={dshow} />
                ) : (
                    <MetaTab
                        current={current}
                        key={metaQ.data?.max_versions ?? 0}
                        maxVersions={metaQ.data?.max_versions ?? 0}
                        onSaveMax={(n) => void saveMax(n)}
                        rows={rows}
                        saving={savingMax}
                    />
                )}
            </Drawer>

            <ConfirmModal
                cancelLabel={t('kv2.cancel')}
                confirmLabel={pending?.kind === 'destroy' ? t('kv2.ver.destroy') : t('kv2.ver.delete')}
                confirmText={pending?.kind === 'destroy' ? String(pending.version) : undefined}
                destructive={pending?.kind === 'destroy'}
                icon={pending?.kind === 'destroy' ? 'trash' : 'warn'}
                message={
                    pending?.kind === 'destroy'
                        ? t('kv2.confirm.destroyText', { version: pending.version })
                        : t('kv2.confirm.softText', { version: pending?.version })
                }
                onClose={() => setPending(null)}
                onConfirm={confirmAct}
                opened={!!pending}
                title={pending?.kind === 'destroy' ? t('kv2.confirm.destroyTitle') : t('kv2.confirm.softTitle')}
            />
        </>
    )
}
