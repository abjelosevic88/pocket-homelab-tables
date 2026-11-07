// How a value looks in a cell, and how it is edited — one place for every field type.
import React, { useEffect, useRef, useState } from 'react'
import { api, upload, V1 } from '../lib/api'
import { chipStyle, choiceOf, computed, text } from '../lib/fields'
import type { Choice, Field, FileRef, LinkRef, PocketRef, Row, RowsResult } from '../lib/types'
import { useData } from '../lib/data'
import { Icon } from './icons'
import { Spinner, useDebounce, useToast } from './ui'
import { fmtBytes } from '../lib/format'

export const fileURL = (f: FileRef, dl = false) => `${V1}/files/${f.id}${dl ? '?dl=1' : ''}`
export const isImage = (f: FileRef) => f.mime.startsWith('image/') && !f.mime.includes('svg')

export function ChoicePill({ c }: { c: Choice }) {
  return <span className="cpill" style={chipStyle(c.color)}>{c.name}</span>
}

/** A cell's value shown (read-only). */
export function CellView({ f, v, wrap = false }: { f: Field; v: unknown; wrap?: boolean }) {
  if (v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0)) {
    if (f.type === 'checkbox') return <span className="cbox" aria-label="No" />
    return null
  }
  switch (f.type) {
    case 'checkbox':
      return v ? <span className="cbox on" aria-label="Yes"><Icon name="check" size={12} strokeWidth={3} /></span> : <span className="cbox" aria-label="No" />
    case 'select': {
      const c = choiceOf(f, v)
      return c ? <ChoicePill c={c} /> : null
    }
    case 'multiselect':
      return <span className={`pills ${wrap ? 'wrap' : ''}`}>{(v as string[]).map(id => { const c = choiceOf(f, id); return c ? <ChoicePill key={id} c={c} /> : null })}</span>
    case 'rating': {
      const max = f.options.max || 5
      return <span className="stars" aria-label={`${v} of ${max}`}>{Array.from({ length: max }, (_, i) => <span key={i} className={i < Number(v) ? 'on' : ''}>★</span>)}</span>
    }
    case 'link':
      return <span className={`pills ${wrap ? 'wrap' : ''}`}>{(v as LinkRef[]).map(l => <span key={l.id} className="lpill">{l.title}</span>)}</span>
    case 'pocket':
      return <span className={`pills ${wrap ? 'wrap' : ''}`}>{(v as PocketRef[]).map(l => <span key={l.app + l.ref} className="lpill pocket" title={l.sub}>{l.title}</span>)}</span>
    case 'attachment':
      return <span className="thumbs">{(v as FileRef[]).map(x => isImage(x) ? <img key={x.id} src={fileURL(x)} alt={x.name} loading="lazy" /> : <span key={x.id} className="file-chip" title={x.name}><Icon name="file" size={12} />{x.name}</span>)}</span>
    case 'url':
      return <a href={String(v)} target="_blank" rel="noreferrer" onClick={e => e.stopPropagation()} className="cell-link">{String(v).replace(/^https?:\/\/(www\.)?/, '')}</a>
    case 'email':
      return <a href={`mailto:${v}`} onClick={e => e.stopPropagation()} className="cell-link">{String(v)}</a>
    case 'phone':
      return <a href={`tel:${String(v).replace(/\s/g, '')}`} onClick={e => e.stopPropagation()} className="cell-link">{String(v)}</a>
    case 'number': case 'currency': case 'percent': case 'autonumber': case 'rollup':
      return <span className="tnum">{text(f, v)}</span>
    case 'formula':
      return <span className={typeof v === 'number' ? 'tnum' : ''}>{text(f, v)}</span>
    case 'lookup': {
      const arr = Array.isArray(v) ? v : [v]
      return <span className={`pills ${wrap ? 'wrap' : ''}`}>{arr.map((x, i) => <span key={i} className="lpill quiet">{text({ ...f, type: 'lookup' }, [x])}</span>)}</span>
    }
  }
  return <span className={wrap ? 'pre' : ''}>{text(f, v)}</span>
}

