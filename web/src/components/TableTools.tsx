// A table's tools: its ties to Pocket Calendar, Forms and Tasks, AI help,
// the bin, bringing rows from a spreadsheet, and its name.
import { useEffect, useRef, useState } from 'react'
import { api, uploadForm, V1 } from '../lib/api'
import { EMOJIS, rowTitle, typeInfo } from '../lib/fields'
import type { Field, FormInfo, Row, RowsResult, TablePage, View } from '../lib/types'
import { ago, fmtDateTime } from '../lib/format'
import { useAppState } from '../lib/data'
import { Icon } from './icons'
import { Select } from './Select'
import { Field as FormField, Loading, Modal, Spinner, Switch, errMsg, useToast } from './ui'

function AppOffNote({ slug, name }: { slug: string; name: string }) {
  const appState = useAppState()
  const s = appState(slug)
  if (s.ok) return null
  return <div className="app-off"><span className="app-off-ico"><Icon name="link" size={15} /></span><div className="grow"><div className="app-off-h">{name} is not connected for you</div><div className="muted small">{s.app?.fix || s.app?.detail || 'Settings → Connected apps.'}</div></div></div>
}

export function NameModal({ title, value, onClose, onSave }: { title: string; value: string; onClose: () => void; onSave: (v: string) => void }) {
  const [v, setV] = useState(value)
  return (
    <Modal title={title} onClose={onClose} size="sm" footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={!v.trim()} onClick={() => onSave(v.trim())}>Save</button></>}>
      <input value={v} autoFocus onChange={e => setV(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && v.trim()) onSave(v.trim()) }} />
    </Modal>
  )
}

export function TableInfoModal({ page, onClose, onSaved }: { page: TablePage; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(page.table.name)
  const [icon, setIcon] = useState(page.table.icon)
  const [desc, setDesc] = useState(page.table.description)
  const { toast } = useToast()
  const save = async () => {
    try { await api.put(`${V1}/tables/${page.table.id}`, { name, icon, description: desc }); onSaved() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title="About this table" onClose={onClose} size="sm" footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save}>Save</button></>}>
      <div className="stack">
        <FormField label="Name"><input value={name} onChange={e => setName(e.target.value)} autoFocus /></FormField>
        <FormField label="Icon"><div className="emoji-pick">{['', ...EMOJIS].map(e => <button key={e || 'none'} type="button" className={icon === e ? 'on' : ''} onClick={() => setIcon(e)} aria-label={e || 'No icon'}>{e || <Icon name="x" size={13} />}</button>)}</div></FormField>
        <FormField label="What it is for"><textarea rows={3} value={desc} onChange={e => setDesc(e.target.value)} placeholder="So that whoever opens it knows" /></FormField>
        <p className="muted small">{page.table.records} rows · {page.fields.length} fields · changed {ago(page.table.updated_at)}</p>
      </div>
    </Modal>
  )
}

