// Package scan is the deterministic static scanner (hp-static-scan).
// Pure Go, no model calls. Output is stable and sorted so runs are comparable.
package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Ref points at one file and line.
type Ref struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

// EnvVar is one environment variable read in code.
type EnvVar struct {
	Name       string `json:"name"`
	Documented bool   `json:"documented"`
	Refs       []Ref  `json:"refs"`
}

// Job is one scheduled or recurring job hint.
type Job struct {
	Kind    string   `json:"kind"`
	EnvVars []string `json:"env_vars"`
	Refs    []Ref    `json:"refs"`
}

// Finding is one literal or phrase worth a human look.
type Finding struct {
	Value string `json:"value"`
	Refs  []Ref  `json:"refs"`
}

// Report is the static_scan_report payload.
type Report struct {
	EnvVars             []EnvVar  `json:"env_vars"`
	Jobs                []Job     `json:"jobs"`
	StoragePaths        []Finding `json:"storage_paths"`
	DataStores          []string  `json:"data_stores"`
	BackupScript        string    `json:"backup_script"`
	BackupCovers        []string  `json:"backup_covers"`
	BackupGaps          []string  `json:"backup_gaps"`
	HardcodedHosts      []Finding `json:"hardcoded_hosts"`
	AskPhrases          []Finding `json:"ask_phrases"`
	UndocumentedEnvVars []string  `json:"undocumented_env_vars"`
}

// fileText is one scanned file.
type fileText struct {
	rel   string
	lines []string
}

var (
	envReadRe  = regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]*)|process\.env\[['"]([A-Z][A-Z0-9_]*)['"]\]`)
	envKeyRe   = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)=`)
	storageRe  = regexp.MustCompile(`['"](/data/[^'"]*)['"]`)
	hostRe     = regexp.MustCompile(`https?://[A-Za-z0-9._-]+(:[0-9]+)?`)
	askRe      = regexp.MustCompile(`(?i)\bask ([A-Z][a-z]+)\b`)
	cronRe     = regexp.MustCompile(`cron|setInterval|setTimeout|node-schedule|agenda|bull`)
	todoRe     = regexp.MustCompile(`TODO|FIXME`)
	dbHintRe   = regexp.MustCompile(`DATABASE_URL|from 'pg'|from "pg"|require\(['"]pg['"]\)|new Pool|new Client`)
	backupDBRe = regexp.MustCompile(`pg_dump|mysqldump|sqlite3.*\.dump`)
	backupFSRe = regexp.MustCompile(`uploads|/data/|tar |rsync|cp -a|cp -r`)
)

func skipDir(name string) bool {
	switch name {
	case "node_modules", "dist", ".git", "coverage", "backups", "data":
		return true
	}
	return false
}

func scannable(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ts", ".js", ".mjs", ".cjs", ".md", ".sh", ".json", ".yml", ".yaml":
		return true
	}
	return filepath.Base(name) == ".env.example"
}

