// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { createMount, type MountEntry } from '@shared/api/endpoints/mounts'
import { CopyButton, cx, Icon, Modal, toast } from '@shared/ui/kit'
import { Eyebrow, Pill } from '@shared/ui/tuck/tuck'

import { CATALOG, normPath, PATH_RE, serverMessage, TYPE_RE, withSlash } from '../lib'
import styles from '../mounts.module.css'
import { TypeIcon } from './type-icon'

const CUSTOM = 'custom'
const DESC_MAX = 120
// KV v2 options are only honoured on this mount (internal/core.KV2MountPath) — the
// only path whose routes are wired to the KV v2 engine in this codebase. In
// practice "secret/" is a builtin registered before any client can call
// create, so the path-taken check below always blocks it — these fields
// exist for completeness / if that ever changes, not for day-to-day use.
const KV2_MOUNT_PATH = 'secret'
const MAX_VERSIONS_LIMIT = 1000

function cardKind(id: string, auth?: boolean): string | undefined {
    if (id === CUSTOM) return styles.custom
    if (auth) return styles.auth
    return undefined
}

export function CreateWizard({
    mounts,
    onClose,
    onCreated,
    opened
}: {
    mounts: MountEntry[]
    onClose: () => void
    onCreated: (path: string) => void
    opened: boolean
}) {
    const { t } = useTranslation('mounts')
    const queryClient = useQueryClient()

    const [step, setStep] = useState(0)
    const [typeId, setTypeId] = useState<null | string>(null)
    const [customType, setCustomType] = useState('')
    const [path, setPath] = useState('')
    const [desc, setDesc] = useState('')
    const [maxVersions, setMaxVersions] = useState(0)
    const [casRequired, setCasRequired] = useState(false)
    const [serverErr, setServerErr] = useState('')

    useEffect(() => {
        if (!opened) return
        setStep(0)
        setTypeId(null)
        setCustomType('')
        setPath('')
        setDesc('')
        setMaxVersions(0)
        setCasRequired(false)
        setServerErr('')
    }, [opened])

    const type = typeId === CUSTOM ? customType.trim() : (typeId ?? '')
    const norm = normPath(path)
    const full = withSlash(norm || '…')
    const suggest = CATALOG.find((c) => c.id === typeId)?.suggest ?? 'my-mount'
    // Only "kv" at the exact "secret/" path maps to the real KV v2 engine (see
    // internal/core/kv2_options.go) — showing the fields anywhere else would
    // be misleading since the server would reject them.
    const isKV2 = typeId === 'kv' && norm === KV2_MOUNT_PATH
    const maxVersionsErr =
        isKV2 && (maxVersions < 0 || maxVersions > MAX_VERSIONS_LIMIT)
            ? t('wizard.errMaxVersions', { limit: MAX_VERSIONS_LIMIT })
            : ''

    let pathErr = ''
    if (!norm) pathErr = t('wizard.pathErrEmpty', { example: suggest })
    else if (!PATH_RE.test(norm)) pathErr = t('wizard.pathErrChars')
    else if (norm.length > 64) pathErr = t('wizard.pathErrLen')
    else {
        const hit = mounts.find((m) => normPath(m.path) === norm)
        if (hit) pathErr = t('wizard.pathErrTaken', { path: withSlash(norm), type: hit.type })
    }
    const typeErr = typeId === CUSTOM && customType.trim() && !TYPE_RE.test(customType.trim())
    const typeOk = !!typeId && (typeId !== CUSTOM || (!!customType.trim() && !typeErr))

    const canNext = step === 0 ? typeOk : step === 1 ? !pathErr && !maxVersionsErr : true

    const mutation = useMutation({
        mutationFn: () =>
            createMount(
                norm,
                type,
                desc.trim() || undefined,
                isKV2 ? { cas_required: casRequired, max_versions: maxVersions } : undefined
            ),
        onSuccess: async () => {
            const created = withSlash(norm)
            toast.success(t('toast.mounted'), t('toast.mountedMsg', { path: created }))
            onClose()
            await queryClient.invalidateQueries({ queryKey: ['mounts', 'list'] })
            onCreated(created)
        },
        onError: (err) => {
            const msg = serverMessage(err)
            setServerErr(msg ?? '')
            toast.error(t('toast.mountFailed'), msg)
        }
    })

    const pick = (id: string) => {
        const prev = CATALOG.find((c) => c.id === typeId)?.suggest ?? ''
        const next = CATALOG.find((c) => c.id === id)?.suggest ?? ''
        setTypeId(id)
        if (!path || path === prev) setPath(next)
    }

    const go = (to: number) => {
        setServerErr('')
        setStep(to)
    }

    const cards = [...CATALOG.map((c) => ({ auth: c.auth, icon: c.icon, id: c.id })), { icon: 'plug', id: CUSTOM }]

    return (
        <Modal
            busy={mutation.isPending}
            footer={
                <>
                    <Button
                        disabled={mutation.isPending}
                        onClick={() => (step ? go(step - 1) : onClose())}
                        variant="default"
                    >
                        {step ? t('wizard.back') : t('wizard.cancel')}
                    </Button>
                    {step < 2 ? (
                        <Button disabled={!canNext} onClick={() => go(step + 1)}>
                            {t('wizard.next')}
                        </Button>
                    ) : (
                        <Button
                            leftSection={<Icon name="plus" size={14} />}
                            loading={mutation.isPending}
                            onClick={() => mutation.mutate()}
                        >
                            {t('wizard.mount')}
                        </Button>
                    )}
                </>
            }
            icon="plus"
            onClose={onClose}
            opened={opened}
            size={780}
            title={t(`wizard.titles.${step}`)}
        >
            <Eyebrow>{t('wizard.eyebrow', { n: step + 1 })}</Eyebrow>
            <div className={styles.steps} style={{ marginTop: 14 }}>
                {(['type', 'path', 'review'] as const).map((k, i) => (
                    <div className={i === step ? styles.on : i < step ? styles.done : undefined} key={k}>
                        {i + 1} · {t(`wizard.steps.${k}`)}
                        <small>{t(`wizard.steps.${k}Hint`)}</small>
                    </div>
                ))}
            </div>

            <div className={styles.pane} key={step}>
                {step === 0 && (
                    <>
                        <p className={styles.intro}>{t('wizard.intro')}</p>
                        <div aria-label={t('wizard.catalogLabel')} className={styles.cat} role="listbox">
                            {cards.map((c) => {
                                return (
                                    <button
                                        aria-selected={typeId === c.id}
                                        className={cx(
                                            styles.card,
                                            cardKind(c.id, 'auth' in c ? c.auth : undefined),
                                            typeId === c.id && styles.sel
                                        )}
                                        key={c.id}
                                        onClick={() => pick(c.id)}
                                        role="option"
                                        type="button"
                                    >
                                        <span className={styles.ico}>
                                            <TypeIcon name={c.icon} />
                                        </span>
                                        <h4>{t(`wizard.types.${c.id}.name`)}</h4>
                                        <p>{t(`wizard.types.${c.id}.desc`)}</p>
                                        <span className={styles.tg}>{t(`wizard.types.${c.id}.tag`)}</span>
                                    </button>
                                )
                            })}
                        </div>
                        {typeId === CUSTOM && (
                            <div className={styles.fld} style={{ marginTop: 18 }}>
                                <label className={styles.lab} htmlFor="mw-type">
                                    {t('wizard.customType')}
                                </label>
                                <input
                                    autoComplete="off"
                                    className={cx(styles.inp, !!typeErr && styles.bad)}
                                    data-autofocus
                                    id="mw-type"
                                    onChange={(e) => setCustomType(e.currentTarget.value)}
                                    placeholder={t('wizard.customTypePlaceholder')}
                                    spellCheck={false}
                                    value={customType}
                                />
                                {typeErr ? (
                                    <div className={styles.err}>{t('wizard.errCustomType')}</div>
                                ) : (
                                    <div className={styles.hint}>{t('wizard.customTypeHint')}</div>
                                )}
                            </div>
                        )}
                    </>
                )}

                {step === 1 && (
                    <>
                        <div className={styles.fld}>
                            <label className={styles.lab} htmlFor="mw-path">
                                {t('wizard.path')}
                            </label>
                            <div className={cx(styles.pathin, !!path && !!pathErr && styles.bad)}>
                                <input
                                    autoComplete="off"
                                    data-autofocus
                                    id="mw-path"
                                    onChange={(e) => setPath(e.currentTarget.value)}
                                    placeholder={suggest}
                                    spellCheck={false}
                                    value={path}
                                />
                                <em>/</em>
                            </div>
                            {path && pathErr && <div className={styles.err}>{pathErr}</div>}
                            <div className={styles.hint}>
                                {t('wizard.willMount')} <b>{full}</b>
                            </div>
                        </div>
                        <div className={styles.fld}>
                            <label className={styles.lab} htmlFor="mw-desc">
                                {t('wizard.description')}
                            </label>
                            <input
                                autoComplete="off"
                                className={styles.inp}
                                id="mw-desc"
                                maxLength={DESC_MAX}
                                onChange={(e) => setDesc(e.currentTarget.value)}
                                placeholder={t('wizard.descPlaceholder')}
                                value={desc}
                            />
                            <div className={styles.hint}>{t('wizard.descHint')}</div>
                        </div>
                        {isKV2 && (
                            <>
                                <div className={styles.fld}>
                                    <label className={styles.lab} htmlFor="mw-max-versions">
                                        {t('wizard.maxVersions')}
                                    </label>
                                    <input
                                        className={cx(styles.inp, !!maxVersionsErr && styles.bad)}
                                        id="mw-max-versions"
                                        inputMode="numeric"
                                        max={MAX_VERSIONS_LIMIT}
                                        min={0}
                                        onChange={(e) => setMaxVersions(e.currentTarget.valueAsNumber || 0)}
                                        type="number"
                                        value={maxVersions}
                                    />
                                    {maxVersionsErr ? (
                                        <div className={styles.err}>{maxVersionsErr}</div>
                                    ) : (
                                        <div className={styles.hint}>{t('wizard.maxVersionsHint')}</div>
                                    )}
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
                                            <b>{t('wizard.casRequired')}</b>
                                            <small>{t('wizard.casRequiredDesc')}</small>
                                        </span>
                                    </button>
                                </div>
                            </>
                        )}
                    </>
                )}

                {step === 2 && (
                    <>
                        <div aria-hidden className={styles.miniBus}>
                            <i className={styles.l} />
                            <i className={styles.c} />
                            <u />
                            <b>{full}</b>
                        </div>
                        <div className={styles.slip}>
                            <div className={styles.rw}>
                                <small>{t('wizard.slip.type')}</small>
                                <b>{type}</b>
                            </div>
                            <div className={styles.rw}>
                                <small>{t('wizard.slip.path')}</small>
                                <b>{full}</b>
                            </div>
                            <div className={styles.rw}>
                                <small>{t('wizard.slip.desc')}</small>
                                <b>{desc.trim() || '—'}</b>
                            </div>
                            {isKV2 && (
                                <div className={styles.rw}>
                                    <small>{t('wizard.slip.config')}</small>
                                    <b>
                                        max_versions={maxVersions}, cas_required={String(casRequired)}
                                    </b>
                                </div>
                            )}
                        </div>
                        <div className={styles.term}>
                            <div className={styles.termTop}>
                                <span>{t('wizard.request')}</span>
                                <em>sys/mounts</em>
                            </div>
                            <pre>
                                <span className="p">POST</span> <span className="w">/v1/sys/mounts/{full}</span>
                                {'\n'}
                                <span className="k">
                                    {JSON.stringify({
                                        ...(desc.trim() ? { description: desc.trim() } : {}),
                                        type,
                                        ...(isKV2 ? { config: { cas_required: casRequired, max_versions: maxVersions } } : {})
                                    })}
                                </span>
                            </pre>
                        </div>
                        {serverErr && <div className={styles.err}>{serverErr}</div>}
                    </>
                )}
            </div>
        </Modal>
    )
}

