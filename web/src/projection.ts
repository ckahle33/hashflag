// Tilted-forward (oblique) projection. The grid stays screen-aligned; rows are
// squashed to TILT of a cell width and blocks are extruded upward by BLOCK_H.
export const TILT = 0.6
export const BLOCK_H = 0.8
const MIN_CELL = 3

export function tiltedSize(cols: number, rows: number, cell: number): { w: number; h: number } {
  return { w: cols * cell, h: Math.ceil(rows * cell * TILT + cell * BLOCK_H) }
}

// Top-left of the cell's floor tile.
export function cellToScreen(x: number, y: number, cell: number): { px: number; py: number } {
  return { px: x * cell, py: cell * BLOCK_H + y * cell * TILT }
}

export function fitCellTilted(availW: number, availH: number, cols: number, rows: number): number {
  const c = Math.max(1, cols), r = Math.max(1, rows)
  let cell = Math.floor(availW / c)
  if (!Number.isFinite(cell)) return MIN_CELL
  while (cell > MIN_CELL && tiltedSize(c, r, cell).h > availH) cell--
  return Math.max(MIN_CELL, cell)
}

// Inverse onto the floor plane; null outside the grid.
export function cellFromPointTilted(
  px: number, py: number, cell: number, cols: number, rows: number,
): { x: number; y: number } | null {
  const fx = px / cell
  const fy = (py - cell * BLOCK_H) / (cell * TILT)
  if (!(fx >= 0 && fy >= 0 && fx < cols && fy < rows)) return null
  return { x: Math.floor(fx), y: Math.floor(fy) }
}

// True when a block in a nearer row would hide the local player.
export function occludes(block: { x: number; y: number }, you: { x: number; y: number }): boolean {
  const dy = block.y - you.y
  return dy >= 1 && dy <= 2 && Math.abs(block.x - you.x) <= 1
}
