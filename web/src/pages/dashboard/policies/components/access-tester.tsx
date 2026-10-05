// SPDX-License-Identifier: AGPL-3.0-only
import { Trans, useTranslation } from 'react-i18next'

import { cx } from '@shared/ui/kit'
import { Plate } from '@shared/ui/tuck/tuck'

import { CAPS, Evaluation, globMatch } from '../lib/policy-text'
import s from '../policies.module.css'

const TESTER_CAPS = CAPS.filter((c) => c !== 'deny')

/**
 * Deterministic path / capability matcher over the policy text being edited. It mirrors the
 * server's glob rules (internal/policy) for this one policy; nothing is sent to the API.
 */
export function AccessTester({
    path,
    cap,
    ev,
    samples,
    hasRules,
    onPath,
    onCap,
    onSample,
    onJump
}: {
    cap: string
    ev: Evaluation | null
    hasRules: boolean
    onCap: (cap: string) => void
    onJump: (line: number) => void
    onPath: (path: string) => void
    onSample: (path: string, cap: string) => void
    path: string
    samples: Array<{ cap: string; path: string }>
}) {
    const { t } = useTranslation('policies')
    const trimmed = path.trim()
    const segs = trimmed ? trimmed.split('/') : []
    const rulePath = ev?.rule ? ev.rule.path.split('/') : []
    const star = rulePath.indexOf('**')

    const verdict = !ev ? undefined : ev.allow ? 'allow' : 'deny'
    const badgeKey = `${verdict}|${ev?.rule?.line ?? '-'}|${cap}`

    return (
        <Plate>
            <div className={s.plateTop}>
                <span className={s.ttl}>{t('tester.title')}</span>
                <span className={s.sp}>{t('tester.sub')}</span>
            </div>
            <div className={s.tester}>
                <div className={s.tin}>
                    <div className={s.field}>
                        <label className={s.fieldLabel} htmlFor="pol-tpath">
                            {t('tester.path')}
                        </label>
                        <input
                            autoComplete="off"
                            className={s.inp}
                            id="pol-tpath"
                            onChange={(e) => onPath(e.currentTarget.value)}
                            placeholder="secret/payments/db"
                            spellCheck={false}
                            value={path}
                        />
                    </div>
                    <div className={s.field}>
                        <span className={s.fieldLabel}>{t('tester.op')}</span>
                        <div aria-label={t('tester.op')} className={s.capsel} role="group">
                            {TESTER_CAPS.map((c) => (
                                <button data-on={c === cap} key={c} onClick={() => onCap(c)} type="button">
                                    {c}
                                </button>
                            ))}
                        </div>
                    </div>
                    {samples.length > 0 && (
                        <div className={s.field}>
                            <span className={s.fieldLabel}>{t('tester.samples')}</span>
                            <div className={s.samples}>
                                {samples.map((x) => (
                                    <button
                                        key={`${x.path}|${x.cap}`}
                                        onClick={() => onSample(x.path, x.cap)}
                                        type="button"
                                    >
                                        {x.path} · {x.cap}
                                    </button>
                                ))}
                            </div>
                        </div>
                    )}
                    <div className={s.field}>
                        <span className={s.fieldLabel}>{t('tester.segments')}</span>
                        <div className={s.segs}>
                            {segs.map((seg, i) => {
                                const m =
                                    !!ev?.rule &&
                                    ((star > -1 && i >= star) ||
                                        (i < rulePath.length &&
                                            (rulePath[i] === '*' || rulePath[i] === seg || globMatch(rulePath[i], seg))))
                                return (
                                    <span data-m={m} key={`${i}-${seg}`} style={{ ['--i' as string]: i }}>
                                        {seg || '∅'}
                                    </span>
                                )
                            })}
                        </div>
                    </div>
                </div>
                <div className={s.vout} data-v={verdict}>
                    <div className={s.stamp}>
                        <span className={cx(s.badge, ev && s.slam)} key={badgeKey}>
                            {ev ? (ev.allow ? 'ALLOW' : 'DENY') : '—'}
                        </span>
                        <div className={s.why}>
                            {!ev ? (
                                <>
                                    <b>{t('tester.noPath')}</b>
                                    {hasRules ? t('tester.noPathHint') : t('tester.noRules')}
                                </>
                            ) : ev.kind === 'deny' ? (
                                <>
                                    <b>{t('tester.explicitDeny')}</b>
                                    <Trans
                                        components={{ em: <span className={s.whyDeny} /> }}
                                        i18nKey="tester.explicitDenyText"
                                        ns="policies"
                                        t={t}
                                        values={{ n: ev.rule?.n }}
                                    />
                                </>
                            ) : ev.kind === 'allow' ? (
                                <>
                                    <b>{t('tester.allowedBy')}</b>
                                    <Trans
                                        components={{ em: <span className={s.whyOk} /> }}
                                        i18nKey="tester.allowedByText"
                                        ns="policies"
                                        t={t}
                                        values={{ cap, n: ev.rule?.n }}
                                    />
                                </>
                            ) : ev.kind === 'nocap' ? (
                                <>
                                    <b>{t('tester.noCap')}</b>
                                    <Trans
                                        components={{ em: <span className={s.whyDeny} /> }}
                                        i18nKey="tester.noCapText"
                                        ns="policies"
                                        t={t}
                                        values={{ cap, count: ev.matches.length }}
                                    />
                                </>
                            ) : (
                                <>
                                    <b>{t('tester.noMatch')}</b>
                                    {t('tester.noMatchText')}
                                </>
                            )}
                        </div>
                    </div>
                    <div className={s.matchbox}>
                        {!ev ? (
                            t('tester.resultHere')
                        ) : ev.matches.length === 0 ? (
                            <div className={s.mrow}>
                                <span>{t('tester.zeroMatches')}</span>
                            </div>
                        ) : (
                            ev.matches.map((r) => {
                                const decides = r === ev.rule
                                const isDeny = r.caps.includes('deny')
                                return (
                                    <div
                                        className={s.mrow}
                                        data-deny={isDeny}
                                        data-use={decides}
                                        key={`${r.line}-${r.path}`}
                                    >
                                        <span className={s.mtag}>
                                            {isDeny ? 'deny' : decides ? t('tester.decides') : t('tester.matched')}
                                        </span>
                                        <span>{t('code.lineShort', { n: r.n })}</span>
                                        <span>{r.path}</span>
                                        <span className={s.dim}>[{r.caps.join(', ')}]</span>
                                        {decides && (
                                            <button className={s.jump} onClick={() => onJump(r.line)} type="button">
                                                {t('tester.jump')}
                                            </button>
                                        )}
                                    </div>
                                )
                            })
                        )}
                    </div>
                </div>
            </div>
        </Plate>
    )
}
