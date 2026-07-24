// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Combobox, Menu } from '@mantine/core'

export default {
    Menu: Menu.extend({
        defaultProps: {
            shadow: 'lg',
            withArrow: false,
            radius: 'md',
            transitionProps: {
                transition: 'fade',
                duration: 180,
                timingFunction: 'ease-out'
            },
            styles: {
                dropdown: {
                    backgroundColor: 'var(--mantine-color-dark-6)',
                    border: '1px solid var(--mantine-color-dark-4)',
                    boxShadow: '0 8px 32px rgba(0, 0, 0, 0.4), 0 2px 8px rgba(0, 0, 0, 0.2)'
                },
                divider: {
                    borderColor: 'var(--mantine-color-dark-5)',
                    margin: '4px 0'
                }
            }
        }
    }),
    Combobox: Combobox.extend({
        defaultProps: {
            transitionProps: { transition: 'fade', duration: 200 }
        }
    })
}
