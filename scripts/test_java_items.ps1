# Run the repository round-trip tests in a disposable local PostgreSQL container.
# No application database, existing container, or project volume is changed.
$ErrorActionPreference = 'Stop'
$taskContainerName = 'codex-java-items-' + [guid]::NewGuid().ToString('N').Substring(0, 12)
$taskPassword = [guid]::NewGuid().ToString('N')
$taskPreviousDSN = $env:L1JGO_TEST_POINTS_DSN
$taskCreated = $false
try {
    docker run --detach --rm --name $taskContainerName --publish '127.0.0.1::5432' --env 'POSTGRES_USER=fixture' --env "POSTGRES_PASSWORD=$taskPassword" --env 'POSTGRES_DB=fixture' postgres:15-alpine | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not create the isolated PostgreSQL test container' }
    $taskCreated = $true
    $taskReady = $false
    for ($taskAttempt = 0; $taskAttempt -lt 40; $taskAttempt++) {
        docker exec $taskContainerName pg_isready -h 127.0.0.1 -U fixture -d fixture *> $null
        if ($LASTEXITCODE -eq 0) { $taskReady = $true; break }
        Start-Sleep -Milliseconds 500
    }
    if (-not $taskReady) { throw 'Isolated PostgreSQL did not become ready' }
    $taskPort = docker inspect --format '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' $taskContainerName
    if ($LASTEXITCODE -ne 0 -or -not $taskPort) { throw 'Could not resolve the isolated PostgreSQL port' }
    $env:L1JGO_TEST_POINTS_DSN = "postgres://fixture:${taskPassword}@127.0.0.1:${taskPort}/fixture?sslmode=disable"
    Push-Location (Split-Path -Parent $PSScriptRoot)
    try {
        go test ./internal/persist -run '^Test(EnchantItems|ElfAttribute)PostgresRoundTrip$' -count=1 -timeout 60s -v
        if ($LASTEXITCODE -ne 0) { throw 'Java item persistence round-trip tests failed' }
    } finally { Pop-Location }
} finally {
    $env:L1JGO_TEST_POINTS_DSN = $taskPreviousDSN
    if ($taskCreated) {
        docker rm --force --volumes $taskContainerName | Out-Null
        if ($LASTEXITCODE -ne 0) { Write-Warning "Test container cleanup failed: $taskContainerName" }
    }
}

