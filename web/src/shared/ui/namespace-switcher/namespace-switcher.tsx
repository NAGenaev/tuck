import { ActionIcon, Indicator, Menu, Tooltip } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbBoxMultiple } from 'react-icons/tb'

import { listNamespaces } from '@shared/api/endpoints/namespace'
import { getNamespace, setNamespace } from '@shared/auth/token'

const ROOT = ''

export function NamespaceSwitcher() {
    const { t } = useTranslation()
    const { data: namespaces } = useQuery({ queryKey: ['namespaces', 'list'], queryFn: listNamespaces })
    const [current, setCurrent] = useState(() => getNamespace() ?? ROOT)

    const switchTo = (ns: string) => {
        if (ns === current) return
        setNamespace(ns)
        setCurrent(ns)
        // Full reload: many query keys across the app aren't namespace-scoped,
        // so an in-place refetch risks showing data cached under another
        // namespace. A reload guarantees a clean slate.
        window.location.reload()
    }

    return (
        <Menu position="bottom-end" width={200}>
            <Menu.Target>
                <Tooltip label={t('header.namespace', { namespace: current || t('header.rootNamespace') })}>
                    <Indicator disabled={!current} label={current} position="bottom-end" size={14}>
                        <ActionIcon color="gray" radius="xl" size="lg" variant="light">
                            <TbBoxMultiple size={18} />
                        </ActionIcon>
                    </Indicator>
                </Tooltip>
            </Menu.Target>
            <Menu.Dropdown>
                <Menu.Label>{t('header.namespaceSwitch')}</Menu.Label>
                <Menu.Item fw={current === ROOT ? 700 : 400} onClick={() => switchTo(ROOT)}>
                    {t('header.rootNamespace')}
                </Menu.Item>
                {namespaces?.map((ns) => (
                    <Menu.Item fw={current === ns ? 700 : 400} key={ns} onClick={() => switchTo(ns)}>
                        {ns}
                    </Menu.Item>
                ))}
            </Menu.Dropdown>
        </Menu>
    )
}
