package web

import (
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
		}
		if !decode(w, r, &in) {
			return
		}
		switch a := r.PathValue("action"); a {
		case "pause", "resume":
			s.command(w, "project", a, "--", r.PathValue("id"))
		case "capture":
			s.command(w, "project", "capture", "--", r.PathValue("id"), in.Level)
		default:
			fail(w, http.StatusNotFound, "not_found", "no such action")
		}
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
