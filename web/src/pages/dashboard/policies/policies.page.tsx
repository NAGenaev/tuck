// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { deletePolicy, Policy, putPolicy } from '@shared/api/endpoints/policy'
import { ConfirmModal, CountUp, Icon, SkeletonLine, Spinner, StateBlock, toast } from '@shared/ui/kit'
import { Page } from '@shared/ui/page/page'
import { Eyebrow, Plate } from '@shared/ui/tuck/tuck'

import { NewPolicyModal, NewTemplate } from './components/new-policy-modal'
import { PolicyEditor } from './components/policy-editor'
import { PolicyList, PolicyRow } from './components/policy-list'
import { LIST_KEY, policyKey, usePolicyDetails, usePolicyList, useTokenUsage } from './hooks/use-policies'
import { canonical, fmtRule, parsePolicy, rulesToText, TEMPLATES } from './lib/policy-text'
import s from './policies.module.css'

interface Draft {
    inh: boolean
    text: string
}

function errMsg(e: unknown): string {
    if (isAxiosError(e)) return (e.response?.data as { error?: string } | undefined)?.error ?? e.message
    return e instanceof Error ? e.message : String(e)
}

const serverCanon = (p: Policy) =>
    canonical(
        (p.rules ?? []).map((r) => ({ caps: r.capabilities ?? [], path: r.path ?? '' })),
        !!p.inheritable
    )

