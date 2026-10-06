package commands

import "github.com/lsariol/lys/internal/adapters/adapter"

func SuppressedReply() (adapter.Response, error) {
	return adapter.Response{SuppressReply: true, Error: true}, nil
}

func HasPrivilege(userID string, channelID string) bool {

	if userID == channelID || userID == "42217464" {
		return true
	}

	return false
}
