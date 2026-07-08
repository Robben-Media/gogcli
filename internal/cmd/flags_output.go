package cmd

// OutputPathFlag is shared by download/export commands (sheets, docs, slides,
// drive, gmail attachments, chat media). When --out is empty, each command derives
// its own filename and writes to a command-specific download directory
// (e.g. <config>/drive-downloads, <config>/gmail-attachments, or the current
// directory for chat media), so the help text must not pin a single location.
type OutputPathFlag struct {
	Path string `name:"out" aliases:"output" help:"Output file path (default: derived filename in the command's download directory)"`
}

type OutputPathRequiredFlag struct {
	Path string `name:"out" aliases:"output" help:"Output file path (required)"`
}

type OutputDirFlag struct {
	Dir string `name:"out-dir" aliases:"output-dir" help:"Directory to write attachments to (default: current directory)"`
}
