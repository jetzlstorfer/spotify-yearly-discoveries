include .env
export 

token:
	go run get-token/main.go

run:
	go run . 

serve:
	go run . -web

open-browser:
	python3 -m webbrowser https://open.spotify.com/playlist/$(PLAYLIST_ID)
