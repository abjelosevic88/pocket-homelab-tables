package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- bases ----

const baseCols = `id, user_id, name, icon, color, description, position, template, created_at, updated_at`

func scanBase(sc interface{ Scan(...any) error }) (*Base, error) {
	b := &Base{}
	err := sc.Scan(&b.ID, &b.UserID, &b.Name, &b.Icon, &b.Color, &b.Description, &b.Position, &b.Template, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}

// ListBases lists a person's bases in order.
func (s *Store) ListBases(ctx context.Context, uid int64) ([]*Base, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+baseCols+` FROM bases WHERE user_id = ? ORDER BY position, id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Base{}
	for rows.Next() {
		b, err := scanBase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBase loads one of a person's bases.
func (s *Store) GetBase(ctx context.Context, uid, id int64) (*Base, error) {
	b, err := scanBase(s.DB.QueryRowContext(ctx, `SELECT `+baseCols+` FROM bases WHERE user_id = ? AND id = ?`, uid, id))
	return b, notFound(err)
}

// CreateBase adds a base at the end.
func (s *Store) CreateBase(ctx context.Context, b *Base) error {
	now := Now()
	if b.Position == 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM bases WHERE user_id = ?`, b.UserID).Scan(&b.Position)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO bases (user_id, name, icon, color, description, position, template, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.UserID, b.Name, b.Icon, b.Color, b.Description, b.Position, b.Template, now, now)
	if err != nil {
		return err
	}
	b.ID, _ = res.LastInsertId()
	b.CreatedAt, b.UpdatedAt = now, now
	return nil
}

// UpdateBase saves a base's name, icon, colour, description and place.
func (s *Store) UpdateBase(ctx context.Context, b *Base) error {
	b.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE bases SET name = ?, icon = ?, color = ?, description = ?, position = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		b.Name, b.Icon, b.Color, b.Description, b.Position, b.UpdatedAt, b.UserID, b.ID)
	return affected(res, err)
}

