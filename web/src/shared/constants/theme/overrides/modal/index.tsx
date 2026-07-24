// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Modal } from '@mantine/core'

import classes from './modal.module.css'

export default {
    Modal: Modal.extend({
        classNames: {
            root: classes.modalRoot,
            header: classes.modalHeader,
            body: classes.modalBody,
            content: classes.modalContent
        },
        defaultProps: {
            radius: 'md',
            centered: true
        }
    })
}
