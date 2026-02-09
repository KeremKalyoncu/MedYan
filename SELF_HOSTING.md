# MedYan Self-Hosting & Configuration Guide

## 🎯 Hızlı Başlangıç

Fork'lediyseniz ve kendi Railway deployment'ını kurmak istiyorsan, 5 dakika çalışır!

### Adım 1: Repository'yi Fork Et
- GitHub üzerinde "Fork" butonuna tıkla
- Kendi repo'nu klonla: `git clone https://github.com/YOUR_USERNAME/MedYan.git`

### Adım 2: .env.local Oluştur
```bash
cp .env.example .env.local
```

**Doldurman gereken alanlar:**
```env
API_KEY=guvenli-bir-anahtar-olustur
RAILWAY_API_URL=https://your-deployment.up.railway.app
REDIS_ADDR=redis-url-vertenin
S3_BUCKET=dosyalarin-depolanacagi-bucket
```

### Adım 3: Frontend Config'i Ayarla
```bash
cp docs/config.example.js docs/config.local.js
```

`docs/config.local.js` düzenle:
```javascript
window.MEDYAN_CONFIG = {
  API_URL: 'https://your-deployment.up.railway.app',
  GITHUB_REPO: 'https://github.com/YOUR_USERNAME/MedYan'
};
```

### Adım 4: Railway'e PUSH ET
```bash
git add -A
git commit -m "kişisel configurasyon"
git push origin main
```

Railway otomatik olarak deploy eder! 🚀

---

## 📚 Detaylı Konfigürasyon

### Backend Environment Variables

| Variable | Default | Açıklama |
|----------|---------|----------|
| `API_KEY` | - | ⚠️ Gerekli! API güvenliği için |
| `API_PORT` | 8080 | Server portu |
| `REDIS_ADDR` | localhost:6379 | Redis bağlantısı |
| `S3_BUCKET` | - | İndirilmiş dosyaların depolanması |
| `YTDLP_TIMEOUT` | 10m | yt-dlp işlem timeout |
| `MAX_CONCURRENT_JOBS` | 8 | Paralel job sayısı |
| `LOG_LEVEL` | info | Logging seviyesi |

### Frontend Configuration (config.local.js)

```javascript
window.MEDYAN_CONFIG = {
  // API bağlantısı
  API_URL: 'https://your-api.com',
  
  // Repo bilgileri
  GITHUB_REPO: 'https://github.com/YOUR_USER/MedYan',
  
  // Özellik kontrolü
  ENABLE_DURATION_LIMIT: true,
  MAX_VIDEO_DURATION_SECONDS: 180,
  
  // UI ayarları
  THEME: 'dark',
  LANGUAGE: 'tr',
  
  // Debug modu
  DEBUG: false
};
```

---

## 🚀 Deployment Seçenekleri

### 1. Railway (Önerilen)
- **Avantaj:** Otomatik Git deployment, basit environment setup
- **Maliyet:** Aylık kredit sistemi (genelde free tier yeterli)
- [railway.app](https://railway.app) → New Project → Kendi GitHub repo'nu seç

### 2. Docker (Self-hosted)
```bash
docker build -t medyan .
docker run -p 8080:8080 \
  -e API_KEY=your-key \
  -e REDIS_ADDR=redis:6379 \
  medyan
```

### 3. Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: medyan
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: medyan
        image: medyan:latest
        env:
        - name: API_KEY
          valueFrom:
            secretKeyRef:
              name: medyan-secrets
              key: api-key
```

### 4. Vercel / Netlify (Frontend)
Frontend'i Vercel/Netlify'ye deploy et:
```bash
# Vercel
vercel --cwd=docs

# Netlify
netlify deploy --dir=docs
```

---

## 🔐 Güvenlik İpuçları

### ⚠️ HAYATTA YAPMAMAN GEREKENLER

- ❌ `API_KEY`'i GitHub'a commit etme
- ❌ `.env.local` dosyasını repo'ya ekleme
- ❌ `config.local.js`'de real API key'leri sakla
- ❌ S3 credentials'ı hardcode etme

### ✅ İPONLA YAPILACAKLARı

- ✅ `.env.local` ve `config.local.js` → `.gitignore`'a ekle (zaten var!)
- ✅ Environment variables kullan
- ✅ Railway/Vercel dashboard'unda secrets yönet
- ✅ SSH keys ve tokens secure tutman

---

## 🐛 Sorun Giderme

### "API_URL bağlantı kuramıyor"
- Railway deployment'ında `API_URL`'i kontrol et
- Frontend'de `config.local.js` API_URL'si doğru mu?
- CORS setting'leri kontrol et

### "Video indirme başlamıyor"
- `YTDLP_PATH` çevre değişkenini kontrol et
- `ffmpeg` kurulu mu? → `ffmpeg -version`
- Timeout ayarlarını kontrol et

### "Redis bağlantı hatası"
- Redis running mi? → `redis-cli ping`
- `REDIS_ADDR`'i kontrol et (örn: `localhost:6379`)

---

## 💡 Örnek Setup: Full Self-Hosted

```bash
# 1. Clone
git clone https://github.com/YOU/MedYan.git && cd MedYan

# 2. Backend config
cat > .env.local << EOF
API_KEY=$(openssl rand -hex 16)
API_PORT=8080
REDIS_ADDR=redis://redis:6379
S3_ENDPOINT=https://your-s3.com
AWS_ACCESS_KEY_ID=your_key
AWS_SECRET_ACCESS_KEY=your_secret
EOF

# 3. Frontend config
cat > docs/config.local.js << EOF
window.MEDYAN_CONFIG = {
  API_URL: 'https://api.your-domain.com',
  DEBUG: false
};
EOF

# 4. Docker Compose ile çalıştır
docker-compose up -d

# 5. Git'e push et (secrets hariç)
git add -A
git commit -m "Setup self-hosted MedYan"
git push
```

---

## 📞 Destek

Setup konusunda yardım lazımsa:
- 📧 Email: gskerem200553@outlook.com
- 💬 GitHub Issues: [issues](https://github.com/KeremKalyoncu/MedYan/issues)
- 🔗 LinkedIn: [Kerem Kalyoncu](https://linkedin.com/in/kerem-kalyoncu-0753k)

Happy deploying! 🚀