export function CalendarModal({ page, onClose, onSaved }: { page: TablePage; onClose: () => void; onSaved: () => void }) {
  const cur = page.table.settings.calendar
  const dates = page.fields.filter(f => f.type === 'date')
  const [on, setOn] = useState(cur?.on ?? true)
  const [field, setField] = useState(cur?.field || dates[0]?.id || 0)
  const [end, setEnd] = useState(cur?.end_field || 0)
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const save = async () => {
    setBusy(true)
    try { await api.put(`${V1}/tables/${page.table.id}`, { calendar: { on, field, end_field: end } }); toast(on ? 'The dates go to Pocket Calendar now' : 'The events are being taken away'); onSaved() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Keep the dates in Pocket Calendar" onClose={onClose} size="sm" footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save} disabled={busy || (on && !field)}>Save</button></>}>
      <div className="stack">
        <AppOffNote slug="calendar" name="Pocket Calendar" />
        <p className="muted">Every row with a date becomes an event in your calendar, named after the row, with its fields and a link back. Change the row and the event changes; delete it and the event goes.</p>
        <label className="check-row"><Switch checked={on} onChange={setOn} /><span>Keep this table’s dates in Pocket Calendar</span></label>
        {dates.length === 0 ? <p className="bad small">The table has no Date field yet.</p> : <>
          <FormField label="The date"><Select ariaLabel="Date" value={String(field)} onChange={v => setField(Number(v))} options={dates.map(f => ({ value: String(f.id), label: f.name + (f.options.time ? ' (with time)' : '') }))} /></FormField>
          <FormField label="Ends (optional)"><Select ariaLabel="End" value={String(end || '')} onChange={v => setEnd(Number(v) || 0)} options={[{ value: '', label: 'One day / the usual length' }, ...dates.filter(f => f.id !== field).map(f => ({ value: String(f.id), label: f.name }))]} /></FormField>
        </>}
      </div>
    </Modal>
  )
}

export function TasksModal({ page, onClose, onSaved }: { page: TablePage; onClose: () => void; onSaved: () => void }) {
  const s = page.table.settings
  const checks = page.fields.filter(f => f.type === 'checkbox'), dates = page.fields.filter(f => f.type === 'date')
  const [done, setDone] = useState(s.task_done || 0)
  const [due, setDue] = useState(s.task_due || 0)
  const { toast } = useToast()
  const save = async () => { try { await api.put(`${V1}/tables/${page.table.id}`, { task_done: done, task_due: due }); onSaved() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Modal title="Tasks made from rows" onClose={onClose} size="sm" footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save}>Save</button></>}>
      <div className="stack">
        <AppOffNote slug="tasks" name="Pocket Tasks" />
        <p className="muted">Open a row and pick <b>Make a task</b> (in ⋯). The task carries the row’s fields and a link back.</p>
        <FormField label="The task is due on"><Select ariaLabel="Due" value={String(due || '')} onChange={v => setDue(Number(v) || 0)} options={[{ value: '', label: 'The first date of the row' }, ...dates.map(f => ({ value: String(f.id), label: f.name }))]} /></FormField>
        <FormField label="When the task is done in Pocket Tasks, tick" help="Checked every ten minutes.">
          <Select ariaLabel="Tick" value={String(done || '')} onChange={v => setDone(Number(v) || 0)} options={[{ value: '', label: 'Nothing (just note it on the row)' }, ...checks.map(f => ({ value: String(f.id), label: f.name }))]} />
        </FormField>
      </div>
    </Modal>
  )
}

