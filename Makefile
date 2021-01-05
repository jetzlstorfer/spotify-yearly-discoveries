include .env

token:
	go run get-token/main.go

run:
	export `cat .env | xargs`
	go run main.go