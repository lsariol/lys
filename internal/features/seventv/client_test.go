package seventv_test

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	api "github.com/lsariol/lys/internal/features/seventv"
	"github.com/lsariol/lys/internal/features/seventv/seventvtest"
)

// No GraphQL document Lys sends may delete anything on 7TV: sets, emotes,
// editors or sessions.
func TestNoDeletes(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("list files: %v", err)
	}
	raw := regexp.MustCompile("`[^`]*`")
	deletes := regexp.MustCompile(`(?i)delete([^d]|$)`)
	documents := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, doc := range raw.FindAllString(string(src), -1) {
			if !strings.Contains(doc, "{") {
				continue
			}
			documents++
			if deletes.MatchString(doc) {
				t.Errorf("%s: a GraphQL document deletes: %s", f, doc)
			}
		}
	}
	if documents < 10 {
		t.Errorf("found only %d GraphQL documents; has the check stopped seeing them?", documents)
	}
}

func TestParseEmote(t *testing.T) {
	const id = "01F61B1440000991F7SWQNMVX7"
	for _, tc := range []struct {
		in       string
		id       string
		link, ok bool
	}{
		{"KEKW", "", false, true},
		{"peepo.Happy", "", false, true},
		{id, id, true, true},
		{"60ae958e229664e8667aea38", "60ae958e229664e8667aea38", true, true},
		{"https://7tv.app/emotes/" + id, id, true, true},
		{"7tv.app/emotes/" + id, id, true, true},
		{"https://old.7tv.app/emotes/" + id + "?x=1", id, true, true},
		{"https://cdn.7tv.app/emote/" + id + "/4x.webp", id, true, true},
		{"https://evil.example/emotes/" + id, "", true, false},
		{"https://7tv.app.evil.example/emotes/" + id, "", true, false},
		{"https://7tv.app/users/" + id, "", true, false},
		{"https://7tv.app/emotes/nope", "", true, false},
		{"https://example.com", "", true, false},
	} {
		id, link, ok := api.ParseEmote(tc.in)
		if id != tc.id || link != tc.link || ok != tc.ok {
			t.Errorf("%q: got %q %v %v, want %q %v %v", tc.in, id, link, ok, tc.id, tc.link, tc.ok)
		}
	}
}

func TestPick(t *testing.T) {
	found := []api.Emote{{ID: "1", Name: "kekw"}, {ID: "2", Name: "KEKW"}}
	if e, _ := api.Pick(found, "KEKW"); e.ID != "2" {
		t.Errorf("exact case: got %s", e.ID)
	}
	if e, _ := api.Pick(found, "Kekw"); e.ID != "1" {
		t.Errorf("most used ignoring case: got %s", e.ID)
	}
	if _, ok := api.Pick(found, "KEK"); ok {
		t.Error("a different name was picked")
	}
}

func TestMay(t *testing.T) {
	var none *api.Editor
	for _, tc := range []struct {
		e    *api.Editor
		p    api.Permission
		want api.Denial
	}{
		{none, api.ManageSets, api.NotEditor},
		{&api.Editor{State: api.Pending, ManageSets: true}, api.ManageSets, api.NotYet},
		{&api.Editor{State: "REJECTED", ManageSets: true}, api.ManageSets, api.NotYet},
		{&api.Editor{State: api.Accepted}, api.ManageSets, api.Lacking},
		{&api.Editor{State: api.Accepted, ManageSets: true}, api.ManageSets, api.Allowed},
		{&api.Editor{State: api.Accepted, ManageSets: true}, api.ManageProfile, api.Lacking},
		{&api.Editor{State: api.Accepted, ManageProfile: true}, api.ManageProfile, api.Allowed},
	} {
		if got := tc.e.May(tc.p); got != tc.want {
			t.Errorf("%+v %v: got %v, want %v", tc.e, tc.p, got, tc.want)
		}
	}
	ch := &api.Channel{Editors: []api.Editor{{ID: "a", TwitchID: "1"}}}
	if ch.EditorByTwitch("") != nil || ch.EditorByTwitch("2") != nil || ch.EditorByTwitch("1").ID != "a" {
		t.Error("EditorByTwitch")
	}
}

func jwt(exp time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x","exp":` + strconv.FormatInt(exp.Unix(), 10) + `}`))
	return "eyJhbGciOiJIUzI1NiJ9." + payload + ".sig"
}

func TestExpiry(t *testing.T) {
	f := seventvtest.New(t)
	exp := time.Date(2026, 11, 28, 22, 24, 55, 0, time.UTC)
	f.Token = jwt(exp)
	got, ok := f.Client().Expiry(context.Background())
	if !ok || !got.Equal(exp) {
		t.Errorf("got %v %v, want %v", got, ok, exp)
	}
	f.Token = "not-a-jwt"
	if _, ok := f.Client().Expiry(context.Background()); ok {
		t.Error("a token without exp has an expiry")
	}
}

// A new session stored in Cove is used after 7TV refuses the old one,
// without a restart.
func TestNewSession(t *testing.T) {
	f := seventvtest.New(t)
	f.Token = "old"
	f.Self = api.Account{ID: "lys", Login: "lyslink"}
	c := f.Client()
	if _, err := c.Self(context.Background()); err != nil {
		t.Fatal(err)
	}

	f.Mu.Lock()
	f.Token = "new"
	f.Mu.Unlock()
	if _, err := c.Invites(context.Background()); err != nil {
		t.Fatalf("after the session changed in Cove: %v", err)
	}
}

func TestErrors(t *testing.T) {
	f := seventvtest.New(t)
	if err := f.Client().AddEmote(context.Background(), "s", "e", "a"); !errors.Is(err, api.ErrSession) && !errors.Is(err, api.ErrNoToken) {
		t.Errorf("no session: %v", err)
	}
	f.Token = "t"
	var apiErr *api.APIError
	if err := f.Client().AddEmote(context.Background(), "nope", "e", "a"); !errors.As(err, &apiErr) || apiErr.Code != "BAD_REQUEST" {
		t.Errorf("unknown set: %v", err)
	}
	if err := f.Client().RemoveEmote(context.Background(), "s", "e", ""); err == nil {
		t.Error("a remove without a name was sent; 7TV would remove every copy")
	}
}
