export const MIN_FIT_CELL = 3

// fitCell is the largest whole-pixel cell size that fits the whole map in the
// available space (never below MIN_FIT_CELL).
export function fitCell(availW: number, availH: number, cols: number, rows: number): number {
  const c = Math.floor(Math.min(availW / Math.max(1, cols), availH / Math.max(1, rows)))
  return Math.max(MIN_FIT_CELL, Number.isFinite(c) ? c : MIN_FIT_CELL)
}

// cellFromPoint maps a pixel to a grid cell, or null outside the grid.
export function cellFromPoint(
  px: number, py: number, cell: number, offsetX: number, offsetY: number, cols: number, rows: number,
): { x: number; y: number } | null {
  const fx = (px - offsetX) / cell
  const fy = (py - offsetY) / cell
  if (!(fx >= 0 && fy >= 0 && fx < cols && fy < rows)) return null
  return { x: Math.floor(fx), y: Math.floor(fy) }
}
