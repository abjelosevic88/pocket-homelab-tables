# Pocket Tables API

Everything is under `/api/v1`, JSON in and out. Authenticate with a session cookie (`ptb_session`), a personal API token
(`Authorization: Bearer ptb_…`, Settings → API & shortcuts), or — for other Pocket apps — a Pocket Core link token
(`pcx_…`, see "Other apps" below). Everything belongs to the person who is signed in; other people's ids answer 404.

A **base** holds **tables**; a table has **fields**, **views** and **rows** (records). A row's `values` are keyed by
field id. When writing, `values` (or `fields`, as Airtable calls it) may be keyed by field id **or field name**.

## Values by field type

| Type | Stored value | Notes |
|---|---|---|
| `text`, `longtext`, `email`, `phone`, `url` | string | emails are lower-cased; a URL without `https://` gets it |
| `number`, `currency`, `percent`, `rating` | number | text like `"1.234,50"` or `"€ 12"` is read too; options `decimals`, `currency`, `max` |
| `checkbox` | bool | `"yes"`, `"da"`, `1`… are read as true |
| `date` | `"YYYY-MM-DD"` or `"YYYY-MM-DDTHH:MM"` (option `time`) | `"7.11.2026"`, `"11/7/2026"` are read too; option `remind` |
| `select`, `multiselect` | choice id / list of ids | send names with `typecast: true` and unknown ones become new options |
| `link` | list of row ids in another table (`options.table_id`) | read as `[{id, title}]`; the other table has the field going back |
| `attachment` | `[{id, name, mime, size}]` | upload with `POST /files`, then put the answer in the list |
| `pocket` | `[{app, ref, title, sub, url}]` | a thing in another Pocket app (`options.app`: contact, task, event, note, document, item, place, deal, book, wish, payment) |
| `lookup`, `rollup`, `formula`, `created`, `updated`, `autonumber` | worked out | read only |

## Bases and tables

| | |
|---|---|
| `GET /bases` | every base with its tables (`records` = rows) |
| `POST /bases` | `{name, icon?, color?, description?, empty?}` → the base with a first table (unless `empty`) |
| `PUT/DELETE /bases/{id}` | |
| `POST /bases/{id}/tables` | `{name?, icon?}` → a table with Name, Notes and Status and a grid view |
| `POST /bases/build` | `{spec, base_id?, rows?}` — make tables from a description (what `POST /ai/design` suggests) |
| `GET /templates`, `POST /templates/{key}` | ready-made bases; `{examples: true}` keeps the example rows |
| `GET /tables/{id}` | `{table, base, fields, views, others, deleted}` |
| `PUT /tables/{id}` | `{name?, icon?, description?, base_id?, calendar?: {on, field, end_field}, task_done?, task_due?}` |
| `DELETE /tables/{id}`, `POST /tables/{id}/duplicate` | `{rows: true}` copies the rows too |
| `GET /tables/{id}/export.csv` | `?view=` — the table as a view shows it |
| `POST /tables/{id}/fields`, `PUT/DELETE /fields/{id}` | `{name, type, options, description, after?, primary?}`; changing the type converts the values |
| `POST /tables/{id}/views`, `PUT/DELETE /views/{id}` | `{name, type: grid\|board\|calendar\|gallery, config, copy_of?}` |
| `POST /{bases\|tables\|fields\|views}/order` | `{ids}` |
| `GET /formula/functions` | what a formula can use |

A view's `config`: `filters: [{field, op, value}]`, `match: all|any`, `sorts: [{field, desc}]`, `hidden`, `order`,
`widths`, `group_by`, `stack_by` (board), `date_field` (calendar), `cover` (gallery), `summaries: {fieldId: sum|avg|min|max|filled|empty|unique|checked|percent_checked|earliest|latest}`, `row_height`.
Filter ops: `contains not_contains is is_not empty not_empty gt gte lt lte before after on_or_before on_or_after within checked unchecked any_of none_of has_all`;
dates may be `today`, `tomorrow`, `yesterday`, `+7`, `-30`; `within` takes `past_week next_week this_week past_month next_month this_month this_year overdue`.

## Rows

