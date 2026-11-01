// Package byoa implements Bring Your Own App onboarding: a declarative
// application profile, validation, and a Docker Compose sandbox runner.
// Profiles contain no secret values and no arbitrary shell commands; the
// controller-side runner is the only component that issues verdicts.
package byoa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const ProfileFile = "handoffproof.json"

var (
	slugRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)
	envRe        = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	sqlCount     = regexp.MustCompile(`(?is)^\s*select\s+count\s*\(\s*(\*|[a-z0-9_."]+)\s*\)\s+from\s+[a-z0-9_." ]+$`)
	headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
)

type Service struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	HealthPath string `json:"health_path,omitempty"`
}

type EnvDecl struct {
	Name     string `json:"name"`
	Required bool   `json:"required,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
}

type Credential struct {
	Name   string `json:"name"`
	EnvVar string `json:"env_var"`
}

type Step struct {
	Name           string            `json:"name"`
	Type           string            `json:"type"` // http, sql_count, log_contains, file_exists
	Service        string            `json:"service,omitempty"`
	Method         string            `json:"method,omitempty"`
	Path           string            `json:"path,omitempty"`
	Body           map[string]string `json:"body,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	ExpectedStatus int               `json:"expected_status,omitempty"`
	Contains       string            `json:"contains,omitempty"`
	Query          string            `json:"query,omitempty"`
	MinCount       int               `json:"min_count,omitempty"`
	FilePath       string            `json:"file_path,omitempty"`
	PollSeconds    int               `json:"poll_seconds,omitempty"`
}

type Workflow struct {
	Name      string   `json:"name"`
	DependsOn []string `json:"depends_on,omitempty"`
	Steps     []Step   `json:"steps"`
}

type DataStore struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // postgres or filesystem
	Service  string `json:"service,omitempty"`
	User     string `json:"user,omitempty"`
	Database string `json:"database,omitempty"`
	Path     string `json:"path,omitempty"`
}

type Backup struct {
	Service     string   `json:"service"`
	DumpArgv    []string `json:"dump_argv"`
	RestoreArgv []string `json:"restore_argv"`
	Stores      []string `json:"stores"`
}

type Profile struct {
	Version      int          `json:"version"`
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	ComposeFile  string       `json:"compose_file"`
	AppService   string       `json:"app_service"`
	ProbeRuntime string       `json:"probe_runtime"` // node, curl, wget, python3
	Services     []Service    `json:"services"`
	Env          []EnvDecl    `json:"env,omitempty"`
	Credentials  []Credential `json:"credentials,omitempty"`
	Workflows    []Workflow   `json:"workflows"`
	DataStores   []DataStore  `json:"data_stores,omitempty"`
	Backup       *Backup      `json:"backup,omitempty"`
}

func Load(dir string) (*Profile, error) {
	raw, err := os.ReadFile(filepath.Join(dir, ProfileFile))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p Profile
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ProfileFile, err)
	}
	if err := p.Validate(dir); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *Profile) Service(name string) (Service, bool) {
	for _, s := range p.Services {
		if s.Name == name {
			return s, true
		}
	}
	return Service{}, false
}

func (p *Profile) Credential(name string) (Credential, bool) {
	for _, c := range p.Credentials {
		if c.Name == name {
			return c, true
		}
	}
	return Credential{}, false
}

func (p *Profile) Workflow(name string) (Workflow, bool) {
	for _, w := range p.Workflows {
		if w.Name == name {
			return w, true
		}
	}
	return Workflow{}, false
}

