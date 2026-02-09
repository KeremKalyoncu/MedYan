// MedYan Frontend Configuration
// Copy this file to config.local.js and customize for your deployment

window.MEDYAN_CONFIG = {
  // API Configuration
  API_URL: 'https://medyan-production.up.railway.app',
  
  // GitHub Pages
  GITHUB_REPO: 'https://github.com/KeremKalyoncu/MedYan',
  
  // Feature Flags
  ENABLE_DURATION_LIMIT: true,
  MAX_VIDEO_DURATION_SECONDS: 180, // 3 minutes
  
  // UI Configuration
  THEME: 'dark', // 'dark' or 'light'
  LANGUAGE: 'tr', // 'tr' or 'en'
  
  // Analytics (optional)
  ANALYTICS_ENABLED: false,
  ANALYTICS_ID: '',
  
  // Support Links
  SUPPORT_EMAIL: 'your-email@example.com',
  GITHUB_ISSUES: 'https://github.com/KeremKalyoncu/MedYan/issues',
  
  // Debug Mode
  DEBUG: false
};
