package cmd

import "github.com/Robben-Media/gogcli/internal/googleapi"

var newCalendarService = googleapi.NewCalendar

const (
	scopeAll    = "all"
	scopeSingle = "single"
	scopeFuture = "future"
)
