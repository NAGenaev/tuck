// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { CSSProperties, FormEvent, useEffect, useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { getAuditStats } from '@shared/api/endpoints/audit'
import { rotate, seal, snapshot, unseal } from '@shared/api/endpoints/sys'
import { listTokens } from '@shared/api/endpoints/token'
import { getSecretsUsage } from '@shared/api/endpoints/usage'
import { useHealth } from '@shared/api/hooks/use-health'
import { useSealStatus } from '@shared/api/hooks/use-seal-status'
import {
    ConfirmModal,
    CopyButton,
    Icon,
    Modal,
    SkeletonLine,
    toast
} from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Plate } from '@shared/ui/tuck/tuck'

import { ActivityTile } from './components/activity-tile'
import { Emblem } from './components/emblem'
import { HaSection } from './components/ha-section'
import { formatClock, useDecode, useUptime, ViewState } from './lib'
import styles from './status.module.css'

const rich = { b: <b /> }
const ACTIVITY_POLL = 10_000

function serverMessage(err: unknown): string | undefined {
    if (isAxiosError(err)) {
        const d = err.response?.data as { error?: string } | undefined
        return d?.error ?? err.message
    }
    return err instanceof Error ? err.message : undefined
}

type Msg = { text: string; tone: 'bad' | 'mute' | 'ok' } | null

