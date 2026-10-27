package commands

import "github.com/lsariol/lys/internal/core"

// botOwnerID is the Twitch ID of the bot's owner, who may run any channel's
// owner-only commands.
const botOwnerID = "42217464"

// HasPrivilege reports whether the sender owns the channel (the broadcaster
// role, or the channel's own ID as on Twitch) or the bot.
func HasPrivilege(req Request) bool {
	return req.User.Roles.Has(core.RoleBroadcaster) || req.User.ID == req.Channel.ID || IsBotOwner(req)
}

// IsBotOwner reports whether the sender is the bot's owner, by ID (names can change).
func IsBotOwner(req Request) bool {
	return req.Platform == "twitch" && req.User.ID == botOwnerID
}
