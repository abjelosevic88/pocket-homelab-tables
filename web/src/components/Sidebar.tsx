// Left column: a new base (and the other ways to start), Home, then every
// base with its tables. The Pocket Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { Icon } from './icons'
import { createItems } from './Layout'
import type { NewHow } from './NewBase'
import { type Anchor, Menu, rectOf } from './ui'

const OPEN_KEY = 'ptb-closed-bases'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (h: NewHow, baseId?: number) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { bases } = useData()
  const navigate = useNavigate()
  const loc = useLocation()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const [closed, setClosed] = useState<number[]>(() => { try { return JSON.parse(localStorage.getItem(OPEN_KEY) || '[]') } catch { return [] } })
  const toggle = (id: number) => setClosed(c => { const n = c.includes(id) ? c.filter(x => x !== id) : [...c, id]; localStorage.setItem(OPEN_KEY, JSON.stringify(n)); return n })
  const title = settings?.title || status?.title || 'Pocket Tables'
  const curTable = Number((loc.pathname.match(/^\/t\/(\d+)/) || [])[1] || 0)
  const curBase = Number((loc.pathname.match(/^\/b\/(\d+)/) || [])[1] || 0)
  const addTable = async (baseId: number) => {
    const t = await api.post<{ id: number }>(`${V1}/bases/${baseId}/tables`, {})
    announceChange(); onNavigate(); navigate(`/t/${t.id}`)
  }
  return (
    <aside className="sidebar" aria-label="Pocket Tables">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Tables' ? title : <>Pocket <b>Tables</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate('start')} title="New base (N)">
          <Icon name="plus" size={15} strokeWidth={2.2} />New base
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createItems(h => onCreate(h))} onClose={() => setMenu(null)} width={300} />}

      <nav className="sb-nav" aria-label="Sections">
        <NavLink to="/" end onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}><Icon name="home" size={16} />Home</NavLink>
      </nav>

      <div className="sb-bases">
        {(bases || []).map(b => {
          const open = !closed.includes(b.id) || b.tables.some(t => t.id === curTable)
          return (
            <section key={b.id} className="sb-sec">
              <div className={`sb-base ${curBase === b.id ? 'active' : ''}`}>
                <button type="button" className="sb-caret" onClick={() => toggle(b.id)} aria-label={open ? `Fold ${b.name}` : `Unfold ${b.name}`} aria-expanded={open}><Icon name={open ? 'chevron-down' : 'chevron-right'} size={13} /></button>
                <Link to={`/b/${b.id}`} onClick={onNavigate} className="sb-base-name"><span className="base-ico" style={{ background: b.color + '26' }}>{b.icon || '📋'}</span><span className="ellipsis">{b.name}</span></Link>
                <button type="button" className="sb-add" onClick={() => addTable(b.id)} aria-label={`Add a table to ${b.name}`} title="Add a table"><Icon name="plus" size={13} /></button>
              </div>
              {open && b.tables.map(t => (
                <Link key={t.id} to={`/t/${t.id}`} onClick={onNavigate} className={`sb-row sb-list sb-table ${curTable === t.id ? 'active' : ''}`}>
                  <span className="sb-list-ico">{t.icon || <Icon name="table" size={14} />}</span>
                  <span className="grow ellipsis">{t.name}</span>
                  <span className="muted small tnum">{t.records || ''}</span>
                </Link>
              ))}
            </section>
          )
        })}
        {bases && bases.length === 0 && <section className="sb-sec">
          <button type="button" className="sb-row sb-list" onClick={() => onCreate('template')}><span className="sb-list-ico"><Icon name="gallery" size={14} /></span><span className="grow">Start from a ready-made base</span></button>
        </section>}
      </div>

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('ptb:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
