// Home: what is due (rows whose dates you asked to be reminded of), what
// changed lately, and every base with its tables.
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData, useOnChange } from '../lib/data'
import type { Hit, Summary } from '../lib/types'
import { ago, fmtDate } from '../lib/format'
import { fmtDateValue } from '../lib/fields'
import { openCreate } from '../components/Layout'
import { Icon } from '../components/icons'
import { Loading } from '../components/ui'

function HitRow({ h, showDate = false }: { h: Hit; showDate?: boolean }) {
  return (
    <li>
      <Link className="list-row" to={`/t/${h.table_id}?row=${h.id}`}>
        <span className="list-ico">{h.table_icon || <Icon name="table" size={15} />}</span>
        <span className="row-main grow"><span className="row-title ellipsis">{h.title}</span><span className="muted small ellipsis">{h.table}{h.field ? ` · ${h.field}` : ''}{h.base ? ` · ${h.base}` : ''}</span></span>
        {showDate && h.date && <span className="muted small tnum">{fmtDateValue(h.date)}</span>}
        {!showDate && h.updated_at && <span className="muted small">{ago(h.updated_at)}</span>}
      </Link>
    </li>
  )
}

export default function Home() {
  const { bases } = useData()
  const [sum, setSum] = useState<Summary | null>(null)
  const load = useCallback(() => { api.get<Summary>(`${V1}/summary`).then(setSum).catch(() => {}) }, [])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  if (!bases) return <Loading />
  const today = new Date().toISOString().slice(0, 10)
  const dueToday = sum?.soon.filter(h => h.date?.slice(0, 10) === today) || []
  const later = sum?.soon.filter(h => h.date?.slice(0, 10) !== today) || []
  if (bases.length === 0) {
    return (
      <div className="page home-empty">
        <img src="/favicon.svg" alt="" width={64} height={64} />
        <h1>Your own small databases</h1>
        <p className="muted">Keep track of anything — bookings, suppliers, a collection, research, a project — in tables you shape yourself: choices, dates, money, files, and links between tables. See them as a grid, a board, a calendar or a gallery.</p>
        <div className="row center wrap">
          <button type="button" className="btn primary lg" onClick={() => openCreate('template')}><Icon name="gallery" size={16} />Start from a ready-made base</button>
          <button type="button" className="btn lg" onClick={() => openCreate('ai')}><Icon name="sparkles" size={16} />Describe it</button>
          <button type="button" className="btn lg" onClick={() => openCreate('sheet')}><Icon name="upload" size={16} />Bring a spreadsheet</button>
          <button type="button" className="btn lg" onClick={() => openCreate('blank')}><Icon name="plus" size={16} />Blank</button>
        </div>
      </div>
    )
  }
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Home</h1><p className="muted">{fmtDate(today)} · {sum ? `${sum.rows.toLocaleString()} rows in ${sum.tables} tables` : ''}</p></div>
      </div>
      {(dueToday.length > 0 || (sum?.late.length || 0) > 0 || later.length > 0) && (
        <div className="two-col">
          <section className="card">
            <h2 className="card-h pad-card-h"><Icon name="today" size={16} />Today{sum && sum.late.length > 0 && <span className="badge bad-badge">{sum.late.length} late</span>}</h2>
            <ul className="list">
              {sum?.late.slice(0, 6).map(h => <HitRow key={'l' + h.id + h.field} h={h} showDate />)}
              {dueToday.map(h => <HitRow key={'t' + h.id + h.field} h={h} showDate />)}
              {!dueToday.length && !sum?.late.length && <li className="muted small pad-s">Nothing due today.</li>}
            </ul>
          </section>
          <section className="card">
            <h2 className="card-h pad-card-h"><Icon name="calendar" size={16} />The next 7 days</h2>
            <ul className="list">
              {later.slice(0, 8).map(h => <HitRow key={'s' + h.id + h.field} h={h} showDate />)}
              {!later.length && <li className="muted small pad-s">Nothing coming up.</li>}
            </ul>
          </section>
        </div>
      )}
      <section>
        <div className="row"><h2 className="sec-h grow">Your bases</h2><button type="button" className="btn sm" onClick={() => openCreate('start')}><Icon name="plus" size={14} />New base</button></div>
        <div className="bases-grid">
          {bases.map(b => (
            <div key={b.id} className="base-card" style={{ ['--tile' as string]: b.color }}>
              <Link to={`/b/${b.id}`} className="base-card-head"><span className="base-ico lg" style={{ background: b.color + '26' }}>{b.icon || '📋'}</span><span className="grow"><b>{b.name}</b>{b.description && <span className="muted small ellipsis block-line">{b.description}</span>}</span></Link>
              <div className="base-card-tables">
                {b.tables.map(t => <Link key={t.id} to={`/t/${t.id}`} className="base-tbl"><span>{t.icon || <Icon name="table" size={13} />}</span><span className="grow ellipsis">{t.name}</span><span className="muted small tnum">{t.records}</span></Link>)}
              </div>
            </div>
          ))}
        </div>
      </section>
      {sum && sum.recent.length > 0 && (
        <section className="card">
          <h2 className="card-h pad-card-h"><Icon name="clock" size={16} />Changed lately</h2>
          <ul className="list">{sum.recent.slice(0, 8).map(h => <HitRow key={'r' + h.id} h={h} />)}</ul>
        </section>
      )}
      <p className="muted small center">Dates show up here when their field has <b>Remind me on the day</b> on.</p>
    </div>
  )
}
