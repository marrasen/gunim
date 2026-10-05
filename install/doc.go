// Package install makes a gunim program its own installer, so the
// program is all there is to download.
//
// Started from anywhere but its own folder, the program opens gunim's
// installer in place of its own window. The installer installs it for
// the user alone, with no administrator: into %LOCALAPPDATA%\Programs on
// Windows, with a Start menu shortcut and an entry under Installed apps
// that uninstalls it; into ~/.local/share on Linux, with a desktop file,
// its icon, and a link in ~/.local/bin. It offers a shortcut on the
// desktop, a start with the computer, the kinds of file the program
// opens, and to keep it up to date, and checks there is room first.
// Installed, the program starts as itself, and looks for newer releases
// now and then, which it puts in place for its next start.
//
// # The defaults
//
// A program that is happy with the defaults calls [Run] first in main,
// before it reads its flags:
//
//	var version = "dev" // set with -ldflags "-X main.version=v1.2.0"
//
//	func main() {
//		install.Run(install.App{
//			Name:    "Marras Mastering Studio",
//			Version: version,
//			Icon:    icon,
//			Updates: install.GitHub{Repo: "marrasen/mastering-studio"},
//		})
//		…
//	}
//
// A build from a working tree, whose version is "dev" or empty, runs as
// it is, with no installer; set GUNIM_INSTALL=show to see the installer
// all the same. A release built for each system and published as
// [GitHub] describes, with a SHA256SUMS of the programs, is then all the
// releasing there is.
//
// # Making it the program's own
//
// Every field of [App] past Name and Version changes one thing and
// leaves the rest:
//
//   - What the installer says and shows: Publisher, Description, URL,
//     Icon or IconFunc, [Look] and [Words].
//   - What it installs: Files, more files beside the program, as a
//     library it loads, and Dir, where it all goes.
//   - What it offers: FileTypes, Autostart, NoDesktop and Desktop, and
//     Choices of the program's own, whose answers reach Installed.
//   - What it does: the hooks Installed, Uninstalling and Quit, and
//     Data, the folders an uninstall offers to take too.
//   - How it keeps up to date: Updates, any [Source], and Updated, to
//     hear of a release put in place and offer to [Restart] into it.
//     [Check] and [Stage] do the same at the program's asking, as for a
//     Check for Updates button, and with NoAutoUpdate the program does
//     it all its own way.
//   - Where it was before: Formerly, the places an older version put
//     it. An install with no record of itself, as an older version's
//     own installer left it, is taken on as the program starts, with
//     what it has on the system.
//
// A program's own settings reach the install through [Find], which says
// what is installed and what it has, and [Change], which turns an offer
// on or off, as a Start with the Computer setting does.
//
// A program that wants a window of its own for all of it sets
// App.Window, and drives the [Session] it is given: the offers to show,
// [Session.Install] and [Session.Uninstall] with their progress, and
// [Session.Open] or [Session.RunHere] for what follows. A program can
// also make a Session itself with [NewSession], as for an Install item
// in its own menu.
//
// # From a script
//
// "program -install" installs with no window, with the choices made the
// last time or the defaults. "program -uninstall" asks in a window, as
// the system's list of installed programs starts it, and
// "program -uninstall -quiet" asks nothing.
//
// macOS is not done yet: there, Run lets the program run as it is.
package install
