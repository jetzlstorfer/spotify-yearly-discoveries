targetScope = 'resourceGroup'

@description('Base name for all resources')
param appName string = 'spotify-yearly-discoveries'

@description('Azure region for all resources')
param location string = resourceGroup().location

@description('Container image to deploy (e.g. myacr.azurecr.io/spotify-yearly-discoveries:latest)')
param containerImage string

@description('Spotify OAuth client ID')
@secure()
param spotifyId string

@description('Spotify OAuth client secret')
@secure()
param spotifySecret string

@description('Spotify OAuth token JSON')
@secure()
param spotifyToken string

@description('Only include loved/saved songs')
param onlyLovedSongs string = 'true'

// ---------- Container Registry ----------

var acrName = replace('acr${appName}', '-', '')

resource acr 'Microsoft.ContainerRegistry/registries@2023-07-01' = {
  name: length(acrName) > 50 ? substring(acrName, 0, 50) : acrName
  location: location
  sku: {
    name: 'Basic'
  }
  properties: {
    adminUserEnabled: false
  }
}

// ---------- Log Analytics ----------

resource logAnalytics 'Microsoft.OperationalInsights/workspaces@2022-10-01' = {
  name: '${appName}-logs'
  location: location
  properties: {
    sku: {
      name: 'PerGB2018'
    }
    retentionInDays: 30
  }
}

// ---------- Container Apps Environment ----------

resource containerAppEnv 'Microsoft.App/managedEnvironments@2024-03-01' = {
  name: '${appName}-env'
  location: location
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logAnalytics.properties.customerId
        sharedKey: logAnalytics.listKeys().primarySharedKey
      }
    }
  }
}

// ---------- User-Assigned Managed Identity ----------

resource identity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: '${appName}-identity'
  location: location
}

// ACR Pull role assignment so the Container App can pull images
resource acrPullRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(acr.id, identity.id, '7f951dda-4ed3-4680-a7ca-43fe172d538d')
  scope: acr
  properties: {
    principalId: identity.properties.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId(
      'Microsoft.Authorization/roleDefinitions',
      '7f951dda-4ed3-4680-a7ca-43fe172d538d' // AcrPull
    )
  }
}

// ---------- Container App ----------

resource containerApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: appName
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${identity.id}': {}
    }
  }
  properties: {
    managedEnvironmentId: containerAppEnv.id
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
          server: acr.properties.loginServer
          identity: identity.id
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
        {
          name: 'spotify-token'
          value: spotifyToken
        }
      ]
    }
    template: {
      containers: [
        {
          name: appName
          image: containerImage
          command: ['/bin/sh', '-c', 'echo "$SPOTIFY_TOKEN_JSON" > /app/token.json && ./spotify-yearly-discoveries -web']
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
              name: 'SPOTIFY_TOKEN_JSON'
              secretRef: 'spotify-token'
            }
            {
              name: 'TOKEN_FILE'
              value: '/app/token.json'
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
  dependsOn: [
    acrPullRole
  ]
}

// ---------- Outputs ----------

output acrLoginServer string = acr.properties.loginServer
output acrName string = acr.name
output containerAppFqdn string = containerApp.properties.configuration.ingress.fqdn
output identityClientId string = identity.properties.clientId
