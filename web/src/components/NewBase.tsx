// Starting a base: blank, ready-made, described to AI, from a spreadsheet,
// from your other Pocket apps or from a file in Pocket Files.
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, uploadForm, V1 } from '../lib/api'
import { announceChange, useAppState, useData } from '../lib/data'
import { EMOJIS, typeInfo } from '../lib/fields'
import type { Base, FieldType, SpecBase, Table, Template } from '../lib/types'
import { fmtBytes } from '../lib/format'
import { Icon } from './icons'
import { Field, Loading, Modal, Spinner, Switch, errMsg, useToast } from './ui'

export type NewHow = 'start' | 'blank' | 'template' | 'ai' | 'sheet' | 'apps' | 'files'

const WAYS: { how: NewHow; icon: string; title: string; sub: string }[] = [
  { how: 'blank', icon: 'plus', title: 'Blank', sub: 'One empty table to start from' },
  { how: 'template', icon: 'gallery', title: 'Ready-made', sub: 'Projects, bookings, suppliers, a collection…' },
  { how: 'ai', icon: 'sparkles', title: 'Describe it', sub: 'Say what you keep track of; AI sets up the tables' },
  { how: 'sheet', icon: 'upload', title: 'A spreadsheet', sub: 'Excel or Google Sheets as CSV, an Airtable export, a paste' },
  { how: 'apps', icon: 'layers', title: 'Your Pocket apps', sub: 'Contacts, places, books, things, deals, wishes, payments' },
  { how: 'files', icon: 'file', title: 'Pocket Files', sub: 'A spreadsheet file you keep there' },
]

export function NewBase({ how: how0, baseId, onClose }: { how: NewHow; baseId?: number; onClose: () => void }) {
  const [how, setHow] = useState<NewHow>(how0)
  const navigate = useNavigate()
  const done = (tableId?: number, baseIdOut?: number) => {
    announceChange()
    onClose()
    if (tableId) navigate(`/t/${tableId}`)
    else if (baseIdOut) navigate(`/b/${baseIdOut}`)
  }
  const title = baseId ? 'Add a table' : how === 'start' ? 'New base' : WAYS.find(w => w.how === how)?.title || 'New base'
  return (
    <Modal title={<span className="row tight">{how !== 'start' && <button type="button" className="btn ghost icon-only sm" onClick={() => setHow('start')} aria-label="Back"><Icon name="chevron-left" size={16} /></button>}{title}</span>} onClose={onClose} size="lg">
      {how === 'start' && (
        <div className="ways">
          {WAYS.filter(w => !(baseId && (w.how === 'blank' || w.how === 'template'))).map(w => (
            <button key={w.how} type="button" className="way" onClick={() => setHow(w.how)}>
              <span className="way-ico"><Icon name={w.icon} size={20} /></span>
              <span className="way-t">{w.title}</span><span className="muted small">{w.sub}</span>
            </button>
          ))}
        </div>
      )}
      {how === 'blank' && <Blank onDone={done} />}
      {how === 'template' && <Templates onDone={done} />}
      {how === 'ai' && <AIDesign baseId={baseId} onDone={done} />}
      {how === 'sheet' && <Sheet baseId={baseId} onDone={done} />}
      {how === 'apps' && <FromApps baseId={baseId} onDone={done} />}
      {how === 'files' && <FromFiles baseId={baseId} onDone={done} />}
    </Modal>
  )
}

type Done = (tableId?: number, baseId?: number) => void

