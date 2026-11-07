package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ---- rows ----

const recordCols = `id, user_id, sheet_id, number, data, position, source, source_ref, COALESCE(deleted_at, ''), created_at, updated_at`

func scanRecord(sc interface{ Scan(...any) error }) (*Record, error) {
	r := &Record{}
	var data string
	err := sc.Scan(&r.ID, &r.UserID, &r.SheetID, &r.Number, &data, &r.Position, &r.Source, &r.SourceRef, &r.DeletedAt, &r.CreatedAt, &r.UpdatedAt)
	if err == nil {
		r.Data = map[string]any{}
		_ = json.Unmarshal([]byte(data), &r.Data)
	}
	return r, err
}

func (s *Store) queryRecords(ctx context.Context, q string, args ...any) ([]*Record, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Record{}
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Records lists a table's rows in their order (or the deleted ones, newest first).
func (s *Store) Records(ctx context.Context, uid, sheetID int64, deleted bool) ([]*Record, error) {
	if deleted {
		return s.queryRecords(ctx, `SELECT `+recordCols+` FROM records WHERE user_id = ? AND sheet_id = ? AND deleted_at IS NOT NULL ORDER BY deleted_at DESC`, uid, sheetID)
	}
	return s.queryRecords(ctx, `SELECT `+recordCols+` FROM records WHERE user_id = ? AND sheet_id = ? AND deleted_at IS NULL ORDER BY position, id`, uid, sheetID)
}

// AllRecords lists all of a person's rows that are not deleted.
func (s *Store) AllRecords(ctx context.Context, uid int64) ([]*Record, error) {
	return s.queryRecords(ctx, `SELECT `+recordCols+` FROM records WHERE user_id = ? AND deleted_at IS NULL ORDER BY sheet_id, position, id`, uid)
}

// GetRecord loads one of a person's rows (deleted ones too).
func (s *Store) GetRecord(ctx context.Context, uid, id int64) (*Record, error) {
	r, err := scanRecord(s.DB.QueryRowContext(ctx, `SELECT `+recordCols+` FROM records WHERE user_id = ? AND id = ?`, uid, id))
	return r, notFound(err)
}

// RecordBySource finds a row another app added with this reference.
func (s *Store) RecordBySource(ctx context.Context, uid, sheetID int64, ref string) (*Record, error) {
	r, err := scanRecord(s.DB.QueryRowContext(ctx, `SELECT `+recordCols+` FROM records WHERE user_id = ? AND sheet_id = ? AND source_ref = ?`, uid, sheetID, ref))
	return r, notFound(err)
}

// CreateRecord adds a row (numbered in its table) at the end, or at
// r.Position when that is set.
func (s *Store) CreateRecord(ctx context.Context, r *Record) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := tx.QueryRowContext(ctx, `UPDATE sheets SET next_number = next_number + 1, updated_at = ? WHERE user_id = ? AND id = ? RETURNING next_number - 1`, Now(), r.UserID, r.SheetID).Scan(&r.Number); err != nil {
		return notFound(err)
	}
	if r.Position == 0 {
		_ = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM records WHERE sheet_id = ?`, r.SheetID).Scan(&r.Position)
	}
	if r.Data == nil {
		r.Data = map[string]any{}
	}
	now := Now()
	if r.CreatedAt == "" {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	res, err := tx.ExecContext(ctx, `INSERT INTO records (user_id, sheet_id, number, data, position, source, source_ref, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.UserID, r.SheetID, r.Number, toJSON(r.Data), r.Position, r.Source, r.SourceRef, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return conflict(err)
	}
	r.ID, _ = res.LastInsertId()
	return tx.Commit()
}

// SaveRecord stores a row's values and position.
func (s *Store) SaveRecord(ctx context.Context, r *Record) error {
	r.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE records SET data = ?, position = ?, updated_at = ? WHERE user_id = ? AND id = ?`, toJSON(r.Data), r.Position, r.UpdatedAt, r.UserID, r.ID)
	return affected(res, err)
}

// DeleteRecords moves rows to the bin (they stay 30 days).
func (s *Store) DeleteRecords(ctx context.Context, uid int64, ids []int64) (int, error) {
	n := 0
	now := Now()
	for _, id := range ids {
		res, err := s.DB.ExecContext(ctx, `UPDATE records SET deleted_at = ? WHERE user_id = ? AND id = ? AND deleted_at IS NULL`, now, uid, id)
		if err != nil {
			return n, err
		}
		k, _ := res.RowsAffected()
		n += int(k)
	}
	return n, nil
}

// RestoreRecord takes a row out of the bin.
func (s *Store) RestoreRecord(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE records SET deleted_at = NULL, updated_at = ? WHERE user_id = ? AND id = ? AND deleted_at IS NOT NULL`, Now(), uid, id)
	return affected(res, err)
}

