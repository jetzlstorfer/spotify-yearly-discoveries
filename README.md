
# Spotify Discoveries (per year)

I wanted to know which songs I've discovered in 2022 and added to my playlists that are actually from the year 2022. Therefore I made this small program.

## Why?
A lot of "best of 202x" / "top 202x songs" / "best artists 202x" lists contain songs that might have received heavy airplay in 202x but are actually not even from 202x but probably 202x-1 or even earlier. This is totally fine. But for myself I wanted to find out which songs I have discovered in 2022 that are _actually_ from 2022.

## What it's doing
The program checks all playlists that contain the term `YEAR_TO_CHHECK`,eg. "2022" and in all playlists that match this expression it looks for song that are actually from the year 2022 (or your desired year) _and_ check if they have been added to your library (❤️ in Spotify). If both is true (in this example: song is from 2022 + you like it) it adds the songs to the specified playlist.

That's it.

Here are my playlists for [2022](https://open.spotify.com/playlist/4AJnjP36kH39gQhgZFL8Ff?si=0f8b2b44f7ca4208), [2021](https://open.spotify.com/playlist/3qDtmE3TrHkjVOow3rM3BY?si=8f212c2c8f0148ee), [2020](https://open.spotify.com/playlist/2D0NidVJbZfnR4wmvYSRiA?si=tVTpL61pRGWypiROYqdeqQ), [2019](https://open.spotify.com/playlist/0uwZfzhqw2G5id1El0oCJE?si=WFk_PEYZSpijQ4gdnYOsXQ).

## How to use it
- Create a [Spotify application](https://developer.spotify.com/dashboard/applications) to receive a client id and secret
- Create a new (empty) playlist for the songs to be stored in
- Have [Go](https://go.dev/) ready on your machine (I tested with Go 1.15.3+ and latest with 1.19.2)
- Adapt values in the `env` file and copy or rename it to `.env`

# Run it

1. Generate and store the token
    ```
    make token
    ```

2. Run the program
    ```
    make run
    ```

3. Check your playlist 🕺
    ```
    make open-browser
    ```
    If this command doesn't work, check the playlist manually either in the browser or in your Spotify client.

