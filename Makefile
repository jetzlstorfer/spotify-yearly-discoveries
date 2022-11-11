include .env
export 

token:
	go run get-token/main.go

run:
	go run main.go

open-browser:
	python3 -m webbrowser https://open.spotify.com/playlist/$(PLAYLIST_ID)
