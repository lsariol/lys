package letterboxd

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// rss is a Letterboxd member's RSS feed.
type rss struct {
	Channel struct {
		Items []item `xml:"item"`
	} `xml:"channel"`
}

type item struct {
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`

	WatchedDate  string  `xml:"https://letterboxd.com watchedDate"`
	Rewatch      string  `xml:"https://letterboxd.com rewatch"`
	FilmTitle    string  `xml:"https://letterboxd.com filmTitle"`
	FilmYear     int     `xml:"https://letterboxd.com filmYear"`
	MemberRating float64 `xml:"https://letterboxd.com memberRating"`
	MemberLike   string  `xml:"https://letterboxd.com memberLike"`
}

// movie is one diary entry, newest first in a feed.
type movie struct {
	Link        string
	GUID        string
	PubDate     string
	WatchedDate string
	Rewatch     bool
	Title       string
	Year        int
	Rating      float64
	Liked       bool
}

func parse(feed rss) []movie {
	out := make([]movie, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		out = append(out, movie{
			Link:        it.Link,
			GUID:        strings.TrimSpace(it.GUID),
			PubDate:     it.PubDate,
			WatchedDate: it.WatchedDate,
			Rewatch:     it.Rewatch == "Yes",
			Title:       it.FilmTitle,
			Year:        it.FilmYear,
			Rating:      it.MemberRating,
			Liked:       it.MemberLike == "Yes",
		})
	}
	return out
}

// newMovies returns the entries since lastSeen, newest first. With no lastSeen
// (a new subscription) it's the newest entry, so the channel sees it works.
// When lastSeen has left the feed (the entry was deleted), it's the entries
// published after lastAlerted.
func newMovies(movies []movie, lastSeen string, lastAlerted *time.Time) []movie {
	if len(movies) == 0 {
		return nil
	}
	if lastSeen == "" {
		return movies[:1]
	}

	for i, m := range movies {
		if m.GUID == lastSeen {
			return movies[:i]
		}
	}

	if lastAlerted == nil {
		return movies[:1]
	}
	var out []movie
	for _, m := range movies {
		if pub, err := time.Parse(time.RFC1123Z, m.PubDate); err == nil && pub.After(*lastAlerted) {
			out = append(out, m)
		}
	}
	return out
}

// maxMessage keeps announcements within Twitch's 500-character limit.
const maxMessage = 480

// message announces new entries the way LetterboxdWatcher did. name is who
// subscribed; profile is their Letterboxd profile address.
func message(name, profile string, movies []movie) string {
	if len(movies) == 1 {
		m := movies[0]
		verb := "watched"
		if m.Rewatch {
			verb = "rewatched"
		}
		liked := ""
		if m.Liked {
			liked = "❤️|"
		}
		return fmt.Sprintf("🎬 %s %s %s (%d) on %s | %s |%s Read their review here: %s",
			name, verb, m.Title, m.Year, m.WatchedDate, stars(m.Rating), liked, m.Link)
	}

	header := fmt.Sprintf("🎬 %s logged %d new movies. ", name, len(movies))
	footer := fmt.Sprintf("See other entries and reviews here: %sactivity/", profile)
	room := maxMessage - len(header) - len(footer)

	var reviews []string
	used := 2 // the closing ". "
	for _, m := range movies {
		r := fmt.Sprintf("%s (%d)", m.Title, m.Year)
		if m.Rewatch {
			r += "🔁"
		}
		r += " - " + stars(m.Rating)
		if m.Liked {
			r += " ❤️"
		}
		add := len(r)
		if len(reviews) > 0 {
			add += len(", ")
		}
		if used+add > room {
			break
		}
		reviews = append(reviews, r)
		used += add
	}
	return header + strings.Join(reviews, ", ") + ". " + footer
}

func stars(rating float64) string {
	s := strings.Repeat("★", int(rating))
	if rating != math.Trunc(rating) {
		s += "½"
	}
	return s
}
