// SPDX-License-Identifier: AGPL-3.0-only
import { Checkbox, Pagination, Table } from '@mantine/core'
import { ReactNode, useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyableValue } from '../copy/copy'
import { cx } from '../cx'
import { Icon } from '../icons'
import styles from './data-table.module.css'

export type SortDir = 'asc' | 'desc'

/**
 * Client-side sorting state. `accessors` maps a column key to the comparable value of a row;
 * keys without an accessor read `row[key]`. Numbers compare numerically, everything else with
 * localeCompare. Bind with `<SortableTh {...sortProps('name')}>`.
 */
export function useSort<T>(
    rows: T[],
    initial: { dir?: SortDir; key: string },
    accessors: Record<string, (row: T) => number | string> = {}
) {
    const [key, setKey] = useState(initial.key)
    const [dir, setDir] = useState<SortDir>(initial.dir ?? 'asc')

    const sorted = useMemo(() => {
        const get = (r: T): number | string =>
            accessors[key] ? accessors[key](r) : (((r as Record<string, unknown>)[key] as number | string) ?? '')
        const out = [...rows].sort((a, b) => {
            const x = get(a)
            const y = get(b)
            const cmp = typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y))
            return dir === 'asc' ? cmp : -cmp
        })
        return out
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [rows, key, dir])

    const toggle = useCallback(
        (k: string) => {
            if (k === key) setDir((d) => (d === 'asc' ? 'desc' : 'asc'))
            else {
                setKey(k)
                setDir('asc')
            }
        },
        [key]
    )

    const sortProps = (k: string) => ({
        dir: k === key ? dir : undefined,
        onSort: () => toggle(k)
    })

    return { dir, sortKey: key, sortProps, sorted, toggle }
}

/** Table header cell that sorts on click / Enter / Space and shows an arrow. */
export function SortableTh({
    children,
    dir,
    onSort,
    className
}: {
    children: ReactNode
    className?: string
    /** current direction when this column is the sorted one, else undefined */
    dir?: SortDir
    onSort: () => void
}) {
    return (
        <Table.Th
            aria-sort={dir ? (dir === 'asc' ? 'ascending' : 'descending') : 'none'}
            className={cx(styles.sortable, className)}
            data-sorted={dir || undefined}
            onClick={onSort}
            onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    onSort()
                }
            }}
            tabIndex={0}
        >
            {children}
            <span aria-hidden className={styles.arrow} data-dir={dir}>
                <Icon name="up" size={11} />
            </span>
        </Table.Th>
    )
}

/** Row selection by id. `all` is the ids currently visible (the page), `selected` can span pages. */
export function useRowSelection<Id extends number | string>(all: Id[]) {
    const [selected, setSelected] = useState<Set<Id>>(new Set())

    const toggle = useCallback(
        (id: Id) =>
            setSelected((prev) => {
                const next = new Set(prev)
                if (next.has(id)) next.delete(id)
                else next.add(id)
                return next
            }),
        []
    )

    const onPage = all.filter((id) => selected.has(id)).length
    const toggleAll = useCallback(
        () =>
            setSelected((prev) => {
                const next = new Set(prev)
                const every = all.length > 0 && all.every((id) => next.has(id))
                all.forEach((id) => (every ? next.delete(id) : next.add(id)))
                return next
            }),
        [all]
    )

    return {
        allChecked: all.length > 0 && onPage === all.length,
        clear: () => setSelected(new Set()),
        isSelected: (id: Id) => selected.has(id),
        selected,
        someChecked: onPage > 0 && onPage < all.length,
        toggle,
        toggleAll
    }
}

/** Header checkbox cell for "select all on this page". */
export function SelectAllTh({
    checked,
    indeterminate,
    onChange
}: {
    checked: boolean
    indeterminate?: boolean
    onChange: () => void
}) {
    const { t } = useTranslation('forms')
    return (
        <Table.Th className={styles.ck}>
            <Checkbox
                aria-label={t('table.selectAll')}
                checked={checked}
                indeterminate={indeterminate}
                onChange={onChange}
            />
        </Table.Th>
    )
}

/** Row checkbox cell. Give the parent <Table.Tr> `data-selected={checked || undefined}`. */
export function SelectCell({
    checked,
    onChange,
    label
}: {
    checked: boolean
    label?: string
    onChange: () => void
}) {
    const { t } = useTranslation('forms')
    return (
        <Table.Td className={styles.ck} onClick={(e) => e.stopPropagation()}>
            <Checkbox aria-label={label ?? t('table.selectRow')} checked={checked} onChange={onChange} />
        </Table.Td>
    )
}

/** Mono cell value: truncated with an ellipsis, full value in the tooltip, click copies it. */
export function MonoCell({
    value,
    maxWidth = 220,
    copy = true
}: {
    /** disable the copy-on-click affordance for plain truncation */
    copy?: boolean
    maxWidth?: number | string
    value: string
}) {
    if (copy) return <CopyableValue maxWidth={maxWidth} value={value} />
    return (
        <span className={styles.trunc} style={{ maxWidth }} title={value}>
            {value}
        </span>
    )
}

/** Bar that appears above a table while rows are selected; children are the bulk actions. */
export function SelectionBar({ count, children }: { children?: ReactNode; count: number }) {
    const { t } = useTranslation('forms')
    if (count === 0) return null
    return (
        <div className={styles.selBar} role="status">
            <span>{t('table.selected', { count })}</span>
            {children}
        </div>
    )
}

/** "Showing 1-5 of 12" plus numbered pagination. Hidden when everything fits on one page. */
export function TablePager({
    page,
    perPage,
    total,
    onChange
}: {
    onChange: (page: number) => void
    /** 1-based */
    page: number
    perPage: number
    total: number
}) {
    const { t } = useTranslation('forms')
    const pages = Math.max(1, Math.ceil(total / perPage))
    if (total <= perPage) return null
    const from = (page - 1) * perPage + 1
    const to = Math.min(page * perPage, total)
    return (
        <div className={styles.tfoot}>
            <span>{t('table.showing', { from, to, total })}</span>
            <Pagination onChange={onChange} size="sm" total={pages} value={page} withEdges={false} />
        </div>
    )
}
