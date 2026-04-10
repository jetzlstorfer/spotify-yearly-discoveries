package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/zmb3/spotify/v2"
)

// fetchPlaylistsForYear returns all playlists owned by userID whose name
// contains yr. If excludeID is non-empty, that playlist is excluded from
// the result. This is the single canonical implementation used by both
// batch mode and web mode.
func fetchPlaylistsForYear(ctx context.Context, client *spotify.Client, yr, userID, excludeID string) []spotify.SimplePlaylist {
	var result []spotify.SimplePlaylist
	const limit = 50
	offset := 0

	for p := 1; ; p++ {
		page, err := client.CurrentUsersPlaylists(ctx, spotify.Limit(limit), spotify.Offset(offset))
		if err != nil {
			slog.Error("couldn't get playlists", "err", err)
			break
		}
		for _, pl := range page.Playlists {
			if strings.Contains(pl.Name, yr) && pl.Owner.ID == userID && string(pl.ID) != excludeID {
				result = append(result, pl)
			}
		}
		offset = p * limit
		if page.Next == "" {
			break
		}
	}
	return result
}
