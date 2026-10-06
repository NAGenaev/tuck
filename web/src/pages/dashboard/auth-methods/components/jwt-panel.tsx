// SPDX-License-Identifier: AGPL-3.0-only
import { Button, TextInput } from '@mantine/core'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import {
    ConfirmModal,
    cx,
    Drawer,
    Icon,
    KeyValueEditor,
    KvPair,
    StateBlock,
    toast,
    TtlField,
    validateKv
} from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import {
    deleteJwtRole,
    getJwtRole,
    JwtRoleBody,
    JwtRoleFull,
    putJwtConfig,
    putJwtRole,
    retryUnlessDenied,
    serverMessage,
    status
} from '../api'
import styles from '../auth-methods.module.css'
import { fmtTtl, NAME_RE, nsToSeconds, toDur } from '../lib'
import { jwtConfigQ, jwtRolesQ, qk } from '../queries'
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

function httpsError(v: string, t: (k: string) => string): string {
    if (!v.trim()) return t('jwt.f.jwksNone')
    try {
        const u = new URL(v.trim())
        return u.protocol === 'https:' ? '' : t('jwt.f.jwksHttps')
    } catch {
        return t('jwt.f.jwksBad')
    }
}

export function JwtPanel() {
    const { t } = useTranslation('auth')
    const qc = useQueryClient()

    const cfg = useQuery(jwtConfigQ())
    const roles = useQuery(jwtRolesQ())
    const names = useMemo(() => [...(roles.data ?? [])].sort((a, b) => a.localeCompare(b)), [roles.data])
    const details = useQueries({
        queries: names.map((n) => ({ queryKey: qk.jwtRole(n), queryFn: () => getJwtRole(n), retry: retryUnlessDenied }))
    })
    const byName = useMemo(() => {
        const m = new Map<string, JwtRoleFull>()
        details.forEach((d, i) => d.data && m.set(names[i], d.data))
        return m
    }, [details, names])

    const notConfigured = cfg.isError && status(cfg.error) === 404

    // ----- config form -----
    const [jwks, setJwks] = useState('')
    const [issuer, setIssuer] = useState('')
    const [audience, setAudience] = useState('')
    const [defTtl, setDefTtl] = useState(0)
    const [cfgTouched, setCfgTouched] = useState(false)
    useEffect(() => {
        if (!cfg.data) return
        setJwks(cfg.data.jwks_uri ?? '')
        setIssuer(cfg.data.issuer ?? '')
        setAudience(cfg.data.audience ?? '')
        setDefTtl(nsToSeconds(cfg.data.default_ttl))
    }, [cfg.data])

    const jwksErr = httpsError(jwks, t)
    const dirty =
        !cfg.data ||
        jwks.trim() !== (cfg.data.jwks_uri ?? '') ||
        issuer.trim() !== (cfg.data.issuer ?? '') ||
        audience.trim() !== (cfg.data.audience ?? '') ||
        defTtl !== nsToSeconds(cfg.data.default_ttl)

    const saveCfg = useMutation({
        mutationFn: () =>
            putJwtConfig({
                audience: audience.trim() || undefined,
                default_ttl: toDur(defTtl),
                issuer: issuer.trim() || undefined,
                jwks_uri: jwks.trim()
            }),
        onSuccess: async () => {
            toast.success(t('jwt.toast.cfgSaved'), t('jwt.toast.cfgSavedMsg'))
            await qc.invalidateQueries({ queryKey: qk.jwtConfig })
        },
        onError: (err) => toast.error(t('jwt.toast.cfgFailed'), serverMessage(err))
    })

    // ----- roles -----
    const [editing, setEditing] = useState<'new' | JwtRoleFull | null>(null)
    const [toDelete, setToDelete] = useState<null | string>(null)
    const [fresh, setFresh] = useState<null | string>(null)
    useEffect(() => {
        if (!fresh) return
        const id = window.setTimeout(() => setFresh(null), 2200)
        return () => window.clearTimeout(id)
    }, [fresh])

    const del = useMutation({ mutationFn: deleteJwtRole })
    const confirmDelete = async (name: string) => {
        try {
            await del.mutateAsync(name)
        } catch (err) {
            toast.error(t('jwt.toast.deleteFailed'), serverMessage(err))
            throw err
        }
        toast.success(t('jwt.toast.deleted'), t('jwt.toast.deletedMsg', { name }))
        await qc.invalidateQueries({ queryKey: qk.jwtRoles })
        qc.removeQueries({ queryKey: qk.jwtRole(name) })
    }

    if (cfg.isLoading) return <PaneSkeleton request="GET /v1/auth/jwt/config" />
    if (cfg.isError && status(cfg.error) === 403) {
        return (
            <ForbiddenPlate
                need="auth/jwt/*"
                onRetry={() => void cfg.refetch()}
                request="GET /v1/auth/jwt/config"
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

    return (
        <>
            <SectionHead
                actions={
                    <Pill dot tone={notConfigured ? 'r' : 'g'}>
                        {notConfigured ? t('notConfigured') : t('configured')}
                    </Pill>
                }
                eyebrow={t('jwt.eyebrow')}
                first
                text={<Trans components={rich} i18nKey="jwt.text" ns="auth" />}
                title={t('jwt.cfgTitle')}
            />

            {notConfigured && (
                <div className={cx(styles.alert, styles.info)} role="status">
                    <Icon name="info" size={18} />
                    <div>
                        <b>{t('jwt.noCfg.title')}</b>
                        {t('jwt.noCfg.text')}
                    </div>
                </div>
            )}

            <Plate>
                <form
                    className={cx(styles.plateIn, styles.form)}
                    noValidate
                    onSubmit={(e) => {
                        e.preventDefault()
                        setCfgTouched(true)
                        if (!jwksErr && !saveCfg.isPending) saveCfg.mutate()
                    }}
                >
                    <Field error={cfgTouched ? jwksErr : ''} hint={t('jwt.f.jwksHint')} label="jwks_uri">
                        <TextInput
                            aria-label="jwks_uri"
                            onChange={(e) => setJwks(e.currentTarget.value)}
                            placeholder="https://idp.example.com/.well-known/jwks.json"
                            value={jwks}
                        />
                    </Field>
                    <div className={styles.grid2}>
                        <Field hint={t('jwt.f.issuerHint')} label="issuer">
                            <TextInput
                                aria-label="issuer"
                                onChange={(e) => setIssuer(e.currentTarget.value)}
                                placeholder="https://idp.example.com/"
                                value={issuer}
                            />
                        </Field>
                        <Field hint={t('jwt.f.audienceHint')} label="audience">
                            <TextInput
                                aria-label="audience"
                                onChange={(e) => setAudience(e.currentTarget.value)}
                                placeholder="tuck"
                                value={audience}
                            />
                        </Field>
                    </div>
                    <TtlField
                        description={t('jwt.f.defaultTtlHint')}
                        label="default_ttl"
                        max={TTL_MAX}
                        onChange={setDefTtl}
                        value={defTtl}
                    />
                    <p className={styles.hint}>{t('jwt.replaceNote')}</p>
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
                                    setJwks(cfg.data.jwks_uri ?? '')
                                    setIssuer(cfg.data.issuer ?? '')
                                    setAudience(cfg.data.audience ?? '')
                                    setDefTtl(nsToSeconds(cfg.data.default_ttl))
                                    setCfgTouched(false)
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
                            {t('jwt.newRole')}
                        </Button>
                    }
                    eyebrow={t('roles')}
                    text={t('jwt.rolesText')}
                    title={t('jwt.rolesTitle')}
                />
                {roles.isLoading ? (
                    <PaneSkeleton request="GET /v1/auth/jwt/role/?list=true" />
                ) : roles.isError ? (
                    status(roles.error) === 403 ? (
                        <ForbiddenPlate
                            need="auth/jwt/role/*"
                            onRetry={() => void roles.refetch()}
                            request="GET /v1/auth/jwt/role/?list=true"
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
                                    <th>{t('jwt.cols.role')}</th>
                                    <th>{t('cols.policies')}</th>
                                    <th>{t('jwt.cols.bound')}</th>
                                    <th>TTL</th>
                                    <th>{t('cols.ns')}</th>
                                    <th />
                                </tr>
                            </thead>
                            <tbody>
                                {names.length === 0 && (
                                    <tr>
                                        <td className={styles.empty} colSpan={6}>
                                            {t('jwt.empty')}
                                        </td>
                                    </tr>
                                )}
                                {names.map((n) => {
                                    const r = byName.get(n)
                                    const claims = Object.keys(r?.bound_claims ?? {}).length
                                    return (
                                        <tr className={cx(fresh === n && styles.fresh)} key={n}>
                                            <td className={styles.k}>{n}</td>
                                            <td>{r ? <PolicyChips list={r.policies} /> : '…'}</td>
                                            <td>
                                                {r ? (
                                                    <div className={styles.chips}>
                                                        {r.bound_subject && (
                                                            <span className={cx(styles.chip, styles.ice)}>
                                                                sub={r.bound_subject}
                                                            </span>
                                                        )}
                                                        {(r.bound_audiences ?? []).map((a) => (
                                                            <span className={cx(styles.chip, styles.ice)} key={a}>
                                                                aud={a}
                                                            </span>
                                                        ))}
                                                        {claims > 0 && (
                                                            <span className={cx(styles.chip, styles.ice)}>
                                                                {t('jwt.claims', { count: claims })}
                                                            </span>
                                                        )}
                                                        {!r.bound_subject && !(r.bound_audiences ?? []).length && !claims && (
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
                                                        aria-label={t('jwt.edit', { name: n })}
                                                        className={styles.ibtn}
                                                        disabled={!r}
                                                        onClick={() => r && setEditing(r)}
                                                        title={t('jwt.edit', { name: n })}
                                                        type="button"
                                                    >
                                                        <Icon name="gear" size={14} />
                                                    </button>
                                                    <button
                                                        aria-label={t('jwt.remove', { name: n })}
                                                        className={cx(styles.ibtn, styles.red)}
                                                        onClick={() => setToDelete(n)}
                                                        title={t('jwt.remove', { name: n })}
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
                    await qc.invalidateQueries({ queryKey: qk.jwtRoles })
                    await qc.invalidateQueries({ queryKey: qk.jwtRole(name) })
                }}
                target={editing}
            />

            <ConfirmModal
                confirmLabel={t('delete')}
                confirmText={toDelete ?? undefined}
                destructive
                message={<Trans components={rich} i18nKey="jwt.delete.msg" ns="auth" values={{ name: toDelete }} />}
                onClose={() => setToDelete(null)}
                onConfirm={async () => {
                    if (toDelete) await confirmDelete(toDelete)
                }}
                opened={toDelete !== null}
                title={t('jwt.delete.title')}
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
    target: 'new' | JwtRoleFull | null
}) {
    const { t } = useTranslation('auth')
    const opened = target !== null
    const edit = target && target !== 'new' ? target : null

    const [name, setName] = useState('')
    const [policies, setPolicies] = useState<string[]>([])
    const [ns, setNs] = useState('')
    const [ttl, setTtl] = useState(0)
    const [auds, setAuds] = useState<string[]>([])
    const [sub, setSub] = useState('')
    const [claims, setClaims] = useState<KvPair[]>([])
    const [touched, setTouched] = useState(false)
    const [serverErr, setServerErr] = useState('')

    useEffect(() => {
        if (!opened) return
        setName(edit?.name ?? '')
        setPolicies(edit?.policies ?? [])
        setNs(edit?.tuck_namespace ?? '')
        setTtl(nsToSeconds(edit?.ttl))
        setAuds(edit?.bound_audiences ?? [])
        setSub(edit?.bound_subject ?? '')
        setClaims(Object.entries(edit?.bound_claims ?? {}).map(([key, value]) => ({ key, value })))
        setTouched(false)
        setServerErr('')
    }, [opened, edit])

    const nameErr = !NAME_RE.test(name.trim())
        ? t('f.nameBad')
        : !edit && existing.includes(name.trim())
          ? t('f.nameTaken')
          : ''
    const polErr = policies.length === 0 ? t('f.policiesNone') : ''
    const kv = validateKv(claims, /^\S+$/)

    const save = useMutation({
        mutationFn: () => {
            const body: JwtRoleBody = {
                bound_audiences: auds.length ? auds : undefined,
                bound_claims: claims.some((c) => c.key.trim())
                    ? Object.fromEntries(claims.filter((c) => c.key.trim()).map((c) => [c.key.trim(), c.value]))
                    : undefined,
                bound_subject: sub.trim() || undefined,
                policies,
                ttl: toDur(ttl),
                tuck_namespace: ns || undefined
            }
            return putJwtRole(name.trim(), body)
        },
        onSuccess: async () => {
            toast.success(t('jwt.toast.saved'), t('jwt.toast.savedMsg', { name: name.trim() }))
            await onSaved(name.trim())
        },
        onError: (err) => {
            setServerErr(serverMessage(err) ?? '')
            toast.error(t('jwt.toast.saveFailed'), serverMessage(err))
        }
    })

    const submit = () => {
        setTouched(true)
        if (nameErr || polErr || !kv.valid || save.isPending) return
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
            title={edit ? t('jwt.drawer.edit', { name: edit.name }) : t('jwt.drawer.new')}
        >
            <form
                className={styles.form}
                noValidate
                onSubmit={(e) => {
                    e.preventDefault()
                    submit()
                }}
            >
                <p className={styles.hint}>{t('jwt.drawer.hint')}</p>
                <Field error={touched ? nameErr : ''} label={t('f.name')}>
                    <TextInput
                        aria-label={t('f.name')}
                        data-autofocus
                        disabled={!!edit}
                        onChange={(e) => setName(e.currentTarget.value)}
                        placeholder="ci-pipeline"
                        value={name}
                    />
                </Field>
                <PolicyField error={touched ? polErr : ''} onChange={setPolicies} value={policies} />
                <NamespaceField onChange={setNs} value={ns} />
                <Field hint={t('jwt.f.audsHint')} label="bound_audiences">
                    <TagInput ariaLabel="bound_audiences" comma={false} onChange={setAuds} value={auds} />
                </Field>
                <Field hint={t('jwt.f.subHint')} label="bound_subject">
                    <TextInput
                        aria-label="bound_subject"
                        onChange={(e) => setSub(e.currentTarget.value)}
                        value={sub}
                    />
                </Field>
                <KeyValueEditor
                    description={t('jwt.f.claimsHint')}
                    keyPattern={/^\S+$/}
                    label="bound_claims"
                    onChange={setClaims}
                    value={claims}
                />
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
                            <b>{t('jwt.toast.saveFailed')}</b>
                            {serverErr}
                        </div>
                    </div>
                )}
            </form>
        </Drawer>
    )
}
