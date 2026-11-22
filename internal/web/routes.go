package web

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// flag builds "--name=value"; the value can never be read as another option.
func flag(name, value string) string { return "--" + name + "=" + value }

// scope turns the page's project choice into command options. The dashboard is operated by the
// owner in their own browser, so unlike the MCP server it may name any project or all of them.
func scope(project string, all bool) []string {
	if all {
		return []string{"--all-projects"}
	}
	return []string{flag("project", project)}
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		csrf := s.csrf
		s.mu.Unlock()
		write(w, http.StatusOK, map[string]any{"data": map[string]string{"csrf": csrf, "version": s.version}})
	})

	// Overview and diagnostics.
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) { s.command(w, "status") })
	mux.HandleFunc("GET /api/doctor", func(w http.ResponseWriter, r *http.Request) { s.command(w, "doctor") })
	mux.HandleFunc("POST /api/ingest", func(w http.ResponseWriter, r *http.Request) { s.command(w, "ingest") })

	// Onboarding: what happens in unregistered folders, exclusions, projects.
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) { s.command(w, "setup") })
	mux.HandleFunc("POST /api/settings/auto-add", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Mode  string `json:"mode"`
			Level string `json:"level"`
		}
		if !decode(w, r, &in) {
			return
		}
		args := []string{"setup", flag("auto-add", in.Mode)}
		if in.Level != "" {
			args = append(args, flag("level", in.Level))
		}
		s.command(w, args...)
	})
	mux.HandleFunc("GET /api/exclusions", func(w http.ResponseWriter, r *http.Request) { s.command(w, "exclude", "list") })
	mux.HandleFunc("POST /api/exclusions", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Path   string `json:"path"`
			Remove bool   `json:"remove"`
		}
		if !decode(w, r, &in) {
			return
		}
		verb := "add"
		if in.Remove {
			verb = "remove"
		}
		s.command(w, "exclude", verb, "--", in.Path)
	})
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) { s.command(w, "project", "list") })
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Path  string `json:"path"`
			Level string `json:"level"`
			Name  string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		args := []string{"project", "add"}
		if in.Level != "" { // no level means the command answers "choose one"; there is no default
			args = append(args, flag("capture", in.Level))
		}
		if in.Name != "" {
			args = append(args, flag("name", in.Name))
		}
		s.command(w, append(args, "--", in.Path)...)
	})
	mux.HandleFunc("POST /api/projects/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Level string `json:"level"`
			Mode  string `json:"mode"`
			To    string `json:"to"`   // move: the folder's new place
			Name  string `json:"name"` // move: a new name, optional
		}
		if !decode(w, r, &in) {
			return
		}
		switch a := r.PathValue("action"); a {
		case "recall":
			s.command(w, "project", "recall", "--", r.PathValue("id"), in.Mode)
		case "pause", "resume":
			s.command(w, "project", a, "--", r.PathValue("id"))
		case "capture":
			s.command(w, "project", "capture", "--", r.PathValue("id"), in.Level)
		case "move":
			args := []string{"project", "move"}
			if in.Name != "" {
				args = append(args, flag("name", in.Name))
			}
			s.command(w, append(args, "--", r.PathValue("id"), in.To)...)
		default:
			fail(w, http.StatusNotFound, "not_found", "no such action")
		}
	})

	// The folder chooser: folder names only, never files (the command reads nothing else).
	mux.HandleFunc("GET /api/folders", func(w http.ResponseWriter, r *http.Request) {
		args := []string{"folders"}
		if p := r.URL.Query().Get("path"); p != "" {
			args = append(args, "--", p)
		}
		s.command(w, args...)
	})

	// Search and records.
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		args := append([]string{"memory", "search"}, scope(q.Get("project"), q.Get("all") == "1")...)
		if q.Get("exact") == "1" {
			args = append(args, "--exact")
		}
		for _, name := range []string{"who", "since", "until", "cursor"} {
			if v := q.Get(name); v != "" {
				args = append(args, flag(name, v))
			}
		}
		if v := q.Get("limit"); v != "" {
			if _, err := strconv.Atoi(v); err != nil {
				fail(w, http.StatusBadRequest, "usage", "limit must be a number")
				return
			}
			args = append(args, flag("limit", v))
		}
		s.command(w, append(args, "--", q.Get("q"))...)
	})
	mux.HandleFunc("GET /api/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s.command(w, append(append([]string{"memory", "get"}, scope(q.Get("project"), q.Get("all") == "1")...), "--", r.PathValue("id"))...)
	})
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		args := append([]string{"session", "list"}, scope(q.Get("project"), q.Get("all") == "1")...)
		if v := q.Get("limit"); v != "" {
			if _, err := strconv.Atoi(v); err != nil {
				fail(w, http.StatusBadRequest, "usage", "limit must be a number")
				return
			}
			args = append(args, flag("limit", v))
		}
		if v := q.Get("offset"); v != "" {
			if _, err := strconv.Atoi(v); err != nil {
				fail(w, http.StatusBadRequest, "usage", "offset must be a number")
				return
			}
			args = append(args, flag("offset", v))
		}
		s.command(w, args...)
	})
	mux.HandleFunc("GET /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s.command(w, append(append([]string{"session", "show"}, scope(q.Get("project"), q.Get("all") == "1")...), "--", r.PathValue("id"))...)
	})
	mux.HandleFunc("GET /api/recap", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		day := q.Get("day")
		if day == "" {
			day = "today"
		}
		s.command(w, append(append([]string{"recap"}, scope(q.Get("project"), q.Get("all") == "1")...), "--", day)...)
	})
	mux.HandleFunc("GET /api/resume", func(w http.ResponseWriter, r *http.Request) {
		s.command(w, "resume", flag("project", r.URL.Query().Get("project")))
	})

	// Instructions. Without "confirm": true the command only previews, and the answer is 409
	// with the exact text that would be stored.
	mux.HandleFunc("GET /api/instructions", func(w http.ResponseWriter, r *http.Request) {
		args := []string{"instruction", "list"}
		if r.URL.Query().Get("all") == "1" {
			args = append(args, "--all")
		}
		s.command(w, args...)
	})
	mux.HandleFunc("GET /api/suggestions", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s.command(w, append([]string{"suggest"}, scope(q.Get("project"), q.Get("all") == "1")...)...)
	})
	mux.HandleFunc("GET /api/instructions/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.command(w, "instruction", "show", "--", r.PathValue("id"))
	})
	mux.HandleFunc("POST /api/instructions", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text    string `json:"text"`
			Scope   string `json:"scope"`
			Project string `json:"project"`
			Confirm bool   `json:"confirm"`
		}
		if !decode(w, r, &in) {
			return
		}
		args := []string{"remember"}
		if in.Scope != "" {
			args = append(args, flag("scope", in.Scope))
		}
		if in.Project != "" {
			args = append(args, flag("project", in.Project))
		}
		if in.Confirm {
			args = append(args, "--yes")
		}
		s.command(w, append(args, "--", in.Text)...)
	})
	mux.HandleFunc("POST /api/instructions/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text    string `json:"text"`
			Reason  string `json:"reason"`
			Confirm bool   `json:"confirm"`
		}
		if !decode(w, r, &in) {
			return
		}
		action := r.PathValue("action")
		if action != "supersede" && action != "archive" {
			fail(w, http.StatusNotFound, "not_found", "no such action")
			return
		}
		args := []string{"instruction", action}
		if in.Reason != "" {
			args = append(args, flag("reason", in.Reason))
		}
		if in.Confirm {
			args = append(args, "--yes")
		}
		args = append(args, "--", r.PathValue("id"))
		if action == "supersede" {
			args = append(args, in.Text)
		}
		s.command(w, args...)
	})
	mux.HandleFunc("POST /api/checkpoints", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Note    string `json:"note"`
			Project string `json:"project"`
		}
		if !decode(w, r, &in) {
			return
		}
		s.command(w, "checkpoint", flag("project", in.Project), "--", in.Note)
	})

	// Optional session summaries. Turning them on here is the owner's confirmation: the page
	// says what it means next to the choice.
	mux.HandleFunc("GET /api/accounts", func(w http.ResponseWriter, r *http.Request) { s.command(w, "account", "list") })
	mux.HandleFunc("POST /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		s.command(w, "account", "use", "--", in.Name)
	})
	// Whether the plugin is on in each Claude Code account, and turning it on or off there.
	// The button on the page is the owner's request; the change is made by Claude Code's own
	// plugin commands, never by writing its settings.
	mux.HandleFunc("GET /api/accounts/plugin", func(w http.ResponseWriter, r *http.Request) { s.command(w, "account", "plugins") })
	mux.HandleFunc("POST /api/accounts/plugin", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
			On   bool   `json:"on"`
		}
		if !decode(w, r, &in) {
			return
		}
		verb := "disable"
		if in.On {
			verb = "enable"
		}
		s.command(w, "account", verb, "--", in.Name)
	})
	// Researched facts. Listing reads only what is stored and the project's lock files. "check"
	// fetches the fact's source page again; it is the owner's click and is refused while
	// fetching is turned off.
	// Whether SessionKeep may fetch pages to check a fact. Turning it on is the owner's consent.
	mux.HandleFunc("GET /api/research/fetching", func(w http.ResponseWriter, r *http.Request) { s.command(w, "research", "status") })
	mux.HandleFunc("POST /api/research/fetching", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			On bool `json:"on"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.On {
			s.command(w, "research", "on", "--yes")
			return
		}
		s.command(w, "research", "off")
	})
	mux.HandleFunc("GET /api/research", func(w http.ResponseWriter, r *http.Request) {
		args := []string{"research", "list", flag("project", r.URL.Query().Get("project")), flag("limit", "200")}
		if r.URL.Query().Get("all") == "1" {
			args = append(args, "--all")
		}
		s.command(w, args...)
	})
	mux.HandleFunc("POST /api/research/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Project string `json:"project"`
			Reason  string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		args := []string{"research", r.PathValue("action"), flag("project", in.Project)}
		switch r.PathValue("action") {
		case "check":
		case "withdraw":
			if in.Reason != "" {
				args = append(args, flag("reason", in.Reason))
			}
		default:
			http.NotFound(w, r)
			return
		}
		s.command(w, append(args, "--", r.PathValue("id"))...)
	})
	mux.HandleFunc("GET /api/summaries", func(w http.ResponseWriter, r *http.Request) { s.command(w, "summarize", "status") })
	mux.HandleFunc("POST /api/summaries", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Enabled bool   `json:"enabled"`
			Model   string `json:"model"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !in.Enabled {
			s.command(w, "summarize", "off")
			return
		}
		args := []string{"summarize", "on", "--yes"}
		if in.Model != "" {
			args = append(args, flag("model", in.Model))
		}
		s.command(w, args...)
	})

	// Earlier Claude Code sessions. Without "confirm": true the answer is 409 with what would
	// be imported; nothing is read from anywhere but Claude Code's own folder on this computer.
	mux.HandleFunc("POST /api/history/import", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Project string `json:"project"`
			Confirm bool   `json:"confirm"`
		}
		if !decode(w, r, &in) {
			return
		}
		args := []string{"history", "import"}
		if in.Project != "" {
			args = append(args, flag("project", in.Project))
		}
		if in.Confirm {
			args = append(args, "--yes")
		}
		s.command(w, args...)
	})

	// Sharing a project through a storage bucket. The keys and a project's passphrase are typed
	// into the page once (ADR-0010); they are handed to the command on its standard input, never
	// on a command line, and no endpoint returns a key. A project's passphrase is returned only by
	// /api/team/passphrase with "confirm": true (D131). Without "confirm": true, sync, push, pull,
	// disconnect and passphrase answer 409 with what would happen.
	mux.HandleFunc("GET /api/team", func(w http.ResponseWriter, r *http.Request) {
		s.command(w, "team", "status", flag("project", r.URL.Query().Get("project")))
	})
	mux.HandleFunc("GET /api/team/overview", func(w http.ResponseWriter, r *http.Request) { s.command(w, "team", "list") })
	mux.HandleFunc("POST /api/team/storage", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Storage   string `json:"storage"` // personal (when empty) or shared
			Endpoint  string `json:"endpoint"`
			Bucket    string `json:"bucket"`
			From      string `json:"from"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
			Forget    bool   `json:"forget"`
		}
		if !decode(w, r, &in) {
			return
		}
		slot := in.Storage
		if slot == "" {
			slot = "personal"
		}
		if in.Forget {
			s.command(w, "team", "storage-forget", flag("storage", slot), "--yes")
			return
		}
		secrets, _ := json.Marshal(map[string]string{"access_key": in.AccessKey, "secret_key": in.SecretKey})
		s.commandIn(w, secrets, "team", "storage", "--secrets-stdin", flag("storage", slot), flag("endpoint", in.Endpoint), flag("bucket", in.Bucket), flag("from", in.From))
	})
	mux.HandleFunc("POST /api/team/connect", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Project    string `json:"project"`
			Name       string `json:"name"`
			Passphrase string `json:"passphrase"`
			// personal (when empty) or shared, or custom with a bucket of the project's own.
			Storage   string `json:"storage"`
			Endpoint  string `json:"endpoint"`
			Bucket    string `json:"bucket"`
			From      string `json:"from"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
		}
		if !decode(w, r, &in) {
			return
		}
		args := []string{"team", "connect", "--secrets-stdin", flag("project", in.Project), flag("name", in.Name)}
		secret := map[string]string{"passphrase": in.Passphrase}
		switch in.Storage {
		case "custom":
			// Without both, the command would fall back to the personal storage.
			if in.Endpoint == "" || in.Bucket == "" {
				fail(w, http.StatusBadRequest, "usage", "a bucket of its own needs the storage address and the bucket name")
				return
			}
			secret["access_key"], secret["secret_key"] = in.AccessKey, in.SecretKey
			args = append(args, flag("endpoint", in.Endpoint), flag("bucket", in.Bucket))
			if in.From != "" {
				args = append(args, flag("from", in.From))
			}
		case "":
			args = append(args, flag("storage", "personal"))
		default:
			args = append(args, flag("storage", in.Storage))
		}
		secrets, _ := json.Marshal(secret)
		s.commandIn(w, secrets, args...)
	})
	mux.HandleFunc("POST /api/team/{action}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Project string `json:"project"`
			Confirm bool   `json:"confirm"`
		}
		if !decode(w, r, &in) {
			return
		}
		a := r.PathValue("action")
		if a != "push" && a != "pull" && a != "sync" && a != "disconnect" && a != "passphrase" {
			fail(w, http.StatusNotFound, "not_found", "no such action")
			return
		}
		args := []string{"team", a, flag("project", in.Project)}
		if in.Confirm {
			args = append(args, "--yes")
		}
		s.command(w, args...)
	})

	// Deletion. Without "confirm": true the answer is 409 with exactly what would be removed.
	mux.HandleFunc("POST /api/forget", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Kind    string `json:"kind"` // record | captured | project
			ID      string `json:"id"`
			Project string `json:"project"`
			Confirm bool   `json:"confirm"`
		}
		if !decode(w, r, &in) {
			return
		}
		var args []string
		switch in.Kind {
		case "record":
			args = []string{"forget", "record", flag("project", in.Project)}
		case "captured":
			args = []string{"forget", "captured", flag("project", in.Project)}
		case "project":
			args = []string{"forget", "project"}
		default:
			fail(w, http.StatusBadRequest, "usage", "kind must be record, captured or project")
			return
		}
		if in.Confirm {
			args = append(args, "--yes")
		}
		switch in.Kind {
		case "record":
			args = append(args, "--", in.ID)
		case "project":
			args = append(args, "--", in.Project)
		}
		s.command(w, args...)
	})

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
}
