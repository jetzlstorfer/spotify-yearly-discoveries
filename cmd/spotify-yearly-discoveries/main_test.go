package main

import (
	"testing"

	"github.com/zmb3/spotify/v2"
)

func TestRemoveDuplicateValues(t *testing.T) {
	tests := []struct {
		name  string
		input []spotify.ID
		want  []spotify.ID
	}{
		{
			name:  "no duplicates",
			input: []spotify.ID{"a", "b", "c"},
			want:  []spotify.ID{"a", "b", "c"},
		},
		{
			name:  "all duplicates",
			input: []spotify.ID{"a", "a", "a"},
			want:  []spotify.ID{"a"},
		},
		{
			name:  "mixed duplicates",
			input: []spotify.ID{"a", "b", "a", "c", "b"},
			want:  []spotify.ID{"a", "b", "c"},
		},
		{
			name:  "empty slice",
			input: []spotify.ID{},
			want:  []spotify.ID{},
		},
		{
			name:  "single element",
			input: []spotify.ID{"a"},
			want:  []spotify.ID{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeDuplicateValues(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("removeDuplicateValues() len = %d, want %d", len(got), len(tt.want))
			}
			for i, id := range got {
				if id != tt.want[i] {
					t.Errorf("removeDuplicateValues()[%d] = %q, want %q", i, id, tt.want[i])
				}
			}
		})
	}
}

func TestTrackIsFromYear(t *testing.T) {
	tests := []struct {
		name        string
		releaseDate string
		year        string
		want        bool
	}{
		{
			name:        "full date match",
			releaseDate: "2024-03-15",
			year:        "2024",
			want:        true,
		},
		{
			name:        "year only date match",
			releaseDate: "2024",
			year:        "2024",
			want:        true,
		},
		{
			name:        "no match",
			releaseDate: "2023-05-01",
			year:        "2024",
			want:        false,
		},
		{
			name:        "empty release date",
			releaseDate: "",
			year:        "2024",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// trackIsFromYear uses the global `year` variable
			year = tt.year
			track := spotify.PlaylistTrack{
				Track: spotify.FullTrack{
					SimpleTrack: spotify.SimpleTrack{},
					Album: spotify.SimpleAlbum{
						ReleaseDate: tt.releaseDate,
					},
				},
			}
			got := trackIsFromYear(track)
			if got != tt.want {
				t.Errorf("trackIsFromYear(%q, year=%q) = %v, want %v",
					tt.releaseDate, tt.year, got, tt.want)
			}
		})
	}
}
