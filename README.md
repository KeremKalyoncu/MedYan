# 📥 Media Extractor

**YouTube videolarını ve müziği kolayca indirin - Web arayüzü ile**

Free, open-source media extraction platform. Video'yu MP4 olarak veya ses'i MP3 olarak indirin. 1000+ video platformu destekleniyor.

[![Web App](https://img.shields.io/badge/Web-App-blue)](https://your-username.github.io/media-extractor)
[![API](https://img.shields.io/badge/API-Railway-9F7AEA)](https://your-api.railway.app)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

---

## 🚀 Özellikler

- ✅ **Video İndirme** - MP4 formatında, seçilebilir kalite (360p - 1080p)
- 🎵 **Müzik Çıkarma** - MP3 formatında ses dosyası
- 🌍 **1000+ Platform** - YouTube, Instagram, TikTok, vb.
- ⚡ **Hızlı İşleme** - Asenkron job queue sistemi
- 🔐 **Güvenli API** - API key authentication
- 💾 **Bulut Depolama** - S3-compatible storage
- 📊 **Job Tracking** - İndirme durumu takip

---

## 📋 Stack

| Bileşen | Teknoloji | Durum |
|---------|-----------|-------|
| **Frontend** | HTML/CSS/JS | ✅ GitHub Pages |
| **Backend API** | Go + Fiber | ✅ Railway |
| **Database** | Redis | ✅ Managed |
| **Storage** | MinIO/S3 | ✅ AWS S3 uyumlu |
| **Download Parser** | yt-dlp | ✅ ~1000+ site |
| **Media Processing** | FFmpeg | ✅ Format convert |

---

## 🎯 Hızlı Başlangıç

### **Web Uygulaması Kullan (En Kolay)**

1. [Media Extractor](https://your-username.github.io/media-extractor) sayfasını aç
2. YouTube linkini yapıştır
3. Video veya müzik seç
4. İndir!

```
https://your-username.github.io/media-extractor
```

---

## 🔧 Development Kurulum

### **Gereksinimler**
- Go 1.22+
- Redis
- FFmpeg
- yt-dlp

### **Local Deploy**

```bash
# 1. Repo klonla
git clone https://github.com/YOUR_USERNAME/media-extractor.git
cd media-extractor

# 2. Dependencies yükle
go mod download

# 3. .env dosyası oluştur
cp .env.example .env

# 4. Backend başlat
go run cmd/api/main.go

# 5. Web sitesini aç
open web/public/index.html
```

**API Status**: `http://localhost:8080/health`

---

## 📱 API Kullanımı

### **Video İndirme İsteği**

```bash
curl -X POST https://your-api.railway.app/api/v1/extract \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://www.youtube.com/watch?v=...",
    "format": "mp4",
    "quality": "720p"
  }'
```

**Yanıt:**
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "message": "Extraction job queued successfully"
}
```

### **Job Durumunu Kontrol Et**

```bash
curl https://your-api.railway.app/api/v1/jobs/{job_id} \
  -H "X-API-Key: YOUR_API_KEY"
```

**Yanıt:**
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "progress": 100,
  "result": {
    "filename": "video.mp4",
    "size_bytes": 52428800,
    "format": "mp4",
    "download_url": "https://s3.example.com/abc123...signed-url"
  }
}
```

---

## 📦 Proje Yapısı

```
media-extractor/
├── cmd/                          # Executable'lar
│   ├── api/main.go              # Web API server
│   └── worker/main.go           # Job worker
├── internal/                      # Paketler
│   ├── config/                  # Konfigürasyon yönetimi
│   ├── middleware/              # Auth, validation
│   ├── queue/                   # Job queue (Redis)
│   ├── storage/                 # S3 storage
│   ├── extractor/               # yt-dlp wrapper
│   └── types/                   # Data types
├── web/
│   └── public/
│       └── index.html           # Frontend sitesi
├── .env.example                 # Environment template
├── docker-compose.yml           # Development stack
├── Dockerfile                   # Production image
└── go.mod, go.sum              # Dependencies
```

---

## ☁️ Production Deploy

### **Railway.app (Recommended)**

```bash
# 1. Railway'e signup et (railway.app)
# 2. GitHub repo bağla
# 3. Deploy! (otomatik)

# URL: https://your-app.railway.app
```

### **Environment Variables**

```bash
API_KEY=your-secret-key-here
REDIS_ADDR=redis:6379
S3_ENDPOINT=https://s3.amazonaws.com
S3_BUCKET=your-bucket-name
AWS_ACCESS_KEY_ID=xxx
AWS_SECRET_ACCESS_KEY=xxx
YTDLP_PATH=/usr/local/bin/yt-dlp
FFMPEG_PATH=/usr/local/bin/ffmpeg
```

---

## 🔒 Güvenlik

- ✅ **API Key Authentication** - X-API-Key header
- ✅ **HTTPS Only** - Railway/GitHub Pages TLS
- ✅ **Environment Secrets** - Railway secrets management
- ✅ **Input Validation** - URL ve format kontrol
- ✅ **Rate Limiting** - (Cloudflare/Railway tarafından)

---

## 🌐 Platform Desteği

yt-dlp tarafından desteklenen 1000+ platform:

- ✅ YouTube
- ✅ Instagram
- ✅ TikTok
- ✅ Vimeo
- ✅ Dailymotion
- ✅ Twitch
- ✅ ve daha fazlası...

[Tüm desteklenen siteleri gör](https://github.com/yt-dlp/yt-dlp/blob/master/README.md#supported-sites)

---

## 📝 API Endpoints

| Method | Endpoint | Açıklama |
|--------|----------|----------|
| `GET` | `/health` | Health check |
| `POST` | `/api/v1/extract` | Video/müzik indirme isteği |
| `GET` | `/api/v1/jobs/:id` | Job durumunu kontrol et |
| `GET` | `/api/v1/download/:id` | S3 presigned URL'ye yönlendir |

---

## 🐛 Troubleshooting

### "Invalid API Key"
```
ÇÖZÜM: X-API-Key header'ı kontrol et veya api_key query parametresi ekle
```

### "Job not found"
```
ÇÖZÜM: Job ID'nin doğru olduğundan emin ol. 4 dakika sonra expire olur.
```

### "FFmpeg not found"
```
ÇÖZÜM: FFmpeg yükle:
  macOS: brew install ffmpeg
  Ubuntu: apt-get install ffmpeg
  Windows: choco install ffmpeg
```

---

## 📄 Lisans

[MIT License](LICENSE)

---

## 👤 İletişim

Sorularınız, önerileriniz veya katkılarınız için issue açın!

**Kontrol:** [GitHub Discussions](https://github.com/YOUR_USERNAME/media-extractor/discussions)

---

## 🙌 Katkıda Bulun

Bu proje açık kaynaktır! Katkılarınızı bekliyoruz.

1. Fork et
2. Branch oluştur (`git checkout -b feature/amazing-feature`)
3. Commit et (`git commit -m 'Add amazing feature'`)
4. Push et (`git push origin feature/amazing-feature`)
5. Pull Request aç

---

<div align="center">

**Made with ❤️ for easy media extraction**

⭐ Bu projeyi beğendiysen yıldız ver!

</div>
