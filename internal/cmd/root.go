package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/errfmt"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/selfupdate"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	colorAuto   = "auto"
	colorNever  = "never"
	colorAlways = "always"
	boolTrue    = "true"
	boolFalse   = "false"
)

var rootHelpDescription = helpDescription

// maybeNotifyUpdate is the background update-check seam used by Execute.
// Tests override it to assert which command paths invoke the notifier.
var maybeNotifyUpdate = selfupdate.MaybeNotify

type RootFlags struct {
	Color          string `help:"Color output: auto|always|never" default:"${color}"`
	Account        string `help:"Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets)"`
	Client         string `help:"OAuth client name (selects stored credentials + token bucket)" default:"${client}"`
	EnableCommands string `help:"Comma-separated list of enabled top-level commands (restricts CLI)" default:"${enabled_commands}"`
	JSON           bool   `help:"Output JSON to stdout (best for scripting)" default:"${json}"`
	Plain          bool   `help:"Output stable, parseable text to stdout (TSV; no colors)" default:"${plain}"`
	Version        bool   `help:"Print version and exit"`
	Force          bool   `help:"Skip confirmations for destructive commands"`
	NoInput        bool   `help:"Never prompt; fail instead (useful for CI)"`
	ReadOnly       bool   `name:"readonly" help:"Require explicit exceptions for mutations" default:"${readonly}"`
	ReadOnlyExcept string `name:"readonly-except" help:"Comma-separated readonly service exemptions" default:"${readonly_except}"`
	Verbose        bool   `help:"Enable verbose logging"`
}

type CLI struct {
	RootFlags `embed:""`

	Auth            AuthCmd               `cmd:"" help:"Auth and credentials"`
	Groups          GroupsCmd             `cmd:"" help:"Google Groups"`
	Drive           DriveCmd              `cmd:"" help:"Google Drive"`
	Docs            DocsCmd               `cmd:"" help:"Google Docs (export via Drive)"`
	Slides          SlidesCmd             `cmd:"" help:"Google Slides"`
	Calendar        CalendarCmd           `cmd:"" help:"Google Calendar"`
	Classroom       ClassroomCmd          `cmd:"" help:"Google Classroom"`
	Time            TimeCmd               `cmd:"" help:"Local time utilities"`
	Gmail           GmailCmd              `cmd:"" aliases:"mail,email" help:"Gmail"`
	Chat            ChatCmd               `cmd:"" help:"Google Chat"`
	Contacts        ContactsCmd           `cmd:"" help:"Google Contacts"`
	Tasks           TasksCmd              `cmd:"" help:"Google Tasks"`
	People          PeopleCmd             `cmd:"" help:"Google People"`
	Keep            KeepCmd               `cmd:"" help:"Google Keep (Workspace only)"`
	Sheets          SheetsCmd             `cmd:"" help:"Google Sheets"`
	Youtube         YoutubeCmd            `cmd:"" aliases:"yt" help:"YouTube"`
	Bigquery        BigqueryCmd           `cmd:"" aliases:"bq" help:"Google BigQuery"`
	Analytics       AnalyticsCmd          `cmd:"" aliases:"ga,ga4" help:"Google Analytics (GA4)"`
	SearchConsole   SearchConsoleCmd      `cmd:"" aliases:"gsc,sc" help:"Google Search Console"`
	TagManager      TagManagerCmd         `cmd:"" aliases:"gtm" help:"Google Tag Manager"`
	BusinessProfile BusinessProfileCmd    `cmd:"" aliases:"gbp,business" help:"Google Business Profile"`
	Config          ConfigCmd             `cmd:"" help:"Manage configuration"`
	Policy          PolicyCmd             `cmd:"" help:"Manage command safety policies"`
	Update          UpdateCmd             `cmd:"" help:"Update gog binary and companion Google skills"`
	Skills          SkillsCmd             `cmd:"" help:"Manage companion Google skill pack (pack skills only)"`
	VersionCmd      VersionCmd            `cmd:"" name:"version" help:"Print version"`
	Completion      CompletionCmd         `cmd:"" help:"Generate shell completion scripts"`
	Complete        CompletionInternalCmd `cmd:"" name:"__complete" hidden:"" help:"Internal completion helper"`
}

type exitPanic struct{ code int }

