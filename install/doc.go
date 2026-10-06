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
//			UpdateKey: "…", // as gunimsign -keygen prints it
//		})
//		…
//	}
//
// A build from a working tree, whose version is "dev" or empty, runs as
// it is, with no installer; set GUNIM_INSTALL=show to see the installer
// all the same. A release built for each system and published as
// [GitHub] describes, with a SHA256SUMS of the programs signed with
// gunimsign, is then all the releasing there is.
//
// # Signed updates
//
// An installed program runs an update only when the release's
// SHA256SUMS carries a signature that App.UpdateKey checks, and it
// fetches only over https. The checksums say a download came whole; the
// signature says the program's maker made it, so a release anyone else
// puts up, with a stolen token or through a broken build, never runs.
//
// Once, for each program, make the key pair. This writes the private
// key to release.key, and prints the public key:
//
//	go run github.com/marrasen/gunim/tools/gunimsign -keygen release.key
//
// Put the public key in App.UpdateKey, and commit it: it is public. Keep
// the private key secret, and never commit it. Give it to the release
// build as a secret, on GitHub with
//
//	gh secret set GUNIM_SIGN_KEY < release.key
//
// and keep a copy somewhere safe, such as a password manager.
//
// For each release, write SHA256SUMS first, then sign it, and publish
// SHA256SUMS.sig beside it. In a GitHub Actions workflow, as a step after
// the one that writes dist/SHA256SUMS:
//
//	# Sign the checksums, for the installed copies' updates.
//	- name: Sign
//	  env:
//	    GUNIM_SIGN_KEY: ${{ secrets.GUNIM_SIGN_KEY }}
//	  run: go run github.com/marrasen/gunim/tools/gunimsign dist/SHA256SUMS
//
// gunimsign fails when GUNIM_SIGN_KEY is empty, so a build that cannot
// sign stops before it publishes. SHA256SUMS.sig stays out of
// SHA256SUMS: it is written after it.
//
// To change the key, publish one release signed with the old key that
// holds the new public key, and sign every release after it with the new
// key. A lost private key cannot be replaced that way: installed copies
// take no more updates, and their users download the next release by
// hand. A [Source] of the program's own sets [Release.Signature].
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
//   - How it keeps up to date: Updates, any [Source]; UpdateMode, the
//     mode until the user chooses, which [SetUpdates] changes, as an
//     Updates setting does: put a new release in place by itself, tell
//     the program through Available so it can ask first, or nothing; and
//     Updated, to hear of a release put in place and offer to [Restart]
//     into it. [Check] and [Stage] do the same at the program's asking,
//     as for a Check for Updates button.
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
