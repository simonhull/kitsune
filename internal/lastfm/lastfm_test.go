package lastfm_test

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/lastfm"
	"github.com/simonhull/kitsune/internal/lastfm/lastfmtest"
)

const (
	sessionKey = "sk-alice-0123456789"
	scrobbleM  = "track.scrobble"
	choir      = "Low Tide Choir"
)

// connected returns a fake with alice's session and a client for it.
func connected(t *testing.T) (*lastfmtest.Server, *lastfm.Client) {
	t.Helper()
	fake := lastfmtest.New(t)
	fake.AddSession(sessionKey, "alice")
	c := fake.Client()
	return fake, c
}

// TestSign checks the signature against one worked out by hand: the md5 of
// "api_keyxxxxmethodauth.getSessiontokenyyyy" followed by the secret "zzzz".
func TestSign(t *testing.T) {
	t.Parallel()
	params := url.Values{
		"method": {"auth.getSession"}, "api_key": {"xxxx"}, "token": {"yyyy"},
		"format": {"json"}, "callback": {"cb"},
	}
	// printf 'api_keyxxxxmethodauth.getSessiontokenyyyyzzzz' | md5sum
	const want = "d3be358e96cb2926e940514f19db6adf"
	if got := lastfm.Sign(params, "zzzz"); got != want {
		t.Errorf("Sign = %s, want %s", got, want)
	}
}

func TestAuthURL(t *testing.T) {
	t.Parallel()
	c := lastfm.New("key-1", "secret")
	got, err := url.Parse(c.AuthURL("https://music.example/api/v1/lastfm/callback?state=a&b"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Scheme+"://"+got.Host+got.Path != lastfm.DefaultAuthBase {
		t.Errorf("AuthURL base = %s, want %s", got, lastfm.DefaultAuthBase)
	}
	if q := got.Query(); q.Get("api_key") != "key-1" || q.Get("cb") != "https://music.example/api/v1/lastfm/callback?state=a&b" {
		t.Errorf("AuthURL query = %v", q)
	}
	if strings.Contains(got.String(), "secret") {
		t.Errorf("AuthURL %s holds the secret", got)
	}
}

func TestSession(t *testing.T) {
	t.Parallel()
	fake := lastfmtest.New(t)
	fake.Approve("tok", "alice", sessionKey)
	c := fake.Client()
	got, err := c.Session(t.Context(), "tok")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if got != (lastfm.Session{Name: "alice", Key: sessionKey}) {
		t.Errorf("Session = %+v", got)
	}
	_, err = c.Session(t.Context(), "tok")
	if !errors.Is(err, lastfm.ErrRejected) {
		t.Errorf("reused token: err = %v, want ErrRejected", err)
	}
	if fake.HasSecret(err.Error()) {
		t.Errorf("error %q holds a secret", err)
	}
}

func TestNowPlayingParams(t *testing.T) {
	t.Parallel()
	fake, c := connected(t)
	tr := lastfm.Track{
		Artist: choir, Title: "Condensation", Album: "Brine", AlbumArtist: choir,
		MBID: "0f1c", Duration: 227*time.Second + 400*time.Millisecond, Number: 3,
	}
	if err := c.NowPlaying(t.Context(), sessionKey, &tr); err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	calls := fake.CallsTo("track.updateNowPlaying")
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	p := calls[0].Params
	want := map[string]string{
		"artist": choir, "track": "Condensation", "album": "Brine", "albumArtist": choir,
		"mbid": "0f1c", "duration": "227", "trackNumber": "3", "sk": sessionKey, "api_key": lastfmtest.APIKey,
	}
	for k, v := range want {
		if p.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, p.Get(k), v)
		}
	}
}

func TestNowPlayingLeavesOutEmpty(t *testing.T) {
	t.Parallel()
	fake, c := connected(t)
	if err := c.NowPlaying(t.Context(), sessionKey, &lastfm.Track{Artist: "A", Title: "T"}); err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	p := fake.CallsTo("track.updateNowPlaying")[0].Params
	for _, k := range []string{"album", "albumArtist", "mbid", "duration", "trackNumber"} {
		if p.Has(k) {
			t.Errorf("%s sent as %q, want it left out", k, p.Get(k))
		}
	}
}

func TestScrobbleInOrder(t *testing.T) {
	t.Parallel()
	fake, c := connected(t)
	at := time.Unix(1_700_000_000, 0)
	plays := []lastfm.Scrobble{
		{Artist: "A", Title: "One", Number: 1, At: at},
		{Artist: "B", Title: "Two", At: at.Add(time.Minute)},
	}
	ignored, err := c.Scrobble(t.Context(), sessionKey, plays)
	if err != nil || len(ignored) != 0 {
		t.Fatalf("Scrobble = %v, %v", ignored, err)
	}
	want := []string{"A - One @ 1700000000", "B - Two @ 1700000060"}
	if got := fake.Scrobbles(); !slices.Equal(got, want) {
		t.Errorf("scrobbles = %q, want %q", got, want)
	}
	if p := fake.CallsTo(scrobbleM)[0].Params; p.Get("trackNumber[0]") != "1" || p.Has("trackNumber[1]") {
		t.Errorf("trackNumber = %q, %q", p.Get("trackNumber[0]"), p.Get("trackNumber[1]"))
	}
}

