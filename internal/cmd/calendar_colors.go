package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarColorsCmd struct{}

func (c *CalendarColorsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	colors, err := svc.Colors.Get().Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"event":    colors.Event,
			"calendar": colors.Calendar,
		})
	}

	if outfmt.IsPlain(ctx) {
		printColorRows(os.Stdout, "event", colors.Event)
		printColorRows(os.Stdout, "calendar", colors.Calendar)
		return nil
	}

	if len(colors.Event) == 0 && len(colors.Calendar) == 0 {
		u.Err().Println("No colors available")
		return nil
	}

	if len(colors.Event) > 0 {
		fmt.Println("EVENT COLORS:")
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tBACKGROUND\tFOREGROUND")

		ids := make([]int, 0, len(colors.Event))
		for id := range colors.Event {
			if num, err := strconv.Atoi(id); err == nil {
				ids = append(ids, num)
			}
		}
		sort.Ints(ids)

		for _, num := range ids {
			id := strconv.Itoa(num)
			c := colors.Event[id]
			fmt.Fprintf(tw, "%s\t%s\t%s\n", id, c.Background, c.Foreground)
		}
		_ = tw.Flush()
		fmt.Println()
	}

	if len(colors.Calendar) > 0 {
		fmt.Println("CALENDAR COLORS:")
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tBACKGROUND\tFOREGROUND")

		ids := make([]int, 0, len(colors.Calendar))
		for id := range colors.Calendar {
			if num, err := strconv.Atoi(id); err == nil {
				ids = append(ids, num)
			}
		}
		sort.Ints(ids)

		for _, num := range ids {
			id := strconv.Itoa(num)
			c := colors.Calendar[id]
			fmt.Fprintf(tw, "%s\t%s\t%s\n", id, c.Background, c.Foreground)
		}
		_ = tw.Flush()
	}

	return nil
}

func printColorRows(w io.Writer, kind string, colors map[string]calendar.ColorDefinition) {
	ids := sortedColorIDs(colors)
	for _, id := range ids {
		c := colors[id]
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", kind, id, c.Background, c.Foreground)
	}
}

func sortedColorIDs(colors map[string]calendar.ColorDefinition) []string {
	numeric := make([]int, 0, len(colors))
	for id := range colors {
		if num, err := strconv.Atoi(id); err == nil {
			numeric = append(numeric, num)
		}
	}
	sort.Ints(numeric)

	ids := make([]string, 0, len(numeric))
	for _, num := range numeric {
		ids = append(ids, strconv.Itoa(num))
	}
	return ids
}
