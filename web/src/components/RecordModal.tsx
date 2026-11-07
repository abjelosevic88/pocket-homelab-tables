// One row, all its fields, its story and what it has in the other apps.
import { useCallback, useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { computed, typeInfo, rowTitle } from '../lib/fields'
import type { Field, HistoryLine, Made, Row, RowPage } from '../lib/types'
import { ago, fmtDateTime } from '../lib/format'
import { useAppState, announceChange } from '../lib/data'
import { CellView, ValueEditor } from './Cell'
import { Icon } from './icons'
import { Confirm, Loading, type MenuItem, MenuButton, Modal, copyText, errMsg, useToast } from './ui'

const APP_NAMES: Record<string, string> = { tasks: 'Pocket Tasks', notes: 'Pocket Notes', calendar: 'Pocket Calendar', forms: 'Pocket Forms', contacts: 'Pocket Contacts', flows: 'Pocket Flows' }

export function RecordModal({ id, onClose, onChanged, onOpen, onFieldsChanged }: { id: number; onClose: () => void; onChanged?: (r: Row | null) => void; onOpen?: (id: number) => void; onFieldsChanged?: () => void }) {
  const [page, setPage] = useState<RowPage | null>(null)
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState('')
  const [confirmDel, setConfirmDel] = useState(false)
  const [make, setMake] = useState<'' | 'task' | 'note' | 'event'>('')
  const { toast } = useToast()
  const appState = useAppState()
  const load = useCallback(async () => {
    try { setPage(await api.get<RowPage>(`${V1}/records/${id}`)) } catch (e) { setErr(errMsg(e)) }
  }, [id])
  useEffect(() => { setPage(null); load() }, [load])

  const set = async (f: Field, v: unknown, typecast = false) => {
    if (!page) return
    setSaving(f.name)
    try {
      const r = await api.put<Row>(`${V1}/records/${id}`, { values: { [f.id]: v }, typecast })
      setPage(p => p && { ...p, record: { ...p.record, ...r, named: p.record.named } })
      onChanged?.(r)
      if (typecast) { onFieldsChanged?.(); load() }
      else if (f.type === 'link') load()
    } catch (e) { toast(errMsg(e), 'err') } finally { setSaving('') }
  }

  const del = async () => {
    try { await api.del(`${V1}/records/${id}`); onChanged?.(null); toast('Moved to the bin — it stays there 30 days'); announceChange(); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  const dup = async () => {
    try { const r = await api.post<Row>(`${V1}/records/${id}/duplicate`); onChanged?.(r); toast('Copied'); onOpen?.(r.id) } catch (e) { toast(errMsg(e), 'err') }
  }

  // ←/→ (with alt) go to the row before and after
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (!page || !e.altKey) return
      if (e.key === 'ArrowLeft' && page.prev) onOpen?.(page.prev)
      if (e.key === 'ArrowRight' && page.next) onOpen?.(page.next)
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [page, onOpen])

  const tasks = appState('tasks', 'tasks:write'), notes = appState('notes', 'notes:write'), cal = appState('calendar', 'calendar:write')
  const menu = (): MenuItem[] => [
    { heading: 'Send to your apps' },
    { label: 'Make a task', icon: 'check-square', disabled: !tasks.ok, hint: tasks.ok ? undefined : 'off', onSelect: () => setMake('task') },
    { label: 'Put on the calendar', icon: 'calendar-plus', disabled: !cal.ok, hint: cal.ok ? undefined : 'off', onSelect: () => setMake('event') },
    { label: 'Save as a note', icon: 'note', disabled: !notes.ok, hint: notes.ok ? undefined : 'off', onSelect: () => setMake('note') },
    'sep',
    { label: 'Copy a link to this row', icon: 'link', onSelect: async () => { await copyText(`${location.origin}/r/${id}`); toast('Link copied') } },
    { label: 'Duplicate', icon: 'copy', onSelect: dup },
    { label: 'Delete', icon: 'trash', danger: true, onSelect: () => setConfirmDel(true) },
  ]
  const deleted = !!page?.record.deleted_at
  return (
    <Modal bare onClose={onClose} size="lg" className="rec-modal" labelledBy="rec-title">
      {!page ? (err ? <div className="pad-card"><p className="bad">{err}</p><button className="btn" onClick={onClose}>Close</button></div> : <div className="pad-card"><Loading /></div>) : <>
        <header className="rec-head">
          <div className="grow">
            <div className="muted small">{page.table.icon} {page.table.name} · row {page.record.number}{saving && <span className="save-state"> · saving…</span>}</div>
            <h2 id="rec-title" className="rec-title">{rowTitle(page.record)}</h2>
          </div>
          <button type="button" className="btn ghost icon-only" disabled={!page.prev} onClick={() => page.prev && onOpen?.(page.prev)} aria-label="The row before" title="The row before (alt ←)"><Icon name="chevron-up" size={18} /></button>
          <button type="button" className="btn ghost icon-only" disabled={!page.next} onClick={() => page.next && onOpen?.(page.next)} aria-label="The row after" title="The row after (alt →)"><Icon name="chevron-down" size={18} /></button>
          {!deleted && <MenuButton label="More" align="right" width={250} items={menu}><Icon name="more" size={18} /></MenuButton>}
          <button type="button" className="btn ghost icon-only" onClick={onClose} aria-label="Close"><Icon name="x" size={18} /></button>
        </header>
        {deleted && <div className="rec-bin"><Icon name="bin" size={15} />This row is in the bin.<button type="button" className="btn sm" onClick={async () => { await api.post(`${V1}/records/${id}/restore`); announceChange(); load(); onChanged?.(null) }}>Bring it back</button></div>}
        <div className="rec-body">
          <div className="rec-fields">
            {page.fields.map(f => (
              <div key={f.id} className={`rec-field ${computed(f.type) ? 'ro' : ''}`}>
                <label className="rec-label"><Icon name={typeInfo(f.type).icon} size={13} />{f.name}{f.description && <span className="muted small" title={f.description}> · {f.description}</span>}</label>
                <div className="rec-value">
                  {computed(f.type) || deleted
                    ? <div className="rec-ro"><CellView f={f} v={page.record.values[f.id]} wrap />{(page.record.values[f.id] === undefined) && <span className="faint">—</span>}</div>
                    : <ValueEditor form f={f} v={page.record.values[f.id]} onChange={(v, tc) => set(f, v, tc)} />}
                </div>
              </div>
            ))}
            {page.made.length > 0 && <div className="rec-field">
              <label className="rec-label"><Icon name="layers" size={13} />In your other apps</label>
              <div className="made-list">{page.made.map(m => <MadeChip key={m.id} m={m} />)}</div>
            </div>}
            <p className="muted small">Added {fmtDateTime(page.record.created_at)}{page.record.source && page.record.source !== 'tables' ? ` by ${APP_NAMES[page.record.source] || page.record.source}` : ''} · changed {ago(page.record.updated_at)}</p>
          </div>
          <Story id={id} lines={page.history} onAdded={load} readOnly={deleted} />
        </div>
        {make && <MakeModal kind={make} row={page} onClose={() => setMake('')} onMade={() => { setMake(''); load() }} />}
        {confirmDel && <Confirm title="Delete this row?" message="It goes to the bin, where it stays 30 days. Its events in Pocket Calendar are taken away." confirmLabel="Delete" onConfirm={() => { setConfirmDel(false); del() }} onCancel={() => setConfirmDel(false)} />}
      </>}
    </Modal>
  )
}

function MadeChip({ m }: { m: Made }) {
  const icon = m.target === 'task' ? 'check-square' : m.target === 'note' ? 'note' : 'calendar'
  const what = m.target === 'sync' ? 'In the calendar' : m.target === 'task' ? (m.state === 'done' ? 'Task done' : 'Task') : m.target === 'note' ? 'Note' : 'Event'
  const body = <><Icon name={icon} size={13} />{what}: <span className="ellipsis">{m.title}</span>{m.state === 'done' && <Icon name="check" size={12} />}</>
  return m.url ? <a className="made-chip" href={m.url} target="_blank" rel="noreferrer">{body}</a> : <span className="made-chip">{body}</span>
}

function Story({ id, lines, onAdded, readOnly }: { id: number; lines: HistoryLine[]; onAdded: () => void; readOnly: boolean }) {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const add = async () => {
    if (!text.trim()) return
    setBusy(true)
    try { await api.post(`${V1}/records/${id}/comments`, { text }); setText(''); onAdded() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <aside className="rec-story" aria-label="Comments and changes">
      <h3 className="rec-story-h">Comments and changes</h3>
      {!readOnly && <div className="rec-comment">
        <textarea rows={2} placeholder="Write a comment…" value={text} onChange={e => setText(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) add() }} />
        <button type="button" className="btn sm primary" onClick={add} disabled={busy || !text.trim()}>Comment</button>
      </div>}
      <ul className="story">
        {lines.map(h => (
          <li key={h.id} className={`story-line k-${h.kind}`}>
            {h.kind === 'comment' && <><div className="story-text pre">{h.text}</div>{h.app && <span className="muted small">from {APP_NAMES[h.app] || h.app} · </span>}</>}
            {h.kind === 'created' && <div className="muted">Added{h.app ? ` by ${APP_NAMES[h.app] || h.app}` : ''}</div>}
            {h.kind === 'restored' && <div className="muted">Brought back from the bin</div>}
            {h.kind === 'made' && <div>{h.data?.url ? <a href={h.data.url} target="_blank" rel="noreferrer">{h.text}</a> : h.text}</div>}
            {h.kind === 'change' && <div className="story-changes">{(h.data.changes || []).map((c, i) => (
              <div key={i}><b>{c.field}</b>{c.before && <> <span className="was">{c.before}</span> →</>} {c.after || <span className="muted">cleared</span>}</div>
            ))}{h.data.ai && <span className="muted small"> (filled by AI)</span>}{h.app && <span className="muted small"> by {APP_NAMES[h.app] || h.app}</span>}</div>}
            <span className="muted small" title={fmtDateTime(h.created_at)}>{ago(h.created_at)}</span>
          </li>
        ))}
      </ul>
    </aside>
  )
}

/** Make a task, an event or a note from a row. */
function MakeModal({ kind, row, onClose, onMade }: { kind: 'task' | 'note' | 'event'; row: RowPage; onClose: () => void; onMade: () => void }) {
  const firstDate = row.fields.find(f => f.type === 'date' && row.record.values[f.id])
  const dateOf = (f?: Field) => (f ? String(row.record.values[f.id] || '') : '')
  const due0 = dateOf(row.fields.find(f => f.id === row.table.settings.task_due) || firstDate)
  const [title, setTitle] = useState(rowTitle(row.record))
  const [date, setDate] = useState(due0.slice(0, 10))
  const [time, setTime] = useState(due0.length > 10 ? due0.slice(11, 16) : '')
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const go = async () => {
    setBusy(true)
    try {
      const m = await api.post<Made>(`${V1}/records/${row.record.id}/make`, { target: kind, title, due: kind === 'task' ? date : '', date, time: kind === 'event' ? time : '', notes })
      toast(kind === 'task' ? 'The task is in Pocket Tasks' : kind === 'event' ? 'The event is in Pocket Calendar' : 'The note is in Pocket Notes', 'ok', m.url ? { label: 'Open', run: () => window.open(m.url, '_blank') } : undefined)
      onMade()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const head = kind === 'task' ? 'A task in Pocket Tasks' : kind === 'event' ? 'An event in Pocket Calendar' : 'A note in Pocket Notes'
  return (
    <Modal title={head} onClose={onClose} size="sm" footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn primary" onClick={go} disabled={busy || (kind === 'event' && !date)}>Make it</button></>}>
      <div className="stack">
        <label className="field"><span className="label">Title</span><input value={title} onChange={e => setTitle(e.target.value)} /></label>
        {kind !== 'note' && <div className="row">
          <label className="field grow"><span className="label">{kind === 'task' ? 'Due' : 'Day'}</span><input type="date" value={date} onChange={e => setDate(e.target.value)} /></label>
          {kind === 'event' && <label className="field"><span className="label">Time (optional)</span><input type="time" value={time} onChange={e => setTime(e.target.value)} /></label>}
        </div>}
        <label className="field"><span className="label">Add a line (optional)</span><textarea rows={2} value={notes} onChange={e => setNotes(e.target.value)} /></label>
        <p className="muted small">The row’s fields and a link back to it go along.</p>
      </div>
    </Modal>
  )
}