// ---------- editing ----------

export interface EditorProps {
  f: Field
  v: unknown
  onChange: (v: unknown, typecast?: boolean) => void
  /** in a cell: commit and close on Enter/blur */
  inline?: boolean
  /** in the row's form: short lists of options as pills */
  form?: boolean
  onDone?: () => void
  autoFocus?: boolean
}

/** The editor for a value; what a field type needs. */
export function ValueEditor(p: EditorProps) {
  const { f } = p
  if (computed(f.type)) return <div className="muted small">{f.type === 'formula' ? 'Worked out from the formula.' : 'Worked out by itself.'}</div>
  switch (f.type) {
    case 'longtext': return <LongInput {...p} />
    case 'checkbox': return <button type="button" className="cbox-btn" onClick={() => p.onChange(!p.v)} aria-pressed={!!p.v}><CellView f={f} v={!!p.v} />{p.v ? 'Yes' : 'No'}</button>
    case 'rating': return <RatingInput {...p} />
    case 'date': return <DateInput {...p} />
    case 'select': return <ChoiceInput {...p} />
    case 'multiselect': return <ChoiceInput {...p} multi />
    case 'link': return <LinkInput {...p} />
    case 'pocket': return <PocketInput {...p} />
    case 'attachment': return <FilesInput {...p} />
  }
  return <TextInput {...p} />
}

function TextInput({ f, v, onChange, inline, onDone, autoFocus }: EditorProps) {
  const [val, setVal] = useState(v === undefined || v === null ? '' : String(v))
  useEffect(() => { if (!inline) setVal(v === undefined || v === null ? '' : String(v)) }, [v, inline])
  const type = f.type === 'email' ? 'email' : f.type === 'url' ? 'url' : f.type === 'phone' ? 'tel' : 'text'
  const mode = ['number', 'currency', 'percent'].includes(f.type) ? 'decimal' : undefined
  const commit = () => { if (val !== (v === undefined || v === null ? '' : String(v))) onChange(val.trim() === '' ? null : val) }
  return <input className={inline ? 'cell-input' : ''} type={type} inputMode={mode} value={val} autoFocus={autoFocus} placeholder={inline ? '' : f.type === 'currency' ? f.options.currency || '' : ''}
    onChange={e => setVal(e.target.value)} onBlur={() => { commit(); onDone?.() }}
    onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); commit(); onDone?.() } if (e.key === 'Escape' && inline) { e.preventDefault(); onDone?.() } }} />
}

function LongInput({ v, onChange, inline, onDone, autoFocus }: EditorProps) {
  const [val, setVal] = useState(v === undefined || v === null ? '' : String(v))
  useEffect(() => { if (!inline) setVal(v === undefined || v === null ? '' : String(v)) }, [v, inline])
  const commit = () => { if (val !== (v ?? '')) onChange(val.trim() === '' ? null : val) }
  return <textarea rows={inline ? 6 : 4} value={val} autoFocus={autoFocus} onChange={e => setVal(e.target.value)} onBlur={() => { commit(); onDone?.() }}
    onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); commit(); onDone?.() } if (e.key === 'Escape' && inline) { e.preventDefault(); onDone?.() } }} />
}

function RatingInput({ f, v, onChange }: EditorProps) {
  const max = f.options.max || 5
  const n = Number(v) || 0
  return <span className="stars big" role="radiogroup" aria-label={f.name}>{Array.from({ length: max }, (_, i) => (
    <button key={i} type="button" className={i < n ? 'on' : ''} aria-label={`${i + 1}`} onClick={() => onChange(i + 1 === n ? null : i + 1)}>★</button>
  ))}</span>
}

