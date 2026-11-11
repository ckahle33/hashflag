import type { Entry } from '../protocol'

export function Leaderboard({ title, entries, limit = 10 }: { title: string; entries: Entry[]; limit?: number }) {
  const rows = (entries ?? []).slice(0, limit)
  return (
    <section aria-label={title}>
      <h3>{title}</h3>
      {rows.length === 0 ? (
        <p className="muted">No captures yet.</p>
      ) : (
        <ol>
          {rows.map((e) => (
            <li key={e.name}>
              {e.name} <span className="muted">{e.captures}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