// PurgeRecord removes a row for good.
func (s *Store) PurgeRecord(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM records WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}

// PurgeDeleted removes rows that were in the bin longer than the given time
// and returns the ids of their files (to remove from the disk).
func (s *Store) PurgeDeleted(ctx context.Context, older time.Duration) ([]string, error) {
	cut := time.Now().Add(-older).UTC().Format(time.RFC3339Nano)
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM records WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cut); err != nil {
		return nil, err
	}
	return s.orphanFiles(ctx)
}

// ---- links ----

// Links lists the stored links of these link fields.
func (s *Store) Links(ctx context.Context, uid int64, fieldIDs []int64) ([]LinkRow, error) {
	out := []LinkRow{}
	if len(fieldIDs) == 0 {
		return out, nil
	}
	q := `SELECT l.field_id, l.from_id, l.to_id FROM links l JOIN records a ON a.id = l.from_id JOIN records b ON b.id = l.to_id
		WHERE l.user_id = ? AND a.deleted_at IS NULL AND b.deleted_at IS NULL AND l.field_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(fieldIDs)), ",") + `) ORDER BY l.field_id, l.from_id, l.position, l.to_id`
	args := []any{uid}
	for _, id := range fieldIDs {
		args = append(args, id)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l LinkRow
		if err := rows.Scan(&l.FieldID, &l.From, &l.To); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetLinks replaces the links of one row in one field. With reverse set the
// row is on the "to" side (the field on the other table).
func (s *Store) SetLinks(ctx context.Context, uid, fieldID, recordID int64, others []int64, reverse bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	col := "from_id"
	if reverse {
		col = "to_id"
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM links WHERE user_id = ? AND field_id = ? AND `+col+` = ?`, uid, fieldID, recordID); err != nil {
		return err
	}
	for i, o := range others {
		from, to := recordID, o
		if reverse {
			from, to = o, recordID
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO links (user_id, field_id, from_id, to_id, position) VALUES (?, ?, ?, ?, ?)`, uid, fieldID, from, to, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OwnRecords keeps the ids that are rows of this person in this table.
func (s *Store) OwnRecords(ctx context.Context, uid, sheetID int64, ids []int64) []int64 {
	out := []int64{}
	for _, id := range ids {
		var n int
		if s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM records WHERE user_id = ? AND sheet_id = ? AND id = ? AND deleted_at IS NULL`, uid, sheetID, id).Scan(&n) == nil && n > 0 {
			out = append(out, id)
		}
	}
	return out
}

// ---- history ----

// AddHistory writes a line in a row's story.
func (s *Store) AddHistory(ctx context.Context, uid int64, h *History) error {
	if h.Data == nil {
		h.Data = map[string]any{}
	}
	h.CreatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO history (user_id, record_id, kind, text, data, app, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uid, h.RecordID, h.Kind, h.Text, toJSON(h.Data), h.App, h.CreatedAt)
	if err != nil {
		return err
	}
	h.ID, _ = res.LastInsertId()
	return nil
}

// HistoryOf lists a row's story, newest first.
func (s *Store) HistoryOf(ctx context.Context, uid, recordID int64) ([]*History, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, record_id, kind, text, data, app, created_at FROM history WHERE user_id = ? AND record_id = ? ORDER BY created_at DESC, id DESC LIMIT 200`, uid, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*History{}
	for rows.Next() {
		h := &History{}
		var data string
		if err := rows.Scan(&h.ID, &h.RecordID, &h.Kind, &h.Text, &data, &h.App, &h.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(data), &h.Data)
		out = append(out, h)
	}
	return out, rows.Err()
}

// DeleteComment removes one of a person's comments.
func (s *Store) DeleteComment(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM history WHERE user_id = ? AND id = ? AND kind = 'comment'`, uid, id)
	return affected(res, err)
}

// CommentCounts counts comments per row of a table.
func (s *Store) CommentCounts(ctx context.Context, uid, sheetID int64) map[int64]int {
	out := map[int64]int{}
	rows, err := s.DB.QueryContext(ctx, `SELECT h.record_id, COUNT(1) FROM history h JOIN records r ON r.id = h.record_id WHERE h.user_id = ? AND r.sheet_id = ? AND h.kind = 'comment' GROUP BY h.record_id`, uid, sheetID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if rows.Scan(&id, &n) == nil {
			out[id] = n
		}
	}
	return out
}

