// One table: its views as tabs, the toolbar, and the grid, board, calendar or gallery.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useAppState, useData, useOnChange } from '../lib/data'
import type { Field, Row, RowsResult, TablePage as Page, View, ViewConfig, ViewType } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { type Anchor, Confirm, Loading, Menu, type MenuItem, MenuButton, Popover, rectOf, useDebounce, useIsMobile, useToast, errMsg } from '../components/ui'
import { GridView } from '../components/GridView'
import { BoardView, CalendarView, GalleryView } from '../components/OtherViews'
import { FieldsPanel, FilterPanel, GroupPanel, SortPanel } from '../components/ViewPanels'
import { FieldEditor } from '../components/FieldEditor'
import { RecordModal } from '../components/RecordModal'
import { Select } from '../components/Select'
import { AskModal, BinModal, CalendarModal, FillModal, FormModal, ImportRowsModal, NameModal, TableInfoModal, TasksModal } from '../components/TableTools'

const VIEW_ICON: Record<ViewType, string> = { grid: 'table', board: 'board', calendar: 'calendar', gallery: 'gallery' }
const VIEW_NAME: Record<ViewType, string> = { grid: 'Grid', board: 'Board', calendar: 'Calendar', gallery: 'Gallery' }

type Tool = '' | 'calendar' | 'form' | 'tasks' | 'ask' | 'bin' | 'import' | 'info'

