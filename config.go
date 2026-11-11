package stackure

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie  = "stackure_session"
	platformCookie = "session"
	codeCookie     = "stackure_code"
	verifierCookie = "stackure_verifier"
	tokenParam     = "session_token"
	codeParam      = "stackure_code"
	sessionMaxAge  = 7 * 24 * 3600
	codeMaxAge     = 60
	verifierMaxAge = 15 * 60
	verifierBytes  = 32
	maxHandoffBody = 4 << 10
	requestTimeout = 2 * time.Second
	maxRetries     = 1
	retryDelay     = 500 * time.Millisecond
	maxEvents      = 100

	magicLinkPath      = "/api/public/auth/magic-link/send"
	validatePath       = "/api/public/auth/session/validate"
	exchangePath       = "/api/public/auth/session/exchange"
	directoryUsersPath = "/api/public/directory/users"
	directoryTeamsPath = "/api/public/directory/teams"
	eventsPath         = "/api/public/events"
	signOutPath        = "/api/public/auth/sign-out"
	signInPage         = "/sign-in/magic-link"
	signOutPage        = "/signout"
	homePage           = "/"

	invalidBaseURL = "invalid STACKURE_BASE_URL format (must be an https URL, or http on localhost or 127.0.0.1)"
)

type environment struct {
	baseURL, baseURLErr  string
	origin               string
	appID, appIDErr      string
	appSecret, secretErr string
}

var loadEnvironment = sync.OnceValue(func() environment {
	e := environment{
		appID:     os.Getenv("STACKURE_APP_ID"),
		appSecret: os.Getenv("STACKURE_APP_SECRET"),
	}
	base := strings.TrimRight(os.Getenv("STACKURE_BASE_URL"), "/")
	u, err := url.Parse(base)
	switch {
	case base == "":
		e.baseURLErr = "STACKURE_BASE_URL is not set"
	case err != nil || strings.ContainsAny(base, "?#") || u.Hostname() == "" || !validPort(u.Port()) || !(u.Scheme == "https" || (u.Scheme == "http" && isLoopbackHost(u.Host))):
		e.baseURLErr = invalidBaseURL
	default:
		e.baseURL, e.origin = base, u.Scheme+"://"+strings.ToLower(u.Host)
	}
	switch {
	case e.appID == "":
		e.appIDErr = "STACKURE_APP_ID is not set"
	case !uuidRegex.MatchString(e.appID):
		e.appIDErr = "invalid STACKURE_APP_ID format (must be a valid UUID)"
	}
	if e.appSecret == "" {
		e.secretErr = "STACKURE_APP_SECRET is not set"
	}
	return e
})

func validPort(p string) bool {
	n, err := strconv.Atoi(p)
	return p == "" || (err == nil && n <= 65535)
}

func baseURL() string {
	return loadEnvironment().baseURL
}

func requiredBaseURL() (string, error) {
	e := loadEnvironment()
	if e.baseURLErr != "" {
		return "", newErr("validation", 0, e.baseURLErr)
	}
	return e.baseURL, nil
}

func origin() string {
	return loadEnvironment().origin
}

func appID() (string, error) {
	e := loadEnvironment()
	if e.appIDErr != "" {
		return "", newErr("validation", 0, e.appIDErr)
	}
	return e.appID, nil
}

func appSecret() (string, error) {
	e := loadEnvironment()
	if e.secretErr != "" {
		return "", newErr("validation", 0, e.secretErr)
	}
	return e.appSecret, nil
}
