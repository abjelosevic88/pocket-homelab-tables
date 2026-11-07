// /r/<id>: a link to one row (from another app, a notification): opens its table with the row.
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { RowPage } from '../lib/types'
import { Loading, errMsg } from '../components/ui'

export default function RecordRoute() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [err, setErr] = useState('')
  useEffect(() => {
    api.get<RowPage>(`${V1}/records/${id}`).then(p => navigate(`/t/${p.table.id}?row=${p.record.id}`, { replace: true })).catch(e => setErr(errMsg(e)))
  }, [id, navigate])
  if (err) return <div className="page"><div className="empty"><h3>This row is not here</h3><p className="muted">{err}</p><Link to="/" className="btn">Home</Link></div></div>
  return <Loading />
}
