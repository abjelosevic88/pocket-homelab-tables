package server

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- demo data (DEMO_DATA=true) ----
//
// A small studio's bases after a few months: projects and tasks, shared
// equipment and its bookings, supplier offers side by side, a content
// calendar and a record collection — enough to try every view and for the
// screenshots.

func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if bases, err := s.store.ListBases(ctx, uid); err != nil || len(bases) > 0 {
		return err
	}
	rnd := rand.New(rand.NewSource(42))
	for _, key := range []string{"projects", "equipment", "suppliers", "content", "collection"} {
		for _, t := range templates() {
			if t.Key != key {
				continue
			}
			if key == "projects" {
				t = moreProjects(t, rnd)
			}
			if _, err := s.build(ctx, uid, t, 0, true); err != nil {
				return fmt.Errorf("demo %s: %w", key, err)
			}
		}
	}
	// a few comments and a change, so a row has a story
	sheets, _ := s.store.ListSheets(ctx, uid, 0)
	for _, t := range sheets {
		recs, _ := s.store.Records(ctx, uid, t.ID, false)
		if len(recs) == 0 {
			continue
		}
		_ = s.store.AddHistory(ctx, uid, &store.History{RecordID: recs[0].ID, Kind: "comment", Text: "Checked this with Ana on Monday — fine as it is."})
	}
	return nil
}

// moreProjects gives the projects template a few months of tasks.
func moreProjects(t specBase, rnd *rand.Rand) specBase {
	who := []string{"Ana", "Emir", "Lejla", "Marko"}
	work := []string{"Call the printer", "Fix the contact page", "Write the about text", "Order business cards", "Send the invoice", "Check the photos",
		"Plan the launch day", "Update the price list", "Answer the feedback", "Back up the old site", "Test on phones", "Translate the menu"}
	projects := []string{"New website", "Spring newsletter", "Office move"}
	for i, w := range work {
		row := map[string]any{"Task": w, "Project": projects[rnd.Intn(len(projects))], "Who": who[rnd.Intn(len(who))],
			"Due": day(rnd.Intn(40) - 15), "Priority": []string{"High", "Normal", "Normal", "Low"}[rnd.Intn(4)]}
		if i%3 == 0 {
			row["Done"] = true
		}
		t.Tables[1].Rows = append(t.Tables[1].Rows, row)
	}
	t.Tables[0].Fields = append(t.Tables[0].Fields,
		specField{Name: "Tasks done", Type: tables.Rollup, Through: "Tasks", Of: "Done", Fn: "checked"},
		specField{Name: "Task count", Type: tables.Rollup, Through: "Tasks", Of: "Task", Fn: "counta"})
	return t
}