// Scan walks root and returns a deterministic report.
func Scan(root string) (Report, error) {
	var files []fileText
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !scannable(d.Name()) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("rel %s: %w", path, err)
		}
		files = append(files, fileText{rel: filepath.ToSlash(rel), lines: strings.Split(string(raw), "\n")})
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })

	documented := map[string]bool{}
	var docText strings.Builder
	for _, f := range files {
		if filepath.Base(f.rel) == ".env.example" {
			for _, line := range f.lines {
				if m := envKeyRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
					documented[m[1]] = true
				}
			}
		}
		if strings.HasSuffix(f.rel, ".md") {
			for _, line := range f.lines {
				docText.WriteString(line)
				docText.WriteString("\n")
			}
		}
	}

	envRefs := map[string][]Ref{}
	envByFile := map[string][]string{}
	jobsByKind := map[string][]Ref{}
	jobEnvDone := map[string]bool{}
	jobEnvs := map[string][]string{}
	storage := map[string][]Ref{}
	hosts := map[string][]Ref{}
	asks := map[string][]Ref{}
	stores := map[string]bool{}
	backupScript := ""
	backupText := ""

	for _, f := range files {
		base := filepath.Base(f.rel)
		if base == "backup.sh" || strings.HasPrefix(base, "backup") && strings.HasSuffix(base, ".sh") {
			backupScript = f.rel
			backupText = strings.Join(f.lines, "\n")
		}
		isCode := strings.HasSuffix(f.rel, ".ts") || strings.HasSuffix(f.rel, ".js") ||
			strings.HasSuffix(f.rel, ".mjs") || strings.HasSuffix(f.rel, ".cjs")
		for i, line := range f.lines {
			ln := i + 1
			if isCode {
				for _, m := range envReadRe.FindAllStringSubmatch(line, -1) {
					name := m[1]
					if name == "" {
						name = m[2]
					}
					envRefs[name] = append(envRefs[name], Ref{f.rel, ln})
					envByFile[f.rel] = append(envByFile[f.rel], name)
				}
				if cronRe.MatchString(line) {
					kind := "recurring"
					if strings.Contains(line, "cron") {
						kind = "cron"
					}
					jobsByKind[kind] = append(jobsByKind[kind], Ref{f.rel, ln})
					if !jobEnvDone[f.rel] {
						jobEnvDone[f.rel] = true
						for _, n := range envByFile[f.rel] {
							if !contains(jobEnvs[kind], n) {
								jobEnvs[kind] = append(jobEnvs[kind], n)
							}
						}
					}
				}
				if m := storageRe.FindStringSubmatch(line); m != nil {
					storage[m[1]] = append(storage[m[1]], Ref{f.rel, ln})
				}
				if dbHintRe.MatchString(line) {
					stores["database"] = true
				}
			}
			for _, m := range hostRe.FindAllString(line, -1) {
				hosts[m] = append(hosts[m], Ref{f.rel, ln})
			}
			if m := askRe.FindStringSubmatch(line); m != nil {
				asks["ask "+m[1]] = append(asks["ask "+m[1]], Ref{f.rel, ln})
			}
			if todoRe.MatchString(line) {
				asks[strings.TrimSpace(line)] = append(asks[strings.TrimSpace(line)], Ref{f.rel, ln})
			}
		}
	}
	if len(storage) > 0 {
		stores["file_storage"] = true
	}

	rep := Report{BackupScript: backupScript}
	for name, refs := range envRefs {
		doc := documented[name] || strings.Contains(docText.String(), name)
		sortRefs(refs)
		rep.EnvVars = append(rep.EnvVars, EnvVar{Name: name, Documented: doc, Refs: refs})
		if !doc {
			rep.UndocumentedEnvVars = append(rep.UndocumentedEnvVars, name)
		}
	}
	sort.Slice(rep.EnvVars, func(i, j int) bool { return rep.EnvVars[i].Name < rep.EnvVars[j].Name })
	sort.Strings(rep.UndocumentedEnvVars)
	for kind, refs := range jobsByKind {
		sortRefs(refs)
		envs := jobEnvs[kind]
		sort.Strings(envs)
		rep.Jobs = append(rep.Jobs, Job{Kind: kind, EnvVars: envs, Refs: refs})
	}
	sort.Slice(rep.Jobs, func(i, j int) bool { return rep.Jobs[i].Kind < rep.Jobs[j].Kind })
	rep.StoragePaths = findings(storage)
	rep.HardcodedHosts = findings(hosts)
	rep.AskPhrases = findings(asks)
	for s := range stores {
		rep.DataStores = append(rep.DataStores, s)
	}
	sort.Strings(rep.DataStores)
	if backupText != "" {
		if backupDBRe.MatchString(backupText) {
			rep.BackupCovers = append(rep.BackupCovers, "database")
		}
		if backupFSRe.MatchString(backupText) {
			rep.BackupCovers = append(rep.BackupCovers, "file_storage")
		}
		sort.Strings(rep.BackupCovers)
	}
	for _, s := range rep.DataStores {
		if !contains(rep.BackupCovers, s) {
			rep.BackupGaps = append(rep.BackupGaps, s)
		}
	}
	sort.Strings(rep.BackupGaps)
	return rep, nil
}

func findings(m map[string][]Ref) []Finding {
	var out []Finding
	for v, refs := range m {
		sortRefs(refs)
		out = append(out, Finding{Value: v, Refs: refs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

func sortRefs(r []Ref) {
	sort.Slice(r, func(i, j int) bool {
		if r[i].Path != r[j].Path {
			return r[i].Path < r[j].Path
		}
		return r[i].Line < r[j].Line
	})
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
