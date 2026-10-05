// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { lookupSelf } from '@shared/api/endpoints/auth'
import { listNamespaces } from '@shared/api/endpoints/namespace'
import { getNamespace, setNamespace } from '@shared/auth/token'
import ctl from '@shared/ui/shell-controls/controls.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'
import { usePopover } from '@shared/ui/shell-controls/use-popover'

const ROOT = ''

export function NamespaceSwitcher() {
    const { t } = useTranslation('shell')
    const pop = usePopover()
    const { data: namespaces } = useQuery({ queryKey: ['namespaces', 'list'], queryFn: listNamespaces })
    const { data: self } = useQuery({
        queryKey: ['auth', 'lookup-self'],
        queryFn: lookupSelf,
        staleTime: 60_000
    })
    // A namespace-bound token may only act inside its own namespace (and children):
    // the server answers 403 for any other X-Tuck-Namespace value.
    const bound = (self as { namespace?: string } | undefined)?.namespace ?? ''
    const [stored, setStored] = useState(() => getNamespace() ?? ROOT)
    const current = stored || bound

    const options = bound
        ? [bound, ...(namespaces ?? []).filter((ns) => ns.startsWith(`${bound}/`))]
        : (namespaces ?? [])

    const switchTo = (ns: string) => {
        pop.close()
        if (ns === current) return
        setNamespace(ns)
        setStored(ns)
        // Full reload: many query keys across the app aren't namespace-scoped,
        // so an in-place refetch risks showing data cached under another
        // namespace. A reload guarantees a clean slate.
        window.location.reload()
    }

    const label = current || t('rootNamespace')

    return (
        <div className={ctl.pop} ref={pop.ref}>
            <button
                aria-expanded={pop.open}
                aria-haspopup="menu"
                aria-label={t('namespaceSwitch')}
                className={`${ctl.ctrl} ${ctl.tip}`}
                data-open={pop.open}
                data-tip={t('namespace', { namespace: label })}
                onClick={pop.toggle}
                type="button"
            >
                <ShellIcon name="ns" />
                {current && <span className={ctl.bdg}>{current}</span>}
            </button>
            <div className={ctl.menu} data-open={pop.open} role="menu">
                <div className={ctl.lbl}>{t('namespaceSwitch')}</div>
                <div className={ctl.scroll}>
                    {!bound && (
                        <button
                            className={`${ctl.item} ${ctl.mono}`}
                            data-current={current === ROOT}
                            onClick={() => switchTo(ROOT)}
                            role="menuitem"
                            type="button"
                        >
                            {t('rootNamespace')}
                        </button>
                    )}
                    {options.map((ns) => (
                        <button
                            className={`${ctl.item} ${ctl.mono}`}
                            data-current={current === ns}
                            key={ns}
                            onClick={() => switchTo(ns)}
                            role="menuitem"
                            type="button"
                        >
                            {ns}
                        </button>
                    ))}
                </div>
            </div>
        </div>
    )
}
