targetScope = 'resourceGroup'

@description('Base name for all resources')
param appName string = 'spotify-yearly-discoveries'

@description('Azure region for all resources')
param location string = resourceGroup().location

// ---------- Container Registry ----------

var acrName = replace('acr${appName}', '-', '')
var acrActualName = length(acrName) > 50 ? substring(acrName, 0, 50) : acrName

resource acr 'Microsoft.ContainerRegistry/registries@2023-07-01' = {
  name: acrActualName
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

// ---------- Outputs ----------

output acrLoginServer string = acr.properties.loginServer
output acrName string = acr.name
output environmentId string = containerAppEnv.id
output identityId string = identity.id
output AZURE_CONTAINER_REGISTRY_ENDPOINT string = acr.properties.loginServer
