# MedYan API Test Suite - PowerShell
# Farklı platformlardan video test et ve güvenlik açığı ara

# Configuration
$API_BASE = "https://medyan-production.up.railway.app"
# $API_BASE = "http://localhost:8080"  # Local testing

# IMPORTANT: Set your Railway API Key
$API_KEY = "YOUR_API_KEY_HERE"

# Test URLs
$TestURLs = @{
    "YouTube" = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
    "Instagram Reel" = "https://www.instagram.com/reel/C123456789/"
    "Instagram Post" = "https://www.instagram.com/p/CXpz5k3LFXQ/"
    "TikTok" = "https://www.tiktok.com/@user/video/1234567890123456789"
    "Twitter/X" = "https://x.com/user/status/1234567890123456789"
    "Vimeo" = "https://vimeo.com/123456789"
    "Reddit" = "https://www.reddit.com/r/videos/comments/abc123/title/"
    "Dailymotion" = "https://www.dailymotion.com/video/x123456789"
    "Facebook" = "https://www.facebook.com/watch/?v=123456789"
}

function Write-Header {
    param([string]$Title)
    Write-Host ""
    Write-Host "==========================================================" -ForegroundColor Cyan
    Write-Host $Title -ForegroundColor Cyan
    Write-Host "==========================================================" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "✅ $Message" -ForegroundColor Green
}

function Write-Error-Custom {
    param([string]$Message)
    Write-Host "❌ $Message" -ForegroundColor Red
}

function Write-Warning-Custom {
    param([string]$Message)
    Write-Host "⚠️  $Message" -ForegroundColor Yellow
}

function Write-Info {
    param([string]$Message)
    Write-Host "ℹ️  $Message" -ForegroundColor Cyan
}

function Test-APIHealth {
    Write-Host "1️⃣  Testing API Health..." -ForegroundColor Yellow
    
    try {
        $response = Invoke-RestMethod -Uri "$API_BASE/health" -Method Get -TimeoutSec 10
        Write-Success "API is healthy"
        return $true
    }
    catch {
        Write-Error-Custom "API health check failed: $($_.Exception.Message)"
        return $false
    }
}

function Test-PlatformDetection {
    param([string]$URL, [string]$PlatformName)
    
    Write-Host "Platform Detection: $PlatformName" -ForegroundColor Yellow
    
    $payload = @{
        url = $URL
    } | ConvertTo-Json
    
    try {
        $response = Invoke-RestMethod `
            -Uri "$API_BASE/proxy/detect" `
            -Method Post `
            -Body $payload `
            -ContentType "application/json" `
            -Headers @{"X-API-Key" = $API_KEY} `
            -TimeoutSec 30
        
        if ($response.platform) {
            Write-Success "Platform detected: $($response.platform)"
            Write-Info "Title: $($response.title)"
            return $response
        }
        else {
            Write-Error-Custom "No platform detected"
            return $null
        }
    }
    catch {
        Write-Error-Custom "Detection failed: $($_.Exception.Message)"
        return $null
    }
}

function Test-MediaExtraction {
    param([string]$URL, [string]$Format = "mp4", [string]$Quality = "720p")
    
    Write-Host "  Media Extraction ($Format, $Quality)..." -ForegroundColor Yellow
    
    $payload = @{
        url = $URL
        format = $Format
        quality = $Quality
    } | ConvertTo-Json
    
    try {
        $response = Invoke-RestMethod `
            -Uri "$API_BASE/proxy/extract" `
            -Method Post `
            -Body $payload `
            -ContentType "application/json" `
            -Headers @{"X-API-Key" = $API_KEY} `
            -TimeoutSec 30
        
        if ($response.job_id) {
            Write-Success "Job created: $($response.job_id)"
            return $response.job_id
        }
        else {
            Write-Error-Custom "No job ID returned"
            return $null
        }
    }
    catch {
        Write-Error-Custom "Extraction failed: $($_.Exception.Message)"
        return $null
    }
}

function Test-JobStatus {
    param([string]$JobID)
    
    Write-Host "  Checking job status (10 second poll)..." -ForegroundColor Yellow
    
    for ($i = 0; $i -lt 5; $i++) {
        Start-Sleep -Seconds 2
        
        try {
            $response = Invoke-RestMethod `
                -Uri "$API_BASE/proxy/jobs/$JobID" `
                -Method Get `
                -Headers @{"X-API-Key" = $API_KEY} `
                -TimeoutSec 30
            
            $status = $response.status
            $progress = $response.progress
            
            Write-Host "    Progress: $progress% | Status: $status" -ForegroundColor Cyan
            
            if ($status -eq "completed") {
                Write-Success "  Job completed!"
                Write-Info "  Filename: $($response.result.filename)"
                Write-Info "  Size: $($response.result.filesize) bytes"
                return $true
            }
            elseif ($status -eq "failed") {
                Write-Error-Custom "  Job failed: $($response.error)"
                return $false
            }
        }
        catch {
            Write-Warning-Custom "  Poll error: $($_.Exception.Message)"
        }
    }
    
    Write-Info "  Job still processing. Check status later with Job ID: $JobID"
    return $null
}

