// A base: its tables, what it is for, its name, icon and colour.
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useData } from '../lib/data'
import { EMOJIS } from '../lib/fields'
import type { Base } from '../lib/types'
import { ago } from '../lib/format'
import { openCreate, useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { Confirm, Field, Loading, Modal, errMsg, useToast } from '../components/ui'

const COLORS = ['#087f5b', '#1c7ed6', '#7048e8', '#c2255c', '#e8590c', '#f08c00', '#0c8599', '#5c940d', '#495057']

export default function BasePage() {
  const { id } = useParams()
  const { bases } = useData()
  const navigate = useNavigate()
  const { toast } = useToast()
  const b = bases?.find(x => x.id === Number(id))
  const [edit, setEdit] = useState(false)
  const [del, setDel] = useState(false)
  useCrumb(b ? `${b.icon} ${b.name}` : '')
  if (!bases) return <Loading />
  if (!b) return <div className="page"><div className="empty"><h3>This base is not here</h3><Link to="/" className="btn">Home</Link></div></div>
  const rows = b.tables.reduce((a, t) => a + t.records, 0)
  return (
    <div className="page stack loose">
      <div className="page-head">
        <span className="base-ico xl" style={{ background: b.color + '26' }}>{b.icon || '📋'}</span>
        <div className="grow"><h1 className="page-title">{b.name}</h1><p className="muted">{b.description || `${b.tables.length} tables · ${rows} rows`}</p></div>
        <button type="button" className="btn" onClick={() => setEdit(true)}><Icon name="edit" size={14} />Change</button>
      </div>
      <div className="bases-grid">
        {b.tables.map(t => (
          <Link key={t.id} to={`/t/${t.id}`} className="tbl-card">
            <span className="tbl-ico">{t.icon || <Icon name="table" size={18} />}</span>
            <b>{t.name}</b>
            <span className="muted small">{t.records} rows · changed {ago(t.updated_at)}</span>
            {t.description && <span className="muted small clamp2">{t.description}</span>}
            <span className="tbl-flags">{t.settings.calendar?.on && <span title="Dates in Pocket Calendar"><Icon name="calendar" size={13} /></span>}{t.settings.form?.on && <span title="Rows from a form"><Icon name="list" size={13} /></span>}</span>
          </Link>
        ))}
        <button type="button" className="tbl-card add" onClick={async () => { try { const t = await api.post<{ id: number }>(`${V1}/bases/${b.id}/tables`, {}); announceChange(); navigate(`/t/${t.id}`) } catch (e) { toast(errMsg(e), 'err') } }}><Icon name="plus" size={20} />A blank table</button>
        <button type="button" className="tbl-card add" onClick={() => openCreate('start', b.id)}><Icon name="sparkles" size={20} />From a spreadsheet, AI or your apps</button>
      </div>
      <div className="row"><button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={14} />Delete the base</button></div>
      {edit && <EditBase b={b} onClose={() => setEdit(false)} />}
      {del && <Confirm title={`Delete “${b.name}”?`} message={`Its ${b.tables.length} tables and ${rows} rows go for good. Export them first (Settings → Backup & data) if you may need them.`} confirmLabel="Delete for good"
        onCancel={() => setDel(false)} onConfirm={async () => { setDel(false); try { await api.del(`${V1}/bases/${b.id}`); announceChange(); navigate('/') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

function EditBase({ b, onClose }: { b: Base; onClose: () => void }) {
  const [name, setName] = useState(b.name)
  const [icon, setIcon] = useState(b.icon)
  const [color, setColor] = useState(b.color)
  const [desc, setDesc] = useState(b.description)
  const { toast } = useToast()
  useEffect(() => { setName(b.name) }, [b.name])
  const save = async () => {
    try { await api.put(`${V1}/bases/${b.id}`, { name, icon, color, description: desc }); announceChange(); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title="The base" onClose={onClose} size="sm" footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save}>Save</button></>}>
      <div className="stack">
        <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} autoFocus /></Field>
        <Field label="Icon"><div className="emoji-pick">{EMOJIS.map(e => <button key={e} type="button" className={icon === e ? 'on' : ''} onClick={() => setIcon(e)}>{e}</button>)}</div></Field>
        <Field label="Colour"><div className="swatches">{COLORS.map(c => <button key={c} type="button" className={`swatch ${color === c ? 'on' : ''}`} style={{ background: c }} onClick={() => setColor(c)} aria-label={c} />)}</div></Field>
        <Field label="What it is for"><textarea rows={2} value={desc} onChange={e => setDesc(e.target.value)} /></Field>
      </div>
    </Modal>
  )
}