function Blank({ onDone }: { onDone: Done }) {
  const [name, setName] = useState('')
  const [icon, setIcon] = useState('📋')
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const go = async () => {
    setBusy(true)
    try { const b = await api.post<Base>(`${V1}/bases`, { name: name.trim() || 'My base', icon }); onDone(b.tables[0]?.id, b.id) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack">
      <Field label="Name"><input value={name} autoFocus onChange={e => setName(e.target.value)} placeholder="Home, the shop, my research…" onKeyDown={e => { if (e.key === 'Enter') go() }} /></Field>
      <Field label="Icon"><div className="emoji-pick">{EMOJIS.map(e => <button key={e} type="button" className={icon === e ? 'on' : ''} onClick={() => setIcon(e)}>{e}</button>)}</div></Field>
      <div className="row end"><button type="button" className="btn primary" onClick={go} disabled={busy}>Make the base</button></div>
    </div>
  )
}

function Templates({ onDone }: { onDone: Done }) {
  const [list, setList] = useState<Template[] | null>(null)
  const [examples, setExamples] = useState(true)
  const [busy, setBusy] = useState('')
  const { toast } = useToast()
  useEffect(() => { api.get<Template[]>(`${V1}/templates`).then(setList).catch(e => toast(errMsg(e), 'err')) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const use = async (t: Template) => {
    setBusy(t.key)
    try { const b = await api.post<Base>(`${V1}/templates/${t.key}`, { examples }); onDone(b.tables[0]?.id, b.id) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  if (!list) return <Loading />
  return (
    <div className="stack">
      <label className="check-row"><Switch checked={examples} onChange={setExamples} /><span>With a few example rows <span className="muted small">— easy to delete later</span></span></label>
      <div className="tpl-grid">
        {list.map(t => (
          <button key={t.key} type="button" className="tpl" onClick={() => use(t)} disabled={!!busy} style={{ ['--tile' as string]: t.color }}>
            <span className="tpl-ico">{t.icon}</span>
            <span className="tpl-t">{t.name}{busy === t.key && <Spinner />}</span>
            <span className="muted small">{t.description}</span>
            <span className="tpl-tables">{t.tables.map(n => <span key={n} className="lpill quiet">{n}</span>)}</span>
          </button>
        ))}
      </div>
    </div>
  )
}

function AIDesign({ baseId, onDone }: { baseId?: number; onDone: Done }) {
  const [prompt, setPrompt] = useState('')
  const [spec, setSpec] = useState<SpecBase | null>(null)
  const [rows, setRows] = useState(true)
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const appState = useAppState()
  const design = async () => {
    setBusy(true); setSpec(null)
    try { setSpec(await api.post<SpecBase>(`${V1}/ai/design`, { prompt })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const make = async () => {
    if (!spec) return
    setBusy(true)
    try {
      const b = await api.post<Base>(`${V1}/bases/build`, { spec, rows, base_id: baseId || 0 })
      const first = baseId ? b.tables[b.tables.length - spec.tables.length] : b.tables[0]
      onDone(first?.id, b.id)
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  void appState
  return (
    <div className="stack">
      <Field label="What do you want to keep track of?" help="A sentence or two is enough. You see what AI suggests before anything is made.">
        <textarea rows={3} autoFocus value={prompt} onChange={e => setPrompt(e.target.value)} placeholder="The plants in my garden: when I sowed them, where they grow, when to water and what I harvested." />
      </Field>
      <div className="row"><button type="button" className="btn primary" onClick={design} disabled={busy || !prompt.trim()}>{busy && !spec ? <Spinner /> : <Icon name="sparkles" size={14} />}{spec ? 'Try again' : 'Suggest the tables'}</button></div>
      {spec && <>
        <div className="ai-spec">
          <h3>{spec.icon} {spec.name}</h3>
          {spec.description && <p className="muted">{spec.description}</p>}
          {spec.tables.map(t => (
            <div key={t.name} className="ai-table">
              <b>{t.icon} {t.name}</b>
              <div className="pills wrap">{t.fields.map(f => <span key={f.name} className="lpill quiet" title={typeInfo(f.type as FieldType).label}>{f.name}<span className="muted"> · {typeInfo(f.type as FieldType).label.toLowerCase()}{f.link ? ' → ' + f.link : ''}</span></span>)}</div>
              {t.rows && t.rows.length > 0 && <span className="muted small">{t.rows.length} example rows</span>}
            </div>
          ))}
        </div>
        <label className="check-row"><Switch checked={rows} onChange={setRows} /><span>Keep the example rows</span></label>
        <div className="row end"><button type="button" className="btn primary" onClick={make} disabled={busy}>{busy ? <Spinner /> : null}Make it</button></div>
      </>}
    </div>
  )
}

function Sheet({ baseId, onDone }: { baseId?: number; onDone: Done }) {
  const [text, setText] = useState('')
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const file = useRef<HTMLInputElement>(null)
  const { toast } = useToast()
  const url = baseId ? `${V1}/bases/${baseId}/import` : `${V1}/import`
  type Out = { table?: Table; made?: number; records?: number }
  const finish = (r: Out) => { toast(`${r.made ?? r.records ?? 0} rows brought in`); onDone(r.table?.id) }
  const sendFile = async (f?: File) => {
    if (!f) return
    setBusy(true)
    try { finish(await uploadForm<Out>(url, f, { name: name || f.name.replace(/\.[^.]+$/, '') })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const sendText = async () => {
    setBusy(true)
    try { finish(await api.post<Out>(url, { text, name: name || 'Pasted' })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack">
      <p className="muted">The first line names the columns. Pocket Tables guesses what each column is — dates, numbers, money, emails, choices — and you can change any field later. From Excel or Google Sheets: save as CSV, or copy the cells and paste them below. From Airtable: download a view as CSV.</p>
      <Field label="Name of the table (optional)"><input value={name} onChange={e => setName(e.target.value)} placeholder="From the file’s name" /></Field>
      <div className="drop" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); sendFile(e.dataTransfer.files[0]) }}>
        <input ref={file} type="file" accept=".csv,.tsv,.txt,.json,text/csv" hidden onChange={e => sendFile(e.target.files?.[0])} />
        <button type="button" className="btn" onClick={() => file.current?.click()} disabled={busy}>{busy ? <Spinner /> : <Icon name="upload" size={15} />}Choose a file</button>
        <span className="muted small">or drop it here (CSV, or a Pocket Tables export)</span>
      </div>
      <Field label="…or paste the cells"><textarea rows={6} className="mono" value={text} onChange={e => setText(e.target.value)} placeholder={'Name\tEmail\tTeam\nAna\tana@example.com\tRed'} /></Field>
      <div className="row end"><button type="button" className="btn primary" onClick={sendText} disabled={busy || !text.trim()}>Bring it in</button></div>
    </div>
  )
}

function FromApps({ baseId, onDone }: { baseId?: number; onDone: Done }) {
  const { kinds } = useData()
  const [busy, setBusy] = useState('')
  const { toast } = useToast()
  const go = async (kind: string) => {
    setBusy(kind)
    try { const r = await api.post<{ table: Table; made: number }>(`${V1}/from-app`, { kind, base_id: baseId || 0 }); toast(`${r.made} rows made`); onDone(r.table.id) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  if (!kinds) return <Loading />
  const list = kinds.filter(k => k.can_import)
  return (
    <div className="stack">
      <p className="muted">Makes a table with one row for each thing you have there, each linked back to it. Add your own fields — a rating, a note, a link to another table. Later, the table’s ⋯ menu brings in anything new.</p>
      <div className="ways">
        {list.map(k => (
          <button key={k.slug} type="button" className={`way ${k.ok ? '' : 'off'}`} disabled={!k.ok || !!busy} onClick={() => go(k.slug)} title={k.ok ? undefined : `${k.app_name}: ${k.fix || k.detail}`}>
            <span className="way-ico"><Icon name={k.icon} size={20} /></span>
            <span className="way-t">{k.plural[0].toUpperCase() + k.plural.slice(1)}{busy === k.slug && <Spinner />}</span>
            <span className="muted small">{k.ok ? `From ${k.app_name}` : `${k.app_name} is not connected`}</span>
          </button>
        ))}
      </div>
      {list.some(k => !k.ok) && <p className="muted small">Greyed apps need a link switched on in Pocket Core (Links), or a connection in Settings → Connected apps.</p>}
    </div>
  )
}

function FromFiles({ baseId, onDone }: { baseId?: number; onDone: Done }) {
  const [space, setSpace] = useState('home')
  const [path, setPath] = useState('/')
  const [data, setData] = useState<{ entries: { name: string; path: string; dir: boolean; size: number }[]; spaces?: { id: string; name: string }[]; off?: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  useEffect(() => { setData(null); api.get<typeof data>(`${V1}/apps/files?space=${encodeURIComponent(space)}&path=${encodeURIComponent(path)}`).then(setData).catch(e => setData({ entries: [], off: errMsg(e) })) }, [space, path])
  const pick = async (p: string, name: string) => {
    setBusy(true)
    try {
      let bid = baseId
      if (!bid) bid = (await api.post<Base>(`${V1}/bases`, { name: name.replace(/\.[^.]+$/, ''), icon: '📄', empty: true })).id
      const r = await api.post<{ table: Table }>(`${V1}/bases/${bid}/from-files`, { space, path: p })
      onDone(r.table.id)
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  if (!data) return <Loading />
  if (data.off) return <div className="app-off"><span className="app-off-ico"><Icon name="file" size={15} /></span><div className="grow"><div className="app-off-h">Pocket Files is not reachable</div><div className="muted small">{data.off}</div></div></div>
  const up = path !== '/' ? path.replace(/\/[^/]+\/?$/, '') || '/' : ''
  return (
    <div className="stack">
      <div className="row">
        {data.spaces && data.spaces.length > 1 && <div className="segmented">{data.spaces.slice(0, 4).map(s => <button key={s.id} type="button" className={s.id === space ? 'active' : ''} onClick={() => { setSpace(s.id); setPath('/') }}>{s.name}</button>)}</div>}
        <span className="muted small ellipsis">{path}</span>
      </div>
      <ul className="list files-list">
        {up && <li><button type="button" className="list-row" onClick={() => setPath(up)}><Icon name="chevron-left" size={15} />Up</button></li>}
        {data.entries.map(e => (
          <li key={e.path}><button type="button" className="list-row" disabled={busy} onClick={() => (e.dir ? setPath(e.path) : pick(e.path, e.name))}>
            <Icon name={e.dir ? 'archive' : 'file'} size={15} /><span className="grow ellipsis">{e.name}</span>{!e.dir && <span className="muted small">{fmtBytes(e.size)}</span>}
          </button></li>
        ))}
        {data.entries.length === 0 && <li className="muted small pad-s">No folders or spreadsheets (CSV) here.</li>}
      </ul>
      {busy && <Loading label="Bringing it in…" />}
    </div>
  )
}
