// The grid: a spreadsheet of the rows a view shows. Click a cell to pick it,
// type or press Enter to change it, arrows to move, paste from a
// spreadsheet to fill many cells (new rows are added as needed).
import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { chipStyle, choiceOf, computed, numeric, SUMMARIES, text, typeInfo } from '../lib/fields'
import type { Field, Row, TablePage, View, ViewConfig } from '../lib/types'
import { CellView, panelEditor, ValueEditor } from './Cell'
import { Icon } from './icons'
import { type Anchor, Menu, type MenuItem, Popover, rectOf, useIsMobile, copyText, useToast } from './ui'
import { visibleFields } from './ViewPanels'

export interface GridProps {
  page: TablePage
  view: View
  rows: Row[]
  summaries?: Record<string, unknown>
  onSet: (row: Row, f: Field, v: unknown, typecast?: boolean) => void
  onOpen: (id: number) => void
  onAdd: (values?: Record<string, unknown>) => void
  onFieldMenu: (f: Field, anchor: Anchor) => void
  onAddField: () => void
  patchView: (c: Partial<ViewConfig>) => void
  onPaste: (row: number, col: number, grid: string[][], cols: Field[]) => void
  selected: Set<number>
  setSelected: (s: Set<number>) => void
}

const HEIGHTS: Record<string, number> = { short: 34, medium: 58, tall: 92 }
const NUM_W = 64

export function defaultWidth(f: Field) {
  if (f.primary) return 230
  switch (f.type) {
    case 'checkbox': return 96
    case 'rating': return 130
    case 'number': case 'percent': case 'autonumber': return 120
    case 'currency': return 140
    case 'date': return f.options.time ? 170 : 140
    case 'longtext': return 280
    case 'created': case 'updated': return 160
  }
  return 180
}

type Item = { kind: 'group'; key: string; label: React.ReactNode; count: number } | { kind: 'row'; row: Row; index: number }

export function GridView(p: GridProps) {
  const mobile = useIsMobile()
  if (mobile) return <CardList {...p} />
  return <Grid {...p} />
}

