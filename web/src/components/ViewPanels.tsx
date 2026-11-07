// The panels of a view's toolbar: filters, sorts, grouping and which fields show.
import { useState } from 'react'
import { opNeedsValue, opsFor, typeInfo, WITHIN } from '../lib/fields'
import type { Field, Filter, Sort, View, ViewConfig } from '../lib/types'
import { ChoicePill } from './Cell'
import { Icon } from './icons'
import { Select } from './Select'
import { Segmented, Switch } from './ui'

type Patch = (c: Partial<ViewConfig>) => void

export function FilterPanel({ fields, view, patch }: { fields: Field[]; view: View; patch: Patch }) {
  const fl = view.config.filters
  const set = (i: number, f: Partial<Filter>) => patch({ filters: fl.map((x, j) => (j === i ? { ...x, ...f } : x)) })
  const add = () => {
    const f = fields[0]
    if (f) patch({ filters: [...fl, { field: f.id, op: opsFor(f)[0].value, value: '' }] })
  }
  return (
    <div className="vp">
      <div className="vp-head"><b>Show only rows where</b>{fl.length > 1 && <Segmented size="sm" value={view.config.match} onChange={v => patch({ match: v })} options={[{ value: 'all', label: 'all match' }, { value: 'any', label: 'any matches' }]} />}</div>
      {fl.length === 0 && <p className="muted small">No filters: every row shows.</p>}
      {fl.map((x, i) => {
        const f = fields.find(y => y.id === x.field) || fields[0]
        const ops = opsFor(f)
        return (
          <div key={i} className="vp-row">
            <Select ariaLabel="Field" value={String(f.id)} onChange={v => { const nf = fields.find(y => y.id === Number(v))!; set(i, { field: nf.id, op: opsFor(nf)[0].value, value: '' }) }} options={fields.map(y => ({ value: String(y.id), label: y.name }))} />
            <Select ariaLabel="How" value={ops.some(o => o.value === x.op) ? x.op : ops[0].value} onChange={v => set(i, { op: v })} options={ops} />
            {opNeedsValue(x.op) && <FilterValue f={f} x={x} onChange={v => set(i, { value: v })} />}
            <button type="button" className="btn ghost icon-only sm" aria-label="Remove the filter" onClick={() => patch({ filters: fl.filter((_, j) => j !== i) })}><Icon name="x" size={14} /></button>
          </div>
        )
      })}
      <button type="button" className="btn sm" onClick={add}><Icon name="plus" size={14} />Add a filter</button>
    </div>
  )
}

function FilterValue({ f, x, onChange }: { f: Field; x: Filter; onChange: (v: unknown) => void }) {
  const [txt, setTxt] = useState(x.value === undefined || x.value === null ? '' : String(x.value))
  if (x.op === 'within') return <Select ariaLabel="When" value={String(x.value || 'next_week')} onChange={onChange} options={WITHIN} />
  if (f.type === 'select' || f.type === 'multiselect') {
    const chosen: string[] = Array.isArray(x.value) ? (x.value as string[]) : x.value ? [String(x.value)] : []
    return (
      <div className="vp-choices">{(f.options.choices || []).map(c => (
        <button key={c.id} type="button" className={`vp-choice ${chosen.includes(c.id) ? 'on' : ''}`} aria-pressed={chosen.includes(c.id)}
          onClick={() => onChange(chosen.includes(c.id) ? chosen.filter(y => y !== c.id) : [...chosen, c.id])}><ChoicePill c={c} /></button>
      ))}</div>
    )
  }
  if (['date', 'created', 'updated'].includes(f.type)) {
    const rel = ['today', 'tomorrow', 'yesterday'].includes(String(x.value))
    return (
      <div className="row tight">
        <Select ariaLabel="Day" value={rel ? String(x.value) : 'exact'} onChange={v => onChange(v === 'exact' ? '' : v)} options={[{ value: 'today', label: 'today' }, { value: 'tomorrow', label: 'tomorrow' }, { value: 'yesterday', label: 'yesterday' }, { value: 'exact', label: 'a day…' }]} />
        {!rel && <input type="date" value={String(x.value || '')} onChange={e => onChange(e.target.value)} />}
      </div>
    )
  }
  const num = ['number', 'currency', 'percent', 'rating', 'autonumber', 'rollup'].includes(f.type)
  return <input className="vp-val" inputMode={num ? 'decimal' : undefined} value={txt} placeholder="Value" onChange={e => setTxt(e.target.value)} onBlur={() => onChange(txt)} onKeyDown={e => { if (e.key === 'Enter') onChange(txt) }} />
}

