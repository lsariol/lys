package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/adapters/twitch/eventsub"
)

func (c *TwitchClient) ingestLoop(ctx context.Context, in <-chan eventsub.EventSubMessage) error {

	fmt.Println("[TwitchBot] Running")
	for {
		select {
		case <-ctx.Done():
			log.Println("context canceled, stopped ingest loop")
			return nil

		case msg, ok := <-in:
			if !ok {
				fmt.Println("twitchClient inboundEvents channel closed")
				return nil
			}

			//Log message inbound
			c.logMessage(msg)
			go c.pack(msg)

			//Check leading words. If words are found send off

		}
	}
}

func (c *TwitchClient) pack(msg eventsub.EventSubMessage) {

	m := msg.Payload.Event.Message.Text

	channelSettings, ok := c.Settings.GetTwitchChannelSettings(msg.Payload.Event.BroadcasterUserID)
	if !ok {
		channelSettings.CommandPrefix = "!"
	}

	prefix := channelSettings.CommandPrefix

	var newEnvelope adapter.Envelope
	cmd, args, reg, ok := c.parseCMD(m, prefix)
	if !ok {
		return
	}

	rawTime, _ := time.Parse(time.RFC3339Nano, msg.Metadata.MessageTimestamp)
	newEnvelope.Platform = "twitch"
	newEnvelope.Username = msg.Payload.Event.ChatterUserName
	newEnvelope.UserID = msg.Payload.Event.ChatterUserID
	newEnvelope.ChannelName = msg.Payload.Event.BroadcasterUserName
	newEnvelope.ChannelID = msg.Payload.Event.BroadcasterUserID
	newEnvelope.Command = strings.ToLower(cmd)
	newEnvelope.Args = args
	newEnvelope.Timestamp = rawTime
	newEnvelope.Prefix = prefix
	newEnvelope.RawMessage = fmt.Sprintf("%s:%s-%s", msg.Payload.Event.BroadcasterUserName, msg.Payload.Event.ChatterUserName, msg.Payload.Event.Message.Text)
	newEnvelope.IsRegex = reg
	c.outEnvelopes <- newEnvelope
}

func (c *TwitchClient) parseCMD(msg string, prefix string) (cmd string, args []string, isRegex bool, ok bool) {

	m := strings.TrimSpace(msg)

	if strings.HasPrefix(m, prefix) {
		withoutPrefix := strings.TrimSpace(m[len(prefix):])

		fields := strings.Fields(withoutPrefix)
		if len(fields) == 0 {
			return "", nil, false, false
		}

		cmdName := strings.ToLower(fields[0])
		cmd, ok := c.CommandRegistry.PrefixMap[cmdName]
		if !ok {
			return "", nil, false, false
		}

		args := fields[1:]
		return cmd, args, false, true
	}

	for regex, cmd := range c.CommandRegistry.RegexMap {
		if matches := regex.FindStringSubmatch(msg); matches != nil {
			return cmd, matches[1:], true, true
		}
	}

	return "", nil, false, false
}

func (c *TwitchClient) logMessage(e eventsub.EventSubMessage) {

	now := timestamp()

	fmt.Printf("[%s] %-10.10s: @%-10.10s - %s\n",
		now,                                 // formatted as YYYY-MM-DD-HH-MM-SS-msms
		e.Payload.Event.BroadcasterUserName, // max width 10 chars, padded right
		e.Payload.Event.ChatterUserName,     // max width 10 chars, padded right
		e.Payload.Event.Message.Text,        // remainder of line
	)
}

func timestamp() string {

	return time.Now().Format("2006-01-02 15:04:05.00")
}
