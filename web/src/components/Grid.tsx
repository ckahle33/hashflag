import { useEffect, useRef, useState } from 'react'
import type { PointerEvent as ReactPointerEvent } from 'react'
import type { MapInfo, StateMsg } from '../protocol'
import { drawCanvas } from '../draw'
import { cellFromPoint, fitCell } from '../layout'
import { aimAction } from '../input'

export type AimPhase = 'down' | 'move' | 'up'

interface Props {
  map: MapInfo
  state: StateMsg
  youId: string
  ascii: boolean
  // Pointer aiming: the grid cell under the pointer (null when outside).
  onAim: (phase: AimPhase, cell: { x: number; y: number } | null, shift: boolean) => void
}

export function Grid({ map, state, youId, ascii, onAim }: Props) {
  const wrap = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const pressed = useRef(false)
  const [avail, setAvail] = useState({ w: 800, h: 600 })
  const [dpr, setDpr] = useState(() => window.devicePixelRatio || 1)

  useEffect(() => {
    const el = wrap.current
    if (!el) return
    const update = () => {
      setAvail({ w: el.clientWidth, h: el.clientHeight })
      setDpr(window.devicePixelRatio || 1)
    }
    update()
    const ro = new ResizeObserver(update)
    ro.observe(el)
    window.addEventListener('resize', update)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', update)
    }
  }, [])

  const cell = fitCell(avail.w, avail.h, map.width, map.height)

  useEffect(() => {
    const c = canvas.current
    const ctx = c?.getContext('2d')
    if (!c || !ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    drawCanvas(ctx, map, state, youId, cell, ascii)
  }, [map, state, youId, cell, ascii, dpr])

  const cellAt = (e: ReactPointerEvent<HTMLCanvasElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    return cellFromPoint(e.clientX - r.left, e.clientY - r.top, cell, 0, 0, map.width, map.height)
  }
  const release = (e?: ReactPointerEvent<HTMLCanvasElement>) => {
    if (e) {
      try { e.currentTarget.releasePointerCapture(e.pointerId) } catch { /* not captured */ }
    }
    if (!pressed.current) return
    pressed.current = false
    onAim('up', null, false)
  }
  const releaseRef = useRef(release)
  releaseRef.current = release

  // A press must not outlive the window losing focus or the tab hiding.
  useEffect(() => {
    const stop = () => releaseRef.current()
    const onVis = () => {
      if (document.visibilityState === 'hidden') stop()
    }
    window.addEventListener('blur', stop)
    document.addEventListener('visibilitychange', onVis)
    return () => {
      window.removeEventListener('blur', stop)
      document.removeEventListener('visibilitychange', onVis)
    }
  }, [])

  return (
    <div ref={wrap} className="gridwrap">
      <canvas
        ref={canvas}
        width={Math.round(map.width * cell * dpr)}
        height={Math.round(map.height * cell * dpr)}
        style={{ width: map.width * cell, height: map.height * cell }}
        role="img"
        aria-label="Game map. Use the HUD text for scores and status."
        onContextMenu={(e) => e.preventDefault()}
        onPointerDown={(e) => {
          if (e.button !== 0) return
          const c = cellAt(e)
          if (!c) return
          try { e.currentTarget.setPointerCapture(e.pointerId) } catch { /* unsupported */ }
          pressed.current = true
          onAim('down', c, e.shiftKey)
        }}
        onPointerMove={(e) => {
          const a = aimAction(e.buttons, pressed.current)
          if (a === 'release') release(e)
          else if (a === 'aim') onAim('move', cellAt(e), e.shiftKey)
        }}
        onPointerUp={release}
        onPointerCancel={release}
        onPointerLeave={() => release()}
      />
    </div>
  )
}