// DeleteBase removes a base with all its tables.
func (s *Store) DeleteBase(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM bases WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- tables (sheets) ----

const sheetCols = `id, user_id, base_id, name, icon, description, position, next_number, settings, created_at, updated_at`

func scanSheet(sc interface{ Scan(...any) error }) (*Sheet, error) {
	t := &Sheet{}
	var set string
	err := sc.Scan(&t.ID, &t.UserID, &t.BaseID, &t.Name, &t.Icon, &t.Description, &t.Position, &t.NextNumber, &set, &t.CreatedAt, &t.UpdatedAt)
	if err == nil {
		_ = json.Unmarshal([]byte(set), &t.Settings)
	}
	return t, err
}

// ListSheets lists all of a person's tables (base 0) or one base's, with row counts.
func (s *Store) ListSheets(ctx context.Context, uid, baseID int64) ([]*Sheet, error) {
	q := `SELECT ` + sheetCols + ` FROM sheets WHERE user_id = ?`
	args := []any{uid}
	if baseID > 0 {
		q += ` AND base_id = ?`
		args = append(args, baseID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY base_id, position, id`, args...)
	if err != nil {
		return nil, err
	}
	out := []*Sheet{}
	for rows.Next() {
		t, err := scanSheet(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, t)
	}
	rows.Close()
	counts := map[int64]int{}
	crow, err := s.DB.QueryContext(ctx, `SELECT sheet_id, COUNT(1) FROM records WHERE user_id = ? AND deleted_at IS NULL GROUP BY sheet_id`, uid)
	if err == nil {
		for crow.Next() {
			var id int64
			var n int
			if crow.Scan(&id, &n) == nil {
				counts[id] = n
			}
		}
		crow.Close()
	}
	for _, t := range out {
		t.Records = counts[t.ID]
	}
	return out, nil
}

// GetSheet loads one of a person's tables.
func (s *Store) GetSheet(ctx context.Context, uid, id int64) (*Sheet, error) {
	t, err := scanSheet(s.DB.QueryRowContext(ctx, `SELECT `+sheetCols+` FROM sheets WHERE user_id = ? AND id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM records WHERE sheet_id = ? AND deleted_at IS NULL`, id).Scan(&t.Records)
	return t, nil
}

// CreateSheet adds a table at the end of its base.
func (s *Store) CreateSheet(ctx context.Context, t *Sheet) error {
	now := Now()
	if t.Position == 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM sheets WHERE user_id = ? AND base_id = ?`, t.UserID, t.BaseID).Scan(&t.Position)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO sheets (user_id, base_id, name, icon, description, position, next_number, settings, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		t.UserID, t.BaseID, t.Name, t.Icon, t.Description, t.Position, toJSON(t.Settings), now, now)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	t.NextNumber, t.CreatedAt, t.UpdatedAt = 1, now, now
	return nil
}

// UpdateSheet saves a table's name, icon, description, base, place and settings.
func (s *Store) UpdateSheet(ctx context.Context, t *Sheet) error {
	t.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE sheets SET base_id = ?, name = ?, icon = ?, description = ?, position = ?, settings = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		t.BaseID, t.Name, t.Icon, t.Description, t.Position, toJSON(t.Settings), t.UpdatedAt, t.UserID, t.ID)
	return affected(res, err)
}

// TouchSheet marks a table changed.
func (s *Store) TouchSheet(ctx context.Context, uid, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE sheets SET updated_at = ? WHERE user_id = ? AND id = ?`, Now(), uid, id)
}

// DeleteSheet removes a table with its fields, views and rows.
func (s *Store) DeleteSheet(ctx context.Context, uid, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	// link fields in other tables pointing here go too (and lookups through them)
	if _, err := tx.ExecContext(ctx, `DELETE FROM fields WHERE user_id = ? AND type = 'link' AND json_extract(options, '$.table_id') = ?`, uid, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM sheets WHERE user_id = ? AND id = ?`, uid, id)
	if err := affected(res, err); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- fields ----

const fieldCols = `id, sheet_id, name, type, options, description, position, is_primary, required`

func scanField(sc interface{ Scan(...any) error }) (*tables.Field, error) {
	f := &tables.Field{}
	var opt string
	var prim, req int
	err := sc.Scan(&f.ID, &f.TableID, &f.Name, &f.Type, &opt, &f.Description, &f.Position, &prim, &req)
	if err == nil {
		_ = json.Unmarshal([]byte(opt), &f.Options)
		f.Primary, f.Required = prim == 1, req == 1
	}
	return f, err
}

// Fields lists a table's fields in order (sheet 0 = all of a person's).
func (s *Store) Fields(ctx context.Context, uid, sheetID int64) ([]*tables.Field, error) {
	q := `SELECT ` + fieldCols + ` FROM fields WHERE user_id = ?`
	args := []any{uid}
	if sheetID > 0 {
		q += ` AND sheet_id = ?`
		args = append(args, sheetID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY sheet_id, position, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*tables.Field{}
	for rows.Next() {
		f, err := scanField(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetField loads one of a person's fields.
func (s *Store) GetField(ctx context.Context, uid, id int64) (*tables.Field, error) {
	f, err := scanField(s.DB.QueryRowContext(ctx, `SELECT `+fieldCols+` FROM fields WHERE user_id = ? AND id = ?`, uid, id))
	return f, notFound(err)
}

// CreateField adds a field (at the end unless a position is given).
func (s *Store) CreateField(ctx context.Context, uid int64, f *tables.Field) error {
	if f.Position == 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM fields WHERE sheet_id = ?`, f.TableID).Scan(&f.Position)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO fields (user_id, sheet_id, name, type, options, description, position, is_primary, required) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, f.TableID, f.Name, f.Type, toJSON(f.Options), f.Description, f.Position, boolInt(f.Primary), boolInt(f.Required))
	if err != nil {
		return err
	}
	f.ID, _ = res.LastInsertId()
	return nil
}

// UpdateField saves a field.
func (s *Store) UpdateField(ctx context.Context, uid int64, f *tables.Field) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE fields SET name = ?, type = ?, options = ?, description = ?, position = ?, is_primary = ?, required = ? WHERE user_id = ? AND id = ?`,
		f.Name, f.Type, toJSON(f.Options), f.Description, f.Position, boolInt(f.Primary), boolInt(f.Required), uid, f.ID)
	return affected(res, err)
}

// SetPrimary makes one field the one that names the rows.
func (s *Store) SetPrimary(ctx context.Context, uid, sheetID, fieldID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE fields SET is_primary = (id = ?) WHERE user_id = ? AND sheet_id = ?`, fieldID, uid, sheetID)
	return err
}

// DeleteField removes a field and its values.
func (s *Store) DeleteField(ctx context.Context, uid int64, f *tables.Field) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	ids := []int64{f.ID}
	if f.Type == tables.Link && f.Options.Inverse > 0 {
		ids = append(ids, f.Options.Inverse) // the other side of a link goes with it
	}
	for _, id := range ids {
		key := "$." + `"` + tables.Key(id) + `"`
		if _, err := tx.ExecContext(ctx, `UPDATE records SET data = json_remove(data, ?) WHERE user_id = ? AND json_extract(data, ?) IS NOT NULL`, key, uid, key); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM fields WHERE user_id = ? AND id = ?`, uid, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- views ----

const viewCols = `id, user_id, sheet_id, name, type, config, position, created_at, updated_at`

func scanView(sc interface{ Scan(...any) error }) (*View, error) {
	v := &View{}
	var cfg string
	err := sc.Scan(&v.ID, &v.UserID, &v.SheetID, &v.Name, &v.Type, &cfg, &v.Position, &v.CreatedAt, &v.UpdatedAt)
	if err == nil {
		_ = json.Unmarshal([]byte(cfg), &v.Config)
		v.Config.Clean()
	}
	return v, err
}

// Clean fills the lists so the page never meets a null.
func (c *ViewConfig) Clean() {
	if c.Filters == nil {
		c.Filters = []tables.Filter{}
	}
	if c.Sorts == nil {
		c.Sorts = []tables.Sort{}
	}
	if c.Hidden == nil {
		c.Hidden = []int64{}
	}
	if c.Order == nil {
		c.Order = []int64{}
	}
	if c.Widths == nil {
		c.Widths = map[string]int{}
	}
	if c.Summaries == nil {
		c.Summaries = map[string]string{}
	}
	if c.Match != "any" {
		c.Match = "all"
	}
}

// Views lists a table's views in order.
func (s *Store) Views(ctx context.Context, uid, sheetID int64) ([]*View, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+viewCols+` FROM views WHERE user_id = ? AND sheet_id = ? ORDER BY position, id`, uid, sheetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*View{}
	for rows.Next() {
		v, err := scanView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetView loads one of a person's views.
func (s *Store) GetView(ctx context.Context, uid, id int64) (*View, error) {
	v, err := scanView(s.DB.QueryRowContext(ctx, `SELECT `+viewCols+` FROM views WHERE user_id = ? AND id = ?`, uid, id))
	return v, notFound(err)
}

// CreateView adds a view at the end.
func (s *Store) CreateView(ctx context.Context, v *View) error {
	now := Now()
	v.Config.Clean()
	if v.Position == 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM views WHERE sheet_id = ?`, v.SheetID).Scan(&v.Position)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO views (user_id, sheet_id, name, type, config, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		v.UserID, v.SheetID, v.Name, v.Type, toJSON(v.Config), v.Position, now, now)
	if err != nil {
		return err
	}
	v.ID, _ = res.LastInsertId()
	v.CreatedAt, v.UpdatedAt = now, now
	return nil
}

// UpdateView saves a view.
func (s *Store) UpdateView(ctx context.Context, v *View) error {
	v.UpdatedAt = Now()
	v.Config.Clean()
	res, err := s.DB.ExecContext(ctx, `UPDATE views SET name = ?, type = ?, config = ?, position = ? , updated_at = ? WHERE user_id = ? AND id = ?`,
		v.Name, v.Type, toJSON(v.Config), v.Position, v.UpdatedAt, v.UserID, v.ID)
	return affected(res, err)
}

// DeleteView removes a view.
func (s *Store) DeleteView(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM views WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}
