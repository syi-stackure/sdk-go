package stackure_test

import (
	"errors"
	"fmt"
	"net/http"

	stackure "stackure.com/sdk-go"
)

func ExampleAuth() {
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := stackure.UserFromContext(r.Context())
		fmt.Fprintf(w, "hello %s", user.UserEmail)
	})

	http.Handle("/admin", stackure.Auth("can_approve_invoice")(protected))
}

func ExampleMCP() {
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := stackure.UserFromContext(r.Context())
		fmt.Fprintf(w, "hello %s", user.UserEmail)
	})

	http.Handle("/mcp", stackure.MCP("can_approve_invoice")(server))
}

func ExampleVerify() {
	http.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		result := stackure.Verify(r, "can_approve_invoice")
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
	resp, err := stackure.SendMagicLink("user@example.com")
	if err != nil {
		return
	}
	fmt.Println(resp.Message)
}

// Mount Logout for every method and trigger it with a form or button that
// POSTs from the app's own page. A link or any other request is sent to
// Stackure's sign-out page, where the user confirms.
func ExampleLogout() {
	http.HandleFunc("/logout", stackure.Logout)
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
