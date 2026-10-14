package pulls

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
)

var firstReviews = ReviewsQuery{Repo: repo, Number: 1}

// reviewsAPI is a fake GitHub with one comment and one review on #1.
func reviewsAPI() *fakeAPI {
	return &fakeAPI{
		comments: func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Comment], error) {
			return core.Page[core.Comment]{Items: []core.Comment{{ID: "c"}}}, nil
		},
		reviews: func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Review], error) {
			return core.Page[core.Review]{Items: []core.Review{{ID: "r"}}}, nil
		},
	}
}

// unreachable makes the comments and reviews of api fail as if GitHub
// couldn't be reached.
func unreachable(api *fakeAPI) *fakeAPI {
	api.comments = func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Comment], error) {
		return core.Page[core.Comment]{}, errDial
	}
	api.reviews = func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Review], error) {
		return core.Page[core.Review]{}, errDial
	}
	return api
}

func TestKeptCommentsServedOfflineWithoutKeptReviews(t *testing.T) {
	store := openStore(t)
	// The first session keeps the comments but never reads the reviews.
	first := New(reviewsAPI(), WithStore(cachetest.Aged(store, time.Hour)))
	if _, err := first.Comments(t.Context(), firstComments); err != nil {
		t.Fatalf("Comments: %v", err)
	}

	s := New(unreachable(reviewsAPI()), WithStore(cachetest.Aged(store, time.Hour)))
	c, err := s.Comments(t.Context(), firstComments)
	if err != nil || !c.Offline || len(c.Items) != 1 {
		t.Errorf("Comments offline = %+v, %v; want the kept page, offline", c, err)
	}
	// Nothing of the reviews is kept, so the read has no data to serve.
	r, err := s.Reviews(t.Context(), firstReviews)
	if !errors.Is(err, core.ErrOffline) || len(r.Items) != 0 || r.Offline {
		t.Errorf("Reviews offline = %+v, %v; want no page and the offline error", r, err)
	}
}

func TestKeptReviewsServedOffline(t *testing.T) {
	store := openStore(t)
	first := New(reviewsAPI(), WithStore(cachetest.Aged(store, time.Hour)))
	if _, err := first.Reviews(t.Context(), firstReviews); err != nil {
		t.Fatalf("Reviews: %v", err)
	}

	api := unreachable(reviewsAPI())
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	p, err := s.Reviews(t.Context(), firstReviews)
	if err != nil || !p.Offline || len(p.Items) != 1 || p.Items[0].ID != "r" {
		t.Errorf("Reviews offline = %+v, %v; want the kept page, offline", p, err)
	}
	if n := api.count("reviews"); n != 1 {
		t.Errorf("reviews called %d times, want 1: the kept page is read again", n)
	}
}

func TestReviewsFollowCursorAcrossPages(t *testing.T) {
	api := reviewsAPI()
	api.reviews = func(_ context.Context, _ core.RepoRef, _ int, cursor string, _ int) (core.Page[core.Review], error) {
		if cursor == "c1" {
			return core.Page[core.Review]{Items: []core.Review{{ID: "r3"}}}, nil
		}
		return core.Page[core.Review]{Items: []core.Review{{ID: "r1"}, {ID: "r2"}}, Next: "c1"}, nil
	}
	s := New(api, WithStore(openStore(t)))

	var got []string
	q := firstReviews
	for range 3 {
		p, err := s.Reviews(t.Context(), q)
		if err != nil {
			t.Fatalf("Reviews: %v", err)
		}
		for _, r := range p.Items {
			got = append(got, r.ID)
		}
		if p.Next == "" {
			break
		}
		q.Cursor = p.Next
	}
	if want := []string{"r1", "r2", "r3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("reviews = %v, want %v", got, want)
	}
	if n := api.count("reviews"); n != 2 {
		t.Errorf("reviews called %d times, want 2", n)
	}
}