func (p *Profile) Validate(dir string) error {
	var probs []string
	add := func(format string, args ...any) { probs = append(probs, fmt.Sprintf(format, args...)) }
	if p.Version != 1 {
		add("version must be 1")
	}
	if !slugRe.MatchString(p.ID) {
		add("id must be a lowercase slug")
	}
	if strings.TrimSpace(p.Name) == "" {
		add("name is required")
	}
	if p.ComposeFile == "" || filepath.IsAbs(p.ComposeFile) || strings.Contains(p.ComposeFile, "..") {
		add("compose_file must be a relative path inside the application")
	} else if dir != "" {
		if _, err := os.Stat(filepath.Join(dir, p.ComposeFile)); err != nil {
			add("compose file %q not found", p.ComposeFile)
		}
	}
	switch p.ProbeRuntime {
	case "node", "curl", "wget", "python3":
	default:
		add("probe_runtime must be node, curl, wget, or python3")
	}
	services := map[string]bool{}
	ports := map[int]bool{}
	for _, s := range p.Services {
		if s.Name == "" || services[s.Name] {
			add("services must have unique names")
		}
		services[s.Name] = true
		if s.Port < 1 || s.Port > 65535 || ports[s.Port] {
			add("service %q has an invalid or duplicate port", s.Name)
		}
		ports[s.Port] = true
	}
	if !services[p.AppService] {
		add("app_service %q is not listed in services", p.AppService)
	}
	envs := map[string]bool{}
	for _, e := range p.Env {
		if !envRe.MatchString(e.Name) || envs[e.Name] {
			add("env %q is invalid or duplicated", e.Name)
		}
		envs[e.Name] = true
	}
	for _, c := range p.Credentials {
		if c.Name == "" || !envs[c.EnvVar] {
			add("credential %q must reference a declared env var", c.Name)
		}
	}
	if len(p.Workflows) == 0 {
		add("at least one workflow is required")
	}
	workflows := map[string]bool{}
	for _, w := range p.Workflows {
		if w.Name == "" || workflows[w.Name] {
			add("workflows must have unique names")
		}
		workflows[w.Name] = true
		if len(w.Steps) == 0 {
			add("workflow %q needs at least one step", w.Name)
		}
		for _, dep := range w.DependsOn {
			if !envs[dep] {
				add("workflow %q depends on undeclared env %q", w.Name, dep)
			}
		}
		for _, st := range w.Steps {
			if st.Name == "" {
				add("workflow %q has an unnamed step", w.Name)
			}
			switch st.Type {
			case "http":
				if !services[st.Service] {
					add("workflow %q step %q references unknown service %q", w.Name, st.Name, st.Service)
				}
				if st.Path == "" || strings.HasPrefix(st.Path, "/http") || strings.HasPrefix(st.Path, "//") || strings.Contains(st.Path, "..") || strings.ContainsAny(st.Path, "\r\n") {
					add("workflow %q step %q has an unsafe path", w.Name, st.Name)
				}
				switch strings.ToUpper(st.Method) {
				case "", "GET", "POST", "PUT", "PATCH", "DELETE":
				default:
					add("workflow %q step %q has an unsupported method", w.Name, st.Name)
				}
				for name, value := range st.Headers {
					if !headerNameRe.MatchString(name) || strings.ContainsAny(value, "\r\n") {
						add("workflow %q step %q has an unsafe header %q", w.Name, st.Name, name)
					}
				}
			case "sql_count":
				if !services[st.Service] || !sqlCount.MatchString(st.Query) || strings.Contains(st.Query, ";") {
					add("workflow %q step %q needs a single SELECT COUNT query on a known service", w.Name, st.Name)
				}
			case "log_contains":
				if !services[st.Service] || st.Contains == "" {
					add("workflow %q step %q needs a service and text to find", w.Name, st.Name)
				}
			case "file_exists":
				if !services[st.Service] || !strings.HasPrefix(st.FilePath, "/") || strings.Contains(st.FilePath, "..") {
					add("workflow %q step %q needs an absolute in-container file path", w.Name, st.Name)
				}
			default:
				add("workflow %q step %q has unsupported type %q", w.Name, st.Name, st.Type)
			}
		}
	}
	stores := map[string]bool{}
	for _, ds := range p.DataStores {
		if ds.ID == "" || stores[ds.ID] {
			add("data stores must have unique ids")
		}
		stores[ds.ID] = true
		if ds.Kind != "postgres" && ds.Kind != "filesystem" {
			add("data store %q kind must be postgres or filesystem", ds.ID)
		}
		if ds.Service != "" && !services[ds.Service] {
			add("data store %q references unknown service %q", ds.ID, ds.Service)
		}
	}
	if p.Backup != nil {
		if !services[p.Backup.Service] || len(p.Backup.DumpArgv) == 0 || len(p.Backup.RestoreArgv) == 0 {
			add("backup needs a known service and dump/restore commands")
		}
		for _, id := range p.Backup.Stores {
			if !stores[id] {
				add("backup covers unknown data store %q", id)
			}
		}
		allowed := map[string]bool{"pg_dump": true, "psql": true, "mysqldump": true, "mysql": true, "sqlite3": true, "tar": true, "cp": true}
		for _, argv := range [][]string{p.Backup.DumpArgv, p.Backup.RestoreArgv} {
			if len(argv) > 0 && !allowed[filepath.Base(argv[0])] {
				add("backup command %q is not allowlisted", argv[0])
			}
			for _, a := range argv {
				if strings.ContainsAny(a, "\r\n") || a == "sh" || a == "bash" {
					add("backup commands must not use a shell")
				}
			}
		}
	}
	if len(probs) > 0 {
		sort.Strings(probs)
		return fmt.Errorf("invalid profile: %s", strings.Join(probs, "; "))
	}
	return nil
}
