package updater

import "golang.org/x/sys/windows"

// launch starts an installer the way Explorer does, so Windows shows a UAC
// prompt when the installer needs elevation instead of refusing to start it.
func launch(path string, args string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString(path)
	params, _ := windows.UTF16PtrFromString(args)
	return windows.ShellExecute(0, verb, file, params, nil, windows.SW_SHOWNORMAL)
}
