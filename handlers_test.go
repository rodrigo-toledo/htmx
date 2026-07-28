package main

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	loadTemplates()
	os.Exit(m.Run())
}

// doReq routes a request through the real chi router (so URL params work)
// and captures the response.
func doReq(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rr := httptest.NewRecorder()
	newRouter().ServeHTTP(rr, req)
	return rr
}

func TestIndexPage(t *testing.T) {
	store.Reset()
	rr := doReq(t, "GET", "/", "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{`id="col-todo"`, `id="col-doing"`, `id="col-done"`, `hx-sse:connect="/events"`} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestCreateCardReturnsTodoColumn(t *testing.T) {
	store.Reset()
	rr := doReq(t, "POST", "/cards", "title=Brand+new")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Brand new") {
		t.Error("response missing the new card")
	}
	if !strings.Contains(body, `id="col-todo"`) {
		t.Error("response should be the todo column")
	}
	if !strings.Contains(body, "(3)") {
		t.Error("todo count should be (3)")
	}
}

func TestMoveCardResponseShape(t *testing.T) {
	store.Reset()
	rr := doReq(t, "POST", "/cards/1/move?dir=right", "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	// main swap = source column; destination arrives out-of-band
	if !strings.Contains(body, `id="col-todo"`) {
		t.Error("missing source column")
	}
	if !strings.Contains(body, `id="col-doing" hx-swap-oob="outerHTML"`) {
		t.Error("missing OOB destination column")
	}
	if !strings.Contains(body, `id="stats"`) {
		t.Error("missing stats fragment")
	}
	// card 1 must now be rendered inside the doing column
	doingIdx := strings.Index(body, `id="col-doing"`)
	cardIdx := strings.Index(body, `id="card-1"`)
	if doingIdx == -1 || cardIdx == -1 || cardIdx < doingIdx {
		t.Error("card-1 should appear within the doing column")
	}
}

func TestMoveBoundaryReturns204(t *testing.T) {
	store.Reset()
	rr := doReq(t, "POST", "/cards/1/move?dir=left", "") // card 1 is in todo
	if rr.Code != 204 {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("204 should have empty body, got %q", rr.Body.String())
	}
}

func TestDropCardResponse(t *testing.T) {
	store.Reset()
	rr := doReq(t, "POST", "/cards/2/drop", "col=doing&index=0")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	doingIdx := strings.Index(body, `id="col-doing"`)
	c2 := strings.Index(body, `id="card-2"`)
	c3 := strings.Index(body, `id="card-3"`)
	if doingIdx == -1 || c2 == -1 || c3 == -1 {
		t.Fatalf("missing expected fragments")
	}
	if !(doingIdx < c2 && c2 < c3) {
		t.Errorf("card-2 should precede card-3 in doing (doing@%d c2@%d c3@%d)", doingIdx, c2, c3)
	}
}

func TestDeleteCardReturnsColumnWithoutCard(t *testing.T) {
	store.Reset()
	rr := doReq(t, "DELETE", "/cards/1", "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, `id="card-1"`) {
		t.Error("deleted card should not be in the response")
	}
	if !strings.Contains(body, "(1)") {
		t.Error("todo count should drop to (1)")
	}
}

func TestUpdateValidationReturns422(t *testing.T) {
	store.Reset()
	rr := doReq(t, "PATCH", "/cards/1", "title=")
	if rr.Code != 422 {
		t.Fatalf("status = %d, want 422", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "error") {
		t.Error("422 response should re-render the form with an error")
	}
}

func TestUpdateCardReturnsView(t *testing.T) {
	store.Reset()
	rr := doReq(t, "PATCH", "/cards/1", "title=Renamed+title")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Renamed title") {
		t.Error("response should show the new title")
	}
}

func TestGetColumnNotFound(t *testing.T) {
	store.Reset()
	if rr := doReq(t, "GET", "/columns/nope", ""); rr.Code != 404 {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestSSEStreamAndBroadcast pins the wire format we depend on: headers flush
// immediately, and a mutation produces an id:/data: frame on the stream.
func TestSSEStreamAndBroadcast(t *testing.T) {
	store.Reset()
	srv := httptest.NewServer(newRouter())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	first, _ := reader.ReadString('\n')
	if !strings.HasPrefix(first, ": connected") {
		t.Fatalf("first line = %q, want ': connected'", first)
	}

	time.Sleep(200 * time.Millisecond) // let the server finish subscribing
	if _, err := http.PostForm(srv.URL+"/cards", url.Values{"title": {"SSE probe"}}); err != nil {
		t.Fatal(err)
	}

	var gotID, gotData bool
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !(gotID && gotData) {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "id: ") {
			gotID = true
		}
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "feed-item") {
			gotData = true
		}
	}
	if !gotID || !gotData {
		t.Fatalf("expected an id:/data: frame with a feed item (gotID=%v gotData=%v)", gotID, gotData)
	}
}
