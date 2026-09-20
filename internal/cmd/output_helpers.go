package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var plainFieldReplacer = strings.NewReplacer(
	"\t", " ",
	"\r\n", " ",
	"\r", " ",
	"\n", " ",
)

func tableWriter(ctx context.Context) (io.Writer, func()) {
	if outfmt.IsPlain(ctx) {
		return os.Stdout, func() {}
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	return tw, func() { _ = tw.Flush() }
}

func writeTableRow(ctx context.Context, w io.Writer, fields []string) {
	fmt.Fprintln(w, strings.Join(plainTableFields(ctx, fields), "\t"))
}

func plainTableFields(ctx context.Context, fields []string) []string {
	if !outfmt.IsPlain(ctx) {
		return fields
	}
	fields = append([]string(nil), fields...)
	for i := range fields {
		fields[i] = sanitizePlainField(fields[i])
	}
	return fields
}

func sanitizePlainField(s string) string {
	return plainFieldReplacer.Replace(s)
}

// writePlainReceipt writes a fixed-schema TSV receipt (header + one data row)
// for --plain mutation output. Fields are sanitized for tab/newline safety.
func writePlainReceipt(ctx context.Context, headers []string, fields []string) {
	_ = writePlainReceiptError(ctx, headers, fields)
}

func writePlainReceiptError(ctx context.Context, headers []string, fields []string) error {
	w, flush := tableWriter(ctx)
	if _, err := fmt.Fprintln(w, strings.Join(plainTableFields(ctx, headers), "\t")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Join(plainTableFields(ctx, fields), "\t")); err != nil {
		return err
	}
	flush()
	return nil
}

// writePlainRoutingReceipt emits the shared Gmail routing mutation receipt:
// RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS (header + one row).
// resourceType/action are machine tokens (lowercase); status is a server
// verification/lifecycle value when available, otherwise "success".
func writePlainRoutingReceipt(ctx context.Context, resourceType, action, resource, status string) {
	if status == "" {
		status = "success"
	}
	writePlainReceipt(ctx,
		[]string{"RESOURCE_TYPE", "ACTION", "RESOURCE", "STATUS"},
		[]string{resourceType, action, resource, status},
	)
}

// writePlainSettingRows writes a multi-row SETTINGS receipt for Gmail account
// setting mutations under --plain. Header is always SETTING/FIELD/VALUE; each
// field is one data row. Values are sanitized for tab/newline safety.
func writePlainSettingRows(ctx context.Context, setting string, fields [][2]string) {
	w, flush := tableWriter(ctx)
	defer flush()
	writeTableRow(ctx, w, []string{"SETTING", "FIELD", "VALUE"})
	for _, field := range fields {
		writeTableRow(ctx, w, []string{setting, field[0], field[1]})
	}
}

func printNextPageHint(u *ui.UI, nextPageToken string) {
	printNextPageHintWithFlag(u, "--page", nextPageToken)
}

func printNextPageHintWithFlag(u *ui.UI, flagName string, nextPageToken string) {
	if u == nil || nextPageToken == "" {
		return
	}
	u.Err().Printf("# Next page: %s %s", flagName, nextPageToken)
}