// ---- files ----

// AddFile records an attached file.
func (s *Store) AddFile(ctx context.Context, f *FileMeta) error {
	f.CreatedAt = Now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO files (id, user_id, record_id, name, mime, size, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.UserID, nullI(&f.RecordID), f.Name, f.Mime, f.Size, f.CreatedAt)
	return err
}

// GetFile loads one of a person's files.
func (s *Store) GetFile(ctx context.Context, uid int64, id string) (*FileMeta, error) {
	f := &FileMeta{}
	var rec sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, record_id, name, mime, size, created_at FROM files WHERE user_id = ? AND id = ?`, uid, id).
		Scan(&f.ID, &f.UserID, &rec, &f.Name, &f.Mime, &f.Size, &f.CreatedAt)
	f.RecordID = rec.Int64
	return f, notFound(err)
}

// orphanFiles removes the file rows no row's value mentions any more (a
// day after upload) and returns their ids.
func (s *Store) orphanFiles(ctx context.Context) ([]string, error) {
	cut := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
	rows, err := s.DB.QueryContext(ctx, `SELECT f.id FROM files f WHERE f.created_at < ? AND NOT EXISTS (SELECT 1 FROM records r WHERE r.user_id = f.user_id AND instr(r.data, '"' || f.id || '"') > 0)`, cut)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id)
	}
	return ids, nil
}

// ---- things in other apps ----

const madeCols = `id, record_id, target, app, ref, title, url, state, stamp, created_at, checked_at`

func scanMade(sc interface{ Scan(...any) error }) (*Made, error) {
	m := &Made{}
	err := sc.Scan(&m.ID, &m.RecordID, &m.Target, &m.App, &m.Ref, &m.Title, &m.URL, &m.State, &m.Stamp, &m.CreatedAt, &m.CheckedAt)
	return m, err
}

// AddMade records a thing made in another app.
func (s *Store) AddMade(ctx context.Context, uid int64, m *Made) error {
	m.CreatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO made (user_id, record_id, target, app, ref, title, url, state, stamp, created_at, checked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, m.RecordID, m.Target, m.App, m.Ref, m.Title, m.URL, m.State, m.Stamp, m.CreatedAt, m.CreatedAt)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

// UpdateMade saves a made thing's title, state and stamp.
func (s *Store) UpdateMade(ctx context.Context, uid int64, m *Made) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE made SET ref = ?, title = ?, url = ?, state = ?, stamp = ?, checked_at = ? WHERE user_id = ? AND id = ?`, m.Ref, m.Title, m.URL, m.State, m.Stamp, Now(), uid, m.ID)
	return err
}

// MadeFor lists what a row has in other apps (target "" = all).
func (s *Store) MadeFor(ctx context.Context, uid, recordID int64, target string) ([]*Made, error) {
	q := `SELECT ` + madeCols + ` FROM made WHERE user_id = ? AND record_id = ?`
	args := []any{uid, recordID}
	if target != "" {
		q += ` AND target = ?`
		args = append(args, target)
	}
	return s.queryMade(ctx, q+` ORDER BY id`, args...)
}

// MadeInSheet lists one kind of made thing for all rows of a table (deleted rows too).
func (s *Store) MadeInSheet(ctx context.Context, uid, sheetID int64, target string) ([]*Made, error) {
	return s.queryMade(ctx, `SELECT m.id, m.record_id, m.target, m.app, m.ref, m.title, m.url, m.state, m.stamp, m.created_at, m.checked_at FROM made m JOIN records r ON r.id = m.record_id
		WHERE m.user_id = ? AND r.sheet_id = ? AND m.target = ? ORDER BY m.id`, uid, sheetID, target)
}

// OpenTasks lists the tasks made from rows that are not done yet.
func (s *Store) OpenTasks(ctx context.Context, uid int64) ([]*Made, error) {
	return s.queryMade(ctx, `SELECT `+madeCols+` FROM made WHERE user_id = ? AND target = 'task' AND state != 'done' AND ref != '' ORDER BY checked_at LIMIT 200`, uid)
}

func (s *Store) queryMade(ctx context.Context, q string, args ...any) ([]*Made, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Made{}
	for rows.Next() {
		m, err := scanMade(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMade forgets a made thing.
func (s *Store) DeleteMade(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND id = ?`, uid, id)
	return affected(res, err)
}