function DateInput({ f, v, onChange, onDone, autoFocus }: EditorProps) {
  const s = v ? String(v) : ''
  const time = !!f.options.time
  const [d, setD] = useState(s.slice(0, 10))
  const [t, setT] = useState(s.length > 10 ? s.slice(11, 16) : '')
  useEffect(() => { setD(s.slice(0, 10)); setT(s.length > 10 ? s.slice(11, 16) : '') }, [s])
  const commit = (nd = d, nt = t) => { const out = nd ? (time && nt ? `${nd}T${nt}` : nd) : null; if ((out || '') !== s) onChange(out) }
  const today = new Date()
  const iso = (x: Date) => `${x.getFullYear()}-${String(x.getMonth() + 1).padStart(2, '0')}-${String(x.getDate()).padStart(2, '0')}`
  return (
    <div className="date-edit">
      <div className="row">
        <input type="date" value={d} autoFocus={autoFocus} onChange={e => { setD(e.target.value); commit(e.target.value, t) }} />
        {time && <input type="time" value={t} onChange={e => { setT(e.target.value); commit(d, e.target.value) }} />}
      </div>
      <div className="row quick">
        <button type="button" className="btn sm ghost" onClick={() => { const x = iso(today); setD(x); commit(x, t) }}>Today</button>
        <button type="button" className="btn sm ghost" onClick={() => { const x = new Date(today); x.setDate(x.getDate() + 1); setD(iso(x)); commit(iso(x), t) }}>Tomorrow</button>
        <button type="button" className="btn sm ghost" onClick={() => { const x = new Date(today); x.setDate(x.getDate() + 7); setD(iso(x)); commit(iso(x), t) }}>In a week</button>
        {s && <button type="button" className="btn sm ghost" onClick={() => { setD(''); setT(''); onChange(null); onDone?.() }}>Clear</button>}
      </div>
    </div>
  )
}

function ChoiceInput({ f, v, onChange, multi, autoFocus, form }: EditorProps & { multi?: boolean }) {
  const [q, setQ] = useState('')
  const [adding, setAdding] = useState(false)
  const chosen: string[] = multi ? ((v as string[]) || []) : v ? [String(v)] : []
  const list = (f.options.choices || []).filter(c => !q || c.name.toLowerCase().includes(q.toLowerCase()))
  const exact = (f.options.choices || []).some(c => c.name.toLowerCase() === q.trim().toLowerCase())
  const pick = (id: string) => {
    if (multi) onChange(chosen.includes(id) ? chosen.filter(x => x !== id) : [...chosen, id])
    else onChange(chosen[0] === id ? null : id)
  }
  const add = () => {
    const name = q.trim()
    if (!name) return
    // a new option: sent by name, the app adds it to the field
    if (multi) onChange([...chosen.map(id => choiceOf(f, id)?.name || id), name], true)
    else onChange(name, true)
    setQ('')
  }
  if (form && (f.options.choices || []).length <= 12) {
    return (
      <div className="choice-pills">
        {(f.options.choices || []).map(c => (
          <button key={c.id} type="button" className={`choice-pill ${chosen.includes(c.id) ? 'on' : ''}`} aria-pressed={chosen.includes(c.id)} onClick={() => pick(c.id)}><ChoicePill c={c} /></button>
        ))}
        {adding
          ? <input className="choice-new" autoFocus value={q} placeholder="New option" onChange={e => setQ(e.target.value)} onBlur={() => { add(); setAdding(false) }} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); add(); setAdding(false) } if (e.key === 'Escape') { setQ(''); setAdding(false) } }} />
          : <button type="button" className="choice-pill add" onClick={() => setAdding(true)} aria-label="Add an option"><Icon name="plus" size={13} /></button>}
      </div>
    )
  }
  return (
    <div className="choice-edit">
      <input placeholder="Find or add an option" value={q} autoFocus={autoFocus} onChange={e => setQ(e.target.value)}
        onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); if (list.length && (exact || !q.trim())) pick(list[0].id); else add() } }} />
      <div className="choice-list" role="listbox" aria-multiselectable={multi}>
        {list.map(c => (
          <button key={c.id} type="button" role="option" aria-selected={chosen.includes(c.id)} className={`choice-opt ${chosen.includes(c.id) ? 'on' : ''}`} onClick={() => pick(c.id)}>
            <span className="choice-check">{chosen.includes(c.id) && <Icon name="check" size={13} strokeWidth={2.5} />}</span><ChoicePill c={c} />
          </button>
        ))}
        {q.trim() && !exact && <button type="button" className="choice-opt add" onClick={add}><Icon name="plus" size={13} />Add “{q.trim()}”</button>}
        {!list.length && !q.trim() && <div className="muted small pad-s">No options yet: type one to add it.</div>}
      </div>
    </div>
  )
}

