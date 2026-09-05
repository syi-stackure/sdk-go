package stackure_test

import (
	"errors"
	"fmt"
	"net/http"

	stackure "stackure.com/sdk-go"
)

const appID = "7f3c1a2e-9b4d-4e6f-8a1b-2c3d4e5f6071"

func ExampleAuth() {
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := stackure.UserFromContext(r.Context())
		fmt.Fprintf(w, "hello %s", user.UserEmail)
	})

	http.Handle("/admin", stackure.Auth(appID, "view_any_app")(protected))
}

func ExampleVerify() {
	http.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		result := stackure.Verify(appID, r, "view_any_app")
		if !result.Authenticated {
			http.Error(w, result.Error.Message, result.Error.Code)
			return
		}
		fmt.Fprintf(w, "hello %s", result.User.UserEmail)
	})
}

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

func ExampleSendMagicLink() {
	resp, err := stackure.SendMagicLink("user@example.com", appID)
	if err != nil {
		return
	}
	fmt.Println(resp.Message)
}

func ExampleLogout() {
	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		stackure.Logout(w, r)
	})
}

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
