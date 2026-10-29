package commands

import "github.com/lsariol/lys/internal/core"

// Access is who may use a command or one of its subcommands. The router
// checks Spec.Access and Spec.Sub before running a command; a command with
// finer rules checks req.Allowed itself.
type Access uint8

const (
	Everyone  Access = iota // the default
	VIP                     // VIPs, moderators and the channel's owner
	Moderator               // moderators and the channel's owner
	Owner                   // the channel's owner
	BotOwner                // the bot's owner, in any channel
)

// botOwnerID is the Twitch ID of the bot's owner.
const botOwnerID = "42217464"

// Allowed reports whether the sender may use something with this access.
func (req Request) Allowed(a Access) bool {
	roles := req.User.Roles
	owner := roles.Has(core.RoleBroadcaster) || (req.User.ID != "" && req.User.ID == req.Channel.ID)
	switch a {
	case Everyone:
		return true
	case VIP:
		return owner || roles.Has(core.RoleModerator) || roles.Has(core.RoleVIP)
	case Moderator:
		return owner || roles.Has(core.RoleModerator)
	case Owner:
		return owner
	case BotOwner:
		return req.Platform == "twitch" && req.User.ID == botOwnerID
	}
	return false
}

// Who names the people with this access, for help and refusals.
func (a Access) Who() string {
	switch a {
	case VIP:
		return "VIPs and moderators"
	case Moderator:
		return "moderators"
	case Owner:
		return "the channel's owner"
	case BotOwner:
		return "the bot's owner"
	}
	return "everyone"
}

// Refuse is the reply when the sender lacks access; action is what they
// tried, such as "use !ff link league".
func Refuse(a Access, action string) Reply {
	return Reply{Text: "Only " + a.Who() + " can " + action + "."}
}
