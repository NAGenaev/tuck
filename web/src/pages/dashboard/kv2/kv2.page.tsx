// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useQueryClient } from '@tanstack/react-query'
import type { CSSProperties } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { getKVv2Meta, softDeleteKVv2 } from '@shared/api/endpoints/kvv2'
import { ConfirmModal, CountUp, Icon, toast } from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow } from '@shared/ui/tuck/tuck'

import { errMessage, errStatus } from './api'
import common from './common.module.css'
import { Entry, ListStatus, SecretList } from './components/secret-list'
import { EditSeed, SecretDrawer } from './components/secret-drawer'
import { WriteModal } from './components/write-modal'
import styles from './kv2.module.css'
import { useKv2List } from './use-kv2'

type Chip = 'all' | 'folder' | 'secret'

const isTyping = (el: EventTarget | null) => {
    const n = el as HTMLElement | null
    if (!n) return false
    const tag = n.tagName
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || n.isContentEditable
}

export function KV2Page() {
    const { t } = useTranslation('kv')
    const qc = useQueryClient()

    const [prefix, setPrefix] = useState('')
    const [slide, setSlide] = useState<null | 'back' | 'fwd'>(null)
    const [filter, setFilter] = useState('')
    const [chip, setChip] = useState<Chip>('all')
    const [jump, setJump] = useState('')
    const [selected, setSelected] = useState<Set<string>>(() => new Set())
    const [drawerPath, setDrawerPath] = useState<null | string>(null)
    const [writeOpen, setWriteOpen] = useState(false)
    const [seed, setSeed] = useState<EditSeed | null>(null)
    const [flash, setFlash] = useState<null | string>(null)
    const [bulkOpen, setBulkOpen] = useState(false)
    const [still, setStill] = useState(false)
    const filterRef = useRef<HTMLInputElement | null>(null)
    const seen = useRef(false)

    const list = useKv2List(prefix)

    // the row entrance animation plays only for the first paint; later refreshes stay still
    useEffect(() => {
        if (list.isSuccess && !seen.current) {
            seen.current = true
            const id = window.setTimeout(() => setStill(true), 1400)
            return () => window.clearTimeout(id)
        }
    }, [list.isSuccess])

    const all = useMemo<Entry[]>(() => {
        const items = (list.data ?? []).map((k) => {
            const folder = k.endsWith('/')
            return { folder, name: k, path: `${prefix}${k}` }
        })
        items.sort((a, b) => (a.folder === b.folder ? a.name.localeCompare(b.name) : a.folder ? -1 : 1))
        return items
    }, [list.data, prefix])

    const status: ListStatus = list.isPending
        ? 'loading'
        : list.isError
          ? errStatus(list.error) === 403
              ? 'forbidden'
              : 'error'
          : 'ready'

    const nFolders = all.filter((e) => e.folder).length
    const nSecrets = all.length - nFolders

    const shown = useMemo(() => {
        const q = filter.trim().toLowerCase()
        return all.filter(
            (e) =>
                (chip === 'all' || (chip === 'folder' ? e.folder : !e.folder)) &&
                (!q || e.name.toLowerCase().includes(q))
        )
    }, [all, chip, filter])

    const go = useCallback(
        (next: string, dir: 'back' | 'fwd' = 'fwd') => {
            setSlide(dir)
            setPrefix(next)
            setSelected(new Set())
            setFilter('')
            setChip('all')
            setStill(false)
            seen.current = false
        },
        []
    )

    const up = useCallback(() => {
        if (!prefix) return
        const parts = prefix.split('/').filter(Boolean)
        parts.pop()
        go(parts.length ? `${parts.join('/')}/` : '', 'back')
    }, [go, prefix])

    const crumbs = ['secret', ...prefix.split('/').filter(Boolean)]

    const doJump = () => {
        const v = jump.trim().replace(/^\/+/, '').replace(/^secret\//, '')
        go(v ? `${v.replace(/\/+$/, '')}/` : '')
        setJump('')
    }

    const openWrite = (s: EditSeed | null = null) => {
        setSeed(s)
        setWriteOpen(true)
    }

    const toggle = (path: string) =>
        setSelected((s) => {
            const n = new Set(s)
            n.has(path) ? n.delete(path) : n.add(path)
            return n
        })

    const toggleAll = () =>
        setSelected((s) => {
            const paths = shown.filter((e) => !e.folder).map((e) => e.path)
            const every = paths.length > 0 && paths.every((p) => s.has(p))
            const n = new Set(s)
            paths.forEach((p) => (every ? n.delete(p) : n.add(p)))
            return n
        })

    const bulkDelete = async () => {
        const paths = [...selected]
        let ok = 0
        let failed = 0
        for (const p of paths) {
            try {
                const m = await getKVv2Meta(p)
                if (!m.current_version) throw new Error('no current version')
                await softDeleteKVv2(p, [m.current_version])
                ok++
            } catch (err) {
                failed++
                if (failed === 1) toast.error(t('kv2.toast.failed'), `${p}: ${errMessage(err)}`)
            }
        }
        await Promise.all([
            qc.invalidateQueries({ queryKey: ['kv2', 'get'] }),
            qc.invalidateQueries({ queryKey: ['kv2', 'meta'] }),
            qc.invalidateQueries({ queryKey: ['kv2', 'list'] })
        ])
        if (ok > 0) toast.success(t('kv2.toast.bulkDone', { count: ok }))
        setSelected(new Set())
        setBulkOpen(false)
    }

    const onSaved = (path: string) => {
        // show the written secret in the list when it lives at the current level
        if (path.startsWith(prefix) && !path.slice(prefix.length).includes('/')) {
            setFlash(path)
            window.setTimeout(() => setFlash(null), 2400)
        }
    }

    const overlayOpen = writeOpen || bulkOpen || drawerPath !== null

    useEffect(() => {
        const onKey = (ev: KeyboardEvent) => {
            if (ev.metaKey || ev.ctrlKey || ev.altKey || overlayOpen || isTyping(ev.target)) return
            if (ev.key === '/') {
                ev.preventDefault()
                filterRef.current?.focus()
            } else if (ev.key === 'n') {
                ev.preventDefault()
                openWrite()
            } else if (ev.key === 'Backspace' && prefix) {
                ev.preventDefault()
                up()
            }
        }
        window.addEventListener('keydown', onKey)
        return () => window.removeEventListener('keydown', onKey)
    }, [overlayOpen, prefix, up])

    const folderSet = useMemo(() => new Set(all.filter((e) => e.folder).map((e) => e.path)), [all])
    const delay = (n: number): CSSProperties => ({ ['--d' as string]: `${n}ms` })

    return (
        <Page title="KV v2">
            <div className={styles.page}>
                <div className={`${styles.headTop} ${styles.rv}`}>
                    <div>
                        <Eyebrow>{t('kv2.eyebrow')}</Eyebrow>
                        <h1 className={styles.title}>
                            {t('kv2.title1')}
                            <br />
                            <span className={styles.gold}>{t('kv2.title2')}</span>
                        </h1>
                        <p className={styles.sub}>{t('kv2.sub')}</p>
                    </div>
                    <div className={styles.stats}>
                        <div className={styles.stat}>
                            <b data-br>{status === 'ready' ? <CountUp value={nSecrets} /> : '—'}</b>
                            {t('kv2.stats.secrets')}
                        </div>
                        <div className={styles.stat}>
                            <b>{status === 'ready' ? <CountUp value={nFolders} /> : '—'}</b>
                            {t('kv2.stats.folders')}
                        </div>
                    </div>
                </div>

                <div className={`${styles.bar} ${styles.rv}`} style={delay(80)}>
                    <nav aria-label={t('kv2.crumbs')} className={styles.crumbs}>
                        {crumbs.map((c, i) => {
                            const last = i === crumbs.length - 1
                            const target = i === 0 ? '' : `${crumbs.slice(1, i + 1).join('/')}/`
                            return (
                                <span key={`${i}-${c}`} style={{ display: 'contents' }}>
                                    {i > 0 && <span className={styles.sl}>/</span>}
                                    <button
                                        aria-current={last ? 'page' : undefined}
                                        onClick={() => !last && go(target, 'back')}
                                        type="button"
                                    >
                                        {i === 0 && <Icon name="lock" size={14} />}
                                        {c}
                                    </button>
                                </span>
                            )
                        })}
                    </nav>
                    <Button leftSection={<Icon name="plus" size={15} />} onClick={() => openWrite()}>
                        {t('kv2.writeBtn')}
                    </Button>
                </div>

                <div className={`${styles.tools} ${styles.rv}`} style={delay(140)}>
                    <form
                        onSubmit={(e) => {
                            e.preventDefault()
                            doJump()
                        }}
                        style={{ display: 'contents' }}
                    >
                        <div className={common.inp}>
                            <Icon name="right" size={15} style={{ color: 'var(--dim)' }} />
                            <input
                                aria-label={t('kv2.jump.label')}
                                autoComplete="off"
                                onChange={(e) => setJump(e.currentTarget.value)}
                                placeholder={t('kv2.jump.placeholder')}
                                spellCheck={false}
                                value={jump}
                            />
                        </div>
                        <Button type="submit" variant="default">
                            {t('kv2.jump.go')}
                        </Button>
                    </form>
                    <div className={`${common.inp} ${styles.grow2}`}>
                        <Icon name="search" size={15} style={{ color: 'var(--dim)' }} />
                        <input
                            aria-label={t('kv2.filter.label')}
                            autoComplete="off"
                            onChange={(e) => setFilter(e.currentTarget.value)}
                            onKeyDown={(e) => {
                                if (e.key === 'Escape') {
                                    setFilter('')
                                    e.currentTarget.blur()
                                }
                            }}
                            placeholder={t('kv2.filter.placeholder')}
                            ref={filterRef}
                            spellCheck={false}
                            value={filter}
                        />
                        <span className={common.kbd}>/</span>
                    </div>
                    <div className={styles.chips} role="group">
                        {(
                            [
                                ['all', all.length],
                                ['folder', nFolders],
                                ['secret', nSecrets]
                            ] as Array<[Chip, number]>
                        ).map(([k, n]) => (
                            <button
                                className={styles.chip}
                                data-on={chip === k || undefined}
                                key={k}
                                onClick={() => setChip(k)}
                                type="button"
                            >
                                {t(`kv2.chip.${k}`)}
                                <em>{status === 'ready' ? n : '·'}</em>
                            </button>
                        ))}
                    </div>
                </div>

                <div className={styles.rv} style={delay(200)}>
                    <SecretList
                        all={all}
                        errorText={list.isError ? errMessage(list.error) : undefined}
                        filter={filter.trim()}
                        flash={flash}
                        onBulkDelete={() => setBulkOpen(true)}
                        onClearSel={() => setSelected(new Set())}
                        onOpenFolder={(p) => go(p)}
                        onOpenSecret={setDrawerPath}
                        onResetFilter={() => {
                            setFilter('')
                            setChip('all')
                        }}
                        onRetry={() => void list.refetch()}
                        onToggle={toggle}
                        onToggleAll={toggleAll}
                        onUp={up}
                        onWrite={() => openWrite()}
                        prefix={prefix}
                        retrying={list.isFetching}
                        selected={selected}
                        shown={shown}
                        slide={slide}
                        status={status}
                        still={still}
                    />
                </div>
            </div>

            <SecretDrawer
                onClose={() => setDrawerPath(null)}
                onEdit={(s) => openWrite(s)}
                path={drawerPath}
                suspended={writeOpen || bulkOpen}
            />

            <WriteModal
                folders={folderSet}
                onClose={() => setWriteOpen(false)}
                onSaved={onSaved}
                opened={writeOpen}
                prefix={prefix}
                seed={seed}
            />

            <ConfirmModal
                cancelLabel={t('kv2.cancel')}
                confirmLabel={t('kv2.bulk.confirm')}
                destructive
                message={t('kv2.bulk.text', { count: selected.size })}
                onClose={() => setBulkOpen(false)}
                onConfirm={bulkDelete}
                opened={bulkOpen}
                title={t('kv2.bulk.title', { count: selected.size })}
            />
        </Page>
    )
}
