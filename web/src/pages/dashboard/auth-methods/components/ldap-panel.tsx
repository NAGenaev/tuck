// SPDX-License-Identifier: AGPL-3.0-only
import { Button, PasswordInput, Switch, Textarea, TextInput } from '@mantine/core'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { ConfirmModal, cx, Drawer, Icon, StateBlock, toast, TtlField } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import {
    deleteLdapRole,
    getLdapRole,
    LdapConfigFull,
    LdapRoleBody,
    LdapRoleFull,
    putLdapConfig,
    putLdapRole,
    retryUnlessDenied,
    serverMessage,
    status
} from '../api'
import styles from '../auth-methods.module.css'
import { fmtTtl, NAME_RE, nsToSeconds, sameSet, toDur } from '../lib'
import { ldapConfigQ, ldapRolesQ, qk } from '../queries'
import {
    Field,
    ForbiddenPlate,
    NamespaceField,
    PaneSkeleton,
    PolicyChips,
    PolicyField,
    rich,
    SectionHead,
    TagInput
} from './shared'

const TTL_MAX = 31_536_000
const URL_RE = /^ldaps?:\/\/\S+$/i

interface Draft {
    bind_dn: string
    bind_password: string
    ca_cert: string
    group_attr: string
    group_dn: string
    group_filter: string
    starttls: boolean
    tls_insecure: boolean
    urls: string[]
    user_attr: string
    user_dn: string
}

const toDraft = (c?: LdapConfigFull): Draft => ({
    bind_dn: c?.bind_dn ?? '',
    bind_password: c?.bind_password ?? '',
    ca_cert: c?.ca_cert ?? '',
    group_attr: c?.group_attr ?? '',
    group_dn: c?.group_dn ?? '',
    group_filter: c?.group_filter ?? '',
    starttls: !!c?.starttls,
    tls_insecure: !!c?.tls_insecure,
    urls: c?.urls ?? [],
    user_attr: c?.user_attr ?? '',
    user_dn: c?.user_dn ?? ''
})

const sameDraft = (a: Draft, b: Draft) =>
    (Object.keys(a) as Array<keyof Draft>).every((k) =>
        k === 'urls' ? sameSet(a.urls, b.urls) && a.urls.join() === b.urls.join() : a[k] === b[k]
    )

