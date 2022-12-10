package auth

type AuthenticationError struct {
	Message string
}

func (e AuthenticationError) Error() string {
	return e.Message
}
