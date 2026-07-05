# Keep the fallback shell usable after Claude.
#
# Claude runs the terminal in raw mode with echo off — it draws its own input
# line, so you see what you type. If it exits WITHOUT cleanly restoring the
# terminal (killed, crashed mid-TUI, or a resumed session that inherited a raw
# baseline), the interactive bash it drops back to inherits that raw, no-echo
# terminal: keystrokes reach the shell and commands still run, but nothing you
# type is echoed — you only see the reaction after Enter. Reset the line
# discipline after every `claude` so bash always echoes again.
function claude() {
	command claude "$@"
	local rc=$?
	stty sane 2>/dev/null || true
	return "$rc"
}
