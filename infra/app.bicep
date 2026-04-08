targetScope = 'resourceGroup'

@description('Base name for all resources')
param appName string = 'spotify-yearly-discoveries'

@description('Azure region for all resources')
param location string = resourceGroup().location

@description('Full container image URI (e.g. myacr.azurecr.io/app:tag)')
param containerImage string

@description('Resource ID of the Container Apps environment')
param environmentId string

@description('Resource ID of the user-assigned managed identity')
param identityId string

@description('ACR login server (e.g. myacr.azurecr.io)')
param acrLoginServer string

@description('Spotify OAuth client ID')
@secure()
param spotifyId string

@description('Spotify OAuth client secret')
@secure()
param spotifySecret string

@description('Only include loved/saved songs')
param onlyLovedSongs string = 'true'

// ---------- Container App ----------

resource containerApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: appName
  location: location
  tags: {
    'azd-service-name': 'web'
  }
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${identityId}': {}
    }
  }
  properties: {
    managedEnvironmentId: environmentId
    configuration: {
      activeRevisionsMode: 'Single'
      ingress: {
        external: true
        targetPort: 8080
        transport: 'http'
        allowInsecure: false
      }
      registries: [
        {
          server: acrLoginServer
          identity: identityId
        }
      ]
      secrets: [
        {
          name: 'spotify-id'
          value: spotifyId
        }
        {
          name: 'spotify-secret'
          value: spotifySecret
        }
      ]
    }
    template: {
      containers: [
        {
          name: appName
          image: containerImage
          resources: {
            cpu: json('0.25')
            memory: '0.5Gi'
          }
          env: [
            {
              name: 'SPOTIFY_ID'
              secretRef: 'spotify-id'
            }
            {
              name: 'SPOTIFY_SECRET'
              secretRef: 'spotify-secret'
            }
            {
              name: 'ONLY_LOVED_SONGS'
              value: onlyLovedSongs
            }
          ]
        }
      ]
      scale: {
        minReplicas: 0
        maxReplicas: 1
      }
    }
  }
}

// ---------- Outputs ----------

output containerAppFqdn string = containerApp.properties.configuration.ingress.fqdn
