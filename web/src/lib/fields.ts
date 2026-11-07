// Field types: their names, icons and help; values as text; which filters fit.
import type { Choice, Field, FieldType, FileRef, LinkRef, PocketRef } from './types'
import { fmtDate, fmtDateTime } from './format'

export const TYPES: { type: FieldType; label: string; icon: string; help: string; group: string }[] = [
  { type: 'text', label: 'Text', icon: 'text-short', help: 'A name, a word, one line', group: 'Basic' },
  { type: 'longtext', label: 'Long text', icon: 'text-long', help: 'Notes, a description, several lines', group: 'Basic' },
  { type: 'number', label: 'Number', icon: 'hash', help: 'A count, a size, pages', group: 'Basic' },
  { type: 'currency', label: 'Money', icon: 'wallet', help: 'A price or an amount, in a currency', group: 'Basic' },
  { type: 'percent', label: 'Percent', icon: 'percent', help: 'How far along, a discount', group: 'Basic' },
  { type: 'rating', label: 'Rating', icon: 'star', help: 'One to five stars', group: 'Basic' },
  { type: 'checkbox', label: 'Yes / no', icon: 'check-square', help: 'Done, paid, packed', group: 'Basic' },
  { type: 'date', label: 'Date', icon: 'calendar', help: 'A day, with a time if you want', group: 'Basic' },
  { type: 'select', label: 'Choice', icon: 'circle', help: 'One option from a list: a status, a kind', group: 'Basic' },
  { type: 'multiselect', label: 'Several choices', icon: 'tag', help: 'Tags: any options from a list', group: 'Basic' },
  { type: 'email', label: 'Email', icon: 'mail', help: 'An email address you can write to', group: 'Basic' },
  { type: 'phone', label: 'Phone', icon: 'phone', help: 'A number you can call', group: 'Basic' },
  { type: 'url', label: 'Web link', icon: 'globe', help: 'A web address', group: 'Basic' },
  { type: 'attachment', label: 'Files', icon: 'paperclip', help: 'Photos, PDFs, any files', group: 'Basic' },
  { type: 'link', label: 'Link to another table', icon: 'link-row', help: 'Rows of another table: the order’s products, the booking’s camera', group: 'Connect' },
  { type: 'lookup', label: 'Look up', icon: 'search', help: 'A field of the linked rows: the product’s price', group: 'Connect' },
  { type: 'rollup', label: 'Total of linked', icon: 'sigma', help: 'Sum, count or average of the linked rows', group: 'Connect' },
  { type: 'pocket', label: 'In your Pocket apps', icon: 'layers', help: 'A contact, task, event, note, place, book… from your other apps', group: 'Connect' },
  { type: 'formula', label: 'Calculation', icon: 'fx', help: 'Worked out from other fields: {Price} * {Quantity}', group: 'Worked out' },
  { type: 'autonumber', label: 'Number of the row', icon: 'hash', help: '1, 2, 3… by itself', group: 'Worked out' },
  { type: 'created', label: 'Added on', icon: 'clock', help: 'When the row was added', group: 'Worked out' },
  { type: 'updated', label: 'Last changed', icon: 'clock', help: 'When the row was last changed', group: 'Worked out' },
]

export const typeInfo = (t: FieldType) => TYPES.find(x => x.type === t) || TYPES[0]

/** Worked out by the app; nobody types into them. */
export const computed = (t: FieldType) => ['lookup', 'rollup', 'formula', 'created', 'updated', 'autonumber'].includes(t)

export const numeric = (t: FieldType) => ['number', 'currency', 'percent', 'rating', 'autonumber'].includes(t)

export const COLORS = ['#4dabf7', '#69db7c', '#ffd43b', '#ff8787', '#b197fc', '#ffa94d', '#63e6be', '#f783ac', '#a9e34b', '#74c0fc', '#ced4da', '#e599f7']

export const ROLLUPS: { value: string; label: string }[] = [
  { value: 'sum', label: 'Sum' }, { value: 'avg', label: 'Average' }, { value: 'min', label: 'Smallest' }, { value: 'max', label: 'Largest' },
  { value: 'count', label: 'How many linked' }, { value: 'counta', label: 'How many filled' }, { value: 'checked', label: 'How many ticked' },
  { value: 'list', label: 'List them' }, { value: 'earliest', label: 'Earliest date' }, { value: 'latest', label: 'Latest date' },
]

export const SUMMARIES: { value: string; label: string; for: 'number' | 'any' | 'check' | 'date' }[] = [
  { value: 'sum', label: 'Sum', for: 'number' }, { value: 'avg', label: 'Average', for: 'number' }, { value: 'min', label: 'Smallest', for: 'number' },
  { value: 'max', label: 'Largest', for: 'number' }, { value: 'filled', label: 'Filled', for: 'any' }, { value: 'empty', label: 'Empty', for: 'any' },
  { value: 'unique', label: 'Different', for: 'any' }, { value: 'checked', label: 'Ticked', for: 'check' }, { value: 'percent_checked', label: '% ticked', for: 'check' },
  { value: 'earliest', label: 'Earliest', for: 'date' }, { value: 'latest', label: 'Latest', for: 'date' },
]

export function choiceOf(f: Field, id: unknown): Choice | undefined {
  return f.options.choices?.find(c => c.id === id)
}

/** The colour text on a choice reads well on (choices are light, so dark text). */
export function chipStyle(color: string) { return { background: color + '55', borderColor: color } }

const toNum = (v: unknown) => (typeof v === 'number' ? v : typeof v === 'string' && v !== '' && !isNaN(Number(v)) ? Number(v) : null)

