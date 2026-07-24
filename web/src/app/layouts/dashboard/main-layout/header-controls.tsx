import { ActionIcon, Group, Tooltip } from '@mantine/core'
import { useFullscreenDocument } from '@mantine/hooks'
import { useTranslation } from 'react-i18next'
import {
    TbBrandGithub,
    TbLock,
    TbLockOpen,
    TbLogout,
    TbMaximize,
    TbMinimize,
    TbTag
} from 'react-icons/tb'

import { useHealth } from '@shared/api/hooks/use-health'
import { useSealStatus } from '@shared/api/hooks/use-seal-status'
import { useLogout } from '@shared/hooks/use-auth'
import { LanguageSwitcher } from '@shared/ui/language-switcher/language-switcher'

const TUCK_REPO_URL = 'https://github.com/NAGenaev/tuck'

export function HeaderControls() {
    const { t } = useTranslation()
    const { data: sealStatus } = useSealStatus()
    const { data: health } = useHealth()
    const { fullscreen, toggle } = useFullscreenDocument()
    const logout = useLogout()

    return (
        <Group gap="xs">
            <Tooltip label={t('header.version', { version: health?.version ?? '—' })}>
                <ActionIcon color="blue" radius="xl" size="lg" variant="light">
                    <TbTag size={18} />
                </ActionIcon>
            </Tooltip>

            <Tooltip label={sealStatus?.sealed ? t('header.sealed') : t('header.unsealed')}>
                <ActionIcon
                    color={sealStatus?.sealed ? 'red' : 'teal'}
                    radius="xl"
                    size="lg"
                    variant="light"
                >
                    {sealStatus?.sealed ? <TbLock size={18} /> : <TbLockOpen size={18} />}
                </ActionIcon>
            </Tooltip>

            <Tooltip label={fullscreen ? t('header.exitFullscreen') : t('header.fullscreen')}>
                <ActionIcon color="gray" onClick={toggle} radius="xl" size="lg" variant="light">
                    {fullscreen ? <TbMinimize size={18} /> : <TbMaximize size={18} />}
                </ActionIcon>
            </Tooltip>

            <LanguageSwitcher />

            <Tooltip label={t('header.source')}>
                <ActionIcon
                    color="gray"
                    component="a"
                    href={TUCK_REPO_URL}
                    radius="xl"
                    rel="noopener noreferrer"
                    size="lg"
                    target="_blank"
                    variant="light"
                >
                    <TbBrandGithub size={18} />
                </ActionIcon>
            </Tooltip>

            <Tooltip label={t('header.logout')}>
                <ActionIcon color="red" onClick={logout} radius="xl" size="lg" variant="light">
                    <TbLogout size={18} />
                </ActionIcon>
            </Tooltip>
        </Group>
    )
}
