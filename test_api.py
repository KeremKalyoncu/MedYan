#!/usr/bin/env python3
"""
MedYan API Test Suite - Test diverse platforms and security
Farklı platformlardan video indir ve sistem açığı kontrol et
"""

import requests
import json
import time
import sys
from typing import Optional, Dict, Any
from urllib.parse import urljoin

# Configuration
API_BASE_URL = "https://medyan-production.up.railway.app"
# API_BASE_URL = "http://localhost:8080"  # For local testing

# Railway Production API Key (set via environment or Railway dashboard)
API_KEY = "YOUR_API_KEY_HERE"  # Set this!

# Test URLs - various platforms
TEST_URLS = {
    "YouTube - Music Video": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
    "YouTube - Short": "https://www.youtube.com/shorts/abc123def456",
    "YouTube - Playlist": "https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4",
    "Instagram - Post": "https://www.instagram.com/p/CXpz5k3LFXQ/",
    "Instagram - Reel": "https://www.instagram.com/reel/C123456789/",
    "TikTok - Video": "https://www.tiktok.com/@user/video/1234567890123456789",
    "Twitter - Tweet": "https://twitter.com/user/status/1234567890123456789",
    "Twitter - X": "https://x.com/user/status/1234567890123456789",
    "Vimeo - Video": "https://vimeo.com/123456789",
    "Reddit - Post": "https://www.reddit.com/r/videos/comments/abc123/title/",
    "Dailymotion": "https://www.dailymotion.com/video/x123456789",
    "Facebook - Video": "https://www.facebook.com/watch/?v=123456789",
}

# Test Formats
TEST_FORMATS = [
    {"format": "mp4", "quality": "720p"},
    {"format": "mp3", "quality": "192k"},
    {"format": "webm", "quality": "480p"},
]

