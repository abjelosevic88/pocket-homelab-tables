// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { NewBase, type NewHow } from './NewBase'
import { Avatar, Loading, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'ptb-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

/** Start a new base from anywhere (a page's button), a certain way. */
export const openCreate = (how: NewHow = 'start', baseId?: number) => window.dispatchEvent(new CustomEvent('ptb:create', { detail: { how, baseId } }))

export function createItems(onCreate: (h: NewHow) => void): MenuItem[] {
  return [
    { label: 'A blank base', icon: 'plus', hint: 'N', onSelect: () => onCreate('blank') },
    { label: 'From a ready-made one', icon: 'gallery', onSelect: () => onCreate('template') },
    { label: 'Describe it, and AI sets it up', icon: 'sparkles', onSelect: () => onCreate('ai') },
    'sep',
    { label: 'From a spreadsheet (Excel, CSV, Airtable)…', icon: 'upload', onSelect: () => onCreate('sheet') },
    { label: 'From your other Pocket apps…', icon: 'layers', onSelect: () => onCreate('apps') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { bases } = useData()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { if (!/^\/t\//.test(loc.pathname)) document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [creating, setCreating] = useState<{ how: NewHow; baseId?: number } | null>(null)
  const create = (how: NewHow, baseId?: number) => { closeDrawer(); setCreating({ how, baseId }) }
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ how: NewHow; baseId?: number }>).detail; createRef.current(d.how, d.baseId) }
    window.addEventListener('ptb:shortcuts', open)
    window.addEventListener('ptb:create', make)
    return () => { window.removeEventListener('ptb:shortcuts', open); window.removeEventListener('ptb:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"], .grid-scroll') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': e.preventDefault(); createRef.current('start'); break
        case 'h': e.preventDefault(); navigate('/'); break
        case ',': e.preventDefault(); navigate('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'new', label: 'New base', icon: 'plus', hint: 'N', keywords: 'add create database', run: () => create('start') },
    { id: 'tpl', label: 'New base from a ready-made one', icon: 'gallery', keywords: 'template', run: () => create('template') },
    { id: 'ai', label: 'Describe a base, and AI sets it up', icon: 'sparkles', keywords: 'pocket ai design', run: () => create('ai') },
    { id: 'sheet', label: 'Bring in a spreadsheet (Excel, CSV, Airtable)', icon: 'upload', keywords: 'import csv excel airtable', run: () => create('sheet') },
    { id: 'apps', label: 'Make a table from your other apps', icon: 'layers', keywords: 'contacts places books inventory deals wishes payments', run: () => create('apps') },
    { id: 'home', label: 'Home', icon: 'home', hint: 'H', run: () => navigate('/') },
    ...(bases || []).flatMap(b => b.tables.map(t => ({ id: `t${t.id}`, label: `${t.name} — ${b.name}`, icon: 'table', keywords: 'table open', run: () => navigate(`/t/${t.id}`) }))),
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'apps-set', label: 'Settings: connected apps', icon: 'link', keywords: 'contacts tasks calendar notes forms files core', run: () => navigate('/settings/apps') },
    { id: 'ai-set', label: 'Settings: AI', icon: 'sparkles', keywords: 'pocket ai openai', run: () => navigate('/settings/ai') },
    { id: 'export', label: 'Settings: export everything', icon: 'download', keywords: 'backup json', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]
  const wide = /^\/t\//.test(loc.pathname)
  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className={`content ${wide ? 'wide table-mode' : ''}`}><div className="content-inner">{bases ? <Outlet /> : <Loading />}</div></main>
        </div>
        {mobile && !wide && !/^\/settings/.test(loc.pathname) && <button type="button" className="fab" aria-label="New base" title="New base" onClick={() => create('start')}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {creating && <NewBase how={creating.how} baseId={creating.baseId} onClose={() => setCreating(null)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const section = loc.pathname === '/' ? 'Home' : loc.pathname.startsWith('/b/') ? 'Base' : ''
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('ptb:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title"><span className="ellipsis">{crumb || section}</span></h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
