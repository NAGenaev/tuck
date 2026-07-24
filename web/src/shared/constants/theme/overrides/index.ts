// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
// "charts" override dropped (Tuck doesn't use @mantine/charts yet); "DateTimePicker"
// entry dropped from inputs (no @mantine/dates dependency yet) — add both back
// together if/when a page needs them.
import badge from './badge'
import breadcrumbs from './breadcrumbs'
import buttons from './buttons'
import card from './card'
import drawer from './drawer'
import fieldset from './fieldset'
import inputs from './inputs'
import layouts from './layouts'
import loadingOverlay from './loading-overlay'
import menu from './menu'
import modal from './modal'
import notification from './notification'
import ringProgress from './ring-progress'
import segmentedControl from './segmented-control'
import table from './table'
import tooltip from './tooltip'

export default {
    ...fieldset,
    ...card,
    ...drawer,
    ...modal,
    ...badge,
    ...breadcrumbs,
    ...buttons,
    ...inputs,
    ...loadingOverlay,
    ...menu,
    ...notification,
    ...ringProgress,
    ...segmentedControl,
    ...table,
    ...tooltip,
    ...layouts
}
