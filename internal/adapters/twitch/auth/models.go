package auth

type RefreshSuccess struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"` // seconds
	Scope        []string `json:"scope"`
	TokenType    string   `json:"token_type"`
}

type RefreshFail struct {
	Error   string `json:"error"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type VerificationFail struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type VerificationSuccess struct {
	ClientId  string   `json:"client_id"`
	Login     string   `json:"login"`
	Scopes    []string `json:"scopes"`
	UserId    string   `json:"user_id"`
	ExpiresIn int      `json:"expires_in"`
}
