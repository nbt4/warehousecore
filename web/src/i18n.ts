import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import de from './locales/de.json';
import en from './locales/en.json';
import { onSuiteLanguageChange, suiteLanguage } from './lib/cores-design';

// Initialize i18next
i18n
  .use(LanguageDetector) // Detects user language
  .use(initReactI18next) // Passes i18n down to react-i18next
  .init({
    resources: {
      de: {
        translation: de,
      },
      en: {
        translation: en,
      },
    },
    lng: suiteLanguage(),
    fallbackLng: 'de', // Default language
    supportedLngs: ['de', 'en'],
    interpolation: {
      escapeValue: false, // React already escapes
    },
    detection: {
      // Order of language detection
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      lookupLocalStorage: 'cores_language',
    },
  });

onSuiteLanguageChange((language) => {
  void i18n.changeLanguage(language);
});

export default i18n;
