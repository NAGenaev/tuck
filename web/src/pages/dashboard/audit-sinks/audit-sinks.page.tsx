// SPDX-License-Identifier: AGPL-3.0-only
import { Button, NumberInput, TextInput } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { CSSProperties, FormEvent, ReactNode, useEffect, useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { AuditSink, deleteAuditSink, listAuditSinks, putFileSink, putWebhookSink } from '@shared/api/endpoints/audit'
import {
    ConfirmModal,
    CopyButton,
    CountUp,
    Drawer,
    Icon,
    SkeletonLine,
    StateBlock,
    toast
} from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Plate, Pill } from '@shared/ui/tuck/tuck'

import styles from './audit-sinks.module.css'
import { buildBody, highlightJson, SinkType, sinkMeta, sinkTarget, urlWarning, validateSink } from './lib'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

const QUERY_KEY = ['audit', 'list']
const TYPE_TONE: Record<string, 'b' | 'c' | 'v'> = { file: 'b', webhook: 'c', syslog: 'v' }

function serverMessage(err: unknown): string | undefined {
    if (isAxiosError(err)) {
        const d = err.response?.data as { error?: string } | undefined
        return d?.error
    }
    return undefined
}

const rich = { b: <b />, br: <br />, code: <code />, em: <em /> }

function ShieldIcon() {
    return (
        <svg fill="none" stroke="currentColor" strokeWidth="1.8" viewBox="0 0 24 24">
            <path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z" />
            <path d="M9 12l2 2 4-4" />
        </svg>
    )
}

function FileIcon({ size = 22 }: { size?: number }) {
    return (
        <svg fill="none" height={size} stroke="currentColor" strokeWidth="1.8" viewBox="0 0 24 24" width={size}>
            <path d="M6 3h9l4 4v14H6z" />
            <path d="M9 12h7M9 16h7" />
        </svg>
    )
}

