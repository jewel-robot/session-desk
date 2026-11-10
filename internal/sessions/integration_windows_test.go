package sessions

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsPowerShellCommandSyntax(t *testing.T) {
	for _, source := range []Source{{Kind: "windows", ClaudeDir: `C:\Store\中文 it's`}, {Kind: "wsl", Distro: "Ubuntu-20.04", ClaudeDir: "/home/test/中文 it's"}} {
		cwd := `C:\Work\中文 it's; $(whoami)`
		if source.Kind == "wsl" {
			cwd = "/home/test/中文 it's; $(touch x)"
		}
		plan, err := resumePlan(Session{ID: testID, Provider: "claude", Cwd: cwd}, source)
		if err != nil {
			t.Fatal(err)
		}
		payload := base64.StdEncoding.EncodeToString(utf16LE(plan.Command))
		script := "$text = [Text.Encoding]::Unicode.GetString([Convert]::FromBase64String('" + payload + "')); $errors = $null; $tokens = $null; [System.Management.Automation.Language.Parser]::ParseInput($text, [ref]$tokens, [ref]$errors) | Out-Null; if ($errors.Count) { $errors | Out-String | Write-Output; exit 1 }"
		cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(utf16LE(script)))
		configureCommand(cmd)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("PowerShell rejected generated command: %v %s", err, decodeWSL(output))
		}
	}
}

