// The board (cards in columns by a choice), the calendar (rows on their
// day) and the gallery (cards with a picture).
import React, { useMemo, useState } from 'react'
import { chipStyle, rowTitle, text } from '../lib/fields'
import type { Field, FileRef, Row, TablePage, View, ViewConfig } from '../lib/types'
import { addDays, todayISO, weekday } from '../lib/format'
import { useApp } from '../lib/app-context'
import { CellView, fileURL, isImage } from './Cell'
import { Icon } from './icons'
import { Select } from './Select'
import { visibleFields } from './ViewPanels'

interface Props {
  page: TablePage
  view: View
  rows: Row[]
  onSet: (row: Row, f: Field, v: unknown) => void
  onOpen: (id: number) => void
  onAdd: (values?: Record<string, unknown>) => void
  patchView: (c: Partial<ViewConfig>) => void
}

function cover(page: TablePage, view: View, r: Row): FileRef | undefined {
  const f = page.fields.find(x => x.id === view.config.cover) || page.fields.find(x => x.type === 'attachment')
  return f ? ((r.values[f.id] as FileRef[]) || []).find(isImage) : undefined
}

function CardBody({ page, view, r, skip, max = 4 }: { page: TablePage; view: View; r: Row; skip?: number; max?: number }) {
  const cols = visibleFields(page.fields, view).filter(f => !f.primary && f.id !== skip && f.type !== 'attachment')
  const shown = cols.filter(f => text(f, r.values[f.id]) !== '').slice(0, max)
  return (
    <>
      <div className="kcard-title">{rowTitle(r)}</div>
      {shown.map(f => <div key={f.id} className="kcard-line" title={f.name}>{['date', 'number', 'currency', 'percent', 'rating', 'created', 'updated', 'formula', 'rollup'].includes(f.type) && <span className="muted">{f.name} </span>}<CellView f={f} v={r.values[f.id]} /></div>)}
    </>
  )
}

// ---------- board ----------

export function BoardView({ page, view, rows, onSet, onOpen, onAdd, patchView }: Props) {
  const stack = page.fields.find(f => f.id === view.config.stack_by && (f.type === 'select'))
  const choices = page.fields.filter(f => f.type === 'select')
  const [over, setOver] = useState<string | null>(null)
  if (!stack) {
    return <div className="view-empty">
      <Icon name="board" size={28} />
      <p>A board puts the rows in columns by a choice field (a status, a stage).</p>
      {choices.length ? <div className="row center"><Select ariaLabel="Columns by" value="" placeholder="Columns by…" onChange={v => patchView({ stack_by: Number(v) })} options={choices.map(f => ({ value: String(f.id), label: f.name }))} /></div>
        : <p className="muted small">Add a Choice field to the table first.</p>}
    </div>
  }
  const cols = [{ id: '', name: `No ${stack.name.toLowerCase()}`, color: '#ced4da' }, ...(stack.options.choices || [])]
  const by = (id: string) => rows.filter(r => (r.values[stack.id] || '') === id)
  const drop = (id: string, e: React.DragEvent) => {
    e.preventDefault()
    setOver(null)
    const rid = Number(e.dataTransfer.getData('text/row'))
    const r = rows.find(x => x.id === rid)
    if (r && (r.values[stack.id] || '') !== id) onSet(r, stack, id || null)
  }
  return (
    <div className="board">
      {cols.map(c => {
        const list = by(c.id)
        if (!c.id && !list.length) return null
        return (
          <section key={c.id || 'none'} className={`bcol ${over === c.id ? 'over' : ''}`} onDragOver={e => { e.preventDefault(); setOver(c.id) }} onDragLeave={() => setOver(o => (o === c.id ? null : o))} onDrop={e => drop(c.id, e)} aria-label={c.name}>
            <header className="bcol-head"><span className="cpill" style={chipStyle(c.color)}>{c.name}</span><span className="muted small">{list.length}</span></header>
            <div className="bcol-cards">
              {list.map(r => {
                const img = cover(page, view, r)
                return (
                  <button key={r.id} type="button" className="kcard" draggable onDragStart={e => { e.dataTransfer.setData('text/row', String(r.id)); e.dataTransfer.effectAllowed = 'move' }} onClick={() => onOpen(r.id)}>
                    {img && <img className="kcard-cover" src={fileURL(img)} alt="" loading="lazy" />}
                    <CardBody page={page} view={view} r={r} skip={stack.id} />
                  </button>
                )
              })}
            </div>
            <button type="button" className="bcol-add" onClick={() => onAdd(c.id ? { [stack.id]: c.id } : {})}><Icon name="plus" size={14} />Add</button>
          </section>
        )
      })}
    </div>
  )
}

// ---------- calendar ----------

