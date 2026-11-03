import { useEffect, useRef, useState } from 'react'
import { Area, AreaChart, ResponsiveContainer } from 'recharts'

interface Point {
  i: number
  down: number
  up: number
}

/**
 * SpeedSparkline keeps a rolling 60-sample window (the metrics tick is 500 ms,
 * so this covers roughly 30 seconds) and draws ingress vs confirmed egress.
 */
export function SpeedSparkline({ down, up, active }: { down: number; up: number; active: boolean }) {
  const [points, setPoints] = useState<Point[]>([])
  const index = useRef(0)

  useEffect(() => {
    index.current += 1
    setPoints((prev) => {
      const next = [...prev, { i: index.current, down, up }]
      return next.length > 60 ? next.slice(next.length - 60) : next
    })
  }, [down, up])

  return (
    <div className="h-12 w-full">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={points} margin={{ top: 2, right: 0, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id="downFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#38bdf8" stopOpacity={0.35} />
              <stop offset="100%" stopColor="#38bdf8" stopOpacity={0} />
            </linearGradient>
            <linearGradient id="upFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#34d399" stopOpacity={0.3} />
              <stop offset="100%" stopColor="#34d399" stopOpacity={0} />
            </linearGradient>
          </defs>
          <Area
            type="monotone"
            dataKey="down"
            stroke="#38bdf8"
            strokeWidth={1.5}
            fill="url(#downFill)"
            isAnimationActive={false}
            dot={false}
          />
          <Area
            type="monotone"
            dataKey="up"
            stroke="#34d399"
            strokeWidth={1.5}
            fill="url(#upFill)"
            isAnimationActive={false}
            dot={false}
          />
        </AreaChart>
      </ResponsiveContainer>
      {!active && points.length === 0 ? <div className="h-12" /> : null}
    </div>
  )
}
