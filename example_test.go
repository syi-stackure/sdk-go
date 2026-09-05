package stackure_test

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	stackure "stackure.com/sdk-go"
)

// Auth wraps a handler so only authenticated users holding the "admin" role
// reach it. The authenticated user is available via UserFromContext.
func ExampleAuth() {
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := stackure.UserFromContext(r.Context())
		fmt.Fprintf(w, "hello %s", user.UserEmail)
	})

	http.Handle("/admin", stackure.Auth("my-app-id", "admin")(protected))
}

// Verify checks a request's session without middleware, letting the caller
// decide how to handle failure.
func ExampleVerify() {
	http.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		result := stackure.Verify("my-app-id", r, "admin")
		if !result.Authenticated {
			http.Error(w, result.Error.Message, result.Error.Code)
			return
		}
		fmt.Fprintf(w, "hello %s", result.User.UserEmail)
	})
}

// UserFromContext reads the user stored by the Auth middleware.
func ExampleUserFromContext() {
	http.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		user := stackure.UserFromContext(r.Context())
		if user == nil {
			http.Error(w, "not signed in", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, "%s %s <%s>", user.UserFirstName, user.UserLastName, user.UserEmail)
	})
}

// SendMagicLink emails a passwordless sign-in link to the user.
func ExampleSendMagicLink() {
	resp, err := stackure.SendMagicLink("user@example.com", "my-app-id")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Message)
}

// Logout revokes the session carried by the request's cookies.
func ExampleLogout() {
	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := stackure.Logout(r.Cookies()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, "signed out")
	})
}

// Every SDK function returns *StackureError; branch on Code to react to each
// category.
func ExampleStackureError() {
	_, err := stackure.SendMagicLink("not-an-email")
	var se *stackure.StackureError
	if errors.As(err, &se) {
		switch se.Code {
		case "validation":
			fmt.Println("bad input:", se.Message)
		case "auth", "forbidden":
			fmt.Println("access denied")
		case "timeout", "network":
			fmt.Println("try again later")
		}
	}
	// Output: bad input: invalid email format
}
