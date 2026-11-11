// localStorage wrappers that never throw.
export function getItem(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

export function setItem(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    /* storage unavailable; carry on */
  }
}

export function removeItem(key: string): void {
  try {
    window.localStorage.removeItem(key)
  } catch {
    /* ignore */
  }
}

// sessionStorage wrappers that never throw. Unlike localStorage this is
// private to one tab, which is what the session token needs.
export function getSessionItem(key: string): string | null {
  try {
    return window.sessionStorage.getItem(key)
  } catch {
    return null
  }
}

export function setSessionItem(key: string, value: string): void {
  try {
    window.sessionStorage.setItem(key, value)
  } catch {
    /* storage unavailable; carry on */
  }
}

export function removeSessionItem(key: string): void {
  try {
    window.sessionStorage.removeItem(key)
  } catch {
    /* ignore */
  }
}
