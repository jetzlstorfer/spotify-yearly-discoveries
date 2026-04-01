include .env
export

.PHONY: token run build open-browser

token:
	go run ./cmd/get-token

run:
	go run ./cmd/spotify-yearly-discoveries

build:
	go build -o spotify-yearly-discoveries ./cmd/spotify-yearly-discoveries

open-browser:
	python3 -m webbrowser https://open.spotify.com/playlist/$(PLAYLIST_ID)