export function FormModal({ page, onClose, onSaved }: { page: TablePage; onClose: () => void; onSaved: () => void }) {
  const cur = page.table.settings.form
  const [forms, setForms] = useState<FormInfo[] | null>(null)
  const [off, setOff] = useState('')
  const [formId, setFormId] = useState(cur?.form_id || '')
  const [map, setMap] = useState<Record<string, number>>(cur?.map || {})
  const [make, setMake] = useState(true)
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const appState = useAppState()
  const canMake = appState('forms', 'forms:write').ok
  useEffect(() => { api.get<{ forms: FormInfo[]; off?: string }>(`${V1}/apps/forms`).then(r => { setForms(r.forms); setOff(r.off || '') }).catch(e => setOff(errMsg(e))) }, [])
  const form = forms?.find(f => f.id === formId)
  useEffect(() => {
    if (!form || (cur?.form_id === form.id && Object.keys(map).length)) return
    // a question goes to the field with the same name, when there is one
    const m: Record<string, number> = {}
    for (const q of form.fields) { const f = page.fields.find(x => x.name.toLowerCase() === q.label.toLowerCase()); if (f) m[q.id] = f.id }
    setMap(m)
  }, [formId, forms]) // eslint-disable-line react-hooks/exhaustive-deps
  const plain = page.fields.filter(f => !['lookup', 'rollup', 'formula', 'created', 'updated', 'autonumber', 'link'].includes(f.type))
  const save = async (on: boolean) => {
    setBusy(true)
    try {
      await api.post(`${V1}/tables/${page.table.id}/form`, { on, form_id: formId, title: form?.title || cur?.title || '', map, make_fields: make, questions: form?.fields.map(q => ({ id: q.id, label: q.label, type: q.type, options: q.options || [] })) || [] })
      toast(on ? 'Tied: the answers come in as rows' : 'Untied')
      onSaved()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const makeForm = async () => {
    setBusy(true)
    try { const r = await api.post<{ url: string }>(`${V1}/tables/${page.table.id}/make-form`); toast('A draft form is in Pocket Forms: look it over and open it', 'ok', r.url ? { label: 'Open', run: () => window.open(r.url, '_blank') } : undefined); onSaved() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Rows from a Pocket Forms form" onClose={onClose} footer={<>
      {cur?.on && <button className="btn" onClick={() => save(false)} disabled={busy}>Untie</button>}
      <span className="grow" />
      <button className="btn" onClick={onClose}>Cancel</button>
      <button className="btn primary" onClick={() => save(true)} disabled={busy || !formId}>Tie the form</button>
    </>}>
      <div className="stack">
        <AppOffNote slug="forms" name="Pocket Forms" />
        <p className="muted">Each answer to the form becomes a row here, by itself (looked at every ten minutes). The same answer never comes twice.</p>
        {cur?.on && <p className="small"><Icon name="check" size={13} /> Tied to <b>{cur.title}</b>{cur.made ? ` — ${cur.made} rows so far` : ''}{cur.at && <span className="muted"> · looked {ago(cur.at)}</span>}{cur.status && cur.status !== 'ok' && <span className="bad"> · {cur.status}</span>}</p>}
        <div className="row wrap">
          <button className="btn sm" onClick={makeForm} disabled={busy || !canMake} title={canMake ? undefined : 'Needs Pocket Forms (write)'}><Icon name="plus" size={14} />Make a form from this table’s fields</button>
        </div>
        {forms === null && !off ? <Loading /> : off ? <p className="muted small">{off}</p> : <>
          <FormField label="Or pick a form you have"><Select ariaLabel="Form" value={formId} onChange={setFormId} placeholder="Pick a form" options={forms!.map(f => ({ value: f.id, label: `${f.emoji ? f.emoji + ' ' : ''}${f.title}`, hint: `${f.responses} answers` }))} /></FormField>
          {form && <>
            <div className="map-list">
              {form.fields.map(q => (
                <div key={q.id} className="map-row">
                  <span className="grow ellipsis">{q.label}</span>
                  <Icon name="chevron-right" size={14} className="muted" />
                  <Select ariaLabel={`Field for ${q.label}`} value={String(map[q.id] || '')} onChange={v => setMap(m => { const n = { ...m }; if (v) n[q.id] = Number(v); else delete n[q.id]; return n })}
                    options={[{ value: '', label: make ? 'A new field' : 'Leave out' }, ...plain.map(f => ({ value: String(f.id), label: f.name, hint: typeInfo(f.type).label }))]} />
                </div>
              ))}
            </div>
            <label className="check-row"><Switch checked={make} onChange={setMake} /><span>Questions without a field get a new one</span></label>
          </>}
        </>}
      </div>
    </Modal>
  )
}

export function FillModal({ field, view, onClose, onDone }: { field: Field; view: View; onClose: () => void; onDone: () => void }) {
  const [how, setHow] = useState('')
  const [onlyEmpty, setOnlyEmpty] = useState(true)
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const go = async () => {
    setBusy(true)
    try { const r = await api.post<{ filled: number; looked: number }>(`${V1}/fields/${field.id}/ai-fill`, { instruction: how, only_empty: onlyEmpty, view: view.id }); toast(`Filled ${r.filled} of ${r.looked} rows`); onDone() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const examples = field.type === 'select' ? 'Sort each into one of the options from its description' : field.type === 'longtext' ? 'Sum up the notes in one sentence' : field.type === 'number' || field.type === 'currency' ? 'Guess the price from the name' : 'Write a short title from the description'
  return (
    <Modal title={`Fill “${field.name}” with AI`} onClose={onClose} size="sm" footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn primary" onClick={go} disabled={busy || !how.trim()}>{busy ? <Spinner /> : <Icon name="sparkles" size={14} />}Fill</button></>}>
      <div className="stack">
        <FormField label="What should go in it?" help="The AI reads the other fields of each row (up to 200 rows of this view).">
          <textarea rows={3} autoFocus value={how} onChange={e => setHow(e.target.value)} placeholder={examples} />
        </FormField>
        <label className="check-row"><Switch checked={onlyEmpty} onChange={setOnlyEmpty} /><span>Only the rows where it is empty</span></label>
        <p className="muted small">Each change shows in the row’s story, marked “filled by AI”.</p>
      </div>
    </Modal>
  )
}

export function AskModal({ page, view, onClose }: { page: TablePage; view: View; onClose: () => void }) {
  const [q, setQ] = useState('')
  const [a, setA] = useState('')
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const ask = async () => {
    setBusy(true); setA('')
    try { setA((await api.post<{ text: string }>(`${V1}/tables/${page.table.id}/ai-ask`, { question: q, view: view.id })).text) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={`Ask about ${page.table.name}`} onClose={onClose}>
      <div className="stack">
        <div className="row">
          <input className="grow" autoFocus value={q} onChange={e => setQ(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && q.trim()) ask() }} placeholder="Which supplier is cheapest? What is due this week?" />
          <button className="btn primary" onClick={ask} disabled={busy || !q.trim()}>{busy ? <Spinner /> : 'Ask'}</button>
        </div>
        <p className="muted small">The AI reads the rows of the view “{view.name}”.</p>
        {a && <div className="ai-answer pre">{a}</div>}
      </div>
    </Modal>
  )
}

export function BinModal({ page, onClose, onChanged }: { page: TablePage; onClose: () => void; onChanged: () => void }) {
  const [rows, setRows] = useState<Row[] | null>(null)
  const { toast } = useToast()
  const load = () => api.get<RowsResult>(`${V1}/tables/${page.table.id}/records?deleted=1`).then(r => setRows(r.records)).catch(e => toast(errMsg(e), 'err'))
  useEffect(() => { load() }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const restore = async (r: Row) => { await api.post(`${V1}/records/${r.id}/restore`); toast('Brought back'); load(); onChanged() }
  const purge = async (r: Row) => { await api.del(`${V1}/records/${r.id}/purge`); load(); onChanged() }
  return (
    <Modal title="Deleted rows" onClose={onClose}>
      <p className="muted small">Deleted rows stay here 30 days, then they go for good.</p>
      {rows === null ? <Loading /> : rows.length === 0 ? <p className="muted">Nothing in the bin.</p> : (
        <ul className="list">
          {rows.map(r => (
            <li key={r.id} className="list-row">
              <div className="grow"><div className="row-title">{rowTitle(r)}</div><div className="muted small">deleted {r.deleted_at ? fmtDateTime(r.deleted_at) : ''}</div></div>
              <button className="btn sm" onClick={() => restore(r)}>Bring back</button>
              <button className="btn sm ghost" onClick={() => purge(r)} aria-label="Remove for good" title="Remove for good"><Icon name="trash" size={14} /></button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  )
}

export function ImportRowsModal({ page, onClose, onDone }: { page: TablePage; onClose: () => void; onDone: () => void }) {
  const [text, setText] = useState('')
  const [addFields, setAddFields] = useState(true)
  const [busy, setBusy] = useState(false)
  const file = useRef<HTMLInputElement>(null)
  const { toast } = useToast()
  const done = (n: number) => { toast(`${n} rows added`); onDone() }
  const send = async () => {
    setBusy(true)
    try { const r = await api.post<{ made: number }>(`${V1}/tables/${page.table.id}/import?add_fields=${addFields ? 1 : 0}`, { text }); done(r.made) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const sendFile = async (f?: File) => {
    if (!f) return
    setBusy(true)
    try { const r = await uploadForm<{ made: number }>(`${V1}/tables/${page.table.id}/import`, f, { add_fields: addFields ? '1' : '0' }); done(r.made) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Bring rows from a spreadsheet" onClose={onClose} footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn primary" onClick={send} disabled={busy || !text.trim()}>Add the rows</button></>}>
      <div className="stack">
        <p className="muted">A CSV file (from Excel, Google Sheets, Numbers, Airtable…) or rows pasted straight from a spreadsheet. The first line names the columns; they go to the fields with the same names.</p>
        <div className="row"><input ref={file} type="file" accept=".csv,.tsv,.txt,text/csv" hidden onChange={e => sendFile(e.target.files?.[0])} /><button className="btn" onClick={() => file.current?.click()} disabled={busy}><Icon name="upload" size={14} />Choose a file</button>{busy && <Spinner />}</div>
        <textarea rows={8} className="mono" value={text} onChange={e => setText(e.target.value)} placeholder={`${page.fields.slice(0, 3).map(f => f.name).join('\t')}\n…`} />
        <label className="check-row"><Switch checked={addFields} onChange={setAddFields} /><span>Columns without a field become new fields</span></label>
      </div>
    </Modal>
  )
}