function Grid({ page, view, rows, summaries, onSet, onOpen, onAdd, onFieldMenu, onAddField, patchView, onPaste, selected, setSelected }: GridProps) {
  const cols = useMemo(() => visibleFields(page.fields, view), [page.fields, view])
  const [widths, setWidths] = useState<Record<string, number>>(view.config.widths)
  useEffect(() => setWidths(view.config.widths), [view.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const w = (f: Field) => widths[f.id] || defaultWidth(f)
  const rowH = HEIGHTS[view.config.row_height] || HEIGHTS.short
  const groupField = view.config.group_by ? page.fields.find(f => f.id === view.config.group_by) : undefined
  const { toast } = useToast()

  // rows and group headers, flattened for drawing only what is on screen
  const items = useMemo<Item[]>(() => {
    if (!groupField) return rows.map((row, index) => ({ kind: 'row' as const, row, index }))
    const out: Item[] = []
    let last = '\u0000'
    rows.forEach((row, index) => {
      const v = row.values[groupField.id]
      const key = text(groupField, v)
      if (key !== last) {
        last = key
        const count = rows.filter(r => text(groupField, r.values[groupField.id]) === key).length
        const c = groupField.type === 'select' ? choiceOf(groupField, v) : undefined
        out.push({ kind: 'group', key: key + index, count, label: c ? <span className="cpill" style={chipStyle(c.color)}>{c.name}</span> : key || <span className="muted">No {groupField.name.toLowerCase()}</span> })
      }
      out.push({ kind: 'row', row, index })
    })
    return out
  }, [rows, groupField])

  const scroller = useRef<HTMLDivElement>(null)
  const [scroll, setScroll] = useState({ top: 0, h: 800 })
  useLayoutEffect(() => {
    const el = scroller.current
    if (!el) return
    const on = () => setScroll({ top: el.scrollTop, h: el.clientHeight })
    on()
    el.addEventListener('scroll', on, { passive: true })
    const ro = new ResizeObserver(on)
    ro.observe(el)
    return () => { el.removeEventListener('scroll', on); ro.disconnect() }
  }, [])
  const GROUP_H = 40
  const offsets = useMemo(() => {
    const o: number[] = []
    let y = 0
    for (const it of items) { o.push(y); y += it.kind === 'group' ? GROUP_H : rowH }
    o.push(y)
    return o
  }, [items, rowH])
  const total = offsets[offsets.length - 1]
  let first = 0
  while (first < items.length && offsets[first + 1] < scroll.top - 400) first++
  let last = first
  while (last < items.length && offsets[last] < scroll.top + scroll.h + 400) last++

  // the picked cell and the one being changed
  const [cur, setCur] = useState<{ r: number; c: number } | null>(null)
  const [edit, setEdit] = useState<{ r: number; c: number; anchor?: Anchor; init?: string } | null>(null)
  const cellRef = (r: number, c: number) => scroller.current?.querySelector<HTMLElement>(`[data-cell="${r}:${c}"]`)
  const startEdit = (r: number, c: number, init?: string) => {
    const f = cols[c], row = rows[r]
    if (!f || !row || computed(f.type)) return
    if (f.type === 'checkbox') { onSet(row, f, !row.values[f.id]); return }
    if (f.type === 'rating') return
    const el = cellRef(r, c)
    setEdit({ r, c, anchor: panelEditor(f) && el ? rectOf(el) : undefined, init })
  }
  const move = (dr: number, dc: number) => setCur(x => {
    const n = { r: Math.max(0, Math.min(rows.length - 1, (x?.r ?? 0) + dr)), c: Math.max(0, Math.min(cols.length - 1, (x?.c ?? 0) + dc)) }
    requestAnimationFrame(() => cellRef(n.r, n.c)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }))
    return n
  })
  const onKey = (e: React.KeyboardEvent) => {
    if (edit || !cur || (e.target as HTMLElement).closest('input, textarea, .popover')) return
    const f = cols[cur.c], row = rows[cur.r]
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'c' && f && row) { e.preventDefault(); copyText(text(f, row.values[f.id])); toast('Copied'); return }
    if (e.metaKey || e.ctrlKey || e.altKey) return
    switch (e.key) {
      case 'ArrowUp': e.preventDefault(); move(-1, 0); return
      case 'ArrowDown': e.preventDefault(); move(1, 0); return
      case 'ArrowLeft': e.preventDefault(); move(0, -1); return
      case 'ArrowRight': case 'Tab': e.preventDefault(); move(0, e.shiftKey ? -1 : 1); return
      case 'Enter': case 'F2': e.preventDefault(); if (e.shiftKey && row) onOpen(row.id); else startEdit(cur.r, cur.c); return
      case 'Escape': setCur(null); return
      case ' ': if (f?.type === 'checkbox' && row) { e.preventDefault(); onSet(row, f, !row.values[f.id]) } else if (row) { e.preventDefault(); onOpen(row.id) } return
      case 'Backspace': case 'Delete':
        if (f && row && !computed(f.type) && row.values[f.id] !== undefined) { e.preventDefault(); onSet(row, f, f.type === 'checkbox' ? false : null) }
        return
    }
    if (e.key.length === 1 && f && !panelEditor(f) && !computed(f.type) && !['checkbox', 'rating'].includes(f.type)) { e.preventDefault(); startEdit(cur.r, cur.c, e.key) }
  }
  const onPasteEv = (e: React.ClipboardEvent) => {
    if (edit || !cur) return
    const t = e.clipboardData.getData('text/plain')
    if (!t) return
    e.preventDefault()
    const grid = t.replace(/\r/g, '').replace(/\n$/, '').split('\n').map(l => l.split('\t'))
    onPaste(cur.r, cur.c, grid, cols)
  }

  // resizing a column: live while dragging, saved when let go
  const resize = (f: Field, e: React.PointerEvent) => {
    e.preventDefault(); e.stopPropagation()
    const x0 = e.clientX, w0 = w(f)
    let nw = w0
    const mv = (ev: PointerEvent) => { nw = Math.max(70, Math.min(800, w0 + ev.clientX - x0)); setWidths(x => ({ ...x, [f.id]: nw })) }
    const up = () => { window.removeEventListener('pointermove', mv); window.removeEventListener('pointerup', up); patchView({ widths: { ...view.config.widths, [f.id]: nw } }) }
    window.addEventListener('pointermove', mv)
    window.addEventListener('pointerup', up)
  }

  const [sumMenu, setSumMenu] = useState<{ f: Field; a: Anchor } | null>(null)
  const sumItems = (f: Field): MenuItem[] => {
    const kind = numeric(f.type) || f.type === 'rollup' || f.type === 'formula' ? 'number' : f.type === 'checkbox' ? 'check' : ['date', 'created', 'updated'].includes(f.type) ? 'date' : 'any'
    const cur = view.config.summaries[f.id] || ''
    return [{ label: 'Nothing', checked: !cur, onSelect: () => { const s = { ...view.config.summaries }; delete s[f.id]; patchView({ summaries: s }) } },
      ...SUMMARIES.filter(s => s.for === 'any' || s.for === kind).map(s => ({ label: s.label, checked: cur === s.value, onSelect: () => patchView({ summaries: { ...view.config.summaries, [f.id]: s.value } }) }))]
  }
  const allSel = rows.length > 0 && rows.every(r => selected.has(r.id))
  const width = NUM_W + cols.reduce((a, f) => a + w(f), 0) + 48

  return (
    <div className={`grid-wrap h-${view.config.row_height || 'short'}`}>
      <div className="grid-scroll" ref={scroller} tabIndex={0} onKeyDown={onKey} onPaste={onPasteEv} role="grid" aria-rowcount={rows.length} aria-colcount={cols.length}>
        <div className="grid-inner" style={{ width }}>
          <div className="g-head" role="row">
            <div className="g-num g-hcell" style={{ width: NUM_W }}>
              <input type="checkbox" aria-label="Pick every row" checked={allSel} onChange={() => setSelected(allSel ? new Set() : new Set(rows.map(r => r.id)))} />
            </div>
            {cols.map(f => (
              <div key={f.id} role="columnheader" className={`g-hcell ${f.primary ? 'g-primary' : ''}`} style={{ width: w(f), left: f.primary ? NUM_W : undefined }}>
                <button type="button" className="g-hbtn" onClick={e => onFieldMenu(f, rectOf(e.currentTarget))} title={f.description || typeInfo(f.type).label}>
                  <Icon name={typeInfo(f.type).icon} size={13} /><span className="ellipsis">{f.name}</span>
                  {view.config.sorts.some(s => s.field === f.id) && <Icon name="arrow-up-down" size={12} className="g-flag" />}
                  {view.config.filters.some(s => s.field === f.id) && <Icon name="filter" size={12} className="g-flag" />}
                </button>
                <span className="g-resize" onPointerDown={e => resize(f, e)} aria-hidden="true" />
              </div>
            ))}
            <div className="g-hcell g-add"><button type="button" className="btn ghost icon-only sm" onClick={onAddField} aria-label="Add a field" title="Add a field"><Icon name="plus" size={15} /></button></div>
          </div>
          <div style={{ height: offsets[first] }} />
          {items.slice(first, last).map(it => it.kind === 'group'
            ? <div key={'g' + it.key} className="g-group" style={{ height: GROUP_H }}><span className="g-group-in">{it.label}<span className="muted small">{it.count}</span></span></div>
            : <GridRow key={it.row.id} row={it.row} r={it.index} cols={cols} w={w} h={rowH} cur={cur} sel={selected.has(it.row.id)}
              onPick={(c) => setCur({ r: it.index, c })} onEdit={c => startEdit(it.index, c)} onOpen={() => onOpen(it.row.id)} onSet={onSet}
              onSel={() => { const s = new Set(selected); if (s.has(it.row.id)) s.delete(it.row.id); else s.add(it.row.id); setSelected(s) }}
              editing={edit && edit.r === it.index && !edit.anchor ? edit : null} onDone={() => { setEdit(null); scroller.current?.focus({ preventScroll: true }) }} />)}
          <div style={{ height: total - offsets[last] }} />
          <button type="button" className="g-addrow" style={{ width }} onClick={() => onAdd()}><Icon name="plus" size={14} />Add a row</button>
          <div className="g-foot" aria-label="Totals" style={{ width }}>
            <div className="g-num muted small" style={{ width: NUM_W }}>{rows.length}</div>
            {cols.map(f => {
              const fn = view.config.summaries[f.id]
              const v = summaries?.[f.id]
              return (
                <button key={f.id} type="button" className={`g-sum ${f.primary ? 'g-primary' : ''}`} style={{ width: w(f) }} onClick={e => setSumMenu({ f, a: rectOf(e.currentTarget) })}>
                  {fn ? <><span className="muted small">{SUMMARIES.find(s => s.value === fn)?.label}</span> <b className="tnum">{v === null || v === undefined ? '—' : typeof v === 'number' && fn !== 'percent_checked' && !['filled', 'empty', 'unique', 'checked'].includes(fn) ? text(f.type === 'rating' ? { ...f, type: 'number' } : f, v) : typeof v === 'number' ? (fn === 'percent_checked' ? Math.round(v) + '%' : String(v)) : text(f, v)}</b></> : <span className="g-sum-add">Total</span>}
                </button>
              )
            })}
          </div>
        </div>
      </div>
      {edit?.anchor && rows[edit.r] && cols[edit.c] && (
        <Popover anchor={edit.anchor} onClose={() => { setEdit(null); scroller.current?.focus({ preventScroll: true }) }} prefer="below" width={cols[edit.c].type === 'longtext' ? 420 : 320} label={cols[edit.c].name} className="cell-pop">
          <div className="cell-pop-in">
            <div className="cell-pop-h"><b>{cols[edit.c].name}</b><button type="button" className="linkish small" onClick={() => onOpen(rows[edit.r].id)}>Open the row</button></div>
            <ValueEditor f={cols[edit.c]} v={rows[edit.r].values[cols[edit.c].id]} autoFocus inline={cols[edit.c].type === 'longtext'}
              onChange={(v, tc) => { onSet(rows[edit.r], cols[edit.c], v, tc); if (cols[edit.c].type === 'select') setEdit(null) }} onDone={() => setEdit(null)} />
          </div>
        </Popover>
      )}
      {sumMenu && <Menu anchor={sumMenu.a} items={sumItems(sumMenu.f)} onClose={() => setSumMenu(null)} width={180} />}
    </div>
  )
}