function LinkInput({ f, v, onChange }: EditorProps) {
  const { bases } = useData()
  const other = bases?.flatMap(b => b.tables).find(t => t.id === f.options.table_id)
  const chosen = (v as LinkRef[]) || []
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 200)
  const [rows, setRows] = useState<Row[] | null>(null)
  useEffect(() => {
    if (!f.options.table_id) return
    let live = true
    api.get<RowsResult>(`${V1}/tables/${f.options.table_id}/records?limit=40&q=${encodeURIComponent(dq)}`).then(r => { if (live) setRows(r.records) }).catch(() => setRows([]))
    return () => { live = false }
  }, [dq, f.options.table_id])
  const ids = chosen.map(c => c.id)
  const set = (next: LinkRef[]) => onChange(next.map(x => x.id))
  const toggle = (r: Row) => {
    if (ids.includes(r.id)) set(chosen.filter(x => x.id !== r.id))
    else set(f.options.single ? [{ id: r.id, title: r.title }] : [...chosen, { id: r.id, title: r.title }])
  }
  const make = async () => {
    const name = q.trim()
    if (!name) return
    // a new row in the other table, named what was typed
    onChange([...ids, name], true)
    setQ('')
  }
  return (
    <div className="choice-edit">
      {chosen.length > 0 && <div className="pills wrap mb-s">{chosen.map(l => <span key={l.id} className="lpill">{l.title}<button type="button" aria-label={`Remove ${l.title}`} onClick={() => set(chosen.filter(x => x.id !== l.id))}><Icon name="x" size={11} /></button></span>)}</div>}
      <input placeholder={`Find in ${other?.name || 'the other table'}`} value={q} onChange={e => setQ(e.target.value)} />
      <div className="choice-list">
        {rows === null && <div className="pad-s"><Spinner /></div>}
        {rows?.map(r => (
          <button key={r.id} type="button" className={`choice-opt ${ids.includes(r.id) ? 'on' : ''}`} onClick={() => toggle(r)}>
            <span className="choice-check">{ids.includes(r.id) && <Icon name="check" size={13} strokeWidth={2.5} />}</span><span className="ellipsis">{r.title || `Row ${r.number}`}</span>
          </button>
        ))}
        {q.trim() && !rows?.some(r => r.title.toLowerCase() === q.trim().toLowerCase()) && <button type="button" className="choice-opt add" onClick={make}><Icon name="plus" size={13} />Add “{q.trim()}” to {other?.name || 'the other table'}</button>}
        {rows && !rows.length && !q && <div className="muted small pad-s">{other?.name || 'The other table'} has no rows yet.</div>}
      </div>
    </div>
  )
}

