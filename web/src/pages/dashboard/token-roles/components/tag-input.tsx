// SPDX-License-Identifier: AGPL-3.0-only
import { KeyboardEvent, useRef, useState } from 'react'

import { cx } from '@shared/ui/kit'

import { useTr } from '../lib'
import styles from './role-drawer.module.css'

/** Chips input: Enter / comma / blur commit, Backspace on empty removes the last chip. */
export function TagInput({
    id,
    value,
    onChange,
    onTouch,
    isValid,
    placeholder,
    suggestions = []
}: {
    id: string
    isValid: (v: string) => boolean
    onChange: (next: string[]) => void
    onTouch?: () => void
    placeholder?: string
    suggestions?: string[]
    value: string[]
}) {
    const { tr } = useTr()
    const [draft, setDraft] = useState('')
    const inputRef = useRef<HTMLInputElement>(null)

    const commit = (raw: string) => {
        const parts = raw
            .split(',')
            .map((p) => p.trim())
            .filter(Boolean)
        if (parts.length) {
            const next = [...value]
            for (const p of parts) if (!next.includes(p)) next.push(p)
            onChange(next)
        }
        onTouch?.()
        setDraft('')
    }

    const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter' || e.key === ',') {
            e.preventDefault()
            e.stopPropagation()
            commit(draft)
        } else if (e.key === 'Backspace' && !draft && value.length) {
            onChange(value.slice(0, -1))
        }
    }

    const free = suggestions.filter((s) => !value.includes(s))

    return (
        <>
            <div className={styles.tags} onClick={() => inputRef.current?.focus()}>
                {value.map((v) => (
                    <span className={cx(styles.tg, !isValid(v) && styles.tgBad)} key={v}>
                        {v}
                        <button
                            aria-label={tr('removeTag', 'Убрать {{v}}', { v })}
                            onClick={(e) => {
                                e.stopPropagation()
                                onChange(value.filter((x) => x !== v))
                                onTouch?.()
                            }}
                            type="button"
                        >
                            ×
                        </button>
                    </span>
                ))}
                <input
                    id={id}
                    onBlur={() => draft.trim() && commit(draft)}
                    onChange={(e) => setDraft(e.currentTarget.value)}
                    onKeyDown={onKey}
                    placeholder={value.length ? '' : placeholder}
                    ref={inputRef}
                    spellCheck={false}
                    value={draft}
                />
            </div>
            {free.length > 0 && (
                <div className={styles.sugg}>
                    {free.map((s) => (
                        <button
                            key={s}
                            onClick={() => {
                                onChange([...value, s])
                                onTouch?.()
                            }}
                            type="button"
                        >
                            + {s}
                        </button>
                    ))}
                </div>
            )}
        </>
    )
}
