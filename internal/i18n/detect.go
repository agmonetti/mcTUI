package i18n

// DetectSystemLanguage inspects the operating system's locale settings
// and returns "es" if Spanish is detected, otherwise defaulting to "en".
func DetectSystemLanguage() string {
	return detectPlatformLanguage()
}
