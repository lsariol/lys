package letterboxd

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

const sampleFeed = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:letterboxd="https://letterboxd.com" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel>
<item>
  <link>https://letterboxd.com/fan/film/heat/</link>
  <guid isPermaLink="false">letterboxd-review-3</guid>
  <pubDate>Mon, 26 Oct 2026 20:00:00 +1300</pubDate>
  <letterboxd:watchedDate>2026-10-25</letterboxd:watchedDate>
  <letterboxd:rewatch>Yes</letterboxd:rewatch>
  <letterboxd:filmTitle>Heat</letterboxd:filmTitle>
  <letterboxd:filmYear>1995</letterboxd:filmYear>
  <letterboxd:memberRating>4.5</letterboxd:memberRating>
  <letterboxd:memberLike>Yes</letterboxd:memberLike>
</item>
<item>
  <link>https://letterboxd.com/fan/film/alien/</link>
  <guid isPermaLink="false">letterboxd-review-2</guid>
  <pubDate>Sun, 25 Oct 2026 20:00:00 +1300</pubDate>
  <letterboxd:watchedDate>2026-10-24</letterboxd:watchedDate>
  <letterboxd:rewatch>No</letterboxd:rewatch>
  <letterboxd:filmTitle>Alien</letterboxd:filmTitle>
  <letterboxd:filmYear>1979</letterboxd:filmYear>
  <letterboxd:memberRating>3.0</letterboxd:memberRating>
</item>
<item>
  <link>https://letterboxd.com/fan/film/jaws/</link>
  <guid isPermaLink="false">letterboxd-review-1</guid>
  <pubDate>Sat, 24 Oct 2026 20:00:00 +1300</pubDate>
  <letterboxd:filmTitle>Jaws</letterboxd:filmTitle>
  <letterboxd:filmYear>1975</letterboxd:filmYear>
  <letterboxd:memberRating>5.0</letterboxd:memberRating>
</item>
</channel>
</rss>`

func sample(t *testing.T) []movie {
	t.Helper()
	var feed rss
	if err := xml.Unmarshal([]byte(sampleFeed), &feed); err != nil {
		t.Fatal(err)
	}
	return parse(feed)
}

func TestParse(t *testing.T) {
	m := sample(t)
	if len(m) != 3 {
		t.Fatalf("got %d entries", len(m))
	}
	if m[0].GUID != "letterboxd-review-3" || m[0].Title != "Heat" || m[0].Year != 1995 || m[0].Rating != 4.5 || !m[0].Liked || !m[0].Rewatch {
		t.Fatalf("first entry: %+v", m[0])
	}
	if m[1].Liked || m[1].Rewatch {
		t.Fatalf("second entry: %+v", m[1])
	}
}

func TestNewMovies(t *testing.T) {
	m := sample(t)
	titles := func(ms []movie) string {
		var out []string
		for _, x := range ms {
			out = append(out, x.Title)
		}
		return strings.Join(out, ",")
	}
	alerted := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

	for name, tc := range map[string]struct {
		lastSeen string
		alerted  *time.Time
		want     string
	}{
		"new subscription":   {"", nil, "Heat"},
		"one new":            {"letterboxd-review-2", nil, "Heat"},
		"two new":            {"letterboxd-review-1", nil, "Heat,Alien"},
		"nothing new":        {"letterboxd-review-3", nil, ""},
		"seen entry deleted": {"gone", &alerted, "Heat"},
		"deleted, no time":   {"gone", nil, "Heat"},
	} {
		if got := titles(newMovies(m, tc.lastSeen, tc.alerted)); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
	if got := newMovies(nil, "x", nil); got != nil {
		t.Errorf("an empty feed: got %v", got)
	}
}

func TestMessage(t *testing.T) {
	m := sample(t)
	if got, want := message("Viewer", "https://letterboxd.com/fan/", m[:1]),
		"🎬 Viewer rewatched Heat (1995) on 2026-10-25 | ★★★★½ |❤️| Read their review here: https://letterboxd.com/fan/film/heat/"; got != want {
		t.Errorf("one:\n got %q\nwant %q", got, want)
	}
	if got, want := message("Viewer", "https://letterboxd.com/fan/", m[1:2]),
		"🎬 Viewer watched Alien (1979) on 2026-10-24 | ★★★ | Read their review here: https://letterboxd.com/fan/film/alien/"; got != want {
		t.Errorf("not liked:\n got %q\nwant %q", got, want)
	}
	if got, want := message("Viewer", "https://letterboxd.com/fan/", m),
		"🎬 Viewer logged 3 new movies. Heat (1995)🔁 - ★★★★½ ❤️, Alien (1979) - ★★★, Jaws (1975) - ★★★★★. See other entries and reviews here: https://letterboxd.com/fan/activity/"; got != want {
		t.Errorf("several:\n got %q\nwant %q", got, want)
	}

	var many []movie
	for range 40 {
		many = append(many, m[1])
	}
	if got := message("Viewer", "https://letterboxd.com/fan/", many); len(got) > maxMessage {
		t.Errorf("too long: %d characters", len(got))
	}
}
