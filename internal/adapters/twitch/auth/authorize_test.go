package auth

import (
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	u, err := url.Parse(AuthorizeURL("cid", "http://localhost:3000", "st", BotScopes))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("redirect_uri") != "http://localhost:3000" || q.Get("state") != "st" ||
		q.Get("scope") != "user:bot user:read:chat user:write:chat" || q.Get("response_type") != "code" {
		t.Fatalf("query: %v", q)
	}
}

func TestParseRedirect(t *testing.T) {
	for name, tc := range map[string]struct {
		pasted, want, wantErr string
	}{
		"address":     {"http://localhost:3000/?code=abc&scope=user%3Abot&state=st", "abc", ""},
		"code only":   {"  abc  ", "abc", ""},
		"other state": {"http://localhost:3000/?code=abc&state=old", "", "state"},
		"denied":      {"http://localhost:3000/?error=access_denied&error_description=The+user+denied&state=st", "", "access_denied"},
		"empty":       {"", "", "nothing"},
		"no code":     {"http://localhost:3000/?code=&state=st", "", "no code"},
	} {
		got, err := ParseRedirect(tc.pasted, "st")
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: got %q, %v; want an error with %q", name, got, err, tc.wantErr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q", name, got, err, tc.want)
		}
	}
}
