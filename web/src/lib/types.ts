export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'contacts' | 'tasks' | 'calendar' | 'notes' | 'forms' | 'files' | 'database' | 'documents' | 'inventory' | 'places' | 'crm' | 'bookshelf' | 'wishlist' | 'money'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  currency: string; meeting_minutes: number; ai_help: boolean
  notify_digest: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'
export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

export type FieldType = 'text' | 'longtext' | 'number' | 'currency' | 'percent' | 'rating' | 'checkbox' | 'date' | 'select' | 'multiselect' | 'email' | 'phone' | 'url'
  | 'link' | 'lookup' | 'rollup' | 'formula' | 'attachment' | 'pocket' | 'created' | 'updated' | 'autonumber'

export interface Choice { id: string; name: string; color: string }

export interface FieldOptions {
  decimals?: number; currency?: string; max?: number; time?: boolean; remind?: boolean; choices?: Choice[]
  table_id?: number; inverse?: number; reverse?: boolean; single?: boolean
  link_field?: number; field?: number; fn?: string; expr?: string; app?: string
}

export interface Field { id: number; table_id: number; name: string; type: FieldType; options: FieldOptions; description: string; position: number; primary: boolean; required: boolean }

export interface CalendarSync { on: boolean; field: number; end_field?: number }
export interface FormSync { on: boolean; form_id: string; title: string; url?: string; map: Record<string, number>; status?: string; at?: string; made?: number }
export interface DBSource { conn: number; conn_name: string; db?: string; schema?: string; table?: string; sql?: string; key?: string[]; at?: string }
export interface SheetSettings { calendar?: CalendarSync; form?: FormSync; task_done?: number; task_due?: number; from_app?: string; db?: DBSource }

export interface Table { id: number; base_id: number; name: string; icon: string; description: string; position: number; settings: SheetSettings; created_at: string; updated_at: string; records: number }
export interface Base { id: number; name: string; icon: string; color: string; description: string; position: number; template: string; created_at: string; updated_at: string; tables: Table[] }

export interface Filter { field: number; op: string; value: unknown }
export interface Sort { field: number; desc: boolean }
export type ViewType = 'grid' | 'board' | 'calendar' | 'gallery'
export interface ViewConfig {
  filters: Filter[]; match: 'all' | 'any'; sorts: Sort[]; hidden: number[]; order: number[]; widths: Record<string, number>
  group_by: number; stack_by: number; date_field: number; cover: number; summaries: Record<string, string>; row_height: string; color: number
}
export interface View { id: number; table_id: number; name: string; type: ViewType; config: ViewConfig; position: number }

export interface TablePage { table: Table; base: Base; fields: Field[]; views: View[]; others: Table[]; deleted: number }

export interface LinkRef { id: number; title: string }
export interface PocketRef { app: string; ref: string; title: string; sub?: string; url?: string }
export interface FileRef { id: string; name: string; mime: string; size: number }

export interface Row {
  id: number; table_id: number; number: number; title: string; values: Record<string, unknown>; source: string; comments: number
  created_at: string; updated_at: string; deleted_at?: string; named?: Record<string, string>
}
export interface RowsResult { records: Row[]; total: number; summaries?: Record<string, unknown> }

export interface HistoryLine { id: number; record_id: number; kind: 'comment' | 'change' | 'created' | 'made' | 'restored'; text: string; data: { changes?: { field: string; before: string; after: string }[]; url?: string; ai?: boolean }; app: string; created_at: string }
export interface Made { id: number; record_id: number; target: string; app: string; ref: string; title: string; url: string; state: string; created_at: string; checked_at: string }
export interface RowPage { record: Row; table: Table; fields: Field[]; history: HistoryLine[]; made: Made[]; prev?: number; next?: number }

export interface Kind { slug: string; label: string; plural: string; icon: string; app: AppSlug; app_name: string; ok: boolean; state: AppStateName; detail: string; fix: string; can_import: boolean }

export interface Hit { id: number; title: string; table_id: number; table: string; table_icon?: string; base: string; field?: string; snippet?: string; date?: string; url: string; updated_at?: string }
export interface Summary { bases: number; tables: number; rows: number; due_today: number; soon: Hit[]; late: Hit[]; recent: Hit[] }

export interface Template { key: string; name: string; icon: string; color: string; description: string; tables: string[] }

export interface SpecField { name: string; type: string; choices?: string[]; link?: string; through?: string; of?: string; fn?: string; expr?: string; app?: string }
export interface SpecTable { name: string; icon?: string; description?: string; fields: SpecField[]; rows?: Record<string, unknown>[]; views?: { name: string; type: string; by?: string }[] }
export interface SpecBase { name: string; icon?: string; color?: string; description?: string; tables: SpecTable[] }

export interface FormInfo { id: string; title: string; emoji: string; status: string; responses: number; url: string; fields: { id: string; label: string; type: string; options?: string[]; suggest: string }[] }
