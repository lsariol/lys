package auth

import "fmt"

// storeUserTokens saves the bot's user token pair to Cove, refresh token first.
func (c *AuthClient) storeUserTokens(access, refresh string) error {

	if err := c.Cove.UpdateSecret(c.Config.RefreshTokenKey, refresh); err != nil {
		return fmt.Errorf("Cove: update %s: %w (the lys Cove token needs write access to it)", c.Config.RefreshTokenKey, err)
	}

	if err := c.Cove.UpdateSecret(c.Config.AccessTokenKey, access); err != nil {
		return fmt.Errorf("Cove: update %s: %w (the lys Cove token needs write access to it)", c.Config.AccessTokenKey, err)
	}

	return nil
}