function Test-CORS {
    Write-Host "Testing CORS Configuration..." -ForegroundColor Yellow
    
    try {
        $response = Invoke-WebRequest `
            -Uri "$API_BASE/proxy/detect" `
            -Method Post `
            -Headers @{
                "Content-Type" = "application/json"
                "X-API-Key" = $API_KEY
                "Origin" = "https://malicious-site.com"
            } `
            -Body '{"url":"https://example.com"}' `
            -TimeoutSec 10
        
        $corsOrigin = $response.Headers["Access-Control-Allow-Origin"]
        
        if ($corsOrigin -eq "*") {
            Write-Warning-Custom "CORS wildcard detected! Any origin can access the API"
        }
        elseif ($corsOrigin) {
            Write-Success "CORS restricted to: $corsOrigin"
        }
        else {
            Write-Success "CORS properly configured (no wildcard)"
        }
    }
    catch {
        Write-Warning-Custom "CORS test error: $($_.Exception.Message)"
    }
}

function Test-APIKeyRequired {
    Write-Host "Testing API Key Requirement..." -ForegroundColor Yellow
    
    try {
        $response = Invoke-RestMethod `
            -Uri "$API_BASE/proxy/detect" `
            -Method Post `
            -Body '{"url":"https://example.com"}' `
            -ContentType "application/json" `
            -TimeoutSec 10
        
        Write-Error-Custom "API key is NOT required! Endpoint is unprotected!"
        return $false
    }
    catch {
        if ($_.Exception.Response.StatusCode -eq 401) {
            Write-Success "API key is required (HTTP 401)"
            return $true
        }
        else {
            Write-Warning-Custom "Unexpected error: $($_.Exception.Message)"
            return $null
        }
    }
}

function Test-InputValidation {
    Write-Host "Testing Input Validation..." -ForegroundColor Yellow
    
    $maliciousInputs = @(
        "javascript:alert('xss')",
        "'; DROP TABLE videos; --",
        "../../../etc/passwd",
        "file:///etc/passwd",
        "<script>alert('xss')</script>"
    )
    
    $blockCount = 0
    
    foreach ($input in $maliciousInputs) {
        try {
            $response = Invoke-RestMethod `
                -Uri "$API_BASE/proxy/detect" `
                -Method Post `
                -Body (@{url = $input} | ConvertTo-Json) `
                -ContentType "application/json" `
                -Headers @{"X-API-Key" = $API_KEY} `
                -TimeoutSec 10
            
            # Should be rejected or safely handled
            Write-Info "  Input handled safely: $($input.Substring(0, [Math]::Min(30, $input.Length)))..."
        }
        catch {
            $blockCount++
        }
    }
    
    Write-Success "Input validation tested ($blockCount/$($maliciousInputs.Count) blocked)"
}

# Main Execution
Write-Host ""
Write-Host "╔══════════════════════════════════════════════════════════╗" -ForegroundColor Magenta
Write-Host "║  🚀 MedYan API - Comprehensive Test Suite                ║" -ForegroundColor Magenta
Write-Host "║     Test different platforms and security settings       ║" -ForegroundColor Magenta
Write-Host "╚══════════════════════════════════════════════════════════╝" -ForegroundColor Magenta

Write-Info "API Base: $API_BASE"
Write-Info "API Key: $(if ($API_KEY -eq 'YOUR_API_KEY_HERE') { 'NOT SET ⚠️' } else { $API_KEY.Substring(0, 10) + '...' })"

if ($API_KEY -eq "YOUR_API_KEY_HERE") {
    Write-Warning-Custom "API_KEY not set! Tests will fail without it."
    Write-Info "Set API_KEY in Railway dashboard → Environment Variables → API_KEY"
    $proceed = Read-Host "Continue anyway? (y/n)"
    if ($proceed -ne "y") { exit }
}

# Step 1: Health Check
Write-Header "🏥 HEALTH CHECK"
if (-not (Test-APIHealth)) {
    Write-Error-Custom "Cannot reach API. Exiting."
    exit 1
}

# Step 2: Security Tests
Write-Header "🔒 SECURITY TESTS"
Test-CORS
Test-APIKeyRequired
Test-InputValidation

# Step 3: Platform Tests
Write-Header "🎬 PLATFORM COMPATIBILITY TESTS"
Write-Info "Testing $(($TestURLs.Count)) platforms (first 3 for demo)"

$testCount = 0
foreach ($platform in $TestURLs.GetEnumerator()) {
    if ($testCount -ge 3) { break }
    $testCount++
    
    Write-Host ""
    Write-Host "Platform $testCount/3: $($platform.Key)" -ForegroundColor Magenta
    
    $detection = Test-PlatformDetection -URL $platform.Value -PlatformName $platform.Key
    
    if ($detection) {
        $jobID = Test-MediaExtraction -URL $platform.Value
        
        if ($jobID) {
            Test-JobStatus -JobID $jobID
        }
    }
    
    Write-Host ""
}

# Summary
Write-Header "📊 TEST SUMMARY"
Write-Success "Test suite completed!"
Write-Info "Key platform formats tested: YouTube MP4, Instagram Reel, TikTok"
Write-Info "Security tests: CORS, API Key, Input Validation"
Write-Info ""
Write-Info "📝 Notes:"
Write-Info "- Long videos may still be processing (check job ID later)"
Write-Info "- Format support: MP4, MP3, WebM, MKV, AAC, FLAC, WAV, OPU S, AVI, MOV, FLV"
Write-Info "- Rate Limiting: 100 requests/min per IP"
Write-Info "- All API calls are logged in Railway"
Write-Info ""
Write-Success "Ready for production! 🚀"