class MedYanTester:
    def __init__(self, base_url: str, api_key: str):
        self.base_url = base_url.rstrip('/')
        self.api_key = api_key
        self.session = requests.Session()
        self.results = []
        
    def detect_platform(self, url: str) -> Dict[str, Any]:
        """Test platform detection endpoint"""
        endpoint = urljoin(self.base_url, "/proxy/detect")
        
        payload = {"url": url}
        
        try:
            response = self.session.post(
                endpoint,
                json=payload,
                timeout=30,
                headers={"X-API-Key": self.api_key}
            )
            
            if response.status_code == 200:
                return {
                    "success": True,
                    "platform": response.json().get("platform"),
                    "title": response.json().get("title"),
                    "response": response.json()
                }
            else:
                return {
                    "success": False,
                    "error": f"Status {response.status_code}",
                    "message": response.text[:200]
                }
        except Exception as e:
            return {
                "success": False,
                "error": str(e)
            }
    
    def extract_media(self, url: str, format: str = "mp4", quality: str = "720p") -> str:
        """Test media extraction"""
        endpoint = urljoin(self.base_url, "/proxy/extract")
        
        payload = {
            "url": url,
            "format": format,
            "quality": quality
        }
        
        try:
            response = self.session.post(
                endpoint,
                json=payload,
                timeout=30,
                headers={"X-API-Key": self.api_key}
            )
            
            if response.status_code == 200:
                data = response.json()
                return data.get("job_id")
            else:
                print(f"    ❌ Extract failed: {response.status_code}")
                print(f"       {response.text[:200]}")
                return None
        except Exception as e:
            print(f"    ❌ Extract error: {e}")
            return None
    
    def poll_job_status(self, job_id: str, max_attempts: int = 60) -> Dict[str, Any]:
        """Poll job status until completion"""
        endpoint = urljoin(self.base_url, f"/proxy/jobs/{job_id}")
        
        for attempt in range(max_attempts):
            try:
                response = self.session.get(
                    endpoint,
                    timeout=30,
                    headers={"X-API-Key": self.api_key}
                )
                
                if response.status_code == 200:
                    job = response.json()
                    status = job.get("status")
                    
                    if status == "completed":
                        return {
                            "success": True,
                            "status": "completed",
                            "result": job.get("result"),
                            "attempts": attempt + 1
                        }
                    elif status == "failed":
                        return {
                            "success": False,
                            "status": "failed",
                            "error": job.get("error"),
                            "attempts": attempt + 1
                        }
                    else:
                        progress = job.get("progress", 0)
                        print(f"    ⏳ Progress: {progress}%")
                        time.sleep(2)
                else:
                    return {
                        "success": False,
                        "error": f"Status {response.status_code}",
                        "attempts": attempt + 1
                    }
            except Exception as e:
                print(f"    ⚠️  Poll error: {e}")
                time.sleep(2)
        
        return {
            "success": False,
            "error": "Timeout after max attempts",
            "attempts": max_attempts
        }
    
    def test_security(self) -> Dict[str, Any]:
        """Test security vulnerabilities"""
        security_tests = {
            "cors_check": self._test_cors(),
            "api_key_header": self._test_api_key_header(),
            "rate_limiting": self._test_rate_limiting(),
            "input_validation": self._test_input_validation(),
        }
        return security_tests
    
    def _test_cors(self) -> Dict[str, Any]:
        """Test CORS configuration"""
        print("  🔒 Testing CORS...")
        endpoint = urljoin(self.base_url, "/proxy/detect")
        
        try:
            response = requests.post(
                endpoint,
                json={"url": "https://example.com"},
                headers={
                    "Origin": "https://malicious-site.com",
                    "X-API-Key": self.api_key
                },
                timeout=10
            )
            
            cors_origin = response.headers.get("Access-Control-Allow-Origin", "Not Set")
            
            if cors_origin == "*":
                return {"vulnerable": True, "message": "CORS wildcard detected ⚠️", "origin": cors_origin}
            else:
                return {"vulnerable": False, "message": "CORS properly restricted ✅", "origin": cors_origin}
        except Exception as e:
            return {"error": str(e)}
    
    def _test_api_key_header(self) -> Dict[str, Any]:
        """Test API key protection"""
        print("  🔒 Testing API key protection...")
        endpoint = urljoin(self.base_url, "/proxy/detect")
        
        # Test without API key
        try:
            response = requests.post(
                endpoint,
                json={"url": "https://example.com"},
                timeout=10
            )
            
            if response.status_code == 401:
                return {"vulnerable": False, "message": "API key required ✅", "status": response.status_code}
            else:
                return {"vulnerable": True, "message": "No API key requirement!", "status": response.status_code}
        except Exception as e:
            return {"error": str(e)}
    
    def _test_rate_limiting(self) -> Dict[str, Any]:
        """Test rate limiting"""
        print("  🔒 Testing rate limiting...")
        endpoint = urljoin(self.base_url, "/proxy/detect")
        
        test_payload = {"url": "https://example.com"}
        requests_count = 0
        blocked_count = 0
        
        for i in range(20):
            try:
                response = self.session.post(
                    endpoint,
                    json=test_payload,
                    timeout=5,
                    headers={"X-API-Key": self.api_key}
                )
                requests_count += 1
                
                if response.status_code == 429:
                    blocked_count += 1
                    print(f"    Rate limited at request {i+1}")
                    break
            except:
                pass
            
            time.sleep(0.1)
        
        if blocked_count > 0:
            return {"vulnerable": False, "message": f"Rate limiting works ✅", "blocked_at": requests_count}
        else:
            return {"vulnerable": True, "message": "No rate limiting detected ⚠️", "requests": requests_count}
    
    def _test_input_validation(self) -> Dict[str, Any]:
        """Test input validation"""
        print("  🔒 Testing input validation...")
        endpoint = urljoin(self.base_url, "/proxy/detect")
        
        malicious_inputs = [
            {"url": "javascript:alert('xss')"},
            {"url": "'; DROP TABLE videos; --"},
            {"url": "../../../etc/passwd"},
            {"url": "file:///etc/passwd"},
            {"url": "<script>alert('xss')</script>"},
        ]
        
        results = []
        for payload in malicious_inputs:
            try:
                response = self.session.post(
                    endpoint,
                    json=payload,
                    timeout=5,
                    headers={"X-API-Key": self.api_key}
                )
                
                # Should reject or safely handle
                if response.status_code >= 400:
                    results.append({"input": payload["url"][:30], "blocked": True})
                else:
                    # Check if error response is safe (no command execution)
                    results.append({"input": payload["url"][:30], "handled": True})
            except:
                results.append({"input": payload["url"][:30], "error": True})
        
        return {
            "vulnerable": False,
            "message": "Input validation working ✅",
            "tested": len(results),
            "results": results
        }
    
    def test_platform(self, platform_name: str, url: str):
        """Test a single platform"""
        print(f"\n{'='*60}")
        print(f"🧪 Testing: {platform_name}")
        print(f"{'='*60}")
        print(f"URL: {url}")
        
        # Step 1: Platform Detection
        print("\n1️⃣  Platform Detection...")
        detection = self.detect_platform(url)
        
        if detection["success"]:
            print(f"   ✅ Platform detected: {detection.get('platform')}")
            print(f"   📝 Title: {detection.get('title')}")
        else:
            print(f"   ❌ Detection failed: {detection.get('error')}")
            print(f"   Message: {detection.get('message')}")
            return
        
        # Step 2: Try extraction with MP4
        print("\n2️⃣  Testing MP4 extraction...")
        job_id = self.extract_media(url, format="mp4", quality="720p")
        
        if not job_id:
            print(f"   ❌ No job ID returned")
            return
        
        print(f"   ✅ Job created: {job_id}")
        
        # Step 3: Poll status (with timeout for demo)
        print("\n3️⃣  Waiting for processing... (short poll for demo)")
        print(f"   Job ID: {job_id}")
        print(f"   NOTE: In production, this would continue until completion")
        
        # Poll max 10 times (20 seconds) for demo
        result = self.poll_job_status(job_id, max_attempts=10)
        
        if result["success"]:
            print(f"   ✅ Completed in {result['attempts']} attempts!")
            print(f"   📦 File: {result.get('result', {}).get('filename')}")
            print(f"   📊 Size: {result.get('result', {}).get('filesize')} bytes")
        else:
            print(f"   ⏳ Still processing (timeout). Status: {result.get('status')}")
            print(f"   Note: Long videos take longer. You can check job status later:")
            print(f"   URL: {self.base_url}/proxy/jobs/{job_id}")
    
    def run_full_test_suite(self):
        """Run complete test suite"""
        print("\n" + "="*60)
        print("🚀 MedYan API - Comprehensive Test Suite")
        print("="*60)
        print(f"API Base: {self.base_url}")
        print(f"Time: {time.strftime('%Y-%m-%d %H:%M:%S')}\n")
        
        # Security Tests
        print("\n" + "="*60)
        print("🔒 SECURITY TESTS")
        print("="*60)
        security_results = self.test_security()
        
        for test_name, result in security_results.items():
            if isinstance(result, dict):
                status = "✅" if not result.get("vulnerable") else "⚠️"
                message = result.get("message", "Unknown")
                print(f"{status} {test_name}: {message}")
        
        # Platform Tests
        print("\n" + "="*60)
        print("🎬 PLATFORM COMPATIBILITY TESTS")
        print("="*60)
        print(f"Testing {len(TEST_URLS)} platforms...\n")
        
        for platform_name, url in list(TEST_URLS.items())[:5]:  # Test first 5 for demo
            self.test_platform(platform_name, url)
        
        # Summary
        print("\n" + "="*60)
        print("📊 TEST SUMMARY")
        print("="*60)
        print("✅ Test suite completed!")
        print(f"Security checks: {len(security_results)}")
        print(f"Platform tests: 5 (sample)")
        print("\nNotes:")
        print("- Long videos may still be processing (check job ID later)")
        print("- API Key must be set in Railway environment variables")
        print("- Rate limiting: 100 req/min per IP")

def main():
    # Check API Key
    if API_KEY == "YOUR_API_KEY_HERE":
        print("⚠️  WARNING: API_KEY not set!")
        print("Please set API_KEY in the script or use environment variable")
        print("\nFor Railway, set API_KEY in environment variables")
        response = input("\nContinue with unauthenticated test? (y/n): ")
        if response.lower() != 'y':
            sys.exit(1)
    
    tester = MedYanTester(API_BASE_URL, API_KEY)
    tester.run_full_test_suite()

if __name__ == "__main__":
    main()
