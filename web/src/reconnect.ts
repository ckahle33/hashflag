// The close reason the server sends when another connection took this
// session over (see internal/server/sessions.go).
export const REPLACED_REASON = 'replaced by a newer connection'

// Whether to reconnect automatically after a close. A replaced connection
// must not, or two tabs sharing a session would knock each other offline.
export function shouldReconnect(_code: number, reason: string): boolean {
  return reason !== REPLACED_REASON
}
