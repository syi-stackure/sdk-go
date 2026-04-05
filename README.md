# Stackure Go SDK

Authentication for your app. One line. Zero dependencies.

## Install

```bash
go get github.com/stackure/stackure-go
```

## Protect a Route

```go
import "github.com/stackure/stackure-go"

http.Handle("/admin", stackure.Auth("my-app-id", stackure.Roles("admin"))(handler))
```

Access the user in your handler:

```go
user := stackure.UserFromContext(r.Context())
fmt.Println(user.UserEmail, user.UserRoles)
```

- API requests get JSON errors
- Browser requests get redirected to sign-in

## Verify Manually

```go
result := stackure.Verify("my-app-id", r)

if !result.Authenticated {
    // result.Error.Code, result.Error.Message, result.Error.SignInURL
}

// result.User
```

## Client Functions

```go
resp, err := stackure.SendMagicLink("user@example.com", "my-app-id")
resp, err := stackure.SignIn("my-app-id", "user@example.com")

session, err := stackure.ValidateSession("my-app-id", r.Cookies())
// session.Authenticated, session.User, session.SignInURL

err := stackure.Logout(r.Cookies())
```

## Custom Client

```go
client := stackure.NewClient(stackure.Config{
    BaseURL: "https://staging.stackure.com",
    Timeout: 5 * time.Second,
})
```

## Errors

`ValidationError` | `NetworkError` | `AuthenticationError` | `TimeoutError` | `ForbiddenError`

## License

MIT