func Execute(args []string) (err error) {
	if hasVersionFlag(args) {
		mode, innerErr := outputModeFromVersionArgs(args)
		if innerErr != nil {
			return reportPreUIError(newUsageError(innerErr))
		}
		ctx := outfmt.WithMode(context.Background(), mode)
		return (&VersionCmd{}).Run(ctx)
	}

	parser, cli, err := newParser(rootHelpDescription())
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(exitPanic); ok {
				if ep.code == 0 {
					err = nil
					return
				}
				err = &ExitError{Code: ep.code, Err: errors.New("exited")}
				return
			}
			panic(r)
		}
	}()

	kctx, err := parser.Parse(args)
	if err != nil {
		parsedErr := wrapParseError(err)
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(parsedErr))
		return parsedErr
	}

	if err = enforceEnabledCommands(kctx, cli.EnableCommands); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}
	if err = enforceCommandPolicies(kctx, &cli.RootFlags); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}

	logLevel := slog.LevelWarn
	if cli.Verbose {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	mode, err := outfmt.FromFlags(cli.JSON, cli.Plain)
	if err != nil {
		return reportPreUIError(newUsageError(err))
	}

	ctx := context.Background()
	ctx = googleapi.WithReadOnly(ctx, cli.ReadOnly)
	if grants, preflightErr := preflightDeclaredWrite(kctx, &cli.RootFlags); preflightErr != nil && cli.Auth.Add.DriveScope != strFile {
		return reportPreUIError(preflightErr)
	} else if len(grants) != 0 {
		ctx = googleapi.WithReadOnlyWriteGrants(ctx, grants...)
		ctx = withReadOnlyApproval(ctx)
	}
	ctx = outfmt.WithMode(ctx, mode)
	ctx = authclient.WithClient(ctx, cli.Client)

	uiColor := cli.Color
	if outfmt.IsJSON(ctx) || outfmt.IsPlain(ctx) {
		uiColor = colorNever
	}

	u, err := ui.New(ui.Options{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Color:  uiColor,
	})
	if err != nil {
		return reportPreUIError(err)
	}
	ctx = ui.WithUI(ctx, u)

	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.Bind(&cli.RootFlags)

	// Throttled non-fatal update notice for humans and agents (stderr only).
	// Shell completion must stay fast/deterministic: never touch update cache
	// or the release service for completion-script generation or __complete.
	if !isCompletionCommand(kctx.Command()) {
		if notice := maybeNotifyUpdate(ctx, &selfupdate.Client{
			Repo:  strings.TrimSpace(os.Getenv("GOG_UPDATE_REPO")),
			Token: envOr("GITHUB_TOKEN", envOr("GH_TOKEN", "")),
		}, version, 0); notice != "" {
			_, _ = fmt.Fprintln(os.Stderr, notice)
		}
	}

	err = kctx.Run()
	if err == nil {
		return nil
	}
	if errors.Is(err, googleapi.ErrReadOnly) {
		err = &ExitError{Code: 2, Err: err}
	}

	if u := ui.FromContext(ctx); u != nil {
		u.Err().Error(errfmt.Format(err))
		return err
	}
	_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
	return err
}

func wrapParseError(err error) error {
	if err == nil {
		return nil
	}
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) {
		return &ExitError{Code: 2, Err: parseErr}
	}
	return err
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// isCompletionCommand reports whether the parsed kong command is shell-completion
// related and must bypass background update checks.
func isCompletionCommand(command string) bool {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case "completion", "__complete":
		return true
	default:
		return false
	}
}

func boolString(v bool) string {
	if v {
		return boolTrue
	}
	return boolFalse
}

func newParser(description string) (*kong.Kong, *CLI, error) {
	envMode := outfmt.FromEnv()
	vars := kong.Vars{
		"auth_services":    googleauth.UserServiceCSV(),
		"color":            envOr("GOG_COLOR", "auto"),
		"calendar_weekday": envOr("GOG_CALENDAR_WEEKDAY", "false"),
		"client":           envOr("GOG_CLIENT", ""),
		"enabled_commands": envOr("GOG_ENABLE_COMMANDS", ""),
		"json":             boolString(envMode.JSON),
		"plain":            boolString(envMode.Plain),
		"readonly":         boolString(envOr("GOG_READONLY", "") == "1"),
		"readonly_except":  envOr("GOG_READONLY_EXCEPT", ""),
		"version":          VersionString(),
	}

	cli := &CLI{}
	parser, err := kong.New(
		cli,
		kong.Name("gog"),
		kong.Description(description),
		kong.ConfigureHelp(helpOptions()),
		kong.Help(helpPrinter),
		kong.Vars(vars),
		kong.Writers(os.Stdout, os.Stderr),
		kong.Exit(func(code int) { panic(exitPanic{code: code}) }),
	)
	if err != nil {
		return nil, nil, err
	}
	return parser, cli, nil
}

func baseDescription() string {
	return "Google CLI for Gmail/Calendar/Chat/Classroom/Drive/Contacts/Tasks/Sheets/Docs/Slides/People/YouTube/BigQuery/Analytics/SearchConsole/TagManager/BusinessProfile"
}

func helpDescription() string {
	desc := baseDescription()

	configPath, err := config.ConfigPath()
	configLine := "unknown"
	if err != nil {
		configLine = fmt.Sprintf("error: %v", err)
	} else if configPath != "" {
		configLine = configPath
	}

	backendInfo, err := secrets.ResolveKeyringBackendInfo()
	var backendLine string
	if err != nil {
		backendLine = fmt.Sprintf("error: %v", err)
	} else if backendInfo.Value != "" {
		backendLine = fmt.Sprintf("%s (source: %s)", backendInfo.Value, backendInfo.Source)
	}

	return fmt.Sprintf("%s\n\nConfig:\n  file: %s\n  keyring backend: %s", desc, configLine, backendLine)
}

// newUsageError wraps errors in a way main() can map to exit code 2.
func newUsageError(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: 2, Err: err}
}

func reportPreUIError(err error) error {
	if err == nil {
		return nil
	}
	_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
	return err
}

func hasVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--version" {
			return true
		}
	}
	return false
}

func outputModeFromVersionArgs(args []string) (outfmt.Mode, error) {
	envMode := outfmt.FromEnv()
	jsonOut := envMode.JSON
	plainOut := envMode.Plain

	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch {
		case arg == "--json":
			jsonOut = true
		case arg == "--plain":
			plainOut = true
		case strings.HasPrefix(arg, "--json="):
			v, err := parseFlagBool(strings.TrimPrefix(arg, "--json="))
			if err != nil {
				return outfmt.Mode{}, err
			}
			jsonOut = v
		case strings.HasPrefix(arg, "--plain="):
			v, err := parseFlagBool(strings.TrimPrefix(arg, "--plain="))
			if err != nil {
				return outfmt.Mode{}, err
			}
			plainOut = v
		}
	}

	return outfmt.FromFlags(jsonOut, plainOut)
}

func parseFlagBool(value string) (bool, error) {
	return strconv.ParseBool(strings.TrimSpace(value))
}
