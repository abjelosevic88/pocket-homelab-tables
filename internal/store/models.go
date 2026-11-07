package store

import (
	"encoding/json"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Base is a group of tables that belong together.
type Base struct {
	ID          int64   `json:"id"`
	UserID      int64   `json:"-"`
	Name        string  `json:"name"`
	Icon        string  `json:"icon"`
	Color       string  `json:"color"`
	Description string  `json:"description"`
	Position    float64 `json:"position"`
	Template    string  `json:"template"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`

	Tables []*Sheet `json:"tables,omitempty"`
}

// CalendarSync keeps a table's rows as events in Pocket Calendar, on the
// day (and time) of a date field.
type CalendarSync struct {
	On       bool  `json:"on"`
	Field    int64 `json:"field"`               // the date field
	EndField int64 `json:"end_field,omitempty"` // optional end date
}

// FormSync turns the answers to a Pocket Forms form into rows.
type FormSync struct {
	On     bool             `json:"on"`
	FormID string           `json:"form_id"`
	Title  string           `json:"title"`
	URL    string           `json:"url,omitempty"`
	Map    map[string]int64 `json:"map"` // form field id → this table's field id
	Status string           `json:"status,omitempty"`
	At     string           `json:"at,omitempty"` // last looked
	Made   int              `json:"made,omitempty"`
}

// DBSource is the database table or query (in Pocket Database) a table was made from.
type DBSource struct {
	Conn     int64    `json:"conn"`
	ConnName string   `json:"conn_name"`
	DB       string   `json:"db,omitempty"`
	Schema   string   `json:"schema,omitempty"`
	Table    string   `json:"table,omitempty"`
	SQL      string   `json:"sql,omitempty"`
	Key      []string `json:"key,omitempty"` // the columns that identify a row
	At       string   `json:"at,omitempty"`  // last brought in
}

// SheetSettings are a table's ties to the other apps.
type SheetSettings struct {
	Calendar *CalendarSync `json:"calendar,omitempty"`
	Form     *FormSync     `json:"form,omitempty"`
	TaskDone int64         `json:"task_done,omitempty"` // a checkbox ticked when a task made from the row is done
	TaskDue  int64         `json:"task_due,omitempty"`  // the date field a new task takes its due day from
	FromApp  string        `json:"from_app,omitempty"`
	DB       *DBSource     `json:"db,omitempty"` // made from a database: the latest rows can be brought in  // made from a list in another app (a kind: contact, place…): new ones can be brought in
}

// Sheet is one table.
type Sheet struct {
	ID          int64         `json:"id"`
	UserID      int64         `json:"-"`
	BaseID      int64         `json:"base_id"`
	Name        string        `json:"name"`
	Icon        string        `json:"icon"`
	Description string        `json:"description"`
	Position    float64       `json:"position"`
	NextNumber  int64         `json:"-"`
	Settings    SheetSettings `json:"settings"`
	CreatedAt   string        `json:"created_at"`
	UpdatedAt   string        `json:"updated_at"`

	Records int `json:"records"` // filled for lists
}

// View is a saved way to look at a table.
type View struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"-"`
	SheetID   int64      `json:"table_id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"` // grid | board | calendar | gallery
	Config    ViewConfig `json:"config"`
	Position  float64    `json:"position"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
}

// ViewTypes are the ways to look at a table.
var ViewTypes = []string{"grid", "board", "calendar", "gallery"}

// ViewConfig is what a view keeps.
type ViewConfig struct {
	Filters   []tables.Filter   `json:"filters"`
	Match     string            `json:"match"` // all | any
	Sorts     []tables.Sort     `json:"sorts"`
	Hidden    []int64           `json:"hidden"`
	Order     []int64           `json:"order"`
	Widths    map[string]int    `json:"widths"`
	GroupBy   int64             `json:"group_by"`
	StackBy   int64             `json:"stack_by"`
	DateField int64             `json:"date_field"`
	Cover     int64             `json:"cover"`
	Summaries map[string]string `json:"summaries"`
	RowHeight string            `json:"row_height"` // short | medium | tall
	Color     int64             `json:"color"`      // a select field whose colour marks rows/cards
}

// Record is one row.
type Record struct {
	ID        int64          `json:"id"`
	UserID    int64          `json:"-"`
	SheetID   int64          `json:"table_id"`
	Number    int64          `json:"number"`
	Data      map[string]any `json:"-"`
	Position  float64        `json:"position"`
	Source    string         `json:"source"`
	SourceRef string         `json:"source_ref,omitempty"`
	DeletedAt string         `json:"deleted_at,omitempty"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// LinkRow is one stored link between two rows.
type LinkRow struct {
	FieldID int64
	From    int64
	To      int64
}

// History is one line of a row's story.
type History struct {
	ID        int64          `json:"id"`
	RecordID  int64          `json:"record_id"`
	Kind      string         `json:"kind"`
	Text      string         `json:"text"`
	Data      map[string]any `json:"data"`
	App       string         `json:"app"`
	CreatedAt string         `json:"created_at"`
}

// FileMeta is an attached file.
type FileMeta struct {
	ID        string `json:"id"`
	UserID    int64  `json:"-"`
	RecordID  int64  `json:"record_id"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

// Made is something a row has in another Pocket app.
type Made struct {
	ID        int64  `json:"id"`
	RecordID  int64  `json:"record_id"`
	Target    string `json:"target"`
	App       string `json:"app"`
	Ref       string `json:"ref"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	State     string `json:"state"`
	Stamp     string `json:"-"`
	CreatedAt string `json:"created_at"`
	CheckedAt string `json:"checked_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	Currency       string `json:"currency"`        // new money fields start in it
	MeetingMinutes int    `json:"meeting_minutes"` // how long a calendar event of a row with a time is
	AIHelp         bool   `json:"ai_help"`

	// Notifications.
	NotifyDigest bool   `json:"notify_digest"` // morning: rows whose reminder date is today
	NotifyHour   int    `json:"notify_hour"`
	NotifyURL    string `json:"notify_url"`
	NotifyFormat string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
