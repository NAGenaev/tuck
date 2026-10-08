// SPDX-License-Identifier: AGPL-3.0-only
import { ActionIcon, Button, Select, TagsInput, TextInput } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import {
    TeamAccess,
    TeamK8sBinding,
    TeamRequest,
    TeamResult,
    createTeam,
    listK8sClusters,
    planTeam
} from '@shared/api/endpoints/teams'
import { CopyButton, Icon, toast } from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Pill } from '@shared/ui/tuck/tuck'

import styles from './teams.module.css'

const ACCESS: TeamAccess[] = ['admin', 'developer', 'reader']
const NAME_RE = /^[a-z0-9][a-z0-9-]*(\/[a-z0-9][a-z0-9-]*)*$/
const TTL_RE = /^\d+(s|m|h)$/

const rich = { b: <b />, code: <code /> }

interface Binding {
    access: 'developer' | 'reader'
    cluster: string
    id: number
    namespace: string
    service_account: string
}

function errorText(err: unknown): string {
    if (isAxiosError(err)) {
        const d = err.response?.data as { error?: string } | undefined
        return d?.error ?? err.message
    }
    return (err as Error)?.message ?? String(err)
}

export function TeamsPage() {
    const { t } = useTranslation('teams')
    const qc = useQueryClient()

    const [name, setName] = useState('')
    const [description, setDescription] = useState('')
    const [owner, setOwner] = useState('')
    const [ttl, setTtl] = useState('')
    const [groups, setGroups] = useState<Record<TeamAccess, string[]>>({ admin: [], developer: [], reader: [] })
    const [bindings, setBindings] = useState<Binding[]>([])
    const [nextId, setNextId] = useState(1)
    const [plan, setPlan] = useState<null | TeamResult>(null)
    const [done, setDone] = useState<null | TeamResult>(null)
    const [error, setError] = useState<null | string>(null)

    const clusters = useQuery({ queryFn: listK8sClusters, queryKey: ['k8s-clusters'], retry: false })
    const clusterOptions = [
        { label: t('k8s.defaultCluster'), value: '' },
        ...(clusters.data ?? []).map((c) => ({ label: c, value: c }))
    ]

    const nameError = name !== '' && !NAME_RE.test(name) ? t('errors.name') : null
    const ttlError = ttl !== '' && !TTL_RE.test(ttl) ? t('errors.ttl') : null
    const groupCount = ACCESS.reduce((n, a) => n + groups[a].length, 0)
    const bindingsOk = bindings.every((b) => b.namespace.trim() && b.service_account.trim())
    const canSubmit = name !== '' && !nameError && !ttlError && groupCount > 0 && bindingsOk

    function request(): TeamRequest {
        return {
            description: description.trim() || undefined,
            groups: Object.fromEntries(ACCESS.filter((a) => groups[a].length > 0).map((a) => [a, groups[a]])),
            k8s: bindings.length
                ? bindings.map<TeamK8sBinding>((b) => ({
                      access: b.access,
                      cluster: b.cluster || undefined,
                      namespace: b.namespace.trim(),
                      service_account: b.service_account.trim()
                  }))
                : undefined,
            name,
            owner: owner.trim() || undefined,
            ttl: ttl || undefined
        }
    }

    const preview = useMutation({
        mutationFn: () => planTeam(request()),
        onError: (e) => {
            setPlan(null)
            setError(errorText(e))
        },
        onSuccess: (r) => {
            setError(null)
            setPlan(r)
        }
    })

    const create = useMutation({
        mutationFn: () => createTeam(request()),
        onError: (e) => {
            setError(errorText(e))
            if (isAxiosError(e)) {
                const r = (e.response?.data as undefined | { result?: TeamResult })?.result
                if (r) setPlan(r)
            }
        },
        onSuccess: (r) => {
            setError(null)
            setPlan(null)
            setDone(r)
            toast.success(t('toast.created'), r.namespace)
            void qc.invalidateQueries()
        }
    })

    const reset = () => {
        setName('')
        setDescription('')
        setOwner('')
        setTtl('')
        setGroups({ admin: [], developer: [], reader: [] })
        setBindings([])
        setPlan(null)
        setDone(null)
        setError(null)
    }

    const patchBinding = (id: number, p: Partial<Binding>) => {
        setBindings((bs) => bs.map((b) => (b.id === id ? { ...b, ...p } : b)))
        setPlan(null)
    }

    const hasConflict = !!plan?.items.some((i) => i.action === 'conflict')
    const busy = preview.isPending || create.isPending

    return (
        <Page title={t('title')}>
            <div className={styles.phead}>
                <Eyebrow>{t('eyebrow')}</Eyebrow>
                <h1 className={styles.h1}>
                    {t('h1a')}
                    <br />
                    <span className={styles.gold}>{t('h1b')}</span>
                </h1>
                <p className={styles.lead}>
                    <Trans components={rich} i18nKey="lead" ns="teams" />
                </p>
            </div>

            {done ? (
                <DoneView onAnother={reset} result={done} />
            ) : (
                <div className={styles.layout}>
                    <div>
                        <section className={styles.panel}>
                            <div className={styles.panelH}>
                                <h3>{t('team.title')}</h3>
                            </div>
                            <div className={styles.panelB}>
                                <TextInput
                                    description={t('team.nameHint')}
                                    error={nameError}
                                    label={t('team.name')}
                                    onChange={(e) => {
                                        setName(e.currentTarget.value.trim())
                                        setPlan(null)
                                    }}
                                    placeholder="team-payments"
                                    required
                                    value={name}
                                />
                                <div className={styles.grid2}>
                                    <TextInput
                                        label={t('team.description')}
                                        onChange={(e) => setDescription(e.currentTarget.value)}
                                        value={description}
                                    />
                                    <TextInput
                                        label={t('team.owner')}
                                        onChange={(e) => setOwner(e.currentTarget.value)}
                                        placeholder="pay-lead@bank.local"
                                        value={owner}
                                    />
                                </div>
                                <TextInput
                                    description={t('team.ttlHint')}
                                    error={ttlError}
                                    label={t('team.ttl')}
                                    onChange={(e) => {
                                        setTtl(e.currentTarget.value.trim())
                                        setPlan(null)
                                    }}
                                    placeholder="8h"
                                    value={ttl}
                                />
                            </div>
                        </section>

                        <section className={styles.panel}>
                            <div className={styles.panelH}>
                                <h3>{t('groups.title')}</h3>
                            </div>
                            <div className={styles.panelB}>
                                <p className={styles.hint}>
                                    <Trans components={rich} i18nKey="groups.hint" ns="teams" />
                                </p>
                                {ACCESS.map((a) => (
                                    <TagsInput
                                        description={t(`groups.${a}Hint`)}
                                        key={a}
                                        label={t(`groups.${a}`)}
                                        onChange={(v) => {
                                            setGroups((g) => ({ ...g, [a]: v }))
                                            setPlan(null)
                                        }}
                                        placeholder="CN=payments-dev,OU=Groups,DC=bank,DC=local"
                                        splitChars={[]}
                                        value={groups[a]}
                                    />
                                ))}
                            </div>
                        </section>

                        <section className={styles.panel}>
                            <div className={styles.panelH}>
                                <h3>{t('k8s.title')}</h3>
                                <Button
                                    leftSection={<Icon name="plus" size={14} />}
                                    onClick={() => {
                                        setBindings((bs) => [
                                            ...bs,
                                            { access: 'reader', cluster: '', id: nextId, namespace: '', service_account: '' }
                                        ])
                                        setNextId((n) => n + 1)
                                        setPlan(null)
                                    }}
                                    variant="default"
                                >
                                    {t('k8s.add')}
                                </Button>
                            </div>
                            <div className={styles.panelB}>
                                <p className={styles.hint}>
                                    <Trans components={rich} i18nKey="k8s.hint" ns="teams" />
                                </p>
                                {bindings.map((b) => (
                                    <div className={styles.bindRow} key={b.id}>
                                        <Select
                                            allowDeselect={false}
                                            data={clusterOptions}
                                            label={t('k8s.cluster')}
                                            onChange={(v) => patchBinding(b.id, { cluster: v ?? '' })}
                                            value={b.cluster}
                                        />
                                        <TextInput
                                            label={t('k8s.namespace')}
                                            onChange={(e) => patchBinding(b.id, { namespace: e.currentTarget.value })}
                                            value={b.namespace}
                                        />
                                        <TextInput
                                            label={t('k8s.serviceAccount')}
                                            onChange={(e) =>
                                                patchBinding(b.id, { service_account: e.currentTarget.value })
                                            }
                                            value={b.service_account}
                                        />
                                        <Select
                                            allowDeselect={false}
                                            data={[
                                                { label: t('groups.reader'), value: 'reader' },
                                                { label: t('groups.developer'), value: 'developer' }
                                            ]}
                                            label={t('k8s.access')}
                                            onChange={(v) =>
                                                patchBinding(b.id, { access: (v as Binding['access']) ?? 'reader' })
                                            }
                                            value={b.access}
                                        />
                                        <ActionIcon
                                            aria-label={t('k8s.remove')}
                                            onClick={() => {
                                                setBindings((bs) => bs.filter((x) => x.id !== b.id))
                                                setPlan(null)
                                            }}
                                            variant="default"
                                        >
                                            <Icon name="trash" size={14} />
                                        </ActionIcon>
                                    </div>
                                ))}
                            </div>
                        </section>
                    </div>

                    <div>
                        <section className={styles.panel}>
                            <div className={styles.panelH}>
                                <h3>{t('plan.title')}</h3>
                            </div>
                            {error && (
                                <div className={styles.panelB}>
                                    <p className={styles.err} role="alert">
                                        {error}
                                    </p>
                                </div>
                            )}
                            {plan ? <PlanTable result={plan} /> : <div className={styles.empty}>{t('plan.empty')}</div>}
                            <div className={styles.actions}>
                                <Button
                                    disabled={!canSubmit || create.isPending}
                                    loading={preview.isPending}
                                    onClick={() => preview.mutate()}
                                    variant="default"
                                >
                                    {t('plan.preview')}
                                </Button>
                                <Button
                                    disabled={!canSubmit || !plan || hasConflict || busy}
                                    loading={create.isPending}
                                    onClick={() => create.mutate()}
                                >
                                    {t('plan.create')}
                                </Button>
                            </div>
                        </section>
                    </div>
                </div>
            )}
        </Page>
    )
}

