// SPDX-License-Identifier: AGPL-3.0-only

/** Joins class names, dropping falsy parts. */
export const cx = (...parts: Array<false | null | string | undefined>) => parts.filter(Boolean).join(' ')

/** True when the user asked the OS for reduced motion. Safe on the server and in tests. */
export const prefersReducedMotion = () =>
    typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
