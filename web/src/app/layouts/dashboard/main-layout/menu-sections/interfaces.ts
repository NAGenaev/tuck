// SPDX-License-Identifier: AGPL-3.0-only
// Interface shape forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import type { ShellIconName } from '@shared/ui/shell-controls/icons'

export interface MenuItem {
    header?: string
    icon?: ShellIconName
    id?: string
    section: {
        dropdownItems?: {
            href: string
            icon?: ShellIconName
            id: string
            name: string
        }[]
        href: string
        icon: ShellIconName
        id: string
        name: string
        newTab?: boolean
    }[]
}
