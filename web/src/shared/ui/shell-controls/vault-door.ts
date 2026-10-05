// SPDX-License-Identifier: AGPL-3.0-only
import styles from './door.module.css'

const SVG =
    '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18"/><circle cx="12" cy="12" r="4"/><path d="M12 3v5M12 16v5M3 12h5M16 12h5"/></svg>'

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

/**
 * "Vault door" entry transition. It is built imperatively and appended to
 * <body> so that it survives the route change that happens while it is shut.
 * `shut()` resolves after ~560ms (doors closed) — navigate then — and
 * `open()` slides the doors away and removes the overlay.
 */
export function vaultDoor(label: string) {
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const el = document.createElement('div')
    el.className = styles.door
    el.setAttribute('aria-hidden', 'true')
    el.innerHTML = `<i class="${styles.l}"></i><i class="${styles.r}"></i>${SVG}<div class="${styles.lbl}"></div>`
    el.querySelector(`.${styles.lbl}`)!.textContent = label
    document.body.appendChild(el)

    return {
        async shut() {
            await sleep(30)
            el.classList.add(styles.shut)
            await sleep(reduced ? 30 : 540)
        },
        async open() {
            await sleep(reduced ? 30 : 180)
            el.classList.remove(styles.shut)
            await sleep(reduced ? 30 : 520)
            el.remove()
        }
    }
}