| | |
|---|---|
| `GET /tables/{id}/records` | `?view=&q=&limit=&offset=&named=1&deleted=1` → `{records: [{id, number, title, values, named?, source, comments, created_at, updated_at}], total, summaries?}` |
| `POST /tables/{id}/records` | `{values, typecast?, after?, before?, ref?}` → 201 row; a `ref` seen before answers 200 with the row it made |
| `POST /tables/{id}/records/batch` | `{records: [{values}], typecast?}` → `{made, ids}` (at most 5000) |
| `POST /tables/{id}/records/delete` | `{ids}` — to the bin (30 days) |
| `GET /records/{id}` | `{record, table, fields, history, made, prev, next}` |
| `PUT /records/{id}` | `{values, typecast?, after?, before?}` → the row |
| `DELETE /records/{id}`, `POST /records/{id}/restore`, `DELETE /records/{id}/purge`, `POST /records/{id}/duplicate` | |
| `POST /records/{id}/comments`, `DELETE /comments/{id}` | `{text}` |
| `POST /files` (multipart `file`), `GET /files/{id}` | attached files (`?dl=1` downloads) |
| `POST /import` | a Pocket Tables export (JSON) or a spreadsheet (CSV/TSV, multipart `file` or `{text, name}`) → a new base |
| `POST /bases/{id}/import` | a spreadsheet → a new table in the base (types guessed) |
| `POST /tables/{id}/import` | `?add_fields=1` — a spreadsheet's rows into the table, columns by field name |
| `GET /export.json` | everything (bases, tables, fields, views, rows, links) |

## Across everything

| | |
|---|---|
| `GET /summary` | `{bases, tables, rows, due_today, soon, late, recent}` — reminders come from date fields with `remind` |
| `GET /today` | `{today, late, soon}` |
| `GET /search` | `?q=&limit=` → `{items: [{id, title, table_id, table, base, field, snippet, url}], tables}` |
| `GET /lookup` | `?email=` / `?contact=<Contacts id>` / `?name=` → the rows about a person |
| `GET /day/{YYYY-MM-DD}` | the rows with a date on that day |

## The other Pocket apps

| | |
|---|---|
| `GET /apps`, `GET /apps/kinds` | per app (and per kind of thing) whether it works for you, and if not why and what to do |
| `GET /apps/pick` | `?kind=contact&q=` — search another app for a `pocket` field |
| `POST /from-app` | `{kind, base_id?, table_id?}` — a table from your contacts, tasks, inventory, places, deals, books, wishes or payments (with `table_id`: bring the new ones in) |
| `POST /records/{id}/make` | `{target: task\|note\|event, title?, due?, date?, time?, minutes?, notes?}` |
| `POST /tables/{id}/note`, `POST /tables/{id}/to-files` | `?view=` — the table as a note in Pocket Notes / a CSV in Pocket Files (folder `Tables`) |
| `GET /apps/files`, `POST /bases/{id}/from-files` | browse Pocket Files; `{space, path}` makes a table from a CSV there |
| `GET /apps/forms`, `POST /tables/{id}/form`, `POST /tables/{id}/form/pull` | tie a Pocket Forms form: `{on, form_id, title, map: {question id: field id}, make_fields?, questions?}` — answers become rows |
| `POST /tables/{id}/make-form` | a draft form in Pocket Forms from the table's fields, tied to the table |
| `GET /apps/db/connections`, `GET /apps/db/objects?conn=&db=` | your Pocket Database connections and their tables |
| `POST /from-db` | `{conn, conn_name, db?, schema?, table? \| sql?, name?, base_id?}` — a table from a database table or a read-only query; `{table_id}` brings in the latest rows (matched by primary key, else by content) |
| `POST /tables/{id}/calendar/sync` | bring the table's events in Pocket Calendar up to date now |

## AI

`POST /ai/design` `{prompt}` → a base description to look at; `POST /fields/{id}/ai-fill` `{instruction, only_empty?, view?, ids?}` → `{filled, looked}`;
`POST /tables/{id}/ai-ask` `{question, view?}` → `{text}`. `GET /ai/status`, `GET /ai/models`.

## Other apps (Pocket Core links)

Offers `tables:read` (bases, tables, rows, search, lookup, day, summary, CSV) and `tables:write` (add rows, change the rows that
app added, comment). Events: `tables.record_created`, `tables.record_updated`, `tables.record_deleted`, `tables.reminder` —
each with `table_id`, `table`, `base`, `record_id`, `title`, `values` (text by field name) and `url`.
