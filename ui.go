package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func cmdUI(args []string) error {
	port := 3000
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" && i+1 < len(args) {
			parsed, err := strconv.Atoi(args[i+1])
			if err != nil {
				return err
			}
			port = parsed
			i++
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(capsuleDir))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capsules, err := allCapsules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		views := make([]CapsuleView, 0, len(capsules))
		for _, capsule := range capsules {
			views = append(views, newCapsuleView(capsule))
		}
		if err := uiTemplate.Execute(w, views); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("Capsule UI: http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func newCapsuleView(session Session) CapsuleView {
	view := CapsuleView{
		ID:            session.ID,
		GitSHA:        shortSHA(session.Git.SHA),
		GitBranch:     fallback(session.Git.Branch, "unknown"),
		CommandCount:  len(session.Commands),
		ArtifactCount: len(session.Artifacts),
		StartedAt:     session.StartedAt.Format(time.RFC3339),
		ReplayCommand: fmt.Sprintf("capsule replay %s --rerun", session.ID),
		BundlePath:    bundleLink(session.ID),
		AgentBriefing: agentBriefing(session, false),
		Commands:      session.Commands,
		Artifacts:     session.Artifacts,
	}
	if failed := firstFailedCommand(session); failed != nil {
		view.FailedCommand = failed
		view.FailedLogPath = filepath.ToSlash(filepath.Join("capsules", session.ID, failed.Logs.Combined))
		view.FailedLogPreview = readLogPreview(filepath.Join(capsuleSnapshotDir(session.ID), failed.Logs.Combined))
	}
	return view
}

func bundleLink(id string) string {
	path := filepath.Join(capsuleDir, "bundles", id+".zip")
	if _, err := os.Stat(path); err == nil {
		return filepath.ToSlash(filepath.Join("bundles", id+".zip"))
	}
	return ""
}

func readLogPreview(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	const maxChars = 1400
	text := string(data)
	if len(text) > maxChars {
		text = text[:maxChars] + "\n..."
	}
	return strings.TrimSpace(text)
}

