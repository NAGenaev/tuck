// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Input } from '@mantine/core'
import { KeyboardEvent, ReactNode, useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from '../cx'
import { Icon } from '../icons'
import { toast } from '../toast/toast'
import styles from './code-field.module.css'

export interface JsonCheck {
    /** 1-based line of the first syntax error */
    line?: number
    message?: string
    ok: boolean
}

/** Parses `text` as JSON and, on failure, locates the offending line. Blank text is ok. */
export function checkJson(text: string): JsonCheck {
    if (!text.trim()) return { ok: true }
    try {
        JSON.parse(text)
        return { ok: true }
    } catch (e) {
        const msg = String((e as Error).message)
        const l = /line (\d+)/.exec(msg)
        const p = /position (\d+)/.exec(msg)
        const line = l ? +l[1] : p ? text.slice(0, +p[1]).split('\n').length : text.split('\n').length
        const message = msg
            .replace(/ in JSON.*$/, '')
            .replace(/\(line.*\)/, '')
            .trim()
            .slice(0, 80)
        return { line, message, ok: false }
    }
}

type Token = { cls?: string; text: string }

// strings (key when followed by a colon), numbers, literals, punctuation
const TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|\b(true|false|null)\b|([{}[\],])/g

function tokenize(text: string): Token[] {
    const out: Token[] = []
    let last = 0
    for (const m of text.matchAll(TOKEN)) {
        const at = m.index ?? 0
        if (at > last) out.push({ text: text.slice(last, at) })
        if (m[1]) {
            out.push({ cls: m[2] ? 'key' : 'str', text: m[1] })
            if (m[2]) out.push({ cls: 'punc', text: m[2] })
        } else if (m[3]) out.push({ cls: 'num', text: m[3] })
        else if (m[4]) out.push({ cls: 'lit', text: m[4] })
        else out.push({ cls: 'punc', text: m[5] })
        last = at + m[0].length
    }
    if (last < text.length) out.push({ text: text.slice(last) })
    return out
}

/**
 * Monospace editor with a line-number gutter, optional JSON validation (error line is flagged and
 * the status bar says what is wrong), format / minify buttons and syntax colors.
 * Controlled via `value` + `onChange`. Tab inserts two spaces.
 */
export function CodeField({
    value,
    onChange,
    onValidityChange,
    language = 'text',
    highlight,
    minRows = 10,
    readOnly,
    disabled,
    placeholder,
    label,
    description,
    actions,
    className
}: {
    /** extra buttons in the status bar (the JSON ones are added automatically) */
    actions?: ReactNode
    className?: string
    description?: ReactNode
    disabled?: boolean
    /** colorize JSON tokens (needs language="json"); costs a hidden mirror layer */
    highlight?: boolean
    label?: ReactNode
    /** 'json' turns on validation + format / minify; 'text' is a plain editor */
    language?: 'json' | 'text'
    minRows?: number
    onChange?: (value: string) => void
    onValidityChange?: (ok: boolean) => void
    placeholder?: string
    readOnly?: boolean
    value: string
}) {
    const { t } = useTranslation('forms')
    const json = language === 'json'
    const area = useRef<HTMLTextAreaElement>(null)
    const gutter = useRef<HTMLDivElement>(null)
    const mirror = useRef<HTMLPreElement>(null)

    const check = useMemo<JsonCheck>(() => (json ? checkJson(value) : { ok: true }), [json, value])
    const lines = value.split('\n').length
    const tokens = useMemo(() => (json && highlight ? tokenize(value) : null), [json, highlight, value])

    useEffect(() => {
        onValidityChange?.(check.ok)
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [check.ok])

    const sync = () => {
        const a = area.current
        if (!a) return
        if (gutter.current) gutter.current.scrollTop = a.scrollTop
        if (mirror.current) mirror.current.style.transform = `translate(${-a.scrollLeft}px, ${-a.scrollTop}px)`
    }
    useEffect(sync, [value])

    const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
        if (e.key !== 'Tab' || readOnly || e.shiftKey) return
        e.preventDefault()
        const a = e.currentTarget
        const s = a.selectionStart
        onChange?.(`${value.slice(0, s)}  ${value.slice(a.selectionEnd)}`)
        requestAnimationFrame(() => {
            a.selectionStart = a.selectionEnd = s + 2
        })
    }

    const rewrite = (pretty: boolean) => {
        if (!check.ok) {
            toast.error(t('code.cannotFormat'), t('code.fixFirst'), 3600)
            return
        }
        const parsed = JSON.parse(value)
        onChange?.(pretty ? JSON.stringify(parsed, null, 2) : JSON.stringify(parsed))
    }

    return (
        <Input.Wrapper className={className} description={description} label={label}>
            <div className={styles.box} data-disabled={disabled || undefined} data-err={!check.ok || undefined}>
                <div className={styles.grid} style={{ ['--rows' as string]: minRows }}>
                    <div aria-hidden className={styles.gutter} ref={gutter}>
                        {Array.from({ length: lines }, (_, i) => (
                            <span className={i + 1 === check.line ? styles.badLine : undefined} key={i}>
                                {i + 1}
                            </span>
                        ))}
                    </div>
                    <div className={styles.editor}>
                        {tokens && (
                            <pre aria-hidden className={styles.mirror} ref={mirror}>
                                {tokens.map((tk, i) => (
                                    <span className={tk.cls ? styles[tk.cls] : undefined} key={i}>
                                        {tk.text}
                                    </span>
                                ))}
                                {'\n'}
                            </pre>
                        )}
                        <textarea
                            aria-invalid={!check.ok}
                            autoCapitalize="off"
                            autoComplete="off"
                            autoCorrect="off"
                            className={cx(styles.ta, tokens && styles.taClear)}
                            disabled={disabled}
                            onChange={(e) => onChange?.(e.currentTarget.value)}
                            onKeyDown={onKeyDown}
                            onScroll={sync}
                            placeholder={placeholder}
                            readOnly={readOnly}
                            ref={area}
                            spellCheck={false}
                            value={value}
                            wrap="off"
                        />
                    </div>
                </div>
                <div className={styles.bar}>
                    <span aria-live="polite" className={styles.stat} data-state={check.ok ? 'ok' : 'err'}>
                        {json
                            ? check.ok
                                ? t('code.valid')
                                : t('code.error', { line: check.line, message: check.message })
                            : t('code.lines', { count: lines })}
                    </span>
                    {json && !readOnly && (
                        <>
                            <Button
                                disabled={disabled}
                                leftSection={<Icon name="wand" size={13} />}
                                onClick={() => rewrite(true)}
                                size="xs"
                                variant="default"
                            >
                                {t('code.format')}
                            </Button>
                            <Button
                                disabled={disabled}
                                leftSection={<Icon name="braces" size={13} />}
                                onClick={() => rewrite(false)}
                                size="xs"
                                variant="default"
                            >
                                {t('code.minify')}
                            </Button>
                        </>
                    )}
                    {actions}
                </div>
            </div>
        </Input.Wrapper>
    )
}
