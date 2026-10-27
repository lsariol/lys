package commands

// botOwnerID is the Twitch ID of the bot's owner, who may run any channel's
// owner-only commands.
const botOwnerID = "42217464"

// HasPrivilege reports whether the user owns the channel or the bot.
func HasPrivilege(userID string, channelID string) bool {
	return userID == channelID || userID == botOwnerID
}
