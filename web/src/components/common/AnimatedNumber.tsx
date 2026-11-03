import { useEffect, useState } from 'react'
import { useReducedMotion, useSpring } from 'framer-motion'
import { GAUGE_SPRING } from '@/lib/motion'

/**
 * AnimatedNumber eases a rapidly-changing gauge value with the spec's spring
 * (stiffness 100 / damping 20) so 500 ms metric ticks read as one smooth needle
 * instead of a stuttering number. Reduced motion snaps to the value.
 */
export function AnimatedNumber({
  value,
  format,
  className,
}: {
  value: number
  format: (value: number) => string
  className?: string
}) {
  const reduce = useReducedMotion()
  const spring = useSpring(value, reduce ? { stiffness: 600, damping: 60 } : GAUGE_SPRING)
  const [display, setDisplay] = useState(value)

  useEffect(() => {
    if (reduce) {
      spring.jump(value)
      setDisplay(value)
      return
    }
    spring.set(value)
  }, [spring, value, reduce])

  useEffect(() => {
    const unsubscribe = spring.on('change', (latest) => setDisplay(latest))
    return unsubscribe
  }, [spring])

  return <span className={className}>{format(display)}</span>
}
