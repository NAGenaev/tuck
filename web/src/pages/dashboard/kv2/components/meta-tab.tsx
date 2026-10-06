// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { KVMeta } from '@shared/api/endpoints/kvv2'
import { KeyValueEditor, KvPair, validateKv } from '@shared/ui/kit'
import { Pill } from '@shared/ui/tuck/tuck'

import { LIMITS } from '../api'
import common from '../common.module.css'
import styles from '../detail.module.css'
import { fmtDate } from '../utils'

// the server accepts any key of 1..128 bytes without control characters
const KEY_PATTERN = /^.+$/
const bytes = (s: string) => new TextEncoder().encode(s).length

export interface MountOptions {
    cas_required?: boolean
    max_versions?: number
}

export function MetaTab({
    meta,
    mount,
    onSaveMax,
    onToggleCas,
    onSaveCustom,
    saving
}: {
    meta: KVMeta | undefined
    mount?: MountOptions
    onSaveCustom: (map: Record<string, string>) => void
    onSaveMax: (n: number) => void
    onToggleCas: (next: boolean) => void
    saving: { cas: boolean; custom: boolean; max: boolean }
}) {
    const { t, i18n } = useTranslation('kv')
    const maxVersions = meta?.max_versions ?? 0
    const casRequired = !!meta?.cas_required
    const stored = useMemo<KvPair[]>(
        () =>
            Object.entries(meta?.custom_metadata ?? {})
                .sort(([a], [b]) => a.localeCompare(b))
                .map(([key, value]) => ({ key, value })),
        [meta?.custom_metadata]
    )

    const [draft, setDraft] = useState(maxVersions)
    const [pairs, setPairs] = useState<KvPair[]>(stored)

    const dirty = draft !== maxVersions
    const check = validateKv(pairs, KEY_PATTERN)
    const tooLong = pairs.some((p) => bytes(p.key.trim()) > LIMITS.customKey || bytes(p.value) > LIMITS.customValue)
    const filled = pairs.filter((p) => p.key.trim() || p.value)
    const normalized = (list: KvPair[]) =>
        JSON.stringify(list.filter((p) => p.key.trim() || p.value).map((p) => [p.key.trim(), p.value]))
    const customDirty = normalized(pairs) !== normalized(stored)
    const customOk = check.valid && !tooLong

    const saveCustom = () => {
        const map: Record<string, string> = {}
        for (const p of filled) map[p.key.trim()] = p.value
        onSaveCustom(map)
    }

    return (
        <div className={styles.tp}>
            <div className={styles.mg}>
                <div>
                    <small>{t('kv2.meta.current')}</small>
                    <strong>{meta?.current_version || '—'}</strong>
                </div>
                <div>
                    <small>{t('kv2.meta.oldest')}</small>
                    <strong>{meta?.oldest_version || '—'}</strong>
                </div>
                <div>
                    <small>{t('kv2.meta.created')}</small>
                    <strong>{fmtDate(meta?.created_at, i18n.language)}</strong>
                </div>
                <div>
                    <small>{t('kv2.meta.updated')}</small>
                    <strong>{fmtDate(meta?.updated_at, i18n.language)}</strong>
                </div>
                <div>
                    <small>max_versions</small>
                    <div className={styles.mini}>
                        <div className={styles.stepper}>
                            <button
                                aria-label={t('kv2.meta.less')}
                                disabled={draft <= 0 || saving.max}
                                onClick={() => setDraft((n) => Math.max(0, n - 1))}
                                type="button"
                            >
                                −
                            </button>
                            <span aria-live="polite">{draft === 0 ? '∞' : draft}</span>
                            <button
                                aria-label={t('kv2.meta.more')}
                                disabled={draft >= LIMITS.maxVersions || saving.max}
                                onClick={() => setDraft((n) => Math.min(LIMITS.maxVersions, n + 1))}
                                type="button"
                            >
                                +
                            </button>
                        </div>
                        <Button disabled={!dirty} loading={saving.max} onClick={() => onSaveMax(draft)} size="compact-sm">
                            {t('kv2.meta.save')}
                        </Button>
                        {dirty && (
                            <Button disabled={saving.max} onClick={() => setDraft(maxVersions)} size="compact-sm" variant="default">
                                {t('kv2.meta.reset')}
                            </Button>
                        )}
                    </div>
                </div>
                <div>
                    <small>cas_required</small>
                    <div className={styles.mini}>
                        <button
                            aria-pressed={casRequired}
                            className={common.sw}
                            data-on={casRequired || undefined}
                            disabled={saving.cas}
                            onClick={() => onToggleCas(!casRequired)}
                            type="button"
                        >
                            <i />
                            {casRequired ? t('kv2.meta.casOn') : t('kv2.meta.casOff')}
                        </button>
                        {mount?.cas_required && !casRequired && <Pill tone="brass">{t('kv2.meta.casByMount')}</Pill>}
                    </div>
                </div>
            </div>
            <div className={common.msg}>
                {t('kv2.meta.hint')}
                {mount && (mount.cas_required || mount.max_versions) ? (
                    <>
                        {' '}
                        {t('kv2.meta.mountNote', {
                            cas: mount.cas_required ? t('kv2.meta.casOn') : t('kv2.meta.casOff'),
                            max: mount.max_versions ? mount.max_versions : t('kv2.meta.builtin')
                        })}
                    </>
                ) : null}
            </div>

            <div>
                <KeyValueEditor
                    addLabel={t('kv2.meta.cmAdd')}
                    description={t('kv2.meta.cmHint', { keys: LIMITS.customKey, pairs: LIMITS.customPairs, value: LIMITS.customValue })}
                    disabled={saving.custom}
                    keyPattern={KEY_PATTERN}
                    label={
                        <span className={common.lab}>
                            {t('kv2.meta.cm')} <small>{t('kv2.meta.cmCount', { count: filled.length })}</small>
                        </span>
                    }
                    maxPairs={LIMITS.customPairs}
                    onChange={setPairs}
                    value={pairs}
                />
                {tooLong && <div className={common.msg} data-err>{t('kv2.meta.cmTooLong', { key: LIMITS.customKey, value: LIMITS.customValue })}</div>}
                <div className={styles.mini} style={{ marginTop: 10 }}>
                    <Button
                        disabled={!customDirty || !customOk}
                        loading={saving.custom}
                        onClick={saveCustom}
                        size="compact-sm"
                    >
                        {t('kv2.meta.cmSave')}
                    </Button>
                    {customDirty && (
                        <Button disabled={saving.custom} onClick={() => setPairs(stored)} size="compact-sm" variant="default">
                            {t('kv2.meta.reset')}
                        </Button>
                    )}
                </div>
            </div>
        </div>
    )
}