function PlanTable({ done, result }: { done?: boolean; result: TeamResult }) {
    const { t } = useTranslation('teams')
    return (
        <table className={styles.plan}>
            <thead>
                <tr>
                    <th>{t('plan.kind')}</th>
                    <th>{t('plan.name')}</th>
                    <th>{t('plan.action')}</th>
                </tr>
            </thead>
            <tbody>
                {result.items.map((it) => (
                    <tr key={`${it.kind}/${it.name}`}>
                        <td>{t(`kind.${it.kind}`)}</td>
                        <td>
                            <span className={styles.mono}>{it.name}</span>
                            {it.detail && <div className={styles.hint}>{it.detail}</div>}
                        </td>
                        <td>
                            <Pill tone={it.action === 'conflict' ? 'bad' : 'ok'}>{t(done && it.action === 'create' ? 'action.created' : `action.${it.action}`)}</Pill>
                        </td>
                    </tr>
                ))}
            </tbody>
        </table>
    )
}

function DoneView({ onAnother, result }: { onAnother: () => void; result: TeamResult }) {
    const { t } = useTranslation('teams')
    const hints = result.k8s_hints ?? []
    return (
        <section className={styles.panel}>
            <div className={styles.panelH}>
                <h3>{t('done.title', { name: result.namespace })}</h3>
                <Button onClick={onAnother} variant="default">
                    {t('done.another')}
                </Button>
            </div>
            <PlanTable done result={result} />
            <div className={styles.panelB}>
                <p className={styles.hint}>
                    <Trans components={rich} i18nKey="done.next" ns="teams" />
                </p>
                {hints.length === 0 && <p className={styles.hint}>{t('done.noHints')}</p>}
                {hints.map((h) => {
                    const ctx = h.cluster ? `--context ${h.cluster} ` : ''
                    const cmd = `kubectl ${ctx}annotate namespace ${h.namespace} ${h.annotation} --overwrite`
                    return (
                        <div className={styles.snippetRow} key={`${h.cluster ?? ''}/${h.namespace}`}>
                            <pre className={styles.snippet}>{cmd}</pre>
                            <CopyButton value={cmd} />
                        </div>
                    )
                })}
            </div>
        </section>
    )
}
