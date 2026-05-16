$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$outDir = Join-Path $root ("dist\nas-package-" + $timestamp)

New-Item -ItemType Directory -Force -Path $outDir | Out-Null

$go = "C:\Program Files\Go\bin\go.exe"
$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH
$oldCgo = $env:CGO_ENABLED
$oldGocache = $env:GOCACHE
try {
    Push-Location $root
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    if (-not $env:GOCACHE) {
        $env:GOCACHE = Join-Path $root ".gocache"
    }
    & $go build -o (Join-Path $outDir "core-proxy") ./cmd/core-proxy
}
finally {
    Pop-Location
    $env:GOOS = $oldGoos
    $env:GOARCH = $oldGoarch
    $env:CGO_ENABLED = $oldCgo
    $env:GOCACHE = $oldGocache
}

Copy-Item -LiteralPath (Join-Path $root "Dockerfile.nas-binary") -Destination (Join-Path $outDir "Dockerfile.nas-binary")
Copy-Item -LiteralPath (Join-Path $root "docker-compose.nas-direct-replace.yml") -Destination (Join-Path $outDir "docker-compose.yml")
Copy-Item -LiteralPath (Join-Path $root ".env.nas.example") -Destination (Join-Path $outDir ".env.example")
Copy-Item -LiteralPath (Join-Path $root "README.md") -Destination $outDir
Copy-Item -LiteralPath (Join-Path $root ".dockerignore") -Destination $outDir
Copy-Item -LiteralPath (Join-Path $root "web-dist") -Destination (Join-Path $outDir "web-dist") -Recurse
Copy-Item -LiteralPath "C:\Program Files\Git\mingw64\etc\ssl\certs\ca-bundle.crt" -Destination (Join-Path $outDir "ca-certificates.crt")

Write-Host ""
Write-Host "NAS package created at: $outDir"
Write-Host ""
