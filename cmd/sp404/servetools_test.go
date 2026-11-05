package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/core"
	"github.com/moahbeatlab/404tui/web"
)

func toolsServer(t *testing.T, key string) (*httptest.Server, *core.Remote) {
	t.Helper()
	dir := t.TempDir()
	samples := filepath.Join(dir, "samples")
	if err := core.WriteDemo(samples); err != nil {
		t.Fatal(err)
	}
	o := options{samples: samples, out: filepath.Join(dir, "out")}
	remote := newToolsApp(o, &sharedUnit{}, nil)
	t.Cleanup(func() { core.Terminal = false })
	srv := &web.Server{Key: key}
	srv.Mount("/tools/api", toolsAPI{remote})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, remote
}

func post(t *testing.T, ts *httptest.Server, key, body string) core.RState {
	t.Helper()
	url := ts.URL + "/tools/api"
	if key != "" {
		url += "?key=" + key
	}
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%s: %d", body, resp.StatusCode)
	}
	var st core.RState
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestTheToolsPageTalksToTheProgram(t *testing.T) {
	ts, _ := toolsServer(t, "")
	st := post(t, ts, "", `{"op":"state"}`)
	if st.Menu != nil || st.Pattern.Slot != 1 {
		t.Fatalf("at first: %+v", st)
	}
	st = post(t, ts, "", `{"op":"open"}`)
	if st.Menu == nil || len(st.Menu.Tabs) < 4 {
		t.Fatalf("the pattern tools have pages: %+v", st.Menu)
	}
	// every page, setting and value is there, as in the terminal
	st = post(t, ts, "", `{"op":"tab","tab":2}`)
	var fill int = -1
	for _, it := range st.Menu.Items {
		if strings.HasPrefix(it.Label, "Fill") {
			fill = it.Index
		}
	}
	if fill < 0 {
		t.Fatalf("Fill is on the Notes page: %+v", st.Menu.Items)
	}
	st = post(t, ts, "", `{"op":"run","i":`+itoa(fill)+`}`)
	if st.Menu == nil || !strings.HasPrefix(strings.ToUpper(st.Menu.Title), "FILL") {
		t.Fatalf("Fill's dialog: %+v", st.Menu)
	}
	var choices int
	for _, it := range st.Menu.Items {
		if it.Kind == "choice" {
			choices++
			if len(it.Vals) < 2 {
				t.Fatalf("a choice lists its values: %+v", it)
			}
		}
	}
	if choices < 3 {
		t.Fatalf("Fill has its settings: %d choices", choices)
	}
	if len(st.Pattern.Tracks) == 0 {
		t.Fatal("the trial is in the pattern")
	}
	if st.Msg == "" {
		t.Fatal("the trial says what it did")
	}
	st = post(t, ts, "", `{"op":"back"}`)
	st = post(t, ts, "", `{"op":"back"}`)
	if st.Menu != nil {
		t.Fatal("closed")
	}
	if len(st.Pattern.Tracks) != 0 {
		t.Fatalf("the trial was thrown away: %+v", st.Pattern.Tracks)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestTheToolsRouteNeedsTheKey(t *testing.T) {
	ts, _ := toolsServer(t, "secret")
	resp, err := http.Post(ts.URL+"/tools/api", "application/json", strings.NewReader(`{"op":"state"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without the key: %d", resp.StatusCode)
	}
	if st := post(t, ts, "secret", `{"op":"state"}`); st.Pattern.Slot != 1 {
		t.Fatalf("with it: %+v", st)
	}
	resp, _ = http.Post(ts.URL+"/tools/api?key=secret", "application/json", strings.NewReader(`{"op":"nothing"}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("an unknown operation: %d", resp.StatusCode)
	}
}