function PocketInput({ f, v, onChange }: EditorProps) {
  const { kinds } = useData()
  const kind = kinds?.find(k => k.slug === f.options.app)
  const chosen = (v as PocketRef[]) || []
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [items, setItems] = useState<PocketRef[] | null>(null)
  const [off, setOff] = useState('')
  useEffect(() => {
    if (!kind) return
    let live = true
    setItems(null)
    api.get<{ items: PocketRef[]; off?: string }>(`${V1}/apps/pick?kind=${kind.slug}&q=${encodeURIComponent(dq)}`).then(r => { if (live) { setItems(r.items); setOff(r.off || '') } }).catch(e => { setItems([]); setOff(String(e.message)) })
    return () => { live = false }
  }, [dq, kind])
  if (kind && !kind.ok) return <div className="app-off compact"><span className="app-off-ico"><Icon name={kind.icon} size={15} /></span><div className="grow"><div className="app-off-h">{kind.app_name} is not connected</div><div className="muted small">{kind.fix || kind.detail}</div></div></div>
  const has = (x: PocketRef) => chosen.some(c => c.ref === x.ref && c.app === x.app)
  const toggle = (x: PocketRef) => onChange(has(x) ? chosen.filter(c => !(c.ref === x.ref && c.app === x.app)) : f.options.single ? [x] : [...chosen, x])
  return (
    <div className="choice-edit">
      {chosen.length > 0 && <div className="pills wrap mb-s">{chosen.map(l => <span key={l.app + l.ref} className="lpill pocket">{l.url ? <a href={l.url} target="_blank" rel="noreferrer">{l.title}</a> : l.title}<button type="button" aria-label={`Remove ${l.title}`} onClick={() => toggle(l)}><Icon name="x" size={11} /></button></span>)}</div>}
      <input placeholder={`Find a ${kind?.label.toLowerCase() || 'thing'} in ${kind?.app_name || 'your apps'}`} value={q} onChange={e => setQ(e.target.value)} />
      <div className="choice-list">
        {items === null && <div className="pad-s"><Spinner /></div>}
        {items?.map(x => (
          <button key={x.ref} type="button" className={`choice-opt ${has(x) ? 'on' : ''}`} onClick={() => toggle(x)}>
            <span className="choice-check">{has(x) && <Icon name="check" size={13} strokeWidth={2.5} />}</span>
            <span className="grow ellipsis">{x.title}{x.sub && <span className="muted small"> · {x.sub}</span>}</span>
          </button>
        ))}
        {items && !items.length && <div className="muted small pad-s">{off || (q ? 'Nothing found.' : 'Type to search.')}</div>}
      </div>
    </div>
  )
}

function FilesInput({ v, onChange }: EditorProps) {
  const files = (v as FileRef[]) || []
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const { toast } = useToast()
  const add = async (list: FileList | null) => {
    if (!list?.length) return
    setBusy(true)
    const added: FileRef[] = []
    try {
      for (const file of Array.from(list)) added.push(await upload<FileRef>(`${V1}/files`, 'file', file))
      onChange([...files, ...added])
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="files-edit" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); add(e.dataTransfer.files) }}>
      {files.map(x => (
        <div key={x.id} className="file-row">
          {isImage(x) ? <img src={fileURL(x)} alt="" /> : <span className="file-ico"><Icon name="file" size={16} /></span>}
          <a href={fileURL(x)} target="_blank" rel="noreferrer" className="grow ellipsis">{x.name}</a>
          <span className="muted small">{fmtBytes(x.size)}</span>
          <a className="btn ghost icon-only sm" href={fileURL(x, true)} aria-label={`Download ${x.name}`}><Icon name="download" size={14} /></a>
          <button type="button" className="btn ghost icon-only sm" aria-label={`Remove ${x.name}`} onClick={() => onChange(files.filter(y => y.id !== x.id))}><Icon name="x" size={14} /></button>
        </div>
      ))}
      <input ref={input} type="file" multiple hidden onChange={e => { add(e.target.files); e.target.value = '' }} />
      <button type="button" className="btn sm" onClick={() => input.current?.click()} disabled={busy}>{busy ? <Spinner /> : <Icon name="paperclip" size={14} />}Add files</button>
      <span className="muted small hide-sm"> or drop them here</span>
    </div>
  )
}

/** Cells whose editor is a small panel (not typed in place). */
export const panelEditor = (f: Field) => ['longtext', 'date', 'select', 'multiselect', 'link', 'pocket', 'attachment'].includes(f.type)

export function stop(e: React.SyntheticEvent) { e.stopPropagation() }
