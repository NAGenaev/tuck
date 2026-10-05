// SPDX-License-Identifier: AGPL-3.0-only
import { Button, TextInput } from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { getKVv1, putKVv1 } from '@shared/api/endpoints/kv'
import { checkJson, CodeField, Drawer, toast } from '@shared/ui/kit'

import styles from '../kv1.module.css'
import { errLine, parseObject, validateName } from '../lib'
import { OverwriteStage } from './overwrite-stage'

export interface DrawerInit {
    /** folder prefix shown before the name input ('' when the name is a full path) */
    base: string
    name: string
    /** current value when overwriting from the detail pane; null for a brand new secret */
    value: null | string
    /** keys of the folder the user is browsing (cheap "already exists" signal) */
    siblings: string[]
    /** folder currently listed (siblings belong to it) */
    listPrefix: string
}

type Mode = 'json' | 'raw'

export function WriteDrawer({
    open,
    init,
    onClose,
    onSaved
}: {
    init: DrawerInit
    onClose: () => void
    onSaved: (full: string, overwrote: boolean) => void
    open: boolean
}) {
    const { t } = useTranslation('kv1')
    const [name, setName] = useState(init.name)
    const [mode, setMode] = useState<Mode>(() => (init.value === null || parseObject(init.value) ? 'json' : 'raw'))
    const [raw, setRaw] = useState(() => {
        if (init.value === null) return ''
        const o = parseObject(init.value)
        return o ? JSON.stringify(o, null, 2) : init.value
    })
    const [touchedName, setTouchedName] = useState(!!init.name)
    const [touchedVal, setTouchedVal] = useState(false)
    const [busy, setBusy] = useState(false)
    const [shake, setShake] = useState<'name' | 'val' | null>(null)
    const [stage, setStage] = useState<null | { newText: string; oldText: null | string }>(null)
    const shakeTimer = useRef<number | undefined>(undefined)
    useEffect(() => () => window.clearTimeout(shakeTimer.current), [])

    const trimmed = name.trim()
    const full = init.base + trimmed
    const issue = validateName(trimmed)
    const [debounced] = useDebouncedValue(trimmed, 300)
    const probe = useQuery({
        queryKey: ['kv1', 'get', init.base + debounced],
        queryFn: () => getKVv1(init.base + debounced),
        enabled: open && !issue && debounced === trimmed,
        retry: false,
        staleTime: 0
    })
    const inList = init.base === init.listPrefix && init.siblings.includes(trimmed)
    const exists = !issue && (inList || (probe.isSuccess && debounced === trimmed))
    const oldValue = probe.isSuccess && probe.data.encoding !== 'base64' ? (probe.data.value ?? '') : null

    const jsonState: 'bad' | 'empty' | 'ok' =
        mode === 'raw' ? 'ok' : !raw.trim() ? 'empty' : checkJson(raw).ok ? 'ok' : 'bad'

    const nameMsg = (() => {
        if (issue && touchedName) {
            const key = { chars: 'nameChars', empty: 'nameEmpty', slashes: 'nameSlashes', space: 'nameSpace' }[issue]
            return { kind: 'err', text: `✕ ${t(key)}` }
        }
        if (issue) return null
        return exists
            ? { kind: 'wr', text: `◆ ${t('nameExists')}` }
            : { kind: 'inf', text: `→ secret/${full}` }
    })()

    const bump = (which: 'name' | 'val') => {
        setShake(null)
        window.requestAnimationFrame(() => setShake(which))
        window.clearTimeout(shakeTimer.current)
        shakeTimer.current = window.setTimeout(() => setShake(null), 400)
    }

    const save = async () => {
        if (busy || stage) return
        setTouchedName(true)
        setTouchedVal(true)
        if (issue) return bump('name')
        if (jsonState !== 'ok') {
            bump('val')
            if (jsonState === 'bad') toast.error(t('fixJson'), t('fixJsonText'))
            return
        }
        const value = mode === 'json' ? raw.trim() : raw
        if (!exists) {
            setBusy(true)
            try {
                await putKVv1(full, value)
                onSaved(full, false)
            } catch (e) {
                toast.error(t('writeFailed'), `PUT secret/${full} → ${errLine(e)}`)
            } finally {
                setBusy(false)
            }
            return
        }
        setStage({ newText: value, oldText: oldValue })
    }

    const stageValue = stage?.newText ?? ''
    const busyAll = busy || !!stage

    return (
        <Drawer
            busy={busyAll}
            footer={
                stage ? undefined : (
                    <>
                        <Button
                            color={exists ? 'red' : undefined}
                            loading={busy}
                            onClick={() => void save()}
                            variant={exists ? 'filled' : undefined}
                        >
                            {exists ? t('overwriteForever') : t('save')}
                        </Button>
                        <Button disabled={busy} onClick={onClose} variant="default">
                            {t('cancel')}
                        </Button>
                    </>
                )
            }
            icon="key"
            onClose={onClose}
            opened={open}
            size={600}
            title={t('drawerTitle')}
        >
            {stage ? (
                <OverwriteStage
                    newText={stageValue}
                    oldText={stage.oldText}
                    onDone={() => onSaved(full, true)}
                    onFail={(e) => {
                        setStage(null)
                        toast.error(t('writeFailed'), `PUT secret/${full} → ${errLine(e)}`)
                    }}
                    path={full}
                    perform={() => putKVv1(full, stageValue)}
                />
            ) : (
                <div className={styles.form}>
                    <div>
                        <label className={styles.lbl} htmlFor="kv1-name">
                            {t('name')}
                            <em>{init.base === '' && init.name ? t('nameHintOverwrite') : t('nameHintPrefix')}</em>
                        </label>
                        <div className={`${styles.prefixed} ${shake === 'name' ? styles.shake : ''}`}>
                            <span title={`secret/${init.base}`}>{`secret/${init.base}`}</span>
                            <TextInput
                                autoComplete="off"
                                data-autofocus
                                error={!!issue && touchedName}
                                id="kv1-name"
                                onBlur={() => setTouchedName(true)}
                                onChange={(e) => {
                                    setName(e.currentTarget.value)
                                    setTouchedName(true)
                                }}
                                placeholder={t('namePlaceholder')}
                                spellCheck={false}
                                styles={{ input: { fontFamily: 'var(--mono)' } }}
                                value={name}
                            />
                        </div>
                        <div className={styles.msg} data-t={nameMsg?.kind}>
                            {nameMsg?.text}
                        </div>
                    </div>

                    {exists && (
                        <div className={styles.owarn} role="alert">
                            <b>{t('owarnTitle')}</b>
                            <Trans
                                components={{ code: <code />, u: <u /> }}
                                i18nKey="owarnText"
                                ns="kv1"
                                values={{ path: `secret/${full}` }}
                            />
                        </div>
                    )}

                    <div className={shake === 'val' ? styles.shake : undefined}>
                        <div className={styles.lbl}>
                            <span>{t('value')}</span>
                            <div className={styles.modes} role="tablist">
                                {(['json', 'raw'] as const).map((m) => (
                                    <button
                                        aria-selected={mode === m}
                                        data-on={mode === m || undefined}
                                        key={m}
                                        onClick={() => setMode(m)}
                                        role="tab"
                                        type="button"
                                    >
                                        {m === 'json' ? t('modeJson') : t('modeRaw')}
                                    </button>
                                ))}
                            </div>
                        </div>
                        <CodeField
                            highlight={mode === 'json'}
                            language={mode === 'json' ? 'json' : 'text'}
                            minRows={9}
                            onChange={(v) => {
                                setRaw(v)
                                setTouchedVal(true)
                            }}
                            placeholder={t('valuePlaceholder')}
                            value={raw}
                        />
                        <div
                            className={styles.msg}
                            data-t={mode === 'raw' ? 'inf' : jsonState === 'empty' && touchedVal ? 'err' : undefined}
                        >
                            {mode === 'raw'
                                ? t('rawNote')
                                : jsonState === 'empty' && touchedVal
                                  ? `✕ ${t('valueEmpty')}`
                                  : null}
                        </div>
                    </div>
                </div>
            )}
        </Drawer>
    )
}
