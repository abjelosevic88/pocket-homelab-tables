package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Tables as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "tables:read", Name: "See your tables", Description: "Read your bases, tables and rows (with their values by field name), search them, and see the rows with a date on a day or about a person — for your journal, search, a contact's page, dashboards and Pocket AI."},
	{Scope: "tables:write", Name: "Add rows to your tables", Description: "Add a row to one of your tables (an answer to a form, a step of an automation) and comment on a row. It may change only the rows that app added itself."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(bases|summary|search|lookup|today|day/\d{4}-\d{2}-\d{2})$`), "tables:read"},
	{"GET", regexp.MustCompile(`^/api/v1/tables/\d+(/records|/export\.csv)?$`), "tables:read"},
	{"GET", regexp.MustCompile(`^/api/v1/records/\d+$`), "tables:read"},
	{"POST", regexp.MustCompile(`^/api/v1/tables/\d+/records(/batch)?$`), "tables:write"},
	{"PUT", regexp.MustCompile(`^/api/v1/records/\d+$`), "tables:write"},
	{"POST", regexp.MustCompile(`^/api/v1/records/\d+/comments$`), "tables:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "tables" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
