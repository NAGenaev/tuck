// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
// Restyled after the "Anatomy of Tuck" forms kit. "charts" override dropped (Tuck doesn't
// use @mantine/charts yet); "DateTimePicker" entry dropped from inputs (no @mantine/dates
// dependency yet) — add both back together if/when a page needs them.
import breadcrumbs from './breadcrumbs'
import buttons from './buttons'
import card from './card'
import controls from './controls'
import feedback from './feedback'
import fieldset from './fieldset'
import inputs from './inputs'
import layouts from './layouts'
import loadingOverlay from './loading-overlay'
import navigation from './navigation'
import overlays from './overlays'
import table from './table'

export default {
    ...fieldset,
    ...card,
    ...breadcrumbs,
    ...buttons,
    ...controls,
    ...feedback,
    ...inputs,
    ...loadingOverlay,
    ...navigation,
    ...overlays,
    ...table,
    ...layouts
}