export default function TablePage() {
  const { id } = useParams()
  const tid = Number(id)
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { toast } = useToast()
  const mobile = useIsMobile()
  const appState = useAppState()
  const { bases, kinds } = useData()
  const [page, setPage] = useState<Page | null>(null)
  const [err, setErr] = useState('')
  const [rows, setRows] = useState<Row[] | null>(null)
  const [sums, setSums] = useState<Record<string, unknown> | undefined>()
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [panel, setPanel] = useState<{ which: 'filter' | 'sort' | 'group' | 'fields'; a: Anchor } | null>(null)
  const [fieldMenu, setFieldMenu] = useState<{ f: Field; a: Anchor } | null>(null)
  const [fieldEdit, setFieldEdit] = useState<{ f?: Field; after?: number } | null>(null)
  const [fill, setFill] = useState<Field | null>(null)
  const [tool, setTool] = useState<Tool>('')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [confirm, setConfirm] = useState<null | { title: string; msg: string; label: string; run: () => void }>(null)
  const [rename, setRename] = useState<View | null>(null)

  const loadPage = useCallback(async () => {
    try { setPage(await api.get<Page>(`${V1}/tables/${tid}`)); setErr('') } catch (e) { setErr(errMsg(e)) }
  }, [tid])
  useEffect(() => { setPage(null); setRows(null); setSelected(new Set()); setQ(''); loadPage() }, [loadPage])

  const view: View | undefined = useMemo(() => {
    if (!page) return undefined
    const want = Number(sp.get('view') || 0)
    return page.views.find(v => v.id === want) || page.views[0]
  }, [page, sp])
  useCrumb(page ? `${page.table.icon ? page.table.icon + ' ' : ''}${page.table.name}` : '')

  const loadRows = useCallback(async () => {
    if (!view) return
    try {
      const r = await api.get<RowsResult>(`${V1}/tables/${tid}/records?view=${view.id}&q=${encodeURIComponent(dq)}`)
      setRows(r.records); setSums(r.summaries)
    } catch (e) { toast(errMsg(e), 'err') }
  }, [tid, view, dq]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { loadRows() }, [loadRows])
  const reloadAll = useCallback(() => { loadPage(); loadRows() }, [loadPage, loadRows])
  useOnChange(loadRows)
  // quietly look again when coming back to the tab (another app may have added rows)
  useEffect(() => {
    const h = () => { if (document.visibilityState === 'visible') loadRows() }
    document.addEventListener('visibilitychange', h)
    return () => document.removeEventListener('visibilitychange', h)
  }, [loadRows])

  const rowId = Number(sp.get('row') || 0)
  const openRow = (rid: number) => { const n = new URLSearchParams(sp); n.set('row', String(rid)); setSp(n) }
  const closeRow = () => { const n = new URLSearchParams(sp); n.delete('row'); setSp(n, { replace: true }) }
  const setView = (vid: number) => { const n = new URLSearchParams(sp); n.set('view', String(vid)); n.delete('row'); setSp(n, { replace: true }) }

  // saving a view's settings: at once on the page, then on the server
  const saveTimer = useRef<number>(0)
  const patchView = useCallback((c: Partial<ViewConfig>) => {
    if (!view || !page) return
    const nv = { ...view, config: { ...view.config, ...c } }
    setPage(p => p && { ...p, views: p.views.map(v => (v.id === nv.id ? nv : v)) })
    window.clearTimeout(saveTimer.current)
    saveTimer.current = window.setTimeout(() => { api.put(`${V1}/views/${nv.id}`, { config: nv.config }).catch(e => toast(errMsg(e), 'err')) }, 300)
  }, [view, page, toast])
  // filters, sorts and grouping change what shows: look again
  const shape = view ? JSON.stringify([view.config.filters, view.config.match, view.config.sorts, view.config.group_by, view.config.summaries]) : ''
  const firstShape = useRef(true)
  useEffect(() => {
    if (firstShape.current) { firstShape.current = false; return }
    const t = window.setTimeout(loadRows, 400)
    return () => window.clearTimeout(t)
  }, [shape]) // eslint-disable-line react-hooks/exhaustive-deps

  const onSet = useCallback(async (row: Row, f: Field, v: unknown, typecast = false) => {
    if (!typecast && f.type !== 'link') setRows(rs => rs && rs.map(r => (r.id === row.id ? { ...r, values: { ...r.values, [f.id]: v === null ? undefined : v } } : r)))
    try {
      const r = await api.put<Row>(`${V1}/records/${row.id}`, { values: { [f.id]: v }, typecast })
      setRows(rs => rs && rs.map(x => (x.id === r.id ? { ...r, comments: x.comments } : x)))
      if (typecast) loadPage()
    } catch (e) { toast(errMsg(e), 'err'); loadRows() }
  }, [toast, loadPage, loadRows])

  const onAdd = useCallback(async (values: Record<string, unknown> = {}) => {
    if (!view) return
    // a new row starts with what the filters ask for, so it does not vanish
    const start: Record<string, unknown> = { ...values }
    for (const fl of view.config.filters) {
      const f = page?.fields.find(x => x.id === fl.field)
      if (!f || start[f.id] !== undefined) continue
      if (fl.op === 'checked') start[f.id] = true
      if ((fl.op === 'any_of' || fl.op === 'is') && f.type === 'select' && Array.isArray(fl.value) && fl.value.length === 1) start[f.id] = fl.value[0]
    }
    try {
      const r = await api.post<Row>(`${V1}/tables/${tid}/records`, { values: start })
      setRows(rs => [...(rs || []), r])
      announceChange()
      if (view.type !== 'grid' || mobile) openRow(r.id)
    } catch (e) { toast(errMsg(e), 'err') }
  }, [view, page, tid, mobile]) // eslint-disable-line react-hooks/exhaustive-deps

  const onPaste = useCallback(async (r0: number, c0: number, grid: string[][], cols: Field[]) => {
    if (!rows) return
    const fresh: Record<string, unknown>[] = []
    let changed = 0
    for (let i = 0; i < grid.length; i++) {
      const vals: Record<string, unknown> = {}
      grid[i].forEach((cell, j) => { const f = cols[c0 + j]; if (f && !['lookup', 'rollup', 'formula', 'created', 'updated', 'autonumber', 'attachment'].includes(f.type)) vals[f.id] = cell })
      if (!Object.keys(vals).length) continue
      const row = rows[r0 + i]
      if (row) { await api.put(`${V1}/records/${row.id}`, { values: vals, typecast: true }).catch(e => toast(errMsg(e), 'err')); changed++ }
      else fresh.push(vals)
    }
    if (fresh.length) await api.post(`${V1}/tables/${tid}/records/batch`, { records: fresh.map(v => ({ values: v })), typecast: true }).catch(e => toast(errMsg(e), 'err'))
    toast(`Pasted into ${changed + fresh.length} ${changed + fresh.length === 1 ? 'row' : 'rows'}${fresh.length ? ` (${fresh.length} new)` : ''}`)
    loadPage(); loadRows(); announceChange()
  }, [rows, tid, toast, loadPage, loadRows])

  if (err) return <div className="page"><div className="empty"><h3>This table is not here</h3><p className="muted">{err}</p><Link to="/" className="btn">Home</Link></div></div>
  if (!page || !view) return <Loading />

  const fields = page.fields
  const baseTables = bases?.find(b => b.id === page.table.base_id)?.tables || [page.table]
  const deleteRows = async () => {
    const ids = [...selected]
    await api.post(`${V1}/tables/${tid}/records/delete`, { ids }).catch(e => toast(errMsg(e), 'err'))
    setSelected(new Set()); toast(`${ids.length} moved to the bin`); loadRows(); loadPage(); announceChange()
  }
  const addView = async (type: ViewType) => {
    try { const v = await api.post<View>(`${V1}/tables/${tid}/views`, { type }); await loadPage(); setView(v.id) } catch (e) { toast(errMsg(e), 'err') }
  }
  const viewMenu = (v: View): MenuItem[] => [
    { label: 'Rename', icon: 'edit', onSelect: () => setRename(v) },
    { label: 'Duplicate', icon: 'copy', onSelect: async () => { const nv = await api.post<View>(`${V1}/tables/${tid}/views`, { copy_of: v.id }); await loadPage(); setView(nv.id) } },
    { label: 'Show as', submenu: (['grid', 'board', 'calendar', 'gallery'] as ViewType[]).map(t => ({ label: VIEW_NAME[t], icon: VIEW_ICON[t], checked: v.type === t, onSelect: async () => { await api.put(`${V1}/views/${v.id}`, { type: t }); loadPage() } })) },
    'sep',
    { label: 'Delete the view', icon: 'trash', danger: true, disabled: page.views.length < 2, onSelect: () => setConfirm({ title: `Delete the view “${v.name}”?`, msg: 'Only this way of looking at the table goes; the rows stay.', label: 'Delete', run: async () => { await api.del(`${V1}/views/${v.id}`); setSp(new URLSearchParams(), { replace: true }); loadPage() } }) },
  ]
  const fMenu = (f: Field): MenuItem[] => [
    { label: 'Change the field', icon: 'edit', onSelect: () => setFieldEdit({ f }) },
    'sep',
    { label: 'Sort A → Z', icon: 'arrow-up', onSelect: () => patchView({ sorts: [{ field: f.id, desc: false }] }) },
    { label: 'Sort Z → A', icon: 'arrow-down', onSelect: () => patchView({ sorts: [{ field: f.id, desc: true }] }) },
    { label: 'Filter by it', icon: 'filter', onSelect: () => { patchView({ filters: [...view.config.filters, { field: f.id, op: 'not_empty', value: '' }] }); if (fieldMenu) setPanel({ which: 'filter', a: fieldMenu.a }) } },
    ...(view.type === 'grid' && !['attachment', 'longtext', 'link', 'lookup', 'pocket'].includes(f.type) ? [{ label: view.config.group_by === f.id ? 'Stop grouping' : 'Group by it', icon: 'group', onSelect: () => patchView({ group_by: view.config.group_by === f.id ? 0 : f.id }) }] : []),
    ...(!f.primary ? [{ label: 'Hide in this view', icon: 'eye-off', onSelect: () => patchView({ hidden: [...view.config.hidden, f.id] }) }] : []),
    'sep',
    { label: 'Add a field after it', icon: 'plus', onSelect: () => setFieldEdit({ after: f.id }) },
    ...(!['lookup', 'rollup', 'formula', 'created', 'updated', 'autonumber', 'link', 'attachment', 'pocket'].includes(f.type) ? [{ label: 'Fill with AI…', icon: 'sparkles', onSelect: () => setFill(f) }] : []),
    ...(!f.primary && !['link', 'attachment', 'checkbox', 'lookup', 'rollup', 'pocket'].includes(f.type) ? [{ label: 'Make it the first field', icon: 'star', onSelect: async () => { await api.put(`${V1}/fields/${f.id}`, { primary: true }).catch(e => toast(errMsg(e), 'err')); reloadAll() } }] : []),
    ...(!f.primary ? ['sep' as const, { label: 'Delete the field', icon: 'trash', danger: true, onSelect: () => setConfirm({ title: `Delete “${f.name}”?`, msg: f.type === 'link' ? 'Its values go, and the field on the other side of the link too.' : 'Its values in every row go too.', label: 'Delete', run: async () => { await api.del(`${V1}/fields/${f.id}`).catch(e => toast(errMsg(e), 'err')); reloadAll() } }) }] : []),
  ]
  const notes = appState('notes', 'notes:write'), files = appState('files', 'files:write'), cal = appState('calendar', 'calendar:write'), forms = appState('forms', 'forms:read'), tasks = appState('tasks', 'tasks:read')
  const offHint = (s: ReturnType<typeof appState>) => (s.ok ? undefined : 'off')
  // made from another app: kept in the settings, or (older tables) its "In Pocket …" column says which
  const fromSlug = page.table.settings.from_app || page.fields.find(f => f.type === 'pocket' && f.options.single && kinds?.some(k => k.slug === f.options.app && f.name === `In ${k.app_name}`))?.options.app
  const fromKind = fromSlug ? kinds?.find(k => k.slug === fromSlug && k.can_import) : undefined
  const more = (): MenuItem[] => [
    { label: 'About this table…', icon: 'info', onSelect: () => setTool('info') },
    { label: 'Bring rows from a spreadsheet…', icon: 'upload', onSelect: () => setTool('import') },
    { label: 'Download as a spreadsheet (CSV)', icon: 'download', onSelect: () => { location.href = `${V1}/tables/${tid}/export.csv?view=${view.id}` } },
    { label: 'Ask AI about this table…', icon: 'sparkles', onSelect: () => setTool('ask') },
    'sep',
    { heading: 'With your other apps' },
    ...(fromKind ? [{ label: `Bring in new ${fromKind.plural} from ${fromKind.app_name}`, icon: fromKind.icon, hint: fromKind.ok ? undefined : 'off', disabled: !fromKind.ok, onSelect: async () => {
      try { const r = await api.post<{ made: number; seen: number }>(`${V1}/from-app`, { kind: fromKind.slug, table_id: tid }); toast(r.made ? `${r.made} new ${r.made === 1 ? 'row' : 'rows'} from ${fromKind.app_name}` : `Nothing new in ${fromKind.app_name}`); reloadAll(); announceChange() } catch (e) { toast(errMsg(e), 'err') }
    } }] : []),
    { label: 'Keep the dates in Pocket Calendar…', icon: 'calendar', hint: page.table.settings.calendar?.on ? 'on' : offHint(cal), disabled: !cal.ok && !page.table.settings.calendar?.on, onSelect: () => setTool('calendar') },
    { label: 'Rows from a Pocket Forms form…', icon: 'list', hint: page.table.settings.form?.on ? 'on' : offHint(forms), disabled: !forms.ok && !page.table.settings.form?.on, onSelect: () => setTool('form') },
    { label: 'Tasks made from rows…', icon: 'check-square', hint: offHint(tasks), disabled: !tasks.ok, onSelect: () => setTool('tasks') },
    { label: 'Save as a note in Pocket Notes', icon: 'note', hint: offHint(notes), disabled: !notes.ok, onSelect: async () => { try { const r = await api.post<{ url: string }>(`${V1}/tables/${tid}/note?view=${view.id}`); toast('Saved in Pocket Notes', 'ok', r.url ? { label: 'Open', run: () => window.open(r.url, '_blank') } : undefined) } catch (e) { toast(errMsg(e), 'err') } } },
    { label: 'Save to Pocket Files', icon: 'file', hint: offHint(files), disabled: !files.ok, onSelect: async () => { try { const r = await api.post<{ url: string; name: string }>(`${V1}/tables/${tid}/to-files?view=${view.id}`); toast(`Saved as ${r.name} in Tables`, 'ok', r.url ? { label: 'Open', run: () => window.open(r.url, '_blank') } : undefined) } catch (e) { toast(errMsg(e), 'err') } } },
    'sep',
    { label: `Deleted rows${page.deleted ? ` (${page.deleted})` : ''}…`, icon: 'bin', onSelect: () => setTool('bin') },
    { label: 'Duplicate the table', icon: 'copy', onSelect: async () => { try { const t = await api.post<{ id: number }>(`${V1}/tables/${tid}/duplicate`, { rows: true }); announceChange(); navigate(`/t/${t.id}`) } catch (e) { toast(errMsg(e), 'err') } } },
    { label: 'Delete the table', icon: 'trash', danger: true, onSelect: () => setConfirm({ title: `Delete “${page.table.name}”?`, msg: `Its ${page.table.records} rows, fields and views go for good, and links to it from other tables. Download it first if you may need it.`, label: 'Delete for good', run: async () => { await api.del(`${V1}/tables/${tid}`); announceChange(); navigate('/') } }) },
  ]
  const n = (k: 'filters' | 'sorts' | 'hidden') => view.config[k].length
  const selectFor = (types: string[], key: 'stack_by' | 'date_field' | 'cover', label: string) => {
    const opts = fields.filter(f => types.includes(f.type))
    return <div className="tb-sel"><span className="muted small hide-sm">{label}</span><Select ariaLabel={label} value={String(view.config[key] || '')} onChange={v => patchView({ [key]: Number(v) })} placeholder="Pick…" options={opts.map(f => ({ value: String(f.id), label: f.name }))} /></div>
  }
  const props = { page, view, rows: rows || [], onSet, onOpen: openRow, onAdd, patchView }
  return (
    <div className="tpage">
      <nav className="ttabs" aria-label="Tables in this base">
        <Link to={`/b/${page.base.id}`} className="ttab-base" title={page.base.name}><span className="base-dot" style={{ background: page.base.color }} />{page.base.icon} <span className="hide-sm">{page.base.name}</span></Link>
        {baseTables.map(t => <Link key={t.id} to={`/t/${t.id}`} className={`ttab ${t.id === tid ? 'on' : ''}`}>{t.icon && <span>{t.icon}</span>}{t.name}</Link>)}
        <button type="button" className="ttab add" onClick={async () => { const t = await api.post<{ id: number }>(`${V1}/bases/${page.base.id}/tables`, {}); announceChange(); navigate(`/t/${t.id}`) }} aria-label="Add a table" title="Add a table"><Icon name="plus" size={14} /></button>
      </nav>
      <div className="vbar">
        <div className="vtabs" role="tablist">
          {page.views.map(v => (
            <div key={v.id} className={`vtab ${v.id === view.id ? 'on' : ''}`}>
              <button type="button" role="tab" aria-selected={v.id === view.id} onClick={() => setView(v.id)}><Icon name={VIEW_ICON[v.type]} size={14} />{v.name}</button>
              {v.id === view.id && <MenuButton label="View options" className="vtab-more" items={() => viewMenu(v)} width={210}><Icon name="chevron-down" size={13} /></MenuButton>}
            </div>
          ))}
          <MenuButton label="Add a view" className="btn ghost sm" items={(['grid', 'board', 'calendar', 'gallery'] as ViewType[]).map(t => ({ label: VIEW_NAME[t], icon: VIEW_ICON[t], onSelect: () => addView(t) }))} width={180}><Icon name="plus" size={14} /><span className="hide-sm">View</span></MenuButton>
        </div>
      </div>
      <div className="tbar">
        <div className="tb-search"><Icon name="search" size={14} /><input placeholder="Find in this view" value={q} onChange={e => setQ(e.target.value)} aria-label="Find in this view" />{q && <button type="button" className="btn ghost icon-only xs" onClick={() => setQ('')} aria-label="Clear"><Icon name="x" size={12} /></button>}</div>
        <button type="button" className={`btn sm ${n('filters') ? 'on' : 'ghost'}`} onClick={e => setPanel({ which: 'filter', a: rectOf(e.currentTarget) })}><Icon name="filter" size={14} /><span className="hide-sm">Filter</span>{n('filters') > 0 && <span className="badge">{n('filters')}</span>}</button>
        <button type="button" className={`btn sm ${n('sorts') ? 'on' : 'ghost'}`} onClick={e => setPanel({ which: 'sort', a: rectOf(e.currentTarget) })}><Icon name="arrow-up-down" size={14} /><span className="hide-sm">Sort</span>{n('sorts') > 0 && <span className="badge">{n('sorts')}</span>}</button>
        {view.type === 'grid' && <button type="button" className={`btn sm ${view.config.group_by ? 'on' : 'ghost'}`} onClick={e => setPanel({ which: 'group', a: rectOf(e.currentTarget) })}><Icon name="group" size={14} /><span className="hide-sm">Group</span></button>}
        <button type="button" className={`btn sm ${n('hidden') ? 'on' : 'ghost'}`} onClick={e => setPanel({ which: 'fields', a: rectOf(e.currentTarget) })}><Icon name="eye-off" size={14} /><span className="hide-sm">{n('hidden') ? `${n('hidden')} hidden` : 'Fields'}</span></button>
        {view.type === 'grid' && !mobile && <MenuButton label="Row height" className="btn ghost sm" width={170} items={[['short', 'Short'], ['medium', 'Medium'], ['tall', 'Tall']].map(([v, l]) => ({ label: l, checked: (view.config.row_height || 'short') === v, onSelect: () => patchView({ row_height: v }) }))}><Icon name="text-long" size={14} /></MenuButton>}
        {view.type === 'board' && selectFor(['select'], 'stack_by', 'Columns by')}
        {view.type === 'calendar' && selectFor(['date', 'created', 'updated'], 'date_field', 'Dates from')}
        {view.type === 'gallery' && fields.some(f => f.type === 'attachment') && selectFor(['attachment'], 'cover', 'Picture from')}
        <span className="grow" />
        {page.table.settings.calendar?.on && <span className="tb-flag" title="The dates are kept in Pocket Calendar"><Icon name="calendar" size={13} /></span>}
        {page.table.settings.form?.on && <span className="tb-flag" title={`Answers to “${page.table.settings.form.title}” come in as rows`}><Icon name="list" size={13} /></span>}
        <span className="muted small tnum hide-sm">{rows ? `${rows.length} ${rows.length === 1 ? 'row' : 'rows'}` : ''}</span>
        <MenuButton label="More about this table" className="btn ghost icon-only sm" align="right" width={300} items={more}><Icon name="more" size={16} /></MenuButton>
      </div>
      {selected.size > 0 && (
        <div className="selbar">
          <b>{selected.size} picked</b>
          <button type="button" className="btn sm danger" onClick={() => setConfirm({ title: `Delete ${selected.size} rows?`, msg: 'They go to the bin, where they stay 30 days.', label: 'Delete', run: deleteRows })}><Icon name="trash" size={14} />Delete</button>
          <button type="button" className="btn sm ghost" onClick={() => setSelected(new Set())}>Unpick</button>
        </div>
      )}
      <div className={`tview v-${view.type}`}>
        {rows === null ? <Loading /> : <>
          {view.type === 'grid' && <GridView {...props} summaries={sums} onFieldMenu={(f, a) => setFieldMenu({ f, a })} onAddField={() => setFieldEdit({})} onPaste={onPaste} selected={selected} setSelected={setSelected} />}
          {view.type === 'board' && <BoardView {...props} />}
          {view.type === 'calendar' && <CalendarView {...props} />}
          {view.type === 'gallery' && <GalleryView {...props} />}
          {rows.length === 0 && view.type === 'grid' && !mobile && (q || view.config.filters.length > 0) && <p className="muted center pad-card">No rows match. {view.config.filters.length > 0 && <button type="button" className="linkish" onClick={() => patchView({ filters: [] })}>Clear the filters</button>}</p>}
        </>}
      </div>
      {mobile && <button type="button" className="fab" aria-label="Add a row" title="Add a row" onClick={() => onAdd()}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}

      {panel && <Popover anchor={panel.a} onClose={() => setPanel(null)} prefer="below" width={panel.which === 'filter' ? 560 : 400} label={panel.which}>
        {panel.which === 'filter' && <FilterPanel fields={fields} view={view} patch={patchView} />}
        {panel.which === 'sort' && <SortPanel fields={fields} view={view} patch={patchView} />}
        {panel.which === 'group' && <GroupPanel fields={fields} view={view} patch={patchView} />}
        {panel.which === 'fields' && <FieldsPanel fields={fields} view={view} patch={patchView} />}
      </Popover>}
      {fieldMenu && <Menu anchor={fieldMenu.a} items={fMenu(fieldMenu.f)} onClose={() => setFieldMenu(null)} width={230} />}
      {fieldEdit && <FieldEditor page={page} field={fieldEdit.f} after={fieldEdit.after} onClose={() => setFieldEdit(null)} onSaved={() => { setFieldEdit(null); reloadAll() }} />}
      {fill && <FillModal field={fill} view={view} onClose={() => setFill(null)} onDone={() => { setFill(null); loadRows() }} />}
      {rowId > 0 && <RecordModal id={rowId} onClose={closeRow} onOpen={openRow} onFieldsChanged={loadPage}
        onChanged={r => { if (r) setRows(rs => rs && (rs.some(x => x.id === r.id) ? rs.map(x => (x.id === r.id ? { ...r, comments: x.comments } : x)) : [...rs, r])); else loadRows() }} />}
      {tool === 'calendar' && <CalendarModal page={page} onClose={() => setTool('')} onSaved={() => { setTool(''); loadPage() }} />}
      {tool === 'form' && <FormModal page={page} onClose={() => setTool('')} onSaved={() => { setTool(''); reloadAll() }} />}
      {tool === 'tasks' && <TasksModal page={page} onClose={() => setTool('')} onSaved={() => { setTool(''); loadPage() }} />}
      {tool === 'ask' && <AskModal page={page} view={view} onClose={() => setTool('')} />}
      {tool === 'bin' && <BinModal page={page} onClose={() => setTool('')} onChanged={() => { loadRows(); loadPage(); announceChange() }} />}
      {tool === 'import' && <ImportRowsModal page={page} onClose={() => setTool('')} onDone={() => { setTool(''); reloadAll(); announceChange() }} />}
      {tool === 'info' && <TableInfoModal page={page} onClose={() => setTool('')} onSaved={() => { setTool(''); loadPage(); announceChange() }} />}
      {rename && <NameModal title="Name of the view" value={rename.name} onClose={() => setRename(null)} onSave={async n => { await api.put(`${V1}/views/${rename.id}`, { name: n }); setRename(null); loadPage() }} />}
      {confirm && <Confirm title={confirm.title} message={confirm.msg} confirmLabel={confirm.label} onConfirm={() => { const c = confirm; setConfirm(null); c.run() }} onCancel={() => setConfirm(null)} />}
    </div>
  )
}
