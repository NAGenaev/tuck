// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Switch } from '@mantine/core'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Modal } from '@shared/ui/kit'

import { fmtRule, NAME_RE, RESERVED_NAMES, TEMPLATES, TemplateId } from '../lib/policy-text'
import s from '../policies.module.css'

export type NewTemplate = 'none' | TemplateId

export function NewPolicyModal({
    opened,
    initialTemplate = 'none',
    existing,
    onClose,
    onCreate
}: {
    existing: string[]
    initialTemplate?: NewTemplate
    onClose: () => void
    onCreate: (v: { inheritable: boolean; name: string; template: NewTemplate }) => void
    opened: boolean
}) {
    const { t } = useTranslation('policies')
    const [name, setName] = useState('')
    const [touched, setTouched] = useState(false)
    const [tpl, setTpl] = useState<NewTemplate>('none')
    const [inh, setInh] = useState(false)

    useEffect(() => {
        if (!opened) return
        setName('')
        setTouched(false)
        setTpl(initialTemplate)
        setInh(false)
    }, [opened, initialTemplate])

    const v = name.trim()
    const error = !v
        ? t('create.errEmpty')
        : RESERVED_NAMES.includes(v)
          ? t('create.errReserved', { name: v })
          : !NAME_RE.test(v)
            ? t('create.errFormat')
            : existing.includes(v)
              ? t('create.errExists', { name: v })
              : ''

    const submit = () => {
        setTouched(true)
        if (error) return
        onCreate({ inheritable: inh, name: v, template: tpl })
    }

    const options: Array<{ id: NewTemplate; label: string }> = [
        { id: 'none', label: t('create.tplNone') },
        { id: 'ro', label: t('tpl.ro') },
        { id: 'rw', label: t('tpl.rw') },
        { id: 'admin', label: t('tpl.admin') }
    ]
    const preview = tpl === 'none' ? t('create.previewNone') : fmtRule(TEMPLATES[tpl].path, TEMPLATES[tpl].caps)

    return (
        <Modal
            footer={
                <>
                    <Button onClick={onClose} variant="default">
                        {t('cancel')}
                    </Button>
                    <Button onClick={submit}>{t('create.submit')}</Button>
                </>
            }
            icon="plus"
            onClose={onClose}
            opened={opened}
            size={560}
            title={t('create.title')}
        >
            <form
                className={s.mBody}
                noValidate
                onSubmit={(e) => {
                    e.preventDefault()
                    submit()
                }}
            >
                <p className={s.mHint}>{t('create.hint')}</p>
                <div className={s.field}>
                    <label className={s.fieldLabel} htmlFor="pol-new-name">
                        {t('create.name')}
                    </label>
                    <input
                        autoComplete="off"
                        autoFocus
                        className={s.inp}
                        data-autofocus
                        id="pol-new-name"
                        onChange={(e) => {
                            setName(e.currentTarget.value)
                            setTouched(true)
                        }}
                        placeholder="payments-reader"
                        spellCheck={false}
                        style={touched && error ? { borderColor: 'var(--alarm)' } : undefined}
                        value={name}
                    />
                    <span
                        style={{
                            color: 'var(--alarm)',
                            font: '500 12px var(--mono)',
                            minHeight: 18
                        }}
                    >
                        {touched ? error : ''}
                    </span>
                </div>
                <div className={s.field}>
                    <span className={s.fieldLabel}>{t('quickFill')}</span>
                    <div className={s.chips}>
                        {options.map((o) => (
                            <button
                                className={s.chip}
                                data-on={o.id === tpl}
                                key={o.id}
                                onClick={() => setTpl(o.id)}
                                type="button"
                            >
                                {o.label}
                            </button>
                        ))}
                    </div>
                    <div className={s.mPrev}>{preview}</div>
                </div>
                <Switch
                    checked={inh}
                    label={t('inheritable')}
                    onChange={(e) => setInh(e.currentTarget.checked)}
                />
                <button hidden type="submit" />
            </form>
        </Modal>
    )
}
