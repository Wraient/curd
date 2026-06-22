package internal

import (
	"fmt"
	"testing"
	"time"
)

func benchmarkAnimeList(size int) AnimeList {
	list := AnimeList{}
	for i := 0; i < size; i++ {
		entry := Entry{
			Media: Media{
				ID:    i + 1,
				MalID: 1000 + i,
				Title: AnimeTitle{
					English: fmt.Sprintf("Anime %d", i+1),
					Romaji:  fmt.Sprintf("Anime %d", i+1),
				},
			},
			Status:    "CURRENT",
			Progress:  i % 24,
			Score:     float64(i % 10),
			UpdatedAt: time.Date(2026, 1, 1, 0, 0, i%60, 0, time.UTC),
		}
		switch i % 5 {
		case 0:
			list.Watching = append(list.Watching, entry)
		case 1:
			entry.Status = "COMPLETED"
			list.Completed = append(list.Completed, entry)
		case 2:
			entry.Status = "PAUSED"
			list.Paused = append(list.Paused, entry)
		case 3:
			entry.Status = "PLANNING"
			list.Planning = append(list.Planning, entry)
		default:
			entry.Status = "DROPPED"
			list.Dropped = append(list.Dropped, entry)
		}
	}
	return list
}

func BenchmarkBuildDualRemoteSyncPlan(b *testing.B) {
	aniList := benchmarkAnimeList(1000)
	myAnimeList := benchmarkAnimeList(1000)
	for i := range myAnimeList.Watching {
		myAnimeList.Watching[i].Progress += 1
		myAnimeList.Watching[i].UpdatedAt = myAnimeList.Watching[i].UpdatedAt.Add(time.Hour)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildDualRemoteSyncPlan(aniList, myAnimeList)
	}
}

func BenchmarkMergeAnimeLists(b *testing.B) {
	aniList := benchmarkAnimeList(1000)
	myAnimeList := benchmarkAnimeList(1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mergeAnimeLists(aniList, myAnimeList)
	}
}

func BenchmarkLevenshteinAnimeTitles(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = levenshtein("Sousou no Frieren Beyond Journey's End", "Frieren Beyond Journey's End")
	}
}