export function StatusPage() {
    const { t } = useTranslation(['status', 'translation'])
    const queryClient = useQueryClient()

    const sealQuery = useSealStatus()
    const healthQuery = useHealth()
    const sealStatus = sealQuery.data
    const health = healthQuery.data

    const sealed = sealStatus?.sealed === true
    const unsealedKnown = sealStatus?.sealed === false
    const type = sealStatus?.type
    const required = sealStatus?.required_shards ?? 0
    const received = sealStatus?.received_shards ?? 0

    let view: ViewState
    if (sealQuery.isError) view = 'error'
    else if (!sealStatus) view = 'loading'
    else if (sealed) view = type === 'shamir' ? 'sealed' : 'auto'
    else view = 'ok'

    const off = view === 'loading' || view === 'error'

    // --- activity (needs an unsealed vault) ---
    const tokensQuery = useQuery({
        queryKey: ['tokens', 'list'],
        queryFn: listTokens,
        enabled: unsealedKnown,
        refetchInterval: ACTIVITY_POLL,
        retry: false
    })

    // secrets tile: needs list on secret/; without it the tile is not shown at all
    const secretsQuery = useQuery({
        queryKey: ['sys', 'usage', 'secrets'],
        queryFn: getSecretsUsage,
        enabled: unsealedKnown,
        refetchInterval: ACTIVITY_POLL,
        retry: false
    })
    const secretsForbidden =
        secretsQuery.isError && isAxiosError(secretsQuery.error) && secretsQuery.error.response?.status === 403

    // audit rate tile: needs a root-namespace token with a sys/audit/* read policy; hidden otherwise
    const auditRateQuery = useQuery({
        queryKey: ['audit', 'stats', 'rate'],
        queryFn: () => getAuditStats(1),
        enabled: unsealedKnown,
        refetchInterval: ACTIVITY_POLL,
        retry: false
    })
    const auditForbidden =
        auditRateQuery.isError && isAxiosError(auditRateQuery.error) && auditRateQuery.error.response?.status === 403

    const deadFor = (isError: boolean): 'error' | 'sealed' | null => {
        if (view === 'sealed' || view === 'auto') return 'sealed'
        if (view === 'error' || isError) return 'error'
        return null
    }


    const tileCount = 1 + (secretsForbidden ? 0 : 1) + (auditForbidden ? 0 : 1)
    const tileW = tileCount === 3 ? styles.w2 : styles.w3

    // --- unseal form ---
    const [shard, setShard] = useState('')
    const [showShard, setShowShard] = useState(false)
    const [bad, setBad] = useState(false)
    const [msg, setMsg] = useState<Msg>(null)
    const [pop, setPop] = useState<null | number>(null)
    const [burstKey, setBurstKey] = useState(0)
    const [shake, setShake] = useState(false)

    // --- modals ---
    const [rotateOpen, setRotateOpen] = useState(false)
    const [sealOpen, setSealOpen] = useState(false)
    const [sealAck, setSealAck] = useState(false)
    const [sealAckErr, setSealAckErr] = useState(false)

    const refreshAfterStateChange = async () => {
        await queryClient.invalidateQueries({ queryKey: ['sys'] })
        await queryClient.invalidateQueries({ queryKey: ['tokens', 'list'] })
    }

    // state transitions: burst on unseal, pop on a newly accepted share
    const prevView = useRef<ViewState>(view)
    const prevReceived = useRef(received)
    useEffect(() => {
        const was = prevView.current
        if ((was === 'sealed' || was === 'auto') && view === 'ok') {
            setBurstKey((k) => k + 1)
            setShake(true)
            const id = window.setTimeout(() => setShake(false), 520)
            setMsg(null)
            prevView.current = view
            return () => window.clearTimeout(id)
        }
        prevView.current = view
    }, [view])

    useEffect(() => {
        if (view === 'sealed' && received > prevReceived.current) {
            setPop(received - 1)
            const id = window.setTimeout(() => setPop(null), 900)
            prevReceived.current = received
            return () => window.clearTimeout(id)
        }
        prevReceived.current = received
    }, [received, view])

    const unsealMutation = useMutation({
        mutationFn: unseal,
        onMutate: () => setMsg({ text: t(type === 'shamir' ? 'shamir.checking' : 'auto.requesting', { type }), tone: 'mute' }),
        onSuccess: async (res) => {
            setShard('')
            if (res.sealed === false) {
                setMsg({ text: t('shamir.assembling'), tone: 'ok' })
                toast.success(t('toast.unsealed'), t('toast.unsealedMsg'))
            } else if (type === 'shamir') {
                const got = res.received_shards ?? 0
                const need = res.required_shards ?? required
                setMsg({ text: t('shamir.accepted', { got, need, left: Math.max(0, need - got) }), tone: 'ok' })
                toast.warning(t('shamir.toastAccepted'), t('shamir.toastAcceptedMsg', { got, need }))
            }
            queryClient.setQueryData(['sys', 'seal-status'], res)
            await refreshAfterStateChange()
        },
        onError: (err) => {
            setBad(true)
            setMsg({ text: serverMessage(err) ?? t('shamir.failed'), tone: 'bad' })
            toast.error(t('shamir.failed'), serverMessage(err))
        }
    })

    const submitShard = (e?: FormEvent) => {
        e?.preventDefault()
        if (unsealMutation.isPending) return
        const value = shard.trim()
        if (!value) {
            setBad(true)
            setMsg({ text: t('shamir.empty'), tone: 'bad' })
            return
        }
        setBad(false)
        unsealMutation.mutate(value)
    }

    const tryAuto = () => {
        if (unsealMutation.isPending) return
        unsealMutation.mutate('')
    }

    // --- rotate / seal / snapshot ---
    const rotateMutation = useMutation({
        mutationFn: rotate,
        onSuccess: () => toast.success(t('toast.rotated'), t('toast.rotatedMsg')),
        onError: (err) => toast.error(t('toast.rotateFailed'), serverMessage(err))
    })

    const sealMutation = useMutation({
        mutationFn: seal,
        onSuccess: async () => {
            setSealOpen(false)
            setMsg(null)
            toast.warning(t('toast.sealed'), t('toast.sealedMsg'))
            await refreshAfterStateChange()
        },
        onError: (err) => toast.error(t('toast.sealFailed'), serverMessage(err))
    })

    const snapshotMutation = useMutation({
        mutationFn: async () => {
            const blob = await snapshot()
            const url = URL.createObjectURL(blob)
            const a = document.createElement('a')
            a.href = url
            a.download = 'tuck-snapshot.snap'
            a.click()
            URL.revokeObjectURL(url)
        },
        onSuccess: () => toast.success(t('toast.snapshotReady')),
        onError: (err) => toast.error(t('toast.snapshotFailed'), serverMessage(err))
    })

    const openSeal = () => {
        setSealAck(false)
        setSealAckErr(false)
        setSealOpen(true)
    }

    const confirmSeal = () => {
        if (!sealAck) {
            setSealAckErr(true)
            return
        }
        sealMutation.mutate()
    }

    const [retrying, setRetrying] = useState(false)
    const retryApi = async () => {
        setRetrying(true)
        try {
            const [s] = await Promise.all([sealQuery.refetch(), healthQuery.refetch()])
            if (s.isError) toast.error(t('toast.stillDown'), t('toast.stillDownMsg'))
            else toast.success(t('toast.reconnected'), t('toast.reconnectedMsg'))
        } finally {
            setRetrying(false)
        }
    }

    // --- derived view ---
    const word = useDecode(t(`state.${view}`).toUpperCase())
    const uptime = useUptime(health?.uptime_seconds, healthQuery.dataUpdatedAt)

    const shardsVisible = required > 0 && view !== 'error' && view !== 'auto'
    const shardState = (i: number): '' | 'done' | 'on' => {
        if (view === 'ok') return 'done'
        if (view === 'sealed' && i < received) return 'on'
        return ''
    }
    const progress = view === 'ok' ? 1 : view === 'sealed' ? (required ? received / required : 0) : view === 'loading' ? 0.2 : 0

    const modeKey = type === 'shamir' ? 'facts.modeManual' : type === 'dev' ? 'facts.modeDev' : 'facts.modeAuto'

    const fact = (v: string) =>
        view === 'loading' ? <SkeletonLine height={14} width="70%" /> : view === 'error' ? '—' : v

    const lastOk = sealQuery.dataUpdatedAt || healthQuery.dataUpdatedAt

    return (
        <Page title={t('title')}>
            <div className={styles.root} data-st={view}>
                <div aria-hidden className={styles.glow} />

                <div className={`${styles.phead} ${styles.rv}`}>
                    <Eyebrow>{t('eyebrow')}</Eyebrow>
                    <h1 className={styles.h1}>{t('title')}</h1>
                </div>

                {view === 'error' && (
                    <div className={`${styles.banner} ${styles.rv}`} role="alert" style={{ '--d': 1 } as CSSProperties}>
                        <div className={styles.grow}>
                            <h3>{t('banner.title')}</h3>
                            <p>
                                {t('banner.text')}
                                {serverMessage(sealQuery.error) && (
                                    <>
                                        <br />
                                        {serverMessage(sealQuery.error)}
                                    </>
                                )}
                                <br />
                                {lastOk ? t('banner.last', { time: formatClock(lastOk) }) : t('banner.never')}
                            </p>
                        </div>
                        <Button loading={retrying} onClick={() => void retryApi()}>
                            {t('banner.retry')}
                        </Button>
                    </div>
                )}

                <Plate className={`${styles.hero} ${styles.rv} ${shake ? styles.heroShake : ''}`} style={{ '--d': 2 } as CSSProperties}>
                    <div className={styles.flash} data-go={burstKey > 0} key={burstKey} />

                    <Emblem burstKey={burstKey} progress={progress} />

                    <div className={styles.heroBody}>
                        <Eyebrow>{t('hero.eyebrow')}</Eyebrow>
                        <h2 className={styles.word}>
                            {view === 'loading' ? <SkeletonLine height="0.8em" width="70%" /> : word}
                        </h2>

                        <div className={styles.facts}>
                            <div>
                                <small>{t('facts.type')}</small>
                                <b className={styles.k}>
                                    {fact(type ?? '—')}
                                </b>
                            </div>
                            <div>
                                <small>{t('facts.mode')}</small>
                                <b>{fact(type ? t(modeKey) : '—')}</b>
                            </div>
                            <div>
                                <small>{t('facts.threshold')}</small>
                                <b>{fact(required > 0 ? String(required) : '—')}</b>
                            </div>
                        </div>

                        {shardsVisible && (
                            <div>
                                <div className={styles.shardhead}>
                                    <span>{t('shards.title')}</span>
                                    <b>{`${view === 'ok' ? required : received} / ${required}`}</b>
                                </div>
                                <div className={styles.shards} style={{ '--n': Math.min(required, 12) } as CSSProperties}>
                                    {Array.from({ length: required }, (_, i) => (
                                        <i
                                            data-pop={pop === i || undefined}
                                            data-s={shardState(i) || undefined}
                                            key={i}
                                            style={{ transitionDelay: view === 'ok' ? `${i * 140}ms` : undefined }}
                                        />
                                    ))}
                                </div>
                                <div className={styles.shardfoot}>
                                    <span className={styles.th}>{t('shards.threshold', { n: required })}</span>
                                    <span>
                                        {view === 'sealed'
                                            ? t('shards.left', { n: Math.max(0, required - received) })
                                            : view === 'ok'
                                              ? t('shards.ready')
                                              : ''}
                                    </span>
                                </div>
                            </div>
                        )}

                        {view === 'ok' && (
                            <div className={styles.pane}>
                                <p>
                                    <Trans
                                        components={rich}
                                        i18nKey={type === 'shamir' ? 'ok.text' : 'okAuto.text'}
                                        ns="status"
                                    />
                                </p>
                            </div>
                        )}

                        {view === 'sealed' && (
                            <form className={styles.pane} noValidate onSubmit={submitShard}>
                                <p>
                                    <Trans components={rich} i18nKey="shamir.text" ns="status" />
                                </p>
                                <div className={styles.field} data-bad={bad || undefined}>
                                    <input
                                        aria-label={t('shamir.placeholder')}
                                        autoComplete="off"
                                        onChange={(e) => {
                                            setShard(e.currentTarget.value)
                                            setBad(false)
                                            setMsg(null)
                                        }}
                                        placeholder={t('shamir.placeholder')}
                                        spellCheck={false}
                                        type={showShard ? 'text' : 'password'}
                                        value={shard}
                                    />
                                    <button onClick={() => setShowShard((s) => !s)} type="button">
                                        {showShard ? t('shamir.hide') : t('shamir.show')}
                                    </button>
                                </div>
                                <div className={styles.msg} data-tone={msg?.tone === 'bad' ? undefined : msg?.tone} role="status">
                                    {msg?.text}
                                </div>
                                <div className={styles.row}>
                                    <Button
                                        leftSection={<Icon name="key" size={16} />}
                                        loading={unsealMutation.isPending}
                                        type="submit"
                                    >
                                        {t('shamir.submit')}
                                    </Button>
                                </div>
                            </form>
                        )}

                        {view === 'auto' && (
                            <div className={styles.pane}>
                                <p>{t('auto.text')}</p>
                                <div className={`${styles.facts} ${styles.factsFlat}`}>
                                    <div>
                                        <small>{t('auto.provider')}</small>
                                        <b>{type}</b>
                                    </div>
                                </div>
                                <div className={styles.msg} data-tone={msg?.tone === 'bad' ? undefined : msg?.tone} role="status">
                                    {msg?.text ?? ''}
                                </div>
                                <div className={styles.row}>
                                    <Button
                                        leftSection={<Icon name="refresh" size={16} />}
                                        loading={unsealMutation.isPending}
                                        onClick={tryAuto}
                                    >
                                        {t('auto.button')}
                                    </Button>
                                </div>
                            </div>
                        )}

                        <div className={styles.actions}>
                            <Button
                                disabled={off}
                                leftSection={<Icon name="download" size={16} />}
                                loading={snapshotMutation.isPending}
                                onClick={() => snapshotMutation.mutate()}
                                variant="default"
                            >
                                {snapshotMutation.isPending ? t('actions.snapshotBusy') : t('actions.snapshot')}
                            </Button>
                            <Button
                                disabled={off}
                                leftSection={<Icon name="refresh" size={16} />}
                                onClick={() => setRotateOpen(true)}
                                variant="default"
                            >
                                {t('actions.rotate')}
                            </Button>
                            {(view === 'ok' || off) && (
                                <Button
                                    color="red"
                                    disabled={off}
                                    leftSection={<Icon name="lock" size={16} />}
                                    onClick={openSeal}
                                    variant="light"
                                >
                                    {t('actions.seal')}
                                </Button>
                            )}
                        </div>
                    </div>
                </Plate>

                <section className={`${styles.sec} ${styles.rv}`} style={{ '--d': 3 } as CSSProperties}>
                    <div className={styles.sech}>
                        <Eyebrow>{t('server.eyebrow')}</Eyebrow>
                    </div>
                    <div className={styles.bento}>
                        <div className={`${styles.tile} ${styles.w2}`}>
                            <small>{t('server.version')}</small>
                            <div className={styles.val} data-m="true" data-tone="brass">
                                {healthQuery.isLoading ? <SkeletonLine height={24} width={110} /> : (health?.version ?? '—')}
                            </div>
                            <span className={styles.delta} data-tone="dim">
                                {health?.build_date ? t('server.build', { date: health.build_date }) : ''}
                            </span>
                        </div>
                        <div className={`${styles.tile} ${styles.w2}`}>
                            <small>
                                {t('server.commit')}
                                {health?.commit && <CopyButton size={24} value={health.commit} />}
                            </small>
                            <div className={styles.val} data-m="true">
                                {healthQuery.isLoading ? (
                                    <SkeletonLine height={24} width={110} />
                                ) : (
                                    (health?.commit?.slice(0, 8) ?? '—')
                                )}
                            </div>
                            <span className={styles.delta} data-tone="dim">
                                {t('server.commitMeta')}
                            </span>
                        </div>
                        <div className={`${styles.tile} ${styles.w2}`}>
                            <small>{t('server.uptime')}</small>
                            <div className={styles.val} data-m="true" data-tone={uptime ? 'ok' : undefined}>
                                {healthQuery.isLoading ? (
                                    <SkeletonLine height={24} width={150} />
                                ) : uptime ? (
                                    t('server.uptimeFmt', { d: uptime.d, hms: uptime.hms })
                                ) : (
                                    '—'
                                )}
                            </div>
                            <span className={styles.delta} data-tone="dim">
                                {t('server.uptimeMeta')}
                            </span>
                        </div>
                    </div>
                </section>

                <section className={`${styles.sec} ${styles.rv}`} style={{ '--d': 4 } as CSSProperties}>
                    <div className={styles.sech}>
                        <Eyebrow>{t('activity.eyebrow')}</Eyebrow>
                        <span className={styles.live} data-off={!unsealedKnown || undefined}>
                            <i />
                            {t('activity.live', { s: ACTIVITY_POLL / 1000 })}
                        </span>
                    </div>
                    <div className={styles.bento}>
                        <ActivityTile
                            className={tileW}
                            dead={deadFor(tokensQuery.isError)}
                            label={t('activity.tokens')}
                            loading={view === 'loading' || tokensQuery.isLoading}
                            stamp={tokensQuery.dataUpdatedAt}
                            value={tokensQuery.data?.length}
                        />
                        {!secretsForbidden && (
                            <ActivityTile
                                className={tileW}
                                dead={deadFor(secretsQuery.isError)}
                                label={t('activity.secrets')}
                                loading={view === 'loading' || secretsQuery.isLoading}
                                stamp={secretsQuery.dataUpdatedAt}
                                value={secretsQuery.data?.secrets}
                            />
                        )}
                        {!auditForbidden && (
                            <ActivityTile
                                className={tileW}
                                dead={deadFor(auditRateQuery.isError)}
                                label={t('activity.auditRate')}
                                loading={view === 'loading' || auditRateQuery.isLoading}
                                stamp={auditRateQuery.dataUpdatedAt}
                                value={auditRateQuery.data?.stats.last_minute}
                            />
                        )}
                    </div>
                </section>

                <HaSection
                    haEnabled={health?.ha_enabled}
                    healthLoaded={!!health}
                    sealed={sealed || view === 'error'}
                />
            </div>

            <ConfirmModal
                confirmLabel={t('rotate.confirm')}
                destructive={false}
                icon="refresh"
                message={
                    <>
                        {t('rotate.message')}
                        <div className={styles.warnbox}>
                            <span>{t('rotate.warn1')}</span>
                            <span>{t('rotate.warn2')}</span>
                        </div>
                    </>
                }
                onClose={() => setRotateOpen(false)}
                onConfirm={() => rotateMutation.mutateAsync()}
                opened={rotateOpen}
                title={t('rotate.title')}
            />

            <Modal
                busy={sealMutation.isPending}
                footer={
                    <>
                        <Button disabled={sealMutation.isPending} onClick={() => setSealOpen(false)} variant="default">
                            {t('sealModal.cancel')}
                        </Button>
                        <Button color="red" loading={sealMutation.isPending} onClick={confirmSeal}>
                            {t('sealModal.confirm')}
                        </Button>
                    </>
                }
                icon="lock"
                onClose={() => setSealOpen(false)}
                opened={sealOpen}
                title={t('sealModal.title')}
                tone="danger"
            >
                <p style={{ margin: 0 }}>{t('sealModal.message')}</p>
                <div className={styles.warnbox} data-tone="danger">
                    <span>{t('sealModal.warn1')}</span>
                    <span>{t('sealModal.warn2')}</span>
                </div>
                <label className={styles.chk}>
                    <input
                        checked={sealAck}
                        data-autofocus
                        onChange={(e) => {
                            setSealAck(e.currentTarget.checked)
                            setSealAckErr(false)
                        }}
                        type="checkbox"
                    />
                    <span>{t('sealModal.check')}</span>
                </label>
                <div className={styles.sealMsg} role="status">
                    {sealAckErr ? t('sealModal.needCheck') : ''}
                </div>
            </Modal>
        </Page>
    )
}
