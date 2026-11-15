package test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestJSPlugins runs one edit through each JavaScript plugin in a fake host:
// OpenCode, Pi, Oh My Pi, and Amp. The plugin must record the edit in the
// session's stage with the host's editor name, add the hook's messages to
// the tool result, and leave the result alone when it cannot run the hook.
func TestJSPlugins(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH; `mise install` installs it")
	}
	driver, _ := filepath.Abs(filepath.Join("testdata", "plugins", "drive.mjs"))
	for _, tc := range []struct{ host, plugin, editor string }{
		{"opencode", "../plugins/opencode/git-multistage.js", "opencode"},
		{"pi", "../plugins/pi/git-multistage.js", "pi"},
		{"omp", "../plugins/pi/git-multistage.js", "omp"},
		{"amp", "../plugins/amp/git-multistage.js", "amp"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			plugin, _ := filepath.Abs(tc.plugin)
			drive := func(r *repo, env []string) string {
				cmd := exec.CommandContext(context.Background(), node, driver, tc.host, plugin, r.dir, "f.txt", "one\nTWO\nthree\n", "s1")
				cmd.Env = env
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("%s driver: %v\n%s", tc.host, err, out)
				}
				var res struct{ Text string }
				if err := json.Unmarshal(out, &res); err != nil {
					t.Fatalf("%s driver printed %q: %v", tc.host, out, err)
				}
				return res.Text
			}
			r := newRepo(t)
			r.write("f.txt", "one\ntwo\nthree\n")
			r.git("add", "f.txt")
			r.git("commit", "-q", "-m", "f")
			stage := tc.editor + "-s1"
			text := drive(r, nil)
			if !strings.HasPrefix(text, "Edit applied.\n\n") || !strings.Contains(text, stage) ||
				!strings.Contains(text, "Other agents may edit files") {
				t.Fatalf("the tool result does not end with the session start notice and the message about stage %s:\n%s", stage, text)
			}
			r.mustMS("", "--stage", stage, "add", "--edits")
			r.mustMS("", "--stage", stage, "commit", "-m", "edit")
			if got := r.git("show", "HEAD:f.txt"); got != "one\nTWO\nthree\n" {
				t.Fatalf("the commit holds %q", got)
			}

			// Without git on PATH the plugin cannot run the hook; the
			// tool result stays as it was.
			r = newRepo(t)
			r.write("f.txt", "one\ntwo\nthree\n")
			var noPath []string
			for _, kv := range os.Environ() {
				if name, _, _ := strings.Cut(kv, "="); !strings.EqualFold(name, "PATH") {
					noPath = append(noPath, kv)
				}
			}
			if text := drive(r, append(noPath, "PATH=")); text != "Edit applied." {
				t.Fatalf("without git multistage, the tool result is %q", text)
			}
		})
	}
}

// TestHermesPlugin runs one write_file call through the Hermes Agent plugin
// in a fake Hermes context. The plugin must register the skill with its
// description, record the edit in stage hermes-<session>, and give the
// first-edit message to the next model call.
func TestHermesPlugin(t *testing.T) {
	t.Parallel()
	python, err := exec.LookPath("python3")
	if err != nil {
		if python, err = exec.LookPath("python"); err != nil {
			t.Skip("python is not on PATH")
		}
	}
	driver, _ := filepath.Abs(filepath.Join("testdata", "plugins", "drive_hermes.py"))
	plugin, _ := filepath.Abs(filepath.Join("..", ".hermes-plugin", "__init__.py"))
	r := newRepo(t)
	out, err := exec.CommandContext(context.Background(), python, driver, plugin, r.dir, "new.txt", "new file\n", "h1").Output()
	if err != nil {
		t.Fatalf("hermes driver: %v\n%s", err, out)
	}
	var res struct {
		Skills []struct {
			Name, Type, Description string
		}
		Context string
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("hermes driver printed %q: %v", out, err)
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "git-multistage" || !strings.HasSuffix(res.Skills[0].Type, "Path") ||
		res.Skills[0].Description == "" {
		t.Fatalf("the plugin registered %+v; Hermes needs the skill's Path and description", res.Skills)
	}
	if !strings.Contains(res.Context, "hermes-h1") {
		t.Fatalf("pre_llm_call gives no message about stage hermes-h1: %q", res.Context)
	}
	r.mustMS("", "--stage", "hermes-h1", "add", "--edits")
	r.mustMS("", "--stage", "hermes-h1", "commit", "-m", "new")
	if got := r.git("show", "HEAD:new.txt"); got != "new file\n" {
		t.Fatalf("the commit holds %q", got)
	}
}
