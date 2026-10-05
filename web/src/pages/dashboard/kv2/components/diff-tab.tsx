// SPDX-License-Identifier: AGPL-3.0-only
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Icon, SkeletonLine, StateBlock } from '@shared/ui/kit'

import common from '../common.module.css'
import styles from '../detail.module.css'
import { useKv2Version } from '../use-kv2'
import { fmtRel, lcsDiff, valueLines, VerRow } from '../utils'
import { errMessage } from '../api'

export function DiffTab({
    path,
    rows,
    a,
    b,
    onA,
    onB,
    show,
    onShow
}: {
    a?: number
    b?: number
    onA: (v: number) => void
    onB: (v: number) => void
    onShow: (v: boolean) => void
    path: string
    rows: VerRow[]
    show: boolean
}) {
    const { t, i18n } = useTranslation('kv')
    const ok = rows.filter((r) => r.state === 'active')
    const okSet = new Set(ok.map((r) => r.version))
    // fall back to the two newest readable versions when the picked ones are not valid
    const valid = a !== undefined && b !== undefined && a !== b && okSet.has(a) && okSet.has(b)
    const A = valid ? a : ok.length >= 2 ? ok[ok.length - 2].version : undefined
    const B = valid ? b : ok.length >= 2 ? ok[ok.length - 1].version : undefined

    const qa = useKv2Version(path, A, A !== undefined)
    const qb = useKv2Version(path, B, B !== undefined)

    const df = useMemo(() => {
        if (qa.data?.value === undefined || qb.data?.value === undefined) return null
        return lcsDiff(valueLines(qa.data.value, show), valueLines(qb.data.value, show))
    }, [qa.data?.value, qb.data?.value, show])

    if (ok.length < 2) {
        return (
            <div className={styles.tp}>
                <div className={styles.locked}>
                    <Icon name="info" size={34} style={{ color: 'var(--dim)' }} />
                    <h4>{t('kv2.diff.noneTitle')}</h4>
                    <p>{t('kv2.diff.noneText')}</p>
                </div>
            </div>
        )
    }

    const opt = (sel: number | undefined) =>
        ok.map((r) => (
            <option key={r.version} value={r.version}>
                {`v${r.version} · ${fmtRel(r.createdAt, i18n.language)}`}
            </option>
        ))

    const na = df?.filter((x) => x.t === 'add').length ?? 0
    const nd = df?.filter((x) => x.t === 'del').length ?? 0
    const ne = (df?.length ?? 0) - na - nd
    const err = qa.error ?? qb.error

    return (
        <div className={styles.tp}>
            <div className={styles.dsel}>
                <div className={common.fld}>
                    <label className={common.lab} htmlFor="kv2-da">
                        {t('kv2.diff.was')}
                    </label>
                    <select className={styles.sel2} id="kv2-da" onChange={(e) => onA(Number(e.target.value))} value={A}>
                        {opt(A)}
                    </select>
                </div>
                <span className={styles.arr}>→</span>
                <div className={common.fld}>
                    <label className={common.lab} htmlFor="kv2-db">
                        {t('kv2.diff.became')}
                    </label>
                    <select className={styles.sel2} id="kv2-db" onChange={(e) => onB(Number(e.target.value))} value={B}>
                        {opt(B)}
                    </select>
                </div>
            </div>
            <div className={styles.bar2}>
                <div className={styles.dstat}>
                    <span className={styles.a}>+{na}</span>
                    <span className={styles.d}>−{nd}</span>
                    <span className={styles.e}>={ne}</span>
                </div>
                <button className={common.sw} data-on={show || undefined} onClick={() => onShow(!show)} type="button">
                    <i />
                    {t('kv2.diff.showValues')}
                </button>
            </div>
            {err ? (
                <StateBlock compact details={errMessage(err)} kind="error" onRetry={() => void Promise.all([qa.refetch(), qb.refetch()])} />
            ) : !df ? (
                <div className={styles.skel} aria-busy>
                    <SkeletonLine />
                    <SkeletonLine width="80%" />
                    <SkeletonLine width="60%" />
                </div>
            ) : (
                <>
                    <div className={styles.diff}>
                        {df.map((x, i) => (
                            <div className={styles.dl} data-t={x.t} key={i} style={{ ['--i' as string]: Math.min(i, 24) }}>
                                <span className={styles.n}>{x.a ?? ''}</span>
                                <span className={styles.n}>{x.b ?? ''}</span>
                                <span className={styles.m}>{x.t === 'add' ? '+' : x.t === 'del' ? '−' : ' '}</span>
                                <span>{x.line.txt}</span>
                            </div>
                        ))}
                    </div>
                    {!show && <div className={common.msg}>{t('kv2.diff.hiddenNote')}</div>}
                </>
            )}
        </div>
    )
}
