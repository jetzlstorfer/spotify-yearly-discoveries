using 'main.bicep'

param appName = 'spotify-yearly-discoveries'
param containerImage = readEnvironmentVariable('SERVICE_WEB_IMAGE_NAME', 'mcr.microsoft.com/azuredocs/containerapps-helloworld:latest')
param spotifyId = readEnvironmentVariable('SPOTIFY_ID', '')
param spotifySecret = readEnvironmentVariable('SPOTIFY_SECRET', '')
param onlyLovedSongs = readEnvironmentVariable('ONLY_LOVED_SONGS', 'false')