func TestScrobbleIgnored(t *testing.T) {
	t.Parallel()
	for _, n := range []int{1, 3} { // One result comes back as a bare object, not a list.
		fake, c := connected(t)
		fake.Ignore("Nope")
		plays := make([]lastfm.Scrobble, n)
		for i := range plays {
			plays[i] = lastfm.Scrobble{Artist: "Fine", Title: "T", At: time.Unix(1_700_000_000, 0)}
		}
		plays[n-1].Artist = "Nope"
		ignored, err := c.Scrobble(t.Context(), sessionKey, plays)
		if err != nil {
			t.Fatalf("Scrobble(%d): %v", n, err)
		}
		if len(ignored) != 1 || ignored[0].Index != n-1 || ignored[0].Code != 1 {
			t.Errorf("Scrobble(%d) ignored = %+v, want only the last", n, ignored)
		}
	}
}

func TestScrobbleLimits(t *testing.T) {
	t.Parallel()
	fake, c := connected(t)
	if ignored, err := c.Scrobble(t.Context(), sessionKey, nil); err != nil || ignored != nil {
		t.Errorf("Scrobble(nil) = %v, %v", ignored, err)
	}
	_, err := c.Scrobble(t.Context(), sessionKey, make([]lastfm.Scrobble, lastfm.MaxScrobbles+1))
	if !errors.Is(err, lastfm.ErrRejected) {
		t.Errorf("Scrobble(51) err = %v, want ErrRejected", err)
	}
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("calls = %d, want none", n)
	}
}

func TestLoveAndUnlove(t *testing.T) {
	t.Parallel()
	fake, c := connected(t)
	if err := c.Love(t.Context(), sessionKey, "A", "T"); err != nil {
		t.Fatalf("Love: %v", err)
	}
	if err := c.Unlove(t.Context(), sessionKey, "A", "T"); err != nil {
		t.Fatalf("Unlove: %v", err)
	}
	if len(fake.CallsTo("track.love")) != 1 || len(fake.CallsTo("track.unlove")) != 1 {
		t.Errorf("calls = %+v", fake.Calls())
	}
}

func TestErrorKinds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		fail int
		want error
	}{
		{name: "offline", fail: 11, want: lastfm.ErrTemporary},
		{name: "temporary", fail: 16, want: lastfm.ErrTemporary},
		{name: "rate limited", fail: 29, want: lastfm.ErrTemporary},
		{name: "server error", fail: http.StatusBadGateway, want: lastfm.ErrTemporary},
		{name: "too many requests", fail: http.StatusTooManyRequests, want: lastfm.ErrTemporary},
		{name: "invalid session", fail: 9, want: lastfm.ErrSession},
		{name: "invalid parameters", fail: 6, want: lastfm.ErrRejected},
		{name: "bad request", fail: http.StatusBadRequest, want: lastfm.ErrRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake, c := connected(t)
			fake.Fail(tt.fail)
			err := c.Love(t.Context(), sessionKey, "A", "T")
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if fake.HasSecret(err.Error()) {
				t.Errorf("error %q holds a secret", err)
			}
		})
	}
}

func TestRevokedSession(t *testing.T) {
	t.Parallel()
	fake, c := connected(t)
	fake.Revoke(sessionKey)
	err := c.NowPlaying(t.Context(), sessionKey, &lastfm.Track{Artist: "A", Title: "T"})
	var apiErr *lastfm.Error
	if !errors.Is(err, lastfm.ErrSession) || !errors.As(err, &apiErr) || apiErr.Code != 9 {
		t.Errorf("err = %v, want code 9 and ErrSession", err)
	}
	if fake.HasSecret(err.Error()) {
		t.Errorf("error %q holds a secret", err)
	}
}

func TestUnreachable(t *testing.T) {
	t.Parallel()
	fake, c := connected(t)
	fake.Close()
	err := c.Love(t.Context(), sessionKey, "A", "T")
	if !errors.Is(err, lastfm.ErrTemporary) {
		t.Errorf("err = %v, want ErrTemporary", err)
	}
	if strings.Contains(err.Error(), sessionKey) || strings.Contains(err.Error(), lastfmtest.Secret) {
		t.Errorf("error %q holds a secret", err)
	}
}

func TestWrongSecretFailsSignature(t *testing.T) {
	t.Parallel()
	fake := lastfmtest.New(t)
	fake.AddSession(sessionKey, "alice")
	c := lastfm.New(lastfmtest.APIKey, "not-the-secret")
	fake.Point(c)
	if err := c.Love(t.Context(), sessionKey, "A", "T"); !errors.Is(err, lastfm.ErrRejected) {
		t.Errorf("err = %v, want ErrRejected for a bad signature", err)
	}
}
