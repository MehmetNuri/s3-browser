package desktop

import "context"

type FileFilter struct {
	DisplayName string `json:"name"`
	Pattern     string `json:"pattern"`
}
type OpenDialogOptions struct {
	Title                string       `json:"title,omitempty"`
	CanCreateDirectories bool         `json:"canCreateDirectories,omitempty"`
	Filters              []FileFilter `json:"filters,omitempty"`
}
type SaveDialogOptions struct {
	Title           string       `json:"title,omitempty"`
	DefaultFilename string       `json:"defaultFilename,omitempty"`
	Filters         []FileFilter `json:"filters,omitempty"`
}

func OpenFileDialog(ctx context.Context, opts OpenDialogOptions) (string, error) {
	var p string
	err := Call(ctx, "openFile", opts, &p)
	return p, err
}
func OpenMultipleFilesDialog(ctx context.Context, opts OpenDialogOptions) ([]string, error) {
	var p []string
	err := Call(ctx, "openFiles", opts, &p)
	return p, err
}
func OpenDirectoryDialog(ctx context.Context, opts OpenDialogOptions) (string, error) {
	var p string
	err := Call(ctx, "openDirectory", opts, &p)
	return p, err
}
func SaveFileDialog(ctx context.Context, opts SaveDialogOptions) (string, error) {
	var p string
	err := Call(ctx, "saveFile", opts, &p)
	return p, err
}
func ClipboardSetText(ctx context.Context, text string) error {
	return Call(ctx, "clipboard", text, nil)
}
