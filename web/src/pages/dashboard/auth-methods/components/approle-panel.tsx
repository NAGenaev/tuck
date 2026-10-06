// SPDX-License-Identifier: AGPL-3.0-only
import { Button, NumberInput, Switch, TextInput } from '@mantine/core'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import {
    ConfirmModal,
    CopyButton,
    Countdown,
    cx,
    Drawer,
    Icon,
    MetaGrid,
    StateBlock,
    toast,
    TtlField
} from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import {
    AppRoleBody,
    AppRoleFull,
    deleteAppRole,
    generateSecretId,
    getAppRole,
    putAppRole,
    retryUnlessDenied,
    SecretIdResp,
    serverMessage,
    status
} from '../api'
import styles from '../auth-methods.module.css'
import { fmtTtl, NAME_RE, nsToSeconds, toDur } from '../lib'
import { approleListQ, qk } from '../queries'
import { ForbiddenPlate, NamespaceField, PaneSkeleton, PolicyChips, PolicyField, rich, SectionHead } from './shared'

const REVEAL_MS = 60_000
const TTL_MAX = 31_536_000

interface Issued {
    at: number
    gone: boolean
    resp: SecretIdResp
    role: string
    shown: boolean
}

export function ApprolePanel() {
    const { t } = useTranslation('auth')
    const qc = useQueryClient()

    const list = useQuery(approleListQ())
    const names = useMemo(() => [...(list.data ?? [])].sort((a, b) => a.localeCompare(b)), [list.data])
    const details = useQueries({
        queries: names.map((n) => ({
            queryKey: qk.approleRole(n),
            queryFn: () => getAppRole(n),
            retry: retryUnlessDenied
        }))
    })
    const byName = useMemo(() => {
        const m = new Map<string, AppRoleFull>()
        details.forEach((d, i) => d.data && m.set(names[i], d.data))
        return m
    }, [details, names])

    const [sel, setSel] = useState<null | string>(null)
    const [editing, setEditing] = useState<'new' | AppRoleFull | null>(null)
    const [toDelete, setToDelete] = useState<null | string>(null)
    const [fresh, setFresh] = useState<null | string>(null)
    const [issued, setIssued] = useState<Issued | null>(null)
    const [history, setHistory] = useState<Array<{ at: number; resp: SecretIdResp; role: string }>>([])
    const timer = useRef<number>(0)

    useEffect(() => () => window.clearTimeout(timer.current), [])
    useEffect(() => {
        if (!fresh) return
        const id = window.setTimeout(() => setFresh(null), 2200)
        return () => window.clearTimeout(id)
    }, [fresh])

    const role = sel ? byName.get(sel) : undefined

    const gen = useMutation({
        mutationFn: (name: string) => generateSecretId(name),
        onSuccess: (resp, name) => {
            window.clearTimeout(timer.current)
            const at = Date.now()
            setIssued({ at, gone: false, resp, role: name, shown: false })
            setHistory((h) => [{ at, resp: { ...resp, id: '' }, role: name }, ...h].slice(0, 6))
            timer.current = window.setTimeout(() => setIssued((i) => (i ? { ...i, gone: true } : i)), REVEAL_MS)
            toast.success(t('approle.toast.issued'), t('approle.toast.issuedMsg', { name }))
        },
        onError: (err) => toast.error(t('approle.toast.issueFailed'), serverMessage(err))
    })

    const del = useMutation({ mutationFn: deleteAppRole })
    const confirmDelete = async (name: string) => {
        try {
            await del.mutateAsync(name)
        } catch (err) {
            toast.error(t('approle.toast.deleteFailed'), serverMessage(err))
            throw err
        }
        toast.success(t('approle.toast.deleted'), t('approle.toast.deletedMsg', { name }))
        if (sel === name) setSel(null)
        if (issued?.role === name) setIssued(null)
        await qc.invalidateQueries({ queryKey: qk.approleList })
        qc.removeQueries({ queryKey: qk.approleRole(name) })
    }

    if (list.isLoading) return <PaneSkeleton request="GET /v1/auth/approle/role/?list=true" />
    if (list.isError && status(list.error) === 403) {
        return (
            <ForbiddenPlate
                need="auth/approle/role/*"
                onRetry={() => void list.refetch()}
                request="GET /v1/auth/approle/role/?list=true"
                retrying={list.isFetching}
            />
        )
    }
    if (list.isError) {
        return (
            <StateBlock
                details={serverMessage(list.error)}
                kind="error"
                message={t('error.message')}
                onRetry={() => void list.refetch()}
                retrying={list.isFetching}
                title={t('error.title')}
            />
        )
    }

    const secret = issued && issued.role === sel ? issued : null

    return (
        <>
            <SectionHead
                actions={
                    <Button leftSection={<Icon name="plus" size={14} />} onClick={() => setEditing('new')}>
                        {t('approle.new')}
                    </Button>
                }
                eyebrow={t('approle.eyebrow')}
                first
                text={<Trans components={rich} i18nKey="approle.text" ns="auth" />}
                title={t('approle.title')}
            />

            <div className={styles.tbl}>
                <table>
                    <thead>
                        <tr>
                            <th>{t('approle.cols.role')}</th>
                            <th>{t('approle.cols.policies')}</th>
                            <th>{t('approle.cols.tokenTtl')}</th>
                            <th>{t('approle.cols.secretId')}</th>
                            <th>{t('approle.cols.ns')}</th>
                            <th />
                        </tr>
                    </thead>
                    <tbody>
                        {names.length === 0 && (
                            <tr>
                                <td className={styles.empty} colSpan={6}>
                                    {t('approle.empty')}
                                </td>
                            </tr>
                        )}
                        {names.map((n) => {
                            const r = byName.get(n)
                            const tt = nsToSeconds(r?.token_ttl)
                            const st = nsToSeconds(r?.secret_id_ttl)
                            return (
                                <tr
                                    className={cx(styles.click, sel === n && styles.sel, fresh === n && styles.fresh)}
                                    key={n}
                                    onClick={() => setSel(sel === n ? null : n)}
                                >
                                    <td className={styles.k}>{n}</td>
                                    <td>{r ? <PolicyChips list={r.policies} /> : '…'}</td>
                                    <td>{r ? fmtTtl(tt) || t('default') : '…'}</td>
                                    <td>
                                        {r
                                            ? `${fmtTtl(st) || t('default')} · ${
                                                  r.secret_id_num_uses ? `${r.secret_id_num_uses}×` : '∞'
                                              }`
                                            : '…'}
                                    </td>
                                    <td>{r?.tuck_namespace || <span className={styles.dim}>root</span>}</td>
                                    <td>
                                        <div className={styles.acts}>
                                            <button
                                                aria-label={t('approle.open', { name: n })}
                                                className={styles.ibtn}
                                                onClick={(e) => {
                                                    e.stopPropagation()
                                                    setSel(n)
                                                }}
                                                title={t('approle.open', { name: n })}
                                                type="button"
                                            >
                                                <Icon name="key" size={14} />
                                            </button>
                                            <button
                                                aria-label={t('approle.edit', { name: n })}
                                                className={styles.ibtn}
                                                disabled={!r}
                                                onClick={(e) => {
                                                    e.stopPropagation()
                                                    if (r) setEditing(r)
                                                }}
                                                title={t('approle.edit', { name: n })}
                                                type="button"
                                            >
                                                <Icon name="gear" size={14} />
                                            </button>
                                            <button
                                                aria-label={t('approle.remove', { name: n })}
                                                className={cx(styles.ibtn, styles.red)}
                                                onClick={(e) => {
                                                    e.stopPropagation()
                                                    setToDelete(n)
                                                }}
                                                title={t('approle.remove', { name: n })}
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

            {sel && (
                <div className={styles.sec}>
                    <Plate>
                        <div className={styles.plateIn}>
                            <div className={styles.sechead} style={{ marginBottom: 12 }}>
                                <div>
                                    <div className={styles.cap}>{t('approle.detail')}</div>
                                    <h3 className={styles.detailTitle}>{sel}</h3>
                                </div>
                                <div className={styles.detailActs}>
                                    <Button
                                        leftSection={<Icon name="key" size={14} />}
                                        loading={gen.isPending}
                                        onClick={() => gen.mutate(sel)}
                                    >
                                        {t('approle.generate')}
                                    </Button>
                                    <Button
                                        disabled={!role}
                                        leftSection={<Icon name="gear" size={14} />}
                                        onClick={() => role && setEditing(role)}
                                        variant="default"
                                    >
                                        {t('approle.editBtn')}
                                    </Button>
                                    <Button onClick={() => setSel(null)} variant="subtle">
                                        {t('close')}
                                    </Button>
                                </div>
                            </div>

                            <div className={styles.two}>
                                <div className={styles.form}>
                                    <div className={styles.fld}>
                                        <div className={styles.lbl}>
                                            <span>role_id</span>
                                            <i>{t('approle.roleIdNote')}</i>
                                        </div>
                                        <div className={styles.copyf}>
                                            <code>{role?.role_id ?? '…'}</code>
                                            {role && <CopyButton label={t('approle.copyRoleId')} value={role.role_id} />}
                                        </div>
                                    </div>
                                    <div className={styles.kv}>
                                        <div>
                                            <span>{t('approle.cols.policies')}</span>
                                            <PolicyChips list={role?.policies} />
                                        </div>
                                        <div>
                                            <span>{t('approle.cols.tokenTtl')}</span>
                                            {role ? fmtTtl(nsToSeconds(role.token_ttl)) || t('default') : '…'}
                                        </div>
                                        <div>
                                            <span>{t('approle.cols.secretId')}</span>
                                            {role
                                                ? `${fmtTtl(nsToSeconds(role.secret_id_ttl)) || t('default')} · ${
                                                      role.secret_id_num_uses
                                                          ? t('approle.uses', { count: role.secret_id_num_uses })
                                                          : t('approle.unlimited')
                                                  }`
                                                : '…'}
                                        </div>
                                        <div>
                                            <span>{t('approle.cols.ns')}</span>
                                            {role?.tuck_namespace || 'root'}
                                        </div>
                                    </div>
                                </div>

                                <div className={styles.form}>
                                    {secret ? (
                                        <div className={styles.reveal} key={secret.at}>
                                            <div className={styles.once}>
                                                <Icon name="warn" size={14} />
                                                {t('approle.once')}
                                                {!secret.gone && (
                                                    <Countdown
                                                        showBar={false}
                                                        total={REVEAL_MS}
                                                        until={secret.at + REVEAL_MS}
                                                    />
                                                )}
                                            </div>
                                            <div
                                                className={cx(
                                                    styles.secid,
                                                    secret.gone && styles.gone,
                                                    !secret.gone && !secret.shown && styles.masked
                                                )}
                                            >
                                                {secret.gone ? t('approle.gone') : secret.resp.id}
                                            </div>
                                            {!secret.gone && (
                                                <div className={styles.revRow}>
                                                    <Button
                                                        leftSection={<Icon name={secret.shown ? 'eyeoff' : 'eye'} size={14} />}
                                                        onClick={() => setIssued({ ...secret, shown: !secret.shown })}
                                                        size="xs"
                                                        variant="default"
                                                    >
                                                        {secret.shown ? t('approle.hide') : t('approle.show')}
                                                    </Button>
                                                    <CopyButton
                                                        label={t('approle.copySecret')}
                                                        size={30}
                                                        toast={false}
                                                        value={secret.resp.id}
                                                    />
                                                    <span className={cx(styles.dim, styles.push)}>
                                                        {secret.resp.uses_left > 0
                                                            ? t('approle.usesLeft', { count: secret.resp.uses_left })
                                                            : t('approle.unlimited')}
                                                        {secret.resp.expires_at &&
                                                            ` · ${t('approle.expires')} ${new Date(
                                                                secret.resp.expires_at
                                                            ).toLocaleString()}`}
                                                    </span>
                                                </div>
                                            )}
                                        </div>
                                    ) : (
                                        <p className={styles.hint}>{t('approle.noSecret')}</p>
                                    )}
                                    {history.filter((h) => h.role === sel).length > 0 && (
                                        <div>
                                            <div className={styles.cap} style={{ marginBottom: 8 }}>
                                                {t('approle.history')}
                                            </div>
                                            <ul className={styles.hist}>
                                                {history
                                                    .filter((h) => h.role === sel)
                                                    .map((h) => (
                                                        <li key={h.at}>
                                                            <span>{new Date(h.at).toLocaleTimeString()}</span>
                                                            <span>
                                                                {h.resp.num_uses
                                                                    ? t('approle.uses', { count: h.resp.num_uses })
                                                                    : t('approle.unlimited')}
                                                            </span>
                                                        </li>
                                                    ))}
                                            </ul>
                                        </div>
                                    )}
                                </div>
                            </div>
                        </div>
                    </Plate>
                </div>
            )}

            <RoleDrawer
                existing={names}
                onClose={() => setEditing(null)}
                onSaved={async (name) => {
                    setEditing(null)
                    setFresh(name)
                    await qc.invalidateQueries({ queryKey: qk.approleList })
                    await qc.invalidateQueries({ queryKey: qk.approleRole(name) })
                }}
                target={editing}
            />

            <ConfirmModal
                confirmLabel={t('approle.delete.confirm')}
                confirmText={toDelete ?? undefined}
                destructive
                message={<Trans components={rich} i18nKey="approle.delete.msg" ns="auth" values={{ name: toDelete }} />}
                onClose={() => setToDelete(null)}
                onConfirm={async () => {
                    if (toDelete) await confirmDelete(toDelete)
                }}
                opened={toDelete !== null}
                title={t('approle.delete.title')}
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
    target: 'new' | AppRoleFull | null
}) {
    const { t } = useTranslation('auth')
    const opened = target !== null
    const edit = target && target !== 'new' ? target : null

    const [name, setName] = useState('')
    const [policies, setPolicies] = useState<string[]>([])
    const [ns, setNs] = useState('')
    const [tokenTtl, setTokenTtl] = useState(0)
    const [sidTtl, setSidTtl] = useState(0)
    const [uses, setUses] = useState<number | string>(0)
    const [regen, setRegen] = useState(false)
    const [touched, setTouched] = useState(false)
    const [serverErr, setServerErr] = useState('')

    useEffect(() => {
        if (!opened) return
        setName(edit?.name ?? '')
        setPolicies(edit?.policies ?? [])
        setNs(edit?.tuck_namespace ?? '')
        setTokenTtl(nsToSeconds(edit?.token_ttl))
        setSidTtl(nsToSeconds(edit?.secret_id_ttl))
        setUses(edit?.secret_id_num_uses ?? 0)
        setRegen(false)
        setTouched(false)
        setServerErr('')
    }, [opened, edit])

    const nameErr = !NAME_RE.test(name.trim())
        ? t('f.nameBad')
        : !edit && existing.includes(name.trim())
          ? t('f.nameTaken')
          : ''
    const polErr = policies.length === 0 ? t('f.policiesNone') : ''

    const save = useMutation({
        mutationFn: () => {
            const body: AppRoleBody = {
                policies,
                secret_id_num_uses: Number(uses) || 0,
                secret_id_ttl: toDur(sidTtl),
                token_ttl: toDur(tokenTtl),
                tuck_namespace: ns || undefined
            }
            // a PUT without role_id mints a new one, so an edit must send the current id back
            if (edit && !regen) body.role_id = edit.role_id
            return putAppRole(name.trim(), body)
        },
        onSuccess: async () => {
            toast.success(t('approle.toast.saved'), t('approle.toast.savedMsg', { name: name.trim() }))
            await onSaved(name.trim())
        },
        onError: (err) => {
            setServerErr(serverMessage(err) ?? '')
            toast.error(t('approle.toast.saveFailed'), serverMessage(err))
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
            title={edit ? t('approle.drawer.edit', { name: edit.name }) : t('approle.drawer.new')}
        >
            <form
                className={styles.form}
                noValidate
                onSubmit={(e) => {
                    e.preventDefault()
                    submit()
                }}
            >
                <p className={styles.hint}>{t('approle.drawer.hint')}</p>
                <div className={cx(styles.fld, touched && nameErr && styles.err)}>
                    <div className={styles.lbl}>
                        <span>{t('f.name')}</span>
                    </div>
                    <TextInput
                        aria-label={t('f.name')}
                        data-autofocus
                        disabled={!!edit}
                        onChange={(e) => setName(e.currentTarget.value)}
                        placeholder="ci-deployer"
                        value={name}
                    />
                    {touched && nameErr && <span className={styles.msg}>{nameErr}</span>}
                </div>
                <PolicyField error={touched ? polErr : ''} onChange={setPolicies} value={policies} />
                <NamespaceField onChange={setNs} value={ns} />
                <TtlField
                    description={t('f.ttlDefault')}
                    label={t('approle.f.tokenTtl')}
                    max={TTL_MAX}
                    onChange={setTokenTtl}
                    value={tokenTtl}
                />
                <TtlField
                    description={t('f.ttlDefault')}
                    label={t('approle.f.secretIdTtl')}
                    max={TTL_MAX}
                    onChange={setSidTtl}
                    value={sidTtl}
                />
                <NumberInput
                    description={t('approle.f.usesHint')}
                    label={t('approle.f.uses')}
                    min={0}
                    onChange={setUses}
                    value={uses}
                />
                {edit && (
                    <Switch
                        checked={regen}
                        description={t('approle.f.regenHint')}
                        label={t('approle.f.regen')}
                        onChange={(e) => setRegen(e.currentTarget.checked)}
                    />
                )}
                {edit && (
                    <MetaGrid items={[['role_id', <code key="r">{edit.role_id}</code>]]} />
                )}
                {serverErr && (
                    <div className={styles.alert} role="alert">
                        <Icon name="alert" size={18} />
                        <div>
                            <b>{t('approle.toast.saveFailed')}</b>
                            {serverErr}
                        </div>
                    </div>
                )}
                {!edit && (
                    <Pill tone="n">PUT /v1/auth/approle/role/{name.trim() || '{name}'}</Pill>
                )}
            </form>
        </Drawer>
    )
}