/** Whether the last press was on the cell already picked (then a click edits it). */
let downOnActive = false

const GridRow = React.memo(function GridRow({ row, r, cols, w, h, cur, sel, onPick, onEdit, onOpen, onSet, onSel, editing, onDone }: {
  row: Row; r: number; cols: Field[]; w: (f: Field) => number; h: number; cur: { r: number; c: number } | null; sel: boolean
  onPick: (c: number) => void; onEdit: (c: number) => void; onOpen: () => void; onSet: GridProps['onSet']; onSel: () => void
  editing: { r: number; c: number; init?: string } | null; onDone: () => void
}) {
  return (
    <div className={`g-row ${sel ? 'sel' : ''}`} style={{ height: h }} role="row" aria-rowindex={r + 2}>
      <div className="g-num" style={{ width: NUM_W }}>
        <span className="g-n tnum">{row.number}</span>
        <input type="checkbox" className="g-check" checked={sel} onChange={onSel} aria-label={`Pick row ${row.number}`} />
        <button type="button" className="g-expand" onClick={onOpen} aria-label="Open the row" title="Open the row (shift Enter)"><Icon name="expand" size={13} /></button>
        {row.comments > 0 && <span className="g-comments" title={`${row.comments} comments`}><Icon name="message" size={11} />{row.comments}</span>}
      </div>
      {cols.map((f, c) => {
        const active = cur?.r === r && cur?.c === c
        const isEditing = editing && editing.c === c
        return (
          <div key={f.id} role="gridcell" data-cell={`${r}:${c}`} className={`g-cell t-${f.type} ${f.primary ? 'g-primary' : ''} ${active ? 'active' : ''} ${computed(f.type) ? 'ro' : ''}`}
            style={{ width: w(f), left: f.primary ? NUM_W : undefined }}
            onMouseDown={() => { downOnActive = !!active; onPick(c) }}
            onDoubleClick={() => onEdit(c)}
            onClick={e => {
              if (f.type === 'checkbox' && (e.target as HTMLElement).closest('.cbox')) onSet(row, f, !row.values[f.id])
              else if (downOnActive && !isEditing && e.detail === 1 && !(e.target as HTMLElement).closest('a, button')) onEdit(c) // a click on the picked cell changes it
            }}>
            {isEditing
              ? <ValueEditor f={f} v={editing.init !== undefined ? editing.init : row.values[f.id]} inline autoFocus onChange={v => onSet(row, f, v)} onDone={onDone} />
              : f.type === 'rating'
                ? <span className="stars">{Array.from({ length: f.options.max || 5 }, (_, i) => <button key={i} type="button" tabIndex={-1} className={i < Number(row.values[f.id] || 0) ? 'on' : ''} onClick={e => { e.stopPropagation(); onSet(row, f, i + 1 === Number(row.values[f.id]) ? null : i + 1) }} aria-label={`${i + 1}`}>★</button>)}</span>
                : <CellView f={f} v={row.values[f.id]} wrap={h > 40} />}
          </div>
        )
      })}
      <div className="g-cell g-add" />
    </div>
  )
})

