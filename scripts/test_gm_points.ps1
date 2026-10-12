param([string]$ContainerName = 'l1jgo-whale-postgres-1')

# Repository integration tests use a unique schema and never put public in their
# search_path. Connection secrets stay in this process and are not printed.
$ErrorActionPreference = 'Stop'
$taskContainer = docker inspect $ContainerName | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect PostgreSQL container' }
$taskVariables = @{}
foreach ($taskVariable in $taskContainer.Config.Env) {
    $taskParts = $taskVariable -split '=', 2
    $taskVariables[$taskParts[0]] = $taskParts[1]
}
$taskPort = @($taskContainer.NetworkSettings.Ports.'5432/tcp')[0].HostPort
if (-not $taskPort) { throw 'PostgreSQL container has no published local port' }
$taskUser = [uri]::EscapeDataString($taskVariables['POSTGRES_USER'])
$taskPassword = [uri]::EscapeDataString($taskVariables['POSTGRES_PASSWORD'])
$taskDatabase = [uri]::EscapeDataString($taskVariables['POSTGRES_DB'])
$taskPreviousDSN = $env:L1JGO_TEST_POINTS_DSN
try {
    $env:L1JGO_TEST_POINTS_DSN = "postgres://${taskUser}:${taskPassword}@127.0.0.1:${taskPort}/${taskDatabase}?sslmode=disable"
    Push-Location (Split-Path -Parent $PSScriptRoot)
    try {
        go test ./internal/persist -run '^TestCustomPointPostgresAtomicity$' -count=1 -timeout 60s
        if ($LASTEXITCODE -ne 0) { throw 'Custom point repository integration tests failed' }
    } finally { Pop-Location }
} finally {
    $env:L1JGO_TEST_POINTS_DSN = $taskPreviousDSN
}
