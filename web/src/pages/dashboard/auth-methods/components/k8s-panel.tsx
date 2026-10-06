// SPDX-License-Identifier: AGPL-3.0-only
import { Button, TextInput } from '@mantine/core'
import { useMutation, useQuery } from '@tanstack/react-query'
import { FormEvent, useEffect, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { ConfirmModal, cx, Drawer, Icon, StateBlock, toast, TtlField } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import {
    deleteK8sRole,
    getK8sRole,
    K8sRoleFull,
    k8sRolePath,
    putK8sRole,
    serverMessage,
    status
} from '../api'
import styles from '../auth-methods.module.css'
import { fmtTtl, K8S_PART_RE, parseGoDuration, toDur } from '../lib'
import { k8sClustersQ } from '../queries'
import { K8sFlow } from './k8s-flow'
import { Field, ForbiddenPlate, NamespaceField, PaneSkeleton, PolicyChips, PolicyField, rich, SectionHead } from './shared'

const TTL_MAX = 31_536_000

interface Entry {
    cluster: string
    ns: string
    role: K8sRoleFull
    sa: string
}

const keyOf = (c: string, ns: string, sa: string) => `${c}|${ns}|${sa}`
const clusterLabel = (c: string, def: string) => c || def

export function K8sPanel() {
    const { t } = useTranslation('auth')
    const clusters = useQuery(k8sClustersQ())
    const def = t('k8s.defaultCluster')
    const all = ['', ...(clusters.data ?? [])]

    const [cluster, setCluster] = useState('')
    const [ns, setNs] = useState('')
    const [sa, setSa] = useState('')
    const [msg, setMsg] = useState<{ kind: 'bad' | 'ok'; text: string } | null>(null)
    const [entries, setEntries] = useState<Entry[]>([])
    const [fresh, setFresh] = useState<null | string>(null)
    const [editing, setEditing] = useState<{ cluster: string; ns: string; role?: K8sRoleFull; sa: string } | null>(null)
    const [toDelete, setToDelete] = useState<Entry | null>(null)

    useEffect(() => {
        if (!fresh) return
        const id = window.setTimeout(() => setFresh(null), 2200)
        return () => window.clearTimeout(id)
    }, [fresh])

    const partErr = (v: string) => (v && !K8S_PART_RE.test(v) ? t('k8s.partBad') : '')
    const canLookup = !!ns && !!sa && !partErr(ns) && !partErr(sa)

    const addEntry = (e: Entry) => {
        setEntries((list) => [e, ...list.filter((x) => keyOf(x.cluster, x.ns, x.sa) !== keyOf(e.cluster, e.ns, e.sa))])
        setFresh(keyOf(e.cluster, e.ns, e.sa))
    }

    const lookup = useMutation({
        mutationFn: () => getK8sRole(cluster, ns.trim(), sa.trim()),
        onSuccess: (role) => {
            setMsg({ kind: 'ok', text: t('k8s.found', { ns: ns.trim(), sa: sa.trim() }) })
            addEntry({ cluster, ns: ns.trim(), role, sa: sa.trim() })
        },
        onError: (err) => {
            if (status(err) === 404) {
                setMsg({ kind: 'bad', text: t('k8s.notFound', { ns: ns.trim(), sa: sa.trim() }) })
            } else {
                setMsg({ kind: 'bad', text: serverMessage(err) ?? t('error.title') })
            }
        }
    })

    const submitLookup = (e: FormEvent) => {
        e.preventDefault()
        if (!canLookup || lookup.isPending) return
        setMsg(null)
        lookup.mutate()
    }

    const del = useMutation({ mutationFn: (e: Entry) => deleteK8sRole(e.cluster, e.ns, e.sa) })
    const confirmDelete = async (e: Entry) => {
        try {
            await del.mutateAsync(e)
        } catch (err) {
            toast.error(t('k8s.toast.deleteFailed'), serverMessage(err))
            throw err
        }
        toast.success(t('k8s.toast.deleted'), t('k8s.toast.deletedMsg', { name: `${e.ns}/${e.sa}` }))
        setEntries((l) => l.filter((x) => keyOf(x.cluster, x.ns, x.sa) !== keyOf(e.cluster, e.ns, e.sa)))
    }

    if (clusters.isLoading) return <PaneSkeleton request="GET /v1/auth/kubernetes/clusters" />
    if (clusters.isError && status(clusters.error) === 403) {
        return (
            <ForbiddenPlate
                need="auth/kubernetes/*"
                onRetry={() => void clusters.refetch()}
                request="GET /v1/auth/kubernetes/clusters"
                retrying={clusters.isFetching}
            />
        )
    }
    if (clusters.isError) {
        return (
            <StateBlock
                details={serverMessage(clusters.error)}
                kind="error"
                message={t('error.message')}
                onRetry={() => void clusters.refetch()}
                retrying={clusters.isFetching}
                title={t('error.title')}
            />
        )
    }

    return (
        <>
            <SectionHead
                eyebrow={t('k8s.eyebrow')}
                first
                text={<Trans components={rich} i18nKey="k8s.text" ns="auth" />}
                title={t('k8s.flowTitle')}
            />
            <K8sFlow defaultCluster={def} />

            <div className={styles.sec}>
                <SectionHead
                    actions={<Pill tone="n">{all.length}</Pill>}
                    eyebrow={t('k8s.clustersEyebrow')}
                    text={<Trans components={rich} i18nKey="k8s.clustersText" ns="auth" />}
                    title={t('k8s.clustersTitle')}
                />
                <div className={styles.clGrid}>
                    {all.map((c) => (
                        <button
                            aria-pressed={cluster === c}
                            className={cx(styles.cl, cluster === c && styles.sel)}
                            key={c || '__default'}
                            onClick={() => setCluster(c)}
                            type="button"
                        >
                            <div className={styles.clTop}>
                                <h4>{clusterLabel(c, def)}</h4>
                                {!c && (
                                    <Pill dot tone="b">
                                        {t('k8s.default')}
                                    </Pill>
                                )}
                                {cluster === c && <Pill tone="g">{t('k8s.selected')}</Pill>}
                            </div>
                            <div className={styles.clApi}>{k8sRolePath(c, '{ns}', '{sa}').replace('/role/', '/role/')}</div>
                            <div className={styles.clMeta}>
                                {t('k8s.rolesThisSession')}
                                <b>{entries.filter((e) => e.cluster === c).length}</b>
                            </div>
                        </button>
                    ))}
                </div>
                <div className={cx(styles.alert, styles.info)} role="note" style={{ marginTop: 16, marginBottom: 0 }}>
                    <Icon name="info" size={18} />
                    <div>
                        <b>{t('k8s.addCluster.title')}</b>
                        {t('k8s.addCluster.text')}
                    </div>
                </div>
            </div>

            <div className={styles.sec}>
                <SectionHead
                    actions={
                        <Button
                            leftSection={<Icon name="plus" size={14} />}
                            onClick={() => setEditing({ cluster, ns: ns.trim(), sa: sa.trim() })}
                            variant="default"
                        >
                            {t('k8s.newRole')}
                        </Button>
                    }
                    eyebrow={t('roles')}
                    text={t('k8s.rolesText')}
                    title={t('k8s.rolesTitle')}
                />
                <form className={styles.lookupBar} noValidate onSubmit={submitLookup}>
                    <div className={styles.lookup}>
                        <Field label={t('k8s.cluster')}>
                            <TextInput aria-label={t('k8s.cluster')} readOnly value={clusterLabel(cluster, def)} />
                        </Field>
                        <Field error={partErr(ns)} label={t('k8s.namespace')}>
                            <TextInput
                                aria-label={t('k8s.namespace')}
                                onChange={(e) => setNs(e.currentTarget.value.trim())}
                                placeholder="payments"
                                value={ns}
                            />
                        </Field>
                        <Field error={partErr(sa)} label={t('k8s.serviceAccount')}>
                            <TextInput
                                aria-label={t('k8s.serviceAccount')}
                                onChange={(e) => setSa(e.currentTarget.value.trim())}
                                placeholder="api"
                                value={sa}
                            />
                        </Field>
                        <Button
                            disabled={!canLookup}
                            leftSection={<Icon name="search" size={14} />}
                            loading={lookup.isPending}
                            type="submit"
                        >
                            {t('k8s.lookup')}
                        </Button>
                    </div>
                    {msg && (
                        <p className={cx(styles.lookupMsg, msg.kind === 'bad' && styles.bad)} role="status">
                            {msg.text}
                            {msg.kind === 'bad' && status(lookup.error) === 404 && (
                                <Button
                                    onClick={() => setEditing({ cluster, ns: ns.trim(), sa: sa.trim() })}
                                    size="xs"
                                    variant="default"
                                >
                                    {t('k8s.createThis')}
                                </Button>
                            )}
                        </p>
                    )}
                    <p className={styles.hint} style={{ marginTop: 10 }}>
                        {t('k8s.noList')}
                    </p>
                </form>

                <div className={styles.tbl}>
                    <table>
                        <thead>
                            <tr>
                                <th>{t('k8s.cols.role')}</th>
                                <th>{t('k8s.cols.cluster')}</th>
                                <th>{t('cols.policies')}</th>
                                <th>TTL</th>
                                <th>{t('cols.ns')}</th>
                                <th />
                            </tr>
                        </thead>
                        <tbody>
                            {entries.length === 0 && (
                                <tr>
                                    <td className={styles.empty} colSpan={6}>
                                        {t('k8s.empty')}
                                    </td>
                                </tr>
                            )}
                            {entries.map((e) => {
                                const k = keyOf(e.cluster, e.ns, e.sa)
                                return (
                                    <tr className={cx(fresh === k && styles.fresh)} key={k}>
                                        <td className={styles.k}>
                                            {e.ns}/{e.sa}
                                        </td>
                                        <td>{clusterLabel(e.cluster, def)}</td>
                                        <td>
                                            <PolicyChips list={e.role.policies} />
                                        </td>
                                        <td>
                                            {fmtTtl(parseGoDuration(e.role.ttl) ?? 0) || t('default')}
                                        </td>
                                        <td>{e.role.tuck_namespace || <span className={styles.dim}>root</span>}</td>
                                        <td>
                                            <div className={styles.acts}>
                                                <button
                                                    aria-label={t('k8s.edit', { name: `${e.ns}/${e.sa}` })}
                                                    className={styles.ibtn}
                                                    onClick={() =>
                                                        setEditing({ cluster: e.cluster, ns: e.ns, role: e.role, sa: e.sa })
                                                    }
                                                    title={t('k8s.edit', { name: `${e.ns}/${e.sa}` })}
                                                    type="button"
                                                >
                                                    <Icon name="gear" size={14} />
                                                </button>
                                                <button
                                                    aria-label={t('k8s.remove', { name: `${e.ns}/${e.sa}` })}
                                                    className={cx(styles.ibtn, styles.red)}
                                                    onClick={() => setToDelete(e)}
                                                    title={t('k8s.remove', { name: `${e.ns}/${e.sa}` })}
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
            </div>

            <RoleDrawer
                clusters={all}
                onClose={() => setEditing(null)}
                onSaved={(e) => {
                    setEditing(null)
                    addEntry(e)
                }}
                target={editing}
            />

            <ConfirmModal
                confirmLabel={t('delete')}
                confirmText={toDelete ? toDelete.sa : undefined}
                destructive
                message={
                    <Trans
                        components={rich}
                        i18nKey="k8s.delete.msg"
                        ns="auth"
                        values={{ cluster: toDelete ? clusterLabel(toDelete.cluster, def) : '', name: toDelete ? `${toDelete.ns}/${toDelete.sa}` : '' }}
                    />
                }
                onClose={() => setToDelete(null)}
                onConfirm={async () => {
                    if (toDelete) await confirmDelete(toDelete)
                }}
                opened={toDelete !== null}
                title={t('k8s.delete.title')}
            />
        </>
    )
}

function RoleDrawer({
    target,
    clusters,
    onClose,
    onSaved
}: {
    clusters: string[]
    onClose: () => void
    onSaved: (e: Entry) => void
    target: null | { cluster: string; ns: string; role?: K8sRoleFull; sa: string }
}) {
    const { t } = useTranslation('auth')
    const opened = target !== null
    const edit = !!target?.role

    const [cluster, setCluster] = useState('')
    const [ns, setNs] = useState('')
    const [sa, setSa] = useState('')
    const [policies, setPolicies] = useState<string[]>([])
    const [tns, setTns] = useState('')
    const [ttl, setTtl] = useState(0)
    const [touched, setTouched] = useState(false)
    const [serverErr, setServerErr] = useState('')

    useEffect(() => {
        if (!target) return
        setCluster(target.cluster)
        setNs(target.ns)
        setSa(target.sa)
        setPolicies(target.role?.policies ?? [])
        setTns(target.role?.tuck_namespace ?? '')
        setTtl(parseGoDuration(target.role?.ttl) ?? 0)
        setTouched(false)
        setServerErr('')
    }, [target])

    const nsErr = !K8S_PART_RE.test(ns) ? t('k8s.partBad') : ''
    const saErr = !K8S_PART_RE.test(sa) ? t('k8s.partBad') : ''
    const polErr = policies.length === 0 ? t('f.policiesNone') : ''

    const body = { policies, ttl: toDur(ttl), tuck_namespace: tns || undefined }

    const save = useMutation({
        mutationFn: async () => {
            await putK8sRole(cluster, ns, sa, body)
            return getK8sRole(cluster, ns, sa)
        },
        onSuccess: (role) => {
            toast.success(t('k8s.toast.saved'), t('k8s.toast.savedMsg', { name: `${ns}/${sa}` }))
            onSaved({ cluster, ns, role, sa })
        },
        onError: (err) => {
            setServerErr(serverMessage(err) ?? '')
            toast.error(t('k8s.toast.saveFailed'), serverMessage(err))
        }
    })

    const submit = () => {
        setTouched(true)
        if (nsErr || saErr || polErr || save.isPending) return
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
            title={edit ? t('k8s.drawer.edit', { name: `${ns}/${sa}` }) : t('k8s.drawer.new')}
        >
            <form
                className={styles.form}
                noValidate
                onSubmit={(e) => {
                    e.preventDefault()
                    submit()
                }}
            >
                <p className={styles.hint}>{t('k8s.drawer.hint')}</p>
                <Field label={t('k8s.cluster')}>
                    <div className={styles.seg}>
                        {clusters.map((c) => (
                            <button
                                aria-pressed={cluster === c}
                                className={cx(cluster === c && styles.on)}
                                disabled={edit}
                                key={c || '__default'}
                                onClick={() => setCluster(c)}
                                type="button"
                            >
                                {c || t('k8s.defaultCluster')}
                            </button>
                        ))}
                    </div>
                </Field>
                <div className={styles.grid2}>
                    <Field error={touched ? nsErr : ''} label={t('k8s.namespace')}>
                        <TextInput
                            aria-label={t('k8s.namespace')}
                            data-autofocus
                            disabled={edit}
                            onChange={(e) => setNs(e.currentTarget.value.trim())}
                            placeholder="payments"
                            value={ns}
                        />
                    </Field>
                    <Field error={touched ? saErr : ''} label={t('k8s.serviceAccount')}>
                        <TextInput
                            aria-label={t('k8s.serviceAccount')}
                            disabled={edit}
                            onChange={(e) => setSa(e.currentTarget.value.trim())}
                            placeholder="api"
                            value={sa}
                        />
                    </Field>
                </div>
                <PolicyField error={touched ? polErr : ''} onChange={setPolicies} value={policies} />
                <NamespaceField onChange={setTns} value={tns} />
                <TtlField
                    description={t('f.ttlDefault')}
                    label="ttl"
                    max={TTL_MAX}
                    onChange={setTtl}
                    value={ttl}
                />
                <div className={styles.fld}>
                    <div className={styles.lbl}>
                        <span>{t('k8s.preview')}</span>
                    </div>
                    <pre className={styles.prev}>
                        <span className={styles.jk}>PUT</span> {k8sRolePath(cluster, ns || '{ns}', sa || '{sa}')}
                        {'\n'}
                        {JSON.stringify(body, null, 2)}
                    </pre>
                </div>
                {serverErr && (
                    <div className={styles.alert} role="alert">
                        <Icon name="alert" size={18} />
                        <div>
                            <b>{t('k8s.toast.saveFailed')}</b>
                            {serverErr}
                        </div>
                    </div>
                )}
            </form>
        </Drawer>
    )
}
