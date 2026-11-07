// Add or change a field: its name, type and what the type needs.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { COLORS, CURRENCIES, ROLLUPS, TYPES, typeInfo } from '../lib/fields'
import type { Choice, Field, FieldOptions, FieldType, TablePage } from '../lib/types'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { Icon } from './icons'
import { Select } from './Select'
import { Field as FormField, Modal, Switch, errMsg, useToast } from './ui'

export function FieldEditor({ page, field, after, onClose, onSaved }: { page: TablePage; field?: Field; after?: number; onClose: () => void; onSaved: (f: Field) => void }) {
  const { settings } = useApp()
  const { kinds, bases } = useData()
  const { toast } = useToast()
  const [name, setName] = useState(field?.name || '')
  const [type, setType] = useState<FieldType>(field?.type || 'text')
  const [opt, setOpt] = useState<FieldOptions>(field?.options || {})
  const [desc, setDesc] = useState(field?.description || '')
  const [busy, setBusy] = useState(false)
  const [fns, setFns] = useState<{ name: string; takes: string; example: string }[] | null>(null)
  const o = (patch: Partial<FieldOptions>) => setOpt(x => ({ ...x, ...patch }))
  const isLink = field?.type === 'link'
  useEffect(() => {
    if (type === 'currency' && !opt.currency) o({ currency: settings?.currency || 'EUR' })
    if (type === 'rating' && !opt.max) o({ max: 5 })
  }, [type]) // eslint-disable-line react-hooks/exhaustive-deps

  const links = page.fields.filter(f => f.type === 'link')
  const linkField = links.find(f => f.id === opt.link_field)
  const [linkedFields, setLinkedFields] = useState<Field[]>([])
  useEffect(() => {
    if (!linkField?.options.table_id) { setLinkedFields([]); return }
    api.get<TablePage>(`${V1}/tables/${linkField.options.table_id}`).then(p => setLinkedFields(p.fields)).catch(() => setLinkedFields([]))
  }, [linkField?.options.table_id])

  const allTables = (bases || []).flatMap(b => b.tables.map(t => ({ ...t, baseName: b.name })))
  const save = async () => {
    setBusy(true)
    try {
      const body = { name, type, options: opt, description: desc, after }
      const f = field ? await api.put<Field>(`${V1}/fields/${field.id}`, body) : await api.post<Field>(`${V1}/tables/${page.table.id}/fields`, body)
      onSaved(f)
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const typeOptions = TYPES.filter(t => (isLink ? t.type === 'link' : !(field && t.type === 'link')))
    .filter(t => !(field?.primary && ['link', 'attachment', 'checkbox', 'lookup', 'rollup', 'pocket'].includes(t.type)))
  const choices = opt.choices || []
  const setChoice = (i: number, c: Partial<Choice>) => o({ choices: choices.map((x, j) => (j === i ? { ...x, ...c } : x)) })
  return (
    <Modal title={field ? `Field: ${field.name}` : 'New field'} onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={save} disabled={busy}>{field ? 'Save' : 'Add field'}</button>
    </>}>
      <div className="stack">
        <FormField label="Name"><input value={name} autoFocus onChange={e => setName(e.target.value)} placeholder={typeInfo(type).label} onKeyDown={e => { if (e.key === 'Enter' && !['select', 'multiselect', 'formula'].includes(type)) save() }} /></FormField>
        <FormField label="Type" help={typeInfo(type).help}>
          {isLink ? <div className="muted small">A link to another table stays one; add a new field for something else.</div>
            : <Select ariaLabel="Type" value={type} onChange={v => setType(v as FieldType)} options={typeOptions.map(t => ({ value: t.type, label: t.label, hint: t.group === 'Basic' ? undefined : t.group, group: t.group }))} />}
          {field && field.type !== type && !['lookup', 'rollup', 'formula', 'created', 'updated', 'autonumber'].includes(type) && <p className="muted small mt-s">The values are changed into the new type where they fit; the rest are cleared.</p>}
        </FormField>

        {(type === 'number' || type === 'percent' || type === 'currency' || type === 'formula' || type === 'rollup') && (
          <FormField label="Decimals"><Select ariaLabel="Decimals" value={String(opt.decimals ?? (type === 'currency' ? 2 : 0))} onChange={v => o({ decimals: Number(v) })} options={[0, 1, 2, 3, 4].map(n => ({ value: String(n), label: n === 0 ? (type === 'currency' ? 'None' : 'As needed') : n === 1 ? '1.0' : '1.' + '0'.repeat(n) }))} /></FormField>
        )}
        {type === 'currency' && <FormField label="Currency"><Select ariaLabel="Currency" value={opt.currency || 'EUR'} onChange={v => o({ currency: v })} options={Array.from(new Set([opt.currency || 'EUR', ...CURRENCIES])).map(c => ({ value: c, label: c }))} /></FormField>}
        {type === 'rating' && <FormField label="Stars"><Select ariaLabel="Stars" value={String(opt.max || 5)} onChange={v => o({ max: Number(v) })} options={[3, 4, 5, 6, 7, 8, 9, 10].map(n => ({ value: String(n), label: String(n) }))} /></FormField>}
        {type === 'date' && <>
          <label className="check-row"><Switch checked={!!opt.time} onChange={v => o({ time: v })} /><span>With a time of day</span></label>
          <label className="check-row"><Switch checked={!!opt.remind} onChange={v => o({ remind: v })} /><span>Remind me on the day <span className="muted small">— in the morning message and on Home</span></span></label>
        </>}
        {(type === 'select' || type === 'multiselect') && (
          <FormField label="Options">
            <div className="choices-edit">
              {choices.map((c, i) => (
                <div key={c.id || i} className="choice-edit-row">
                  <button type="button" className="swatch-pick" style={{ background: c.color }} title="Another colour" aria-label="Another colour"
                    onClick={() => setChoice(i, { color: COLORS[(COLORS.indexOf(c.color) + 1) % COLORS.length] })} />
                  <input value={c.name} onChange={e => setChoice(i, { name: e.target.value })} aria-label="Option" />
                  <button type="button" className="btn ghost icon-only sm" disabled={i === 0} onClick={() => { const n = [...choices]; [n[i - 1], n[i]] = [n[i], n[i - 1]]; o({ choices: n }) }} aria-label="Move up"><Icon name="chevron-up" size={14} /></button>
                  <button type="button" className="btn ghost icon-only sm" onClick={() => o({ choices: choices.filter((_, j) => j !== i) })} aria-label="Remove"><Icon name="x" size={14} /></button>
                </div>
              ))}
              <button type="button" className="btn sm" onClick={() => o({ choices: [...choices, { id: '', name: '', color: COLORS[choices.length % COLORS.length] }] })}><Icon name="plus" size={14} />Add an option</button>
            </div>
          </FormField>
        )}
        {type === 'link' && !isLink && (
          <FormField label="Link to rows of" help="The other table gets a field that shows the links back.">
            <Select ariaLabel="Table" value={String(opt.table_id || '')} onChange={v => o({ table_id: Number(v) })} placeholder="Pick a table" options={allTables.map(t => ({ value: String(t.id), label: t.name, group: t.baseName }))} />
          </FormField>
        )}
        {type === 'link' && <label className="check-row"><Switch checked={!!opt.single} onChange={v => o({ single: v })} /><span>Only one linked row</span></label>}
        {(type === 'lookup' || type === 'rollup') && (links.length === 0
          ? <div className="app-off compact"><span className="app-off-ico"><Icon name="link-row" size={15} /></span><div className="grow"><div className="app-off-h">Add a link to another table first</div><div className="muted small">A look-up and a total read the rows a link points at.</div></div></div>
          : <>
            <FormField label="Through the link"><Select ariaLabel="Link" value={String(opt.link_field || '')} onChange={v => o({ link_field: Number(v), field: 0 })} placeholder="Pick a link field" options={links.map(f => ({ value: String(f.id), label: f.name }))} /></FormField>
            {linkField && <FormField label="Which field of the linked rows"><Select ariaLabel="Field" value={String(opt.field || '')} onChange={v => o({ field: Number(v) })} placeholder="Pick a field" options={linkedFields.map(f => ({ value: String(f.id), label: f.name, hint: typeInfo(f.type).label }))} /></FormField>}
            {type === 'rollup' && <FormField label="What to work out"><Select ariaLabel="Total" value={opt.fn || 'sum'} onChange={v => o({ fn: v })} options={ROLLUPS} /></FormField>}
          </>)}
        {type === 'formula' && (
          <FormField label="Formula" help={<>Use fields in braces, like <code>{'{Price} * {Quantity}'}</code>. <button type="button" className="linkish" onClick={() => api.get<typeof fns>(`${V1}/formula/functions`).then(setFns)}>What can I use?</button></>}>
            <textarea rows={3} className="mono" value={opt.expr || ''} onChange={e => o({ expr: e.target.value })} placeholder={'IF({Done}, "✓", DAYS({Due}, TODAY()) & " days left")'} />
            <div className="chips mt-s">{page.fields.filter(f => f.id !== field?.id).map(f => <button key={f.id} type="button" className="chip-btn" onClick={() => o({ expr: (opt.expr || '') + `{${f.name}}` })}>{f.name}</button>)}</div>
            {fns && <div className="fn-help">{fns!.map(f => <div key={f.name}><code>{f.name}</code> <span className="muted small">{f.takes}{f.example && <> — <code>{f.example}</code></>}</span></div>)}</div>}
          </FormField>
        )}
        {type === 'pocket' && (
          <>
            <FormField label="Points at" help="Pick from your other Pocket apps; the cell keeps the name and a link.">
              <Select ariaLabel="Kind" value={opt.app || ''} onChange={v => o({ app: v })} placeholder="Pick what" options={(kinds || []).map(k => ({ value: k.slug, label: `${k.label} — ${k.app_name}`, hint: k.ok ? undefined : 'not connected' }))} />
            </FormField>
            <label className="check-row"><Switch checked={!!opt.single} onChange={v => o({ single: v })} /><span>Only one</span></label>
          </>
        )}
        <FormField label="Description (optional)"><input value={desc} onChange={e => setDesc(e.target.value)} placeholder="What goes here, for whoever fills it in" /></FormField>
      </div>
    </Modal>
  )
}
