// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { getMountConfig, type MountEntry, putMountConfig } from '@shared/api/endpoints/mounts'
import { CopyButton, cx, Drawer, Icon, SkeletonLine, toast, TtlField } from '@shared/ui/kit'

import { fmtTtl, nsToSec, serverMessage } from '../lib'
import styles from '../mounts.module.css'

const TTL_MAX = 31_536_000
const TTL_PRESETS = [3600, 86_400, 604_800, 2_592_000, TTL_MAX]
// KV v2 options are only honoured on this mount (internal/core.KV2MountPath) — the
// only path whose routes are wired to the KV v2 engine in this codebase.
const KV2_MOUNT_PATH = 'secret/'
const MAX_VERSIONS_LIMIT = 1000

const mask = (a: string) => {
    const i = a.lastIndexOf('_')
    return a.length > 12 ? `${a.slice(0, i + 5)}…${a.slice(-4)}` : a
}

const rich = { b: <b /> }

export function TuneDrawer({ mount, onClose }: { mount: MountEntry | null; onClose: () => void }) {
    const { t } = useTranslation('mounts')
    // keep the last mount while the drawer slides out, so the content does not vanish mid-animation
    const last = useRef<MountEntry | null>(null)
    if (mount) last.current = mount
    const m = mount ?? last.current

    return (
        <Drawer
            icon="gear"
            onClose={onClose}
            opened={!!mount}
            size={460}
            title={t('tune.title', { path: m?.path ?? '' })}
        >
            {m && <TuneBody key={m.path} mount={m} onClose={onClose} open={!!mount} />}
        </Drawer>
    )
}

function TuneBody({ mount, onClose, open }: { mount: MountEntry; onClose: () => void; open: boolean }) {
    const { data, isError, isPending } = useQuery({
        enabled: open,
        queryFn: () => getMountConfig(mount.path),
        queryKey: ['mounts', 'config', mount.path],
        retry: 1
    })

    // the TtlField keeps its own unit state, so the form mounts once the config is known
    if (isPending && open) {
        return (
            <div aria-busy>
                <SkeletonLine height={44} />
                <SkeletonLine height={120} />
                <SkeletonLine height={120} />
            </div>
        )
    }
    return (
        <TuneForm
            failed={isError}
            initial={{
                casRequired: !!data?.cas_required,
                def: nsToSec(data?.default_lease_ttl),
                max: nsToSec(data?.max_lease_ttl),
                maxVersions: data?.max_versions ?? 0,
                noCache: !!data?.force_no_cache
            }}
            mount={mount}
            onClose={onClose}
        />
    )
}

