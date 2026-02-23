package cmd

type ChatCmd struct {
	Spaces               ChatSpacesCmd               `cmd:"" name:"spaces" help:"Chat spaces"`
	Messages             ChatMessagesCmd             `cmd:"" name:"messages" help:"Chat messages"`
	Threads              ChatThreadsCmd              `cmd:"" name:"threads" help:"Chat threads"`
	DM                   ChatDMCmd                   `cmd:"" name:"dm" help:"Direct messages"`
	Emoji                ChatEmojiCmd                `cmd:"" name:"emoji" help:"Custom emoji management"`
	Media                ChatMediaCmd                `cmd:"" name:"media" help:"Media upload and download"`
	Members              ChatMembersCmd              `cmd:"" name:"members" help:"Space members management"`
	Reactions            ChatReactionsCmd            `cmd:"" name:"reactions" help:"Message reactions"`
	Events               ChatEventsCmd               `cmd:"" name:"events" help:"Space events"`
	NotificationSettings ChatNotificationSettingsCmd `cmd:"" name:"notification-settings" help:"Space notification settings"`
	ThreadReadState      ChatThreadReadStateCmd      `cmd:"" name:"thread-read-state" help:"Thread read state"`
	SpaceReadState       ChatSpaceReadStateCmd       `cmd:"" name:"space-read-state" help:"Space read state"`
}