/** On a phone: one card per row; tap to open it. */
function CardList({ page, view, rows, onOpen, onAdd }: GridProps) {
  const cols = visibleFields(page.fields, view)
  const primary = cols.find(f => f.primary)
  const rest = cols.filter(f => !f.primary).slice(0, 4)
  const groupField = view.config.group_by ? page.fields.find(f => f.id === view.config.group_by) : undefined
  let last = '\u0000'
  return (
    <div className="cards">
      {rows.map(r => {
        const g = groupField ? text(groupField, r.values[groupField.id]) : ''
        const head = groupField && g !== last ? (last = g, <h3 key={'g' + r.id} className="card-group">{g || `No ${groupField.name.toLowerCase()}`}</h3>) : null
        return (
          <React.Fragment key={r.id}>
            {head}
            <button type="button" className="rcard" onClick={() => onOpen(r.id)}>
              <div className="rcard-title">{primary ? (text(primary, r.values[primary.id]) || `Row ${r.number}`) : r.title}</div>
              {rest.map(f => r.values[f.id] !== undefined && r.values[f.id] !== null && r.values[f.id] !== '' && (
                <div key={f.id} className="rcard-line"><span className="muted small">{f.name}</span><span className="rcard-v"><CellView f={f} v={r.values[f.id]} /></span></div>
              ))}
            </button>
          </React.Fragment>
        )
      })}
      {rows.length === 0 && <p className="muted center pad-card">No rows here yet.</p>}
      <button type="button" className="btn block-btn" onClick={() => onAdd()}><Icon name="plus" size={15} />Add a row</button>
    </div>
  )
}
