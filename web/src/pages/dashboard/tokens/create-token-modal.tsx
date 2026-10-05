// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Switch } from '@mantine/core'
import { useMutation, useQuery } from '@tanstack/react-query'
import { FormEvent, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { listPolicies } from '@shared/api/endpoints/policy'
import { getNamespace } from '@shared/auth/token'
import { cx, Modal, SkeletonLine, toast, TtlField } from '@shared/ui/kit'

import { apiError, createTokenFull, TokenInfo } from './tokens.api'
import s from './tokens.module.css'

const MIN_TTL = 60
const DEFAULT_TTL = 86400

export function CreateTokenModal({
    opened,
    onClose,
    onCreated
}: {
    onClose: () => void
    onCreated: (token: TokenInfo) => void
    opened: boolean
}) {
    const { t } = useTranslation('tokens')
    const [name, setName] = useState('')
    const [picked, setPicked] = useState<string[]>([])
    const [typedPolicies, setTypedPolicies] = useState('')
    const [ttl, setTtl] = useState(DEFAULT_TTL)
    const [uses, setUses] = useState('0')
    const [renewable, setRenewable] = useState(false)
    const [noParent, setNoParent] = useState(false)
    const [tried, setTried] = useState(false)

    const policiesQ = useQuery({
        enabled: opened,
        queryFn: listPolicies,
        queryKey: ['policy', 'list'],
        retry: false
    })
    const available = policiesQ.data ?? []
    const useChips = policiesQ.isSuccess && available.length > 0

    useEffect(() => {
        if (!opened) return
        setName('')
        setPicked([])
        setTypedPolicies('')
        setTtl(DEFAULT_TTL)
        setUses('0')
        setRenewable(false)
        setNoParent(false)
        setTried(false)
    }, [opened])

    const mutation = useMutation({
        mutationFn: createTokenFull,
        onError: (e) => toast.error(t('toast.createFailed'), apiError(e)),
        onSuccess: (token) => onCreated(token)
    })

    const policies = useChips
        ? picked
        : typedPolicies
              .split(',')
              .map((p) => p.trim())
              .filter(Boolean)
    const nameBad = name.trim().length > 64
    const ttlBad = ttl < MIN_TTL
    const usesBad = !/^\d{1,9}$/.test(uses.trim())

    const submit = (e: FormEvent) => {
        e.preventDefault()
        setTried(true)
        if (nameBad || ttlBad || usesBad || mutation.isPending) return
        mutation.mutate({
            display_name: name.trim(),
            max_uses: Number(uses),
            no_parent: noParent,
            policies,
            renewable,
            ttl: `${ttl}s`
        })
    }

    const toggle = (p: string) => setPicked((cur) => (cur.includes(p) ? cur.filter((x) => x !== p) : [...cur, p]))

    return (
        <Modal
            busy={mutation.isPending}
            footer={
                <>
                    <Button disabled={mutation.isPending} onClick={onClose} variant="default">
                        {t('create.cancel')}
                    </Button>
                    <Button form="tk-create-form" loading={mutation.isPending} type="submit">
                        {t('create.submit')}
                    </Button>
                </>
            }
            icon="key"
            onClose={onClose}
            opened={opened}
            size={560}
            title={t('create.title')}
        >
            <form className={s.form} id="tk-create-form" noValidate onSubmit={submit}>
                <p className={s.hint} style={{ margin: 0 }}>
                    {t('create.desc')}
                </p>

                <div className={s.fld}>
                    <label className={s.lbl} htmlFor="tk-name">
                        {t('create.name')}
                    </label>
                    <input
                        autoComplete="off"
                        autoFocus
                        className={cx(s.field, tried && nameBad && s.fieldBad)}
                        id="tk-name"
                        onChange={(e) => setName(e.currentTarget.value)}
                        placeholder="ci-deploy"
                        spellCheck={false}
                        value={name}
                    />
                    {tried && nameBad && <span className={s.errMsg}>{t('create.nameErr')}</span>}
                </div>

                <div className={s.fld}>
                    {useChips ? (
                        <>
                            <span className={s.lbl}>
                                {t('create.policies')}
                                <i>{t('create.policiesCount', { count: picked.length })}</i>
                            </span>
                            <div aria-label={t('create.policies')} className={s.chipbox} role="group">
                                {available.map((p) => (
                                    <button
                                        aria-pressed={picked.includes(p)}
                                        className={s.chip}
                                        key={p}
                                        onClick={() => toggle(p)}
                                        type="button"
                                    >
                                        {p}
                                    </button>
                                ))}
                            </div>
                        </>
                    ) : policiesQ.isPending ? (
                        <>
                            <span className={s.lbl}>{t('create.policies')}</span>
                            <SkeletonLine width="100%" />
                        </>
                    ) : (
                        <>
                            <label className={s.lbl} htmlFor="tk-policies">
                                {t('create.policiesText')}
                            </label>
                            <input
                                autoComplete="off"
                                className={s.field}
                                id="tk-policies"
                                onChange={(e) => setTypedPolicies(e.currentTarget.value)}
                                placeholder="default, app-read"
                                spellCheck={false}
                                value={typedPolicies}
                            />
                            <span className={s.hint}>{t('create.policiesTextHint')}</span>
                        </>
                    )}
                    {policies.length === 0 && <span className={s.hint}>{t('create.policiesNone')}</span>}
                </div>

                <div className={s.fld}>
                    <TtlField label={t('create.ttl')} onChange={setTtl} value={ttl} />
                    {tried && ttlBad && <span className={s.errMsg}>{t('create.ttlErr')}</span>}
                </div>

                <div className={s.row2}>
                    <div className={s.fld}>
                        <label className={s.lbl} htmlFor="tk-uses">
                            {t('create.uses')}
                        </label>
                        <input
                            className={cx(s.field, tried && usesBad && s.fieldBad)}
                            id="tk-uses"
                            inputMode="numeric"
                            onChange={(e) => setUses(e.currentTarget.value)}
                            value={uses}
                        />
                        {tried && usesBad ? (
                            <span className={s.errMsg}>{t('create.usesErr')}</span>
                        ) : (
                            <span className={s.hint}>{t('create.usesHint')}</span>
                        )}
                    </div>
                    <div className={s.fld}>
                        <span className={s.lbl}>{t('create.ns')}</span>
                        <code className={s.field} style={{ display: 'block' }}>
                            {getNamespace() || 'root'}
                        </code>
                        <span className={s.hint}>{t('create.nsHint')}</span>
                    </div>
                </div>

                <Switch
                    checked={renewable}
                    label={t('create.renewable')}
                    onChange={(e) => setRenewable(e.currentTarget.checked)}
                />
                <Switch
                    checked={noParent}
                    label={t('create.noParent')}
                    onChange={(e) => setNoParent(e.currentTarget.checked)}
                />
            </form>
        </Modal>
    )
}
