package internal

import (
	"sort"
	"testing"
)

// Regression test for a real user complaint: searching "bunny girl senpai
// rascal does not dream" (the TV series' title words, reordered/combined)
// should still rank "Rascal Does Not Dream of Bunny Girl Senpai" as the best
// match. Levenshtein distance alone compares characters IN ORDER, so
// reordering a title's own words into a query used to tank its score below
// unrelated candidates that happened to share a longer contiguous run of
// characters with the query.
func TestWordOverlapScoreRanksReorderedQueryTitleHighest(t *testing.T) {
	query := "bunny girl senpai rascal does not dream"
	got := wordOverlapScore("Rascal Does Not Dream of Bunny Girl Senpai", query)
	if got != 1.0 {
		t.Fatalf("expected perfect word overlap (1.0) for a title containing every query word, got %v", got)
	}
}

// The movie sequel doesn't contain "bunny"/"senpai", but should still score
// meaningfully (not near-zero the way raw Levenshtein against the full,
// reordered query would) so it survives into a top-10 cutoff.
func TestWordOverlapScorePartialMatchStillMeaningful(t *testing.T) {
	query := "bunny girl senpai rascal does not dream"
	got := wordOverlapScore("Rascal Does Not Dream of a Dreaming Girl", query)
	if got < 0.5 {
		t.Fatalf("expected partial word overlap >= 0.5 for a related title sharing most query words, got %v", got)
	}
}

// An unrelated title sharing no words with the query should score zero,
// regardless of any incidental character-level similarity.
func TestWordOverlapScoreUnrelatedTitleScoresZero(t *testing.T) {
	query := "bunny girl senpai rascal does not dream"
	got := wordOverlapScore("Attack on Titan", query)
	if got != 0 {
		t.Fatalf("expected zero word overlap for an unrelated title, got %v", got)
	}
}

// End-to-end shape of the actual fix: sorting candidates the same way
// SearchAnimeAnilist does (word overlap first, Levenshtein as tiebreaker)
// must put the real title first even for a jumbled multi-word query, which a
// pure-Levenshtein sort would not reliably do.
func TestSearchRankingPutsReorderedQueryTitleFirst(t *testing.T) {
	query := "bunny girl senpai rascal does not dream"
	titles := []string{
		"Sword Art Online",
		"Rascal Does Not Dream of a Dreaming Girl",
		"Rascal Does Not Dream of Bunny Girl Senpai",
		"My Teen Romantic Comedy SNAFU",
	}
	type scored struct {
		title   string
		overlap float64
		dist    int
	}
	var results []scored
	for _, title := range titles {
		results = append(results, scored{title, wordOverlapScore(title, query), levenshtein(title, query)})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].overlap != results[j].overlap {
			return results[i].overlap > results[j].overlap
		}
		return results[i].dist < results[j].dist
	})
	if results[0].title != "Rascal Does Not Dream of Bunny Girl Senpai" {
		t.Fatalf("expected the exact-word-match title to rank first, got %q", results[0].title)
	}
	if results[1].title != "Rascal Does Not Dream of a Dreaming Girl" {
		t.Fatalf("expected the related movie to rank second, got %q", results[1].title)
	}
}
