
# Songs I've discovered in 2020

I wanted to know which songs that are actually from the year 2020 I've added to my Spotify library. Therefore I made this small program.

## Why?
A lot of "best of 2020" / "top 2020 songs" / "best artists 2020" lists contain songs that might have received heavy airplay in 2020 but are actually not even from 2020 but probalby 2019 or even earlier. This is totally fine. But for myself I wanted to find out which songs I have discovered in 2020 that are _actually_ from 2020.

## What it's doing
The program checks all playlists that contain the term "2020" (which can be customizable of course) and in all playlists that match this expression it looks for song that are actually from the year 2020 (or your desired year) _and_ check if they have been added to your library. If both is true (song is from 2020 + you like it) it adds the songs to a new playlist.
That's it.

## How to use it
- Create a [Spotify application](https://developer.spotify.com/dashboard/applications) to receive a client id and secret
- Create a new (empty) playlist for the songs to be stored in
- Have Go ready on your machine (I tested with Go 1.15.3)
- Adapt values in the `env` file and copy or rename it to `.env`

# Run it

Generate and store the token
```
make token
```

Run the program
```
make run
```