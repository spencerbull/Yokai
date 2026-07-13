package cloudsync

// Public deployment defaults. Firebase API keys and desktop OAuth client
// credentials identify the application; Firestore Security Rules authorize
// access. OAuth values are injected into release builds with Go linker flags.
var (
	DefaultProjectID          = "yokai-config-260709-5820"
	DefaultAPIKey             = "AIzaSyCzqb4u5v4Dp-EucLI1F3HW3UnuXzvYmeI"
	DefaultGoogleClientID     string
	DefaultGoogleClientSecret string
)