function fmtNumber(n: number, decimals?: number) {
  if (decimals !== undefined && decimals > 0) return n.toLocaleString(undefined, { minimumFractionDigits: decimals, maximumFractionDigits: decimals })
  return n.toLocaleString(undefined, { maximumFractionDigits: 6 })
}

export function fmtDateValue(v: string) {
  if (!v) return ''
  if (v.length > 10) return fmtDateTime(v.length === 16 ? v + ':00' : v)
  return fmtDate(v)
}

/** A cell's value as text (lists joined), the way the grid shows it in words. */
export function text(f: Field, v: unknown): string {
  if (v === undefined || v === null || v === '') return ''
  switch (f.type) {
    case 'number': { const n = toNum(v); return n === null ? String(v) : fmtNumber(n, f.options.decimals) }
    case 'currency': { const n = toNum(v); return n === null ? String(v) : `${fmtNumber(n, f.options.decimals ?? 2)}${f.options.currency ? ' ' + f.options.currency : ''}` }
    case 'percent': { const n = toNum(v); return n === null ? String(v) : `${fmtNumber(n, f.options.decimals)}%` }
    case 'rating': return '★'.repeat(Number(v) || 0)
    case 'checkbox': return v ? 'Yes' : ''
    case 'date': return fmtDateValue(String(v))
    case 'created': case 'updated': return fmtDateTime(String(v))
    case 'select': return choiceOf(f, v)?.name || ''
    case 'multiselect': return (v as string[]).map(id => choiceOf(f, id)?.name).filter(Boolean).join(', ')
    case 'link': return (v as LinkRef[]).map(l => l.title).join(', ')
    case 'pocket': return (v as PocketRef[]).map(l => l.title).join(', ')
    case 'attachment': return (v as FileRef[]).map(l => l.name).join(', ')
    case 'autonumber': return String(v)
    case 'lookup': return (Array.isArray(v) ? v : [v]).map(x => (typeof x === 'number' ? fmtNumber(x) : typeof x === 'boolean' ? (x ? 'Yes' : '') : /^\d{4}-\d{2}-\d{2}/.test(String(x)) ? fmtDateValue(String(x)) : String(x))).filter(Boolean).join(', ')
    case 'rollup': case 'formula': {
      if (typeof v === 'number') return fmtNumber(Math.round(v * 1e6) / 1e6, f.options.decimals)
      if (typeof v === 'boolean') return v ? 'Yes' : 'No'
      if (typeof v === 'string' && /^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2})?$/.test(v)) return fmtDateValue(v)
      return Array.isArray(v) ? v.join(', ') : String(v)
    }
  }
  return String(v)
}

/** Which filters fit a field, in plain words. */
export function opsFor(f: Field): { value: string; label: string }[] {
  const empty = [{ value: 'empty', label: 'is empty' }, { value: 'not_empty', label: 'is not empty' }]
  switch (f.type) {
    case 'number': case 'currency': case 'percent': case 'rating': case 'autonumber': case 'rollup':
      return [{ value: 'is', label: '=' }, { value: 'is_not', label: '≠' }, { value: 'gt', label: '>' }, { value: 'gte', label: '≥' }, { value: 'lt', label: '<' }, { value: 'lte', label: '≤' }, ...empty]
    case 'date': case 'created': case 'updated':
      return [{ value: 'is', label: 'is on' }, { value: 'before', label: 'is before' }, { value: 'after', label: 'is after' }, { value: 'on_or_before', label: 'is on or before' },
        { value: 'on_or_after', label: 'is on or after' }, { value: 'within', label: 'is within' }, ...empty]
    case 'checkbox': return [{ value: 'checked', label: 'is ticked' }, { value: 'unchecked', label: 'is not ticked' }]
    case 'select': return [{ value: 'any_of', label: 'is any of' }, { value: 'none_of', label: 'is none of' }, ...empty]
    case 'multiselect': return [{ value: 'any_of', label: 'has any of' }, { value: 'has_all', label: 'has all of' }, { value: 'none_of', label: 'has none of' }, ...empty]
    case 'link': case 'lookup': case 'pocket': case 'attachment':
      return [{ value: 'contains', label: 'has' }, { value: 'not_contains', label: 'does not have' }, ...empty]
  }
  return [{ value: 'contains', label: 'has' }, { value: 'not_contains', label: 'does not have' }, { value: 'is', label: 'is' }, { value: 'is_not', label: 'is not' }, ...empty]
}

export const WITHIN = [
  { value: 'past_week', label: 'the past week' }, { value: 'next_week', label: 'the next week' }, { value: 'this_week', label: 'this week' },
  { value: 'past_month', label: 'the past month' }, { value: 'next_month', label: 'the next month' }, { value: 'this_month', label: 'this month' },
  { value: 'this_year', label: 'this year' }, { value: 'overdue', label: 'before today' },
]

/** The value a new filter starts with. */
export function opNeedsValue(op: string) { return !['empty', 'not_empty', 'checked', 'unchecked'].includes(op) }

/** A name for a row number when the first field is empty. */
export const rowTitle = (r: { title: string; number: number }) => r.title || `Row ${r.number}`

export const CURRENCIES = ['EUR', 'BAM', 'USD', 'GBP', 'CHF', 'RSD', 'HRK', 'SEK', 'NOK', 'DKK', 'PLN', 'CZK', 'HUF', 'RON', 'BGN', 'TRY', 'CAD', 'AUD']

export const EMOJIS = ['📋', '🗂️', '📦', '🏠', '🎛️', '📅', '💼', '📚', '🔬', '💡', '🎉', '🍽️', '🚗', '🌱', '🐾', '💿', '🧾', '💰', '🛠️', '✈️', '🎓', '🏃', '🎨', '📣', '⚖️', '🏭', '🙋', '✅', '⭐', '🔖']