export function SortPanel({ fields, view, patch }: { fields: Field[]; view: View; patch: Patch }) {
  const so = view.config.sorts
  const set = (i: number, s: Partial<Sort>) => patch({ sorts: so.map((x, j) => (j === i ? { ...x, ...s } : x)) })
  const unused = fields.filter(f => !so.some(s => s.field === f.id))
  return (
    <div className="vp">
      <div className="vp-head"><b>Order rows by</b></div>
      {so.length === 0 && <p className="muted small">In the order they were added (drag rows to move them).</p>}
      {so.map((x, i) => (
        <div key={i} className="vp-row">
          <Select ariaLabel="Field" value={String(x.field)} onChange={v => set(i, { field: Number(v) })} options={fields.map(y => ({ value: String(y.id), label: y.name }))} />
          <Segmented size="sm" value={x.desc ? 'desc' : 'asc'} onChange={v => set(i, { desc: v === 'desc' })} options={[{ value: 'asc', label: 'A → Z, 1 → 9' }, { value: 'desc', label: 'Z → A, 9 → 1' }]} />
          <button type="button" className="btn ghost icon-only sm" aria-label="Remove" onClick={() => patch({ sorts: so.filter((_, j) => j !== i) })}><Icon name="x" size={14} /></button>
        </div>
      ))}
      {unused.length > 0 && <button type="button" className="btn sm" onClick={() => patch({ sorts: [...so, { field: unused[0].id, desc: false }] })}><Icon name="plus" size={14} />Add a sort</button>}
    </div>
  )
}

export function GroupPanel({ fields, view, patch }: { fields: Field[]; view: View; patch: Patch }) {
  const ok = fields.filter(f => !['attachment', 'longtext', 'link', 'lookup', 'pocket'].includes(f.type))
  return (
    <div className="vp">
      <div className="vp-head"><b>Group rows by</b></div>
      <Select ariaLabel="Group by" value={String(view.config.group_by || '')} onChange={v => patch({ group_by: Number(v) || 0 })} clearLabel="No groups"
        options={[{ value: '', label: 'No groups' }, ...ok.map(f => ({ value: String(f.id), label: f.name, hint: typeInfo(f.type).label }))]} />
      <p className="muted small">Rows with the same value sit together, with a count and the totals of each group.</p>
    </div>
  )
}

export function FieldsPanel({ fields, view, patch }: { fields: Field[]; view: View; patch: Patch }) {
  const hidden = new Set(view.config.hidden)
  const order = orderFields(fields, view)
  const move = (i: number, d: number) => {
    const ids = order.filter(f => !f.primary).map(f => f.id)
    const j = i + d
    if (j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    patch({ order: ids })
  }
  const rest = order.filter(f => !f.primary)
  return (
    <div className="vp">
      <div className="vp-head"><b>Fields in this view</b>
        <span className="row tight">
          <button type="button" className="linkish small" onClick={() => patch({ hidden: [] })}>Show all</button>
          <button type="button" className="linkish small" onClick={() => patch({ hidden: rest.map(f => f.id) })}>Hide all</button>
        </span>
      </div>
      <div className="vp-fields">
        {order.filter(f => f.primary).map(f => <div key={f.id} className="vp-field"><Icon name={typeInfo(f.type).icon} size={14} /><span className="grow">{f.name}</span><span className="muted small">names the rows</span></div>)}
        {rest.map((f, i) => (
          <div key={f.id} className="vp-field">
            <Icon name={typeInfo(f.type).icon} size={14} /><span className="grow ellipsis">{f.name}</span>
            <button type="button" className="btn ghost icon-only xs" aria-label="Move up" disabled={i === 0} onClick={() => move(i, -1)}><Icon name="chevron-up" size={13} /></button>
            <button type="button" className="btn ghost icon-only xs" aria-label="Move down" disabled={i === rest.length - 1} onClick={() => move(i, 1)}><Icon name="chevron-down" size={13} /></button>
            <Switch size="sm" checked={!hidden.has(f.id)} label={`Show ${f.name}`} onChange={v => patch({ hidden: v ? view.config.hidden.filter(x => x !== f.id) : [...view.config.hidden, f.id] })} />
          </div>
        ))}
      </div>
    </div>
  )
}

/** A view's fields in its order: the first field, then the view's order, then the rest. */
export function orderFields(fields: Field[], view: View): Field[] {
  const pos = new Map(view.config.order.map((id, i) => [id, i]))
  return [...fields].sort((a, b) => {
    if (a.primary !== b.primary) return a.primary ? -1 : 1
    const pa = pos.has(a.id) ? pos.get(a.id)! : 1e6 + a.position, pb = pos.has(b.id) ? pos.get(b.id)! : 1e6 + b.position
    return pa - pb
  })
}

export function visibleFields(fields: Field[], view: View): Field[] {
  const hidden = new Set(view.config.hidden)
  return orderFields(fields, view).filter(f => f.primary || !hidden.has(f.id))
}