function TuneForm({
    failed,
    initial,
    mount,
    onClose
}: {
    failed: boolean
    initial: { casRequired: boolean; def: number; max: number; maxVersions: number; noCache: boolean }
    mount: MountEntry
    onClose: () => void
}) {
    const { t } = useTranslation('mounts')
    const queryClient = useQueryClient()
    const [def, setDef] = useState(initial.def)
    const [max, setMax] = useState(initial.max)
    const [noCache, setNoCache] = useState(initial.noCache)
    const isKV2 = mount.path === KV2_MOUNT_PATH
    const [maxVersions, setMaxVersions] = useState(initial.maxVersions)
    const [casRequired, setCasRequired] = useState(initial.casRequired)
    const [reveal, setReveal] = useState(false)
    const [serverErr, setServerErr] = useState('')

    const orderErr = max > 0 && def > max ? t('tune.errOrder') : ''
    const maxVersionsErr =
        isKV2 && (maxVersions < 0 || maxVersions > MAX_VERSIONS_LIMIT)
            ? t('tune.errMaxVersions', { limit: MAX_VERSIONS_LIMIT })
            : ''

    const mutation = useMutation({
        mutationFn: () => {
            // the server resets force_no_cache to false when it is omitted, so it is always sent.
            // Built as a variable: the shared body type does not list force_no_cache yet.
            const body = { default_lease_ttl: `${def}s`, force_no_cache: noCache, max_lease_ttl: `${max}s` }
            if (isKV2) Object.assign(body, { cas_required: casRequired, max_versions: maxVersions })
            return putMountConfig(mount.path, body)
        },
        onError: (err) => {
            const msg = serverMessage(err)
            setServerErr(msg ?? '')
            toast.error(t('toast.tuneFailed'), msg)
        },
        onSuccess: async () => {
            toast.success(t('toast.tuned'), t('toast.tunedMsg', { path: mount.path }))
            await Promise.all([
                queryClient.invalidateQueries({ queryKey: ['mounts', 'config', mount.path] }),
                queryClient.invalidateQueries({ queryKey: ['mounts', 'list'] })
            ])
            onClose()
        }
    })

    const submit = () => {
        if (orderErr || maxVersionsErr || mutation.isPending) return
        setServerErr('')
        mutation.mutate()
    }

    return (
        <form
            noValidate
            onSubmit={(e) => {
                e.preventDefault()
                submit()
            }}
        >
            <div className={styles.callout} style={{ marginTop: 0 }}>
                <Icon name="gear" size={18} />
                <div>
                    {failed ? (
                        t('tune.loadFailed')
                    ) : (
                        <Trans
                            components={rich}
                            i18nKey="tune.current"
                            ns="mounts"
                            values={{ def: fmtTtl(initial.def), max: fmtTtl(initial.max) }}
                        />
                    )}
                </div>
            </div>

            <div className={styles.fld}>
                <span className={styles.lab}>{t('tune.accessor')}</span>
                <div className={styles.accBox}>
                    <code>{reveal ? mount.accessor : mask(mount.accessor)}</code>
                    <button
                        aria-label={t('tune.reveal')}
                        aria-pressed={reveal}
                        className={styles.ib}
                        onClick={() => setReveal((v) => !v)}
                        title={t('tune.reveal')}
                        type="button"
                    >
                        <Icon name={reveal ? 'eyeoff' : 'eye'} size={16} />
                    </button>
                    <CopyButton label={t('tune.copy')} size={36} value={mount.accessor} />
                </div>
            </div>

            <div className={styles.fld}>
                <TtlField
                    label={t('tune.defaultTtl')}
                    max={TTL_MAX}
                    onChange={setDef}
                    presets={TTL_PRESETS}
                    value={def}
                />
            </div>
            <div className={styles.fld}>
                <TtlField
                    description={t('tune.ttlHint')}
                    label={t('tune.maxTtl')}
                    max={TTL_MAX}
                    onChange={setMax}
                    presets={TTL_PRESETS}
                    value={max}
                />
            </div>

            <div className={styles.fld}>
                <button
                    aria-checked={noCache}
                    className={styles.tog}
                    onClick={() => setNoCache((v) => !v)}
                    role="switch"
                    type="button"
                >
                    <span className={styles.sw} />
                    <span>
                        <b>{t('tune.noCache')}</b>
                        <small>{t('tune.noCacheDesc')}</small>
                    </span>
                </button>
            </div>

            {isKV2 && (
                <>
                    <div className={styles.fld}>
                        <label className={styles.lab} htmlFor="tune-max-versions">
                            {t('tune.maxVersions')}
                        </label>
                        <input
                            className={cx(styles.inp, !!maxVersionsErr && styles.bad)}
                            id="tune-max-versions"
                            inputMode="numeric"
                            max={MAX_VERSIONS_LIMIT}
                            min={0}
                            onChange={(e) => setMaxVersions(e.currentTarget.valueAsNumber || 0)}
                            type="number"
                            value={maxVersions}
                        />
                        <div className={styles.hint}>{t('tune.maxVersionsHint')}</div>
                    </div>
                    <div className={styles.fld}>
                        <button
                            aria-checked={casRequired}
                            className={styles.tog}
                            onClick={() => setCasRequired((v) => !v)}
                            role="switch"
                            type="button"
                        >
                            <span className={styles.sw} />
                            <span>
                                <b>{t('tune.casRequired')}</b>
                                <small>{t('tune.casRequiredDesc')}</small>
                            </span>
                        </button>
                    </div>
                </>
            )}

            {(orderErr || maxVersionsErr || serverErr) && (
                <div className={styles.err}>{orderErr || maxVersionsErr || serverErr}</div>
            )}

            <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 20 }}>
                <Button disabled={mutation.isPending} onClick={onClose} variant="default">
                    {t('tune.cancel')}
                </Button>
                <Button disabled={!!orderErr || !!maxVersionsErr} loading={mutation.isPending} type="submit">
                    {t('tune.save')}
                </Button>
            </div>
        </form>
    )
}