var uiTemplate = template.Must(template.New("ui").Funcs(template.FuncMap{
	"short": shortSHA,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Capsule</title>
  <style>
    :root { color-scheme: light; --ink:#162019; --muted:#667069; --line:#d8ded8; --panel:#f7f8f4; --accent:#126a5a; --bad:#a83232; --ok:#1f7a45; --warnbg:#fff3f0; }
    * { box-sizing: border-box; }
    body { margin:0; font:14px/1.45 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color:var(--ink); background:#fdfdf9; }
    header { padding:28px 32px 20px; border-bottom:1px solid var(--line); background:#fff; }
    h1 { margin:0; font-size:28px; letter-spacing:0; }
    .sub { margin-top:6px; color:var(--muted); }
    main { max-width:1120px; margin:0 auto; padding:24px; }
    .capsule { border:1px solid var(--line); border-radius:8px; background:#fff; margin-bottom:18px; overflow:hidden; }
    .cap-head { display:flex; gap:16px; justify-content:space-between; padding:16px 18px; background:var(--panel); border-bottom:1px solid var(--line); }
    .id { font-family:ui-monospace, SFMono-Regular, Menlo, monospace; font-weight:700; color:var(--accent); }
    .meta { display:flex; flex-wrap:wrap; gap:10px 18px; color:var(--muted); }
    .section { padding:16px 18px; }
    h2 { font-size:15px; margin:0 0 10px; }
    table { width:100%; border-collapse:collapse; }
    th, td { text-align:left; padding:9px 8px; border-top:1px solid var(--line); vertical-align:top; }
    th { color:var(--muted); font-weight:600; font-size:12px; text-transform:uppercase; }
    code { font-family:ui-monospace, SFMono-Regular, Menlo, monospace; font-size:13px; }
    pre, textarea { font:13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
    .ok { color:var(--ok); font-weight:700; }
    .bad { color:var(--bad); font-weight:700; }
    .empty { padding:48px; text-align:center; color:var(--muted); border:1px dashed var(--line); border-radius:8px; background:#fff; }
    .failure { border:1px solid #f2cbc4; background:var(--warnbg); border-radius:8px; padding:14px; margin-bottom:16px; }
    .preview { margin:10px 0 0; padding:12px; background:#fff; border:1px solid var(--line); border-radius:8px; overflow:auto; white-space:pre-wrap; }
    .tools { display:flex; flex-wrap:wrap; gap:10px; margin-top:12px; }
    .btn { border:1px solid var(--line); border-radius:8px; background:#fff; color:var(--ink); padding:8px 10px; cursor:pointer; }
    textarea { width:100%; min-height:220px; border:1px solid var(--line); border-radius:8px; padding:12px; resize:vertical; background:#fbfbf8; color:var(--ink); }
    .section-head { display:flex; justify-content:space-between; align-items:center; gap:12px; margin-bottom:10px; }
    a { color:var(--accent); }
  </style>
</head>
<body>
  <header>
    <h1>Capsule</h1>
    <div class="sub">Replayable execution sessions attached to Git commits.</div>
  </header>
  <main>
    {{if not .}}<div class="empty">No finished Capsules yet. Run <code>capsule start</code>, <code>capsule run</code>, then <code>capsule finish</code>.</div>{{end}}
    {{range .}}
    {{$cap := .}}
    <article class="capsule">
      <div class="cap-head">
        <div>
          <div class="id">{{.ID}}</div>
          <div class="meta">
            <span>{{.GitSHA}}</span>
            <span>{{.GitBranch}}</span>
            <span>{{.CommandCount}} commands</span>
            <span>{{.ArtifactCount}} artifacts</span>
            <span>{{.StartedAt}}</span>
          </div>
        </div>
        <code>{{.ReplayCommand}}</code>
      </div>
      <div class="section">
        {{if .FailedCommand}}
        <div class="failure">
          <div><strong>Failure</strong>: <code>{{.FailedCommand.Command}}</code> exited with <span class="bad">{{.FailedCommand.ExitCode}}</span>.</div>
          <div class="tools">
            <a class="btn" href="/files/{{.FailedLogPath}}">Open combined log</a>
            {{if .BundlePath}}<a class="btn" href="/files/{{.BundlePath}}">Download bundle</a>{{end}}
          </div>
          {{if .FailedLogPreview}}<pre class="preview">{{.FailedLogPreview}}</pre>{{end}}
        </div>
        {{end}}
        <div class="section-head">
          <h2>Agent Briefing</h2>
          <button class="btn" type="button" onclick="navigator.clipboard.writeText(document.getElementById('agent-{{.ID}}').value)">Copy</button>
        </div>
        <textarea id="agent-{{.ID}}" readonly>{{.AgentBriefing}}</textarea>
      </div>
      <div class="section">
        <h2>Execution Timeline</h2>
        <table>
          <thead><tr><th>#</th><th>Command</th><th>Exit</th><th>Duration</th><th>Logs</th></tr></thead>
          <tbody>
            {{range .Commands}}
            <tr>
              <td>{{.Index}}</td>
              <td><code>{{.Command}}</code></td>
              <td>{{if eq .ExitCode 0}}<span class="ok">0</span>{{else}}<span class="bad">{{.ExitCode}}</span>{{end}}</td>
              <td>{{.DurationMS}}ms</td>
              <td><a href="/files/capsules/{{$cap.ID}}/{{.Logs.Combined}}"><code>{{.Logs.Combined}}</code></a></td>
            </tr>
            {{end}}
          </tbody>
        </table>
      </div>
      <div class="section">
        <h2>Artifacts</h2>
        {{if not .Artifacts}}<div class="meta">No artifacts detected.</div>{{end}}
        {{if .Artifacts}}
        <table>
          <thead><tr><th>Kind</th><th>Path</th><th>Size</th></tr></thead>
          <tbody>
            {{range .Artifacts}}<tr><td>{{.Kind}}</td><td><a href="/files/capsules/{{$cap.ID}}/{{.CapsulePath}}"><code>{{.Path}}</code></a></td><td>{{.SizeBytes}} bytes</td></tr>{{end}}
          </tbody>
        </table>
        {{end}}
      </div>
    </article>
    {{end}}
  </main>
</body>
</html>`))