export function CalendarView({ page, view, rows, onSet, onOpen, onAdd, patchView }: Props) {
  const { settings } = useApp()
  const dateField = page.fields.find(f => f.id === view.config.date_field && ['date', 'created', 'updated'].includes(f.type))
  const dates = page.fields.filter(f => ['date', 'created', 'updated'].includes(f.type))
  const [month, setMonth] = useState(() => todayISO().slice(0, 7))
  const [over, setOver] = useState('')
  const byDay = useMemo(() => {
    const m = new Map<string, Row[]>()
    if (!dateField) return m
    for (const r of rows) {
      const v = String(r.values[dateField.id] || '')
      if (!v) continue
      let d = v.slice(0, 10)
      if (dateField.type !== 'date') { const x = new Date(v); d = `${x.getFullYear()}-${String(x.getMonth() + 1).padStart(2, '0')}-${String(x.getDate()).padStart(2, '0')}` }
      m.set(d, [...(m.get(d) || []), r])
    }
    return m
  }, [rows, dateField])
  if (!dateField) {
    return <div className="view-empty">
      <Icon name="calendar" size={28} />
      <p>A calendar puts each row on the day of a date field.</p>
      {dates.length ? <div className="row center"><Select ariaLabel="Date" value="" placeholder="Which date…" onChange={v => patchView({ date_field: Number(v) })} options={dates.map(f => ({ value: String(f.id), label: f.name }))} /></div>
        : <p className="muted small">Add a Date field to the table first.</p>}
    </div>
  }
  const ws = settings?.week_start === 7 ? 0 : settings?.week_start === 6 ? 6 : 1
  const first = new Date(month + '-01T12:00:00')
  const lead = (first.getDay() - ws + 7) % 7
  const start = addDays(month + '-01', -lead)
  const days = Array.from({ length: 42 }, (_, i) => addDays(start, i))
  const weeks = days[35].slice(0, 7) !== month ? 5 : 6
  const shown = days.slice(0, weeks * 7)
  const today = todayISO()
  const label = first.toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
  const shift = (n: number) => { const d = new Date(month + '-01T12:00:00'); d.setMonth(d.getMonth() + n); setMonth(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`) }
  const undated = rows.filter(r => !r.values[dateField.id]).length
  const editable = dateField.type === 'date'
  const drop = (d: string, e: React.DragEvent) => {
    e.preventDefault()
    setOver('')
    const r = rows.find(x => x.id === Number(e.dataTransfer.getData('text/row')))
    if (!r || !editable) return
    const old = String(r.values[dateField.id] || '')
    if (old.slice(0, 10) !== d) onSet(r, dateField, old.length > 10 ? d + old.slice(10) : d)
  }
  return (
    <div className="cal">
      <div className="cal-bar">
        <button type="button" className="btn ghost icon-only" onClick={() => shift(-1)} aria-label="Month before"><Icon name="chevron-left" size={18} /></button>
        <h2 className="cal-title">{label}</h2>
        <button type="button" className="btn ghost icon-only" onClick={() => shift(1)} aria-label="Month after"><Icon name="chevron-right" size={18} /></button>
        <button type="button" className="btn sm" onClick={() => setMonth(today.slice(0, 7))}>Today</button>
        <span className="grow" />
        {undated > 0 && <span className="muted small">{undated} without {dateField.name.toLowerCase()}</span>}
      </div>
      <div className="cal-grid" style={{ gridTemplateRows: `auto repeat(${weeks}, minmax(96px, 1fr))` }}>
        {shown.slice(0, 7).map(d => <div key={'h' + d} className="cal-dow">{weekday(d)}</div>)}
        {shown.map(d => {
          const list = byDay.get(d) || []
          return (
            <div key={d} className={`cal-day ${d.slice(0, 7) !== month ? 'other' : ''} ${d === today ? 'today' : ''} ${over === d ? 'over' : ''}`}
              onDragOver={e => { if (editable) { e.preventDefault(); setOver(d) } }} onDragLeave={() => setOver(o => (o === d ? '' : o))} onDrop={e => drop(d, e)}>
              <div className="cal-day-h">
                <span className="cal-n">{Number(d.slice(8))}</span>
                {editable && <button type="button" className="cal-add" aria-label={`Add a row on ${d}`} onClick={() => onAdd({ [dateField.id]: d })}><Icon name="plus" size={12} /></button>}
              </div>
              {list.slice(0, 4).map(r => {
                const v = String(r.values[dateField.id] || '')
                return (
                  <button key={r.id} type="button" className="cal-chip" draggable={editable} onDragStart={e => e.dataTransfer.setData('text/row', String(r.id))} onClick={() => onOpen(r.id)} title={rowTitle(r)}>
                    {v.length > 10 && <span className="cal-time tnum">{v.slice(11, 16)}</span>}<span className="ellipsis">{rowTitle(r)}</span>
                  </button>
                )
              })}
              {list.length > 4 && <span className="muted small cal-more">+{list.length - 4} more</span>}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ---------- gallery ----------

export function GalleryView({ page, view, rows, onOpen, onAdd }: Props) {
  const hasFiles = page.fields.some(f => f.type === 'attachment')
  return (
    <div className="gallery">
      {rows.map(r => {
        const img = cover(page, view, r)
        return (
          <button key={r.id} type="button" className="gcard" onClick={() => onOpen(r.id)}>
            {hasFiles && <div className="gcard-cover">{img ? <img src={fileURL(img)} alt="" loading="lazy" /> : <Icon name="image" size={26} />}</div>}
            <div className="gcard-body"><CardBody page={page} view={view} r={r} max={5} /></div>
          </button>
        )
      })}
      <button type="button" className="gcard add" onClick={() => onAdd()}><Icon name="plus" size={22} />Add a row</button>
    </div>
  )
}
