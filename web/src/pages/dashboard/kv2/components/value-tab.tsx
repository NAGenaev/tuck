// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { CopyButton, copyText, Icon, SkeletonLine, StateBlock, toast } from '@shared/ui/kit'

import common from '../common.module.css'
import styles from '../detail.module.css'
import { ParsedValue, VerRow } from '../utils'
import { SecretText } from './secret-text'

/** reveal key used for a secret that is one plain string */
export const TEXT_KEY = '__text__'

export function revealKeys(parsed: ParsedValue | null): string[] {
    if (!parsed) return []
    return parsed.kind === 'pairs' ? parsed.pairs.map(([k]) => k) : [TEXT_KEY]
}

export function ValueTab({
    row,
    current,
    viewing,
    loading,
    error,
    onRetry,
    parsed,
    base64,
    rev,
    onToggle,
    onToggleAll,
    leftMs,
    ttlMs,
    raw,
    onRaw,
    onBack,
    onUndelete,
    undeleting
}: {
    base64: boolean
    current: number
    error?: string
    leftMs: number
    loading: boolean
    onBack: () => void
    onRaw: (raw: boolean) => void
    onRetry: () => void
    onToggle: (key: string) => void
    onToggleAll: () => void
    onUndelete: () => void
    parsed: ParsedValue | null
    raw: boolean
    rev: Set<string>
    row: undefined | VerRow
    ttlMs: number
    undeleting: boolean
    viewing: boolean
}) {
    const { t } = useTranslation('kv')

    const banner =
        viewing && row ? (
            <div className={common.banner} data-tone="i">
                <Icon name="info" size={20} />
                <h5>{t('kv2.value.viewingTitle', { version: row.version })}</h5>
                <p>{t('kv2.value.viewingText', { version: current })}</p>
                <div className={common.acts}>
                    <Button onClick={onBack} size="compact-sm" variant="default">
                        {t('kv2.value.backToCurrent')}
                    </Button>
                </div>
            </div>
        ) : null

    if (!row) {
        return (
            <div className={styles.tp}>
                <div className={styles.skel} aria-busy>
                    <SkeletonLine width={180} />
                    <SkeletonLine />
                    <SkeletonLine />
                </div>
            </div>
        )
    }

    if (row.state !== 'active') {
        const destroyed = row.state === 'destroyed'
        return (
            <div className={styles.tp}>
                {banner}
                <div className={styles.locked}>
                    <Icon
                        name={destroyed ? 'lock' : 'eyeoff'}
                        size={34}
                        style={{ color: destroyed ? 'var(--alarm)' : 'var(--brass)' }}
                    />
                    <h4>{destroyed ? t('kv2.value.destroyedTitle') : t('kv2.value.deletedTitle')}</h4>
                    <p>
                        {destroyed
                            ? t('kv2.value.destroyedText', { version: row.version })
                            : t('kv2.value.deletedText', { version: row.version })}
                    </p>
                    {!destroyed && (
                        <Button
                            leftSection={<Icon name="up" size={15} />}
                            loading={undeleting}
                            onClick={onUndelete}
                        >
                            {t('kv2.value.restore', { version: row.version })}
                        </Button>
                    )}
                </div>
            </div>
        )
    }

    if (error) {
        return (
            <div className={styles.tp}>
                {banner}
                <StateBlock compact details={error} kind="error" onRetry={onRetry} />
            </div>
        )
    }

    if (loading || !parsed) {
        return (
            <div className={styles.tp}>
                {banner}
                <div className={styles.skel} aria-busy>
                    <SkeletonLine width={180} />
                    <SkeletonLine />
                    <SkeletonLine width="70%" />
                </div>
            </div>
        )
    }

    const keys = revealKeys(parsed)
    const allOn = keys.length > 0 && keys.every((k) => rev.has(k))
    const live = rev.size > 0
    const eye = (k: string) => (
        <button
            aria-label={rev.has(k) ? t('kv2.value.hide') : t('kv2.value.show')}
            aria-pressed={rev.has(k)}
            className={common.ibtn}
            onClick={() => onToggle(k)}
            type="button"
        >
            <Icon name={rev.has(k) ? 'eyeoff' : 'eye'} size={16} />
        </button>
    )

    return (
        <div className={styles.tp}>
            {banner}
            <div className={styles.bar2}>
                {parsed.kind === 'pairs' ? (
                    <div className={common.seg} role="group">
                        <button data-on={!raw || undefined} onClick={() => onRaw(false)} type="button">
                            {t('kv2.value.modeKv')}
                        </button>
                        <button data-on={raw || undefined} onClick={() => onRaw(true)} type="button">
                            Raw
                        </button>
                    </div>
                ) : (
                    <span className={common.kbd}>{base64 ? 'base64' : t('kv2.value.plain')}</span>
                )}
                <div className={styles.grp}>
                    <div className={styles.ttl} data-live={live || undefined} aria-live="off">
                        <span>
                            {live ? t('kv2.value.hidesIn', { sec: Math.ceil(leftMs / 1000) }) : t('kv2.value.hidden')}
                        </span>
                        <span className={styles.tr}>
                            <i style={{ transform: `scaleX(${live ? Math.max(0, Math.min(1, leftMs / ttlMs)) : 0})` }} />
                        </span>
                    </div>
                    <Button
                        leftSection={<Icon name={allOn ? 'eyeoff' : 'eye'} size={15} />}
                        onClick={onToggleAll}
                        size="compact-sm"
                        variant="default"
                    >
                        {allOn ? t('kv2.value.hideAll') : t('kv2.value.revealAll')}
                    </Button>
                </div>
            </div>

            {parsed.kind === 'text' ? (
                <div className={styles.textBox}>
                    <SecretText shown={rev.has(TEXT_KEY)} text={parsed.text} />
                    <span className={styles.a}>
                        {eye(TEXT_KEY)}
                        <CopyButton label={t('kv2.value.copy')} size={34} value={parsed.text} />
                    </span>
                </div>
            ) : !raw ? (
                <div className={styles.kv}>
                    {parsed.pairs.map(([k, v]) => (
                        <div className={styles.kvl} key={k}>
                            <span className={styles.k}>{k}</span>
                            <SecretText shown={rev.has(k)} text={v} />
                            <span className={styles.a}>
                                {eye(k)}
                                <CopyButton label={t('kv2.value.copy')} size={34} value={v} />
                            </span>
                        </div>
                    ))}
                </div>
            ) : (
                <>
                    <div className={styles.raw}>
                        {'{\n'}
                        {parsed.pairs.map(([k, v], i) => (
                            <span key={k}>
                                {'  '}
                                <span className={styles.k}>{JSON.stringify(k)}</span>
                                <span className={styles.p}>:</span> &quot;
                                <SecretText className={styles.s} shown={rev.has(k)} text={v} />
                                &quot;
                                {i < parsed.pairs.length - 1 && <span className={styles.p}>,</span>}
                                {'\n'}
                            </span>
                        ))}
                        {'}'}
                    </div>
                    <div className={styles.grp}>
                        <Button
                            leftSection={<Icon name="copy" size={15} />}
                            onClick={async () => {
                                const ok = await copyText(JSON.stringify(Object.fromEntries(parsed.pairs), null, 2))
                                if (ok) toast.success(t('kv2.value.copied'))
                                else toast.error(t('kv2.value.copyFailed'))
                            }}
                            size="compact-sm"
                            variant="default"
                        >
                            {t('kv2.value.copyJson')}
                        </Button>
                        <span className={common.msg}>{t('kv2.value.rawNote')}</span>
                    </div>
                </>
            )}
        </div>
    )
}
