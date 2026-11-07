package server

import (
	"context"
	"net/http"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// handleApps says per app whether tables can reach it, and if not why and
// what to do — the pages grey out what does not work.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

// handleFunctions lists what a formula can use, for the help sheet.
func (s *Server) handleFunctions(w http.ResponseWriter, r *http.Request) {
	out := []map[string]string{}
	for _, f := range tables.Functions() {
		out = append(out, map[string]string{"name": f[0], "takes": f[1], "example": f[2]})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCalendarNow brings a table's calendar events up to date now.
func (s *Server) handleCalendarNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t, err := s.store.GetSheet(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	s.syncSheetCalendar(context.WithoutCancel(ctx), uid, t.ID)
	made, _ := s.store.MadeInSheet(ctx, uid, t.ID, "sync")
	writeJSON(w, http.StatusOK, map[string]any{"events": len(made)})
}
