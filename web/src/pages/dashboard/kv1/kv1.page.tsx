// SPDX-License-Identifier: AGPL-3.0-only
import { Button, TextInput } from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { listKVv1 } from '@shared/api/endpoints/kv'
import { Icon, prefersReducedMotion, toast } from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow } from '@shared/ui/tuck/tuck'

import { DrawerInit, WriteDrawer } from './components/write-drawer'
import { DetailPlaceholder, SecretDetail } from './components/secret-detail'
import { SecretList } from './components/secret-list'
import styles from './kv1.module.css'
import { dirOf, errStatus, sleep } from './lib'

export function KV1Page() {
    const { t } = useTranslation('kv1')
    const queryClient = useQueryClient()
    const [prefix, setPrefix] = useState('')
    const [jump, setJump] = useState('')
    const [selected, setSelected] = useState<null | string>(null)
    const [flash, setFlash] = useState<null | string>(null)
    const [leaving, setLeaving] = useState<null | string>(null)
    const [drawer, setDrawer] = useState<{ init: DrawerInit; open: boolean; seq: number } | null>(null)

    const list = useQuery({
        queryKey: ['kv1', 'list', prefix],
        queryFn: async () => {
            try {
                return await listKVv1(prefix)
            } catch (e) {
                if (errStatus(e) === 404) return []
                throw e
            }
        },
        retry: false
    })

    // folders first, then keys
    const keys = useMemo(() => {
        if (!list.data) return undefined
        const dirs = list.data.filter((k) => k.endsWith('/')).sort()
        const files = list.data.filter((k) => !k.endsWith('/')).sort()
        return [...dirs, ...files]
    }, [list.data])

    const crumbs = ['secret', ...prefix.split('/').filter(Boolean)]

    const goUp = () => {
        const p = prefix.split('/').filter(Boolean)
        p.pop()
        setPrefix(p.length ? `${p.join('/')}/` : '')
    }

    const doJump = () => {
        const v = jump
            .trim()
            .replace(/^\/?(secret\/)?/, '')
            .replace(/\/+$/, '')
        setPrefix(v ? `${v}/` : '')
    }

    const openDrawer = (init: Partial<DrawerInit> = {}) =>
        setDrawer((d) => ({
            init: {
                base: prefix,
                listPrefix: prefix,
                name: '',
                siblings: keys ?? [],
                value: null,
                ...init
            },
            open: true,
            seq: (d?.seq ?? 0) + 1
        }))

    const closeDrawer = () => setDrawer((d) => (d ? { ...d, open: false } : d))

    const onSaved = (full: string, overwrote: boolean) => {
        closeDrawer()
        const dir = dirOf(full)
        setPrefix(dir)
        setSelected(full)
        setFlash(full)
        window.setTimeout(() => setFlash(null), 1600)
        void queryClient.invalidateQueries({ queryKey: ['kv1', 'list'] })
        void queryClient.invalidateQueries({ queryKey: ['kv1', 'get', full] })
        toast.success(
            overwrote ? t('overwritten') : t('saved'),
            t(overwrote ? 'overwrittenText' : 'savedText', { path: `secret/${full}` })
        )
    }

    const onDeleted = async (path: string) => {
        setLeaving(path)
        await sleep(prefersReducedMotion() ? 30 : 520)
        setLeaving(null)
        setSelected((s) => (s === path ? null : s))
        queryClient.removeQueries({ queryKey: ['kv1', 'get', path] })
        void queryClient.invalidateQueries({ queryKey: ['kv1', 'list'] })
        toast.success(t('deleted'), t('deletedText', { path: `secret/${path}` }))
    }

    return (
        <Page title="KV v1">
            <div className={styles.page}>
                <section className={styles.intro}>
                    <div>
                        <Eyebrow className={`${styles.reveal} ${styles.eyebrowGap}`}>{t('eyebrow')}</Eyebrow>
                        <h1 className={`${styles.h1} ${styles.reveal}`} style={{ ['--i' as string]: 1 }}>
                            KV <span className={styles.ice}>v1</span>
                            <small>{t('subtitle')}</small>
                        </h1>
                    </div>
                    <div className={`${styles.warn} ${styles.reveal}`} role="note" style={{ ['--i' as string]: 3 }}>
                        <div className={styles.sign}>
                            <Icon name="warn" size={22} />
                        </div>
                        <h3>
                            {t('warnTitle')} <b>·</b> {t('warnTitle2')}
                        </h3>
                        <p>
                            <Trans components={{ b: <b /> }} i18nKey="warnText" ns="kv1" />
                        </p>
                    </div>
                </section>

                <div className={`${styles.bar} ${styles.reveal}`} style={{ ['--i' as string]: 5 }}>
                    <nav aria-label={t('pathAria')} className={styles.crumbs}>
                        {crumbs.map((c, i) => (
                            <span key={i} style={{ display: 'contents' }}>
                                {i > 0 && <span className={styles.sep}>/</span>}
                                <button
                                    aria-current={i === crumbs.length - 1 ? 'page' : undefined}
                                    onClick={() => {
                                        if (i !== crumbs.length - 1)
                                            setPrefix(i === 0 ? '' : `${crumbs.slice(1, i + 1).join('/')}/`)
                                    }}
                                    type="button"
                                >
                                    {c}
                                </button>
                            </span>
                        ))}
                    </nav>
                    <Button leftSection={<Icon name="plus" size={15} />} onClick={() => openDrawer()}>
                        {t('write')}
                    </Button>
                </div>

                <div className={`${styles.jump} ${styles.reveal}`} style={{ ['--i' as string]: 6 }}>
                    <TextInput
                        autoComplete="off"
                        onChange={(e) => setJump(e.currentTarget.value)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') doJump()
                        }}
                        placeholder={t('jumpPlaceholder')}
                        spellCheck={false}
                        styles={{ input: { fontFamily: 'var(--mono)' } }}
                        value={jump}
                    />
                    <Button onClick={doJump} variant="default">
                        {t('jump')}
                    </Button>
                </div>

                <div className={styles.grid}>
                    <div className={styles.reveal} style={{ ['--i' as string]: 7 }}>
                        <SecretList
                            error={list.error}
                            fetching={list.isFetching}
                            flash={flash}
                            keys={keys}
                            leaving={leaving}
                            loading={list.isLoading}
                            onFolder={(k) => setPrefix(prefix + k)}
                            onRetry={() => void list.refetch()}
                            onSelect={(k) => setSelected(prefix + k)}
                            onUp={goUp}
                            onWrite={() => openDrawer()}
                            prefix={prefix}
                            selected={selected}
                        />
                    </div>
                    <div className={styles.reveal} style={{ ['--i' as string]: 8 }}>
                        {selected ? (
                            <SecretDetail
                                key={selected}
                                onDeleted={(p) => void onDeleted(p)}
                                onOverwrite={(p, value) => openDrawer({ base: '', name: p, value })}
                                path={selected}
                            />
                        ) : (
                            <DetailPlaceholder />
                        )}
                    </div>
                </div>
            </div>

            {drawer && (
                <WriteDrawer
                    init={drawer.init}
                    key={drawer.seq}
                    onClose={closeDrawer}
                    onSaved={onSaved}
                    open={drawer.open}
                />
            )}
        </Page>
    )
}
