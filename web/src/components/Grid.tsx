import { useEffect, useRef, useState } from 'react'
import type { MapInfo, StateMsg } from '../protocol'
import { cellSizeFor, drawCanvas } from '../draw'

interface Props {
  map: MapInfo
  state: StateMsg
  youId: string
  ascii: boolean
}

export function Grid({ map, state, youId, ascii }: Props) {
  const wrap = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const [width, setWidth] = useState(800)

  useEffect(() => {
    const el = wrap.current
    if (!el) return
    const update = () => setWidth(el.clientWidth || 800)
    update()
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [])

  const cell = cellSizeFor(width, map.width)
  const dpr = window.devicePixelRatio || 1

  useEffect(() => {
    const c = canvas.current
    const ctx = c?.getContext('2d')
    if (!c || !ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    drawCanvas(ctx, map, state, youId, cell, ascii)
  }, [map, state, youId, cell, ascii, dpr])

  return (
    <div ref={wrap} className="gridwrap">
      <canvas
        ref={canvas}
        width={map.width * cell * dpr}
        height={map.height * cell * dpr}
        style={{ width: map.width * cell, height: map.height * cell }}
        role="img"
        aria-label="Game map. Use the HUD text for scores and status."
      />
    </div>
  )
}