func TestWindowsResumeUsesApplicationUnderRestrictedPolicy(t *testing.T) {
	for _, shellName := range []string{"powershell.exe", "pwsh.exe"} {
		t.Run(shellName, func(t *testing.T) {
			shell, err := exec.LookPath(shellName)
			if err != nil {
				t.Skipf("%s unavailable: %v", shellName, err)
			}
			root := t.TempDir()
			bin := filepath.Join(root, "cli bin")
			cwd := filepath.Join(root, "project it's; $(whoami)")
			store := filepath.Join(root, "store it's; $(whoami)")
			for _, dir := range []string{bin, cwd, store} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			source := Source{Kind: "windows", ClaudeDir: store, CodexDir: store, OpenCodeDir: store, AgyDir: store}
			for _, agent := range []struct {
				name, id, arguments, variable, data string
			}{
				{"claude", testID, "--resume " + testID, "CLAUDE_CONFIG_DIR", store},
				{"codex", testID, "resume " + testID, "CODEX_HOME", store},
				{"opencode", "ses_test", "--session ses_test", "OPENCODE_DB", filepath.Join(store, "opencode.db")},
				{"agy", testID, "--conversation " + testID, "AGY_HOME", store},
			} {
				t.Run(agent.name, func(t *testing.T) {
					writeFixture(t, filepath.Join(bin, agent.name+".cmd"), "@echo off\r\necho args=%*\r\necho cwd=%CD%\r\necho data=%"+agent.variable+"%\r\necho xdg=%XDG_DATA_HOME%\r\n")
					writeFixture(t, filepath.Join(bin, agent.name+".ps1"), "throw 'PowerShell shim must not run'\n")
					// npm also installs an extensionless shim, which Get-Command can return alongside .cmd.
					writeFixture(t, filepath.Join(bin, agent.name), "extensionless shim must not run\n")
					plan, err := resumePlan(Session{ID: agent.id, Provider: agent.name, Cwd: cwd}, source)
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					script := "$ErrorActionPreference = 'Stop'; " + plan.Command
					cmd := exec.CommandContext(ctx, shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Restricted", "-EncodedCommand", base64.StdEncoding.EncodeToString(utf16LE(script)))
					configureCommand(cmd)
					cmd.Env = append(os.Environ(), "PATH="+bin)
					output, err := cmd.CombinedOutput()
					text := decodeWSL(output)
					if err != nil {
						t.Fatalf("resume failed under Restricted policy: %v %s", err, text)
					}
					for _, want := range []string{"args=" + agent.arguments, "cwd=" + cwd, "data=" + agent.data} {
						if !strings.Contains(text, want) {
							t.Fatalf("missing %q in CLI output: %s", want, text)
						}
					}
					if agent.name == "opencode" && !strings.Contains(text, "xdg="+filepath.Dir(store)) {
						t.Fatalf("XDG_DATA_HOME did not reach OpenCode: %s", text)
					}
				})
			}
		})
	}
}

func TestWindowsWSLDiscovery(t *testing.T) {
	sources, err := discoverSources(context.Background(), "", nil)
	if len(sources) == 0 || sources[0].Kind != "windows" || sources[0].Home == "" {
		t.Fatalf("Windows source missing: %+v", sources)
	}
	if err != nil {
		t.Logf("WSL discovery returned: %v", err)
	}
	for _, source := range sources {
		if source.Kind != "wsl" || !source.Running || source.Error != "" {
			continue
		}
		if !strings.HasPrefix(source.Home, "/") {
			t.Fatalf("invalid Linux home: %+v", source)
		}
		stat, err := os.Stat(source.filePath(source.Home))
		if err != nil || !stat.IsDir() {
			t.Fatalf("UNC access failed for %s: %v", source.Name, err)
		}
		t.Logf("WSL discovery and UNC access verified: %s", source.Name)
	}
}

func TestWindowsWSLConfiguredStoreReachesCLI(t *testing.T) {
	sources, err := discoverSources(context.Background(), "", nil)
	if err != nil {
		t.Skipf("WSL unavailable: %v", err)
	}
	for _, source := range sources {
		if source.Kind != "wsl" || !source.Running || source.Error != "" {
			continue
		}
		root := "/tmp/session desk/中文 it's; $(whoami)"
		source.CodexDir = root
		plan, err := resumePlan(Session{ID: testID, Provider: "codex", Cwd: source.Home}, source)
		if err != nil {
			t.Fatal(err)
		}
		// Inspect the configured variable through the real shell without starting a CLI.
		command := strings.Replace(plan.Command, "codex resume "+testID, "printenv CODEX_HOME", 1)
		output, err := runOutput(context.Background(), "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(utf16LE(command)))
		if err != nil || !strings.Contains(decodeWSL(output), root) {
			t.Fatalf("configured store did not reach WSL shell: %v %s", err, decodeWSL(output))
		}
		return
	}
	t.Skip("no running WSL distribution")
}

func TestWindowsWSLWorkerRoundTrip(t *testing.T) {
	ctx := context.Background()
	sources, err := discoverSources(ctx, "", nil)
	if err != nil {
		t.Skipf("WSL unavailable: %v", err)
	}
	var source Source
	for _, item := range sources {
		if item.Kind == "wsl" && item.Running && item.Error == "" {
			source = item
			break
		}
	}
	if source.ID == "" {
		t.Skip("no running WSL distribution")
	}
	if _, err := workerExecutable(); err != nil {
		t.Skipf("packaged worker required: %v", err)
	}
	data, err := runOutput(ctx, "wsl.exe", "--distribution", source.Distro, "--exec", "mktemp", "-d", "/tmp/session-desk-worker-test-XXXXXX")
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(string(data))
	if !strings.HasPrefix(root, "/tmp/session-desk-worker-test-") || strings.ContainsAny(root, "\r\n") {
		t.Fatal("invalid temp directory")
	}
	defer runOutput(ctx, "wsl.exe", "--distribution", source.Distro, "--exec", "rm", "-rf", "--", root)
	script := `import sqlite3,os,sys,json,time
root=sys.argv[1]
os.makedirs(root+'/opencode')
db=sqlite3.connect(root+'/opencode/opencode.db')
db.executescript('CREATE TABLE session_v2 (id TEXT PRIMARY KEY,title TEXT,directory TEXT,parent_id TEXT,time_created INTEGER,time_updated INTEGER); CREATE TABLE session_message (id TEXT PRIMARY KEY,session_id TEXT REFERENCES session_v2(id) ON DELETE CASCADE,seq INTEGER,type TEXT,data TEXT);')
at=int((time.time()-3600)*1000)
db.execute('INSERT INTO session_v2 VALUES (?,?,?,?,?,?)',('ses_worker','WSL worker fixture',root,None,at,at))
db.execute('INSERT INTO session_message VALUES (?,?,?,?,?)',('msg_worker','ses_worker',0,'user',json.dumps({'text':'worker round trip'})))
db.commit()
db.close()`
	if _, err := runOutput(ctx, "wsl.exe", "--distribution", source.Distro, "--exec", "python3", "-c", script, root); err != nil {
		t.Fatal(err)
	}
	source.Home = root
	source.ClaudeDir = root + "/.claude"
	source.CodexDir = root + "/.codex"
	source.OpenCodeDir = root + "/opencode"
	source.AgyDir = root + "/agy"
	service, err := New(Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json")})
	if err != nil {
		t.Fatal(err)
	}
	service.fixed = []Source{source}
	measuredAt := time.Now()
	mark := func(stage string) {
		t.Logf("latency %s: %s", stage, time.Since(measuredAt).Round(time.Millisecond))
		measuredAt = time.Now()
	}
	view, err := service.Refresh(ctx)
	if err != nil || len(view.Warnings) != 0 || len(view.Sessions) != 1 {
		t.Fatalf("worker list: %+v %v", view, err)
	}
	mark("initial_refresh")
	key := view.Sessions[0].Key
	page, err := service.Transcript(ctx, key, 0)
	if err != nil || len(page.Parts) != 1 || page.Parts[0].Text != "worker round trip" {
		t.Fatalf("worker transcript: %+v %v", page, err)
	}
	mark("transcript")
	plan, err := service.PrepareDelete(ctx, []string{key})
	if err != nil {
		t.Fatal(err)
	}
	mark("preview")
	result := service.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Failed) > 0 || len(result.Done) != 1 {
		t.Fatalf("worker delete: %+v", result)
	}
	mark("delete")
	view = service.View()
	if len(view.Sessions) != 0 {
		t.Fatalf("worker deleted list: %+v %v", view, err)
	}
	mark("state_after_delete")
	entries, err := service.Trash(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("worker trash: %+v %v", entries, err)
	}
	mark("trash")
	if err := service.Restore(ctx, entries[0].Key); err != nil {
		t.Fatal(err)
	}
	mark("restore")
	view = service.View()
	if len(view.Sessions) != 1 {
		t.Fatalf("worker restored list: %+v %v", view, err)
	}
	mark("state_after_restore")
	t.Log("Windows-to-WSL list, transcript, delete and restore verified on isolated SQLite data")
	batchScript := `import sqlite3,os,sys,json,time
root=sys.argv[1]
db=sqlite3.connect(root+'/opencode/opencode.db')
at=int((time.time()-3600)*1000)
for n in range(2):
 db.execute('INSERT INTO session_v2 VALUES (?,?,?,?,?,?)',('ses_batch'+str(n),'Batch fixture',root,None,at,at))
db.commit()
db.close()
os.makedirs(root+'/.claude/projects/test')
p=root+'/.claude/projects/test/11111111-2222-4333-8444-555555555555.jsonl'
with open(p,'w') as f:
 f.write(json.dumps({'type':'user','sessionId':'11111111-2222-4333-8444-555555555555','cwd':root,'timestamp':'2020-01-01T00:00:00Z','message':{'content':'batch Claude fixture'}})+'\n')
os.utime(p,(time.time()-3600,time.time()-3600))`
	if _, err := runOutput(ctx, "wsl.exe", "--distribution", source.Distro, "--exec", "python3", "-c", batchScript, root); err != nil {
		t.Fatal(err)
	}
	selected := []string{source.ID + "|opencode|ses_batch0", source.ID + "|opencode|ses_batch1", source.ID + "|claude|" + testID}
	plan, err = service.PrepareDelete(ctx, selected)
	if err != nil {
		t.Fatal(err)
	}
	result = service.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Done) != 3 || len(result.Failed) != 0 || len(service.View().Sessions) != 1 {
		t.Fatalf("batch delete: %+v %+v", result, service.View())
	}
	entries, err = service.Trash(ctx)
	if err != nil || len(entries) != 2 {
		t.Fatalf("batch trash: %+v %v", entries, err)
	}
	entryKeys := []string{entries[0].Key, entries[1].Key}
	result = service.RestoreMany(ctx, entryKeys)
	if len(result.Done) != 2 || len(result.Failed) != 0 || len(service.View().Sessions) != 4 {
		t.Fatalf("batch restore: %+v %+v", result, service.View())
	}
	plan, err = service.PrepareDelete(ctx, selected)
	if err != nil {
		t.Fatal(err)
	}
	result = service.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Failed) != 0 {
		t.Fatal(result)
	}
	entries, err = service.Trash(ctx)
	if err != nil || len(entries) != 2 {
		t.Fatalf("batch trash: %+v %v", entries, err)
	}
	result = service.PurgeMany(ctx, []string{entries[0].Key, entries[1].Key})
	if len(result.Done) != 2 || len(result.Failed) != 0 {
		t.Fatalf("batch purge: %+v", result)
	}
	entries, err = service.Trash(ctx)
	if err != nil || len(entries) != 0 {
		t.Fatalf("purged trash: %+v %v", entries, err)
	}
	t.Log("Windows-to-WSL mixed-provider batch delete, restore and purge verified")
}
