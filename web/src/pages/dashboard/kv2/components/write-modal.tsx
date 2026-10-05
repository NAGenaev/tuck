// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useQueryClient } from '@tanstack/react-query'
import { CSSProperties, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { getKVv2Meta } from '@shared/api/endpoints/kvv2'
import { cx, Icon, Modal, prefersReducedMotion, toast } from '@shared/ui/kit'

import { errMessage, errStatus, writeSecret } from '../api'
import common from '../common.module.css'
import { useKv2Meta } from '../use-kv2'
import { dirName, KEY_PATTERN, pairsToJson, parseValue, validateName } from '../utils'
import styles from '../write.module.css'
import type { EditSeed } from './secret-drawer'

interface Row {
    id: number
    key: string
    out?: boolean
    show: boolean
    value: string
}

const FORM_ID = 'kv2-write-form'

export function WriteModal({
    opened,
    onClose,
    prefix,
    seed,
    folders,
    onSaved
}: {
    folders: Set<string>
    onClose: () => void
    onSaved: (path: string, version?: number) => void
    opened: boolean
    prefix: string
    seed: EditSeed | null
}) {
    const { t } = useTranslation('kv')
    const qc = useQueryClient()
    const nextId = useRef(1)
    const mkRow = (key = '', value = ''): Row => ({ id: nextId.current++, key, show: false, value })

    const [name, setName] = useState('')
    const [rows, setRows] = useState<Row[]>(() => [mkRow()])
    const [mode, setMode] = useState<'pairs' | 'text'>('pairs')
    const [text, setText] = useState('')
    const [showText, setShowText] = useState(false)
    const [cas, setCas] = useState('')
    const [tried, setTried] = useState(false)
    const [saving, setSaving] = useState(false)
    const [conflict, setConflict] = useState<null | { actual: number }>(null)
    const [serverErr, setServerErr] = useState('')
    const formRef = useRef<HTMLFormElement | null>(null)

    const base = seed ? dirName(seed.path) : prefix

    // reset every time the modal opens
    useEffect(() => {
        if (!opened) return
        setTried(false)
        setSaving(false)
        setConflict(null)
        setServerErr('')
        setShowText(false)
        if (seed) {
            setName(seed.path.slice(dirName(seed.path).length))
            setCas(seed.version !== undefined ? String(seed.version) : '')
            const parsed = seed.value !== undefined ? parseValue(seed.value) : null
            if (parsed?.kind === 'pairs') {
                setMode('pairs')
                setRows(parsed.pairs.map(([k, v]) => mkRow(k, v)))
                setText('')
            } else if (parsed?.kind === 'text') {
                setMode('text')
                setText(parsed.text)
                setRows([mkRow()])
            } else {
                setMode('pairs')
                setRows([mkRow()])
                setText('')
            }
        } else {
            setName('')
            setCas('')
            setMode('pairs')
            setRows([mkRow()])
            setText('')
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [opened])

    const trimmed = name.trim()
    const fullPath = `${base}${trimmed}`
    const nameReason = validateName(name)
    const isFolder = !nameReason && folders.has(`${fullPath}/`)

    const [debPath] = useDebouncedValue(fullPath, 350)
    const lookup = opened && !nameReason && debPath === fullPath
    const metaQ = useKv2Meta(fullPath, lookup)
    const existing = metaQ.data
    const missing = metaQ.isError && errStatus(metaQ.error) === 404
    const existingVersion = existing?.current_version

    const errors = useMemo(() => {
        let nameErr = ''
        if (nameReason === 'empty') nameErr = t('kv2.write.nameEmpty')
        else if (nameReason === 'format') nameErr = t('kv2.write.nameFormat')
        else if (isFolder) nameErr = t('kv2.write.nameFolder', { path: fullPath })

        let valueErr = ''
        const rowErr = new Map<number, { k?: boolean; v?: boolean }>()
        if (mode === 'pairs') {
            const seen = new Set<string>()
            for (const r of rows) {
                const k = r.key.trim()
                const e: { k?: boolean; v?: boolean } = {}
                if (!k) {
                    e.k = true
                    valueErr ||= t('kv2.write.keyEmpty')
                } else if (!KEY_PATTERN.test(k)) {
                    e.k = true
                    valueErr ||= t('kv2.write.keyFormat')
                } else if (seen.has(k)) {
                    e.k = true
                    valueErr ||= t('kv2.write.keyDup', { key: k })
                }
                seen.add(k)
                if (!r.value) {
                    e.v = true
                    valueErr ||= t('kv2.write.valueEmpty')
                }
                if (e.k || e.v) rowErr.set(r.id, e)
            }
        } else if (!text) {
            valueErr = t('kv2.write.valueEmpty')
        }

        let casErr = ''
        const c = cas.trim()
        if (c !== '' && !/^\d+$/.test(c)) casErr = t('kv2.write.casFormat')

        return { casErr, nameErr, rowErr, valueErr }
    }, [cas, fullPath, isFolder, mode, nameReason, rows, t, text])

    const hasErrors = !!(errors.nameErr || errors.valueErr || errors.casErr)
    const show = tried

    const setRow = (id: number, patch: Partial<Row>) =>
        setRows((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)))

    const addRow = () => {
        const r = mkRow()
        setRows((rs) => [...rs, r])
        window.setTimeout(() => {
            const el = formRef.current?.querySelector<HTMLInputElement>(`[data-row="${r.id}"] input`)
            el?.focus()
            el?.scrollIntoView({ block: 'nearest' })
        }, 30)
    }

    const removeRow = (id: number) => {
        if (rows.length < 2) return
        const rm = () => setRows((rs) => (rs.length > 1 ? rs.filter((r) => r.id !== id) : rs))
        if (prefersReducedMotion()) {
            rm()
            return
        }
        setRow(id, { out: true })
        window.setTimeout(rm, 220)
    }

    const switchMode = (m: 'pairs' | 'text') => {
        if (m === mode) return
        if (m === 'text') {
            const filled = rows.filter((r) => r.key.trim())
            if (filled.length > 0 && !text) setText(pairsToJson(filled))
        } else {
            const parsed = parseValue(text)
            if (parsed.kind === 'pairs') setRows(parsed.pairs.map(([k, v]) => mkRow(k, v)))
        }
        setMode(m)
    }

    const submit = async () => {
        setTried(true)
        setConflict(null)
        setServerErr('')
        if (hasErrors) {
            window.setTimeout(() => {
                const bad = formRef.current?.querySelector<HTMLElement>('[data-err] input, [data-err] textarea')
                bad?.focus()
            }, 0)
            return
        }
        setSaving(true)
        try {
            const value = mode === 'pairs' ? pairsToJson(rows) : text
            const c = cas.trim()
            const version = await writeSecret(fullPath, value, c === '' ? undefined : Number(c))
            await Promise.all([
                qc.invalidateQueries({ queryKey: ['kv2', 'list'] }),
                qc.invalidateQueries({ queryKey: ['kv2', 'get', fullPath] }),
                qc.invalidateQueries({ queryKey: ['kv2', 'meta', fullPath] })
            ])
            toast.success(t('kv2.toast.saved'), `${fullPath}${version !== undefined ? ` · v${version}` : ''}`)
            onSaved(fullPath, version)
            onClose()
        } catch (err) {
            if (errStatus(err) === 409) {
                let actual = 0
                try {
                    const m = await qc.fetchQuery({
                        queryFn: () => getKVv2Meta(fullPath),
                        queryKey: ['kv2', 'meta', fullPath],
                        staleTime: 0
                    })
                    actual = m.current_version ?? 0
                } catch {
                    actual = 0
                }
                setConflict({ actual })
                toast.error(t('kv2.write.toast409'), t('kv2.write.toast409Text'))
            } else {
                const msg = errMessage(err) || t('kv2.toast.failed')
                setServerErr(msg)
                toast.error(t('kv2.toast.failed'), msg)
            }
        } finally {
            setSaving(false)
        }
    }

    const nameState = !trimmed
        ? ''
        : isFolder
          ? t('kv2.write.isFolder')
          : metaQ.isFetching
            ? ''
            : existing
              ? t('kv2.write.exists', { version: existingVersion })
              : missing
                ? t('kv2.write.isNew')
                : ''

    const casHint = existing ? (
        <>
            {t('kv2.write.casNow')} <b>{existingVersion}</b>.{' '}
            <button
                className={common.linkBtn}
                onClick={() => setCas(String(existingVersion ?? 0))}
                type="button"
            >
                {t('kv2.write.casFill', { version: existingVersion })}
            </button>
        </>
    ) : missing && trimmed ? (
        t('kv2.write.casMissing')
    ) : (
        t('kv2.write.casOptional')
    )

    const maskStyle = (on: boolean): CSSProperties =>
        ({ WebkitTextSecurity: on ? 'none' : 'disc' }) as CSSProperties

    return (
        <Modal
            busy={saving}
            footer={
                <>
                    <Button disabled={saving} onClick={onClose} variant="default">
                        {t('kv2.cancel')}
                    </Button>
                    <Button form={FORM_ID} loading={saving} type="submit">
                        {t('kv2.write.save')}
                    </Button>
                </>
            }
            icon="key"
            onClose={onClose}
            opened={opened}
            size={680}
            title={seed ? t('kv2.write.titleEdit') : t('kv2.write.title')}
        >
            <form
                className={styles.form}
                id={FORM_ID}
                noValidate
                onSubmit={(e) => {
                    e.preventDefault()
                    void submit()
                }}
                ref={formRef}
            >
                {conflict && (
                    <div className={common.banner} data-tone="r" role="alert">
                        <Icon name="warn" size={20} />
                        <h5>{t('kv2.write.conflictTitle')}</h5>
                        <p>
                            {t('kv2.write.conflictText', { actual: conflict.actual, given: cas.trim() })}
                        </p>
                        <div className={common.acts}>
                            <Button
                                onClick={() => {
                                    setCas(String(conflict.actual))
                                    setConflict(null)
                                }}
                                size="compact-sm"
                            >
                                {t('kv2.write.casFill', { version: conflict.actual })}
                            </Button>
                            <Button
                                onClick={() => {
                                    setCas('')
                                    setConflict(null)
                                }}
                                size="compact-sm"
                                variant="default"
                            >
                                {t('kv2.write.noCas')}
                            </Button>
                        </div>
                    </div>
                )}
                {serverErr && (
                    <div className={common.banner} data-tone="r" role="alert">
                        <Icon name="warn" size={20} />
                        <h5>{t('kv2.toast.failed')}</h5>
                        <p>{serverErr}</p>
                    </div>
                )}

                <div className={common.fld}>
                    <label className={common.lab} htmlFor="kv2-wname">
                        {t('kv2.write.name')}
                        <small className={styles.exists}>{nameState}</small>
                    </label>
                    <div className={common.inp} data-err={(show && !!errors.nameErr) || undefined}>
                        <span className={styles.pfx} title={`secret/${base}`}>
                            secret/{base}
                        </span>
                        <input
                            autoComplete="off"
                            data-autofocus={!seed || undefined}
                            id="kv2-wname"
                            onChange={(e) => setName(e.currentTarget.value)}
                            placeholder={t('kv2.write.namePlaceholder')}
                            spellCheck={false}
                            value={name}
                        />
                    </div>
                    <div className={common.msg} data-err={(show && !!errors.nameErr) || undefined}>
                        {show && errors.nameErr ? errors.nameErr : <>{t('kv2.write.nameHint')}</>}
                    </div>
                </div>

                <div className={common.fld}>
                    <div className={styles.modeBar}>
                        <div className={common.lab}>
                            {t('kv2.write.value')}
                            {mode === 'pairs' && <small>{t('kv2.write.keys', { count: rows.length })}</small>}
                        </div>
                        <div className={common.seg} role="group">
                            <button data-on={mode === 'pairs' || undefined} onClick={() => switchMode('pairs')} type="button">
                                {t('kv2.write.modePairs')}
                            </button>
                            <button data-on={mode === 'text' || undefined} onClick={() => switchMode('text')} type="button">
                                {t('kv2.write.modeText')}
                            </button>
                        </div>
                    </div>

                    {mode === 'pairs' ? (
                        <>
                            <div className={styles.rows}>
                                {rows.map((r, i) => {
                                    const e = show ? errors.rowErr.get(r.id) : undefined
                                    return (
                                        <div className={styles.row} data-out={r.out || undefined} data-row={r.id} key={r.id}>
                                            <div className={common.inp} data-err={e?.k || undefined}>
                                                <input
                                                    aria-label={t('kv2.write.keyN', { n: i + 1 })}
                                                    autoComplete="off"
                                                    onChange={(ev) => setRow(r.id, { key: ev.currentTarget.value })}
                                                    placeholder={t('kv2.write.keyPlaceholder')}
                                                    spellCheck={false}
                                                    value={r.key}
                                                />
                                            </div>
                                            <div className={common.inp} data-err={e?.v || undefined}>
                                                <input
                                                    aria-label={t('kv2.write.valueN', { n: i + 1 })}
                                                    autoComplete="off"
                                                    onChange={(ev) => setRow(r.id, { value: ev.currentTarget.value })}
                                                    placeholder={t('kv2.write.valuePlaceholder')}
                                                    spellCheck={false}
                                                    type={r.show ? 'text' : 'password'}
                                                    value={r.value}
                                                />
                                                <button
                                                    aria-label={r.show ? t('kv2.value.hide') : t('kv2.value.show')}
                                                    className={styles.eye}
                                                    onClick={() => setRow(r.id, { show: !r.show })}
                                                    type="button"
                                                >
                                                    <Icon name={r.show ? 'eyeoff' : 'eye'} size={15} />
                                                </button>
                                            </div>
                                            <button
                                                aria-label={t('kv2.write.removeKey')}
                                                className={common.ibtn}
                                                disabled={rows.length < 2}
                                                onClick={() => removeRow(r.id)}
                                                type="button"
                                            >
                                                <Icon name="x" size={15} />
                                            </button>
                                        </div>
                                    )
                                })}
                            </div>
                            <div className={styles.addRow}>
                                <Button
                                    leftSection={<Icon name="plus" size={14} />}
                                    onClick={addRow}
                                    size="compact-sm"
                                    variant="default"
                                >
                                    {t('kv2.write.addKey')}
                                </Button>
                                <span className={common.msg} data-err={(show && !!errors.valueErr) || undefined}>
                                    {show ? errors.valueErr : ''}
                                </span>
                            </div>
                        </>
                    ) : (
                        <>
                            <div className={common.inp} data-err={(show && !!errors.valueErr) || undefined}>
                                <textarea
                                    aria-label={t('kv2.write.value')}
                                    autoComplete="off"
                                    onChange={(ev) => setText(ev.currentTarget.value)}
                                    placeholder={t('kv2.write.textPlaceholder')}
                                    rows={5}
                                    spellCheck={false}
                                    style={maskStyle(showText)}
                                    value={text}
                                />
                                <button
                                    aria-label={showText ? t('kv2.value.hide') : t('kv2.value.show')}
                                    className={styles.eye}
                                    onClick={() => setShowText((v) => !v)}
                                    type="button"
                                >
                                    <Icon name={showText ? 'eyeoff' : 'eye'} size={15} />
                                </button>
                            </div>
                            <div className={common.msg} data-err={(show && !!errors.valueErr) || undefined}>
                                {show && errors.valueErr ? errors.valueErr : t('kv2.write.textHint')}
                            </div>
                        </>
                    )}
                </div>

                <div className={common.fld}>
                    <label className={common.lab} htmlFor="kv2-wcas">
                        {t('kv2.write.cas')} <small>{t('kv2.write.optional')}</small>
                    </label>
                    <div className={styles.casbox} data-err={(show && !!errors.casErr) || undefined}>
                        <div className={common.inp} data-err={(show && !!errors.casErr) || undefined}>
                            <input
                                autoComplete="off"
                                id="kv2-wcas"
                                inputMode="numeric"
                                onChange={(e) => {
                                    setCas(e.currentTarget.value)
                                    setConflict(null)
                                }}
                                placeholder={t('kv2.write.casPlaceholder')}
                                value={cas}
                            />
                        </div>
                        <div className={cx(common.msg)} data-err={(show && !!errors.casErr) || undefined}>
                            {show && errors.casErr ? errors.casErr : casHint}
                        </div>
                        <div className={styles.casx}>
                            <div>
                                <b>{t('kv2.write.casEmptyB')}</b>
                                {t('kv2.write.casEmpty')}
                            </div>
                            <div>
                                <b>0</b>
                                {t('kv2.write.cas0')}
                            </div>
                            <div>
                                <b>N</b>
                                {t('kv2.write.casN')}
                            </div>
                        </div>
                    </div>
                </div>
            </form>
        </Modal>
    )
}
