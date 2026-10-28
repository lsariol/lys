package auth

import "fmt"

// storeUserTokens saves the bot's user token pair to Cove: the refresh token
// first, and only when it changed.
func (c *AuthClient) storeUserTokens(access, refresh string) error {

	if refresh != c.storedRefresh {
		if err := c.Cove.UpdateSecret(c.Config.RefreshTokenKey, refresh); err != nil {
			return fmt.Errorf("Cove: update %s: %w (the lys Cove token needs write access to it)", c.Config.RefreshTokenKey, err)
		}
		c.storedRefresh = refresh
	}

	if err := c.Cove.UpdateSecret(c.Config.AccessTokenKey, access); err != nil {
		return fmt.Errorf("Cove: update %s: %w (the lys Cove token needs write access to it)", c.Config.AccessTokenKey, err)
	}

	return nil
}
