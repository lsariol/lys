package randomcat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lsariol/lys/internal/commands"
)

func TestRun(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		switch r.URL.Path {
		case "/cat":
			w.Write([]byte(`{"id":"pic","mimetype":"image/jpeg"}`))
		case "/cat/gif":
			w.Write([]byte(`{"id":"anim","mimetype":"image/gif"}`))
		case "/broken/cat":
			w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	for _, tc := range []struct {
		endpoint string
		gif      bool
		want     string
		err      bool
	}{
		{srv.URL, false, "Here's a random cat. https://cataas.com/cat/pic", false},
		{srv.URL, true, "Here's a random cat. https://cataas.com/cat/anim", false},
		{srv.URL + "/broken", false, failReply, true},
		{srv.URL + "/missing", false, failReply, true},
	} {
		c := &RandomCat{http: srv.Client(), endpoint: tc.endpoint, gif: func() bool { return tc.gif }}
		reply, err := c.Run(context.Background(), commands.Request{})
		if reply.Text != tc.want || (err != nil) != tc.err {
			t.Errorf("%s gif=%v: %q, %v; want %q", tc.endpoint, tc.gif, reply.Text, err, tc.want)
		}
	}
	if len(asked) != 4 || asked[0] != "/cat" || asked[1] != "/cat/gif" {
		t.Errorf("asked for %v", asked)
	}
}
