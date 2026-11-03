/** Shared motion tokens (front_spec.md §2): fast, strict, no flourishes. */

export type Bezier = [number, number, number, number]

/** The spec's curve: strong ease-out for anything entering or exiting. */
export const EASE_OUT: Bezier = [0.16, 1, 0.3, 1]
/** On-screen movement between two known positions. */
export const EASE_IN_OUT: Bezier = [0.77, 0, 0.175, 1]

/** Task cards and panels: fade-in-up by 6px. */
export const FADE_UP = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: { duration: 0.2, ease: EASE_OUT },
}

/** Queue reordering: physical, but damped — no visible overshoot. */
export const LAYOUT_SPRING = {
  type: 'spring',
  stiffness: 300,
  damping: 32,
  mass: 0.6,
} as const

/** Front spec §2: smoothing gauges with stiffness 100 / damping 20. */
export const GAUGE_SPRING = { stiffness: 100, damping: 20 } as const
