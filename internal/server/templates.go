package server

import (
	"net/http"
	"time"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- ready-made bases ----

func day(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }
func at(n int, hm string) string {
	return time.Now().AddDate(0, 0, n).Format("2006-01-02") + "T" + hm
}

func templates() []specBase {
	return []specBase{
		{Key: "projects", Name: "Projects & tasks", Icon: "🗂️", Color: "#1c7ed6", Description: "Projects with their tasks, who does what and by when, and how far each project is.",
			Tables: []specTable{
				{Name: "Projects", Icon: "🗂️", Fields: []specField{
					{Name: "Project", Type: tables.Text}, {Name: "Status", Type: tables.Select, Choices: []string{"Planned", "Active", "On hold", "Finished"}},
					{Name: "Start", Type: tables.Date}, {Name: "Deadline", Type: tables.Date, Options: tables.Options{Remind: true}}, {Name: "Budget", Type: tables.Currency},
					{Name: "About", Type: tables.LongText},
				}, Rows: []map[string]any{
					{"Project": "New website", "Status": "Active", "Start": day(-20), "Deadline": day(25), "Budget": 2400, "About": "Calmer pages that work on phones."},
					{"Project": "Spring newsletter", "Status": "Planned", "Start": day(10), "Deadline": day(40), "Budget": 300},
					{"Project": "Office move", "Status": "Finished", "Start": day(-90), "Deadline": day(-30), "Budget": 5200},
				}, Views: []specView{{Name: "By status", Type: "board", By: "Status"}, {Name: "Deadlines", Type: "calendar", By: "Deadline"}}},
				{Name: "Tasks", Icon: "✅", Fields: []specField{
					{Name: "Task", Type: tables.Text}, {Name: "Project", Type: tables.Link, Link: "Projects"}, {Name: "Who", Type: tables.Text},
					{Name: "Due", Type: tables.Date}, {Name: "Done", Type: tables.Checkbox}, {Name: "Priority", Type: tables.Select, Choices: []string{"High", "Normal", "Low"}},
					{Name: "Days left", Type: tables.Formula, Expr: `IF(AND({Due}, NOT({Done})), DAYS({Due}, TODAY()))`},
				}, Rows: []map[string]any{
					{"Task": "Write the texts", "Project": "New website", "Who": "Ana", "Due": day(3), "Priority": "High"},
					{"Task": "Pick photos", "Project": "New website", "Who": "Emir", "Due": day(6), "Priority": "Normal"},
					{"Task": "Show it to the client", "Project": "New website", "Who": "Ana", "Due": day(14), "Priority": "Normal"},
					{"Task": "Book the van", "Project": "Office move", "Who": "Emir", "Due": day(-60), "Done": true},
					{"Task": "Draft the first issue", "Project": "Spring newsletter", "Who": "Lejla", "Due": day(18), "Priority": "Low"},
				}, Views: []specView{{Name: "Open", Type: "grid", By: "Project", Sort: "Due", Filters: []specFilter{{Field: "Done", Op: "unchecked"}}}, {Name: "Calendar", Type: "calendar", By: "Due"}}},
			}},
		{Key: "equipment", Name: "Equipment bookings", Icon: "🎛️", Color: "#0c8599", Description: "Shared things (a camera, the van, a meeting room) and who has them when, so nobody books twice.",
			Tables: []specTable{
				{Name: "Equipment", Icon: "🎛️", Fields: []specField{
					{Name: "Name", Type: tables.Text}, {Name: "Kind", Type: tables.Select, Choices: []string{"Camera", "Audio", "Vehicle", "Room", "Tools"}},
					{Name: "Where it is", Type: tables.Text}, {Name: "Photo", Type: tables.Attachment}, {Name: "Value", Type: tables.Currency}, {Name: "Notes", Type: tables.LongText},
				}, Rows: []map[string]any{
					{"Name": "Sony A7 IV", "Kind": "Camera", "Where it is": "Cupboard 2", "Value": 2300},
					{"Name": "Zoom H6 recorder", "Kind": "Audio", "Where it is": "Cupboard 2", "Value": 320},
					{"Name": "Office van", "Kind": "Vehicle", "Where it is": "Yard", "Value": 18000},
					{"Name": "Big meeting room", "Kind": "Room", "Where it is": "First floor"},
				}, Views: []specView{{Name: "Gallery", Type: "gallery", By: "Photo"}}},
				{Name: "Bookings", Icon: "📅", Fields: []specField{
					{Name: "Booking", Type: tables.Text}, {Name: "Equipment", Type: tables.Link, Link: "Equipment"}, {Name: "Who", Type: tables.Text},
					{Name: "From", Type: tables.Date, Options: tables.Options{Time: true}}, {Name: "Until", Type: tables.Date, Options: tables.Options{Time: true}},
					{Name: "Status", Type: tables.Select, Choices: []string{"Asked", "Confirmed", "Returned"}}, {Name: "Notes", Type: tables.LongText},
				}, Rows: []map[string]any{
					{"Booking": "Wedding shoot", "Equipment": "Sony A7 IV", "Who": "Lejla", "From": at(2, "09:00"), "Until": at(2, "22:00"), "Status": "Confirmed"},
					{"Booking": "Podcast recording", "Equipment": "Zoom H6 recorder", "Who": "Emir", "From": at(4, "14:00"), "Until": at(4, "17:00"), "Status": "Asked"},
					{"Booking": "Delivery to Mostar", "Equipment": "Office van", "Who": "Marko", "From": at(1, "07:30"), "Until": at(1, "18:00"), "Status": "Confirmed"},
					{"Booking": "Team meeting", "Equipment": "Big meeting room", "Who": "Ana", "From": at(-3, "10:00"), "Until": at(-3, "11:30"), "Status": "Returned"},
				}, Views: []specView{{Name: "Calendar", Type: "calendar", By: "From"}, {Name: "By status", Type: "board", By: "Status"}}},
			}},
		{Key: "suppliers", Name: "Supplier comparison", Icon: "⚖️", Color: "#e8590c", Description: "Offers from several suppliers side by side: price, delivery time, how good they are, and which one you picked.",
			Tables: []specTable{
				{Name: "Suppliers", Icon: "🏭", Fields: []specField{
					{Name: "Supplier", Type: tables.Text}, {Name: "Website", Type: tables.URL}, {Name: "Email", Type: tables.Email}, {Name: "Phone", Type: tables.Phone},
					{Name: "How good", Type: tables.Rating}, {Name: "Notes", Type: tables.LongText},
				}, Rows: []map[string]any{
					{"Supplier": "Drvo Commerce", "Website": "https://example.com/drvo", "Email": "prodaja@example.com", "How good": 4},
					{"Supplier": "Bosna Print", "Website": "https://example.com/print", "How good": 5},
					{"Supplier": "EuroPack GmbH", "Email": "sales@example.de", "How good": 3},
				}},
				{Name: "Offers", Icon: "🏷️", Fields: []specField{
					{Name: "What", Type: tables.Text}, {Name: "Supplier", Type: tables.Link, Link: "Suppliers"}, {Name: "Price", Type: tables.Currency},
					{Name: "Delivery days", Type: tables.Number}, {Name: "Valid until", Type: tables.Date}, {Name: "Picked", Type: tables.Checkbox},
					{Name: "Price per day of waiting", Type: tables.Formula, Expr: `IF({Delivery days}, ROUND({Price} / {Delivery days}, 2))`},
				}, Rows: []map[string]any{
					{"What": "1000 boxes", "Supplier": "EuroPack GmbH", "Price": 840, "Delivery days": 14, "Valid until": day(20)},
					{"What": "1000 boxes", "Supplier": "Bosna Print", "Price": 910, "Delivery days": 5, "Valid until": day(12), "Picked": true},
					{"What": "Shelves for the shop", "Supplier": "Drvo Commerce", "Price": 1250, "Delivery days": 21, "Valid until": day(30)},
				}, Views: []specView{{Name: "Cheapest first", Type: "grid", By: "What", Sort: "Price", Sums: map[string]string{"Price": "min"}}}},
			}},
		{Key: "research", Name: "Research notes", Icon: "🔬", Color: "#7048e8", Description: "What you read and what you found in it, with tags, how sure you are and where it came from.",
			Tables: []specTable{
				{Name: "Sources", Icon: "📚", Fields: []specField{
					{Name: "Title", Type: tables.Text}, {Name: "Author", Type: tables.Text}, {Name: "Year", Type: tables.Number}, {Name: "Link", Type: tables.URL},
					{Name: "Kind", Type: tables.Select, Choices: []string{"Paper", "Book", "Article", "Interview", "Data"}}, {Name: "Read", Type: tables.Checkbox}, {Name: "File", Type: tables.Attachment},
				}, Rows: []map[string]any{
					{"Title": "Sleep and memory in adults", "Author": "Walker", "Year": 2017, "Kind": "Book", "Read": true},
					{"Title": "Remote work and focus", "Author": "Newport", "Year": 2021, "Kind": "Article"},
					{"Title": "Interview with a nurse", "Author": "—", "Year": 2026, "Kind": "Interview", "Read": true},
				}},
				{Name: "Findings", Icon: "💡", Fields: []specField{
					{Name: "Finding", Type: tables.Text}, {Name: "Source", Type: tables.Link, Link: "Sources"}, {Name: "Tags", Type: tables.Multi, Choices: []string{"sleep", "focus", "work", "health"}},
					{Name: "How sure", Type: tables.Rating}, {Name: "Details", Type: tables.LongText}, {Name: "Added", Type: tables.Created},
				}, Rows: []map[string]any{
					{"Finding": "Seven hours is the floor for most people", "Source": "Sleep and memory in adults", "Tags": "sleep, health", "How sure": 4},
					{"Finding": "Notifications cost more focus than people think", "Source": "Remote work and focus", "Tags": "focus, work", "How sure": 3},
					{"Finding": "Night shifts change appetite", "Source": "Interview with a nurse", "Tags": "sleep, work", "How sure": 2},
				}, Views: []specView{{Name: "Cards", Type: "gallery"}}},
			}},
		{Key: "content", Name: "Content calendar", Icon: "📣", Color: "#c2255c", Description: "Posts, newsletters and videos from idea to published, on a board and a calendar.",
			Tables: []specTable{{Name: "Posts", Icon: "📣", Fields: []specField{
				{Name: "Title", Type: tables.Text}, {Name: "Stage", Type: tables.Select, Choices: []string{"Idea", "Writing", "Ready", "Published"}},
				{Name: "Where", Type: tables.Multi, Choices: []string{"Blog", "Instagram", "LinkedIn", "Newsletter", "YouTube"}}, {Name: "Publish on", Type: tables.Date, Options: tables.Options{Remind: true}},
				{Name: "Who", Type: tables.Text}, {Name: "Draft", Type: tables.LongText}, {Name: "Image", Type: tables.Attachment},
			}, Rows: []map[string]any{
				{"Title": "Five things we learned this year", "Stage": "Writing", "Where": "Blog, Newsletter", "Publish on": day(5), "Who": "Ana"},
				{"Title": "Behind the scenes video", "Stage": "Idea", "Where": "YouTube, Instagram", "Publish on": day(12)},
				{"Title": "New price list", "Stage": "Ready", "Where": "LinkedIn", "Publish on": day(2), "Who": "Emir"},
				{"Title": "Summer recap", "Stage": "Published", "Where": "Blog", "Publish on": day(-40)},
			}, Views: []specView{{Name: "Board", Type: "board", By: "Stage"}, {Name: "Calendar", Type: "calendar", By: "Publish on"}}}}},
		{Key: "event", Name: "Event planning", Icon: "🎉", Color: "#f08c00", Description: "Guests and their answers, what they eat, and the to-dos before the day.",
			Tables: []specTable{
				{Name: "Guests", Icon: "🙋", Fields: []specField{
					{Name: "Name", Type: tables.Text}, {Name: "Email", Type: tables.Email}, {Name: "Answer", Type: tables.Select, Choices: []string{"Coming", "Maybe", "Not coming", "No answer"}},
					{Name: "People", Type: tables.Number}, {Name: "Food", Type: tables.Select, Choices: []string{"Anything", "Vegetarian", "Vegan", "No gluten"}}, {Name: "Contact", Type: tables.Pocket, App: "contact"},
				}, Rows: []map[string]any{
					{"Name": "Selma Hadžić", "Answer": "Coming", "People": 2, "Food": "Vegetarian"},
					{"Name": "Marko Jurić", "Answer": "Maybe", "People": 1, "Food": "Anything"},
					{"Name": "Nina Vuković", "Answer": "Coming", "People": 3, "Food": "No gluten"},
					{"Name": "Daniel Weber", "Answer": "No answer", "People": 1},
				}, Views: []specView{{Name: "Answers", Type: "board", By: "Answer"}, {Name: "Totals", Type: "grid", By: "Answer", Sums: map[string]string{"People": "sum"}}}},
				{Name: "To do", Icon: "📝", Fields: []specField{
					{Name: "What", Type: tables.Text}, {Name: "By", Type: tables.Date, Options: tables.Options{Remind: true}}, {Name: "Who", Type: tables.Text}, {Name: "Done", Type: tables.Checkbox}, {Name: "Cost", Type: tables.Currency},
				}, Rows: []map[string]any{
					{"What": "Book the place", "By": day(-5), "Who": "Ana", "Done": true, "Cost": 400},
					{"What": "Order the cake", "By": day(7), "Who": "Emir", "Cost": 85},
					{"What": "Send reminders", "By": day(10), "Who": "Ana"},
				}, Views: []specView{{Name: "Calendar", Type: "calendar", By: "By"}}},
			}},
		{Key: "collection", Name: "A collection", Icon: "💿", Color: "#5c940d", Description: "Records, LEGO, coins or cards: what you have, its state, what it cost and what it is worth now.",
			Tables: []specTable{{Name: "Collection", Icon: "💿", Fields: []specField{
				{Name: "Name", Type: tables.Text}, {Name: "Kind", Type: tables.Select, Choices: []string{"Vinyl", "LEGO", "Coin", "Card", "Book"}}, {Name: "Photo", Type: tables.Attachment},
				{Name: "Year", Type: tables.Number}, {Name: "State", Type: tables.Select, Choices: []string{"Mint", "Very good", "Good", "Worn"}},
				{Name: "Paid", Type: tables.Currency}, {Name: "Worth now", Type: tables.Currency}, {Name: "Gain", Type: tables.Formula, Expr: `{Worth now} - {Paid}`, Options: tables.Options{Decimals: 2}},
				{Name: "Want more like it", Type: tables.Checkbox},
			}, Rows: []map[string]any{
				{"Name": "Pink Floyd – Animals (1977)", "Kind": "Vinyl", "Year": 1977, "State": "Very good", "Paid": 35, "Worth now": 60},
				{"Name": "LEGO 10497 Galaxy Explorer", "Kind": "LEGO", "Year": 2022, "State": "Mint", "Paid": 90, "Worth now": 120},
				{"Name": "1 KM 1998", "Kind": "Coin", "Year": 1998, "State": "Good", "Paid": 2, "Worth now": 5},
			}, Views: []specView{{Name: "Gallery", Type: "gallery", By: "Photo"}, {Name: "By kind", Type: "grid", By: "Kind", Sums: map[string]string{"Paid": "sum", "Worth now": "sum"}}}}}},
		{Key: "jobs", Name: "Job applications", Icon: "💼", Color: "#495057", Description: "Where you applied, what happened, the next step and the people you talked to.",
			Tables: []specTable{{Name: "Applications", Icon: "💼", Fields: []specField{
				{Name: "Company", Type: tables.Text}, {Name: "Role", Type: tables.Text}, {Name: "Stage", Type: tables.Select, Choices: []string{"Want to apply", "Applied", "Talking", "Offer", "No"}},
				{Name: "Applied on", Type: tables.Date}, {Name: "Next step", Type: tables.Text}, {Name: "Next step on", Type: tables.Date, Options: tables.Options{Remind: true}},
				{Name: "Salary", Type: tables.Currency}, {Name: "Ad", Type: tables.URL}, {Name: "Person there", Type: tables.Pocket, App: "contact"}, {Name: "Notes", Type: tables.LongText},
			}, Rows: []map[string]any{
				{"Company": "Nordtrail", "Role": "Product designer", "Stage": "Talking", "Applied on": day(-12), "Next step": "Second interview", "Next step on": day(3)},
				{"Company": "Studio Atlas", "Role": "Frontend developer", "Stage": "Applied", "Applied on": day(-4)},
				{"Company": "Bergholz GmbH", "Role": "UX lead", "Stage": "Want to apply"},
			}, Views: []specView{{Name: "Board", Type: "board", By: "Stage"}, {Name: "Next steps", Type: "calendar", By: "Next step on"}}}}},
	}
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, t := range templates() {
		var names []string
		for _, tb := range t.Tables {
			names = append(names, tb.Name)
		}
		out = append(out, map[string]any{"key": t.Key, "name": t.Name, "icon": t.Icon, "color": t.Color, "description": t.Description, "tables": names})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUseTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Examples bool `json:"examples"`
	}
	_ = decode(r, &in)
	for _, t := range templates() {
		if t.Key == chiParam(r, "key") {
			b, err := s.build(ctx, actorID(ctx), t, 0, in.Examples)
			if err != nil {
				s.fail(w, err, "template")
				return
			}
			writeJSON(w, http.StatusCreated, b)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "not found")
}
