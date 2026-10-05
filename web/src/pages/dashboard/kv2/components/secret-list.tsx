// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { KeyboardEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cx, Icon, StateBlock } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import styles from '../kv2.module.css'

export interface Entry {
    folder: boolean
    name: string
    path: string
}

export type ListStatus = 'error' | 'forbidden' | 'loading' | 'ready'

function Highlight({ name, filter }: { filter: string; name: string }) {
    if (!filter) return <>{name}</>
    const i = name.toLowerCase().indexOf(filter.toLowerCase())
    if (i < 0) return <>{name}</>
    return (
        <>
            {name.slice(0, i)}
            <b>{name.slice(i, i + filter.length)}</b>
            {name.slice(i + filter.length)}
        </>
    )
}

function Glyph({ children, red }: { children: ReactNode; red?: boolean }) {
    return (
        <div className={styles.glyph} data-red={red || undefined}>
            <svg fill="none" viewBox="0 0 88 88">
                {children}
            </svg>
        </div>
    )
}

export function SecretList({
    prefix,
    all,
    shown,
    filter,
    status,
    errorText,
    onRetry,
    retrying,
    onOpenFolder,
    onOpenSecret,
    selected,
    onToggle,
    onToggleAll,
    onClearSel,
    onBulkDelete,
    flash,
    slide,
    still,
    onWrite,
    onUp,
    onResetFilter
}: {
    all: Entry[]
    errorText?: string
    filter: string
    flash: null | string
    onBulkDelete: () => void
    onClearSel: () => void
    onOpenFolder: (path: string) => void
    onOpenSecret: (path: string) => void
    onResetFilter: () => void
    onRetry: () => void
    onToggle: (path: string) => void
    onToggleAll: () => void
    onUp: () => void
    onWrite: () => void
    prefix: string
    retrying?: boolean
    selected: Set<string>
    shown: Entry[]
    slide: null | 'back' | 'fwd'
    status: ListStatus
    still: boolean
}) {
    const { t } = useTranslation('kv')
    const secrets = shown.filter((e) => !e.folder)
    const nSel = selected.size
    const allOn = secrets.length > 0 && secrets.every((e) => selected.has(e.path))
    const some = secrets.some((e) => selected.has(e.path))
    const pathLabel = `secret/${prefix}`

    const onKey = (ev: KeyboardEvent<HTMLDivElement>, e: Entry) => {
        const row = ev.currentTarget
        if (ev.key === 'Enter') {
            ev.preventDefault()
            e.folder ? onOpenFolder(e.path) : onOpenSecret(e.path)
        } else if (ev.key === ' ' && !e.folder) {
            ev.preventDefault()
            onToggle(e.path)
        } else if (ev.key === 'ArrowDown') {
            ev.preventDefault()
            ;(row.nextElementSibling as HTMLElement | null)?.focus()
        } else if (ev.key === 'ArrowUp') {
            ev.preventDefault()
            const prev = row.previousElementSibling as HTMLElement | null
            if (prev && prev.getAttribute('role') === 'button') prev.focus()
        }
    }

    let head: ReactNode
    let body: ReactNode

    if (status === 'loading') {
        head = (
            <>
                <span className={styles.path}>{pathLabel}</span>
                <em>
                    <span className={styles.dotb} />
                    {t('kv2.list.loading')}
                </em>
            </>
        )
        body = (
            <div aria-busy>
                <div className={styles.cols} style={{ opacity: 0.5 }}>
                    <span />
                    <span>{t('kv2.list.name')}</span>
                    <span className={styles.typeCol}>{t('kv2.list.type')}</span>
                    <span />
                </div>
                {Array.from({ length: 6 }, (_, i) => (
                    <div className={styles.skrow} key={i}>
                        <span />
                        <span style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
                            <i className={styles.sk} style={{ width: 30, height: 30, flex: 'none' }} />
                            <i className={styles.sk} style={{ width: 120 + ((i * 37) % 110) }} />
                        </span>
                        <i className={cx(styles.sk, styles.typeCol)} style={{ width: 64 }} />
                        <span />
                    </div>
                ))}
            </div>
        )
    } else if (status === 'forbidden') {
        head = (
            <>
                <span className={styles.path}>{pathLabel}</span>
                <em style={{ color: 'var(--alarm)' }}>{t('kv2.list.denied')}</em>
            </>
        )
        body = (
            <div className={styles.empty}>
                <Glyph red>
                    <g className={styles.ring}>
                        <circle cx="44" cy="44" r="40" stroke="rgba(255,90,77,.35)" strokeWidth="1.5" />
                        <circle cx="44" cy="44" r="30" stroke="currentColor" strokeDasharray="5 7" strokeWidth="1.5" />
                    </g>
                    <rect height="20" stroke="currentColor" strokeWidth="2" width="28" x="30" y="42" />
                    <path d="M36 42v-6a8 8 0 0116 0v6" stroke="currentColor" strokeWidth="2" />
                </Glyph>
                <Pill tone="bad">403 · forbidden</Pill>
                <h3>{t('kv2.forbidden.title')}</h3>
                <p>
                    {t('kv2.forbidden.text')} <code>list</code> {t('kv2.forbidden.text2')}
                </p>
                <div className={styles.code403}>
                    <span className={styles.c}># {t('kv2.forbidden.comment')}</span>
                    {'\n'}
                    <span className={styles.k}>path</span>
                    {'      '}
                    <span className={styles.s}>{`"secret/metadata/${prefix}"`}</span>
                    {'\n'}
                    <span className={styles.k}>required</span>
                    {'  '}[&quot;list&quot;]
                </div>
                <div className={styles.emptyActs}>
                    <Button loading={retrying} onClick={onRetry} variant="default">
                        {t('kv2.retry')}
                    </Button>
                    {prefix && (
                        <Button leftSection={<Icon name="up" size={15} />} onClick={onUp} variant="default">
                            {t('kv2.list.up')}
                        </Button>
                    )}
                </div>
            </div>
        )
    } else if (status === 'error') {
        head = (
            <>
                <span className={styles.path}>{pathLabel}</span>
                <em style={{ color: 'var(--alarm)' }}>{t('kv2.list.failed')}</em>
            </>
        )
        body = <StateBlock details={errorText} kind="error" onRetry={onRetry} retrying={retrying} />
    } else if (all.length === 0) {
        head = (
            <>
                <span className={styles.path}>{pathLabel}</span>
                <em>{t('kv2.list.zero')}</em>
            </>
        )
        body = (
            <div className={styles.empty}>
                <Glyph>
                    <g className={styles.ring}>
                        <circle cx="44" cy="44" r="40" stroke="var(--line2)" strokeWidth="1.5" />
                        <circle cx="44" cy="44" r="30" stroke="currentColor" strokeDasharray="5 7" strokeWidth="1.5" />
                    </g>
                    <path d="M28 36h12l4 4h16v20H28z" stroke="currentColor" strokeWidth="2" />
                </Glyph>
                <h3>{t('kv2.empty.title')}</h3>
                <p>
                    <code>{pathLabel}</code> {t('kv2.empty.text')}
                </p>
                <div className={styles.emptyActs}>
                    <Button leftSection={<Icon name="plus" size={15} />} onClick={onWrite}>
                        {t('kv2.writeBtn')}
                    </Button>
                    {prefix && (
                        <Button leftSection={<Icon name="up" size={15} />} onClick={onUp} variant="default">
                            {t('kv2.list.up')}
                        </Button>
                    )}
                </div>
            </div>
        )
    } else {
        head = (
            <>
                <span className={styles.path}>{pathLabel}</span>
                {nSel > 0 ? (
                    <div className={styles.selBar}>
                        <b>{t('kv2.list.selected', { count: nSel })}</b>
                        <Button color="red" onClick={onBulkDelete} size="compact-xs" variant="outline">
                            {t('kv2.list.bulkDelete')}
                        </Button>
                        <Button onClick={onClearSel} size="compact-xs" variant="default">
                            {t('kv2.list.clear')}
                        </Button>
                    </div>
                ) : (
                    <em>{t('kv2.list.count', { count: all.length, shown: shown.length })}</em>
                )}
            </>
        )
        body =
            shown.length === 0 ? (
                <div className={styles.empty}>
                    <Glyph>
                        <g className={styles.ring}>
                            <circle cx="44" cy="44" r="40" stroke="var(--line2)" strokeWidth="1.5" />
                        </g>
                        <circle cx="40" cy="40" r="13" stroke="currentColor" strokeWidth="2" />
                        <path d="M50 50l12 12" stroke="currentColor" strokeWidth="2" />
                    </Glyph>
                    <h3>{t('kv2.nomatch.title')}</h3>
                    <p>
                        {t('kv2.nomatch.text')} «<code>{filter || '—'}</code>»
                    </p>
                    <div className={styles.emptyActs}>
                        <Button onClick={onResetFilter} variant="default">
                            {t('kv2.nomatch.reset')}
                        </Button>
                    </div>
                </div>
            ) : (
                <>
                    <div className={styles.cols}>
                        <button
                            aria-label={t('kv2.list.selectAll')}
                            className={styles.cb}
                            data-on={allOn || undefined}
                            data-part={(!allOn && some) || undefined}
                            disabled={secrets.length === 0}
                            onClick={onToggleAll}
                            type="button"
                        >
                            <Icon name={!allOn && some ? 'minus' : 'check'} size={12} />
                        </button>
                        <span>{t('kv2.list.name')}</span>
                        <span className={styles.typeCol}>{t('kv2.list.type')}</span>
                        <span />
                    </div>
                    {shown.map((e, i) => {
                        const sel = selected.has(e.path)
                        return (
                            <div
                                aria-label={e.name}
                                className={styles.row}
                                data-flash={flash === e.path || undefined}
                                data-folder={e.folder || undefined}
                                data-secret={!e.folder || undefined}
                                data-sel={sel || undefined}
                                data-still={still || undefined}
                                key={e.path}
                                onClick={() => (e.folder ? onOpenFolder(e.path) : onOpenSecret(e.path))}
                                onKeyDown={(ev) => onKey(ev, e)}
                                role="button"
                                style={{ ['--i' as string]: Math.min(i, 14) }}
                                tabIndex={0}
                            >
                                {e.folder ? (
                                    <span />
                                ) : (
                                    <button
                                        aria-label={t('kv2.list.select')}
                                        aria-pressed={sel}
                                        className={styles.cb}
                                        data-on={sel || undefined}
                                        onClick={(ev) => {
                                            ev.stopPropagation()
                                            onToggle(e.path)
                                        }}
                                        tabIndex={-1}
                                        type="button"
                                    >
                                        <Icon name="check" size={12} />
                                    </button>
                                )}
                                <div className={styles.nm}>
                                    <span className={styles.ico}>
                                        <Icon name={e.folder ? 'inbox' : 'key'} size={16} />
                                    </span>
                                    <span className={styles.t}>
                                        <Highlight filter={filter} name={e.name} />
                                    </span>
                                </div>
                                <span className={styles.typeCol}>
                                    <Pill tone={e.folder ? 'brass' : 'ice'}>
                                        {e.folder ? t('kv2.list.folder') : t('kv2.list.secret')}
                                    </Pill>
                                </span>
                                <Icon className={styles.chev} name="right" size={16} />
                            </div>
                        )
                    })}
                </>
            )
    }

    return (
        <Plate className={styles.listplate}>
            <div className={styles.lhead}>{head}</div>
            <div className={cx(styles.lview, slide === 'fwd' && styles.slideFwd, slide === 'back' && styles.slideBack)} key={`${prefix}|${slide}`}>
                {body}
            </div>
            <div className={styles.foot}>
                <span>
                    <kbd>↑</kbd>
                    <kbd>↓</kbd>
                    {t('kv2.keys.nav')}
                </span>
                <span>
                    <kbd>Enter</kbd>
                    {t('kv2.keys.open')}
                </span>
                <span>
                    <kbd>Space</kbd>
                    {t('kv2.keys.select')}
                </span>
                <span>
                    <kbd>⌫</kbd>
                    {t('kv2.keys.up')}
                </span>
                <span>
                    <kbd>/</kbd>
                    {t('kv2.keys.filter')}
                </span>
                <span>
                    <kbd>N</kbd>
                    {t('kv2.keys.write')}
                </span>
            </div>
        </Plate>
    )
}