export function LdapPanel() {
    const { t } = useTranslation('auth')
    const qc = useQueryClient()

    const cfg = useQuery(ldapConfigQ())
    const roles = useQuery(ldapRolesQ())
    const names = useMemo(() => [...(roles.data ?? [])].sort((a, b) => a.localeCompare(b)), [roles.data])
    const details = useQueries({
        queries: names.map((n) => ({ queryKey: qk.ldapRole(n), queryFn: () => getLdapRole(n), retry: retryUnlessDenied }))
    })
    const byName = useMemo(() => {
        const m = new Map<string, LdapRoleFull>()
        details.forEach((d, i) => d.data && m.set(names[i], d.data))
        return m
    }, [details, names])

    const notConfigured = cfg.isError && status(cfg.error) === 404

    const [draft, setDraft] = useState<Draft>(toDraft())
    const [touched, setTouched] = useState(false)
    useEffect(() => {
        if (cfg.data) setDraft(toDraft(cfg.data))
    }, [cfg.data])
    const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setDraft((d) => ({ ...d, [k]: v }))
    const dirty = !cfg.data || !sameDraft(draft, toDraft(cfg.data))

    const urlsErr = draft.urls.length === 0 ? t('ldap.f.urlsNone') : ''
    const dnErr = draft.user_dn.trim() ? '' : t('ldap.f.userDnNone')

    const saveCfg = useMutation({
        mutationFn: () => {
            const d = draft
            return putLdapConfig({
                bind_dn: d.bind_dn.trim() || undefined,
                bind_password: d.bind_password || undefined,
                ca_cert: d.ca_cert.trim() || undefined,
                group_attr: d.group_attr.trim() || undefined,
                group_dn: d.group_dn.trim() || undefined,
                group_filter: d.group_filter.trim() || undefined,
                starttls: d.starttls,
                tls_insecure: d.tls_insecure,
                urls: d.urls,
                user_attr: d.user_attr.trim() || undefined,
                user_dn: d.user_dn.trim()
            })
        },
        onSuccess: async () => {
            toast.success(t('ldap.toast.cfgSaved'), t('ldap.toast.cfgSavedMsg'))
            await qc.invalidateQueries({ queryKey: qk.ldapConfig })
        },
        onError: (err) => toast.error(t('ldap.toast.cfgFailed'), serverMessage(err))
    })

    const [editing, setEditing] = useState<'new' | LdapRoleFull | null>(null)
    const [toDelete, setToDelete] = useState<null | string>(null)
    const [fresh, setFresh] = useState<null | string>(null)
    useEffect(() => {
        if (!fresh) return
        const id = window.setTimeout(() => setFresh(null), 2200)
        return () => window.clearTimeout(id)
    }, [fresh])

    const del = useMutation({ mutationFn: deleteLdapRole })
    const confirmDelete = async (name: string) => {
        try {
            await del.mutateAsync(name)
        } catch (err) {
            toast.error(t('ldap.toast.deleteFailed'), serverMessage(err))
            throw err
        }
        toast.success(t('ldap.toast.deleted'), t('ldap.toast.deletedMsg', { name }))
        await qc.invalidateQueries({ queryKey: qk.ldapRoles })
        qc.removeQueries({ queryKey: qk.ldapRole(name) })
    }

    if (cfg.isLoading) return <PaneSkeleton request="GET /v1/auth/ldap/config" />
    if (cfg.isError && status(cfg.error) === 403) {
        return (
            <ForbiddenPlate
                need="auth/ldap/*"
                onRetry={() => void cfg.refetch()}
                request="GET /v1/auth/ldap/config"
                retrying={cfg.isFetching}
            />
        )
    }
    if (cfg.isError && !notConfigured) {
        return (
            <StateBlock
                details={serverMessage(cfg.error)}
                kind="error"
                message={t('error.message')}
                onRetry={() => void cfg.refetch()}
                retrying={cfg.isFetching}
                title={t('error.title')}
            />
        )
    }

    const plainUrl = draft.urls.some((u) => u.toLowerCase().startsWith('ldap://')) && !draft.starttls

    return (
        <>
            <SectionHead
                actions={
                    <Pill dot tone={notConfigured ? 'r' : 'g'}>
                        {notConfigured ? t('notConfigured') : t('configured')}
                    </Pill>
                }
                eyebrow={t('ldap.eyebrow')}
                first
                text={<Trans components={rich} i18nKey="ldap.text" ns="auth" />}
                title={t('ldap.cfgTitle')}
            />

            {notConfigured && (
                <div className={cx(styles.alert, styles.info)} role="status">
                    <Icon name="info" size={18} />
                    <div>
                        <b>{t('ldap.noCfg.title')}</b>
                        {t('ldap.noCfg.text')}
                    </div>
                </div>
            )}

            <Plate>
                <form
                    className={cx(styles.plateIn, styles.form)}
                    noValidate
                    onSubmit={(e) => {
                        e.preventDefault()
                        setTouched(true)
                        if (!urlsErr && !dnErr && !saveCfg.isPending) saveCfg.mutate()
                    }}
                >
                    <Field error={touched ? urlsErr : ''} hint={t('ldap.f.urlsHint')} label="urls">
                        <TagInput
                            ariaLabel="urls"
                            onChange={(v) => set('urls', v)}
                            pattern={URL_RE}
                            placeholder="ldaps://ldap.example.com:636"
                            value={draft.urls}
                        />
                    </Field>
                    <div className={styles.grid2}>
                        <Field error={touched ? dnErr : ''} hint={t('ldap.f.userDnHint')} label="user_dn">
                            <TextInput
                                aria-label="user_dn"
                                onChange={(e) => set('user_dn', e.currentTarget.value)}
                                placeholder="ou=people,dc=example,dc=com"
                                value={draft.user_dn}
                            />
                        </Field>
                        <Field hint={t('ldap.f.userAttrHint')} label="user_attr">
                            <TextInput
                                aria-label="user_attr"
                                onChange={(e) => set('user_attr', e.currentTarget.value)}
                                placeholder="uid"
                                value={draft.user_attr}
                            />
                        </Field>
                        <Field hint={t('ldap.f.bindDnHint')} label="bind_dn">
                            <TextInput
                                aria-label="bind_dn"
                                onChange={(e) => set('bind_dn', e.currentTarget.value)}
                                placeholder="cn=svc-tuck,dc=example,dc=com"
                                value={draft.bind_dn}
                            />
                        </Field>
                        <Field hint={t('ldap.f.bindPwHint')} label="bind_password">
                            <PasswordInput
                                aria-label="bind_password"
                                autoComplete="new-password"
                                onChange={(e) => set('bind_password', e.currentTarget.value)}
                                value={draft.bind_password}
                            />
                        </Field>
                        <Field hint={t('ldap.f.groupDnHint')} label="group_dn">
                            <TextInput
                                aria-label="group_dn"
                                onChange={(e) => set('group_dn', e.currentTarget.value)}
                                placeholder="ou=groups,dc=example,dc=com"
                                value={draft.group_dn}
                            />
                        </Field>
                        <Field hint={t('ldap.f.groupAttrHint')} label="group_attr">
                            <TextInput
                                aria-label="group_attr"
                                onChange={(e) => set('group_attr', e.currentTarget.value)}
                                placeholder="cn"
                                value={draft.group_attr}
                            />
                        </Field>
                    </div>
                    <Field hint={t('ldap.f.groupFilterHint')} label="group_filter">
                        <TextInput
                            aria-label="group_filter"
                            onChange={(e) => set('group_filter', e.currentTarget.value)}
                            placeholder="(&(objectClass=groupOfNames)(member={{.UserDN}}))"
                            value={draft.group_filter}
                        />
                    </Field>
                    <Field hint={t('ldap.f.caCertHint')} label="ca_cert">
                        <Textarea
                            aria-label="ca_cert"
                            autosize
                            minRows={3}
                            onChange={(e) => set('ca_cert', e.currentTarget.value)}
                            placeholder="-----BEGIN CERTIFICATE-----"
                            spellCheck={false}
                            styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
                            value={draft.ca_cert}
                        />
                    </Field>
                    <div className={styles.grid2}>
                        <Switch
                            checked={draft.starttls}
                            description={t('ldap.f.starttlsHint')}
                            label="starttls"
                            onChange={(e) => set('starttls', e.currentTarget.checked)}
                        />
                        <Switch
                            checked={draft.tls_insecure}
                            color="red"
                            description={t('ldap.f.insecureHint')}
                            label="tls_insecure"
                            onChange={(e) => set('tls_insecure', e.currentTarget.checked)}
                        />
                    </div>
                    {draft.tls_insecure && (
                        <div className={styles.alert} role="alert">
                            <Icon name="warn" size={18} />
                            <div>
                                <b>{t('ldap.insecureWarn.title')}</b>
                                {t('ldap.insecureWarn.text')}
                            </div>
                        </div>
                    )}
                    {plainUrl && !draft.tls_insecure && (
                        <div className={cx(styles.alert, styles.info)} role="status">
                            <Icon name="info" size={18} />
                            <div>
                                <b>{t('ldap.plainWarn.title')}</b>
                                {t('ldap.plainWarn.text')}
                            </div>
                        </div>
                    )}
                    <p className={styles.hint}>{t('ldap.replaceNote')}</p>
                    <div className={styles.secActs}>
                        <Button
                            disabled={!dirty}
                            leftSection={<Icon name="check" size={14} />}
                            loading={saveCfg.isPending}
                            type="submit"
                        >
                            {t('save')}
                        </Button>
                        {cfg.data && (
                            <Button
                                disabled={!dirty}
                                onClick={() => {
                                    setDraft(toDraft(cfg.data))
                                    setTouched(false)
                                }}
                                variant="default"
                            >
                                {t('reset')}
                            </Button>
                        )}
                    </div>
                </form>
            </Plate>

            <div className={styles.sec}>
                <SectionHead
                    actions={
                        <Button leftSection={<Icon name="plus" size={14} />} onClick={() => setEditing('new')}>
                            {t('ldap.newRole')}
                        </Button>
                    }
                    eyebrow={t('roles')}
                    text={t('ldap.rolesText')}
                    title={t('ldap.rolesTitle')}
                />
                {roles.isLoading ? (
                    <PaneSkeleton request="GET /v1/auth/ldap/role/?list=true" />
                ) : roles.isError ? (
                    status(roles.error) === 403 ? (
                        <ForbiddenPlate
                            need="auth/ldap/role/*"
                            onRetry={() => void roles.refetch()}
                            request="GET /v1/auth/ldap/role/?list=true"
                            retrying={roles.isFetching}
                        />
                    ) : (
                        <StateBlock
                            details={serverMessage(roles.error)}
                            kind="error"
                            onRetry={() => void roles.refetch()}
                            retrying={roles.isFetching}
                            title={t('error.title')}
                        />
                    )
                ) : (
                    <div className={styles.tbl}>
                        <table>
                            <thead>
                                <tr>
                                    <th>{t('ldap.cols.role')}</th>
                                    <th>{t('cols.policies')}</th>
                                    <th>{t('ldap.cols.members')}</th>
                                    <th>TTL</th>
                                    <th>{t('cols.ns')}</th>
                                    <th />
                                </tr>
                            </thead>
                            <tbody>
                                {names.length === 0 && (
                                    <tr>
                                        <td className={styles.empty} colSpan={6}>
                                            {t('ldap.empty')}
                                        </td>
                                    </tr>
                                )}
                                {names.map((n) => {
                                    const r = byName.get(n)
                                    return (
                                        <tr className={cx(fresh === n && styles.fresh)} key={n}>
                                            <td className={styles.k}>{n}</td>
                                            <td>{r ? <PolicyChips list={r.policies} /> : '…'}</td>
                                            <td>
                                                {r ? (
                                                    <div className={styles.chips}>
                                                        {(r.users ?? []).map((u) => (
                                                            <span className={cx(styles.chip, styles.ice)} key={`u${u}`}>
                                                                {u}
                                                            </span>
                                                        ))}
                                                        {(r.groups ?? []).map((g) => (
                                                            <span className={styles.chip} key={`g${g}`}>
                                                                {g}
                                                            </span>
                                                        ))}
                                                        {!(r.users ?? []).length && !(r.groups ?? []).length && (
                                                            <span className={styles.dim}>—</span>
                                                        )}
                                                    </div>
                                                ) : (
                                                    '…'
                                                )}
                                            </td>
                                            <td>{r ? fmtTtl(nsToSeconds(r.ttl)) || t('default') : '…'}</td>
                                            <td>{r?.tuck_namespace || <span className={styles.dim}>root</span>}</td>
                                            <td>
                                                <div className={styles.acts}>
                                                    <button
                                                        aria-label={t('ldap.edit', { name: n })}
                                                        className={styles.ibtn}
                                                        disabled={!r}
                                                        onClick={() => r && setEditing(r)}
                                                        title={t('ldap.edit', { name: n })}
                                                        type="button"
                                                    >
                                                        <Icon name="gear" size={14} />
                                                    </button>
                                                    <button
                                                        aria-label={t('ldap.remove', { name: n })}
                                                        className={cx(styles.ibtn, styles.red)}
                                                        onClick={() => setToDelete(n)}
                                                        title={t('ldap.remove', { name: n })}
                                                        type="button"
                                                    >
                                                        <Icon name="trash" size={14} />
                                                    </button>
                                                </div>
                                            </td>
                                        </tr>
                                    )
                                })}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>

            <RoleDrawer
                existing={names}
                onClose={() => setEditing(null)}
                onSaved={async (name) => {
                    setEditing(null)
                    setFresh(name)
                    await qc.invalidateQueries({ queryKey: qk.ldapRoles })
                    await qc.invalidateQueries({ queryKey: qk.ldapRole(name) })
                }}
                target={editing}
            />

            <ConfirmModal
                confirmLabel={t('delete')}
                confirmText={toDelete ?? undefined}
                destructive
                message={<Trans components={rich} i18nKey="ldap.delete.msg" ns="auth" values={{ name: toDelete }} />}
                onClose={() => setToDelete(null)}
                onConfirm={async () => {
                    if (toDelete) await confirmDelete(toDelete)
                }}
                opened={toDelete !== null}
                title={t('ldap.delete.title')}
            />
        </>
    )
}

function RoleDrawer({
    target,
    existing,
    onClose,
    onSaved
}: {
    existing: string[]
    onClose: () => void
    onSaved: (name: string) => Promise<void> | void
    target: 'new' | LdapRoleFull | null
}) {
    const { t } = useTranslation('auth')
    const opened = target !== null
    const edit = target && target !== 'new' ? target : null

    const [name, setName] = useState('')
    const [policies, setPolicies] = useState<string[]>([])
    const [ns, setNs] = useState('')
    const [ttl, setTtl] = useState(0)
    const [users, setUsers] = useState<string[]>([])
    const [groups, setGroups] = useState<string[]>([])
    const [touched, setTouched] = useState(false)
    const [serverErr, setServerErr] = useState('')

    useEffect(() => {
        if (!opened) return
        setName(edit?.name ?? '')
        setPolicies(edit?.policies ?? [])
        setNs(edit?.tuck_namespace ?? '')
        setTtl(nsToSeconds(edit?.ttl))
        setUsers(edit?.users ?? [])
        setGroups(edit?.groups ?? [])
        setTouched(false)
        setServerErr('')
    }, [opened, edit])

    const nameErr = !NAME_RE.test(name.trim())
        ? t('f.nameBad')
        : !edit && existing.includes(name.trim())
          ? t('f.nameTaken')
          : ''
    const polErr = policies.length === 0 ? t('f.policiesNone') : ''
    const memWarn = users.length === 0 && groups.length === 0 ? t('ldap.f.membersNone') : ''

    const save = useMutation({
        mutationFn: () => {
            const body: LdapRoleBody = {
                groups: groups.length ? groups : undefined,
                policies,
                ttl: toDur(ttl),
                tuck_namespace: ns || undefined,
                users: users.length ? users : undefined
            }
            return putLdapRole(name.trim(), body)
        },
        onSuccess: async () => {
            toast.success(t('ldap.toast.saved'), t('ldap.toast.savedMsg', { name: name.trim() }))
            await onSaved(name.trim())
        },
        onError: (err) => {
            setServerErr(serverMessage(err) ?? '')
            toast.error(t('ldap.toast.saveFailed'), serverMessage(err))
        }
    })

    const submit = () => {
        setTouched(true)
        if (nameErr || polErr || save.isPending) return
        setServerErr('')
        save.mutate()
    }

    return (
        <Drawer
            busy={save.isPending}
            footer={
                <>
                    <Button disabled={save.isPending} onClick={onClose} variant="default">
                        {t('cancel')}
                    </Button>
                    <Button leftSection={<Icon name="check" size={14} />} loading={save.isPending} onClick={submit}>
                        {t('save')}
                    </Button>
                </>
            }
            onClose={onClose}
            opened={opened}
            title={edit ? t('ldap.drawer.edit', { name: edit.name }) : t('ldap.drawer.new')}
        >
            <form
                className={styles.form}
                noValidate
                onSubmit={(e) => {
                    e.preventDefault()
                    submit()
                }}
            >
                <p className={styles.hint}>{t('ldap.drawer.hint')}</p>
                <Field error={touched ? nameErr : ''} label={t('f.name')}>
                    <TextInput
                        aria-label={t('f.name')}
                        data-autofocus
                        disabled={!!edit}
                        onChange={(e) => setName(e.currentTarget.value)}
                        placeholder="platform-admins"
                        value={name}
                    />
                </Field>
                <PolicyField error={touched ? polErr : ''} onChange={setPolicies} value={policies} />
                <NamespaceField onChange={setNs} value={ns} />
                <Field hint={memWarn || t('ldap.f.usersHint')} label="users">
                    <TagInput ariaLabel="users" onChange={setUsers} placeholder="alice" value={users} />
                </Field>
                <Field hint={t('ldap.f.groupsHint')} label="groups">
                    <TagInput
                        ariaLabel="groups"
                        comma={false}
                        onChange={setGroups}
                        placeholder="cn=admins,ou=groups,dc=example,dc=com"
                        value={groups}
                    />
                </Field>
                <TtlField
                    description={t('f.ttlDefault')}
                    label="ttl"
                    max={TTL_MAX}
                    onChange={setTtl}
                    value={ttl}
                />
                {serverErr && (
                    <div className={styles.alert} role="alert">
                        <Icon name="alert" size={18} />
                        <div>
                            <b>{t('ldap.toast.saveFailed')}</b>
                            {serverErr}
                        </div>
                    </div>
                )}
            </form>
        </Drawer>
    )
}
