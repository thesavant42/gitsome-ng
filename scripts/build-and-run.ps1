# PowerShell script to build and run yolosint with a fresh build
# This script clears the screen, runs go mod tidy, builds the binary, and executes it

# Clear the screen
Clear-Host

# Change to the project root directory (where go.mod is located)
$scriptPath = $PSScriptRoot
$projectRoot = if ($scriptPath) { Join-Path $scriptPath ".." } else { (Get-Location).Path }
Set-Location $projectRoot

# Run go mod tidy to clean up dependencies
Write-Host "Running go mod tidy..."
go mod tidy

# Build the yolosint binary
Write-Host "Building yolosint.exe..."
go build -o yolosint.exe ./cmd/yolosint

# Check if build was successful
if ($LASTEXITCODE -eq 0) {
    Write-Host "Build successful. Running yolosint.exe..."
    Write-Host "----------------------------------------"
    # Execute the binary
    .\yolosint.exe
} else {
    Write-Host "Build failed with exit code $LASTEXITCODE"
    exit $LASTEXITCODE
}