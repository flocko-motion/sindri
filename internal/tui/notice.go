// package: tui / notice
// type:    ui (startup notices)
// job:     the text of the one-off warning shown at startup when the project
//          expects a tool that isn't installed (currently: openspec).
// limits:  just the message; detection is in Run (-> tui.go) and rendering is the
//          warning modal (-> component_modal.go).
package tui

// openspecMissingNotice warns at startup when the project uses openspec (it has
// an openspec/ folder) but the CLI isn't installed — the folder is the signal
// the project depends on it, so the missing tool is worth surfacing rather than
// silently degrading. Without the folder the tool is optional and we say nothing.
const openspecMissingNotice = "This project uses openspec (there's an openspec/ folder), " +
	"but the openspec CLI isn't installed.\n\n" +
	"Spec validation and the openspec workflow are disabled until you install it:\n\n" +
	"  npm i -g @fission-ai/openspec"