export function AuditSinksPage() {
    const { t } = useTranslation('audit')
    const queryClient = useQueryClient()

    const [creating, setCreating] = useState(false)
    const [toDelete, setToDelete] = useState<{ last: boolean; sink: AuditSink } | null>(null)
    const [dying, setDying] = useState<null | string>(null)
    const [fresh, setFresh] = useState<null | string>(null)

    // --- create form ---
    const [type, setType] = useState<SinkType>('webhook')
    const [name, setName] = useState('')
    const [url, setUrl] = useState('')
    const [timeoutSec, setTimeoutSec] = useState<number | string>(5)
    const [path, setPath] = useState('')
    const [maxSizeMb, setMaxSizeMb] = useState<number | string>(100)
    const [maxBackups, setMaxBackups] = useState<number | string>(7)
    const [touched, setTouched] = useState(false)
    const [serverErr, setServerErr] = useState('')
    const [shakeKey, setShakeKey] = useState(0)

    const {
        data: sinks,
        error,
        isError,
        isFetching,
        isLoading,
        refetch
    } = useQuery({
        queryKey: QUERY_KEY,
        queryFn: listAuditSinks,
        refetchInterval: 15_000,
        retry: (count, err) => !(isAxiosError(err) && err.response?.status === 403) && count < 2
    })

    const forbidden = isError && isAxiosError(error) && error.response?.status === 403
    const list = useMemo(() => [...(sinks ?? [])].sort((a, b) => a.name.localeCompare(b.name)), [sinks])
    const failing = useMemo(() => list.filter((s) => s.errors > 0), [list])
    const totalErrors = useMemo(() => list.reduce((a, s) => a + s.errors, 0), [list])
    const files = list.filter((s) => s.type === 'file').length
    const hooks = list.filter((s) => s.type === 'webhook').length
    const showData = !isLoading && !isError
    const none = showData && list.length === 0

    const fields = { type, name, url, timeoutSec, path, maxSizeMb, maxBackups }
    const errors = validateSink(fields, t)
    const visibleErrors = touched ? errors : {}
    const exists = list.some((s) => s.name === name.trim())
    const body = buildBody(fields)

    const resetForm = () => {
        setType('webhook')
        setName('')
        setUrl('')
        setTimeoutSec(5)
        setPath('')
        setMaxSizeMb(100)
        setMaxBackups(7)
        setTouched(false)
        setServerErr('')
    }

    const openCreate = () => {
        resetForm()
        setCreating(true)
    }

    const createMutation = useMutation({
        mutationFn: () => {
            const n = name.trim()
            return type === 'webhook'
                ? putWebhookSink(n, url.trim(), Number(timeoutSec))
                : putFileSink(n, path.trim(), Number(maxSizeMb), Number(maxBackups))
        },
        onSuccess: async () => {
            const created = name.trim()
            setCreating(false)
            setFresh(created)
            toast.success(t('toast.created'), t('toast.createdMsg', { name: created }))
            await queryClient.invalidateQueries({ queryKey: QUERY_KEY })
        },
        onError: (err) => {
            const msg = serverMessage(err)
            setServerErr(msg ?? '')
            toast.error(t('toast.createFailed'), msg)
        }
    })

    const submitCreate = (e?: FormEvent) => {
        e?.preventDefault()
        if (createMutation.isPending) return
        setTouched(true)
        if (Object.keys(errors).length > 0) {
            setShakeKey((k) => k + 1)
            return
        }
        setServerErr('')
        createMutation.mutate()
    }

    useEffect(() => {
        if (!fresh) return
        const el = document.querySelector(`[data-sink="${CSS.escape(fresh)}"]`)
        if (el) {
            const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
            el.scrollIntoView({ block: 'center', behavior: reduce ? 'auto' : 'smooth' })
            const id = window.setTimeout(() => setFresh(null), 2200)
            return () => window.clearTimeout(id)
        }
    }, [fresh, list])

    // --- delete ---
    const deleteMutation = useMutation({ mutationFn: deleteAuditSink })

    const confirmDelete = async (sink: AuditSink) => {
        setDying(sink.name)
        try {
            await deleteMutation.mutateAsync(sink.name)
        } catch (err) {
            setDying(null)
            toast.error(t('toast.deleteFailed'), serverMessage(err))
            throw err
        }
        toast.success(t('toast.deleted'), t('toast.deletedMsg', { name: sink.name }))
        await queryClient.invalidateQueries({ queryKey: QUERY_KEY })
        setDying(null)
    }

    const isLast = toDelete?.last ?? false

    // --- body ---
    let content: ReactNode
    if (isLoading) {
        content = (
            <>
                <section aria-busy className={styles.tiles}>
                    {[0, 1, 2].map((i) => (
                        <div className={cx(styles.tile, styles.tileSk)} key={i}>
                            <SkeletonLine width="45%" />
                            <SkeletonLine height={38} width="50%" />
                            <SkeletonLine width="70%" />
                        </div>
                    ))}
                </section>
                <Plate>
                    <div className={styles.plateTop}>
                        <h3>{t('plate.title')}</h3>
                    </div>
                    <div className={styles.scrollX}>
                        {[0, 1, 2].map((i) => (
                            <div className={styles.skRow} key={i}>
                                <div>
                                    <SkeletonLine width="60%" />
                                    <SkeletonLine height={10} style={{ marginTop: 8 }} width="90%" />
                                </div>
                                <SkeletonLine />
                                <SkeletonLine />
                                <SkeletonLine />
                            </div>
                        ))}
                    </div>
                    <div className={styles.bootLine}>
                        GET /v1/sys/audit/?list=true … {t('loading')}
                        <i />
                    </div>
                </Plate>
            </>
        )
    } else if (forbidden) {
        content = (
            <Plate>
                <div className={styles.forb}>
                    <div className={styles.big403}>403</div>
                    <div>
                        <Eyebrow className={styles.alarmEyebrow}>{t('forbidden.eyebrow')}</Eyebrow>
                        <h2>{t('forbidden.title')}</h2>
                        <p>{t('forbidden.text')}</p>
                        <div className={styles.reqs}>
                            <div>
                                <b>{t('forbidden.request')}</b>
                                <code>GET /v1/sys/audit/?list=true</code>
                            </div>
                            <div>
                                <b>{t('forbidden.need')}</b>
                                <code>path &quot;sys/audit/*&quot; {'{ capabilities = ["read","list"] }'}</code>
                            </div>
                        </div>
                        <Button
                            leftSection={<Icon name="refresh" size={14} />}
                            loading={isFetching}
                            onClick={() => void refetch()}
                            variant="default"
                        >
                            {t('forbidden.retry')}
                        </Button>
                    </div>
                </div>
            </Plate>
        )
    } else if (isError) {
        content = (
            <StateBlock
                details={serverMessage(error) ?? (error as Error)?.message}
                kind="error"
                message={t('error.message')}
                onRetry={() => void refetch()}
                retrying={isFetching}
                title={t('error.title')}
            />
        )
    } else {
        content = (
            <>
                <section aria-label={t('tiles.label')} className={styles.tiles}>
                    <div className={styles.tile}>
                        <small>{t('tiles.sinks')}</small>
                        <strong>
                            <CountUp value={list.length} />
                        </strong>
                        <p>{t('tiles.sinksSub', { files, hooks })}</p>
                    </div>
                    <div className={cx(styles.tile, failing.length > 0 && styles.bad)}>
                        <small>{t('tiles.failing')}</small>
                        <strong>
                            <CountUp value={failing.length} />
                        </strong>
                        <p>{failing.length > 0 ? t('tiles.failingBad') : t('tiles.failingOk')}</p>
                    </div>
                    <div className={cx(styles.tile, totalErrors > 0 && styles.bad)}>
                        <small>{t('tiles.errors')}</small>
                        <strong>
                            <CountUp value={totalErrors} />
                        </strong>
                        <p>{totalErrors > 0 ? t('tiles.errorsBad') : t('tiles.errorsOk')}</p>
                    </div>
                </section>

                {none && (
                    <div className={styles.banner} role="alert">
                        <div className={styles.haz} />
                        <div className={styles.bannerIn}>
                            <div>
                                <h2>{t('banner.title')}</h2>
                                <p>
                                    <Trans components={rich} i18nKey="banner.text" ns="audit" />
                                </p>
                                <Button leftSection={<Icon name="plus" size={14} />} onClick={openCreate}>
                                    {t('banner.cta')}
                                </Button>
                            </div>
                            <ul>
                                <li>
                                    <span>
                                        <Trans components={rich} i18nKey="banner.li1" ns="audit" />
                                    </span>
                                </li>
                                <li>{t('banner.li2')}</li>
                                <li>{t('banner.li3')}</li>
                                <li>{t('banner.li4')}</li>
                            </ul>
                        </div>
                    </div>
                )}

                {failing.length > 0 && (
                    <div className={styles.alert} role="alert">
                        <i className={cx(styles.dot, styles.fail)} />
                        <div>
                            <b>{t('alert.title')}</b>
                            <span>
                                {t('alert.text', { name: failing[0].name, count: failing[0].errors })}
                                {failing.length > 1 && ` ${t('alert.more', { count: failing.length - 1 })}`}
                            </span>
                        </div>
                        <Button
                            color="red"
                            leftSection={<Icon name="refresh" size={14} />}
                            loading={isFetching}
                            onClick={() => void refetch()}
                            size="xs"
                            variant="default"
                        >
                            {t('alert.refresh')}
                        </Button>
                    </div>
                )}

                <section aria-label={t('plate.title')} className={styles.sec}>
                    <Plate>
                        <div className={styles.plateTop}>
                            <h3>{t('plate.title')}</h3>
                            <Pill tone="n">{list.length}</Pill>
                            <div className={styles.sp}>
                                <Button
                                    leftSection={<Icon name="plus" size={14} />}
                                    onClick={openCreate}
                                    size="xs"
                                    variant="default"
                                >
                                    {t('plate.new')}
                                </Button>
                            </div>
                        </div>
                        <div className={styles.scrollX}>
                            <div className={styles.shead}>
                                <div>{t('cols.name')}</div>
                                <div>{t('cols.type')}</div>
                                <div>{t('cols.status')}</div>
                                <div>{t('cols.params')}</div>
                                <div>{t('cols.errors')}</div>
                                <div />
                            </div>
                            {list.length === 0 ? (
                                <div className={styles.sinkEmpty}>
                                    <FileIcon size={34} />
                                    <br />
                                    {t('plate.empty')}
                                </div>
                            ) : (
                                list.map((sink, i) => {
                                    const bad = sink.errors > 0
                                    const target = sinkTarget(sink)
                                    return (
                                        <div
                                            className={cx(
                                                styles.srow,
                                                bad && styles.fail,
                                                fresh === sink.name && styles.fresh,
                                                dying === sink.name && styles.dying
                                            )}
                                            data-sink={sink.name}
                                            key={sink.name}
                                            style={{ '--i': Math.min(i, 10) } as CSSProperties}
                                        >
                                            <div className={styles.cName}>
                                                <b>{sink.name}</b>
                                                <div className={styles.tgt}>
                                                    <span>{target}</span>
                                                    {target !== '—' && (
                                                        <CopyButton label={t('row.copy')} size={24} value={target} />
                                                    )}
                                                </div>
                                                {bad && (
                                                    <div className={styles.errline}>
                                                        {t('row.errline', { count: sink.errors })}
                                                    </div>
                                                )}
                                            </div>
                                            <div className={styles.cType}>
                                                <Pill tone={TYPE_TONE[sink.type] ?? 'n'}>{sink.type}</Pill>
                                                <Pill tone="n">json</Pill>
                                            </div>
                                            <div className={cx(styles.st, bad ? styles.r : styles.g)}>
                                                <i className={cx(styles.dot, bad && styles.fail)} />
                                                {bad ? t('status.fail') : t('status.ok')}
                                            </div>
                                            <div className={styles.cMeta} data-l={t('cols.params')}>
                                                {sinkMeta(sink, t)}
                                            </div>
                                            <div
                                                className={cx(styles.cErr, bad && styles.bad)}
                                                data-l={t('cols.errors')}
                                            >
                                                {sink.errors.toLocaleString()}
                                            </div>
                                            <div>
                                                <button
                                                    aria-label={t('row.remove', { name: sink.name })}
                                                    className={styles.ib}
                                                    disabled={dying === sink.name}
                                                    onClick={() => setToDelete({ sink, last: list.length === 1 })}
                                                    title={t('row.remove', { name: sink.name })}
                                                    type="button"
                                                >
                                                    <Icon name="trash" size={15} />
                                                </button>
                                            </div>
                                        </div>
                                    )
                                })
                            )}
                        </div>
                        <div className={styles.guard}>
                            <ShieldIcon />
                            <div>
                                <b>{t('guard.title')}</b>
                                <ul>
                                    <li>
                                        <span>
                                            <Trans components={rich} i18nKey="guard.g1" ns="audit" />
                                        </span>
                                    </li>
                                    <li>
                                        <span>
                                            <Trans components={rich} i18nKey="guard.g2" ns="audit" />
                                        </span>
                                    </li>
                                    <li>
                                        <span>
                                            <Trans components={rich} i18nKey="guard.g3" ns="audit" />
                                        </span>
                                    </li>
                                </ul>
                            </div>
                        </div>
                    </Plate>
                </section>
            </>
        )
    }

    const nameError = visibleErrors.name || ''

    return (
        <Page title={t('title')}>
            <div className={styles.phead}>
                <div>
                    <Eyebrow className={styles.rv}>{t('eyebrow')}</Eyebrow>
                    <h1 className={styles.h1}>
                        <span>{t('h1a')}</span>
                        <span className={styles.gold}>{t('h1b')}</span>
                    </h1>
                    <p className={cx(styles.lead, styles.rv)} style={{ '--d': 3 } as CSSProperties}>
                        <Trans components={rich} i18nKey="lead" ns="audit" />
                    </p>
                </div>
                {!forbidden && (
                    <div className={styles.rv} style={{ '--d': 4 } as CSSProperties}>
                        <Button leftSection={<Icon name="plus" size={14} />} onClick={openCreate}>
                            {t('create')}
                        </Button>
                    </div>
                )}
            </div>

            <div className={styles.rv} style={{ '--d': 5 } as CSSProperties}>
                {content}
            </div>

            <Drawer
                busy={createMutation.isPending}
                footer={
                    <>
                        <Button disabled={createMutation.isPending} onClick={() => setCreating(false)} variant="default">
                            {t('drawer.cancel')}
                        </Button>
                        <Button
                            leftSection={<Icon name="plus" size={14} />}
                            loading={createMutation.isPending}
                            onClick={() => submitCreate()}
                        >
                            {t('drawer.submit')}
                        </Button>
                    </>
                }
                onClose={() => setCreating(false)}
                opened={creating}
                title={t('drawer.title')}
            >
                <form className={styles.form} noValidate onSubmit={submitCreate}>
                    <p className={styles.hint}>{t('drawer.hint')}</p>

                    <div className={styles.f}>
                        <span className={styles.lb}>{t('drawer.type')}</span>
                        <div aria-label={t('drawer.type')} className={styles.seg} role="radiogroup">
                            {(['webhook', 'file'] as const).map((v) => (
                                <button
                                    aria-checked={type === v}
                                    className={type === v ? styles.on : undefined}
                                    key={v}
                                    onClick={() => setType(v)}
                                    role="radio"
                                    type="button"
                                >
                                    {t(v === 'file' ? 'drawer.typeFile' : 'drawer.typeWebhook')}
                                </button>
                            ))}
                        </div>
                    </div>

                    <div className={nameError ? styles.shake : undefined} key={`n${shakeKey}`}>
                        <TextInput
                            autoComplete="off"
                            data-autofocus
                            error={nameError || undefined}
                            label={t('drawer.name')}
                            onChange={(e) => {
                                setName(e.currentTarget.value)
                                setServerErr('')
                            }}
                            placeholder={t('drawer.namePlaceholder')}
                            spellCheck={false}
                            value={name}
                        />
                    </div>
                    {exists && !nameError && <p className={styles.note}>{t('drawer.overwrite')}</p>}

                    {type === 'webhook' ? (
                        <>
                            <TextInput
                                autoComplete="off"
                                error={visibleErrors.url || undefined}
                                label={t('drawer.url')}
                                onChange={(e) => setUrl(e.currentTarget.value)}
                                placeholder={t('drawer.urlPlaceholder')}
                                spellCheck={false}
                                value={url}
                            />
                            {!errors.url && urlWarning(url, t) && <p className={styles.note}>{urlWarning(url, t)}</p>}
                            <div className={styles.f}>
                                <NumberInput
                                    error={visibleErrors.timeout || undefined}
                                    label={t('drawer.timeout')}
                                    onChange={setTimeoutSec}
                                    value={timeoutSec}
                                />
                                <small>{t('drawer.timeoutHint')}</small>
                            </div>
                        </>
                    ) : (
                        <>
                            <TextInput
                                autoComplete="off"
                                error={visibleErrors.path || undefined}
                                label={t('drawer.path')}
                                onChange={(e) => setPath(e.currentTarget.value)}
                                placeholder={t('drawer.pathPlaceholder')}
                                spellCheck={false}
                                value={path}
                            />
                            <div className={styles.fr}>
                                <NumberInput
                                    error={visibleErrors.size || undefined}
                                    label={t('drawer.size')}
                                    onChange={setMaxSizeMb}
                                    value={maxSizeMb}
                                />
                                <NumberInput
                                    error={visibleErrors.backups || undefined}
                                    label={t('drawer.backups')}
                                    onChange={setMaxBackups}
                                    value={maxBackups}
                                />
                            </div>
                        </>
                    )}

                    <div className={styles.f}>
                        <span className={styles.lb}>{t('drawer.format')}</span>
                        <div className={styles.lock}>
                            <Icon name="lock" size={14} />
                            <span>
                                <b>json</b> · {t('drawer.formatText')}
                            </span>
                        </div>
                    </div>

                    <div className={styles.warnbox}>
                        <Icon name="warn" size={20} />
                        <div>
                            <Trans components={rich} i18nKey="drawer.warn" ns="audit" />
                        </div>
                    </div>

                    <div className={styles.f}>
                        <span className={styles.lb}>{t('drawer.preview')}</span>
                        <div className={styles.prev}>
                            <span className={styles.jk}>PUT</span> {body.url}
                            {'\n'}
                            {highlightJson(JSON.stringify(body.json, null, 2), styles)}
                        </div>
                    </div>

                    {serverErr && <p className={styles.note}>{serverErr}</p>}
                </form>
            </Drawer>

            <ConfirmModal
                confirmLabel={t('delete.confirm')}
                confirmText={isLast ? toDelete?.sink.name : undefined}
                message={
                    toDelete && (
                        <>
                            {t('delete.message', { name: toDelete.sink.name })}
                            {isLast && (
                                <div className={styles.stop}>
                                    <Trans components={rich} i18nKey="delete.last" ns="audit" />
                                </div>
                            )}
                        </>
                    )
                }
                onClose={() => setToDelete(null)}
                onConfirm={() => confirmDelete((toDelete as { sink: AuditSink }).sink)}
                opened={toDelete !== null}
                title={t('delete.title')}
            />
        </Page>
    )
}