export function PoliciesPage() {
    const { t } = useTranslation('policies')
    const queryClient = useQueryClient()

    const list = usePolicyList()
    const serverNames = useMemo(() => list.data ?? [], [list.data])
    const details = usePolicyDetails(serverNames)

    // local, not-yet-saved state
    const [newDrafts, setNewDrafts] = useState<Record<string, Draft>>({})
    const [drafts, setDrafts] = useState<Record<string, Draft>>({})
    const [selectedRaw, setSelected] = useState<null | string>(null)
    const [creating, setCreating] = useState<false | NewTemplate>(false)
    const [deleting, setDeleting] = useState<null | string>(null)

    const allNames = useMemo(() => [...Object.keys(newDrafts), ...serverNames], [newDrafts, serverNames])
    const selected = selectedRaw && allNames.includes(selectedRaw) ? selectedRaw : (allNames[0] ?? null)

    const current = (name: string): Draft | null => {
        if (newDrafts[name]) return newDrafts[name]
        if (drafts[name]) return drafts[name]
        const d = details[name]?.data
        return d ? { inh: !!d.inheritable, text: rulesToText(d.rules) } : null
    }

    const rows: PolicyRow[] = allNames.map((name) => {
        const cur = current(name)
        const isNew = !!newDrafts[name]
        const parsed = cur ? parsePolicy(cur.text) : null
        const data = details[name]?.data
        const dirty =
            !isNew && !!drafts[name] && !!data && !!parsed
                ? canonical(parsed.rules, drafts[name].inh) !== serverCanon(data)
                : false
        return { dirty, failed: !isNew && !!details[name]?.isError, isNew, name, parsed }
    })
    const rowOf = (name: string) => rows.find((r) => r.name === name)

    const totals = useMemo(() => {
        let rules = 0
        let deny = 0
        rows.forEach((r) => {
            r.parsed?.rules.forEach((x) => {
                rules += 1
                if (x.caps.includes('deny')) deny += 1
            })
        })
        return { deny, rules }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [rows.map((r) => `${r.name}:${r.parsed?.rules.length}:${r.parsed?.diags.length}`).join('|'), drafts, newDrafts])

    const patchDraft = (name: string, patch: Partial<Draft>) => {
        const base = current(name)
        if (!base) return
        const next = { ...base, ...patch }
        if (newDrafts[name]) setNewDrafts((d) => ({ ...d, [name]: next }))
        else setDrafts((d) => ({ ...d, [name]: next }))
    }

    const saveMutation = useMutation({
        mutationFn: (v: { inh: boolean; name: string; policy: Policy }) => putPolicy(v.policy),
        onError: (e) => toast.error(t('toast.saveFailed'), errMsg(e)),
        onSuccess: (_, v) => {
            queryClient.setQueryData(policyKey(v.name), v.policy)
            setNewDrafts(({ [v.name]: _gone, ...rest }) => rest)
            setDrafts(({ [v.name]: _gone, ...rest }) => rest)
            void queryClient.invalidateQueries({ queryKey: LIST_KEY })
            toast.success(t('toast.saved'), t('toast.savedText', { count: v.policy.rules.length, name: v.name }))
        }
    })

    const save = (name: string) => {
        const cur = current(name)
        if (!cur) return
        const parsed = parsePolicy(cur.text)
        if (parsed.errors > 0) {
            toast.error(t('toast.notSaved'), t('toast.fixErrors', { count: parsed.errors }))
            return
        }
        const policy: Policy = {
            inheritable: cur.inh,
            name,
            rules: parsed.rules.map((r) => ({ capabilities: r.caps, path: r.path }))
        }
        saveMutation.mutate({ inh: cur.inh, name, policy })
    }

    const reset = (name: string) => {
        if (newDrafts[name]) {
            setNewDrafts(({ [name]: _gone, ...rest }) => rest)
            toast.info(t('toast.discarded'), name)
            return
        }
        setDrafts(({ [name]: _gone, ...rest }) => rest)
        toast.info(t('toast.reverted'), name)
    }

    const usage = useTokenUsage(deleting)

    const remove = async (name: string) => {
        try {
            await deletePolicy(name)
        } catch (e) {
            toast.error(t('toast.deleteFailed'), errMsg(e))
            throw e
        }
        queryClient.removeQueries({ queryKey: policyKey(name) })
        setDrafts(({ [name]: _gone, ...rest }) => rest)
        await queryClient.invalidateQueries({ queryKey: LIST_KEY })
        toast.success(t('toast.deleted'), name)
    }

    const create = (v: { inheritable: boolean; name: string; template: NewTemplate }) => {
        const tpl = v.template === 'none' ? null : TEMPLATES[v.template]
        setNewDrafts((d) => ({
            [v.name]: { inh: v.inheritable, text: tpl ? fmtRule(tpl.path, tpl.caps) : '' },
            ...d
        }))
        setSelected(v.name)
        setCreating(false)
        toast.info(t('toast.created'), t('toast.createdText', { name: v.name }))
    }

    const listForbidden = list.isError && isAxiosError(list.error) && list.error.response?.status === 403
    const selRow = selected ? rowOf(selected) : undefined
    const selDetail = selected ? details[selected] : undefined
    const selCur = selected ? current(selected) : null

    const deletingBound = usage.data?.bound ?? []
    const usageKnown = !!usage.data
    const confirmName = deleting && (!usageKnown || deletingBound.length > 0) ? deleting : undefined

    let editor
    if (list.isLoading || (selected && !selRow?.isNew && selDetail?.isLoading)) {
        editor = (
            <Plate>
                <div className={s.plateTop}>
                    <span className={s.ttl}>{t('loading')}</span>
                    {selected && <span className={s.sp}>GET /v1/policy/{selected}</span>}
                </div>
                <div aria-busy className={s.skBody}>
                    <SkeletonLine height={22} width="38%" />
                    <SkeletonLine width="92%" />
                    <SkeletonLine width="76%" />
                    <SkeletonLine width="84%" />
                    <SkeletonLine width="60%" />
                    <SkeletonLine width="88%" />
                    <div className={s.skCells}>
                        {[0, 1, 2, 3, 4].map((i) => (
                            <SkeletonLine height={30} key={i} />
                        ))}
                    </div>
                </div>
            </Plate>
        )
    } else if (allNames.length === 0) {
        editor = (
            <Plate>
                <div className={s.emptyBox}>
                    <svg
                        className={s.emptyShield}
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        viewBox="0 0 84 84"
                    >
                        <path d="M42 8l28 10v20c0 18-12 31-28 38C26 69 14 56 14 38V18z" />
                        <path d="M30 42l8 8 16-18" strokeDasharray="4 4" />
                    </svg>
                    <h3>{t('empty.title')}</h3>
                    <p>{t('empty.text')}</p>
                    <div className={s.chips} style={{ justifyContent: 'center' }}>
                        <Button onClick={() => setCreating('ro')} size="xs" variant="default">
                            {t('tpl.ro')}
                        </Button>
                        <Button onClick={() => setCreating('rw')} size="xs" variant="default">
                            {t('tpl.rw')}
                        </Button>
                    </div>
                    <Button onClick={() => setCreating('none')}>{t('empty.cta')}</Button>
                </div>
            </Plate>
        )
    } else if (selected && !selRow?.isNew && selDetail?.isError) {
        const forbidden = isAxiosError(selDetail.error) && selDetail.error.response?.status === 403
        editor = (
            <Plate>
                <StateBlock
                    details={errMsg(selDetail.error)}
                    kind={forbidden ? 'forbidden' : 'error'}
                    onRetry={() => void queryClient.invalidateQueries({ queryKey: policyKey(selected) })}
                />
            </Plate>
        )
    } else if (selected && selCur && selRow?.parsed) {
        editor = (
            <PolicyEditor
                dirty={selRow.isNew || selRow.dirty}
                inheritable={selCur.inh}
                isNew={selRow.isNew}
                key={selected}
                name={selected}
                onDelete={() => setDeleting(selected)}
                onInheritable={(v) => patchDraft(selected, { inh: v })}
                onReset={() => reset(selected)}
                onSave={() => save(selected)}
                onText={(text) => patchDraft(selected, { text })}
                parsed={selRow.parsed}
                saving={saveMutation.isPending && saveMutation.variables?.name === selected}
                text={selCur.text}
            />
        )
    } else {
        editor = null
    }

    return (
        <Page title="Policies">
            <header className={s.head}>
                <div className={s.rise} style={{ ['--d' as string]: 0 }}>
                    <Eyebrow>{t('eyebrow')}</Eyebrow>
                    <h1 className={s.h1}>
                        {t('headA')} <span className={s.gold}>{t('headB')}</span>
                    </h1>
                    <p className={s.lead}>
                        <Trans components={{ b: <b /> }} i18nKey="lead" ns="policies" t={t} />
                    </p>
                </div>
                <div className={`${s.rise} ${s.headSide}`} style={{ ['--d' as string]: 120 }}>
                    <div className={s.meters}>
                        <div className={s.meter}>
                            <small>{t('meter.policies')}</small>
                            <strong>{list.isLoading ? <Spinner /> : <CountUp value={allNames.length} />}</strong>
                        </div>
                        <div className={s.meter}>
                            <small>{t('meter.rules')}</small>
                            <strong>
                                <CountUp value={totals.rules} />
                            </strong>
                        </div>
                        <div className={`${s.meter} ${s.meterHot}`}>
                            <small>{t('meter.deny')}</small>
                            <strong>
                                <CountUp value={totals.deny} />
                            </strong>
                        </div>
                    </div>
                    <Button
                        className={s.sweep}
                        leftSection={<Icon name="plus" size={15} />}
                        onClick={() => setCreating('none')}
                    >
                        {t('new')}
                    </Button>
                </div>
            </header>

            {list.isError ? (
                <StateBlock
                    details={errMsg(list.error)}
                    kind={listForbidden ? 'forbidden' : 'error'}
                    onRetry={() => void list.refetch()}
                    retrying={list.isRefetching}
                />
            ) : (
                <div className={s.work}>
                    <div className={s.rise} style={{ ['--d' as string]: 260 }}>
                        <PolicyList
                            loading={list.isLoading}
                            onSelect={setSelected}
                            rows={rows}
                            selected={selected}
                        />
                    </div>
                    <section className={`${s.edcol} ${s.rise}`} style={{ ['--d' as string]: 340 }}>
                        {editor}
                    </section>
                </div>
            )}

            <NewPolicyModal
                existing={allNames}
                initialTemplate={creating || 'none'}
                onClose={() => setCreating(false)}
                onCreate={create}
                opened={creating !== false}
            />

            <ConfirmModal
                confirmLabel={t('delete')}
                confirmText={confirmName}
                message={
                    <>
                        <p style={{ margin: '0 0 12px' }}>
                            <Trans
                                components={{ b: <b style={{ color: 'var(--bone)' }} /> }}
                                i18nKey="del.text"
                                ns="policies"
                                t={t}
                                values={{ name: deleting }}
                            />
                        </p>
                        {usage.isLoading && (
                            <div className={s.checking}>
                                <Spinner /> {t('del.checking')}
                            </div>
                        )}
                        {usage.isError && <div className={s.checking}>{t('del.checkFailed')}</div>}
                        {usageKnown && deletingBound.length > 0 && (
                            <div className={s.warnbox}>
                                <b>{t('del.bound', { count: deletingBound.length })}</b>
                                <div className={s.toklist}>
                                    {deletingBound.slice(0, 4).map((b, i) => (
                                        <div key={i}>
                                            <span>{b.label}</span>
                                        </div>
                                    ))}
                                    {deletingBound.length > 4 && (
                                        <div>{t('del.more', { count: deletingBound.length - 4 })}</div>
                                    )}
                                </div>
                                <span className={s.mHint}>{t('del.warn403')}</span>
                            </div>
                        )}
                        {usageKnown && deletingBound.length === 0 && (
                            <div className={s.okbox}>
                                {t('del.safe')}
                                {usage.data && usage.data.total > usage.data.scanned
                                    ? ` ${t('del.partial', { scanned: usage.data.scanned, total: usage.data.total })}`
                                    : ''}
                            </div>
                        )}
                    </>
                }
                onClose={() => setDeleting(null)}
                onConfirm={() => (deleting ? remove(deleting) : undefined)}
                opened={deleting !== null}
                title={t('del.title')}
            />
        </Page>
    )
}
