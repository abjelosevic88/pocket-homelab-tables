package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
)

// pushes runs every minute; once a day, at each person's own hour, it
// lists the rows whose reminder dates are today (date fields with "remind
// me" on) and tells the box (tables.reminder, one per row) for Pocket Flows.
func (s *Server) pushes(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
			continue
		}
		s.dailyCheck(ctx, uid, st, now)
	}
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

// dailyCheck builds and sends one person's morning message.
func (s *Server) dailyCheck(ctx context.Context, uid int64, st store.Settings, now time.Time) (string, bool) {
	all, bm, _, err := s.everything(ctx, uid)
	if err != nil {
		return "", false
	}
	t := now.Format("2006-01-02")
	due := s.due(all, bm, t, t, true)
	for _, h := range due {
		s.publish(ctx, uid, "tables.reminder", h.Table+": "+h.Title, map[string]any{"table_id": h.TableID, "table": h.Table, "base": h.Base,
			"record_id": h.ID, "title": h.Title, "field": h.Field, "date": h.Date, "url": h.URL})
	}
	if len(due) == 0 || !st.NotifyDigest {
		return "", false
	}
	var lines []string
	for i, h := range due {
		if i >= 12 {
			lines = append(lines, fmt.Sprintf("…and %d more.", len(due)-12))
			break
		}
		when := ""
		if len(h.Date) > 10 {
			when = " at " + h.Date[11:16]
		}
		lines = append(lines, fmt.Sprintf("• %s — %s (%s%s)", h.Title, h.Table, strings.ToLower(h.Field), when))
	}
	body := strings.Join(lines, "\n")
	link := ""
	if s.cfg.BaseURL != "" {
		link = strings.TrimRight(s.cfg.BaseURL, "/") + "/"
	}
	title := fmt.Sprintf("%d %s today", len(due), plural(len(due), "row", "rows"))
	return body, s.push(ctx, uid, st, title, body, link)
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, link string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"spiral_notepad"}, Link: link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Tables", Body: notifyTestBody, Priority: "info", Tags: []string{"spiral_notepad"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const notifyTestBody = "Notifications work. Each morning, if you want it, a note lists the rows whose dates you asked to be reminded of are today."
