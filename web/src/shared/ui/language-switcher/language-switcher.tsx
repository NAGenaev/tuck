import { ActionIcon, Menu, Tooltip } from '@mantine/core'
import { useTranslation } from 'react-i18next'
import { TbLanguage } from 'react-icons/tb'

const LANGUAGES = [
    { code: 'en', label: 'English' },
    { code: 'ru', label: 'Русский' }
]

export function LanguageSwitcher() {
    const { t, i18n } = useTranslation()

    return (
        <Menu position="bottom-end" width={140}>
            <Menu.Target>
                <Tooltip label={t('header.language')}>
                    <ActionIcon color="gray" radius="xl" size="lg" variant="light">
                        <TbLanguage size={18} />
                    </ActionIcon>
                </Tooltip>
            </Menu.Target>
            <Menu.Dropdown>
                {LANGUAGES.map((lang) => (
                    <Menu.Item
                        fw={i18n.resolvedLanguage === lang.code ? 700 : 400}
                        key={lang.code}
                        onClick={() => i18n.changeLanguage(lang.code)}
                    >
                        {lang.label}
                    </Menu.Item>
                ))}
            </Menu.Dropdown>
        </Menu>
    )
}
