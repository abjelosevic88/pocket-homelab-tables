// The person's bases and tables, and what the other apps can do, shared by every page.
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { AppState, Base, Kind } from './types'

const EVENT = 'ptb:changed'

interface DataState {
  bases: Base[] | null
  apps: AppState[] | null
  kinds: Kind[] | null
  reload: () => Promise<void>
  reloadApps: () => Promise<void>
}
const Ctx = createContext<DataState>({ bases: null, apps: null, kinds: null, reload: async () => {}, reloadApps: async () => {} })
export function useData() { return useContext(Ctx) }

/** Something changed (a table added, a row deleted): lists and counts reload. */
export function announceChange() { window.dispatchEvent(new CustomEvent(EVENT)) }

export function useOnChange(fn: () => void) {
  useEffect(() => {
    window.addEventListener(EVENT, fn)
    return () => window.removeEventListener(EVENT, fn)
  }, [fn])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [bases, setBases] = useState<Base[] | null>(null)
  const [apps, setApps] = useState<AppState[] | null>(null)
  const [kinds, setKinds] = useState<Kind[] | null>(null)
  const reload = useCallback(async () => {
    const b = await api.get<Base[]>(`${V1}/bases`).catch(() => null)
    setBases(x => b || x || [])
  }, [])
  const reloadApps = useCallback(async () => {
    try { setApps((await api.get<{ apps: AppState[] }>(`${V1}/apps`)).apps) } catch { setApps(a => a || []) }
    try { setKinds(await api.get<Kind[]>(`${V1}/apps/kinds`)) } catch { setKinds(k => k || []) }
  }, [])
  useEffect(() => { reload(); reloadApps() }, [reload, reloadApps])
  useOnChange(reload)
  // links switched on elsewhere: look again now and then, quietly
  useEffect(() => {
    const t = window.setInterval(() => { if (document.visibilityState === 'visible') { reload(); reloadApps() } }, 120000)
    return () => window.clearInterval(t)
  }, [reload, reloadApps])
  return <Ctx.Provider value={{ bases, apps, kinds, reload, reloadApps }}>{children}</Ctx.Provider>
}

/** One app's state for this person; `ok` = the scope works. */
export function useAppState() {
  const { apps } = useData()
  return (slug: string, scope?: string) => {
    const a = apps?.find(x => x.slug === slug)
    const ok = !!a && a.via !== '' && (!scope || a.on.includes(scope))
    return { app: a, ok, loading: !apps }
  }
}
